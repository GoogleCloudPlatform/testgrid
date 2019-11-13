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

package main

import (
	"compress/zlib"
	"context"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/url"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"github.com/golang/protobuf/proto"
	"github.com/sirupsen/logrus"

	"github.com/GoogleCloudPlatform/testgrid/config"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/pb/state"
	"github.com/GoogleCloudPlatform/testgrid/pb/summary"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
)

type options struct {
	config      gcs.Path // gcs://path/to/config/proto
	creds       string
	confirm     bool
	dashboard   string
	concurrency int
}

func (o *options) validate() error {
	if o.config.String() == "" {
		return errors.New("empty --config")
	}
	if o.config.Bucket() == "k8s-testgrid" && o.confirm { // TODO(fejta): remove
		return fmt.Errorf("--config=%q cannot start with gs://k8s-testgrid", o.config)
	}
	if o.concurrency == 0 {
		o.concurrency = 4 * runtime.NumCPU()
	}
	return nil
}

func gatherOptions() options {
	var o options
	flag.Var(&o.config, "config", "gs://path/to/config.pb")
	flag.StringVar(&o.creds, "gcp-service-account", "", "/path/to/gcp/creds (use local creds if empty)")
	flag.BoolVar(&o.confirm, "confirm", false, "Upload data if set")
	flag.StringVar(&o.dashboard, "dashboard", "", "Only update named dashboard if set")
	flag.IntVar(&o.concurrency, "concurrency", 0, "Manually define the number of groups to concurrently update if non-zero")
	flag.Parse()
	return o
}

type groupFinder func(string) (*configpb.TestGroup, *gcs.Path, error)

func main() {

	opt := gatherOptions()
	if err := opt.validate(); err != nil {
		logrus.Fatalf("Invalid flags: %v", err)
	}
	if !opt.confirm {
		logrus.Info("--confirm=false (DRY-RUN): will not write to gcs")
	}

	ctx := context.Background()
	client, err := gcs.ClientWithCreds(ctx, opt.creds)
	if err != nil {
		logrus.Fatalf("Failed to read storage client: %v", err)
	}

	cfg, err := config.ReadGCS(ctx, client.Bucket(opt.config.Bucket()).Object(opt.config.Object()))
	if err != nil {
		logrus.Fatalf("Failed to read %q: %v", opt.config, err)
	}
	logrus.Infof("Found %d dashboards", len(cfg.Dashboards))

	dashboards := make(chan *configpb.Dashboard)
	var wg sync.WaitGroup

	groupFinder := func(name string) (*configpb.TestGroup, *gcs.Path, error) {
		group := config.FindTestGroup(name, cfg)
		if group == nil {
			return nil, nil, nil
		}
		path, err := opt.config.ResolveReference(&url.URL{Path: name})
		return group, path, err
	}

	for i := 0; i < opt.concurrency; i++ {
		wg.Add(1)
		go func() {
			for dash := range dashboards {
				if err := updateDashboard(ctx, client, dash, groupFinder); err != nil {
					logrus.WithField("dashboard", dash.Name).WithError(err).Fatal("Cannot summarize dashboard")
				}
			}
			wg.Done()
		}()
	}

	for _, d := range cfg.Dashboards {
		if opt.dashboard != "" && opt.dashboard != d.Name {
			logrus.WithField("dashboard", d.Name).Info("Skipping")
			continue
		}
		dashboards <- d
	}
	close(dashboards)
	wg.Wait()
}

func updateDashboard(ctx context.Context, client *storage.Client, dash *configpb.Dashboard, finder groupFinder) error {
	log := logrus.WithField("dashboard", dash.Name)
	var badTabs []string
	var sum summary.DashboardSummary
	for _, tab := range dash.DashboardTab {
		s, err := updateTab(ctx, client, tab, finder)
		if err != nil {
			log.WithField("tab", tab.Name).WithError(err).Error("Cannot summarize tab")
			badTabs = append(badTabs, tab.Name)
		}
		sum.TabSummaries = append(sum.TabSummaries, s)
	}
	if d := len(badTabs); d > 0 {
		return fmt.Errorf("Failed %d tabs: %s", d, strings.Join(badTabs, ", "))
	}
	log.WithField("summary", sum).Info("summarized") // TODO(fejta): write it
	return nil
}

func staleHours(tab *configpb.DashboardTab) time.Duration {
	if tab.AlertOptions == nil {
		return 0
	}
	return time.Duration(tab.AlertOptions.AlertStaleResultsHours) * time.Hour
}

func updateTab(ctx context.Context, client *storage.Client, tab *configpb.DashboardTab, findGroup groupFinder) (*summary.DashboardTabSummary, error) {
	groupName := tab.TestGroupName
	group, groupPath, err := findGroup(groupName)
	if err != nil {
		return nil, fmt.Errorf("find group: %v", err)
	}
	if group == nil {
		return nil, fmt.Errorf("not found: %q", groupName)
	}
	grid, mod, _, err := loadGrid(ctx, client, *groupPath) // TODO(fejta): track gen
	if err != nil {
		return nil, fmt.Errorf("load %q: %v", groupName, err)
	}

	recent := recentColumns(tab, group)
	filterGrid(tab, grid, recent)

	latest, latestSeconds := latestRun(grid.Columns)
	alert := staleAlert(mod, latest, staleHours(tab))
	failures := failingTestSummaries(grid.Rows)
	return &summary.DashboardTabSummary{
		DashboardTabName:     tab.Name,
		LastUpdateTimestamp:  float64(mod.Unix()),
		LastRunTimestamp:     float64(latestSeconds),
		Alert:                alert,
		FailingTestSummaries: failures,
		OverallStatus:        overallStatus(grid, recent, alert, failures),
		// TODO(fejta): Status:               summarize(grid, recent),
		LatestGreen: latestGreen(grid),
		// TODO(fejta): BugUrl
	}, nil
}

// loadGrid downloads and deserializes the current test group state.
func loadGrid(ctx context.Context, client *storage.Client, path gcs.Path) (*state.Grid, time.Time, int64, error) {
	var t time.Time
	r, err := client.Bucket(path.Bucket()).Object(path.Object()).NewReader(ctx)
	if err != nil {
		return nil, t, 0, fmt.Errorf("open %q: %v", path, err)
	}
	defer r.Close()
	zlibReader, err := zlib.NewReader(r)
	if err != nil {
		return nil, t, 0, fmt.Errorf("decompress %s: %v", path, err)
	}
	buf, err := ioutil.ReadAll(zlibReader)
	if err != nil {
		return nil, t, 0, fmt.Errorf("read %s: %v", path, err)
	}
	var g state.Grid
	if err = proto.Unmarshal(buf, &g); err != nil {
		return nil, t, 0, fmt.Errorf("parse %s: %v", path, err)
	}
	return &g, r.Attrs.LastModified, r.Attrs.Generation, nil
}

// recentColumns returns the configured number of recent columns to summarize, or 5.
func recentColumns(tab *configpb.DashboardTab, group *configpb.TestGroup) int {
	return firstFilled(tab.NumColumnsRecent, group.NumColumnsRecent, 5)
}

func failuresToOpen(tab *configpb.DashboardTab, group *configpb.TestGroup) int {
	return firstFilled(tab.AlertOptions.NumFailuresToAlert, group.NumFailuresToAlert)

}

// passesToClose returns the number of consecutive passes configured to close an alert, or 1.
func passesToClose(tab configpb.DashboardTab, group *configpb.TestGroup) int {
	return firstFilled(tab.AlertOptions.NumPassesToDisableAlert, group.NumPassesToDisableAlert, 1)
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

// filterGrid truncates the grid to rows with recent results and matching the white/blacklist.
func filterGrid(tab *configpb.DashboardTab, grid *state.Grid, recent int) error {
	const (
		includeFilter = "include-filter-by-regex"
		excludeFilter = "exclude-filter-by-regex"
		// TODO(fejta): others, which are not used by testgrid.k8s.io
	)

	vals, err := url.ParseQuery(tab.BaseOptions)
	if err != nil {
		return fmt.Errorf("parse base options %q: %v", tab.BaseOptions, err)
	}

	grid.Rows = recentRows(grid.Rows, recent)

	for _, include := range vals[includeFilter] {
		if grid.Rows, err = includeRows(grid.Rows, include); err != nil {
			return fmt.Errorf("bad %s=%s: %v", includeFilter, include, err)
		}
	}

	for _, exclude := range vals[excludeFilter] {
		if grid.Rows, err = excludeRows(grid.Rows, exclude); err != nil {
			return fmt.Errorf("bad %s=%s: %v", excludeFilter, exclude, err)
		}
	}

	// TODO(fejta): grouping, which is not used by testgrid.k8s.io
	// TODO(fejta): sorting, unused by testgrid.k8s.io
	// TODO(fejta): graph, unused by testgrid.k8s.io
	// TODO(fejta): tabuluar, unused by testgrid.k8s.io
	return nil
}

// recentRows returns the subset of rows with at least one recent result
func recentRows(in []*state.Row, recent int) []*state.Row {
	var rows []*state.Row
	for _, r := range in {
		if state.Row_Result(r.Results[0]) == state.Row_NO_RESULT && int(r.Results[1]) >= recent {
			continue
		}
		rows = append(rows, r)
	}
	return rows
}

// includeRows returns the subset of rows that match the regex
func includeRows(in []*state.Row, include string) ([]*state.Row, error) {
	re, err := regexp.Compile(include)
	if err != nil {
		return nil, err
	}
	var rows []*state.Row
	for _, r := range in {
		if !re.MatchString(r.Name) {
			continue
		}
		rows = append(rows, r)
	}
	return rows, nil
}

// excludeRows returns the subset of rows that do not match the regex
func excludeRows(in []*state.Row, exclude string) ([]*state.Row, error) {
	re, err := regexp.Compile(exclude)
	if err != nil {
		return nil, err
	}
	var rows []*state.Row
	for _, r := range in {
		if re.MatchString(r.Name) {
			continue
		}
		rows = append(rows, r)
	}
	return rows, nil
}

// latestRun returns the Time (and seconds-since-epoch) of the most recent run.
func latestRun(columns []*state.Column) (time.Time, int64) {
	for _, col := range columns {
		start := int64(col.Started)
		if start > 0 {
			return time.Unix(start, 0), start
		}
		return time.Time{}, start
	}
	return time.Time{}, 0
}

// staleAlert returns an explanatory message if the latest results are stale.
func staleAlert(mod, ran time.Time, stale time.Duration) string {
	if mod.IsZero() {
		return "no stored results"
	}
	if ran.IsZero() {
		return "no completed results"
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
func failingTestSummaries(rows []*state.Row) []*summary.FailingTestSummary {
	var failures []*summary.FailingTestSummary
	for _, row := range rows {
		if row.AlertInfo == nil {
			continue
		}
		alert := row.AlertInfo
		failures = append(failures, &summary.FailingTestSummary{
			DisplayName: row.Name,
			TestName:    row.Id,
			FailBuildId: alert.FailBuildId,
			// TODO(fejta): FailTimestamp
			PassBuildId: alert.PassBuildId,
			// TODO(fejta): PassTimestamp
			FailCount:      alert.FailCount,
			BuildLink:      alert.BuildLink,
			BuildLinkText:  alert.BuildLinkText,
			BuildUrlText:   alert.BuildUrlText,
			FailureMessage: alert.FailureMessage,
			// TODO(fejta): LinkedBugs
			// TODO(fejta): FailTestLink
		})
	}
	return failures
}

// overallStatus determines whether the tab is stale, failing, flaky or healthy.
func overallStatus(grid *state.Grid, recent int, stale string, alerts []*summary.FailingTestSummary) summary.DashboardTabSummary_TabStatus {
	if stale != "" {
		return summary.DashboardTabSummary_STALE
	}
	if len(alerts) > 0 {
		return summary.DashboardTabSummary_FAIL
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	results := results(ctx, grid.Rows)
	var found bool
	for _, resultCh := range results {
		recentResults := recent
		for result := range resultCh {
			result = coalesceResult(result)
			if result == state.Row_NO_RESULT {
				continue
			}
			if result != state.Row_PASS {
				return summary.DashboardTabSummary_FLAKY
			}
			recentResults--
			if recentResults == 0 {
				break
			}
			found = true
		}

	}
	if found {
		return summary.DashboardTabSummary_PASS
	}
	return summary.DashboardTabSummary_UNKNOWN
}

// latestGreen finds the ID for the most recent column with all passing rows.
func latestGreen(grid *state.Grid) string {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := results(ctx, grid.Rows)
	for _, col := range grid.Columns {
		var failures bool
		var passes bool
		for _, resultCh := range results {
			result := coalesceResult(<-resultCh)
			if result == state.Row_PASS {
				passes = true
			}
			if result == state.Row_FLAKY || result == state.Row_FAIL {
				failures = true
				break
			}
		}
		if passes && !failures {
			return col.Extra[0]
		}
	}
	return "no recent greens"
}

// coalesceResult reduces the result to PASS, NO_RESULT, FAIL or FLAKY.
func coalesceResult(result state.Row_Result) state.Row_Result {
	// TODO(fejta): other result types, not used by k8s testgrid
	const allowRunning = true // TODO(fejta): move to arg, auto-fail old results
	if result == state.Row_NO_RESULT || result == state.Row_RUNNING && allowRunning {
		return state.Row_NO_RESULT
	}
	if result == state.Row_FAIL || result == state.Row_RUNNING {
		return state.Row_FAIL
	}
	if result == state.Row_FLAKY {
		return result
	}
	return state.Row_PASS
}

// resultIter returns a channel that outputs the result for each column, decoding the run-length-encoding.
func resultIter(ctx context.Context, results []int32) <-chan state.Row_Result {
	out := make(chan state.Row_Result)
	go func() {
		defer close(out)
		for i := 0; i+1 < len(results); i += 2 {
			result := state.Row_Result(results[i])
			count := results[i+1]
			for count > 0 {
				select {
				case <-ctx.Done():
					return
				case out <- result:
					count--
				}
			}
		}
	}()
	return out
}

// results returns a per-column result output channel for each row.
func results(ctx context.Context, rows []*state.Row) map[string]<-chan state.Row_Result {
	iters := map[string]<-chan state.Row_Result{}
	for _, r := range rows {
		iters[r.Name] = resultIter(ctx, r.Results)
	}
	return iters
}
