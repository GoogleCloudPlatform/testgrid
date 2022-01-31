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
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/golang/protobuf/proto"

	"github.com/GoogleCloudPlatform/testgrid/config"
	pb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
)

func TestConfigPath(t *testing.T) {
	tests := []struct {
		name          string
		defaultBucket string
		scopeParam    string
		expected      *gcs.Path
		expectDefault bool
	}{
		{
			name:          "Defaults to default",
			defaultBucket: "gs://example",
			expected:      getPathOrDie(t, "gs://example/config"),
			expectDefault: true,
		},
		{
			name:          "Use config if specified",
			defaultBucket: "gs://wrong",
			scopeParam:    "gs://example/path",
			expected:      getPathOrDie(t, "gs://example/path/config"),
		},
		{
			name:       "Do not require a default",
			scopeParam: "gs://example/path",
			expected:   getPathOrDie(t, "gs://example/path/config"),
		},
		{
			name: "Return error if no way to find config",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := Server{
				DefaultBucket: test.defaultBucket,
			}

			result, isDefault, err := s.configPath(test.scopeParam)
			if test.expected == nil && err == nil {
				t.Fatalf("Expected an error, but got none")
			}

			if test.expectDefault != isDefault {
				t.Errorf("Default Flag: Want %t, got %t", test.expectDefault, isDefault)
			}

			if test.expected != nil && result.String() != test.expected.String() {
				t.Errorf("Want %s, but got %s", test.expected.String(), result.String())
			}
		})
	}
}

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
			url:      "host/foo?scope=gs://example/bucket&bucket=fake&format=json",
			expected: "?scope=gs://example/bucket",
		},
		{
			name:     "Use only the first scope parameter",
			url:      "host/foo?scope=gs://example/bucket&scope=gs://fake/bucket",
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
	config           map[string]*pb.Configuration
	grid             map[string]*statepb.Grid
	endpoint         string
	params           string
	expectedResponse string
	expectedCode     int
}

func TestListDashboardGroups(t *testing.T) {
	tests := []TestSpec{
		{
			name: "Returns an empty JSON when there's no groups",
			config: map[string]*pb.Configuration{
				"gs://default/config": {},
			},
			expectedResponse: `{}`,
			expectedCode:     http.StatusOK,
		},
		{
			name: "Returns a Dashboard Group",
			config: map[string]*pb.Configuration{
				"gs://default/config": {
					DashboardGroups: []*pb.DashboardGroup{
						{
							Name: "Group1",
						},
					},
				},
			},
			expectedResponse: `{"dashboard_groups":[{"name":"Group1","link":"host/dashboard-groups/group1"}]}`,
			expectedCode:     http.StatusOK,
		},
		{
			name: "Returns multiple Dashboard Groups",
			config: map[string]*pb.Configuration{
				"gs://default/config": {
					DashboardGroups: []*pb.DashboardGroup{
						{
							Name: "Group1",
						},
						{
							Name: "Second Group",
						},
					},
				},
			},
			expectedResponse: `{"dashboard_groups":[{"name":"Group1","link":"host/dashboard-groups/group1"},{"name":"Second Group","link":"host/dashboard-groups/secondgroup"}]}`,
			expectedCode:     http.StatusOK,
		},
		{
			name: "Reads specified configs",
			config: map[string]*pb.Configuration{
				"gs://example/config": {
					DashboardGroups: []*pb.DashboardGroup{
						{
							Name: "Group1",
						},
					},
				},
			},
			params:           "?scope=gs://example",
			expectedResponse: `{"dashboard_groups":[{"name":"Group1","link":"host/dashboard-groups/group1?scope=gs://example"}]}`,
			expectedCode:     http.StatusOK,
		},
		{
			name:             "Server error with unreadable config",
			params:           "?scope=gs://bad-path",
			expectedResponse: "Could not read config at \"gs://bad-path/config\"\n",
			expectedCode:     http.StatusNotFound,
		},
	}

	RunTestsAgainstEndpoint(t, "/dashboard-groups", tests)
}

func TestGetDashboardGroup(t *testing.T) {
	tests := []TestSpec{
		{
			name: "Returns an error when there's no resource",
			config: map[string]*pb.Configuration{
				"gs://default/config": {},
			},
			endpoint:         "missing",
			expectedResponse: "Dashboard group \"missing\" not found\n",
			expectedCode:     http.StatusNotFound,
		},
		{
			name: "Returns empty JSON from an empty Dashboard Group",
			config: map[string]*pb.Configuration{
				"gs://default/config": {
					DashboardGroups: []*pb.DashboardGroup{
						{
							Name: "Group1",
						},
					},
				},
			},
			endpoint:         "group1",
			expectedResponse: `{}`,
			expectedCode:     http.StatusOK,
		},
		{
			name: "Returns dashboards from group",
			config: map[string]*pb.Configuration{
				"gs://default/config": {
					DashboardGroups: []*pb.DashboardGroup{
						{
							Name:           "stooges",
							DashboardNames: []string{"larry", "curly", "moe"},
						},
					},
				},
			},
			endpoint:         "stooges",
			expectedResponse: `{"dashboards":[{"name":"larry","link":"host/dashboards/larry"},{"name":"curly","link":"host/dashboards/curly"},{"name":"moe","link":"host/dashboards/moe"}]}`,
			expectedCode:     http.StatusOK,
		},
		{
			name: "Reads specified configs",
			config: map[string]*pb.Configuration{
				"gs://default/config": {
					DashboardGroups: []*pb.DashboardGroup{
						{
							Name:           "wrong-group",
							DashboardNames: []string{"no"},
						},
					},
				},
				"gs://example/config": {
					DashboardGroups: []*pb.DashboardGroup{
						{
							Name:           "right-group",
							DashboardNames: []string{"yes"},
						},
					},
				},
			},
			endpoint:         "rightgroup?scope=gs://example",
			expectedResponse: `{"dashboards":[{"name":"yes","link":"host/dashboards/yes?scope=gs://example"}]}`,
			expectedCode:     http.StatusOK,
		},
		{
			name: "Specified configs never reads default config",
			config: map[string]*pb.Configuration{
				"gs://default/config": {
					DashboardGroups: []*pb.DashboardGroup{
						{
							Name:           "wrong-group",
							DashboardNames: []string{"no"},
						},
					},
				},
				"gs://example/config": {
					DashboardGroups: []*pb.DashboardGroup{
						{
							Name:           "right-group",
							DashboardNames: []string{"yes"},
						},
					},
				},
			},
			endpoint:         "wronggroup?scope=gs://example",
			expectedResponse: "Dashboard group \"wronggroup\" not found\n",
			expectedCode:     http.StatusNotFound,
		},
		{
			name:             "Server error with unreadable config",
			endpoint:         "group?scope=gs://bad-path",
			expectedResponse: "Could not read config at \"gs://bad-path/config\"\n",
			expectedCode:     http.StatusNotFound,
		},
	}
	RunTestsAgainstEndpoint(t, "/dashboard-groups/", tests)
}

func TestListDashboards(t *testing.T) {
	tests := []TestSpec{
		{
			name: "Returns an empty JSON when there is no dashboards",
			config: map[string]*pb.Configuration{
				"gs://default/config": {},
			},
			expectedResponse: `{}`,
			expectedCode:     http.StatusOK,
		},
		{
			name: "Returns a Dashboard",
			config: map[string]*pb.Configuration{
				"gs://default/config": {
					Dashboards: []*pb.Dashboard{
						{
							Name: "Dashboard1",
						},
					},
				},
			},
			expectedResponse: `{"dashboards":[{"name":"Dashboard1","link":"host/dashboards/dashboard1"}]}`,
			expectedCode:     http.StatusOK,
		},
		{
			name: "Returns multiple Dashboards",
			config: map[string]*pb.Configuration{
				"gs://default/config": {
					Dashboards: []*pb.Dashboard{
						{
							Name: "Dashboard1",
						},
						{
							Name: "Dashboard2",
						},
					},
				},
			},
			expectedResponse: `{"dashboards":[{"name":"Dashboard1","link":"host/dashboards/dashboard1"},{"name":"Dashboard2","link":"host/dashboards/dashboard2"}]}`,
			expectedCode:     http.StatusOK,
		},
		{
			name: "Reads from other config/scope",
			config: map[string]*pb.Configuration{
				"gs://example/config": {
					Dashboards: []*pb.Dashboard{
						{
							Name: "Dashboard1",
						},
					},
				},
			},
			params:           "?scope=gs://example",
			expectedResponse: `{"dashboards":[{"name":"Dashboard1","link":"host/dashboards/dashboard1?scope=gs://example"}]}`,
			expectedCode:     http.StatusOK,
		},
		{
			name:             "Server error with unreadable config",
			params:           "?scope=gs://bad-path",
			expectedResponse: "Could not read config at \"gs://bad-path/config\"\n",
			expectedCode:     http.StatusNotFound,
		},
	}

	RunTestsAgainstEndpoint(t, "/dashboards", tests)
}

func TestGetDashboard(t *testing.T) {
	tests := []TestSpec{
		{
			name: "Returns an error when there's no resource",
			config: map[string]*pb.Configuration{
				"gs://default/config": {},
			},
			endpoint:         "missing",
			expectedResponse: "Dashboard \"missing\" not found\n",
			expectedCode:     http.StatusNotFound,
		},
		{
			name: "Returns empty JSON from an empty Dashboard",
			config: map[string]*pb.Configuration{
				"gs://default/config": {
					Dashboards: []*pb.Dashboard{
						{
							Name: "Dashboard1",
						},
					},
				},
			},
			endpoint:         "dashboard1",
			expectedResponse: `{}`,
			expectedCode:     http.StatusOK,
		},
		{
			name: "Returns dashboard info from dashboard",
			config: map[string]*pb.Configuration{
				"gs://default/config": {
					Dashboards: []*pb.Dashboard{
						{
							Name:                "Dashboard1",
							DefaultTab:          "defaultTab",
							HighlightToday:      true,
							DownplayFailingTabs: true,
							Notifications: []*pb.Notification{
								{
									Summary:     "Notification summary",
									ContextLink: "Notification context link",
								},
							},
						},
					},
				},
			},
			endpoint:         "dashboard1",
			expectedResponse: `{"notifications":[{"summary":"Notification summary","context_link":"Notification context link"}],"default_tab":"defaultTab","suppress_failing_tabs":true,"highlight_today":true}`,
			expectedCode:     http.StatusOK,
		},
		{
			name: "Reads specified configs",
			config: map[string]*pb.Configuration{
				"gs://default/config": {
					Dashboards: []*pb.Dashboard{
						{
							Name:                "wrong-dashboard",
							DefaultTab:          "wrong-dashboard defaultTab",
							HighlightToday:      true,
							DownplayFailingTabs: true,
							Notifications: []*pb.Notification{
								{
									Summary:     "Notification summary",
									ContextLink: "Notification context link",
								},
							},
						},
					},
				},
				"gs://example/config": {
					Dashboards: []*pb.Dashboard{
						{
							Name:                "correct-dashboard",
							DefaultTab:          "correct-dashboard defaultTab",
							HighlightToday:      true,
							DownplayFailingTabs: true,
							Notifications:       []*pb.Notification{},
						},
					},
				},
			},
			endpoint:         "correctdashboard",
			params:           "?scope=gs://example",
			expectedResponse: `{"default_tab":"correct-dashboard defaultTab","suppress_failing_tabs":true,"highlight_today":true}`,
			expectedCode:     http.StatusOK,
		},
		{
			name: "Specified configs never reads default config",
			config: map[string]*pb.Configuration{
				"gs://default/config": {
					Dashboards: []*pb.Dashboard{
						{
							Name:                "wrong-dashboard",
							DefaultTab:          "wrong-dashboard defaultTab",
							HighlightToday:      true,
							DownplayFailingTabs: true,
							Notifications: []*pb.Notification{
								{
									Summary:     "Notification summary",
									ContextLink: "Notification context link",
								},
							},
						},
					},
				},
				"gs://example/config": {
					Dashboards: []*pb.Dashboard{
						{
							Name:                "correct-dashboard",
							DefaultTab:          "correct-dashboard defaultTab",
							HighlightToday:      true,
							DownplayFailingTabs: true,
							Notifications:       []*pb.Notification{},
						},
					},
				},
			},
			endpoint:         "wrongdashboard",
			params:           "?scope=gs://example",
			expectedResponse: "Dashboard \"wrongdashboard\" not found\n",
			expectedCode:     http.StatusNotFound,
		},
		{
			name:             "Server error with unreadable config",
			endpoint:         "dashboard",
			params:           "?scope=gs://bad-path",
			expectedResponse: "Could not read config at \"gs://bad-path/config\"\n",
			expectedCode:     http.StatusNotFound,
		},
	}
	RunTestsAgainstEndpoint(t, "/dashboards/", tests)
}

func TestGetDashboardTabs(t *testing.T) {
	tests := []TestSpec{
		{
			name: "Returns an error when there's no resource",
			config: map[string]*pb.Configuration{
				"gs://default/config": {},
			},
			endpoint:         "missingdashboard/tabs",
			expectedResponse: "Dashboard \"missingdashboard\" not found\n",
			expectedCode:     http.StatusNotFound,
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
			endpoint:         "dashboard1/tabs",
			expectedResponse: `{}`,
			expectedCode:     http.StatusOK,
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
			endpoint:         "dashboard1/tabs",
			expectedResponse: `{"dashboard_tabs":[{"name":"tab 1","link":"host/dashboards/dashboard1/tabs/tab1"},{"name":"tab 2","link":"host/dashboards/dashboard1/tabs/tab2"}]}`,
			expectedCode:     http.StatusOK,
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
			endpoint:         "correctdashboard/tabs",
			params:           "?scope=gs://example",
			expectedResponse: `{"dashboard_tabs":[{"name":"correct-dashboard tab 1","link":"host/dashboards/correctdashboard/tabs/correctdashboardtab1?scope=gs://example"}]}`,
			expectedCode:     http.StatusOK,
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
			endpoint:         "wrongdashboard/tabs",
			params:           "?scope=gs://example",
			expectedResponse: "Dashboard \"wrongdashboard\" not found\n",
			expectedCode:     http.StatusNotFound,
		},
		{
			name:             "Server error with unreadable config",
			endpoint:         "dashboard1/tabs",
			params:           "?scope=gs://bad-path",
			expectedResponse: "Could not read config at \"gs://bad-path/config\"\n",
			expectedCode:     http.StatusNotFound,
		},
	}
	RunTestsAgainstEndpoint(t, "/dashboards/", tests)
}

func RunTestsAgainstEndpoint(t *testing.T, baseEndpoint string, tests []TestSpec) {
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := Route(nil, setupTestServer(t, test.config, test.grid))
			request, err := http.NewRequest("GET", baseEndpoint+test.endpoint+test.params, nil)
			if err != nil {
				t.Fatalf("Can't form request: %v", err)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.expectedCode {
				t.Errorf("Expected %d, but got %d", test.expectedCode, response.Code)
			}
			if response.Body.String() != test.expectedResponse {
				t.Errorf("In Body, Expected %q; got %q", test.expectedResponse, response.Body.String())
			}
		})
	}
}

///////////////////
// Helper Functions
///////////////////

func setupTestServer(t *testing.T, configurations map[string]*pb.Configuration, grids map[string]*statepb.Grid) Server {
	t.Helper()

	var fc fakeClient
	fc.Datastore = map[gcs.Path][]byte{}

	config.InitCache()
	for p, cfg := range configurations {
		path, err := gcs.NewPath(p)
		if err != nil {
			t.Fatalf("setupTestServer() can't generate path: %v", err)
		}

		fc.Datastore[*path], err = proto.Marshal(cfg)
		if err != nil {
			t.Fatalf("Could not serialize proto: %v\n\nProto:\n%s", err, cfg.String())
		}
	}

	for p, grid := range grids {
		path, err := gcs.NewPath(p)
		if err != nil {
			t.Fatalf("setupTestServer() can't generate path: %v", err)
		}

		fc.Datastore[*path], err = gcs.MarshalGrid(grid)
		if err != nil {
			t.Fatalf("Could not serialize proto: %v\n\nProto:\n%s", err, grid.String())
		}
	}

	host, err := url.Parse("host")
	if err != nil {
		t.Fatalf("Could not form host: %v", err)
	}

	return Server{
		Client:         fc,
		Host:           host,
		DefaultBucket:  "gs://default",
		GridPathPrefix: "grid",
		Timeout:        10 * time.Minute,
	}
}

func getPathOrDie(t *testing.T, s string) *gcs.Path {
	t.Helper()
	path, err := gcs.NewPath(s)
	if err != nil {
		t.Fatalf("Couldn't make path %s: %v", s, err)
	}
	return path
}

type fakeClient struct {
	Datastore map[gcs.Path][]byte
}

func (f fakeClient) Open(ctx context.Context, path gcs.Path) (io.ReadCloser, *storage.ReaderObjectAttrs, error) {
	data, exists := f.Datastore[path]
	if !exists {
		return nil, nil, fmt.Errorf("fake file %s does not exist", path.String())
	}
	return ioutil.NopCloser(bytes.NewReader(data)), &storage.ReaderObjectAttrs{}, nil
}

func (f fakeClient) Upload(ctx context.Context, path gcs.Path, bytes []byte, b bool, s string) (*storage.ObjectAttrs, error) {
	panic("fakeClient Upload not implemented")
}

func (f fakeClient) Objects(ctx context.Context, prefix gcs.Path, delimiter, start string) gcs.Iterator {
	panic("fakeClient Objects not implemented")
}

func (f fakeClient) Stat(ctx context.Context, prefix gcs.Path) (*storage.ObjectAttrs, error) {
	panic("fakeClient Stat not implemented")
}

func (f fakeClient) Copy(ctx context.Context, from, to gcs.Path) (*storage.ObjectAttrs, error) {
	panic("fakeClient Copy not implemented")
}
