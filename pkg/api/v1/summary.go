/*
Copyright 2023 The TestGrid Authors.

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

package v1

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sort"

	"github.com/go-chi/chi"

	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/sirupsen/logrus"

	"github.com/GoogleCloudPlatform/testgrid/config"
	apipb "github.com/GoogleCloudPlatform/testgrid/pb/api/v1"
	summarypb "github.com/GoogleCloudPlatform/testgrid/pb/summary"
	"github.com/GoogleCloudPlatform/testgrid/pkg/summarizer"
)

// TODO(sultan-duisenbay) - consider setting these as API module flags in main.go
const (
	numSummaryFailingTests = 5
	numSummaryFlakyTests   = 5

	passing    = "PASSING"
	failing    = "FAILING"
	flaky      = "FLAKY"
	stale      = "STALE"
	broken     = "BROKEN"
	pending    = "PENDING"
	acceptable = "ACCEPTABLE"
	unknown    = "UNKNOWN"
)

var (
	// These should be in line with the TabStatus enum in state.proto.
	tabStatusStr = map[summarypb.DashboardTabSummary_TabStatus]string{
		summarypb.DashboardTabSummary_PASS:       passing,
		summarypb.DashboardTabSummary_FAIL:       failing,
		summarypb.DashboardTabSummary_FLAKY:      flaky,
		summarypb.DashboardTabSummary_STALE:      stale,
		summarypb.DashboardTabSummary_BROKEN:     broken,
		summarypb.DashboardTabSummary_PENDING:    pending,
		summarypb.DashboardTabSummary_ACCEPTABLE: acceptable,
	}
)

// convertSummary converts the tab summary from storage format (summary.proto) to wire format (data.proto)
func convertSummary(tabSummary *summarypb.DashboardTabSummary) *apipb.TabSummary {

	fs := extractFailuresSummary(tabSummary.GetFailingTestSummaries())
	hs := extractHealthinessSummary(tabSummary.GetHealthiness())
	return &apipb.TabSummary{
		DashboardName:         tabSummary.DashboardName,
		TabName:               tabSummary.DashboardTabName,
		OverallStatus:         tabStatusStr[tabSummary.OverallStatus],
		DetailedStatusMessage: tabSummary.Status,
		LastRunTimestamp:      generateTimestamp(tabSummary.LastRunTimestamp),
		LastUpdateTimestamp:   generateTimestamp(tabSummary.LastUpdateTimestamp),
		LatestPassingBuild:    tabSummary.LatestGreen,
		FailuresSummary:       fs,
		HealthinessSummary:    hs,
	}
}

// generateTimestamp converts the float to a pointer to Timestamp proto struct
func generateTimestamp(ts float64) *timestamp.Timestamp {
	sec, nano := math.Modf(ts)
	return &timestamp.Timestamp{
		Seconds: int64(sec),
		Nanos:   int32(nano * 1e9),
	}
}

// extractFailuresSummary extracts the most important info from summary proto's FailingTestSummaries field.
// This includes stats as well top failing tests info.
// Top failing tests # is determined by numSummaryFailingTests.
func extractFailuresSummary(failingTests []*summarypb.FailingTestSummary) *apipb.FailuresSummary {
	if len(failingTests) == 0 {
		return nil
	}

	// fetch top failing tests
	topN := int(math.Min(float64(numSummaryFailingTests), float64(len(failingTests))))

	sort.SliceStable(failingTests, func(i, j int) bool {
		return failingTests[i].FailCount > failingTests[j].FailCount
	})

	var topTests []*apipb.FailingTestInfo
	for i := 0; i < topN; i++ {
		test := failingTests[i]

		fti := &apipb.FailingTestInfo{
			DisplayName:   test.DisplayName,
			FailCount:     test.FailCount,
			PassTimestamp: generateTimestamp(test.PassTimestamp),
			FailTimestamp: generateTimestamp(test.FailTimestamp),
		}
		topTests = append(topTests, fti)
	}

	return &apipb.FailuresSummary{
		FailureStats: &apipb.FailureStats{
			NumFailingTests: int32(len(failingTests)),
		},
		TopFailingTests: topTests,
	}
}

// extractHealthinessSummary extracts the most important info from summary proto's Healthiness field.
// This includes stats as well top flaky tests info.
// Top flaky tests # is determined by numSummaryFlakyTests.
func extractHealthinessSummary(healthiness *summarypb.HealthinessInfo) *apipb.HealthinessSummary {
	if healthiness == nil {
		return nil
	}

	// obtain previous flakiness for the whole tab
	// need to distinguish between zero flakiness and absent flakiness
	var prevFlakiness float32
	if len(healthiness.PreviousFlakiness) == 0 {
		prevFlakiness = -1.0
	} else {
		prevFlakiness = healthiness.PreviousFlakiness[0]
	}

	sort.SliceStable(healthiness.Tests, func(i, j int) bool {
		return healthiness.Tests[i].Flakiness > healthiness.Tests[j].Flakiness
	})

	// fetch top flaky tests (with +ve flakiness)
	numFlakyTests := 0
	for i := 0; i < len(healthiness.Tests); i++ {
		t := healthiness.Tests[i]
		if t.Flakiness <= 0 {
			break
		}
		numFlakyTests++
	}

	topN := int(math.Min(float64(numSummaryFlakyTests), float64(numFlakyTests)))

	var topTests []*apipb.FlakyTestInfo
	for i := 0; i < topN; i++ {
		test := healthiness.Tests[i]
		fti := &apipb.FlakyTestInfo{
			DisplayName: test.DisplayName,
			Flakiness:   test.Flakiness,
			Change:      test.ChangeFromLastInterval,
		}
		topTests = append(topTests, fti)
	}

	return &apipb.HealthinessSummary{
		TopFlakyTests: topTests,
		HealthinessStats: &apipb.HealthinessStats{
			Start:             healthiness.Start,
			End:               healthiness.End,
			AverageFlakiness:  healthiness.AverageFlakiness,
			PreviousFlakiness: prevFlakiness,
			NumFlakyTests:     int32(numFlakyTests),
		},
	}
}

// fetchSummary returns the summary struct as defined in summary.proto.
// input dashboard doesn't have to be normalized.
// Returns an error iff the scope refers to non-existent bucket OR server fails to read the summary.
func (s *Server) fetchSummary(ctx context.Context, scope, dashboard string) (*summarypb.DashboardSummary, error) {
	configPath, _, err := s.configPath(scope)

	summaryPath, err := summarizer.SummaryPath(*configPath, s.SummaryPathPrefix, dashboard)
	if err != nil {
		return nil, fmt.Errorf("failed to create the summary path: %v", err)
	}

	summary, _, _, err := summarizer.ReadSummary(ctx, s.Client, *summaryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to download summary at %v: %v", summaryPath.String(), err)
	}

	return summary, nil
}

// ListTabSummaries returns the list of tab summaries for the particular dashboard.
// Dashboard name doesn't have to be normalized.
// Returns an error iff dashboard does not exist OR the server can't read summary from GCS bucket.
func (s *Server) ListTabSummaries(ctx context.Context, req *apipb.ListTabSummariesRequest) (*apipb.ListTabSummariesResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	scope := req.GetScope()
	cfg, err := s.getConfig(ctx, logrus.WithContext(ctx), scope)

	// TODO(sultan-duisenbay): return canonical error codes
	if err != nil {
		return nil, fmt.Errorf("failed to fetch config from {%q}: %v", scope, err)
	}

	cfg.Mutex.RLock()
	defer cfg.Mutex.RUnlock()

	dashboardKey := config.Normalize(req.GetDashboard())
	if _, ok := cfg.NormalDashboard[dashboardKey]; !ok {
		return nil, fmt.Errorf("dashboard {%q} not found", dashboardKey)
	}

	summary, err := s.fetchSummary(ctx, scope, dashboardKey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch summary for dashboard {%q}: %v", dashboardKey, err)
	}

	if summary == nil {
		return nil, fmt.Errorf("summary for dashboard {%q} not found.", dashboardKey)
	}

	var resp apipb.ListTabSummariesResponse
	for _, tabSummary := range summary.TabSummaries {
		ts := convertSummary(tabSummary)
		resp.TabSummaries = append(resp.TabSummaries, ts)
	}
	return &resp, nil

}

// ListTabSummariesHTTP returns the list of tab summaries as a json.
// Response json: ListTabSummariesResponse
func (s Server) ListTabSummariesHTTP(w http.ResponseWriter, r *http.Request) {
	req := apipb.ListTabSummariesRequest{
		Scope:     r.URL.Query().Get(scopeParam),
		Dashboard: chi.URLParam(r, "dashboard"),
	}
	resp, err := s.ListTabSummaries(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	s.writeJSON(w, resp)
}

// GetTabSummary returns the tab summary for the particular dashboard and tab.
// Dashboard and tab names don't have to be normalized.
// Returns an error iff
// - dashboard or tab does not exist
// - the server can't read summary from GCS bucket
// - tab summary for particular tab doesn't exist
func (s *Server) GetTabSummary(ctx context.Context, req *apipb.GetTabSummaryRequest) (*apipb.GetTabSummaryResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	scope := req.GetScope()
	cfg, err := s.getConfig(ctx, logrus.WithContext(ctx), scope)

	// TODO(sultan-duisenbay): return canonical error codes
	if err != nil {
		return nil, fmt.Errorf("failed to fetch config from {%q}: %v", scope, err)
	}

	cfg.Mutex.RLock()
	defer cfg.Mutex.RUnlock()

	reqDashboardName, reqTabName := req.GetDashboard(), req.GetTab()

	_, tabName, _, err := findDashboardTab(cfg, reqDashboardName, reqTabName)
	if err != nil {
		return nil, fmt.Errorf("invalid request input {%q, %q}: %v", reqDashboardName, reqTabName, err)
	}

	summary, err := s.fetchSummary(ctx, scope, reqDashboardName)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch summary for dashboard {%q}: %v", reqDashboardName, err)
	}

	if summary == nil {
		return nil, fmt.Errorf("summary for dashboard {%q} not found.", reqDashboardName)
	}

	var resp apipb.GetTabSummaryResponse
	for _, tabSummary := range summary.GetTabSummaries() {
		if tabSummary.DashboardTabName == tabName {
			resp.TabSummary = convertSummary(tabSummary)
			return &resp, nil
		}
	}

	return nil, fmt.Errorf("failed to find summary for tab {%q}.", tabName)
}

// GetTabSummaryHTTP returns the tab summary as a json.
// Response json: GetTabSummaryResponse
func (s Server) GetTabSummaryHTTP(w http.ResponseWriter, r *http.Request) {
	req := apipb.GetTabSummaryRequest{
		Scope:     r.URL.Query().Get(scopeParam),
		Dashboard: chi.URLParam(r, "dashboard"),
		Tab:       chi.URLParam(r, "tab"),
	}
	resp, err := s.GetTabSummary(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	s.writeJSON(w, resp)
}

// ListDashboardSummaries returns the list of dashboard summaries for the particular dashboard group. Think of it as aggregated view of ListTabSummaries data.
// Dashboard group name doesn't have to be normalized. Returns an error iff
// - dashboard name does not exist in config
// - the server can't read summary from GCS bucket
func (s *Server) ListDashboardSummaries(ctx context.Context, req *apipb.ListDashboardSummariesRequest) (*apipb.ListDashboardSummariesResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	scope := req.GetScope()
	cfg, err := s.getConfig(ctx, logrus.WithContext(ctx), scope)

	// TODO(sultan-duisenbay): return canonical error codes
	if err != nil {
		return nil, fmt.Errorf("failed to fetch config from {%q}: %v", scope, err)
	}

	cfg.Mutex.RLock()
	defer cfg.Mutex.RUnlock()

	dashboardGroupKey := config.Normalize(req.GetDashboardGroup())
	denormalizedName, ok := cfg.NormalDashboardGroup[dashboardGroupKey]
	if !ok {
		return nil, fmt.Errorf("dashboard group {%q} not found", denormalizedName)
	}

	var resp apipb.ListDashboardSummariesResponse
	for _, dashboardName := range cfg.Config.DashboardGroups[denormalizedName].DashboardNames {
		summary, err := s.fetchSummary(ctx, scope, dashboardName)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch summary for dashboard {%q}: %v", dashboardName, err)
		}
		// skip over non-existing dashboards
		if summary == nil {
			continue
		}
		resp.DashboardSummaries = append(resp.DashboardSummaries, dashboardSummary(summary, dashboardName))
	}

	return &resp, nil
}

// ListDashboardSummariesHTTP returns the list of dashboard summaries as a json.
// Response json: ListDashboardSummariesResponse
func (s Server) ListDashboardSummariesHTTP(w http.ResponseWriter, r *http.Request) {
	req := apipb.ListDashboardSummariesRequest{
		Scope:          r.URL.Query().Get(scopeParam),
		DashboardGroup: chi.URLParam(r, "dashboard-group"),
	}
	resp, err := s.ListDashboardSummaries(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	s.writeJSON(w, resp)
}

// GetDashboardSummary returns the dashboard summary for the particular dashboard. Think of it as aggregated view of ListTabSummaries data.
// Dashboard name doesn't have to be normalized. Returns an error iff
// - dashboard name does not exist in config
// - the server can't read summary from GCS bucket
// - dashboard summary doesn't exist
func (s *Server) GetDashboardSummary(ctx context.Context, req *apipb.GetDashboardSummaryRequest) (*apipb.GetDashboardSummaryResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	scope := req.GetScope()
	cfg, err := s.getConfig(ctx, logrus.WithContext(ctx), scope)

	// TODO(sultan-duisenbay): return canonical error codes
	if err != nil {
		return nil, fmt.Errorf("failed to fetch config from {%q}: %v", scope, err)
	}

	cfg.Mutex.RLock()
	defer cfg.Mutex.RUnlock()

	dashboardKey := config.Normalize(req.GetDashboard())
	denormalizedName, ok := cfg.NormalDashboard[dashboardKey]
	if !ok {
		return nil, fmt.Errorf("dashboard {%q} not found", dashboardKey)
	}

	summary, err := s.fetchSummary(ctx, scope, dashboardKey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch summary for dashboard {%q}: %v", dashboardKey, err)
	}

	if summary == nil {
		return nil, fmt.Errorf("summary for dashboard {%q} not found", dashboardKey)
	}

	return &apipb.GetDashboardSummaryResponse{
		DashboardSummary: dashboardSummary(summary, denormalizedName),
	}, nil

}

// GetDashboardSummaryHTTP returns the dashboard summary as a json.
// Response json: GetDashboardSummaryResponse
func (s Server) GetDashboardSummaryHTTP(w http.ResponseWriter, r *http.Request) {
	req := apipb.GetDashboardSummaryRequest{
		Scope:     r.URL.Query().Get(scopeParam),
		Dashboard: chi.URLParam(r, "dashboard"),
	}
	resp, err := s.GetDashboardSummary(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	s.writeJSON(w, resp)
}

// dashboardSummary generates a dashboard summary in a wire data format defined in api/v1/data.proto
// overall dashboard status is defined by priority/severity within this function.
func dashboardSummary(summary *summarypb.DashboardSummary, dashboardName string) *apipb.DashboardSummary {

	tabStatusCount := make(map[string]int32)
	for _, tab := range summary.TabSummaries {
		statusStr := tabStatusStr[tab.OverallStatus]
		tabStatusCount[statusStr]++
	}

	overallStatus := unknown
	switch {
	case tabStatusCount[broken] > 0:
		overallStatus = broken
	case tabStatusCount[stale] > 0:
		overallStatus = stale
	case tabStatusCount[failing] > 0:
		overallStatus = failing
	case tabStatusCount[flaky] > 0:
		overallStatus = flaky
	case tabStatusCount[pending] > 0:
		overallStatus = pending
	case tabStatusCount[acceptable] > 0:
		overallStatus = acceptable
	case tabStatusCount[passing] > 0:
		overallStatus = passing
	}

	return &apipb.DashboardSummary{
		Name:           dashboardName,
		OverallStatus:  overallStatus,
		TabStatusCount: tabStatusCount,
	}
}
