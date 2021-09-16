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
	"net/http"
	"net/url"
	"path"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/testgrid/config"
	"github.com/GoogleCloudPlatform/testgrid/internal/result"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	statuspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/GoogleCloudPlatform/testgrid/util/metrics"
	"github.com/fvbommel/sortorder"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/googleapi"
)

const componentName = "updater"

// Metrics holds metrics relevant to the Updater.
type Metrics struct {
	Errors       metrics.Counter
	Skips        metrics.Counter
	Successes    metrics.Counter
	DelaySeconds metrics.Int64
	CycleSeconds metrics.Int64
}

type finish struct {
	m    *Metrics
	when time.Time
}

func (f finish) done() {
	seconds := int64(time.Since(f.when).Seconds())
	f.m.CycleSeconds.Set(seconds, componentName)
}

func (f *finish) skip() {
	if f == nil {
		return
	}
	f.done()
	f.m.Skips.Add(1, componentName)
}

func (f *finish) fail() {
	if f == nil {
		return
	}
	f.done()
	f.m.Errors.Add(1, componentName)
}

func (f *finish) success() {
	if f == nil {
		return
	}
	f.done()
	f.m.Successes.Add(1, componentName)
}

func (mets *Metrics) start() *finish {
	if mets == nil {
		return nil
	}
	return &finish{mets, time.Now()}
}

func (mets *Metrics) delay(dur time.Duration) {
	if mets == nil {
		return
	}

	seconds := int64(dur.Seconds())
	mets.DelaySeconds.Set(seconds, componentName)
}

// GroupUpdater will compile the grid state proto for the specified group and upload it.
//
// This typically involves downloading the existing state, dropping old columns,
// compiling any new columns and inserting them into the front and then uploading
// the proto to GCS.
//
// Return true if there are more results to process.
type GroupUpdater func(parent context.Context, log logrus.FieldLogger, client gcs.Client, tg *configpb.TestGroup, gridPath gcs.Path) (bool, error)

// GCS returns a GCS-based GroupUpdater, which knows how to process result data stored in GCS.
func GCS(colClient gcs.Client, groupTimeout, buildTimeout time.Duration, concurrency int, write bool, sortCols ColumnSorter) GroupUpdater {
	return func(parent context.Context, log logrus.FieldLogger, client gcs.Client, tg *configpb.TestGroup, gridPath gcs.Path) (bool, error) {
		if !tg.UseKubernetesClient && (tg.ResultSource == nil || tg.ResultSource.GetGcsConfig() == nil) {
			log.Debug("Skipping non-kubernetes client group")
			return false, nil
		}
		ctx, cancel := context.WithTimeout(parent, groupTimeout)
		defer cancel()
		gcsColReader := gcsColumnReader(colClient, buildTimeout, concurrency)
		reprocess := 20 * time.Minute // allow 20m for prow to finish uploading artifacts
		return InflateDropAppend(ctx, log, client, tg, gridPath, write, gcsColReader, sortCols, reprocess)
	}
}

func gridPaths(configPath gcs.Path, gridPrefix string, groups []*configpb.TestGroup) ([]gcs.Path, error) {
	paths := make([]gcs.Path, 0, len(groups))
	for _, tg := range groups {
		tgp, err := testGroupPath(configPath, gridPrefix, tg.Name)
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

func isPreconditionFailed(err error) bool {
	if err == nil {
		return false
	}
	var e *googleapi.Error
	if !errors.As(err, &e) {
		return false
	}
	return e.Code == http.StatusPreconditionFailed
}

func testGroups(ctx context.Context, opener gcs.Opener, path gcs.Path, groupNames ...string) ([]*configpb.TestGroup, *storage.ReaderObjectAttrs, error) {
	r, attrs, err := opener.Open(ctx, path)
	if err != nil {
		return nil, nil, fmt.Errorf("open: %w", err)
	}
	cfg, err := config.Unmarshal(r)
	if err != nil {
		return nil, nil, fmt.Errorf("unmarshal: %v", err)
	}
	if len(groupNames) == 0 {
		return cfg.TestGroups, attrs, nil
	}
	groups := make([]*configpb.TestGroup, 0, len(groupNames))
	for _, groupName := range groupNames {
		tg := config.FindTestGroup(groupName, cfg)
		if tg == nil {
			return nil, nil, fmt.Errorf("group %q not found", groupName)
		}
		groups = append(groups, tg)
	}
	return groups, attrs, nil
}

type lastUpdated struct {
	stater     gcs.Stater
	gridPrefix string
	configPath gcs.Path
	freq       time.Duration
}

func (fixer lastUpdated) FixOnce(ctx context.Context, log logrus.FieldLogger, q *config.TestGroupQueue, groups []*configpb.TestGroup) error {
	attrs, err := gridAttrs(ctx, log, fixer.stater, fixer.configPath, fixer.gridPrefix, groups)
	if err != nil {
		return err
	}
	q.Init(groups, time.Now().Add(fixer.freq))
	updates := make(map[string]time.Time, len(attrs))
	for i, attr := range attrs {
		if attr == nil {
			continue
		}
		name := groups[i].Name
		if attr.Generation > 0 {
			updates[name] = attr.Updated.Add(fixer.freq)
		} else {
			updates[name] = time.Now()
		}
	}
	q.FixAll(updates, false)
	return nil
}

func (fixer lastUpdated) Fix(ctx context.Context, log logrus.FieldLogger, q *config.TestGroupQueue, groups []*configpb.TestGroup) error {
	if fixer.freq == 0 {
		return nil
	}
	ticker := time.NewTicker(fixer.freq)
	fix := func() {
		if err := fixer.FixOnce(ctx, log, q, groups); err != nil {
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

func gridAttrs(ctx context.Context, log logrus.FieldLogger, client gcs.Stater, configPath gcs.Path, gridPrefix string, groups []*configpb.TestGroup) ([]*storage.ObjectAttrs, error) {
	paths, err := gridPaths(configPath, gridPrefix, groups)
	if err != nil {
		return nil, fmt.Errorf("grid paths: %v", err)
	}

	out := make([]*storage.ObjectAttrs, len(groups))

	attrs := gcs.Stat(ctx, client, 20, paths...)
	for i, attrs := range attrs {
		err := attrs.Err
		name := groups[i].Name
		switch {
		case attrs.Attrs != nil:
			out[i] = attrs.Attrs
		case errors.Is(err, storage.ErrObjectNotExist):
			out[i] = &storage.ObjectAttrs{}
		default:
			log.WithError(err).WithField("name", name).Info("Failed to stat")
		}
	}
	return out, nil
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

// Update test groups with the specified freq.
//
// Retries errors at double and unfinished groups as soon as possible.
//
// Filters down to a single group when set.
// Returns after all groups updated once if freq is zero.
func Update(parent context.Context, client gcs.ConditionalClient, mets *Metrics, configPath gcs.Path, gridPrefix string, groupConcurrency int, groupNames []string, updateGroup GroupUpdater, write bool, freq time.Duration, fixers ...Fixer) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	log := logrus.WithField("config", configPath)

	var q config.TestGroupQueue

	log.Debug("Fetching config...")
	groups, cfgAttrs, err := testGroups(ctx, client, configPath, groupNames...)
	if err != nil {
		return fmt.Errorf("read config: %v", err)
	}
	log.Info("Fetched config")
	if err != nil {
		return fmt.Errorf("test groups: %v", err)
	}

	q.Init(groups, time.Now().Add(freq))

	log.Debug("Fetching initial start times...")
	fixLastUpdated := lastUpdated{
		stater:     client,
		gridPrefix: gridPrefix,
		configPath: configPath,
		freq:       freq,
	}
	if err := fixLastUpdated.FixOnce(ctx, log, &q, groups); err != nil {
		return fmt.Errorf("get generations: %v", err)
	}
	log.Info("Fetched initial start times")

	fixers = append(fixers, fixLastUpdated.Fix)

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
		cond := storage.Conditions{GenerationNotMatch: cfgAttrs.Generation}
		opener := client.If(&cond, &cond)
		ticker := time.NewTicker(time.Minute) // TODO(fejta): subscribe to notifications
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
			log = log.WithField("delay", delay.Round(time.Second))
			mets.delay(delay)
			log.Info("Updating groups")
			select {
			case <-ctx.Done():
				ticker.Stop()
				fixCancel()
				return
			case <-ticker.C:
				groups, cfgAttrs, err = testGroups(ctx, opener, configPath, groupNames...)
				switch {
				case err == nil:
					log.Info("Configuration changed")
					// Config changed, setup fixers again.
					cond.GenerationNotMatch = cfgAttrs.Generation
					log.Trace("Cancelling fixers on old test groups...")
					fixCancel()
					fixWg.Wait()
					q.Init(groups, time.Now().Add(freq))
					log.Debug("Canceled fixers on old test groups")
					fixCtx, fixCancel = context.WithCancel(ctx)
					fixAll()
				case !isPreconditionFailed(err):
					log.WithError(err).Error("Failed to update configuration")
				}
			}
		}
	}()

	active := map[string]bool{}
	var lock sync.RWMutex
	var wg sync.WaitGroup
	wg.Add(groupConcurrency)
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
		tgp, err := testGroupPath(configPath, gridPrefix, name)
		if err != nil {
			fin.fail()
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
		unprocessed, err := updateGroup(ctx, log, client, tg, *tgp)
		if err != nil {
			log := log.WithError(err)
			if isPreconditionFailed(err) {
				fin.skip()
				log.Info("Group was modified while updating")
			} else {
				fin.fail()
				log.Error("Failed to update group")
			}
			var delay time.Duration
			if freq > 0 {
				delay = freq/4 + time.Duration(rand.Int63n(int64(freq/4))) // Int63n() panics if freq <= 0
				log = log.WithField("delay", delay.Seconds())
				q.Fix(tg.Name, time.Now().Add(delay), true)
			}
			return
		}
		fin.success()
		if unprocessed { // process another chunk ASAP
			q.Fix(name, time.Now(), false)
		}
	}

	for i := 0; i < groupConcurrency; i++ {
		go func() {
			defer wg.Done()
			for tg := range channel {
				updateTestGroup(tg)
				runtime.GC()
			}
		}()
	}

	log.Info("Starting to process test groups...")
	return q.Send(ctx, channel, freq)
}

// testGroupPath() returns the path to a test_group proto given this proto
func testGroupPath(g gcs.Path, gridPrefix, groupName string) (*gcs.Path, error) {
	name := path.Join(gridPrefix, groupName)
	u, err := url.Parse(name)
	if err != nil {
		return nil, fmt.Errorf("invalid url %s: %w", name, err)
	}
	np, err := g.ResolveReference(u)
	if err != nil {
		return nil, fmt.Errorf("resolve reference: %w", err)
	}
	if err == nil && np.Bucket() != g.Bucket() {
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
func truncateRunning(cols []InflatedColumn) []InflatedColumn {
	if len(cols) == 0 {
		return cols
	}

	floor := float64(time.Now().Add(-72*time.Hour).UTC().Unix() * 1000)

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

// A ColumnSorter sort InflatedColumns as desired.
type ColumnSorter func(*configpb.TestGroup, []InflatedColumn)

// SortStarted sorts InflatedColumns by column start time.
func SortStarted(_ *configpb.TestGroup, cols []InflatedColumn) {
	sort.SliceStable(cols, func(i, j int) bool {
		return cols[i].Column.Started > cols[j].Column.Started
	})
}

// InflateDropAppend updates groups by downloading the existing grid, dropping old rows and appending new ones.
func InflateDropAppend(ctx context.Context, alog logrus.FieldLogger, client gcs.Client, tg *configpb.TestGroup, gridPath gcs.Path, write bool, readCols ColumnReader, sortCols ColumnSorter, reprocess time.Duration) (bool, error) {
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

	var oldCols []InflatedColumn
	var issues map[string][]string

	log.Trace("Downloading existing grid...")
	old, attrs, err := gcs.DownloadGrid(ctx, client, gridPath)
	if err != nil {
		log.WithField("path", gridPath).WithError(err).Error("Failed to download existing grid")
	}
	if old != nil {
		var cols []InflatedColumn
		log.Trace("Inflating grid...")
		cols, issues = InflateGrid(old, stop, time.Now().Add(-reprocess))
		SortStarted(tg, cols) // Our processing requires descending start time.
		oldCols = truncateRunning(cols)
	}
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

	var cols []InflatedColumn

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
	var unreadColumns bool
	if more {
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
	}

	added := len(cols)

	overrideBuild(tg, cols) // so we group correctly
	cols = append(cols, oldCols...)
	cols = groupColumns(tg, cols)

	sortCols(tg, cols)

	const byteCeiling = 10e6 // 10mb
	grid, buf, err := shrinkGrid(shrinkGrace, log, tg, cols, issues, byteCeiling)
	if err != nil {
		return false, fmt.Errorf("shrink grid: %v", err)
	}

	log = log.WithField("url", gridPath).WithField("bytes", len(buf))
	if !write {
		log = log.WithField("dryrun", true)
	} else {
		log.Debug("Writing grid...")
		// TODO(fejta): configurable cache value
		if _, err := client.Upload(ctx, gridPath, buf, gcs.DefaultACL, "no-cache"); err != nil {
			return false, fmt.Errorf("upload %d bytes: %w", len(buf), err)
		}
	}
	if unreadColumns {
		log = log.WithField("more", true)
	}
	log.WithFields(logrus.Fields{
		"cols":     len(grid.Columns),
		"rows":     len(grid.Rows),
		"appended": added,
	}).Info("Wrote grid")
	return unreadColumns, nil
}

func shrinkGrid(ctx context.Context, log logrus.FieldLogger, tg *configpb.TestGroup, cols []InflatedColumn, issues map[string][]string, byteCeiling int) (*statepb.Grid, []byte, error) {
	// Hopefully the grid is small enough...
	grid := ConstructGrid(log, tg, cols, issues)
	buf, err := gcs.MarshalGrid(grid)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal grid: %w", err)
	}
	orig := len(buf)
	if byteCeiling == 0 || orig < byteCeiling {
		return grid, buf, nil
	}

	// Nope, let's drop old row data...
	byteCeiling /= 2

	log = log.WithField("originally", orig)
	for i := len(cols) / 2; i >= 0; i = i/2 - 1 {
		select {
		case <-ctx.Done():
			log.WithField("finally", orig).Info("Timeout shrinking row data")
			return grid, buf, nil
		default:
		}

		log.WithField("currently", orig).Debug("Shrinking row data")

		for j := i; j < len(cols); j++ {
			nc := len(cols[j].Cells)
			if nc < 2 {
				continue
			}
			cols[j].Cells = truncatedCells(orig, byteCeiling, nc)
		}
		grid = ConstructGrid(log, tg, cols, issues)
		buf, err = gcs.MarshalGrid(grid)
		if err != nil {
			return nil, nil, fmt.Errorf("marshal grid: %w", err)
		}

		orig = len(buf)
		if orig < byteCeiling {
			return grid, buf, nil
		}
	}

	// Not enough, let's try merging columns
	for i := len(cols) / 2; i >= 0; i = i/2 - 1 {
		select {
		case <-ctx.Done():
			log.WithField("finally", orig).Info("Timeout shrinking column data")
			return grid, buf, nil
		default:
		}

		log.WithField("bytes", orig).Info("Shrinking column data")

		// Nope, let's drop old row data...

		for j := i; j < len(cols); j++ {
			cols[j].Column.Name = ""
			cols[j].Column.Build = ""
		}
		cols = groupColumns(tg, cols)

		// Hopefully it is the right size...
		grid = ConstructGrid(log, tg, cols, issues)
		buf, err = gcs.MarshalGrid(grid)
		if err != nil {
			return nil, nil, fmt.Errorf("marshal grid: %w", err)
		}

		orig := len(buf)
		if orig < byteCeiling {
			return grid, buf, nil
		}
	}
	return grid, buf, err
}

func truncatedCells(orig, max, dropped int) map[string]Cell {
	return map[string]Cell{
		"Truncated": {
			Result:  statuspb.TestStatus_UNKNOWN,
			ID:      "Truncated",
			Message: fmt.Sprintf("%d byte grid exceeds maximum size of %d bytes, removed %d rows", orig, max, dropped),
		},
	}
}

// formatStrftime replaces python codes with what go expects.
//
// aka %Y-%m-%d becomes 2006-01-02
func formatStrftime(in string) string {
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
	fmt = formatStrftime(fmt)
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
					if val == "" || val == col.Column.Extra[i] {
						continue
					}
					if col.Column.Extra[i] == "" {
						col.Column.Extra[i] = c.Column.Extra[i]
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
func ConstructGrid(log logrus.FieldLogger, group *configpb.TestGroup, cols []InflatedColumn, issues map[string][]string) *statepb.Grid {
	// Add the columns into a grid message
	var grid statepb.Grid
	rows := map[string]*statepb.Row{} // For fast target => row lookup
	failsOpen := int(group.NumFailuresToAlert)
	passesClose := int(group.NumPassesToDisableAlert)
	if failsOpen > 0 && passesClose == 0 {
		passesClose = 1
	}

	for _, col := range cols {
		appendColumn(&grid, rows, col)
	}

	dropEmptyRows(log, &grid, rows)

	for name, row := range rows {
		row.Issues = append(row.Issues, issues[name]...)
		issues := make(map[string]bool, len(row.Issues))
		for _, i := range row.Issues {
			issues[i] = true
		}
		row.Issues = make([]string, 0, len(issues))
		for i := range issues {
			row.Issues = append(row.Issues, i)
		}
		sort.SliceStable(row.Issues, func(i, j int) bool {
			// Largest issues at the front of the list
			return !sortorder.NaturalLess(row.Issues[i], row.Issues[j])
		})
	}

	alertRows(grid.Columns, grid.Rows, failsOpen, passesClose)
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

func dropEmptyRows(log logrus.FieldLogger, grid *statepb.Grid, rows map[string]*statepb.Row) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	filled := make([]*statepb.Row, 0, len(rows))
	var dropped int
	for _, r := range grid.Rows {
		var found bool
		for res := range result.Iter(ctx, r.Results) {
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
			row.CellIds = append(row.CellIds, cell.CellID)
		}
		row.Properties = append(row.Properties, &statepb.Property{
			Property: cell.Properties,
		})
		// Javascript client expects no result cells to skip icons/messages
		row.Messages = append(row.Messages, cell.Message)
		row.Icons = append(row.Icons, cell.Icon)
		row.UserProperty = append(row.UserProperty, cell.UserProperty)
	}

	row.Issues = append(row.Issues, cell.Issues...)
}

// appendColumn adds the build column to the grid.
//
// This handles details like:
// * rows appearing/disappearing in the middle of the run.
// * adding auto metadata like duration, commit as well as any user-added metadata
// * extracting build metadata into the appropriate column header
// * Ensuring row names are unique and formatted with metadata
func appendColumn(grid *statepb.Grid, rows map[string]*statepb.Row, inflated InflatedColumn) {
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
func alertRows(cols []*statepb.Column, rows []*statepb.Row, openFailures, closePasses int) {
	for _, r := range rows {
		r.AlertInfo = alertRow(cols, r, openFailures, closePasses)
	}
}

// alertRow returns an AlertInfo proto if there have been failuresToOpen consecutive failures more recently than passesToClose.
func alertRow(cols []*statepb.Column, row *statepb.Row, failuresToOpen, passesToClose int) *statepb.AlertInfo {
	if failuresToOpen == 0 {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var failures int
	var totalFailures int32
	var passes int
	var compressedIdx int
	ch := result.Iter(ctx, row.Results)
	var firstFail *statepb.Column
	var latestFail *statepb.Column
	var latestPass *statepb.Column
	var failIdx int
	var latestFailIdx int
	// find the first number of consecutive passesToClose (no alert)
	// or else failuresToOpen (alert).
	for _, col := range cols {
		// TODO(fejta): ignore old running
		rawRes := <-ch
		res := result.Coalesce(rawRes, result.IgnoreRunning)
		if res == statuspb.TestStatus_NO_RESULT {
			if rawRes == statuspb.TestStatus_RUNNING {
				compressedIdx++
			}
			continue
		}
		if res == statuspb.TestStatus_PASS {
			passes++
			if failures >= failuresToOpen {
				latestPass = col // most recent pass before outage
				break
			}
			if passes >= passesToClose {
				return nil // there is no outage
			}
			failures = 0
		}
		if res == statuspb.TestStatus_FAIL {
			passes = 0
			failures++
			totalFailures++
			if failures == 1 { // note most recent failure for this outage
				latestFailIdx = compressedIdx
				latestFail = col
			}
			failIdx = compressedIdx
			firstFail = col
		}
		if res == statuspb.TestStatus_FLAKY {
			passes = 0
			if failures >= failuresToOpen {
				break // cannot definitively say which commit is at fault
			}
			failures = 0
		}
		compressedIdx++
	}
	if failures < failuresToOpen {
		return nil
	}
	var id string
	var latestID string
	if len(row.CellIds) > 0 { // not all rows have cell ids
		id = row.CellIds[failIdx]
		latestID = row.CellIds[latestFailIdx]
	}
	msg := row.Messages[latestFailIdx]
	return alertInfo(totalFailures, msg, id, latestID, firstFail, latestFail, latestPass)
}

// alertInfo returns an alert proto with the configured fields
func alertInfo(failures int32, msg, cellID, latestCellID string, fail, latestFail, pass *statepb.Column) *statepb.AlertInfo {
	return &statepb.AlertInfo{
		FailCount:         failures,
		FailBuildId:       buildID(fail),
		LatestFailBuildId: buildID(latestFail),
		FailTime:          stamp(fail),
		FailTestId:        cellID,
		LatestFailTestId:  latestCellID,
		FailureMessage:    msg,
		PassTime:          stamp(pass),
		PassBuildId:       buildID(pass),
		EmailAddresses:    emailAddresses(fail),
	}
}

func emailAddresses(col *statepb.Column) []string {
	if col == nil {
		return []string{}
	}
	return col.GetEmailAddresses()
}

// buildID extracts the ID from the first extra row or else the Build field.
func buildID(col *statepb.Column) string {
	if col == nil {
		return ""
	}
	if len(col.Extra) > 0 {
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
