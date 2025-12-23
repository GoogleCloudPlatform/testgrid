/*
Copyright 2022 The TestGrid Authors.

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
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
)

func getPathOrDie(t *testing.T, s string) *gcs.Path {
	t.Helper()
	path, err := gcs.NewPath(s)
	if err != nil {
		t.Fatalf("Couldn't make path %s: %v", s, err)
	}
	return path
}

func Test_GetDashboardGroup(t *testing.T) {
	tests := []struct {
		name        string
		config      map[string]*configpb.Configuration
		req         *apipb.GetDashboardGroupRequest
		expected    *apipb.GetDashboardGroupResponse
		expectError bool
	}{
		{
			name: "Returns an error when there's no resource",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {},
			},
			req: &apipb.GetDashboardGroupRequest{
				DashboardGroup: "",
			},
			expectError: true,
		},
		{
			name: "Returns empty response from an empty Dashboard Group",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					DashboardGroups: []*configpb.DashboardGroup{
						{
							Name: "Group1",
						},
					},
				},
			},
			req: &apipb.GetDashboardGroupRequest{
				DashboardGroup: "group1",
			},
			expected: &apipb.GetDashboardGroupResponse{},
		},
		{
			name: "Returns dashboards from group",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					DashboardGroups: []*configpb.DashboardGroup{
						{
							Name:           "stooges",
							DashboardNames: []string{"Larry", "Curly", "Moe"},
						},
					},
				},
			},
			req: &apipb.GetDashboardGroupRequest{
				DashboardGroup: "stooges",
			},
			expected: &apipb.GetDashboardGroupResponse{
				Dashboards: []*apipb.Resource{
					{Name: "Curly", Link: "/dashboards/curly"},
					{Name: "Larry", Link: "/dashboards/larry"},
					{Name: "Moe", Link: "/dashboards/moe"},
				},
			},
		},
		{
			name: "Reads specified configs",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					DashboardGroups: []*configpb.DashboardGroup{
						{
							Name:           "wrong-group",
							DashboardNames: []string{"no"},
						},
					},
				},
				"gs://example/config": {
					DashboardGroups: []*configpb.DashboardGroup{
						{
							Name:           "right-group",
							DashboardNames: []string{"yes"},
						},
					},
				},
			},
			req: &apipb.GetDashboardGroupRequest{
				DashboardGroup: "right-group",
				Scope:          "gs://example",
			},
			expected: &apipb.GetDashboardGroupResponse{
				Dashboards: []*apipb.Resource{
					{Name: "yes", Link: "/dashboards/yes?scope=gs://example"},
				},
			},
		},
		{
			name: "Specified configs never reads default config",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					DashboardGroups: []*configpb.DashboardGroup{
						{
							Name:           "wrong-group",
							DashboardNames: []string{"no"},
						},
					},
				},
				"gs://example/config": {
					DashboardGroups: []*configpb.DashboardGroup{
						{
							Name:           "right-group",
							DashboardNames: []string{"yes"},
						},
					},
				},
			},
			req: &apipb.GetDashboardGroupRequest{
				DashboardGroup: "wrong-group",
				Scope:          "gs://example",
			},
			expectError: true,
		},
		{
			name: "Server error with unreadable config",
			req: &apipb.GetDashboardGroupRequest{
				DashboardGroup: "group",
				Scope:          "garbage",
			},
			expectError: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := setupTestServer(t, tc.config, nil, nil)
			actual, err := server.GetDashboardGroup(context.Background(), tc.req)
			if !tc.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if tc.expectError && err == nil {
				t.Errorf("Expected error, but got none")
			}
			if diff := cmp.Diff(actual, tc.expected, protocmp.Transform()); diff != "" {
				t.Errorf("(-got, +want): %s", diff)
			}
		})
	}
}

func Test_GetDashboard(t *testing.T) {
	tests := []struct {
		name        string
		config      map[string]*configpb.Configuration
		req         *apipb.GetDashboardRequest
		expected    *apipb.GetDashboardResponse
		expectError bool
	}{
		{
			name: "Returns an error when there's no resource",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {},
			},
			req: &apipb.GetDashboardRequest{
				Dashboard: "",
			},
			expectError: true,
		},
		{
			name: "Returns empty JSON from an empty Dashboard",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name: "Dashboard1",
						},
					},
				},
			},
			req: &apipb.GetDashboardRequest{
				Dashboard: "Dashboard1",
			},
			expected: &apipb.GetDashboardResponse{},
		},
		{
			name: "Returns dashboard info from dashboard",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name:                "Dashboard1",
							DefaultTab:          "defaultTab",
							HighlightToday:      true,
							DownplayFailingTabs: true,
							Notifications: []*configpb.Notification{
								{
									Summary:     "Notification summary",
									ContextLink: "Notification context link",
								},
							},
						},
					},
				},
			},
			req: &apipb.GetDashboardRequest{
				Dashboard: "Dashboard-1",
			},
			expected: &apipb.GetDashboardResponse{
				Notifications: []*configpb.Notification{
					{Summary: "Notification summary", ContextLink: "Notification context link"},
				},
				DefaultTab:          "defaultTab",
				HighlightToday:      true,
				SuppressFailingTabs: true,
			},
		},
		{
			name: "Reads specified configs",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name:       "wrong-dashboard",
							DefaultTab: "wrong-dashboard defaultTab",
						},
					},
				},
				"gs://example/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name:           "correct-dashboard",
							DefaultTab:     "correct-dashboard defaultTab",
							HighlightToday: true,
						},
					},
				},
			},
			req: &apipb.GetDashboardRequest{
				Dashboard: "correct-dashboard",
				Scope:     "gs://example",
			},
			expected: &apipb.GetDashboardResponse{
				HighlightToday: true,
				DefaultTab:     "correct-dashboard defaultTab",
			},
		},
		{
			name: "Specified configs never reads default config",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name:       "wrong-dashboard",
							DefaultTab: "wrong-dashboard defaultTab",
						},
					},
				},
				"gs://example/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name:       "correct-dashboard",
							DefaultTab: "correct-dashboard defaultTab",
						},
					},
				},
			},
			req: &apipb.GetDashboardRequest{
				Dashboard: "wrong-dashboard",
				Scope:     "gs://example",
			},
			expectError: true,
		},
		{
			name: "Server error with unreadable config",
			req: &apipb.GetDashboardRequest{
				Dashboard: "who",
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := setupTestServer(t, tc.config, nil, nil)
			actual, err := server.GetDashboard(context.Background(), tc.req)
			if !tc.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if tc.expectError && err == nil {
				t.Errorf("Expected error, but got none")
			}
			if diff := cmp.Diff(actual, tc.expected, protocmp.Transform()); diff != "" {
				t.Errorf("(-got, +want): %s", diff)
			}
		})
	}
}

func Test_GetDashboardTab(t *testing.T) {
	tests := []struct {
		name     string
		config   map[string]*configpb.Configuration
		req      *apipb.GetDashboardTabRequest
		expected *apipb.GetDashboardTabResponse
	}{
		{
			name: "Returns from an empty tab",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name: "Dashboard1",
							DashboardTab: []*configpb.DashboardTab{
								{
									Name:          "Tab1",
									TestGroupName: "TestGroup1",
								},
							},
						},
					},
					TestGroups: []*configpb.TestGroup{
						{Name: "TestGroup1"},
					},
				},
			},
			req: &apipb.GetDashboardTabRequest{
				Dashboard: "Dashboard1",
				Tab:       "Tab1",
			},
			expected: &apipb.GetDashboardTabResponse{
				Name:          "Tab1",
				TestGroupName: "TestGroup1",
				ResultSource: &apipb.ResultSource{
					ResultSourceConfig: &apipb.ResultSource_GcsConfig{},
				},
				AlertOptions: &apipb.AlertOptions{},
			},
		},
		{
			name: "Returns dashboard tab info from dashboard tab",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name:       "Dashboard1",
							DefaultTab: "defaultTab",
							DashboardTab: []*configpb.DashboardTab{
								{
									Name:              "Tab1",
									TestGroupName:     "TestGroup1",
									BaseOptions:       "width=20",
									Description:       "A tab.",
									AboutDashboardUrl: "overall-tab-link",
									ResultsText:       "See results, somewhere",
									ResultsUrlTemplate: &configpb.LinkTemplate{
										Url: "results-link",
									},
									CodeSearchUrlTemplate: &configpb.LinkTemplate{
										Url: "code-search-link",
									},
									OpenTestTemplate: &configpb.LinkTemplate{
										Url: "open-test-link",
									},
									FileBugTemplate: &configpb.LinkTemplate{
										Url: "file-bug-link",
									},
									OpenBugTemplate: &configpb.LinkTemplate{
										Url: "open-bug-link",
									},
									AttachBugTemplate: &configpb.LinkTemplate{
										Url: "attach-bug-link",
									},
									ColumnDiffLinkTemplates: []*configpb.LinkTemplate{
										{
											Name: "Foo Diff",
											Url:  "diff-link/foo",
										},
										{
											Name: "Bar Diff",
											Url:  "diff-link/bar",
										},
									},
									AlertOptions: &configpb.DashboardTabAlertOptions{},
								},
							},
						},
					},
					TestGroups: []*configpb.TestGroup{
						{
							Name: "TestGroup1",
							ResultSource: &configpb.TestGroup_ResultSource{
								ResultSourceConfig: &configpb.TestGroup_ResultSource_GcsConfig{
									GcsConfig: &configpb.GCSConfig{
										GcsPrefix:          "my-bucket/dir",
										PubsubProject:      "my-project",
										PubsubSubscription: "my-subscription",
									},
								},
							}},
					},
				},
			},
			req: &apipb.GetDashboardTabRequest{
				Dashboard: "Dashboard1",
				Tab:       "Tab1",
			},
			expected: &apipb.GetDashboardTabResponse{
				Name:          "Tab1",
				TestGroupName: "TestGroup1",
				Description:   "A tab.",
				ResultSource: &apipb.ResultSource{
					ResultSourceConfig: &apipb.ResultSource_GcsConfig{
						GcsConfig: &apipb.GCSConfig{
							GcsPrefix:          "my-bucket/dir",
							PubsubProject:      "my-project",
							PubsubSubscription: "my-subscription",
						},
					},
				},
				TabMenuTemplates: []*apipb.LinkTemplate{
					{
						Text: "See results, somewhere",
						Url:  "results-link",
					},
					{
						Text: "About this tab",
						Url:  "overall-tab-link",
					},
				},
				ColumnDiffTemplates: []*apipb.LinkTemplate{
					{
						Text: "Search for Changes",
						Url:  "code-search-link",
					},
					{
						Text: "Foo Diff",
						Url:  "diff-link/foo",
					},
					{
						Text: "Bar Diff",
						Url:  "diff-link/bar",
					},
				},
				CellMenuTemplates: []*apipb.LinkTemplate{
					{
						Text: "Open Test",
						Url:  "open-test-link",
					},
					{
						Text: "File a Bug",
						Url:  "file-bug-link",
					},
					{
						Text: "Attach to Bug",
						Url:  "attach-bug-link",
					},
				},
				AlertOptions:      &apipb.AlertOptions{},
				BaseOptions:       "width=20",
				DisplayLocalTime:  false,
				TabularNamesRegex: "",
			},
		},
		{
			name: "Returns dashboard tab info from dashboard tab with GCS prefix",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name:       "Dashboard1",
							DefaultTab: "defaultTab",
							DashboardTab: []*configpb.DashboardTab{
								{
									Name:          "Tab1",
									TestGroupName: "TestGroup1",
									BaseOptions:   "width=20",
									Description:   "A tab.",
								},
							},
						},
					},
					TestGroups: []*configpb.TestGroup{
						{
							Name:      "TestGroup1",
							GcsPrefix: "my-bucket/dir",
						},
					},
				},
			},
			req: &apipb.GetDashboardTabRequest{
				Dashboard: "Dashboard1",
				Tab:       "Tab1",
			},
			expected: &apipb.GetDashboardTabResponse{
				Name:          "Tab1",
				TestGroupName: "TestGroup1",
				Description:   "A tab.",
				ResultSource: &apipb.ResultSource{
					ResultSourceConfig: &apipb.ResultSource_GcsConfig{
						GcsConfig: &apipb.GCSConfig{
							GcsPrefix: "my-bucket/dir",
						},
					},
				},
				AlertOptions:     &apipb.AlertOptions{},
				BaseOptions:      "width=20",
				DisplayLocalTime: false,
			},
		},
		{
			name: "Reads specified configs",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name:       "wrong-dashboard",
							DefaultTab: "wrong-dashboard defaultTab",
							DashboardTab: []*configpb.DashboardTab{
								{
									Name:          "wrong-tab",
									TestGroupName: "wrong-group",
								},
							},
						},
					},
					TestGroups: []*configpb.TestGroup{
						{Name: "wrong-group"},
					},
				},
				"gs://example/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name:           "correct-dashboard",
							DefaultTab:     "correct-dashboard defaultTab",
							HighlightToday: true,
							DashboardTab: []*configpb.DashboardTab{
								{
									Name:          "correct-tab",
									TestGroupName: "correct-group",
								},
							},
						},
					},
					TestGroups: []*configpb.TestGroup{
						{Name: "correct-group"},
					},
				},
			},
			req: &apipb.GetDashboardTabRequest{
				Dashboard: "correct-dashboard",
				Tab:       "correct-tab",
				Scope:     "gs://example",
			},
			expected: &apipb.GetDashboardTabResponse{
				Name:          "correct-tab",
				TestGroupName: "correct-group",
				ResultSource: &apipb.ResultSource{
					ResultSourceConfig: &apipb.ResultSource_GcsConfig{},
				},
				AlertOptions:      &apipb.AlertOptions{},
				BaseOptions:       "",
				DisplayLocalTime:  false,
				TabularNamesRegex: "",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := setupTestServer(t, tc.config, nil, nil)
			actual, err := server.GetDashboardTab(context.Background(), tc.req)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if diff := cmp.Diff(actual, tc.expected, protocmp.Transform()); diff != "" {
				t.Errorf("(-got, +want): %s", diff)
			}
		})
	}
}

func Test_GetDashboardTab_Error(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]*configpb.Configuration
		req    *apipb.GetDashboardTabRequest
	}{
		{
			name: "Returns an error when missing config",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {},
			},
			req: &apipb.GetDashboardTabRequest{
				Dashboard: "Dashboard1",
				Tab:       "Tab1",
			},
		},
		{
			name: "Returns an error when missing test group",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name: "Dashboard1",
						},
					},
					TestGroups: []*configpb.TestGroup{},
				},
			},
			req: &apipb.GetDashboardTabRequest{
				Dashboard: "Dashboard1",
				Tab:       "Tab1",
			},
		},
		{
			name: "Returns an error when missing tab",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name:         "Dashboard1",
							DashboardTab: []*configpb.DashboardTab{},
						},
					},
					TestGroups: []*configpb.TestGroup{
						{Name: "TestGroup1"},
					},
				},
			},
			req: &apipb.GetDashboardTabRequest{
				Dashboard: "Dashboard1",
				Tab:       "Tab1",
			},
		},
		{
			name: "Server error with unreadable config",
			req: &apipb.GetDashboardTabRequest{
				Dashboard: "who",
				Tab:       "what",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := setupTestServer(t, tc.config, nil, nil)
			actual, err := server.GetDashboardTab(context.Background(), tc.req)
			if err == nil {
				t.Errorf("Expected error, but got response instead: %v", actual)
			}
		})
	}
}

func Test_ListDashboardTabs(t *testing.T) {
	tests := []struct {
		name        string
		config      map[string]*configpb.Configuration
		req         *apipb.ListDashboardTabsRequest
		expected    *apipb.ListDashboardTabsResponse
		expectError bool
	}{
		{
			name: "Returns an error when there's no resource",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {},
			},
			req: &apipb.ListDashboardTabsRequest{
				Dashboard: "",
			},
			expectError: true,
		},
		{
			name: "Returns empty JSON from an empty Dashboard",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name:         "Dashboard1",
							DashboardTab: []*configpb.DashboardTab{},
						},
					},
				},
			},
			req: &apipb.ListDashboardTabsRequest{
				Dashboard: "Dashboard1",
			},
			expected: &apipb.ListDashboardTabsResponse{},
		},
		{
			name: "Returns tabs list from a Dashboard",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name: "Dashboard1",
							DashboardTab: []*configpb.DashboardTab{
								{
									Name: "tab 1",
								},
								{
									Name: "tab 2",
								},
							},
						},
					},
				},
			},
			req: &apipb.ListDashboardTabsRequest{
				Dashboard: "Dashboard1",
			},
			expected: &apipb.ListDashboardTabsResponse{
				DashboardTabs: []*apipb.Resource{
					{Name: "tab 1", Link: "/dashboards/dashboard1/tabs/tab1"},
					{Name: "tab 2", Link: "/dashboards/dashboard1/tabs/tab2"},
				},
			},
		},
		{
			name: "Reads specified configs",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name: "wrong-dashboard",
							DashboardTab: []*configpb.DashboardTab{
								{
									Name: "wrong-dashboard tab 1",
								},
							},
						},
					},
				},
				"gs://example/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name: "correct-dashboard",
							DashboardTab: []*configpb.DashboardTab{
								{
									Name: "correct-dashboard tab 1",
								},
							},
						},
					},
				},
			},
			req: &apipb.ListDashboardTabsRequest{
				Dashboard: "correct-dashboard",
				Scope:     "gs://example",
			},
			expected: &apipb.ListDashboardTabsResponse{
				DashboardTabs: []*apipb.Resource{
					{Name: "correct-dashboard tab 1", Link: "/dashboards/correctdashboard/tabs/correctdashboardtab1?scope=gs://example"},
				},
			},
		},
		{
			name: "Specified configs never reads default config",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name: "wrong-dashboard",
							DashboardTab: []*configpb.DashboardTab{
								{
									Name: "wrong-dashboard tab 1",
								},
							},
						},
					},
				},
				"gs://example/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name: "correct-dashboard",
							DashboardTab: []*configpb.DashboardTab{
								{
									Name: "correct-dashboard tab 1",
								},
							},
						},
					},
				},
			},
			req: &apipb.ListDashboardTabsRequest{
				Dashboard: "wrong-dashboard",
				Scope:     "gs://example",
			},
			expectError: true,
		},
		{
			name: "Server error with unreadable config",
			req: &apipb.ListDashboardTabsRequest{
				Dashboard: "dashboard1",
				Scope:     "gibberish",
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := setupTestServer(t, tc.config, nil, nil)
			actual, err := server.ListDashboardTabs(context.Background(), tc.req)
			if !tc.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if tc.expectError && err == nil {
				t.Errorf("Expected error, but got none")
			}
			if diff := cmp.Diff(actual, tc.expected, protocmp.Transform()); diff != "" {
				t.Errorf("(-got, +want): %s", diff)
			}
		})
	}
}

// These methods are currently covered in HTTP tests only:
// ListDashboardGroup
// ListDashboard
