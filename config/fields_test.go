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

package config

import (
	"testing"

	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/google/go-cmp/cmp"
)

func TestFields(t *testing.T) {
	cases := []struct {
		name     string
		config   *configpb.Configuration
		expected map[string]int64
	}{
		{
			name:     "Nil config",
			expected: map[string]int64{},
		},
		{
			name:     "Empty config",
			config:   &configpb.Configuration{},
			expected: map[string]int64{},
		},
		{
			name: "Simple config",
			config: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dash_1",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name:          "tab_1",
								TestGroupName: "test_group_1",
							},
						},
					},
				},
				TestGroups: []*configpb.TestGroup{
					{
						Name:             "test_group_1",
						GcsPrefix:        "tests_live_here",
						DaysOfResults:    1,
						NumColumnsRecent: 1,
					},
				},
			},
			expected: map[string]int64{
				"dashboards.name":                          1,
				"dashboards.dashboard_tab.name":            1,
				"dashboards.dashboard_tab.test_group_name": 1,
				"test_groups.name":                         1,
				"test_groups.gcs_prefix":                   1,
				"test_groups.days_of_results":              1,
				"test_groups.num_columns_recent":           1,
			},
		},
		{
			name: "Multiple configs",
			config: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dash_1",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name:          "tab_1",
								TestGroupName: "test_group_1",
							},
						},
					},
					{
						Name: "dash_2",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name:          "tab_2",
								TestGroupName: "test_group_2",
							},
							{
								Name:         "tab_3",
								BugComponent: 1,
							},
						},
						HighlightToday: true,
					},
				},
			},
			expected: map[string]int64{
				"dashboards.name":                          2,
				"dashboards.dashboard_tab.name":            3,
				"dashboards.dashboard_tab.test_group_name": 2,
				"dashboards.dashboard_tab.bug_component":   1,
				"dashboards.highlight_today":               1,
			},
		},
		{
			name: "Ignore default values",
			config: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name:           "",
						HighlightToday: false,
						DashboardTab: []*configpb.DashboardTab{
							{
								Name:         "DashTab",
								BugComponent: 0,
							},
						},
					},
				},
			},
			expected: map[string]int64{
				"dashboards.dashboard_tab.name": 1,
			},
		},
		{
			name: "Nested messages and one-ofs",
			config: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						DashboardTab: []*configpb.DashboardTab{
							{
								Name: "t",
								BetaAutobugOptions: &configpb.AutoBugOptions{
									BetaAutobugComponent: 1,
									HotlistIdsFromSource: []*configpb.HotlistIdFromSource{
										{
											HotlistIdSource: &configpb.HotlistIdFromSource_Label{Label: "foo"},
										},
										{
											HotlistIdSource: &configpb.HotlistIdFromSource_Value{Value: 1},
										},
									},
								},
							},
						},
					},
				},
				TestGroups: []*configpb.TestGroup{
					{
						AutoBugOptions: &configpb.AutoBugOptions{
							BetaAutobugComponent: 1,
							HotlistIdsFromSource: []*configpb.HotlistIdFromSource{
								{
									HotlistIdSource: &configpb.HotlistIdFromSource_Value{Value: 2},
								},
								{
									HotlistIdSource: &configpb.HotlistIdFromSource_Value{Value: 3},
								},
							},
						},
					},
				},
			},
			expected: map[string]int64{
				"dashboards.dashboard_tab.name":                                               1,
				"dashboards.dashboard_tab.beta_autobug_options.beta_autobug_component":        1,
				"dashboards.dashboard_tab.beta_autobug_options.hotlist_ids_from_source.label": 1,
				"dashboards.dashboard_tab.beta_autobug_options.hotlist_ids_from_source.value": 1,
				"test_groups.auto_bug_options.beta_autobug_component":                         1,
				"test_groups.auto_bug_options.hotlist_ids_from_source.value":                  2,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := Fields(tc.config)
			if diff := cmp.Diff(tc.expected, result); diff != "" {
				t.Errorf("Incorrect fields (-want +got): %s", diff)
			}
		})
	}
}
