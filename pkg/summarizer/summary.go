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
	"regexp"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"github.com/golang/protobuf/proto"
	"github.com/sirupsen/logrus"

	"github.com/GoogleCloudPlatform/testgrid/config"
	"github.com/GoogleCloudPlatform/testgrid/internal/result"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	summarypb "github.com/GoogleCloudPlatform/testgrid/pb/summary"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
)

// gridReader returns the grid content and metadata (last updated time, generation id)
type gridReader func(ctx context.Context) (io.ReadCloser, time.Time, int64, error)

// groupFinder returns the named group as well as reader for the grid state
type groupFinder func(string) (*configpb.TestGroup, gridReader, error)

// Update summary protos by reading the state protos defined in the config.
//
// Will use concurrency go routines to update dashboards in parallel.
// Setting dashboard will limit update to this dashboard.
// Will write summary proto when confirm is set.
func Update(ctx context.Context, client *storage.Client, path gcs.Path, concurrency int, dashboard string, confirm bool) error {
	if concurrency < 1 {
		return fmt.Errorf("concurrency must be positive, got: %d", concurrency)
	}
	cfg, err := config.ReadGCS(ctx, client.Bucket(path.Bucket()).Object(path.Object()))
	if err != nil {
		return fmt.Errorf("Failed to read config: %w", err)
	}
	logrus.Infof("Found %d dashboards", len(cfg.Dashboards))

	dashboards := make(chan *configpb.Dashboard)
	var wg sync.WaitGroup

	groupFinder := func(name string) (*configpb.TestGroup, gridReader, error) {
		group := config.FindTestGroup(name, cfg)
		if group == nil {
			return nil, nil, nil
		}
		path, err := path.ResolveReference(&url.URL{Path: name})
		if err != nil {
			return group, nil, err
		}
		reader := func(ctx context.Context) (io.ReadCloser, time.Time, int64, error) {
			return pathReader(ctx, client, *path)
		}
		return group, reader, nil
	}

	errCh := make(chan error)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			for dash := range dashboards {
				log := logrus.WithField("dashboard", dash.Name)
				log.Info("Summarizing dashboard")
				sum, err := updateDashboard(ctx, dash, groupFinder)
				if err != nil {
					log.WithError(err).Error("Cannot summarize dashboard")
					errCh <- errors.New(dash.Name)
					continue
				}
				log.WithField("summary", sum).Info("summarized")
				if !confirm {
					continue
				}
				path, err := path.ResolveReference(&url.URL{Path: summaryPath(dash.Name)})
				if err != nil {
					log.WithError(err).Error("Cannot resolve summary path")
					errCh <- errors.New(dash.Name)
					continue
				}
				if err := writeSummary(ctx, client, *path, sum); err != nil {
					log.WithError(err).Error("Cannot write summary")
					errCh <- errors.New(dash.Name)
					continue
				}
				errCh <- nil
			}
			wg.Done()
		}()
	}

	resultCh := make(chan error)
	go func() {
		var errs []string
		for err := range errCh {
			if err == nil {
				continue
			}
			errs = append(errs, err.Error())
		}
		if n := len(errs); n > 0 {
			resultCh <- fmt.Errorf("failed to update %d dashboards: %v", n, strings.Join(errs, ", "))
		}
		resultCh <- nil
		close(resultCh)
	}()

	for _, d := range cfg.Dashboards {
		if dashboard != "" && dashboard != d.Name {
			logrus.WithField("dashboard", d.Name).Info("Skipping")
			continue
		}
		dashboards <- d
	}
	close(dashboards)
	wg.Wait()
	close(errCh)
	return <-resultCh
}

var (
	normalizer = regexp.MustCompile(`[^a-z0-9]+`)
)

func summaryPath(name string) string {
	// ''.join(c for c in n.lower() if c is alphanumeric
	return "summary-" + normalizer.ReplaceAllString(strings.ToLower(name), "")
}

func writeSummary(ctx context.Context, client *storage.Client, path gcs.Path, sum *summarypb.DashboardSummary) error {
	buf, err := proto.Marshal(sum)
	if err != nil {
		return fmt.Errorf("marshal: %v", err)
	}
	return gcs.Upload(ctx, client, path, buf, gcs.DefaultAcl, "no-cache") // TODO(fejta): configurable cache value
}

// pathReader returns a reader for the specified path and last modified, generation metadata.
func pathReader(ctx context.Context, client *storage.Client, path gcs.Path) (io.ReadCloser, time.Time, int64, error) {
	r, err := client.Bucket(path.Bucket()).Object(path.Object()).NewReader(ctx)
	if err != nil {
		return nil, time.Time{}, 0, fmt.Errorf("read %s: %w", path, err)
	}
	return r, r.Attrs.LastModified, r.Attrs.Generation, nil
}

// updateDashboard will summarize all the tabs (through errors), returning an error if any fail to summarize.
func updateDashboard(ctx context.Context, dash *configpb.Dashboard, finder groupFinder) (*summarypb.DashboardSummary, error) {
	log := logrus.WithField("dashboard", dash.Name)
	var badTabs []string
	var sum summarypb.DashboardSummary
	for _, tab := range dash.DashboardTab {
		log := log.WithField("tab", tab.Name)
		log.Info("Summarizing tab")
		s, err := updateTab(ctx, tab, finder)
		if err != nil {
			log.WithError(err).Error("Cannot summarize tab")
			badTabs = append(badTabs, tab.Name)
			sum.TabSummaries = append(sum.TabSummaries, problemTab(dash.Name, tab.Name))
			continue
		}
		s.DashboardName = dash.Name
		sum.TabSummaries = append(sum.TabSummaries, s)
	}
	var err error
	if d := len(badTabs); d > 0 {
		err = fmt.Errorf("Failed %d tabs: %s", d, strings.Join(badTabs, ", "))
	}
	return &sum, err
}

// problemTab summarizes a tab that cannot summarize
func problemTab(dashboardName, tabName string) *summarypb.DashboardTabSummary {
	return &summarypb.DashboardTabSummary{
		DashboardName:    dashboardName,
		DashboardTabName: tabName,
		Alert:            "failed to summarize tab",
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
func updateTab(ctx context.Context, tab *configpb.DashboardTab, findGroup groupFinder) (*summarypb.DashboardTabSummary, error) {
	groupName := tab.TestGroupName
	group, groupReader, err := findGroup(groupName)
	if err != nil {
		return nil, fmt.Errorf("find group: %v", err)
	}
	if group == nil {
		return nil, fmt.Errorf("not found: %q", groupName)
	}
	grid, mod, _, err := readGrid(ctx, groupReader) // TODO(fejta): track gen
	if err != nil && errors.Is(err, storage.ErrObjectNotExist) {
		return &summarypb.DashboardTabSummary{
			DashboardTabName: tab.Name,
			Alert:            noRuns,
			OverallStatus:    overallStatus(nil, 0, noRuns, nil),
			Status:           noRuns,
			LatestGreen:      noGreens,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load %s: %v", groupName, err)
	}

	recent := recentColumns(tab, group)
	grid.Rows, err = filterGrid(tab.BaseOptions, grid.Rows, recent)
	if err != nil {
		return nil, fmt.Errorf("filter: %v", err)
	}

	latest, latestSeconds := latestRun(grid.Columns)
	alert := staleAlert(mod, latest, staleHours(tab))
	failures := failingTestSummaries(grid.Rows)
	passingCols, completedCols, passingCells, filledCells, columnFailureRatios := gridMetrics(len(grid.Columns), grid.Rows, recent)
	return &summarypb.DashboardTabSummary{
		DashboardTabName:     tab.Name,
		LastUpdateTimestamp:  float64(mod.Unix()),
		LastRunTimestamp:     float64(latestSeconds),
		Alert:                alert,
		FailingTestSummaries: failures,
		OverallStatus:        overallStatus(grid, recent, alert, failures),
		Status:               statusMessage(passingCols, completedCols, passingCells, filledCells),
		LatestGreen:          latestGreen(grid, group.UseKubernetesClient),
		// TODO(fejta): BugUrl
		ColumnFailureRatios: columnFailureRatios,
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
		if statepb.Row_Result(r.Results[0]) == statepb.Row_NO_RESULT && int(r.Results[1]) >= recent {
			continue
		}
		rows = append(rows, r)
	}
	return rows
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
			return time.Unix(start, 0), start
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
	if ran.IsZero() {
		return noRuns
	}
	if stale == 0 {
		return ""
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
			DisplayName:    row.Name,
			TestName:       row.Id,
			FailBuildId:    alert.FailBuildId,
			FailCount:      alert.FailCount,
			FailureMessage: alert.FailureMessage,
			PassBuildId:    alert.PassBuildId,
			// TODO(fejta): better build info
			BuildLink:     alert.BuildLink,
			BuildLinkText: alert.BuildLinkText,
			BuildUrlText:  alert.BuildUrlText,
			// TODO(fejta): LinkedBugs
			FailTestLink: buildFailLink(alert.FailTestId, row.Id),
			// TODO(fejta): FailTestLink
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
	return fmt.Sprintf("%s %s", testID, target)
}

// overallStatus determines whether the tab is stale, failing, flaky or healthy.
func overallStatus(grid *statepb.Grid, recent int, stale string, alerts []*summarypb.FailingTestSummary) summarypb.DashboardTabSummary_TabStatus {
	if stale != "" {
		return summarypb.DashboardTabSummary_STALE
	}
	if len(alerts) > 0 {
		return summarypb.DashboardTabSummary_FAIL
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	results := results(ctx, grid.Rows)
	var found bool
	for _, resultCh := range results {
		recentResults := recent
		for r := range resultCh {
			// TODO(fejta): fail old running results.
			r = coalesceResult(r, result.IgnoreRunning)
			if r == statepb.Row_NO_RESULT {
				continue
			}
			if r != statepb.Row_PASS {
				return summarypb.DashboardTabSummary_FLAKY
			}
			found = true
			recentResults--
			if recentResults == 0 {
				break
			}
		}

	}
	if found {
		return summarypb.DashboardTabSummary_PASS
	}
	return summarypb.DashboardTabSummary_UNKNOWN
}

// Culminate set of metrics related to a section of the Grid
func gridMetrics(cols int, rows []*statepb.Row, recent int) (int, int, int, int, []float32) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	results := results(ctx, rows)
	var columnFailureRatios []float32
	var passingCells int
	var filledCells int
	var passingCols int
	var completedCols int

	for idx := 0; idx < cols; idx++ {
		if idx >= recent {
			break
		}
		var passes int
		var failures int
		for _, ch := range results {
			// TODO(fejta): fail old running cols
			switch coalesceResult(<-ch, result.IgnoreRunning) {
			//TODO(michelle192837): Create utility to standardize pass/fail boundaries
			case statepb.Row_PASS, statepb.Row_PASS_WITH_ERRORS, statepb.Row_PASS_WITH_SKIPS:
				passes++
				passingCells++
				filledCells++
			case statepb.Row_NO_RESULT:
				// noop
			default:
				failures++
				filledCells++
			}
		}

		if failures > 0 || passes > 0 {
			completedCols++
		}
		if failures == 0 && passes > 0 {
			passingCols++
		}

		if passes+failures > 0 {
			columnFailureRatios = append(columnFailureRatios, float32(failures)/float32(passes+failures))
		} else {
			columnFailureRatios = append(columnFailureRatios, 0.0)
		}
	}

	return passingCols, completedCols, passingCells, filledCells, columnFailureRatios
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
			result := coalesceResult(<-resultCh, result.FailRunning)
			if result == statepb.Row_PASS {
				passes = true
			}
			if result == statepb.Row_FLAKY || result == statepb.Row_FAIL {
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

// coalesceResult reduces the result to PASS, NO_RESULT, FAIL or FLAKY.
func coalesceResult(rowResult statepb.Row_Result, ignoreRunning bool) statepb.Row_Result {
	return result.Coalesce(rowResult, ignoreRunning)
}

// resultIter returns a channel that outputs the result for each column, decoding the run-length-encoding.
func resultIter(ctx context.Context, results []int32) <-chan statepb.Row_Result {
	return result.Iter(ctx, results)
}

// results returns a per-column result output channel for each row.
func results(ctx context.Context, rows []*statepb.Row) map[string]<-chan statepb.Row_Result {
	return result.Map(ctx, rows)
}
