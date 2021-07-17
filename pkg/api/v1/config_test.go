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
	"testing"

	"cloud.google.com/go/storage"
	"github.com/golang/protobuf/proto"

	pb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
)

func TestConfigPath(t *testing.T) {
	tests := []struct {
		name          string
		defaultBucket string
		url           string
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
			url:           "http://testgrid/api/v1/foo?scope=gs://example/path",
			expected:      getPathOrDie(t, "gs://example/path/config"),
		},
		{
			name:     "Do not require a default",
			url:      "http://testgrid/api/v1/foo?scope=gs://example/path",
			expected: getPathOrDie(t, "gs://example/path/config"),
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

			request, err := http.NewRequest("GET", test.url, nil)
			if err != nil {
				t.Fatalf("Can't form request")
			}

			result, isDefault, err := s.configPath(request)
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

func TestPassQueryParameters(t *testing.T) {
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
			result := passQueryParameters(req)
			if result != test.expected {
				t.Errorf("Want %q, but got %q", test.expected, result)
			}
		})
	}
}

func TestListDashboardGroups(t *testing.T) {
	tests := []struct {
		name             string
		config           map[string]*pb.Configuration
		params           string
		expectedResponse string
		expectedCode     int
	}{
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
			expectedCode:     http.StatusInternalServerError,
			params:           "?scope=gs://bad-path",
			expectedResponse: "Could not read config at \"gs://bad-path/config\"\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := Route(nil, setupTestServer(t, test.config))
			request, err := http.NewRequest("GET", "/dashboard-groups"+test.params, nil)
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

func TestGetDashboardGroup(t *testing.T) {
	tests := []struct {
		name             string
		config           map[string]*pb.Configuration
		endpoint         string
		expectedResponse string
		expectedCode     int
	}{
		{
			name: "Returns an error when there's no resource",
			config: map[string]*pb.Configuration{
				"gs://default/config": {},
			},
			endpoint:         "/dashboard-groups/missing",
			expectedCode:     http.StatusNotFound,
			expectedResponse: "Dashboard group \"missing\" not found\n",
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
			endpoint:         "/dashboard-groups/Group1",
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
			endpoint:         "/dashboard-groups/stooges",
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
			endpoint:         "/dashboard-groups/right-group?scope=gs://example",
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
			endpoint:         "/dashboard-groups/wrong-group?scope=gs://example",
			expectedResponse: "Dashboard group \"wrong-group\" not found\n",
			expectedCode:     http.StatusNotFound,
		},
		{
			name:             "Server error with unreadable config",
			expectedCode:     http.StatusInternalServerError,
			endpoint:         "/dashboard-groups/group?scope=gs://bad-path",
			expectedResponse: "Could not read config at \"gs://bad-path/config\"\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := Route(nil, setupTestServer(t, test.config))
			request, err := http.NewRequest("GET", test.endpoint, nil)
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

func setupTestServer(t *testing.T, configurations map[string]*pb.Configuration) Server {
	t.Helper()

	var fc fakeClient
	fc.Datastore = map[gcs.Path][]byte{}

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

	return Server{
		Client:        fc,
		Host:          "host",
		DefaultBucket: "gs://default",
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
	return ioutil.NopCloser(bytes.NewReader(data)), nil, nil
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
