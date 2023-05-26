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
	"net/http"
	"net/http/httptest"
	"testing"

	apipb "github.com/GoogleCloudPlatform/testgrid/pb/api/v1"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	summarypb "github.com/GoogleCloudPlatform/testgrid/pb/summary"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestQueryParams(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name: "No Query Parameters",
		},
		{
			name:     "Passes scope parameter only",
			url:      "/foo?scope=gs://example/bucket&bucket=fake&format=json",
			expected: "?scope=gs://example/bucket",
		},
		{
			name:     "Use only the first scope parameter",
			url:      "/foo?scope=gs://example/bucket&scope=gs://fake/bucket",
			expected: "?scope=gs://example/bucket",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", test.url, nil)
			if err != nil {
				t.Fatalf("can't create request for %s", test.url)
			}
			result := queryParams(req.URL.Query().Get(scopeParam))
			if result != test.expected {
				t.Errorf("Want %q, but got %q", test.expected, result)
			}
		})
	}
}

type TestSpec struct {
	name             string
	config           map[string]*configpb.Configuration
	summaries        map[string]*summarypb.DashboardSummary
	grid             map[string]*statepb.Grid
	endpoint         string
	params           string
	expectedResponse proto.Message
	expectedCode     int
}

func TestListDashboardGroupHTTP(t *testing.T) {
	tests := []TestSpec{
		{
			name: "Returns an empty JSON when there's no groups",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {},
			},
			expectedResponse: &apipb.ListDashboardGroupResponse{},
			expectedCode:     http.StatusOK,
		},
		{
			name: "Returns a Dashboard Group",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					DashboardGroups: []*configpb.DashboardGroup{
						{
							Name: "Group1",
						},
					},
				},
			},
			expectedResponse: &apipb.ListDashboardGroupResponse{
				DashboardGroups: []*apipb.Resource{
					{
						Name: "Group1",
						Link: "/dashboard-groups/group1",
					},
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "Returns multiple Dashboard Groups",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					DashboardGroups: []*configpb.DashboardGroup{
						{
							Name: "Group1",
						},
						{
							Name: "Second Group",
						},
					},
				},
			},
			expectedResponse: &apipb.ListDashboardGroupResponse{
				DashboardGroups: []*apipb.Resource{
					{
						Name: "Group1",
						Link: "/dashboard-groups/group1",
					},
					{
						Name: "Second Group",
						Link: "/dashboard-groups/secondgroup",
					},
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "Reads specified configs",
			config: map[string]*configpb.Configuration{
				"gs://example/config": {
					DashboardGroups: []*configpb.DashboardGroup{
						{
							Name: "Group1",
						},
					},
				},
			},
			params: "?scope=gs://example",
			expectedResponse: &apipb.ListDashboardGroupResponse{
				DashboardGroups: []*apipb.Resource{
					{
						Name: "Group1",
						Link: "/dashboard-groups/group1?scope=gs://example",
					},
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name:         "Server error with unreadable config",
			params:       "?scope=gs://bad-path",
			expectedCode: http.StatusNotFound,
		},
	}
	var resp apipb.ListDashboardGroupResponse
	RunTestsAgainstEndpoint(t, "/dashboard-groups", tests, &resp)
}

func TestGetDashboardGroupHTTP(t *testing.T) {
	tests := []TestSpec{
		{
			name: "Returns an error when there's no resource",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {},
			},
			endpoint:     "missing",
			expectedCode: http.StatusNotFound,
		},
		{
			name: "Returns empty JSON from an empty Dashboard Group",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					DashboardGroups: []*configpb.DashboardGroup{
						{
							Name: "Group1",
						},
					},
				},
			},
			endpoint:         "/group1",
			expectedResponse: &apipb.GetDashboardGroupResponse{},
			expectedCode:     http.StatusOK,
		},
		{
			name: "Returns dashboards from group",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					DashboardGroups: []*configpb.DashboardGroup{
						{
							Name:           "stooges",
							DashboardNames: []string{"larry", "curly", "moe"},
						},
					},
				},
			},
			endpoint: "/stooges",
			expectedResponse: &apipb.GetDashboardGroupResponse{
				Dashboards: []*apipb.Resource{
					{
						Name: "curly",
						Link: "/dashboards/curly",
					},
					{
						Name: "larry",
						Link: "/dashboards/larry",
					},
					{
						Name: "moe",
						Link: "/dashboards/moe",
					},
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "Reads 'scope' parameter",
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
			endpoint: "/rightgroup?scope=gs://example",
			expectedResponse: &apipb.GetDashboardGroupResponse{
				Dashboards: []*apipb.Resource{
					{
						Name: "yes",
						Link: "/dashboards/yes?scope=gs://example",
					},
				},
			},
			expectedCode: http.StatusOK,
		},
	}
	var resp apipb.GetDashboardGroupResponse
	RunTestsAgainstEndpoint(t, "/dashboard-groups", tests, &resp)
}

func TestListDashboardsHTTP(t *testing.T) {
	tests := []TestSpec{
		{
			name: "Returns an empty JSON when there is no dashboards",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {},
			},
			expectedResponse: &apipb.ListDashboardResponse{},
			expectedCode:     http.StatusOK,
		},
		{
			name: "Returns a Dashboard which doesn't belong to a grop",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name: "Dashboard1",
						},
					},
				},
			},
			expectedResponse: &apipb.ListDashboardResponse{
				Dashboards: []*apipb.DashboardResource{
					{
						Name: "Dashboard1",
						Link: "/dashboards/dashboard1",
					},
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "Returns multiple Dashboards which belong to different groups",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name: "Dashboard1",
						},
						{
							Name: "Dashboard2",
						},
					},
					DashboardGroups: []*configpb.DashboardGroup{
						{
							Name:           "DashboardGroup1",
							DashboardNames: []string{"Dashboard1"},
						},
						{
							Name:           "DashboardGroup2",
							DashboardNames: []string{"Dashboard2"},
						},
					},
				},
			},
			expectedResponse: &apipb.ListDashboardResponse{
				Dashboards: []*apipb.DashboardResource{
					{
						Name:               "Dashboard1",
						Link:               "/dashboards/dashboard1",
						DashboardGroupName: "DashboardGroup1",
					},
					{
						Name:               "Dashboard2",
						Link:               "/dashboards/dashboard2",
						DashboardGroupName: "DashboardGroup2",
					},
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "Reads from other config/scope",
			config: map[string]*configpb.Configuration{
				"gs://example/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name: "Dashboard1",
						},
					},
				},
			},
			params: "?scope=gs://example",
			expectedResponse: &apipb.ListDashboardResponse{
				Dashboards: []*apipb.DashboardResource{
					{
						Name: "Dashboard1",
						Link: "/dashboards/dashboard1?scope=gs://example",
					},
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name:         "Server error with unreadable config",
			params:       "?scope=gs://bad-path",
			expectedCode: http.StatusNotFound,
		},
	}
	var resp apipb.ListDashboardResponse
	RunTestsAgainstEndpoint(t, "/dashboards", tests, &resp)
}

func TestGetDashboardHTTP(t *testing.T) {
	tests := []TestSpec{
		{
			name: "Returns an error when there's no resource",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {},
			},
			endpoint:     "/missing",
			expectedCode: http.StatusNotFound,
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
			endpoint:         "/dashboard1",
			expectedResponse: &apipb.GetDashboardResponse{},
			expectedCode:     http.StatusOK,
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
			endpoint: "/dashboard1",
			expectedResponse: &apipb.GetDashboardResponse{
				Notifications: []*configpb.Notification{
					{
						Summary:     "Notification summary",
						ContextLink: "Notification context link",
					},
				},
				DefaultTab:          "defaultTab",
				SuppressFailingTabs: true,
				HighlightToday:      true,
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "Reads 'scope' parameter",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name:                "wrong-dashboard",
							DefaultTab:          "wrong-dashboard defaultTab",
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
				"gs://example/config": {
					Dashboards: []*configpb.Dashboard{
						{
							Name:                "correct-dashboard",
							DefaultTab:          "correct-dashboard defaultTab",
							HighlightToday:      true,
							DownplayFailingTabs: true,
							Notifications:       []*configpb.Notification{},
						},
					},
				},
			},
			endpoint: "/correctdashboard",
			params:   "?scope=gs://example",
			expectedResponse: &apipb.GetDashboardResponse{
				DefaultTab:          "correct-dashboard defaultTab",
				SuppressFailingTabs: true,
				HighlightToday:      true,
			},
			expectedCode: http.StatusOK,
		},
	}
	var resp apipb.GetDashboardResponse
	RunTestsAgainstEndpoint(t, "/dashboards", tests, &resp)
}

func TestListDashboardTabsHTTP(t *testing.T) {
	tests := []TestSpec{
		{
			name: "Returns an error",
			config: map[string]*configpb.Configuration{
				"gs://default/config": {},
			},
			endpoint:     "/missingdashboard/tabs",
			expectedCode: http.StatusNotFound,
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
			endpoint:         "/dashboard1/tabs",
			expectedResponse: &apipb.ListDashboardTabsResponse{},
			expectedCode:     http.StatusOK,
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
			endpoint: "/dashboard1/tabs",
			expectedResponse: &apipb.ListDashboardTabsResponse{
				DashboardTabs: []*apipb.Resource{
					{
						Name: "tab 1",
						Link: "/dashboards/dashboard1/tabs/tab1",
					},
					{
						Name: "tab 2",
						Link: "/dashboards/dashboard1/tabs/tab2",
					},
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "Reads 'scope' parameter",
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
			endpoint: "/correctdashboard/tabs",
			params:   "?scope=gs://example",
			expectedResponse: &apipb.ListDashboardTabsResponse{
				DashboardTabs: []*apipb.Resource{
					{
						Name: "correct-dashboard tab 1",
						Link: "/dashboards/correctdashboard/tabs/correctdashboardtab1?scope=gs://example",
					},
				},
			},
			expectedCode: http.StatusOK,
		},
	}
	var resp apipb.ListDashboardTabsResponse
	RunTestsAgainstEndpoint(t, "/dashboards", tests, &resp)
}

func RunTestsAgainstEndpoint(t *testing.T, baseEndpoint string, tests []TestSpec, resp proto.Message) {
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := Route(nil, setupTestServer(t, test.config, nil, nil))
			absEndpoint := baseEndpoint + test.endpoint + test.params
			request, err := http.NewRequest("GET", absEndpoint, nil)
			if err != nil {
				t.Fatalf("Can't form request: %v", err)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != test.expectedCode {
				t.Errorf("Expected %d, but got %d", test.expectedCode, response.Code)
			}

			if response.Code == http.StatusOK {
				if err := protojson.Unmarshal(response.Body.Bytes(), resp); err != nil {
					t.Fatalf("Failed to unmarshal json message into a proto message: %v", err)
				}
				if diff := cmp.Diff(test.expectedResponse, resp, protocmp.Transform()); diff != "" {
					t.Errorf("Obtained unexpected  diff (-want +got):\n%s", diff)
				}
			}
		})
	}
}
