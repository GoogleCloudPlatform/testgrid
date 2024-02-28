/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package updater reads the latest test results and saves updated state.
package updater

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/testgrid/config"
	"github.com/GoogleCloudPlatform/testgrid/config/snapshot"
	"github.com/GoogleCloudPlatform/testgrid/internal/result"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	statuspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/GoogleCloudPlatform/testgrid/util/metrics"
	"github.com/fvbommel/sortorder"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/sirupsen/logrus"
)

const componentName = "updater"

// Metrics holds metrics relevant to the Updater.
type Metrics struct {
	UpdateState  metrics.Cyclic
	DelaySeconds metrics.Duration
}

// CreateMetrics creates metrics for this controller
func CreateMetrics(factory metrics.Factory) *Metrics {
	return &Metrics{
		UpdateState:  factory.NewCyclic(componentName),
		DelaySeconds: factory.NewDuration("delay", "Seconds updater is behind schedule", "component"),
	}
}

func (mets *Metrics) delay(dur time.Duration) {
	if mets == nil {
		return
	}
	mets.DelaySeconds.Set(dur, componentName)
}

func (mets *Metrics) start() *metrics.CycleReporter {
	if mets == nil {
		return nil
	}
	return mets.UpdateState.Start()
}

// GroupUpdater will compile the grid state proto for the specified group and upload it.
//
// This typically involves downloading the existing state, dropping old columns,
// compiling any new columns and inserting them into the front and then uploading
// the proto to GCS.
//
// Disable pooled downloads with a nil poolCtx, otherwise at most concurrency builds
// will be downloaded at the same time.
//
// Return true if there are more results to process.
type GroupUpdater func(parent context.Context, log logrus.FieldLogger, client gcs.Client, tg *configpb.TestGroup, gridPath gcs.Path) (bool, error)

// GCS returns a GCS-based GroupUpdater, which knows how to process result data stored in GCS.
func GCS(poolCtx context.Context, colClient gcs.Client, groupTimeout, buildTimeout time.Duration, concurrency int, write bool, enableIgnoreSkip bool) GroupUpdater {
	var readResult *resultReader
	if poolCtx == nil {
		// TODO(fejta): remove check soon
		panic("Context must be non-nil")
	}
	readResult = resultReaderPool(poolCtx, logrus.WithField("pool", "readResult"), concurrency)

	return func(parent context.Context, log logrus.FieldLogger, client gcs.Client, tg *configpb.TestGroup, gridPath gcs.Path) (bool, error) {
		if !tg.UseKubernetesClient && (tg.ResultSource == nil || tg.ResultSource.GetGcsConfig() == nil) {
			log.Debug("Skipping non-kubernetes client group")
			return false, nil
		}
		ctx, cancel := context.WithTimeout(parent, groupTimeout)
		defer cancel()
		gcsColReader := gcsColumnReader(colClient, buildTimeout, readResult, enableIgnoreSkip)
		reprocess := 20 * time.Minute // allow 20m for prow to finish uploading artifacts
		return InflateDropAppend(ctx, log, client, tg, gridPath, write, gcsColReader, reprocess)
	}
}

func gridPaths(configPath gcs.Path, gridPrefix string, groups []*configpb.TestGroup) ([]gcs.Path, error) {
	paths := make([]gcs.Path, 0, len(groups))
	for _, tg := range groups {
		tgp, err := TestGroupPath(configPath, gridPrefix, tg.Name)
		if err != nil {
			return nil, fmt.Errorf("%s bad group path: %w", tg.Name, err)
		}
		paths = append(paths, *tgp)
	}
	return paths, nil
}

// lockGroup makes a conditional GCS write operation to ensure it has authority to update this object.
//
// This allows multiple decentralized updaters to collaborate on updating groups:
// Regardless of how many updaters are trying to concurrently update an object foo at generation X, GCS
// will only allow one of them to "win". The others receive a PreconditionFailed error and can
// move onto the next group.
func lockGroup(ctx context.Context, client gcs.ConditionalClient, path gcs.Path, generation int64) (*storage.ObjectAttrs, error) {
	var buf []byte
	if generation == 0 {
		var grid statepb.Grid
		var err error
		if buf, err = gcs.MarshalGrid(&grid); err != nil {
			return nil, fmt.Errorf("marshal: %w", err)
		}
	}

	return gcs.Touch(ctx, client, path, generation, buf)
}

func testGroups(cfg *snapshot.Config, groupNames ...string) ([]*configpb.TestGroup, error) {
	var groups []*configpb.TestGroup

	if len(groupNames) == 0 {
		groups = make([]*configpb.TestGroup, 0, len(groupNames))
		for _, testConfig := range cfg.Groups {
			groups = append(groups, testConfig)
		}
		return groups, nil
	}

	groups = make([]*configpb.TestGroup, 0, len(groupNames))
	for _, groupName := range groupNames {
		tg := cfg.Groups[groupName]
		if tg == nil {
			return nil, fmt.Errorf("group %q not found", groupName)
		}
		groups = append(groups, tg)
	}
	return groups, nil
}

type lastUpdated struct {
	client     gcs.ConditionalClient
	gridPrefix string
	configPath gcs.Path
	freq       time.Duration
}

func (fixer lastUpdated) fixOnce(ctx context.Context, log logrus.FieldLogger, q *config.TestGroupQueue, groups []*configpb.TestGroup) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	paths, err := gridPaths(fixer.configPath, fixer.gridPrefix, groups)
	if err != nil {
		return err
	}
	attrs := gcs.StatExisting(ctx, log, fixer.client, paths...)
	updates := make(map[string]time.Time, len(attrs))
	var wg sync.WaitGroup
	for i, attr := range attrs {
		if attr == nil {
			continue
		}
		name := groups[i].Name
		if attr.Generation > 0 {
			updates[name] = attr.Updated.Add(fixer.freq)
		} else if attr.Generation == 0 {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				if _, err := lockGroup(ctx, fixer.client, paths[i], 0); err != nil && !gcs.IsPreconditionFailed(err) {
					log.WithError(err).Error("Failed to create empty group state")
				}
			}(i)
			updates[name] = time.Now()
		}
	}
	wg.Wait()
	q.Init(log, groups, time.Now().Add(fixer.freq))
	q.FixAll(updates, false)
	return nil
}

func (fixer lastUpdated) Fix(ctx context.Context, log logrus.FieldLogger, q *config.TestGroupQueue, groups []*configpb.TestGroup) error {
	if fixer.freq == 0 {
		return nil
	}
	ticker := time.NewTicker(fixer.freq)
	fix := func() {
		if err := fixer.fixOnce(ctx, log, q, groups); err != nil {
			log.WithError(err).Warning("Failed to fix groups based on last update time")
		}
	}
	fix()

	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return ctx.Err()
		case <-ticker.C:
			fix()
		}
	}
}

// Fixer will fix the TestGroupQueue's next time for TestGroups.
//
// Fixer should:
// * work continually and not return until the context expires.
// * expect to be called multiple times with different contexts and test groups.
//
// For example, it might use the last updated time of the test group to
// specify the next update time. Or it might watch the data backing these groups and
// request an immediate update whenever the data changes.
type Fixer func(context.Context, logrus.FieldLogger, *config.TestGroupQueue, []*configpb.TestGroup) error

// UpdateOptions aggregates the Update function parameter into a single structure.
type UpdateOptions struct {
	ConfigPath       gcs.Path
	GridPrefix       string
	GroupConcurrency int
	GroupNames       []string
	Write            bool
	Freq             time.Duration
}

// Update test groups with the specified freq.
//
// Retries errors at double and unfinished groups as soon as possible.
//
// Filters down to a single group when set.
// Returns after all groups updated once if freq is zero.
func Update(parent context.Context, client gcs.ConditionalClient, mets *Metrics, updateGroup GroupUpdater, opts *UpdateOptions, fixers ...Fixer) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	log := logrus.WithField("config", opts.ConfigPath)

	var q config.TestGroupQueue

	log.Debug("Observing config...")
	newConfig, err := snapshot.Observe(ctx, log, client, opts.ConfigPath, time.NewTicker(time.Minute).C)
	if err != nil {
		return fmt.Errorf("observe config: %w", err)

	}
	cfg := <-newConfig
	groups, err := testGroups(cfg, opts.GroupNames...)
	if err != nil {
		return fmt.Errorf("filter test groups: %w", err)
	}

	q.Init(log, groups, time.Now().Add(opts.Freq))

	log.Debug("Fetching initial start times...")
	fixLastUpdated := lastUpdated{
		client:     client,
		gridPrefix: opts.GridPrefix,
		configPath: opts.ConfigPath,
		freq:       opts.Freq,
	}
	if err := fixLastUpdated.fixOnce(ctx, log, &q, groups); err != nil {
		return fmt.Errorf("get generations: %v", err)
	}
	log.Info("Fetched initial start times")

	fixers = append(fixers, fixLastUpdated.Fix)

	go func() {
		ticker := time.NewTicker(time.Minute)
		log := log
		for {
			depth, next, when := q.Status()
			log := log.WithField("depth", depth)
			if next != nil {
				log = log.WithField("next", next.Name)
			}
			delay := time.Since(when)
			if delay < 0 {
				delay = 0
				log = log.WithField("sleep", -delay)
			}
			if max := opts.Freq * 2; max > 0 && delay > max {
				delay = max
			}
			log = log.WithField("delay", delay.Round(time.Second))
			mets.delay(delay)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				log.Info("Queue Status")
			}
		}
	}()

	go func() {
		fixCtx, fixCancel := context.WithCancel(ctx)
		var fixWg sync.WaitGroup
		fixAll := func() {
			n := len(fixers)
			log.WithField("fixers", n).Trace("Starting fixers on current test groups...")
			fixWg.Add(n)
			for i, fix := range fixers {
				go func(i int, fix Fixer) {
					defer fixWg.Done()
					if err := fix(fixCtx, log, &q, groups); err != nil && !errors.Is(err, context.Canceled) {
						log.WithError(err).WithField("fixer", i).Warning("Fixer failed")
					}
				}(i, fix)
			}
			log.Debug("Started fixers on current test groups")
		}
		fixAll()
		for {
			select {
			case <-ctx.Done():
				fixCancel()
				return
			case cfg, ok := <-newConfig:
				if !ok {
					fixCancel()
					return
				}
				log.Info("Updating config")
				groups, err = testGroups(cfg, opts.GroupNames...)
				if err != nil {
					log.Errorf("Error during config update: %v", err)
				}
				log.Debug("Cancelling fixers on old test groups...")
				fixCancel()
				fixWg.Wait()
				q.Init(log, groups, time.Now().Add(opts.Freq))
				log.Debug("Canceled fixers on old test groups")
				fixCtx, fixCancel = context.WithCancel(ctx)
				fixAll()
			}
		}
	}()

	active := map[string]bool{}
	var lock sync.RWMutex
	var wg sync.WaitGroup
	wg.Add(opts.GroupConcurrency)
	defer wg.Wait()
	channel := make(chan *configpb.TestGroup)
	defer close(channel)

	updateTestGroup := func(tg *configpb.TestGroup) {
		name := tg.Name
		log := log.WithField("group", name)
		lock.RLock()
		on := active[name]
		lock.RUnlock()
		if on {
			log.Debug("Already updating...")
			return
		}
		fin := mets.start()
		tgp, err := TestGroupPath(opts.ConfigPath, opts.GridPrefix, name)
		if err != nil {
			fin.Fail()
			log.WithError(err).Error("Bad path")
			return
		}
		lock.Lock()
		if active[name] {
			lock.Unlock()
			log.Debug("Another routine started updating...")
			return
		}
		active[name] = true
		lock.Unlock()
		defer func() {
			lock.Lock()
			active[name] = false
			lock.Unlock()
		}()
		start := time.Now()
		unprocessed, err := updateGroup(ctx, log, client, tg, *tgp)
		log.WithField("duration", time.Since(start)).Info("Finished processing group.")
		if err != nil {
			log := log.WithError(err)
			if gcs.IsPreconditionFailed(err) {
				fin.Skip()
				log.Info("Group was modified while updating")
			} else {
				fin.Fail()
				log.Error("Failed to update group")
			}
			var delay time.Duration
			if opts.Freq > 0 {
				delay = opts.Freq/4 + time.Duration(rand.Int63n(int64(opts.Freq/4))) // Int63n() panics if freq <= 0
				log = log.WithField("delay", delay.Seconds())
				q.Fix(tg.Name, time.Now().Add(delay), true)
			}
			return
		}
		fin.Success()
		if unprocessed { // process another chunk ASAP
			q.Fix(name, time.Now(), false)
		}
	}

	for i := 0; i < opts.GroupConcurrency; i++ {
		go func() {
			defer wg.Done()
			for tg := range channel {
				updateTestGroup(tg)
			}
		}()
	}

	log.Info("Starting to process test groups...")
	return q.Send(ctx, channel, opts.Freq)
}

// TestGroupPath returns the path to a test_group proto given this proto
func TestGroupPath(g gcs.Path, gridPrefix, groupName string) (*gcs.Path, error) {
	name := path.Join(gridPrefix, groupName)
	u, err := url.Parse(name)
	if err != nil {
		return nil, fmt.Errorf("invalid url %s: %w", name, err)
	}
	np, err := g.ResolveReference(u)
	if err != nil {
		return nil, fmt.Errorf("resolve reference: %w", err)
	}
	if np.Bucket() != g.Bucket() {
		return nil, fmt.Errorf("testGroup %s should not change bucket", name)
	}
	return np, nil
}

func gcsPrefix(tg *configpb.TestGroup) string {
	if tg.ResultSource == nil {
		return tg.GcsPrefix
	}
	if gcsCfg := tg.ResultSource.GetGcsConfig(); gcsCfg != nil {
		return gcsCfg.GcsPrefix
	}
	return tg.GcsPrefix
}

func groupPaths(tg *configpb.TestGroup) ([]gcs.Path, error) {
	var out []gcs.Path
	prefixes := strings.Split(gcsPrefix(tg), ",")
	for idx, prefix := range prefixes {
		prefix := strings.TrimSpace(prefix)
		if prefix == "" {
			continue
		}
		u, err := url.Parse("gs://" + prefix)
		if err != nil {
			return nil, fmt.Errorf("parse: %w", err)
		}
		if u.Path != "" && u.Path[len(u.Path)-1] != '/' {
			u.Path += "/"
		}

		var p gcs.Path
		if err := p.SetURL(u); err != nil {
			if idx > 0 {
				return nil, fmt.Errorf("%d: %s: %w", idx, prefix, err)
			}
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// truncateRunning filters out all columns until the oldest still running column.
//
// If there are 20 columns where all are complete except the 3rd and 7th, this will
// return the 8th and later columns.
//
// Running columns more than 3 days old are not considered.
func truncateRunning(cols []InflatedColumn, floorTime time.Time) []InflatedColumn {
	if len(cols) == 0 {
		return cols
	}

	floor := float64(floorTime.UTC().Unix() * 1000)

	for i := len(cols) - 1; i >= 0; i-- {
		if cols[i].Column.Started < floor {
			continue
		}
		for _, cell := range cols[i].Cells {
			if cell.Result == statuspb.TestStatus_RUNNING {
				return cols[i+1:]
			}
		}
	}
	// No cells are found to be running; do not truncate
	return cols
}

func listBuilds(ctx context.Context, client gcs.Lister, since string, paths ...gcs.Path) ([]gcs.Build, error) {
	var out []gcs.Build

	for idx, tgPath := range paths {
		var offset *gcs.Path
		var err error
		if since != "" {
			if !strings.HasSuffix(since, "/") {
				since = since + "/"
			}
			if offset, err = tgPath.ResolveReference(&url.URL{Path: since}); err != nil {
				return nil, fmt.Errorf("resolve since: %w", err)
			}
		}
		builds, err := gcs.ListBuilds(ctx, client, tgPath, offset)
		if err != nil {
			return nil, fmt.Errorf("%d: %s: %w", idx, tgPath, err)
		}
		out = append(out, builds...)
	}

	if len(paths) > 1 {
		gcs.Sort(out)
	}

	return out, nil
}

// ColumnReader finds, processes and new columns to send to the receivers.
//
// * Columns with the same Name and Build will get merged together.
// * Readers must be reentrant.
//   - Processing must expect every sent column to be the final column this cycle.
//     AKA calling this method once and reading two columns should be equivalent to
//     calling the method once, reading one column and then calling it a second time
//     and reading a second column.
type ColumnReader func(ctx context.Context, log logrus.FieldLogger, tg *configpb.TestGroup, oldCols []InflatedColumn, stop time.Time, receivers chan<- InflatedColumn) error

// SortStarted sorts InflatedColumns by column start time.
func SortStarted(cols []InflatedColumn) {
	sort.SliceStable(cols, func(i, j int) bool {
		return cols[i].Column.Started > cols[j].Column.Started
	})
}

const byteCeiling = 2e6 // 2 megabytes

// InflateDropAppend updates groups by downloading the existing grid, dropping old rows and appending new ones.
func InflateDropAppend(ctx context.Context, alog logrus.FieldLogger, client gcs.Client, tg *configpb.TestGroup, gridPath gcs.Path, write bool, readCols ColumnReader, reprocess time.Duration) (bool, error) {
	log := alog.(logrus.Ext1FieldLogger) // Add trace method
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Grace period to read additional column.
	var grace context.Context
	if deadline, present := ctx.Deadline(); present {
		var cancel context.CancelFunc
		dur := time.Until(deadline) / 2
		grace, cancel = context.WithTimeout(context.Background(), dur)
		defer cancel()
	} else {
		grace = context.Background()
	}

	var shrinkGrace context.Context
	if deadline, present := ctx.Deadline(); present {
		var cancel context.CancelFunc
		dur := 3 * time.Until(deadline) / 4
		shrinkGrace, cancel = context.WithTimeout(context.Background(), dur)
		defer cancel()
	} else {
		shrinkGrace = context.Background()
	}

	var dur time.Duration
	if tg.DaysOfResults > 0 {
		dur = days(float64(tg.DaysOfResults))
	} else {
		dur = days(7)
	}

	stop := time.Now().Add(-dur)
	log = log.WithField("stop", stop)

	var oldCols []InflatedColumn
	var issues map[string][]string

	log.Trace("Downloading existing grid...")
	old, attrs, err := gcs.DownloadGrid(ctx, client, gridPath)
	if err != nil {
		log.WithField("path", gridPath).WithError(err).Error("Failed to download existing grid")
	}
	inflateStart := time.Now()
	if old != nil {
		var cols []InflatedColumn
		var err error
		log.Trace("Inflating grid...")
		if cols, issues, err = InflateGrid(ctx, old, stop, time.Now().Add(-reprocess)); err != nil {
			return false, fmt.Errorf("inflate: %w", err)
		}
		var floor time.Time
		when := time.Now().Add(-7 * 24 * time.Hour)
		if col := reprocessColumn(log, old, tg, when); col != nil {
			cols = append(cols, *col)
			floor = when
		}
		SortStarted(cols) // Our processing requires descending start time.
		oldCols = truncateRunning(cols, floor)
	}
	inflateDur := time.Since(inflateStart)
	readColsStart := time.Now()
	var cols []InflatedColumn
	var unreadColumns bool
	if attrs != nil && attrs.Size >= int64(byteCeiling) {
		log.WithField("size", attrs.Size).Info("Grid too large, compressing...")
		unreadColumns = true
		cols = oldCols
	} else {
		if condClient, ok := client.(gcs.ConditionalClient); ok {
			var cond storage.Conditions
			if attrs == nil {
				cond.DoesNotExist = true
			} else {
				cond.GenerationMatch = attrs.Generation
			}
			client = condClient.If(&cond, &cond)
		}

		newCols := make(chan InflatedColumn)
		ec := make(chan error)

		log.Trace("Reading first column...")
		go func() {
			err := readCols(ctx, log, tg, oldCols, stop, newCols)
			select {
			case <-ctx.Done():
			case ec <- err:
			}
		}()

		// Must read at least one column every cycle to ensure we make forward progress.
		more := true
		select {
		case <-ctx.Done():
			return false, fmt.Errorf("first column: %w", ctx.Err())
		case col := <-newCols:
			if len(col.Cells) == 0 {
				// Group all empty columns together by setting build/name empty.
				col.Column.Build = ""
				col.Column.Name = ""
			}
			cols = append(cols, col)
		case err := <-ec:
			if err != nil {
				return false, fmt.Errorf("read first column: %w", err)
			}
			more = false
		}

		// Read as many additional columns as we can within the allocated time.
		log.Trace("Reading additional columns...")
		for more {
			select {
			case <-grace.Done():
				unreadColumns = true
				more = false
			case <-ctx.Done():
				return false, ctx.Err()
			case col := <-newCols:
				if len(col.Cells) == 0 {
					// Group all empty columns together by setting build/name empty.
					col.Column.Build = ""
					col.Column.Name = ""
				}
				cols = append(cols, col)
			case err := <-ec:
				if err != nil {
					return false, fmt.Errorf("read columns: %w", err)
				}
				more = false
			}
		}

		log = log.WithField("appended", len(cols))

		overrideBuild(tg, cols) // so we group correctly
		cols = append(cols, oldCols...)
		cols = groupColumns(tg, cols)
	}
	readColsDur := time.Since(readColsStart)

	SortStarted(cols)

	shrinkStart := time.Now()
	cols = truncateGrid(cols, byteCeiling) // Assume each cell is at least 1 byte
	var grid *statepb.Grid
	var buf []byte
	grid, buf, err = shrinkGridInline(shrinkGrace, log, tg, cols, issues, byteCeiling)
	if err != nil {
		return false, fmt.Errorf("shrink grid inline: %v", err)
	}
	shrinkDur := time.Since(shrinkStart)

	grid.Config = tg

	log = log.WithField("url", gridPath).WithField("bytes", len(buf))
	if !write {
		log = log.WithField("dryrun", true)
	} else {
		log.Debug("Writing grid...")
		// TODO(fejta): configurable cache value
		if _, err := client.Upload(ctx, gridPath, buf, gcs.DefaultACL, gcs.NoCache); err != nil {
			return false, fmt.Errorf("upload %d bytes: %w", len(buf), err)
		}
	}
	if unreadColumns {
		log = log.WithField("more", true)
	}
	log.WithFields(logrus.Fields{
		"cols":     len(grid.Columns),
		"rows":     len(grid.Rows),
		"inflate":  inflateDur,
		"readCols": readColsDur,
		"shrink":   shrinkDur,
	}).Info("Wrote grid")
	return unreadColumns, nil
}

// truncateGrid cuts grid down to 'cellCeiling' or fewer cells
// Used as a cheap way to truncate before the finer-tuned shrinkGridInline.
func truncateGrid(cols []InflatedColumn, cellCeiling int) []InflatedColumn {
	var cells int
	for i := 0; i < len(cols); i++ {
		nc := len(cols[i].Cells)
		cells += nc
		if i < 2 || cells <= cellCeiling {
			continue
		}
		return cols[:i]
	}
	return cols
}

// reprocessColumn returns a column with a running result if the previous config differs from the current one
func reprocessColumn(log logrus.FieldLogger, old *statepb.Grid, currentCfg *configpb.TestGroup, when time.Time) *InflatedColumn {
	if old.Config == nil || old.Config.String() == currentCfg.String() {
		return nil
	}

	log.WithField("since", when.Round(time.Minute)).Info("Reprocessing results after changed config")

	return &InflatedColumn{
		Column: &statepb.Column{
			Started: float64(when.UTC().Unix() * 1000),
		},
		Cells: map[string]Cell{
			"reprocess": {
				Result: statuspb.TestStatus_RUNNING,
			},
		},
	}
}

func shrinkGridInline(ctx context.Context, log logrus.FieldLogger, tg *configpb.TestGroup, cols []InflatedColumn, issues map[string][]string, byteCeiling int) (*statepb.Grid, []byte, error) {
	// Hopefully the grid is small enough...
	grid := constructGridFromGroupConfig(log, tg, cols, issues)
	buf, err := gcs.MarshalGrid(grid)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal grid: %w", err)
	}
	orig := len(buf)
	if byteCeiling == 0 || orig < byteCeiling {
		return grid, buf, nil
	}

	// Nope, let's drop old data...
	newCeiling := byteCeiling / 2

	log = log.WithField("originally", orig)
	for i := len(cols) / 2; i > 0; i = i / 2 {
		select {
		case <-ctx.Done():
			log.WithField("size", len(buf)).Info("Timeout shrinking row data")
			return grid, buf, nil
		default:
		}

		log.WithField("size", len(buf)).Debug("Shrinking row data")

		// shrink cols to half and cap
		truncateLastColumn(cols[0:i], orig, byteCeiling, "byte")

		grid = constructGridFromGroupConfig(log, tg, cols[0:i], issues)
		buf, err = gcs.MarshalGrid(grid)
		if err != nil {
			return nil, nil, fmt.Errorf("marshal grid: %w", err)
		}

		if len(buf) < newCeiling {
			log.WithField("size", len(buf)).Info("Shrunk row data")
			return grid, buf, nil
		}

	}

	// One column isn't small enough. Return a single-cell grid.
	grid = constructGridFromGroupConfig(log, tg, deletedColumn(cols[0]), nil)
	buf, err = gcs.MarshalGrid(grid)
	log.WithField("size", len(buf)).Info("Shrunk to minimum; storing metadata only")
	return grid, buf, err
}

// Legacy row name to report data truncation
const truncatedRowName = "Truncated"

func truncateLastColumn(grid []InflatedColumn, orig, max int, entity string) {
	if len(grid) == 0 {
		return
	}
	last := len(grid) - 1
	for name, cell := range grid[last].Cells {
		if name == truncatedRowName {
			delete(grid[last].Cells, truncatedRowName)
			continue
		}
		if cell.Result == statuspb.TestStatus_NO_RESULT {
			continue
		}
		cell.Result = statuspb.TestStatus_UNKNOWN
		cell.Message = fmt.Sprintf("%d %s grid exceeds maximum size of %d %ss", orig, entity, max, entity)
		cell.Icon = "..." // Overwritten by the UI
		grid[last].Cells[name] = cell
	}
}

// A column with the same header data, but all the rows deleted.
func deletedColumn(latestColumn InflatedColumn) []InflatedColumn {
	return []InflatedColumn{
		{
			Column: latestColumn.Column,
			Cells: map[string]Cell{
				truncatedRowName: {
					Result:  statuspb.TestStatus_UNKNOWN,
					ID:      truncatedRowName,
					Message: fmt.Sprintf("The grid is too large to update. Split this testgroup into multiple testgroups."),
				},
			},
		},
	}
}

// FormatStrftime replaces python codes with what go expects.
//
// aka %Y-%m-%d becomes 2006-01-02
func FormatStrftime(in string) string {
	replacements := map[string]string{
		"%p": "PM",
		"%Y": "2006",
		"%y": "06",
		"%m": "01",
		"%d": "02",
		"%H": "15",
		"%M": "04",
		"%S": "05",
	}

	out := in

	for bad, good := range replacements {
		out = strings.ReplaceAll(out, bad, good)
	}
	return out
}

func overrideBuild(tg *configpb.TestGroup, cols []InflatedColumn) {
	fmt := tg.BuildOverrideStrftime
	if fmt == "" {
		return
	}
	fmt = FormatStrftime(fmt)
	for _, col := range cols {
		started := int64(col.Column.Started)
		when := time.Unix(started/1000, (started%1000)*int64(time.Millisecond/time.Nanosecond))
		col.Column.Build = when.Format(fmt)
	}
}

const columnIDSeparator = "\ue000"

// GroupColumns merges columns with the same Name and Build.
//
// Cells are joined together, splitting those with the same name.
// Started is the smallest value.
// Extra is the most recent filled value.
func groupColumns(tg *configpb.TestGroup, cols []InflatedColumn) []InflatedColumn {
	groups := map[string][]InflatedColumn{}
	var ids []string
	for _, c := range cols {
		id := c.Column.Name + columnIDSeparator + c.Column.Build
		groups[id] = append(groups[id], c)
		ids = append(ids, id)
	}

	if len(groups) == 0 {
		return nil
	}

	out := make([]InflatedColumn, 0, len(groups))

	seen := make(map[string]bool, len(groups))

	for _, id := range ids {
		if seen[id] {
			continue // already merged this group.
		}
		seen[id] = true
		var col InflatedColumn

		groupedCells := groups[id]
		if len(groupedCells) == 1 {
			out = append(out, groupedCells[0])
			continue
		}

		cells := map[string][]Cell{}

		var count int
		for i, c := range groupedCells {
			if i == 0 {
				col.Column = c.Column
			} else {
				if c.Column.Started < col.Column.Started {
					col.Column.Started = c.Column.Started
				}
				if !sortorder.NaturalLess(c.Column.Hint, col.Column.Hint) {
					col.Column.Hint = c.Column.Hint
				}
				for i, val := range c.Column.Extra {
					if i == len(col.Column.Extra) {
						col.Column.Extra = append(col.Column.Extra, val)
						continue
					}
					if val == "" || val == col.Column.Extra[i] {
						continue
					}
					if col.Column.Extra[i] == "" {
						col.Column.Extra[i] = val
					} else if i < len(tg.GetColumnHeader()) && tg.GetColumnHeader()[i].ListAllValues {
						col.Column.Extra[i] = joinHeaders(col.Column.Extra[i], val)
					} else {
						col.Column.Extra[i] = "*" // values differ
					}
				}
			}
			for key, cell := range c.Cells {
				cells[key] = append(cells[key], cell)
				count++
			}
		}
		if tg.IgnoreOldResults {
			col.Cells = make(map[string]Cell, len(cells))
		} else {
			col.Cells = make(map[string]Cell, count)
		}
		for name, duplicateCells := range cells {
			if tg.IgnoreOldResults {
				col.Cells[name] = duplicateCells[0]
				continue
			}
			for name, cell := range SplitCells(name, duplicateCells...) {
				col.Cells[name] = cell
			}
		}
		out = append(out, col)
	}
	return out
}

func joinHeaders(headers ...string) string {
	headerSet := make(map[string]bool)
	for _, header := range headers {
		vals := strings.Split(header, "||")
		for _, val := range vals {
			if val == "" {
				continue
			}
			headerSet[val] = true
		}
	}
	keys := []string{}
	for k := range headerSet {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, "||")
}

// days converts days float into a time.Duration, assuming a 24 hour day.
//
// A day is not always 24 hours due to things like leap-seconds.
// We do not need this level of precision though, so ignore the complexity.
func days(d float64) time.Duration {
	return time.Duration(24*d) * time.Hour // Close enough
}

// ConstructGrid will append all the inflatedColumns into the returned Grid.
//
// The returned Grid has correctly compressed row values.
func ConstructGrid(log logrus.FieldLogger, cols []InflatedColumn, issues map[string][]string, failuresToAlert, passesToDisableAlert int, useCommitAsBuildID bool, userProperty string, brokenThreshold float32, columnHeader []*configpb.TestGroup_ColumnHeader) *statepb.Grid {
	// Add the columns into a grid message
	var grid statepb.Grid
	rows := map[string]*statepb.Row{} // For fast target => row lookup
	if failuresToAlert > 0 && passesToDisableAlert == 0 {
		passesToDisableAlert = 1
	}

	for _, col := range cols {
		if brokenThreshold > 0.0 && col.Column != nil {
			col.Column.Stats = columnStats(col.Cells, brokenThreshold)
		}
		AppendColumn(&grid, rows, col)
	}

	dropEmptyRows(log, &grid, rows)

	for name, row := range rows {
		row.Issues = append(row.Issues, issues[name]...)
		issueSet := make(map[string]bool, len(row.Issues))
		for _, i := range row.Issues {
			issueSet[i] = true
		}
		row.Issues = make([]string, 0, len(issueSet))
		for i := range issueSet {
			row.Issues = append(row.Issues, i)
		}
		sort.SliceStable(row.Issues, func(i, j int) bool {
			// Largest issues at the front of the list
			return !sortorder.NaturalLess(row.Issues[i], row.Issues[j])
		})
	}

	alertRows(grid.Columns, grid.Rows, failuresToAlert, passesToDisableAlert, useCommitAsBuildID, userProperty, columnHeader)
	sort.SliceStable(grid.Rows, func(i, j int) bool {
		return sortorder.NaturalLess(grid.Rows[i].Name, grid.Rows[j].Name)
	})

	for _, row := range grid.Rows {
		del := true
		for _, up := range row.UserProperty {
			if up != "" {
				del = false
				break
			}
		}
		if del {
			row.UserProperty = nil
		}
		sort.SliceStable(row.Metric, func(i, j int) bool {
			return sortorder.NaturalLess(row.Metric[i], row.Metric[j])
		})
		sort.SliceStable(row.Metrics, func(i, j int) bool {
			return sortorder.NaturalLess(row.Metrics[i].Name, row.Metrics[j].Name)
		})
	}
	return &grid
}

// constructGridFromGroupConfig will append all the inflatedColumns into the returned Grid.
//
// The returned Grid has correctly compressed row values.
func constructGridFromGroupConfig(log logrus.FieldLogger, group *configpb.TestGroup, cols []InflatedColumn, issues map[string][]string) *statepb.Grid {
	usesK8sClient := group.UseKubernetesClient || (group.GetResultSource().GetGcsConfig() != nil)
	return ConstructGrid(log, cols, issues, int(group.GetNumFailuresToAlert()), int(group.GetNumPassesToDisableAlert()), usesK8sClient, group.GetUserProperty(), 0.0, group.GetColumnHeader())
}

func dropEmptyRows(log logrus.FieldLogger, grid *statepb.Grid, rows map[string]*statepb.Row) {
	filled := make([]*statepb.Row, 0, len(rows))
	var dropped int
	for _, r := range grid.Rows {
		var found bool
		f := result.Iter(r.Results)
		for {
			res, more := f()
			if !more {
				break
			}
			if res == statuspb.TestStatus_NO_RESULT {
				continue
			}
			found = true
			break
		}
		if !found {
			dropped++
			delete(rows, r.Name)
			continue
		}
		filled = append(filled, r)
	}

	if dropped == 0 {
		return
	}

	grid.Rows = filled
	log.WithField("dropped", dropped).Info("Dropped old rows")
}

// appendMetric adds the value at index to metric.
//
// Handles the details of sparse-encoding the results.
// Indices must be monotonically increasing for the same metric.
func appendMetric(metric *statepb.Metric, idx int32, value float64) {
	if l := int32(len(metric.Indices)); l == 0 || metric.Indices[l-2]+metric.Indices[l-1] != idx {
		// If we append V to idx 9 and metric.Indices = [3, 4] then the last filled index is 3+4-1=7
		// So that means we have holes in idx 7 and 8, so start a new group.
		metric.Indices = append(metric.Indices, idx, 1)
	} else {
		metric.Indices[l-1]++ // Expand the length of the current filled list
	}
	metric.Values = append(metric.Values, value)
}

var emptyCell = Cell{Result: statuspb.TestStatus_NO_RESULT}

func hasCellID(name string) bool {
	return !strings.Contains(name, "@TESTGRID@")
}

// truncate truncates a message to max runes (and ellipses). Max = 0 returns the original message.
func truncate(msg string, max int) string {
	if max == 0 || len(msg) <= max {
		return msg
	}
	convert := func(s string) string {
		if utf8.ValidString(s) {
			return s
		}
		return strings.ToValidUTF8(s, "")
		// return s
	}
	start := convert(msg[:max/2])
	end := convert(msg[len(msg)-max/2:])
	return start + "..." + end
}

// appendCell adds the rowResult column to the row.
//
// Handles the details like missing fields and run-length-encoding the result.
func appendCell(row *statepb.Row, cell Cell, start, count int) {
	latest := int32(cell.Result)
	n := len(row.Results)
	switch {
	case n == 0, row.Results[n-2] != latest:
		row.Results = append(row.Results, latest, int32(count))
	default:
		row.Results[n-1] += int32(count)
	}

	addCellID := hasCellID(row.Name)

	for i := 0; i < count; i++ {
		columnIdx := int32(start + i)
		for metricName, measurement := range cell.Metrics {
			var metric *statepb.Metric
			var ok bool
			for _, name := range row.Metric {
				if name == metricName {
					ok = true
					break
				}
			}
			if !ok {
				row.Metric = append(row.Metric, metricName)
			}
			for _, metric = range row.Metrics {
				if metric.Name == metricName {
					break
				}
				metric = nil
			}
			if metric == nil {
				metric = &statepb.Metric{Name: metricName}
				row.Metrics = append(row.Metrics, metric)
			}
			// len()-1 because we already appended the cell id
			appendMetric(metric, columnIdx, measurement)
		}
		if cell.Result == statuspb.TestStatus_NO_RESULT {
			continue
		}
		if addCellID {
			// These values can be derived from the parent row and don't need to be repeated here.
			row.CellIds = append(row.CellIds, cell.CellID)
			row.Properties = append(row.Properties, &statepb.Property{
				Property: cell.Properties,
			})
		}
		// Javascript client expects no result cells to skip icons/messages
		row.Messages = append(row.Messages, truncate(cell.Message, 140))
		row.Icons = append(row.Icons, cell.Icon)
		row.UserProperty = append(row.UserProperty, cell.UserProperty)
	}

	row.Issues = append(row.Issues, cell.Issues...)
}

// AppendColumn adds the build column to the grid.
//
// This handles details like:
// * rows appearing/disappearing in the middle of the run.
// * adding auto metadata like duration, commit as well as any user-added metadata
// * extracting build metadata into the appropriate column header
// * Ensuring row names are unique and formatted with metadata
func AppendColumn(grid *statepb.Grid, rows map[string]*statepb.Row, inflated InflatedColumn) {
	grid.Columns = append(grid.Columns, inflated.Column)
	colIdx := len(grid.Columns) - 1

	missing := map[string]*statepb.Row{}
	for name, row := range rows {
		missing[name] = row
	}

	for name, cell := range inflated.Cells {
		delete(missing, name)

		row, ok := rows[name]
		if !ok {
			id := cell.ID
			if id == "" {
				id = name
			}
			row = &statepb.Row{
				Name:    name,
				Id:      id,
				CellIds: []string{}, // TODO(fejta): try and leave this nil
			}
			rows[name] = row
			grid.Rows = append(grid.Rows, row)
			if colIdx > 0 {
				appendCell(row, emptyCell, 0, colIdx)
			}
		}
		appendCell(row, cell, colIdx, 1)
	}

	for _, row := range missing {
		appendCell(row, emptyCell, colIdx, 1)
	}
}

// alertRows configures the alert for every row that has one.
func alertRows(cols []*statepb.Column, rows []*statepb.Row, openFailures, closePasses int, useCommitAsBuildID bool, userProperty string, columnHeader []*configpb.TestGroup_ColumnHeader) {
	for _, r := range rows {
		r.AlertInfo = alertRow(cols, r, openFailures, closePasses, useCommitAsBuildID, userProperty, columnHeader)
	}
}

// alertRow returns an AlertInfo proto if there have been failuresToOpen consecutive failures more recently than passesToClose.
func alertRow(cols []*statepb.Column, row *statepb.Row, failuresToOpen, passesToClose int, useCommitAsBuildID bool, userPropertyName string, columnHeader []*configpb.TestGroup_ColumnHeader) *statepb.AlertInfo {
	if failuresToOpen == 0 {
		return nil
	}
	var concurrentFailures int
	var totalFailures int32
	var passes int
	var compressedIdx int
	f := result.Iter(row.Results)
	var firstFail *statepb.Column
	var latestFail *statepb.Column
	var latestPass *statepb.Column
	var failIdx int
	var latestFailIdx int
	customColumnHeaders := make(map[string]string)
	// find the first number of consecutive passesToClose (no alert)
	// or else failuresToOpen (alert).
	for _, col := range cols {
		// TODO(fejta): ignore old running
		rawRes, _ := f()
		res := result.Coalesce(rawRes, result.IgnoreRunning)
		if res == statuspb.TestStatus_NO_RESULT {
			if rawRes == statuspb.TestStatus_RUNNING {
				compressedIdx++
			}
			continue
		}
		if res == statuspb.TestStatus_PASS {
			passes++
			if concurrentFailures >= failuresToOpen {
				if latestPass == nil {
					latestPass = col // most recent pass before outage
				}
				if passes >= passesToClose {
					break // enough failures and enough passes, definitely past the start of the failure
				}
			} else if passes >= passesToClose {
				return nil // enough passes but not enough failures, there is no outage
			} else {
				concurrentFailures = 0
			}
		}
		if res == statuspb.TestStatus_FAIL {
			passes = 0
			latestPass = nil
			concurrentFailures++
			totalFailures++
			if totalFailures == 1 { // note most recent failure for this outage
				latestFailIdx = compressedIdx
				latestFail = col
			}
			failIdx = compressedIdx
			firstFail = col
		}
		if res == statuspb.TestStatus_FLAKY {
			passes = 0
			if concurrentFailures >= failuresToOpen {
				break // cannot definitively say which commit is at fault
			}
			concurrentFailures = 0
		}
		compressedIdx++

		for i := 0; i < len(columnHeader); i++ {
			if i >= len(col.Extra) {
				logrus.WithFields(logrus.Fields{
					"started":                 time.Unix(0, int64(col.GetStarted()*float64(time.Millisecond))),
					"additionalColumnHeaders": col.GetExtra(),
				}).Trace("Insufficient column header values to record.")
				break
			}
			if columnHeader[i].Label != "" {
				customColumnHeaders[columnHeader[i].Label] = col.Extra[i]
			} else if columnHeader[i].Property != "" {
				customColumnHeaders[columnHeader[i].Property] = col.Extra[i]
			} else {
				customColumnHeaders[columnHeader[i].ConfigurationValue] = col.Extra[i]
			}
		}
	}
	if concurrentFailures < failuresToOpen {
		return nil
	}
	var id string
	var latestID string
	if len(row.CellIds) > 0 { // not all rows have cell ids
		id = row.CellIds[failIdx]
		latestID = row.CellIds[latestFailIdx]
	}
	msg := row.Messages[latestFailIdx]
	var userProperties map[string]string
	if row.UserProperty != nil && latestFailIdx < len(row.UserProperty) && row.UserProperty[latestFailIdx] != "" {
		userProperties = map[string]string{
			userPropertyName: row.UserProperty[latestFailIdx],
		}
	}

	return alertInfo(totalFailures, msg, id, latestID, userProperties, firstFail, latestFail, latestPass, useCommitAsBuildID, customColumnHeaders)
}

// alertInfo returns an alert proto with the configured fields
func alertInfo(failures int32, msg, cellID, latestCellID string, userProperties map[string]string, fail, latestFail, pass *statepb.Column, useCommitAsBuildID bool, customColumnHeaders map[string]string) *statepb.AlertInfo {
	return &statepb.AlertInfo{
		FailCount:           failures,
		FailBuildId:         buildID(fail, useCommitAsBuildID),
		LatestFailBuildId:   buildID(latestFail, useCommitAsBuildID),
		FailTime:            stamp(fail),
		FailTestId:          cellID,
		LatestFailTestId:    latestCellID,
		FailureMessage:      msg,
		PassTime:            stamp(pass),
		PassBuildId:         buildID(pass, useCommitAsBuildID),
		EmailAddresses:      emailAddresses(fail),
		HotlistIds:          hotlistIDs(fail),
		Properties:          userProperties,
		CustomColumnHeaders: customColumnHeaders,
	}
}

func columnStats(cells map[string]Cell, brokenThreshold float32) *statepb.Stats {
	var passes, fails, total int32
	var pending bool
	if brokenThreshold <= 0.0 {
		return nil
	}
	if cells == nil {
		return nil
	}
	for _, cell := range cells {
		if cell.Result == statuspb.TestStatus_RUNNING {
			pending = true
		}
		status := result.Coalesce(cell.Result, false)
		switch status {
		case statuspb.TestStatus_PASS:
			passes++
			total++
		case statuspb.TestStatus_FAIL:
			fails++
			total++
		case statuspb.TestStatus_FLAKY, statuspb.TestStatus_UNKNOWN:
			total++
		default:
			// blank cell or unrecognized status, do nothing
		}
	}
	var failRatio float32
	if total != 0.0 {
		failRatio = float32(fails) / float32(total)
	}
	return &statepb.Stats{
		FailCount:  fails,
		PassCount:  passes,
		TotalCount: total,
		Pending:    pending,
		Broken:     failRatio > brokenThreshold,
	}
}

func hotlistIDs(col *statepb.Column) []string {
	var ids []string
	for _, hotlistID := range strings.Split(col.HotlistIds, ",") {
		if id := strings.TrimSpace(hotlistID); id != "" {
			ids = append(ids, strings.TrimSpace(hotlistID))
		}
	}
	return ids
}

func emailAddresses(col *statepb.Column) []string {
	if col == nil {
		return []string{}
	}
	return col.GetEmailAddresses()
}

// buildID extracts the ID from the first extra row (where commit data is) or else the Build field.
func buildID(col *statepb.Column, getCommitHeader bool) string {
	if col == nil {
		return ""
	}
	if getCommitHeader && len(col.Extra) > 0 {
		return col.Extra[0]
	}
	return col.Build
}

const billion = 1e9

// stamp converts seconds into a timestamp proto
// TODO(#683): col.Started should be a timestamp instead of a float
func stamp(col *statepb.Column) *timestamp.Timestamp {
	if col == nil {
		return nil
	}
	seconds := col.Started / 1000
	floor := math.Floor(seconds)
	remain := seconds - floor
	return &timestamp.Timestamp{
		Seconds: int64(floor),
		Nanos:   int32(remain * billion),
	}
}
