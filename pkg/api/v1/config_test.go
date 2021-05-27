/*
Copyright 2021 The Kubernetes Authors.

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
	pb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/golang/protobuf/proto"
)

func TestConfigPath(t *testing.T) {
	tests := []struct {
		name          string
		defaultBucket string
		url           string
		expected      *gcs.Path
	}{
		{
			name:          "Defaults to default",
			defaultBucket: "gs://example",
			expected:      getPathOrDie(t, "gs://example/config"),
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

			result, err := s.configPath(request)
			if test.expected == nil && err == nil {
				t.Fatalf("Expected an error, but got none")
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
		name         string
		config       pb.Configuration
		expectedJSON string
	}{
		{
			name:         "Returns an empty JSON when there's no groups",
			expectedJSON: `{}`,
		},
		{
			name: "Returns a Dashboard Group",
			config: pb.Configuration{
				DashboardGroups: []*pb.DashboardGroup{
					{
						Name: "Group1",
					},
				},
			},
			expectedJSON: `{"dashboard_groups":[{"name":"Group1","link":"host/dashboard-groups/group1"}]}`,
		},
		{
			name: "Returns multiple Dashboard Groups",
			config: pb.Configuration{
				DashboardGroups: []*pb.DashboardGroup{
					{
						Name: "Group1",
					},
					{
						Name: "Second Group",
					},
				},
			},
			expectedJSON: `{"dashboard_groups":[{"name":"Group1","link":"host/dashboard-groups/group1"},{"name":"Second Group","link":"host/dashboard-groups/secondgroup"}]}`,
		},
	}

	for _, test := range tests {
		server := setupTestServer(t, &test.config)
		request, err := http.NewRequest("GET", "/api/v1/dashboardgroups", nil)
		if err != nil {
			t.Fatalf("Can't form dashboardgroup request")
		}
		response := httptest.NewRecorder()
		server.ListDashboardGroups(response, request)
		if response.Code != http.StatusOK {
			t.Errorf("Expected OK status, but got %v", response.Code)
		}
		if response.Body.String() != test.expectedJSON {
			t.Errorf("In Body, Expected %s; got %s", test.expectedJSON, response.Body.String())
		}
	}

}

///////////////////
// Helper Functions
///////////////////

func setupTestServer(t *testing.T, configuration *pb.Configuration) Server {
	t.Helper()
	var fc fakeClient
	fc.Datastore = map[gcs.Path][]byte{}

	path, err := gcs.NewPath("gs://tg-example/config")
	if err != nil {
		t.Fatalf("setupTestServer() can't generate path: %v", err)
	}

	fc.Datastore[*path], err = proto.Marshal(configuration)
	if err != nil {
		t.Fatalf("Could not serialize proto: %v\n\nProto:\n%s", err, configuration.String())
	}

	return Server{
		Client:        fc,
		Host:          "host",
		DefaultBucket: "gs://tg-example",
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
