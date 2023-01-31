/*
Copyright 2019 The Kubernetes Authors.

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

// Package summarizer provides a method to read state protos defined in a config an output summary protos.
package summarizer

import (
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"bitbucket.org/creachadair/stringset"
	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/testgrid/config"
	"github.com/GoogleCloudPlatform/testgrid/config/snapshot"
	"github.com/GoogleCloudPlatform/testgrid/internal/result"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	summarypb "github.com/GoogleCloudPlatform/testgrid/pb/summary"
	statuspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	"github.com/GoogleCloudPlatform/testgrid/pkg/tabulator"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/GoogleCloudPlatform/testgrid/util/metrics"
	"github.com/golang/protobuf/proto"
	"github.com/sirupsen/logrus"
)

// Metrics holds metrics relevant to the Updater.
type Metrics struct {
	Summarize metrics.Cyclic
}

// CreateMetrics creates all the metrics that the Summarizer will use
// This should be called once
func CreateMetrics(factory metrics.Factory) *Metrics {
	return &Metrics{
		Summarize: factory.NewCyclic("summarizer"),
	}
}

// FeatureFlags aggregates the knobs to enable/disable certain features.
type FeatureFlags struct {
	// controls the acceptable flakiness calculation logic for dashboard tab
	AllowFuzzyFlakiness bool

	// allows ignoring columns with specific test statuses during summarization
	AllowIgnoredColumns bool

	// allows enforcing minimum number of runs for a dashboard tab
	AllowMinNumberOfRuns bool
}

// gridReader returns the grid content and metadata (last updated time, generation id)
type gridReader func(ctx context.Context) (io.ReadCloser, time.Time, int64, error)

// groupFinder returns the named group as well as reader for the grid state
type groupFinder func(dashboardName string, tab *configpb.DashboardTab) (*gcs.Path, *configpb.TestGroup, gridReader, error)

func lockDashboard(ctx context.Context, client gcs.ConditionalClient, path gcs.Path, generation int64) (*storage.ObjectAttrs, error) {
	var buf []byte
	if generation == 0 {
		var sum summarypb.DashboardSummary
		var err error
		buf, err = proto.Marshal(&sum)
		if err != nil {
			return nil, fmt.Errorf("marshal: %w", err)
		}
	}

	return gcs.Touch(ctx, client, path, generation, buf)
}

// Fixer should adjust the dashboard queue until the context expires.
type Fixer func(context.Context, *config.DashboardQueue) error

// Update summary protos by reading the state protos defined in the config.
//
// Will use concurrency go routines to update dashboards in parallel.
// Setting dashboard will limit update to this dashboard.
// Will write summary proto when confirm is set.
func Update(ctx context.Context, client gcs.ConditionalClient, mets *Metrics, configPath gcs.Path, concurrency int, tabPathPrefix, summaryPathPrefix string, allowedDashboards []string, confirm bool, features FeatureFlags, freq time.Duration, fixers ...Fixer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if concurrency < 1 {
		return fmt.Errorf("concurrency must be positive, got: %d", concurrency)
	}
	log := logrus.WithField("config", configPath)

	var q config.DashboardQueue
	var cfg *snapshot.Config

	allowed := stringset.New(allowedDashboards...)
	fixSnapshot := func(newConfig *snapshot.Config) error {
		baseLog := log
		log := log.WithField("fixSnapshot()", true)
		newConfig.Dashboards = filterDashboards(newConfig.Dashboards, allowed)
		cfg = newConfig

		dashCap := len(cfg.Dashboards)
		paths := make([]gcs.Path, 0, dashCap)
		dashboards := make([]*configpb.Dashboard, 0, dashCap)
		for _, d := range cfg.Dashboards {
			path, err := SummaryPath(configPath, summaryPathPrefix, d.Name)
			if err != nil {
				log.WithError(err).WithField("dashboard", d.Name).Error("Bad dashboard path")
			}
			paths = append(paths, *path)
			dashboards = append(dashboards, d)
		}

		stats := gcs.Stat(ctx, client, 10, paths...)
		whens := make(map[string]time.Time, len(stats))
		var wg sync.WaitGroup
		for i, stat := range stats {
			name := dashboards[i].Name
			path := paths[i]
			log := log.WithField("path", path)
			switch {
			case stat.Attrs != nil:
				whens[name] = stat.Attrs.Updated.Add(freq)
			default:
				if errors.Is(stat.Err, storage.ErrObjectNotExist) {
					wg.Add(1)
					go func() {
						defer wg.Done()
						_, err := lockDashboard(ctx, client, path, 0)
						switch {
						case gcs.IsPreconditionFailed(err):
							log.WithError(err).Debug("Lost race to create initial summary")
						case err != nil:
							log.WithError(err).Error("Failed to lock initial summary")
						default:
							log.Info("Created initial summary")
						}
					}()
				} else {
					log.WithError(stat.Err).Info("Failed to stat")
				}
				whens[name] = time.Now()
			}
		}

		wg.Wait()

		q.Init(baseLog, dashboards, time.Now().Add(freq))
		if err := q.FixAll(whens, false); err != nil {
			log.WithError(err).Error("Failed to fix all dashboards based on last update time")
		}
		return nil
	}

	log.Debug("Observing config...")
	cfgChanged, err := snapshot.Observe(ctx, log, client, configPath, time.NewTicker(time.Minute).C)
	if err != nil {
		return fmt.Errorf("observe config: %w", err)
	}
	fixSnapshot(<-cfgChanged) // Bootstrap queue before use

	var active stringset.Set
	var waiting stringset.Set
	var lock sync.Mutex

	go func() {
		fixCtx, fixCancel := context.WithCancel(ctx)
		var fixWg sync.WaitGroup
		fixAll := func() {
			n := len(fixers)
			log.WithField("fixers", n).Trace("Starting fixers on current dashboards...")
			fixWg.Add(n)
			for i, fix := range fixers {
				go func(i int, fix Fixer) {
					defer fixWg.Done()
					if err := fix(fixCtx, &q); err != nil && !errors.Is(err, context.Canceled) {
						log.WithError(err).WithField("fixer", i).Warning("Fixer failed")
					}
				}(i, fix)
			}
			log.Debug("Started fixers on current dashboards")
		}

		ticker := time.NewTicker(time.Minute) // TODO(fejta): subscribe to notifications
		fixAll()
		for {
			lock.Lock()
			activeDashboards := active.Elements()
			lock.Unlock()

			depth, next, when := q.Status()
			log := log.WithFields(logrus.Fields{
				"depth":  depth,
				"active": activeDashboards,
			})
			if next != nil {
				log = log.WithField("next", *next)
			}
			delay := time.Since(when)
			if delay < 0 {
				delay = 0
				log = log.WithField("sleep", -delay)
			}
			log = log.WithField("delay", delay.Round(time.Second))
			log.Info("Updating dashboards")
			select {
			case <-ctx.Done():
				ticker.Stop()
				fixCancel()
				fixWg.Wait()
				return
			case newConfig := <-cfgChanged:
				log.Info("Configuration changed")
				fixCancel()
				fixWg.Wait()
				fixCtx, fixCancel = context.WithCancel(ctx)
				fixSnapshot(newConfig)
				fixAll()
			case <-ticker.C:
			}

		}
	}()

	dashboardNames := make(chan string)

	// TODO(fejta): cache downloaded group?
	findGroup := func(dash string, tab *configpb.DashboardTab) (*gcs.Path, *configpb.TestGroup, gridReader, error) {
		name := tab.TestGroupName
		group := cfg.Groups[name]
		if group == nil {
			return nil, nil, nil, nil
		}
		groupPath, err := tabulator.TabStatePath(configPath, tabPathPrefix, dash, tab.Name)
		if err != nil {
			return nil, group, nil, err
		}
		reader := func(ctx context.Context) (io.ReadCloser, time.Time, int64, error) {
			return pathReader(ctx, client, *groupPath)
		}
		return groupPath, group, reader, nil
	}

	tabUpdater := tabUpdatePool(ctx, log, concurrency, features)

	updateName := func(log *logrus.Entry, dashName string) (logrus.FieldLogger, bool, error) {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		dash := cfg.Dashboards[dashName]
		if dash == nil {
			return log, false, errors.New("dashboard not found")
		}
		log.Debug("Summarizing dashboard")
		summaryPath, err := SummaryPath(configPath, summaryPathPrefix, dashName)
		if err != nil {
			return log, false, fmt.Errorf("summary path: %v", err)
		}
		sum, _, _, err := ReadSummary(ctx, client, *summaryPath)
		if err != nil {
			return log, false, fmt.Errorf("read %q: %v", *summaryPath, err)
		}

		if sum == nil {
			sum = &summarypb.DashboardSummary{}
		}

		// TODO(fejta): refactor to note whether there is more work
		more := updateDashboard(ctx, client, dash, sum, findGroup, tabUpdater)

		var healthyTests int
		var failures int
		for _, tab := range sum.TabSummaries {
			failures += len(tab.FailingTestSummaries)
			if h := tab.Healthiness; h != nil {
				healthyTests += len(h.Tests)
			}
		}

		log = log.WithFields(logrus.Fields{
			"path":          summaryPath,
			"tabs":          len(sum.TabSummaries),
			"failures":      failures,
			"healthy-tests": healthyTests,
		})
		if !confirm {
			return log, more, nil
		}
		size, err := writeSummary(ctx, client, *summaryPath, sum)
		log = log.WithField("bytes", size)
		if err != nil {
			return log, more, fmt.Errorf("write: %w", err)
		}
		return log, more, nil
	}

	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			for dashName := range dashboardNames {
				lock.Lock()
				start := active.Add(dashName)
				if !start {
					waiting.Add(dashName)
				}
				lock.Unlock()
				if !start {
					continue
				}

				log := log.WithField("dashboard", dashName)
				finish := mets.Summarize.Start()
				if log, more, err := updateName(log, dashName); err != nil {
					finish.Fail()
					q.Fix(dashName, time.Now().Add(freq/2), false)
					log.WithError(err).Error("Failed to summarize dashboard")
				} else {
					finish.Success()
					if more {
						q.Fix(dashName, time.Now(), false)
						log = log.WithField("more", more)
					}
					log.Info("Summarized dashboard")
				}

				lock.Lock()
				active.Discard(dashName)
				restart := waiting.Discard(dashName)
				lock.Unlock()
				if restart {
					q.Fix(dashName, time.Now(), false)
				}

			}
		}()
	}
	defer wg.Wait()
	defer close(dashboardNames)

	return q.Send(ctx, dashboardNames, freq)
}

func filterDashboards(dashboards map[string]*configpb.Dashboard, allowed stringset.Set) map[string]*configpb.Dashboard {
	if allowed.Len() == 0 {
		return dashboards
	}

	for key, d := range dashboards {
		if allowed.Contains(d.Name) {
			continue
		}
		delete(dashboards, key)
	}
	return dashboards
}

var (
	normalizer = regexp.MustCompile(`[^a-z0-9]+`)
)

func SummaryPath(g gcs.Path, prefix, dashboard string) (*gcs.Path, error) {
	// ''.join(c for c in n.lower() if c is alphanumeric
	name := "summary-" + normalizer.ReplaceAllString(strings.ToLower(dashboard), "")
	fullName := path.Join(prefix, name)
	u, err := url.Parse(fullName)
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	np, err := g.ResolveReference(u)
	if err != nil {
		return nil, fmt.Errorf("resolve reference: %w", err)
	}
	if np.Bucket() != g.Bucket() {
		return nil, fmt.Errorf("dashboard %s should not change bucket", fullName)
	}
	return np, nil
}

func ReadSummary(ctx context.Context, client gcs.Client, path gcs.Path) (*summarypb.DashboardSummary, time.Time, int64, error) {
	r, modified, gen, err := pathReader(ctx, client, path)
	if errors.Is(err, storage.ErrObjectNotExist) {
		return nil, time.Time{}, 0, nil
	} else if err != nil {
		return nil, time.Time{}, 0, fmt.Errorf("open: %w", err)
	}
	buf, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, time.Time{}, 0, fmt.Errorf("read: %w", err)
	}
	var sum summarypb.DashboardSummary

	if err := proto.Unmarshal(buf, &sum); err != nil {
		return nil, time.Time{}, 0, fmt.Errorf("unmarhsal: %v", err)
	}

	return &sum, modified, gen, nil
}

func writeSummary(ctx context.Context, client gcs.Client, path gcs.Path, sum *summarypb.DashboardSummary) (int, error) {
	buf, err := proto.Marshal(sum)
	if err != nil {
		return 0, fmt.Errorf("marshal: %v", err)
	}
	_, err = client.Upload(ctx, path, buf, gcs.DefaultACL, gcs.NoCache)
	return len(buf), err
}

func statPaths(ctx context.Context, log logrus.FieldLogger, client gcs.Stater, paths ...gcs.Path) []*storage.ObjectAttrs {
	return gcs.StatExisting(ctx, log, client, paths...)
}

// pathReader returns a reader for the specified path and last modified, generation metadata.
func pathReader(ctx context.Context, client gcs.Client, path gcs.Path) (io.ReadCloser, time.Time, int64, error) {
	r, attrs, err := client.Open(ctx, path)
	if err != nil {
		return nil, time.Time{}, 0, fmt.Errorf("client.Open(): %w", err)
	}
	if attrs == nil {
		return r, time.Time{}, 0, nil
	}
	return r, attrs.LastModified, attrs.Generation, nil
}

func tabStatus(dashName, tabName, msg string) *summarypb.DashboardTabSummary {
	return &summarypb.DashboardTabSummary{
		DashboardName:    dashName,
		DashboardTabName: tabName,
		OverallStatus:    summarypb.DashboardTabSummary_UNKNOWN,
		Alert:            msg,
		Status:           msg,
	}
}

// updateDashboard will summarize all the tabs.
//
// Errors summarizing tabs are displayed on the summary for the dashboard.
//
// Returns true when there is more work to to.
func updateDashboard(ctx context.Context, client gcs.Stater, dash *configpb.Dashboard, sum *summarypb.DashboardSummary, findGroup groupFinder, tabUpdater *tabUpdater) bool {
	log := logrus.WithField("dashboard", dash.Name)

	var graceCtx context.Context
	if when, ok := ctx.Deadline(); ok {
		dur := time.Until(when) / 2
		var cancel func()
		graceCtx, cancel = context.WithTimeout(ctx, dur)
		defer cancel()
	} else {
		graceCtx = ctx
	}

	// First collect the previously summarized tabs.
	tabSummaries := make(map[string]*summarypb.DashboardTabSummary, len(sum.TabSummaries))
	for _, tabSum := range sum.TabSummaries {
		tabSummaries[tabSum.DashboardTabName] = tabSum
	}

	// Now create info about which tabs we need to summarize and where the grid state lives.
	type groupInfo struct {
		group  *configpb.TestGroup
		reader gridReader
		tabs   []*configpb.DashboardTab
	}
	groupInfos := make(map[gcs.Path]*groupInfo, len(dash.DashboardTab))

	var paths []gcs.Path
	for _, tab := range dash.DashboardTab {
		groupPath, group, groupReader, err := findGroup(dash.Name, tab)
		if err != nil {
			tabSummaries[tab.Name] = tabStatus(dash.Name, tab.Name, fmt.Sprintf("Error reading group info: %v", err))
			continue
		}
		if group == nil {
			tabSummaries[tab.Name] = tabStatus(dash.Name, tab.Name, fmt.Sprintf("Test group does not exist: %q", tab.TestGroupName))
			continue
		}
		info := groupInfos[*groupPath]
		if info == nil {
			info = &groupInfo{
				group:  group,
				reader: groupReader, // TODO(fejta): optimize (only read once)
			}
			paths = append(paths, *groupPath)
			groupInfos[*groupPath] = info
		}
		info.tabs = append(info.tabs, tab)
	}

	// Check the attributes of the grid states.
	attrs := gcs.StatExisting(ctx, log, client, paths...)

	delays := make(map[gcs.Path]float64, len(paths))

	// determine how much behind each summary is
	for i, path := range paths {
		a := attrs[i]
		for _, tab := range groupInfos[path].tabs {
			// TODO(fejta): optimize (only read once)
			name := tab.Name
			sum := tabSummaries[name]
			if a == nil {
				tabSummaries[name] = tabStatus(dash.Name, name, noRuns)
				delays[path] = -1
			} else if sum == nil {
				tabSummaries[name] = tabStatus(dash.Name, name, "Newly created tab")
				delays[path] = float64(24 * time.Hour / time.Second)
				log.WithField("tab", name).Debug("Found new tab")
			} else {
				delays[path] = float64(attrs[i].Updated.Unix()) - tabSummaries[name].LastUpdateTimestamp
			}
		}
	}

	// sort by delay
	sort.SliceStable(paths, func(i, j int) bool {
		return delays[paths[i]] > delays[paths[j]]
	})

	// Now let's update the tab summaries in parallel, starting with most delayed

	type future struct {
		log    *logrus.Entry
		name   string
		result func() (*summarypb.DashboardTabSummary, error)
	}

	// channel to receive updated tabs
	ch := make(chan future)

	// request an update for each tab, starting with the least recently modified one.
	go func() {
		defer close(ch)
		tabUpdater.lock.Lock()
		defer tabUpdater.lock.Unlock()
		for _, path := range paths {
			info := groupInfos[path]
			log := log.WithField("group", path)
			for _, tab := range info.tabs {
				log := log.WithField("tab", tab.Name)
				delay := delays[path]
				if delay == 0 {
					log.Debug("Already up to date")
					continue
				} else if delay == -1 {
					log.Debug("No grid state to process")
				}
				log = log.WithField("delay", delay)
				if err := graceCtx.Err(); err != nil {
					log.WithError(err).Info("Interrupted")
					return
				}
				log.Debug("Requesting tab summary update")
				f := tabUpdater.update(ctx, tab, info.group, info.reader)
				select {
				case <-ctx.Done():
					return
				case ch <- future{log, tab.Name, f}:
				}
			}
		}
	}()

	// Update the summary for any tabs that give a response
	for fut := range ch {
		tabName := fut.name
		log := fut.log
		log.Trace("Waiting for updated tab summary response")
		s, err := fut.result()
		if err != nil {
			s = tabStatus(dash.Name, tabName, fmt.Sprintf("Error attempting to summarize tab: %v", err))
			log = log.WithError(err)
		} else {
			s.DashboardName = dash.Name
		}
		tabSummaries[tabName] = s
		log.Trace("Updated tab summary")
	}

	// assemble them back into the dashboard summary.
	sum.TabSummaries = make([]*summarypb.DashboardTabSummary, len(dash.DashboardTab))
	for idx, tab := range dash.DashboardTab {
		sum.TabSummaries[idx] = tabSummaries[tab.Name]
	}

	return graceCtx.Err() != nil
}

type tabUpdater struct {
	lock   sync.Mutex
	update func(context.Context, *configpb.DashboardTab, *configpb.TestGroup, gridReader) func() (*summarypb.DashboardTabSummary, error)
}

func tabUpdatePool(poolCtx context.Context, log *logrus.Entry, concurrency int, features FeatureFlags) *tabUpdater {
	type request struct {
		ctx   context.Context
		tab   *configpb.DashboardTab
		group *configpb.TestGroup
		read  gridReader
		sum   *summarypb.DashboardTabSummary
		err   error
		wg    sync.WaitGroup
	}

	ch := make(chan *request, concurrency)

	var wg sync.WaitGroup
	wg.Add(concurrency)
	log = log.WithField("concurrency", concurrency)
	log.Info("Starting up worker pool")

	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			for req := range ch {
				req.sum, req.err = updateTab(req.ctx, req.tab, req.group, req.read, features)
				req.wg.Done()
			}
		}()
	}

	go func() {
		<-poolCtx.Done()
		log.Info("Shutting down worker pool")
		close(ch)
		wg.Wait()
		log.Info("Worker pool stopped")
	}()

	updateTabViaPool := func(ctx context.Context, tab *configpb.DashboardTab, group *configpb.TestGroup, groupReader gridReader) func() (*summarypb.DashboardTabSummary, error) {
		req := &request{
			ctx:   ctx,
			tab:   tab,
			group: group,
			read:  groupReader,
		}
		req.wg.Add(1)
		select {
		case <-ctx.Done():
			return func() (*summarypb.DashboardTabSummary, error) { return nil, ctx.Err() }
		case ch <- req:
			return func() (*summarypb.DashboardTabSummary, error) {
				req.wg.Wait()
				return req.sum, req.err
			}
		}
	}

	return &tabUpdater{
		update: updateTabViaPool,
	}
}

// staleHours returns the configured number of stale hours for the tab.
func staleHours(tab *configpb.DashboardTab) time.Duration {
	if tab.AlertOptions == nil {
		return 0
	}
	return time.Duration(tab.AlertOptions.AlertStaleResultsHours) * time.Hour
}

// updateTab reads the latest grid state for the tab and summarizes it.
func updateTab(ctx context.Context, tab *configpb.DashboardTab, group *configpb.TestGroup, groupReader gridReader, features FeatureFlags) (*summarypb.DashboardTabSummary, error) {
	groupName := tab.TestGroupName
	grid, mod, _, err := readGrid(ctx, groupReader) // TODO(fejta): track gen
	if err != nil {
		return nil, fmt.Errorf("load %s: %v", groupName, err)
	}

	var healthiness *summarypb.HealthinessInfo
	if shouldRunHealthiness(tab) {
		// TODO (itsazhuhere@): Change to rely on YAML defaults rather than consts
		interval := int(tab.HealthAnalysisOptions.DaysOfAnalysis)
		if interval <= 0 {
			interval = DefaultInterval
		}
		healthiness = getHealthinessForInterval(grid, tab.Name, time.Now(), interval)
	}

	recent := recentColumns(tab, group)
	grid.Rows = recentRows(grid.Rows, recent)

	grid.Rows = filterMethods(grid.Rows)

	latest, latestSeconds := latestRun(grid.Columns)
	alert := staleAlert(mod, latest, staleHours(tab))
	failures := failingTestSummaries(grid.Rows)
	colsCells, brokenState := gridMetrics(len(grid.Columns), grid.Rows, recent, tab.BrokenColumnThreshold, features, tab.GetStatusCustomizationOptions())
	metrics := tabMetrics(colsCells)
	tabStatus := overallStatus(grid, recent, alert, brokenState, failures, features, colsCells, tab.GetStatusCustomizationOptions())
	return &summarypb.DashboardTabSummary{
		DashboardTabName:     tab.Name,
		LastUpdateTimestamp:  float64(mod.Unix()),
		LastRunTimestamp:     float64(latestSeconds),
		Alert:                alert,
		FailingTestSummaries: failures,
		OverallStatus:        tabStatus,
		Status:               statusMessage(colsCells, tabStatus, tab.GetStatusCustomizationOptions()),
		LatestGreen:          latestGreen(grid, group.UseKubernetesClient),
		BugUrl:               tab.GetOpenBugTemplate().GetUrl(),
		Healthiness:          healthiness,
		LinkedIssues:         allLinkedIssues(grid.Rows),
		SummaryMetrics:       metrics,
	}, nil
}

// readGrid downloads and deserializes the current test group state.
func readGrid(ctx context.Context, reader gridReader) (*statepb.Grid, time.Time, int64, error) {
	var t time.Time
	r, mod, gen, err := reader(ctx)
	if err != nil {
		return nil, t, 0, fmt.Errorf("open: %w", err)
	}
	defer r.Close()
	zlibReader, err := zlib.NewReader(r)
	if err != nil {
		return nil, t, 0, fmt.Errorf("decompress: %v", err)
	}
	buf, err := ioutil.ReadAll(zlibReader)
	if err != nil {
		return nil, t, 0, fmt.Errorf("read: %v", err)
	}
	var g statepb.Grid
	if err = proto.Unmarshal(buf, &g); err != nil {
		return nil, t, 0, fmt.Errorf("parse: %v", err)
	}
	return &g, mod, gen, nil
}

// recentColumns returns the configured number of recent columns to summarize, or 5.
func recentColumns(tab *configpb.DashboardTab, group *configpb.TestGroup) int {
	return firstFilled(tab.NumColumnsRecent, group.NumColumnsRecent, 5)
}

// firstFilled returns the first non-empty value, or zero.
func firstFilled(values ...int32) int {
	for _, v := range values {
		if v != 0 {
			return int(v)
		}
	}
	return 0
}

// recentRows returns the subset of rows with at least one recent result
func recentRows(in []*statepb.Row, recent int) []*statepb.Row {
	var rows []*statepb.Row
	for _, r := range in {
		if r.Results == nil {
			continue
		}
		if statuspb.TestStatus(r.Results[0]) == statuspb.TestStatus_NO_RESULT && int(r.Results[1]) >= recent {
			continue
		}
		rows = append(rows, r)
	}
	return rows
}

// filterMethods returns the subset of rows that do not have test method names
func filterMethods(rows []*statepb.Row) []*statepb.Row {
	var filtered []*statepb.Row
	for _, r := range rows {
		if !isValidTestName(r.Id) || !isValidTestName(r.Name) {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

// latestRun returns the Time (and seconds-since-epoch) of the most recent run.
func latestRun(columns []*statepb.Column) (time.Time, int64) {
	if len(columns) > 0 {
		if start := int64(columns[0].Started); start > 0 {
			second := start / 1000
			mills := start % 1000
			return time.Unix(second, mills*1e6), second
		}
	}
	return time.Time{}, 0
}

const noRuns = "no completed results"

// staleAlert returns an explanatory message if the latest results are stale.
func staleAlert(mod, ran time.Time, stale time.Duration) string {
	if mod.IsZero() {
		return "no stored results"
	}
	if stale == 0 {
		return ""
	}
	if ran.IsZero() {
		return noRuns
	}
	now := time.Now()
	if dur := now.Sub(mod); dur > stale {
		return fmt.Sprintf("data has not changed since %s (%s old)", mod, dur.Truncate(15*time.Minute))
	}
	if dur := now.Sub(ran); dur > stale {
		return fmt.Sprintf("latest column from %s (%s old)", ran, dur.Truncate(15*time.Minute))
	}
	return ""
}

// failingTestSummaries returns details for every row with an active alert.
func failingTestSummaries(rows []*statepb.Row) []*summarypb.FailingTestSummary {
	var failures []*summarypb.FailingTestSummary
	for _, row := range rows {
		if row.AlertInfo == nil {
			continue
		}
		alert := row.AlertInfo
		sum := summarypb.FailingTestSummary{
			DisplayName:       row.Name,
			TestName:          row.Id,
			FailBuildId:       alert.FailBuildId,
			LatestFailBuildId: alert.LatestFailBuildId,
			FailCount:         alert.FailCount,
			FailureMessage:    alert.FailureMessage,
			PassBuildId:       alert.PassBuildId,
			// TODO(fejta): better build info
			BuildLink:          alert.BuildLink,
			BuildLinkText:      alert.BuildLinkText,
			BuildUrlText:       alert.BuildUrlText,
			LinkedBugs:         row.Issues,
			FailTestLink:       buildFailLink(alert.FailTestId, row.Id),
			LatestFailTestLink: buildFailLink(alert.LatestFailTestId, row.Id),
			Properties:         alert.Properties,
			HotlistIds:         alert.HotlistIds,
			EmailAddresses:     alert.EmailAddresses,
		}
		if alert.PassTime != nil {
			sum.PassTimestamp = float64(alert.PassTime.Seconds)
		}
		if alert.FailTime != nil {
			sum.FailTimestamp = float64(alert.FailTime.Seconds)
		}

		failures = append(failures, &sum)
	}
	return failures
}

// buildFailLink creates a search link
// TODO(#134): Build proper url for both internal and external jobs
func buildFailLink(testID, target string) string {
	return fmt.Sprintf("%s %s", url.PathEscape(testID), url.PathEscape(target))
}

// overallStatus determines whether the tab is stale, failing, flaky or healthy.
//
// Tabs are:
// BROKEN - called with brokenState (typically when most rows are red)
// STALE - called with a stale mstring (typically when most recent column is old)
// FAIL - there is at least one alert
// ACCEPTABLE - the ratio of (valid) failing to total columns is less than configured threshold
// FLAKY - at least one recent column has failing cells
// PENDING - number of valid columns is less than minimum # of runs required
// PASS - all recent columns are entirely green
func overallStatus(grid *statepb.Grid, recent int, stale string, brokenState bool, alerts []*summarypb.FailingTestSummary, features FeatureFlags, colCells gridStats, opts *configpb.DashboardTabStatusCustomizationOptions) summarypb.DashboardTabSummary_TabStatus {
	if brokenState {
		return summarypb.DashboardTabSummary_BROKEN
	}
	if stale != "" {
		return summarypb.DashboardTabSummary_STALE
	}
	if len(alerts) > 0 {
		return summarypb.DashboardTabSummary_FAIL
	}
	// safeguard PENDING status behind a flag
	if features.AllowMinNumberOfRuns && opts.GetMinAcceptableRuns() > int32(colCells.completedCols-colCells.ignoredCols) {
		return summarypb.DashboardTabSummary_PENDING
	}

	results := result.Map(grid.Rows)
	moreCols := true
	var passing bool
	var flaky bool
	// We want to look at recent columns, skipping over any that are still running.
	for moreCols && recent > 0 {
		moreCols = false
		var foundCol bool
		var running bool
		var ignored bool
		// One result off each column since we don't know which
		// cells are running ahead of time.
		for _, resultF := range results {
			r, ok := resultF()
			if !ok {
				continue
			}
			moreCols = true
			if r == statuspb.TestStatus_RUNNING {
				running = true
				// not break because we need to pull this column's
				// result off every row's channel.
				continue
			}
			if features.AllowIgnoredColumns && result.Ignored(r, opts) {
				ignored = true
				continue
			}
			r = coalesceResult(r, result.IgnoreRunning)
			if r == statuspb.TestStatus_NO_RESULT {
				continue
			}
			// any failure in a recent column results in flaky
			if r != statuspb.TestStatus_PASS {
				flaky = true
				continue
			}
			foundCol = true
		}

		// Running columns are unfinished and therefore should
		// not count as "recent" until they finish.
		if running {
			continue
		}

		// Ignored columns are ignored from tab status but they do count as recent
		// Failures in this col are ignored too
		if ignored {
			recent--
			flaky = false
			continue
		}

		if flaky {
			if isAcceptable(colCells, opts, features) {
				return summarypb.DashboardTabSummary_ACCEPTABLE
			}
			return summarypb.DashboardTabSummary_FLAKY
		}

		if foundCol {
			passing = true
			recent--
		}
	}

	if passing {
		return summarypb.DashboardTabSummary_PASS
	}
	return summarypb.DashboardTabSummary_UNKNOWN
}

// isAcceptable determines if the flakiness is within acceptable range.
// Return true iff the feature is enabled, `max_acceptable_flakiness` is set and flakiness is < than configured.
func isAcceptable(colCells gridStats, opts *configpb.DashboardTabStatusCustomizationOptions, features FeatureFlags) bool {
	if features.AllowFuzzyFlakiness && opts.GetMaxAcceptableFlakiness() > 0 &&
		100*float64(colCells.passingCols)/float64(colCells.completedCols-colCells.ignoredCols) >= float64(100-opts.GetMaxAcceptableFlakiness()) {
		return true
	}

	return false
}

func allLinkedIssues(rows []*statepb.Row) []string {
	issueSet := make(map[string]bool)
	for _, row := range rows {
		for _, issueID := range row.Issues {
			issueSet[issueID] = true
		}
	}
	linkedIssues := []string{}
	for issueID := range issueSet {
		linkedIssues = append(linkedIssues, issueID)
	}
	return linkedIssues
}

// gridStats aggregates columnar and cellular metrics as a struct
type gridStats struct {
	passingCols   int
	completedCols int
	ignoredCols   int
	passingCells  int
	filledCells   int
}

// Culminate set of metrics related to a section of the Grid
func gridMetrics(cols int, rows []*statepb.Row, recent int, brokenThreshold float32, features FeatureFlags, opts *configpb.DashboardTabStatusCustomizationOptions) (gridStats, bool) {
	results := result.Map(rows)
	var passingCells int
	var filledCells int
	var passingCols int
	var completedCols int
	var ignoredCols int
	var brokenState bool

	for idx := 0; idx < cols; idx++ {
		if idx >= recent {
			break
		}
		var passes int
		var failures int
		var ignores int
		var other int
		for _, iter := range results {
			// TODO(fejta): fail old running cols
			r, _ := iter()
			// check for ignores first
			if features.AllowIgnoredColumns && result.Ignored(r, opts) {
				ignores++
			}
			// proceed with the rest of calculations
			status := coalesceResult(r, result.IgnoreRunning)
			if result.Passing(status) {
				passes++
				passingCells++
				filledCells++
			} else if result.Failing(status) {
				failures++
				filledCells++
			} else if status != statuspb.TestStatus_NO_RESULT {
				other++
				filledCells++
			}
		}

		if passes+failures+other > 0 {
			completedCols++
		}
		// only one of those can be true
		if ignores > 0 {
			ignoredCols++
		} else if failures == 0 && passes > 0 {
			passingCols++
		}

		if passes+failures > 0 && brokenThreshold > 0 {
			if float32(failures)/float32(passes+failures+other) > brokenThreshold {
				brokenState = true
			}
		}
	}

	metrics := gridStats{
		passingCols:   passingCols,
		completedCols: completedCols,
		ignoredCols:   ignoredCols,
		passingCells:  passingCells,
		filledCells:   filledCells,
	}

	return metrics, brokenState
}

// Add a subset of colCellMetrics to summary proto
func tabMetrics(colCells gridStats) *summarypb.DashboardTabSummaryMetrics {
	return &summarypb.DashboardTabSummaryMetrics{
		PassingColumns:   int32(colCells.passingCols),
		CompletedColumns: int32(colCells.completedCols),
		IgnoredColumns:   int32(colCells.ignoredCols),
	}
}

func fmtStatus(colCells gridStats, tabStatus summarypb.DashboardTabSummary_TabStatus, opts *configpb.DashboardTabStatusCustomizationOptions) string {
	colCent := 100 * float64(colCells.passingCols) / float64(colCells.completedCols)
	cellCent := 100 * float64(colCells.passingCells) / float64(colCells.filledCells)
	flakyCent := 100 * float64(colCells.completedCols-colCells.ignoredCols-colCells.passingCols) / float64(colCells.completedCols-colCells.ignoredCols)
	// put tab stats on a single line and additional status info on the next
	statusMsg := fmt.Sprintf("Tab stats: %d of %d (%.1f%%) recent columns passed (%d of %d or %.1f%% cells)", colCells.passingCols, colCells.completedCols, colCent, colCells.passingCells, colCells.filledCells, cellCent)
	if colCells.ignoredCols > 0 {
		statusMsg += fmt.Sprintf(". %d columns ignored", colCells.ignoredCols)
	}
	// add status info message for certain cases
	if tabStatus == summarypb.DashboardTabSummary_PENDING {
		statusMsg += "\nStatus info: Not enough runs"
	} else if tabStatus == summarypb.DashboardTabSummary_ACCEPTABLE {
		statusMsg += fmt.Sprintf("\nStatus info: Recent flakiness (%.1f%%) over valid columns is within configured acceptable level of %.1f%%.", flakyCent, opts.GetMaxAcceptableFlakiness())
	}
	return statusMsg
}

// Tab stats: 3 out of 5 (60.0%) recent columns passed (35 of 50 or 70.0% cells). 1 columns ignored.
// (OPTIONAL) Status info: Recent flakiness (40.0%) flakiness is within configured acceptable level of X
// OR Status info: Not enough runs
func statusMessage(colCells gridStats, tabStatus summarypb.DashboardTabSummary_TabStatus, opts *configpb.DashboardTabStatusCustomizationOptions) string {
	if colCells.filledCells == 0 {
		return noRuns
	}
	return fmtStatus(colCells, tabStatus, opts)
}

const noGreens = "no recent greens"

// latestGreen finds the ID for the most recent column with all passing rows.
//
// Returns the build, first extra column header and/or a no recent greens message.
func latestGreen(grid *statepb.Grid, useFirstExtra bool) string {
	results := result.Map(grid.Rows)
	for _, col := range grid.Columns {
		var failures bool
		var passes bool
		for _, resultF := range results {
			r, _ := resultF()
			result := coalesceResult(r, result.ShowRunning)
			if result == statuspb.TestStatus_PASS {
				passes = true
			}
			if result == statuspb.TestStatus_FLAKY || result == statuspb.TestStatus_FAIL || result == statuspb.TestStatus_UNKNOWN {
				failures = true
			}
		}
		if failures || !passes {
			continue
		}
		if useFirstExtra && len(col.Extra) > 0 {
			return col.Extra[0]
		}
		return col.Build
	}
	return noGreens
}

func getHealthinessForInterval(grid *statepb.Grid, tabName string, currentTime time.Time, interval int) *summarypb.HealthinessInfo {
	now := goBackDays(0, currentTime)
	oneInterval := goBackDays(interval, currentTime)
	twoIntervals := goBackDays(2*interval, currentTime)

	healthiness := CalculateHealthiness(grid, oneInterval, now, tabName)
	pastHealthiness := CalculateHealthiness(grid, twoIntervals, oneInterval, tabName)
	CalculateTrend(healthiness, pastHealthiness)

	healthiness.PreviousFlakiness = []float32{pastHealthiness.AverageFlakiness}
	return healthiness
}

func goBackDays(days int, currentTime time.Time) int {
	// goBackDays gets the time intervals for our flakiness report.
	// The old version of this function would round to the 12am of the given day.
	// Since the new flakiness report will be run with Summarizer and therefore more often
	// than the once-a-week of the old flakiness report, we will not round to 12am anymore.
	date := currentTime.AddDate(0, 0, -1*days)
	intDate := int(date.Unix())
	return intDate
}

func shouldRunHealthiness(tab *configpb.DashboardTab) bool {
	if tab.HealthAnalysisOptions == nil {
		return false
	}
	return tab.HealthAnalysisOptions.Enable
}

// coalesceResult reduces the result to PASS, NO_RESULT, FAIL or FLAKY.
func coalesceResult(rowResult statuspb.TestStatus, ignoreRunning bool) statuspb.TestStatus {
	return result.Coalesce(rowResult, ignoreRunning)
}
