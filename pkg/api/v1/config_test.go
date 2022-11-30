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

	v1 "github.com/GoogleCloudPlatform/testgrid/pb/api/v1"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	pb "github.com/GoogleCloudPlatform/testgrid/pb/config"
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
		req         *v1.GetDashboardGroupRequest
		expected    *v1.GetDashboardGroupResponse
		expectError bool
	}{
		{
			name: "Returns an error when there's no resource",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {},
			},
			req: &v1.GetDashboardGroupRequest{
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
			req: &v1.GetDashboardGroupRequest{
				DashboardGroup: "group1",
			},
			expected: &v1.GetDashboardGroupResponse{},
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
			req: &v1.GetDashboardGroupRequest{
				DashboardGroup: "stooges",
			},
			expected: &v1.GetDashboardGroupResponse{
				Dashboards: []*v1.Resource{
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
			req: &v1.GetDashboardGroupRequest{
				DashboardGroup: "right-group",
				Scope:          "gs://example",
			},
			expected: &v1.GetDashboardGroupResponse{
				Dashboards: []*v1.Resource{
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
			req: &v1.GetDashboardGroupRequest{
				DashboardGroup: "wrong-group",
				Scope:          "gs://example",
			},
			expectError: true,
		},
		{
			name: "Server error with unreadable config",
			req: &v1.GetDashboardGroupRequest{
				DashboardGroup: "group",
				Scope:          "garbage",
			},
			expectError: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := setupTestServer(t, tc.config, nil)
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
		req         *v1.GetDashboardRequest
		expected    *v1.GetDashboardResponse
		expectError bool
	}{
		{
			name: "Returns an error when there's no resource",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {},
			},
			req: &v1.GetDashboardRequest{
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
			req: &v1.GetDashboardRequest{
				Dashboard: "Dashboard1",
			},
			expected: &v1.GetDashboardResponse{},
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
			req: &v1.GetDashboardRequest{
				Dashboard: "Dashboard-1",
			},
			expected: &v1.GetDashboardResponse{
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
			config: map[string]*pb.Configuration{
				"gs://default/config": {
					Dashboards: []*pb.Dashboard{
						{
							Name:       "wrong-dashboard",
							DefaultTab: "wrong-dashboard defaultTab",
						},
					},
				},
				"gs://example/config": {
					Dashboards: []*pb.Dashboard{
						{
							Name:           "correct-dashboard",
							DefaultTab:     "correct-dashboard defaultTab",
							HighlightToday: true,
						},
					},
				},
			},
			req: &v1.GetDashboardRequest{
				Dashboard: "correct-dashboard",
				Scope:     "gs://example",
			},
			expected: &v1.GetDashboardResponse{
				HighlightToday: true,
				DefaultTab:     "correct-dashboard defaultTab",
			},
		},
		{
			name: "Specified configs never reads default config",
			config: map[string]*pb.Configuration{
				"gs://default/config": {
					Dashboards: []*pb.Dashboard{
						{
							Name:       "wrong-dashboard",
							DefaultTab: "wrong-dashboard defaultTab",
						},
					},
				},
				"gs://example/config": {
					Dashboards: []*pb.Dashboard{
						{
							Name:       "correct-dashboard",
							DefaultTab: "correct-dashboard defaultTab",
						},
					},
				},
			},
			req: &v1.GetDashboardRequest{
				Dashboard: "wrong-dashboard",
				Scope:     "gs://example",
			},
			expectError: true,
		},
		{
			name: "Server error with unreadable config",
			req: &v1.GetDashboardRequest{
				Dashboard: "who",
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := setupTestServer(t, tc.config, nil)
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
		req         *v1.ListDashboardTabsRequest
		expected    *v1.ListDashboardTabsResponse
		expectError bool
	}{
		{
			name: "Returns an error when there's no resource",
			config: map[string]*pb.Configuration{
				"gs://default/config": {},
			},
			req: &v1.ListDashboardTabsRequest{
				Dashboard: "",
			},
			expectError: true,
		},
		{
			name: "Returns empty JSON from an empty Dashboard",
			config: map[string]*pb.Configuration{
				"gs://default/config": {
					Dashboards: []*pb.Dashboard{
						{
							Name:         "Dashboard1",
							DashboardTab: []*pb.DashboardTab{},
						},
					},
				},
			},
			req: &v1.ListDashboardTabsRequest{
				Dashboard: "Dashboard1",
			},
			expected: &v1.ListDashboardTabsResponse{},
		},
		{
			name: "Returns tabs list from a Dashboard",
			config: map[string]*pb.Configuration{
				"gs://default/config": {
					Dashboards: []*pb.Dashboard{
						{
							Name: "Dashboard1",
							DashboardTab: []*pb.DashboardTab{
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
			req: &v1.ListDashboardTabsRequest{
				Dashboard: "Dashboard1",
			},
			expected: &v1.ListDashboardTabsResponse{
				DashboardTabs: []*v1.Resource{
					{Name: "tab 1", Link: "/dashboards/dashboard1/tabs/tab1"},
					{Name: "tab 2", Link: "/dashboards/dashboard1/tabs/tab2"},
				},
			},
		},
		{
			name: "Reads specified configs",
			config: map[string]*pb.Configuration{
				"gs://default/config": {
					Dashboards: []*pb.Dashboard{
						{
							Name: "wrong-dashboard",
							DashboardTab: []*pb.DashboardTab{
								{
									Name: "wrong-dashboard tab 1",
								},
							},
						},
					},
				},
				"gs://example/config": {
					Dashboards: []*pb.Dashboard{
						{
							Name: "correct-dashboard",
							DashboardTab: []*pb.DashboardTab{
								{
									Name: "correct-dashboard tab 1",
								},
							},
						},
					},
				},
			},
			req: &v1.ListDashboardTabsRequest{
				Dashboard: "correct-dashboard",
				Scope:     "gs://example",
			},
			expected: &v1.ListDashboardTabsResponse{
				DashboardTabs: []*v1.Resource{
					{Name: "correct-dashboard tab 1", Link: "/dashboards/correctdashboard/tabs/correctdashboardtab1?scope=gs://example"},
				},
			},
		},
		{
			name: "Specified configs never reads default config",
			config: map[string]*pb.Configuration{
				"gs://default/config": {
					Dashboards: []*pb.Dashboard{
						{
							Name: "wrong-dashboard",
							DashboardTab: []*pb.DashboardTab{
								{
									Name: "wrong-dashboard tab 1",
								},
							},
						},
					},
				},
				"gs://example/config": {
					Dashboards: []*pb.Dashboard{
						{
							Name: "correct-dashboard",
							DashboardTab: []*pb.DashboardTab{
								{
									Name: "correct-dashboard tab 1",
								},
							},
						},
					},
				},
			},
			req: &v1.ListDashboardTabsRequest{
				Dashboard: "wrong-dashboard",
				Scope:     "gs://example",
			},
			expectError: true,
		},
		{
			name: "Server error with unreadable config",
			req: &v1.ListDashboardTabsRequest{
				Dashboard: "dashboard1",
				Scope:     "gibberish",
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := setupTestServer(t, tc.config, nil)
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
