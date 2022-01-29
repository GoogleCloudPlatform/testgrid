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

// gridReader returns the grid content and metadata (last updated time, generation id)
type gridReader func(ctx context.Context) (io.ReadCloser, time.Time, int64, error)

// groupFinder returns the named group as well as reader for the grid state
type groupFinder func(string) (*gcs.Path, *configpb.TestGroup, gridReader, error)

func fetchConfig(ctx context.Context, client gcs.ConditionalClient, configPath gcs.Path, dashboards []string) (*configpb.Configuration, *storage.ReaderObjectAttrs, error) {
	r, attrs, err := client.Open(ctx, configPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open: %w", err)
	}

	cfg, err := config.Unmarshal(r)
	if err != nil {
		return nil, nil, fmt.Errorf("unmarshal: %v", err)
	}

	if n := len(dashboards); n > 0 {
		valid := stringset.New(dashboards...)
		var found stringset.Set
		dashes := make([]*configpb.Dashboard, 0, 1)
		for _, d := range cfg.Dashboards {
			name := d.Name
			if !valid.Contains(name) {
				continue
			}
			found.Add(name)
			dashes = append(dashes, d)
		}
		if missing := valid.Diff(found); missing.Len() > 0 {
			return nil, nil, fmt.Errorf("not found: %s", missing)
		}
		cfg.Dashboards = dashes
	}
	return cfg, attrs, nil
}

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

// Update summary protos by reading the state protos defined in the config.
//
// Will use concurrency go routines to update dashboards in parallel.
// Setting dashboard will limit update to this dashboard.
// Will write summary proto when confirm is set.
func Update(ctx context.Context, client gcs.ConditionalClient, mets *Metrics, configPath gcs.Path, concurrency int, gridPathPrefix, summaryPathPrefix string, dashboards []string, confirm bool, freq time.Duration, fix Fixer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if concurrency < 1 {
		return fmt.Errorf("concurrency must be positive, got: %d", concurrency)
	}
	log := logrus.WithField("config", configPath)

	var q config.DashboardQueue

	onConfigUpdate := func(cfg *snapshot.Config) error {
		dashboards := cfg.AllDashboards()
		log := log.WithField("onConfigUpdate()", true)

		paths := make([]gcs.Path, 0, len(dashboards))
		for _, d := range dashboards {
			path, err := summaryPath(configPath, summaryPathPrefix, d.Name)
			if err != nil {
				log.WithError(err).WithField("dashboard", d.Name).Error("bad dashboard path")
			}
			paths = append(paths, *path)
		}

		stats := gcs.Stat(ctx, client, 10, paths...)
		whens := make(map[string]time.Time, len(stats))
		for i, stat := range stats {
			name := dashboards[i].Name
			switch {
			case stat.Attrs != nil:
				whens[name] = stat.Attrs.Updated.Add(freq)
			default:
				if errors.Is(stat.Err, storage.ErrObjectNotExist) {
					defer func(i int) {
						if _, err := lockDashboard(ctx, client, paths[i], 0); err != nil && !gcs.IsPreconditionFailed(err) {
							log.WithError(err).WithField("path", paths[i]).Error("Failed to lock initial summary")
						}
					}(i)
				} else {
					log.WithError(stat.Err).WithField("path", paths[i]).Info("Failed to stat")
				}
				whens[name] = time.Now()
			}
		}

		q.Init(log, cfg.AllDashboards(), time.Now().Add(freq))
		if err := q.FixAll(whens, false); err != nil {
			log.WithError(err).Error("Failed to fix all dashboards based on last update time")
		}
		return nil
	}

	log.Debug("Observing config...")
	cfg, err := snapshot.Observe(ctx, log, client, configPath, onConfigUpdate)
	if err != nil {
		return fmt.Errorf("observe config: %w", err)
	}

	if fix != nil {
		go func() {
			fix(ctx, &q)
		}()
	}

	var active stringset.Set
	var waiting stringset.Set
	var lock sync.Mutex

	go func() {
		ticker := time.NewTicker(time.Minute) // TODO(fejta): subscribe to notifications
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
			log.Info("Updating groups")
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
			}

		}
	}()
	dashboardNames := make(chan string)

	// TODO(fejta): cache downloaded group?
	findGroup := func(name string) (*gcs.Path, *configpb.TestGroup, gridReader, error) {
		group := cfg.Group(name)
		if group == nil {
			return nil, nil, nil, nil
		}
		groupPath, err := configPath.ResolveReference(&url.URL{Path: path.Join(gridPathPrefix, name)})
		if err != nil {
			return nil, group, nil, err
		}
		reader := func(ctx context.Context) (io.ReadCloser, time.Time, int64, error) {
			return pathReader(ctx, client, *groupPath)
		}
		return groupPath, group, reader, nil
	}

	updateName := func(log *logrus.Entry, dashName string) (logrus.FieldLogger, error) {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		dash := cfg.Dashboard(dashName)
		log.Debug("Summarizing dashboard")
		summaryPath, err := summaryPath(configPath, summaryPathPrefix, dashName)
		if err != nil {
			return log, fmt.Errorf("summary path: %v", err)
		}
		sum, _, _, err := readSummary(ctx, client, *summaryPath)
		if err != nil {
			return log, fmt.Errorf("read %q: %v", *summaryPath, err)
		}

		if sum == nil {
			sum = &summarypb.DashboardSummary{}
		}

		updateDashboard(ctx, client, dash, sum, findGroup)

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
			return log, nil
		}
		size, err := writeSummary(ctx, client, *summaryPath, sum)
		log = log.WithField("bytes", size)
		if err != nil {
			return log, fmt.Errorf("write: %w", err)
		}
		return log, nil
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
				if log, err := updateName(log, dashName); err != nil {
					finish.Fail()
					q.Fix(dashName, time.Now().Add(freq/2), false)
					log.WithError(err).Error("Failed to summarize dashboard")
				} else {
					finish.Success()
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

var (
	normalizer = regexp.MustCompile(`[^a-z0-9]+`)
)

func summaryPath(g gcs.Path, prefix, dashboard string) (*gcs.Path, error) {
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

func readSummary(ctx context.Context, client gcs.Client, path gcs.Path) (*summarypb.DashboardSummary, time.Time, int64, error) {
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
	_, err = client.Upload(ctx, path, buf, gcs.DefaultACL, "no-cache")
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
func updateDashboard(ctx context.Context, client gcs.Stater, dash *configpb.Dashboard, sum *summarypb.DashboardSummary, findGroup groupFinder) {
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
		groupName := tab.TestGroupName
		groupPath, group, groupReader, err := findGroup(groupName)
		if err != nil {
			tabSummaries[tab.Name] = tabStatus(dash.Name, tab.Name, fmt.Sprintf("Error reading group info: %v", err))
			continue
		}
		if group == nil {
			tabSummaries[tab.Name] = tabStatus(dash.Name, tab.Name, fmt.Sprintf("Test group does not exist: %q", groupName))
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

	// update the summaries, starting with most delayed
nextPath:
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
				break nextPath
			}
			log.Debug("Updating tab summary")
			s, err := updateTab(ctx, tab, info.group, info.reader)
			if err != nil {
				s = tabStatus(dash.Name, tab.Name, fmt.Sprintf("Error attempting to summarize tab: %v", err))
				log = log.WithError(err)
			} else {
				s.DashboardName = dash.Name
			}
			tabSummaries[tab.Name] = s
			log.Trace("Updated tab summary")
		}
	}

	// assemble them back into the dashboard summary.
	sum.TabSummaries = make([]*summarypb.DashboardTabSummary, len(dash.DashboardTab))
	for idx, tab := range dash.DashboardTab {
		sum.TabSummaries[idx] = tabSummaries[tab.Name]
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
func updateTab(ctx context.Context, tab *configpb.DashboardTab, group *configpb.TestGroup, groupReader gridReader) (*summarypb.DashboardTabSummary, error) {
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
	grid.Rows, err = filterGrid(tab.BaseOptions, grid.Rows, recent)
	if err != nil {
		return nil, fmt.Errorf("filter: %v", err)
	}

	latest, latestSeconds := latestRun(grid.Columns)
	alert := staleAlert(mod, latest, staleHours(tab))
	// TODO(chases2): Determine alerts (updater.alertRows) again; the pass/fail thresholds may be different on tabs
	failures := failingTestSummaries(grid.Rows)
	passingCols, completedCols, passingCells, filledCells, brokenState := gridMetrics(len(grid.Columns), grid.Rows, recent, tab.BrokenColumnThreshold)
	return &summarypb.DashboardTabSummary{
		DashboardTabName:     tab.Name,
		LastUpdateTimestamp:  float64(mod.Unix()),
		LastRunTimestamp:     float64(latestSeconds),
		Alert:                alert,
		FailingTestSummaries: failures,
		OverallStatus:        overallStatus(grid, recent, alert, brokenState, failures),
		Status:               statusMessage(passingCols, completedCols, passingCells, filledCells),
		LatestGreen:          latestGreen(grid, group.UseKubernetesClient),
		BugUrl:               tab.GetOpenBugTemplate().GetUrl(),
		Healthiness:          healthiness,
		LinkedIssues:         allLinkedIssues(grid.Rows),
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

const (
	includeFilter = "include-filter-by-regex"
	excludeFilter = "exclude-filter-by-regex"
	// TODO(fejta): others, which are not used by testgrid.k8s.io
)

// filterGrid truncates the grid to rows with recent results and matching the white/blacklist.
func filterGrid(baseOptions string, rows []*statepb.Row, recent int) ([]*statepb.Row, error) {

	vals, err := url.ParseQuery(baseOptions)
	if err != nil {
		return nil, fmt.Errorf("parse %q: %v", baseOptions, err)
	}

	rows = recentRows(rows, recent)

	rows = filterMethods(rows)

	for _, include := range vals[includeFilter] {
		if rows, err = includeRows(rows, include); err != nil {
			return nil, fmt.Errorf("bad %s=%s: %v", includeFilter, include, err)
		}
	}

	for _, exclude := range vals[excludeFilter] {
		if rows, err = excludeRows(rows, exclude); err != nil {
			return nil, fmt.Errorf("bad %s=%s: %v", excludeFilter, exclude, err)
		}
	}

	// TODO(fejta): grouping, which is not used by testgrid.k8s.io
	// TODO(fejta): sorting, unused by testgrid.k8s.io
	// TODO(fejta): graph, unused by testgrid.k8s.io
	// TODO(fejta): tabuluar, unused by testgrid.k8s.io
	return rows, nil
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

// includeRows returns the subset of rows that match the regex
func includeRows(in []*statepb.Row, include string) ([]*statepb.Row, error) {
	re, err := regexp.Compile(include)
	if err != nil {
		return nil, err
	}
	var rows []*statepb.Row
	for _, r := range in {
		if !re.MatchString(r.Name) {
			continue
		}
		rows = append(rows, r)
	}
	return rows, nil
}

// excludeRows returns the subset of rows that do not match the regex
func excludeRows(in []*statepb.Row, exclude string) ([]*statepb.Row, error) {
	re, err := regexp.Compile(exclude)
	if err != nil {
		return nil, err
	}
	var rows []*statepb.Row
	for _, r := range in {
		if re.MatchString(r.Name) {
			continue
		}
		rows = append(rows, r)
	}
	return rows, nil
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
// FLAKY - at least one recent column has failing cells
// PASS - all recent columns are entirely green
func overallStatus(grid *statepb.Grid, recent int, stale string, brokenState bool, alerts []*summarypb.FailingTestSummary) summarypb.DashboardTabSummary_TabStatus {
	if brokenState {
		return summarypb.DashboardTabSummary_BROKEN
	}
	if stale != "" {
		return summarypb.DashboardTabSummary_STALE
	}
	if len(alerts) > 0 {
		return summarypb.DashboardTabSummary_FAIL
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	results := results(ctx, grid.Rows)
	moreCols := true
	var found bool
	// We want to look at recent columns, skipping over any that are still running.
	for moreCols && recent > 0 {
		moreCols = false
		var foundCol bool
		var running bool
		// One result off each column since we don't know which
		// cells are running ahead of time.
		for _, resultCh := range results {
			r, ok := <-resultCh
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
			r = coalesceResult(r, result.IgnoreRunning)
			if r == statuspb.TestStatus_NO_RESULT {
				continue
			}
			// any failure in a recent column results in flaky
			if r != statuspb.TestStatus_PASS {
				return summarypb.DashboardTabSummary_FLAKY
			}
			foundCol = true
		}

		// Running columns are unfinished and therefore should
		// not count as "recent" until they finish.
		if running {
			continue
		}

		if foundCol {
			found = true
			recent--
		}

	}
	if found {
		return summarypb.DashboardTabSummary_PASS
	}
	return summarypb.DashboardTabSummary_UNKNOWN
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

// Culminate set of metrics related to a section of the Grid
func gridMetrics(cols int, rows []*statepb.Row, recent int, brokenThreshold float32) (int, int, int, int, bool) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	results := results(ctx, rows)
	var passingCells int
	var filledCells int
	var passingCols int
	var completedCols int
	var brokenState bool

	for idx := 0; idx < cols; idx++ {
		if idx >= recent {
			break
		}
		var passes int
		var failures int
		var other int
		for _, ch := range results {
			// TODO(fejta): fail old running cols
			status := coalesceResult(<-ch, result.IgnoreRunning)
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
		if failures == 0 && passes > 0 {
			passingCols++
		}

		if passes+failures > 0 && brokenThreshold > 0 {
			if float32(failures)/float32(passes+failures+other) > brokenThreshold {
				brokenState = true
			}
		}
	}

	return passingCols, completedCols, passingCells, filledCells, brokenState
}

func fmtStatus(passCols, cols, passCells, cells int) string {
	colCent := 100 * float64(passCols) / float64(cols)
	cellCent := 100 * float64(passCells) / float64(cells)
	return fmt.Sprintf("%d of %d (%.1f%%) recent columns passed (%d of %d or %.1f%% cells)", passCols, cols, colCent, passCells, cells, cellCent)
}

//  2483 of 115784 tests (2.1%) and 163 of 164 runs (99.4%) failed in the past 7 days
func statusMessage(passingCols, completedCols, passingCells, filledCells int) string {
	if filledCells == 0 {
		return noRuns
	}
	return fmtStatus(passingCols, completedCols, passingCells, filledCells)
}

const noGreens = "no recent greens"

// latestGreen finds the ID for the most recent column with all passing rows.
//
// Returns the build, first extra column header and/or a no recent greens message.
func latestGreen(grid *statepb.Grid, useFirstExtra bool) string {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := results(ctx, grid.Rows)
	for _, col := range grid.Columns {
		var failures bool
		var passes bool
		for _, resultCh := range results {
			result := coalesceResult(<-resultCh, result.ShowRunning)
			if result == statuspb.TestStatus_PASS {
				passes = true
			}
			if result == statuspb.TestStatus_FLAKY || result == statuspb.TestStatus_FAIL || result == statuspb.TestStatus_UNKNOWN {
				failures = true
				break
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

// resultIter returns a channel that outputs the result for each column, decoding the run-length-encoding.
func resultIter(ctx context.Context, results []int32) <-chan statuspb.TestStatus {
	return result.Iter(ctx, results)
}

// results returns a per-column result output channel for each row.
func results(ctx context.Context, rows []*statepb.Row) map[string]<-chan statuspb.TestStatus {
	return result.Map(ctx, rows)
}
