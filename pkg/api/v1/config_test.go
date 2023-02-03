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
