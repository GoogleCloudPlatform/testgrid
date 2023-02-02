/*
Copyright 2021 The TestGrid Authors.

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
	"testing"

	apipb "github.com/GoogleCloudPlatform/testgrid/pb/api/v1"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	summarypb "github.com/GoogleCloudPlatform/testgrid/pb/summary"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/google/go-cmp/cmp"
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
			name: "Returns empty tab summaries list for non-existing summary",
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
			summaries: map[string]*summarypb.DashboardSummary{
				"gs://default/summary/summary-acme": {},
			},
			req: &apipb.ListTabSummariesRequest{
				Dashboard: "acme",
			},
			want: &apipb.ListTabSummariesResponse{},
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
						LatestGreen:           "Hulk",
						LastRunTimestamp: &timestamp.Timestamp{
							Seconds: 915166800,
						},
						LastUpdateTimestamp: &timestamp.Timestamp{
							Seconds: 915166800,
						},
					},
					{
						DashboardName:         "Marco",
						TabName:               "polo-2",
						DetailedStatusMessage: "1/7 tests are failing!",
						OverallStatus:         "ACCEPTABLE",
						LatestGreen:           "Lantern",
						LastRunTimestamp: &timestamp.Timestamp{
							Seconds: 916166800,
						},
						LastUpdateTimestamp: &timestamp.Timestamp{
							Seconds: 916166800,
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