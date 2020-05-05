/*
Copyright 2020 The Kubernetes Authors.

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

package alerter

import (
	"context"
	"testing"

	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	summarypb "github.com/GoogleCloudPlatform/testgrid/pb/summary"
)

func TestCallOutDifferences(t *testing.T) {
	statusOnly := func(status summarypb.DashboardTabSummary_TabStatus) summarypb.DashboardSummary {
		return summarypb.DashboardSummary{
			TabSummaries: []*summarypb.DashboardTabSummary{
				{
					DashboardName:    "dash",
					DashboardTabName: "tab",
					OverallStatus:    status,
				},
			},
		}
	}

	tests := []struct {
		name                      string
		newSummary                summarypb.DashboardSummary
		oldSummary                summarypb.DashboardSummary
		config                    configpb.Dashboard
		expectOnConsistentFailure int
		expectOnBecomingStale     int
	}{
		{
			name:       "No DashboardTabs; No Alert",
			oldSummary: summarypb.DashboardSummary{},
			newSummary: summarypb.DashboardSummary{},
			config:     configpb.Dashboard{},
		},
		{
			name:       "Passing Dashboards; No Alert",
			oldSummary: statusOnly(summarypb.DashboardTabSummary_PASS),
			newSummary: statusOnly(summarypb.DashboardTabSummary_PASS),
			config: configpb.Dashboard{
				DashboardTab: []*configpb.DashboardTab{
					{
						Name: "tab",
						AlertOptions: &configpb.DashboardTabAlertOptions{
							NumFailuresToAlert: 1,
						},
					},
				},
				Name: "dash",
			},
		},
		{
			name:       "Pass to Fail Dashboards; Calls Failure",
			oldSummary: statusOnly(summarypb.DashboardTabSummary_PASS),
			newSummary: summarypb.DashboardSummary{
				TabSummaries: []*summarypb.DashboardTabSummary{
					{
						DashboardName:    "dash",
						DashboardTabName: "tab",
						OverallStatus:    summarypb.DashboardTabSummary_FAIL,
						FailingTestSummaries: []*summarypb.FailingTestSummary{
							{
								TestName:  "badtest",
								FailCount: 1,
							},
						},
					},
				},
			},
			config: configpb.Dashboard{
				DashboardTab: []*configpb.DashboardTab{
					{
						Name: "tab",
						AlertOptions: &configpb.DashboardTabAlertOptions{
							NumFailuresToAlert: 1,
						},
					},
				},
				Name: "dash",
			},
			expectOnConsistentFailure: 1,
		},
		{
			name:       "Pass to Fail Dashboards with high failures_to_alert; No Alert",
			oldSummary: statusOnly(summarypb.DashboardTabSummary_PASS),
			newSummary: summarypb.DashboardSummary{
				TabSummaries: []*summarypb.DashboardTabSummary{
					{
						DashboardName:    "dash",
						DashboardTabName: "tab",
						OverallStatus:    summarypb.DashboardTabSummary_FAIL,
						FailingTestSummaries: []*summarypb.FailingTestSummary{
							{
								TestName:  "badtest",
								FailCount: 1,
							},
						},
					},
				},
			},
			config: configpb.Dashboard{
				DashboardTab: []*configpb.DashboardTab{
					{
						Name: "tab",
						AlertOptions: &configpb.DashboardTabAlertOptions{
							NumFailuresToAlert: 100,
						},
					},
				},
				Name: "dash",
			},
		},
		{
			name: "Fail to Fail Dashboards; no alert",
			oldSummary: summarypb.DashboardSummary{
				TabSummaries: []*summarypb.DashboardTabSummary{
					{
						DashboardName:    "dash",
						DashboardTabName: "tab",
						OverallStatus:    summarypb.DashboardTabSummary_FAIL,
						FailingTestSummaries: []*summarypb.FailingTestSummary{
							{
								TestName:  "badtest",
								FailCount: 1,
							},
						},
					},
				},
			},
			newSummary: summarypb.DashboardSummary{
				TabSummaries: []*summarypb.DashboardTabSummary{
					{
						DashboardName:    "dash",
						DashboardTabName: "tab",
						OverallStatus:    summarypb.DashboardTabSummary_FAIL,
						FailingTestSummaries: []*summarypb.FailingTestSummary{
							{
								TestName:  "badtest",
								FailCount: 2,
							},
						},
					},
				},
			},
			config: configpb.Dashboard{
				DashboardTab: []*configpb.DashboardTab{
					{
						Name: "tab",
						AlertOptions: &configpb.DashboardTabAlertOptions{
							NumFailuresToAlert: 1,
						},
					},
				},
				Name: "dash",
			},
		},
		{
			name:       "Pass to Stale Dashboards; calls stale",
			oldSummary: statusOnly(summarypb.DashboardTabSummary_PASS),
			newSummary: statusOnly(summarypb.DashboardTabSummary_STALE),
			config: configpb.Dashboard{
				DashboardTab: []*configpb.DashboardTab{
					{
						Name: "tab",
						AlertOptions: &configpb.DashboardTabAlertOptions{
							NumFailuresToAlert: 1,
						},
					},
				},
				Name: "dash",
			},
			expectOnBecomingStale: 1,
		},
		{
			name:       "Stale to Stale Dashbord; no alert",
			oldSummary: statusOnly(summarypb.DashboardTabSummary_STALE),
			newSummary: statusOnly(summarypb.DashboardTabSummary_STALE),
			config: configpb.Dashboard{
				DashboardTab: []*configpb.DashboardTab{
					{
						Name: "tab",
						AlertOptions: &configpb.DashboardTabAlertOptions{
							NumFailuresToAlert: 1,
						},
					},
				},
				Name: "dash",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var k TestKlaxon
			callOutDifferences(context.Background(), &test.newSummary, &test.oldSummary, &test.config, k.OnConsistentFailure, k.OnBecomingStale)

			if k.counts.OnBecomingStale != test.expectOnBecomingStale {
				t.Errorf("Number of OnBecomingStale calls was unexpected; got %d, expected %d", k.counts.OnBecomingStale, test.expectOnBecomingStale)
			}

			if k.counts.OnConsistentFailure != test.expectOnConsistentFailure {
				t.Errorf("Number of OnConsistentFailure calls was unexpected; got %d, expected %d", k.counts.OnConsistentFailure, test.expectOnConsistentFailure)
			}
		})
	}
}

type TestKlaxon struct {
	counts struct {
		OnConsistentFailure int
		OnBecomingStale     int
	}
}

func (t *TestKlaxon) OnConsistentFailure(ctx context.Context, new, old *summarypb.DashboardTabSummary, config *configpb.DashboardTab) error {
	t.counts.OnConsistentFailure++
	return nil
}

func (t *TestKlaxon) OnBecomingStale(ctx context.Context, new, old *summarypb.DashboardTabSummary, config *configpb.DashboardTab) error {
	t.counts.OnBecomingStale++
	return nil
}
