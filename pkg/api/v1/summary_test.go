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
	"net/http"
	"net/http/httptest"
	"testing"

	apipb "github.com/GoogleCloudPlatform/testgrid/pb/api/v1"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	summarypb "github.com/GoogleCloudPlatform/testgrid/pb/summary"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestListTabSummaries(t *testing.T) {
	tests := []struct {
		name        string
		config      map[string]*configpb.Configuration
		summaries   map[string]*summarypb.DashboardSummary
		req         *apipb.ListTabSummariesRequest
		want        *apipb.ListTabSummariesResponse
		expectError bool
	}{
		{
			name: "Returns an error when there's no dashboard in config",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {},
			},
			req: &apipb.ListTabSummariesRequest{
				Dashboard: "missing",
			},
			expectError: true,
		},
		{
			name: "Returns an error when there's no summary for dashboard yet",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name: "ACME",
							DashboardTab: []*configpb.DashboardTab{
								{
									Name:          "me-me",
									TestGroupName: "testgroupname",
								},
							},
						},
					},
				},
			},
			req: &apipb.ListTabSummariesRequest{
				Dashboard: "acme",
			},
			expectError: true,
		},
		{
			name: "Returns correct tab summaries for a dashboard, with failing tests",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name: "Marco",
							DashboardTab: []*configpb.DashboardTab{
								{
									Name:          "polo-1",
									TestGroupName: "cheesecake",
								},
								{
									Name:          "polo-2",
									TestGroupName: "tiramisu",
								},
							},
						},
					},
				},
			},
			summaries: map[string]*summarypb.DashboardSummary{
				"gs://default/summary/summary-marco": {
					TabSummaries: []*summarypb.DashboardTabSummary{
						{
							DashboardName:       "Marco",
							DashboardTabName:    "polo-1",
							Status:              "1/7 tests are passing!",
							OverallStatus:       summarypb.DashboardTabSummary_FLAKY,
							LatestGreen:         "Hulk",
							LastUpdateTimestamp: float64(915166800.916166782),
							LastRunTimestamp:    float64(915166800.916166782),
							FailingTestSummaries: []*summarypb.FailingTestSummary{
								{
									DisplayName:   "top-failure-3",
									FailCount:     3,
									PassTimestamp: float64(914166800.33),
									FailTimestamp: float64(914166852.33),
								},
								{
									DisplayName:   "top-failure-1",
									FailCount:     322,
									PassTimestamp: float64(1677883461.2543),
									FailTimestamp: float64(1677883441),
								},
								{
									DisplayName:   "top-failure-2",
									FailCount:     128,
									PassTimestamp: float64(1677983461.354),
									FailTimestamp: float64(1677983561),
								},
								{
									DisplayName:   "top-failure-4",
									FailCount:     64,
									PassTimestamp: float64(1677983461.354),
									FailTimestamp: float64(1677983561),
								},
								{
									DisplayName:   "top-failure-5",
									FailCount:     32,
									PassTimestamp: float64(1677983461.354),
									FailTimestamp: float64(1677983561),
								},
								{
									DisplayName:   "not-top-failure-1",
									FailCount:     2,
									PassTimestamp: float64(1677983461.354),
									FailTimestamp: float64(1677983561),
								},
							},
						},
						{
							DashboardName:       "Marco",
							DashboardTabName:    "polo-2",
							Status:              "1/7 tests are passing!",
							OverallStatus:       summarypb.DashboardTabSummary_ACCEPTABLE,
							LatestGreen:         "Lantern",
							LastUpdateTimestamp: float64(0.1),
							LastRunTimestamp:    float64(0.1),
							FailingTestSummaries: []*summarypb.FailingTestSummary{
								{
									DisplayName:   "top-failure-1",
									FailCount:     33,
									PassTimestamp: float64(914166800.213),
									FailTimestamp: float64(914176800),
								},
							},
						},
					},
				},
			},
			req: &apipb.ListTabSummariesRequest{
				Dashboard: "marco",
			},
			want: &apipb.ListTabSummariesResponse{
				TabSummaries: []*apipb.TabSummary{
					{
						DashboardName:         "Marco",
						TabName:               "polo-1",
						DetailedStatusMessage: "1/7 tests are passing!",
						OverallStatus:         "FLAKY",
						LatestPassingBuild:    "Hulk",
						LastRunTimestamp: &timestamp.Timestamp{
							Seconds: 915166800,
							Nanos:   916166782,
						},
						LastUpdateTimestamp: &timestamp.Timestamp{
							Seconds: 915166800,
							Nanos:   916166782,
						},
						FailuresSummary: &apipb.FailuresSummary{
							FailureStats: &apipb.FailureStats{
								NumFailingTests: 6,
							},
							TopFailingTests: []*apipb.FailingTestInfo{
								{
									DisplayName: "top-failure-1",
									FailCount:   322,
									PassTimestamp: &timestamp.Timestamp{
										Seconds: int64(1677883461),
										Nanos:   int32(254300117),
									},
									FailTimestamp: &timestamp.Timestamp{
										Seconds: int64(1677883441),
									},
								},
								{
									DisplayName: "top-failure-2",
									FailCount:   128,
									PassTimestamp: &timestamp.Timestamp{
										Seconds: int64(1677983461),
										Nanos:   int32(354000091),
									},
									FailTimestamp: &timestamp.Timestamp{
										Seconds: int64(1677983561),
									},
								},
								{
									DisplayName: "top-failure-4",
									FailCount:   64,
									PassTimestamp: &timestamp.Timestamp{
										Seconds: int64(1677983461),
										Nanos:   int32(354000091),
									},
									FailTimestamp: &timestamp.Timestamp{
										Seconds: int64(1677983561),
									},
								},
								{
									DisplayName: "top-failure-5",
									FailCount:   32,
									PassTimestamp: &timestamp.Timestamp{
										Seconds: int64(1677983461),
										Nanos:   int32(354000091),
									},
									FailTimestamp: &timestamp.Timestamp{
										Seconds: int64(1677983561),
									},
								},
								{
									DisplayName: "top-failure-3",
									FailCount:   3,
									PassTimestamp: &timestamp.Timestamp{
										Seconds: int64(914166800),
										Nanos:   int32(330000042),
									},
									FailTimestamp: &timestamp.Timestamp{
										Seconds: int64(914166852),
										Nanos:   int32(330000042),
									},
								},
							},
						},
					},
					{
						DashboardName:         "Marco",
						TabName:               "polo-2",
						DetailedStatusMessage: "1/7 tests are passing!",
						OverallStatus:         "ACCEPTABLE",
						LatestPassingBuild:    "Lantern",
						LastRunTimestamp: &timestamp.Timestamp{
							Nanos: 100000000,
						},
						LastUpdateTimestamp: &timestamp.Timestamp{
							Nanos: 100000000,
						},
						FailuresSummary: &apipb.FailuresSummary{
							FailureStats: &apipb.FailureStats{
								NumFailingTests: 1,
							},
							TopFailingTests: []*apipb.FailingTestInfo{
								{
									DisplayName: "top-failure-1",
									FailCount:   33,
									PassTimestamp: &timestamp.Timestamp{
										Seconds: int64(914166800),
										Nanos:   int32(213000059),
									},
									FailTimestamp: &timestamp.Timestamp{
										Seconds: int64(914176800),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Returns correct tab summaries for a dashboard, with healthiness info",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name: "Marco",
							DashboardTab: []*configpb.DashboardTab{
								{
									Name:          "polo-1",
									TestGroupName: "cheesecake",
								},
								{
									Name:          "polo-2",
									TestGroupName: "tiramisu",
								},
							},
						},
					},
				},
			},
			summaries: map[string]*summarypb.DashboardSummary{
				"gs://default/summary/summary-marco": {
					TabSummaries: []*summarypb.DashboardTabSummary{
						{
							DashboardName:       "Marco",
							DashboardTabName:    "polo-1",
							Status:              "1/7 tests are passing!",
							OverallStatus:       summarypb.DashboardTabSummary_FLAKY,
							LatestGreen:         "Hulk",
							LastUpdateTimestamp: float64(915166800.916166782),
							LastRunTimestamp:    float64(915166800.916166782),
							Healthiness: &summarypb.HealthinessInfo{
								Start: &timestamp.Timestamp{
									Seconds: int64(915166800),
									Nanos:   int32(916166782),
								},
								End: &timestamp.Timestamp{
									Seconds: int64(916166800),
									Nanos:   int32(916166782),
								},
								AverageFlakiness:  float32(35.0),
								PreviousFlakiness: []float32{44.0},
								Tests: []*summarypb.TestInfo{
									{
										DisplayName:            "top-flaky-1",
										Flakiness:              float32(47.0),
										ChangeFromLastInterval: summarypb.TestInfo_DOWN,
									},
									{
										DisplayName:            "top-flaky-2",
										Flakiness:              float32(67.6),
										ChangeFromLastInterval: summarypb.TestInfo_UP,
									},
									{
										DisplayName:            "top-flaky-3",
										Flakiness:              float32(67.6),
										ChangeFromLastInterval: summarypb.TestInfo_NO_CHANGE,
									},
									{
										DisplayName:            "top-flaky-4",
										Flakiness:              float32(33.3),
										ChangeFromLastInterval: summarypb.TestInfo_DOWN,
									},
									{
										DisplayName:            "top-flaky-5",
										Flakiness:              float32(89.25),
										ChangeFromLastInterval: summarypb.TestInfo_NO_CHANGE,
									},
									{
										DisplayName:            "not-top-flaky-1",
										Flakiness:              float32(15.0),
										ChangeFromLastInterval: summarypb.TestInfo_UP,
									},
									{
										DisplayName:            "not-top-flaky-2",
										Flakiness:              float32(0.0),
										ChangeFromLastInterval: summarypb.TestInfo_UNKNOWN,
									},
								},
							},
						},
						{
							DashboardName:       "Marco",
							DashboardTabName:    "polo-2",
							Status:              "1/7 tests are passing!",
							OverallStatus:       summarypb.DashboardTabSummary_ACCEPTABLE,
							LatestGreen:         "Lantern",
							LastUpdateTimestamp: float64(0.1),
							LastRunTimestamp:    float64(0.1),
							Healthiness: &summarypb.HealthinessInfo{
								Start: &timestamp.Timestamp{
									Seconds: int64(946702801),
								},
								End: &timestamp.Timestamp{
									Seconds: int64(946704801),
								},
								AverageFlakiness: float32(15.2),
								Tests: []*summarypb.TestInfo{
									{
										DisplayName:            "top-flaky-1",
										Flakiness:              float32(75.0),
										ChangeFromLastInterval: summarypb.TestInfo_UP,
									},
									{
										DisplayName:            "not-top-flaky-1",
										Flakiness:              float32(0.0),
										ChangeFromLastInterval: summarypb.TestInfo_UNKNOWN,
									},
								},
							},
						},
					},
				},
			},
			req: &apipb.ListTabSummariesRequest{
				Dashboard: "marco",
			},
			want: &apipb.ListTabSummariesResponse{
				TabSummaries: []*apipb.TabSummary{
					{
						DashboardName:         "Marco",
						TabName:               "polo-1",
						DetailedStatusMessage: "1/7 tests are passing!",
						OverallStatus:         "FLAKY",
						LatestPassingBuild:    "Hulk",
						LastRunTimestamp: &timestamp.Timestamp{
							Seconds: 915166800,
							Nanos:   916166782,
						},
						LastUpdateTimestamp: &timestamp.Timestamp{
							Seconds: 915166800,
							Nanos:   916166782,
						},
						HealthinessSummary: &apipb.HealthinessSummary{
							HealthinessStats: &apipb.HealthinessStats{
								Start: &timestamp.Timestamp{
									Seconds: int64(915166800),
									Nanos:   int32(916166782),
								},
								End: &timestamp.Timestamp{
									Seconds: int64(916166800),
									Nanos:   int32(916166782),
								},
								AverageFlakiness:  float32(35.0),
								PreviousFlakiness: float32(44.0),
								NumFlakyTests:     int32(6),
							},
							TopFlakyTests: []*apipb.FlakyTestInfo{
								{
									DisplayName: "top-flaky-5",
									Flakiness:   float32(89.25),
									Change:      summarypb.TestInfo_NO_CHANGE,
								},
								{
									DisplayName: "top-flaky-2",
									Flakiness:   float32(67.6),
									Change:      summarypb.TestInfo_UP,
								},
								{
									DisplayName: "top-flaky-3",
									Flakiness:   float32(67.6),
									Change:      summarypb.TestInfo_NO_CHANGE,
								},
								{
									DisplayName: "top-flaky-1",
									Flakiness:   float32(47.0),
									Change:      summarypb.TestInfo_DOWN,
								},
								{
									DisplayName: "top-flaky-4",
									Flakiness:   float32(33.3),
									Change:      summarypb.TestInfo_DOWN,
								},
							},
						},
					},
					{
						DashboardName:         "Marco",
						TabName:               "polo-2",
						DetailedStatusMessage: "1/7 tests are passing!",
						OverallStatus:         "ACCEPTABLE",
						LatestPassingBuild:    "Lantern",
						LastRunTimestamp: &timestamp.Timestamp{
							Nanos: 100000000,
						},
						LastUpdateTimestamp: &timestamp.Timestamp{
							Nanos: 100000000,
						},
						HealthinessSummary: &apipb.HealthinessSummary{
							HealthinessStats: &apipb.HealthinessStats{
								Start: &timestamp.Timestamp{
									Seconds: int64(946702801),
								},
								End: &timestamp.Timestamp{
									Seconds: int64(946704801),
								},
								AverageFlakiness:  float32(15.2),
								PreviousFlakiness: float32(-1.0),
								NumFlakyTests:     int32(1),
							},
							TopFlakyTests: []*apipb.FlakyTestInfo{
								{
									DisplayName: "top-flaky-1",
									Flakiness:   float32(75.0),
									Change:      summarypb.TestInfo_UP,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Server error with unreadable config",
			config: map[string]*configpb.Configuration{
				"gs://welp/config": {},
			},
			req: &apipb.ListTabSummariesRequest{
				Dashboard: "doesntmatter",
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := setupTestServer(t, tc.config, nil, tc.summaries)
			got, err := server.ListTabSummaries(context.Background(), tc.req)
			switch {
			case err != nil:
				if !tc.expectError {
					t.Errorf("got unexpected error: %v", err)
				}
			case tc.expectError:
				t.Error("failed to receive an error")
			default:
				if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
					t.Errorf("got unexpected diff (-want +got):\n%s", diff)
				}
			}
		})

	}

}

func GetTabSummary(t *testing.T) {
	tests := []struct {
		name        string
		config      map[string]*configpb.Configuration
		summaries   map[string]*summarypb.DashboardSummary
		req         *apipb.GetTabSummaryRequest
		want        *apipb.GetTabSummaryResponse
		expectError bool
	}{
		{
			name: "Returns an error when there's no dashboard in config",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {},
			},
			req: &apipb.GetTabSummaryRequest{
				Dashboard: "missing",
				Tab:       "Carpe Noctem",
			},
			expectError: true,
		},
		{
			name: "Returns an error when there's no tab in config",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name: "Aurora",
							DashboardTab: []*configpb.DashboardTab{
								{
									Name: "Borealis",
								},
							},
						},
					},
				},
			},
			req: &apipb.GetTabSummaryRequest{
				Dashboard: "Aurora",
				Tab:       "Noctem",
			},
			expectError: true,
		},
		{
			name: "Returns an error when there's no summary for dashboard yet",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name: "ACME",
							DashboardTab: []*configpb.DashboardTab{
								{
									Name:          "me-me",
									TestGroupName: "testgroupname",
								},
							},
						},
					},
				},
			},
			req: &apipb.GetTabSummaryRequest{
				Dashboard: "acme",
				Tab:       "me-me",
			},
			expectError: true,
		},
		{
			name: "Returns an error when there's no summary for a tab",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name: "Marco",
							DashboardTab: []*configpb.DashboardTab{
								{
									Name:          "polo-1",
									TestGroupName: "cheesecake",
								},
								{
									Name:          "polo-2",
									TestGroupName: "tiramisu",
								},
							},
						},
					},
				},
			},
			summaries: map[string]*summarypb.DashboardSummary{
				"gs://default/summary/summary-marco": {
					TabSummaries: []*summarypb.DashboardTabSummary{
						{
							DashboardName:       "Marco",
							DashboardTabName:    "polo-1",
							Status:              "1/7 tests are passing!",
							OverallStatus:       summarypb.DashboardTabSummary_FLAKY,
							LatestGreen:         "Hulk",
							LastUpdateTimestamp: float64(915166800),
							LastRunTimestamp:    float64(915166800),
						},
					},
				},
			},
			req: &apipb.GetTabSummaryRequest{
				Dashboard: "marco",
				Tab:       "polo-2",
			},
			expectError: true,
		},
		{
			name: "Returns correct tab summary for a dashboard-tab",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name: "Marco",
							DashboardTab: []*configpb.DashboardTab{
								{
									Name:          "polo-1",
									TestGroupName: "cheesecake",
								},
								{
									Name:          "polo-2",
									TestGroupName: "tiramisu",
								},
							},
						},
					},
				},
			},
			summaries: map[string]*summarypb.DashboardSummary{
				"gs://default/summary/summary-marco": {
					TabSummaries: []*summarypb.DashboardTabSummary{
						{
							DashboardName:       "Marco",
							DashboardTabName:    "polo-1",
							Status:              "1/7 tests are passing!",
							OverallStatus:       summarypb.DashboardTabSummary_FLAKY,
							LatestGreen:         "Hulk",
							LastUpdateTimestamp: float64(915166800),
							LastRunTimestamp:    float64(915166800),
						},
						{
							DashboardName:       "Marco",
							DashboardTabName:    "polo-2",
							Status:              "1/7 tests are failing!",
							OverallStatus:       summarypb.DashboardTabSummary_ACCEPTABLE,
							LatestGreen:         "Lantern",
							LastUpdateTimestamp: float64(916166800),
							LastRunTimestamp:    float64(916166800),
						},
					},
				},
			},
			req: &apipb.GetTabSummaryRequest{
				Dashboard: "marco",
				Tab:       "polo-1",
			},
			want: &apipb.GetTabSummaryResponse{
				TabSummary: &apipb.TabSummary{
					DashboardName:         "Marco",
					TabName:               "polo-1",
					DetailedStatusMessage: "1/7 tests are passing!",
					OverallStatus:         "FLAKY",
					LatestPassingBuild:    "Hulk",
					LastRunTimestamp: &timestamp.Timestamp{
						Seconds: 915166800,
					},
					LastUpdateTimestamp: &timestamp.Timestamp{
						Seconds: 915166800,
					},
				},
			},
		},
		{
			name: "Server error with unreadable config",
			config: map[string]*configpb.Configuration{
				"gs://welp/config": {},
			},
			req: &apipb.GetTabSummaryRequest{
				Dashboard: "non refert",
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := setupTestServer(t, tc.config, nil, tc.summaries)
			got, err := server.GetTabSummary(context.Background(), tc.req)
			switch {
			case err != nil:
				if !tc.expectError {
					t.Errorf("got unexpected error: %v", err)
				}
			case tc.expectError:
				t.Error("failed to receive an error")
			default:
				if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
					t.Errorf("got unexpected diff (-want +got):\n%s", diff)
				}
			}
		})

	}

}

func TestListTabSummariesHTTP(t *testing.T) {
	tests := []struct {
		name             string
		config           map[string]*configpb.Configuration
		summaries        map[string]*summarypb.DashboardSummary
		endpoint         string
		params           string
		expectedCode     int
		expectedResponse *apipb.ListTabSummariesResponse
	}{
		{
			name: "Returns an error when there's no dashboard in config",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {},
			},
			endpoint:     "/dashboards/whatever/tab-summaries",
			expectedCode: http.StatusNotFound,
		},
		{
			name: "Returns an error when there's no summary for dashboard yet",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name: "ACME",
							DashboardTab: []*configpb.DashboardTab{
								{
									Name:          "me-me",
									TestGroupName: "testgroupname",
								},
							},
						},
					},
				},
			},
			endpoint:     "/dashboards/acme/tab-summaries",
			expectedCode: http.StatusNotFound,
		},
		{
			name: "Returns correct tab summaries for a dashboard",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name: "Marco",
							DashboardTab: []*configpb.DashboardTab{
								{
									Name:          "polo-1",
									TestGroupName: "cheesecake",
								},
								{
									Name:          "polo-2",
									TestGroupName: "tiramisu",
								},
							},
						},
					},
				},
			},
			summaries: map[string]*summarypb.DashboardSummary{
				"gs://default/summary/summary-marco": {
					TabSummaries: []*summarypb.DashboardTabSummary{
						{
							DashboardName:       "Marco",
							DashboardTabName:    "polo-1",
							Status:              "1/7 tests are passing!",
							OverallStatus:       summarypb.DashboardTabSummary_FLAKY,
							LatestGreen:         "Hulk",
							LastUpdateTimestamp: float64(915166800),
							LastRunTimestamp:    float64(915166800),
						},
						{
							DashboardName:       "Marco",
							DashboardTabName:    "polo-2",
							Status:              "1/7 tests are failing!",
							OverallStatus:       summarypb.DashboardTabSummary_ACCEPTABLE,
							LatestGreen:         "Lantern",
							LastUpdateTimestamp: float64(916166800),
							LastRunTimestamp:    float64(916166800),
						},
					},
				},
			},
			endpoint:     "/dashboards/marco/tab-summaries",
			expectedCode: http.StatusOK,
			expectedResponse: &apipb.ListTabSummariesResponse{
				TabSummaries: []*apipb.TabSummary{
					{
						DashboardName:         "Marco",
						TabName:               "polo-1",
						OverallStatus:         "FLAKY",
						DetailedStatusMessage: "1/7 tests are passing!",
						LatestPassingBuild:    "Hulk",
						LastUpdateTimestamp: &timestamp.Timestamp{
							Seconds: 915166800,
						},
						LastRunTimestamp: &timestamp.Timestamp{
							Seconds: 915166800,
						},
					},
					{
						DashboardName:         "Marco",
						TabName:               "polo-2",
						OverallStatus:         "ACCEPTABLE",
						DetailedStatusMessage: "1/7 tests are failing!",
						LatestPassingBuild:    "Lantern",
						LastUpdateTimestamp: &timestamp.Timestamp{
							Seconds: 916166800,
						},
						LastRunTimestamp: &timestamp.Timestamp{
							Seconds: 916166800,
						},
					},
				},
			},
		},
		{
			name: "Returns correct tab summaries for a dashboard with an updated scope",
			config: map[string]*configpb.Configuration{
				"gs://k9s/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name: "Marco",
							DashboardTab: []*configpb.DashboardTab{
								{
									Name:          "polo-1",
									TestGroupName: "cheesecake",
								},
							},
						},
					},
				},
			},
			summaries: map[string]*summarypb.DashboardSummary{
				"gs://k9s/summary/summary-marco": {
					TabSummaries: []*summarypb.DashboardTabSummary{
						{
							DashboardName:       "Marco",
							DashboardTabName:    "polo-1",
							Status:              "1/7 tests are passing!",
							OverallStatus:       summarypb.DashboardTabSummary_FLAKY,
							LatestGreen:         "Hulk",
							LastUpdateTimestamp: float64(915166800),
							LastRunTimestamp:    float64(915166800),
						},
					},
				},
			},
			endpoint:     "/dashboards/marco/tab-summaries?scope=gs://k9s",
			expectedCode: http.StatusOK,
			expectedResponse: &apipb.ListTabSummariesResponse{
				TabSummaries: []*apipb.TabSummary{
					{
						DashboardName:         "Marco",
						TabName:               "polo-1",
						OverallStatus:         "FLAKY",
						DetailedStatusMessage: "1/7 tests are passing!",
						LatestPassingBuild:    "Hulk",
						LastUpdateTimestamp: &timestamp.Timestamp{
							Seconds: 915166800,
						},
						LastRunTimestamp: &timestamp.Timestamp{
							Seconds: 915166800,
						},
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := Route(nil, setupTestServer(t, test.config, nil, test.summaries))
			request, err := http.NewRequest("GET", test.endpoint, nil)
			if err != nil {
				t.Fatalf("Can't form request: %v", err)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != test.expectedCode {
				t.Errorf("Expected %d, but got %d", test.expectedCode, response.Code)
			}

			if response.Code == http.StatusOK {
				var ts apipb.ListTabSummariesResponse
				if err := protojson.Unmarshal(response.Body.Bytes(), &ts); err != nil {
					t.Fatalf("Failed to unmarshal json message into a proto message: %v", err)
				}
				if diff := cmp.Diff(test.expectedResponse, &ts, protocmp.Transform()); diff != "" {
					t.Errorf("Obtained unexpected  diff (-want +got):\n%s", diff)
				}
			}
		})
	}
}
