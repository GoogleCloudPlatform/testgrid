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
	"fmt"
	"net/http"
	"reflect"
	"testing"

	pb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
)

func TestFindDashboardTab(t *testing.T) {
	tests := []struct {
		name         string
		config       *pb.Configuration
		dashboardKey string
		tabKey       string
		expected     *pb.DashboardTab
	}{
		{
			name: "returns nil if no dashboards exists",
			config: &pb.Configuration{
				Dashboards: []*pb.Dashboard{},
			},
			dashboardKey: "dashboard1",
			tabKey:       "tab1",
			expected:     nil,
		},
		{
			name: "return nil if no dashboards match",
			config: &pb.Configuration{
				Dashboards: []*pb.Dashboard{
					{
						Name: "Dashboard-2",
					},
				},
			},
			dashboardKey: "dashboard1",
			tabKey:       "tab1",
			expected:     nil,
		},
		{
			name: "return nil if no tab match",
			config: &pb.Configuration{
				Dashboards: []*pb.Dashboard{
					{
						Name: "dashboard1",
						DashboardTab: []*pb.DashboardTab{
							{
								Name: "tab-2",
							},
						},
					},
				},
			},
			dashboardKey: "dashboard1",
			tabKey:       "tab1",
			expected:     nil,
		},
		{
			name: "return correct tab if match found",
			config: &pb.Configuration{
				Dashboards: []*pb.Dashboard{
					{
						Name: "dashboard1",
						DashboardTab: []*pb.DashboardTab{
							{
								Name: "tab-1",
							},
						},
					},
				},
			},
			dashboardKey: "dashboard1",
			tabKey:       "tab1",
			expected: &pb.DashboardTab{
				Name: "tab-1",
			},
		},
		{
			name: "Return error if config is null",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, _ := findDashboardTab(test.config, test.dashboardKey, test.tabKey)
			if (test.expected == nil && result != nil) || result.String() != test.expected.String() {
				t.Errorf("Want %s, but got %s", test.expected.String(), result.String())
			}
		})
	}
}

func TestDecodeRLE(t *testing.T) {
	tests := []struct {
		name        string
		encodedData []int32
		expected    []int32
	}{
		{
			name:        "returns empty result if empty encoded data",
			encodedData: []int32{},
			expected:    []int32{},
		},
		{
			name:        "returns empty result if not valid encoded data",
			encodedData: []int32{1, 3, 4},
			expected:    []int32{},
		},
		{
			name:        "returns correct decoded result if valid encoded data",
			encodedData: []int32{1, 2, 3, 4},
			expected:    []int32{1, 1, 3, 3, 3, 3},
		},
		{
			name:     "returns empty result if null encoded data",
			expected: []int32{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := decodeRLE(test.encodedData)
			if (test.expected == nil && result != nil) || len(result) != len(test.expected) || (len(test.expected) != 0 && !reflect.DeepEqual(result, test.expected)) {
				t.Errorf("Want %q, but got %q", test.expected, result)
			}
		})
	}
}

func TestListHeaders(t *testing.T) {
	build1Started, build2Started := float64(1628893101099000), float64(1628893101080000)
	tests := []TestSpec{
		{
			name: "Returns an error when there's no dashboard resource",
			config: map[string]*pb.Configuration{
				"gs://default/config": {},
			},
			endpoint:         "missingdashboard/tabs/tabname/headers",
			expectedResponse: "Dashboard {\"missingdashboard\"} or tab {\"tabname\"} not found\n",
			expectedCode:     http.StatusNotFound,
		},
		{
			name: "Returns an error when there's no tab resource",
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
			endpoint:         "dashboard1/tabs/tab1/headers",
			expectedResponse: "Dashboard {\"dashboard1\"} or tab {\"tab1\"} not found\n",
			expectedCode:     http.StatusNotFound,
		},
		{
			name: "Returns empty headers list from a tab",
			config: map[string]*pb.Configuration{
				"gs://default/config": {
					Dashboards: []*pb.Dashboard{
						{
							Name: "Dashboard1",
							DashboardTab: []*pb.DashboardTab{
								{
									Name:          "tab 1",
									TestGroupName: "testgroupname",
								},
							},
						},
					},
				},
			},
			grid: map[string]*statepb.Grid{
				"gs://default/grid/testgroupname": {},
			},
			endpoint:         "dashboard1/tabs/tab1/headers",
			expectedResponse: `{}`,
			expectedCode:     http.StatusOK,
		},
		{
			name: "Returns correct headers from a tab",
			config: map[string]*pb.Configuration{
				"gs://default/config": {
					Dashboards: []*pb.Dashboard{
						{
							Name: "Dashboard1",
							DashboardTab: []*pb.DashboardTab{
								{
									Name:          "tab 1",
									TestGroupName: "testgroupname",
								},
							},
						},
					},
				},
			},
			grid: map[string]*statepb.Grid{
				"gs://default/grid/testgroupname": {
					Columns: []*statepb.Column{
						{
							Build:   "99",
							Hint:    "99",
							Started: build1Started,
							Extra:   []string{""},
						},
						{
							Build:   "80",
							Hint:    "80",
							Started: build2Started,
							Extra:   []string{"build80"},
						},
					},
				},
			},
			endpoint:         "dashboard1/tabs/tab1/headers",
			expectedResponse: `{"headers":[{"build":"99","started":{"seconds":` + fmt.Sprintf("%.0f", build1Started) + `},"extra":[""]},{"build":"80","started":{"seconds":` + fmt.Sprintf("%.0f", build2Started) + `},"extra":["build80"]}]}`,
			expectedCode:     http.StatusOK,
		},
		{
			name: "Server error with unreadable config",
			config: map[string]*pb.Configuration{
				"gs://default/config": {},
			},
			endpoint:         "dashboard1/tabs/tab1/headers",
			expectedResponse: "Dashboard {\"dashboard1\"} or tab {\"tab1\"} not found\n",
			expectedCode:     http.StatusNotFound,
		},
	}
	RunTestsAgainstEndpoint(t, "/dashboards/", tests)
}

func TestListRows(t *testing.T) {
	tests := []TestSpec{
		{
			name: "Returns an error when there's no dashboard resource",
			config: map[string]*pb.Configuration{
				"gs://default/config": {},
			},
			endpoint:         "missingdashboard/tabs/tabname/rows",
			expectedResponse: "Dashboard {\"missingdashboard\"} or tab {\"tabname\"} not found\n",
			expectedCode:     http.StatusNotFound,
		},
		{
			name: "Returns an error when there's no tab resource",
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
			endpoint:         "dashboard1/tabs/tab1/rows",
			expectedResponse: "Dashboard {\"dashboard1\"} or tab {\"tab1\"} not found\n",
			expectedCode:     http.StatusNotFound,
		},
		{
			name: "Returns empty rows list from a tab",
			config: map[string]*pb.Configuration{
				"gs://default/config": {
					Dashboards: []*pb.Dashboard{
						{
							Name: "Dashboard1",
							DashboardTab: []*pb.DashboardTab{
								{
									Name:          "tab 1",
									TestGroupName: "testgroupname",
								},
							},
						},
					},
				},
			},
			grid: map[string]*statepb.Grid{
				"gs://default/grid/testgroupname": {},
			},
			endpoint:         "dashboard1/tabs/tab1/rows",
			expectedResponse: `{}`,
			expectedCode:     http.StatusOK,
		},
		{
			name: "Returns correct rows from a tab",
			config: map[string]*pb.Configuration{
				"gs://default/config": {
					Dashboards: []*pb.Dashboard{
						{
							Name: "Dashboard1",
							DashboardTab: []*pb.DashboardTab{
								{
									Name:          "tab 1",
									TestGroupName: "testgroupname",
								},
							},
						},
					},
				},
			},
			grid: map[string]*statepb.Grid{
				"gs://default/grid/testgroupname": {
					Rows: []*statepb.Row{
						{
							Name:     "tabrow1",
							Id:       "tabrow1",
							Results:  []int32{1, 2},
							CellIds:  []string{"cell-1", "cell-2"},
							Messages: []string{"", "", "", ""},
							Icons:    []string{"", "", "", ""},
						},
					},
				},
			},
			endpoint:         "dashboard1/tabs/tab1/rows",
			expectedResponse: `{"rows":[{"name":"tabrow1","cells":[{"result":1,"cell_id":"cell-1"},{"result":1,"cell_id":"cell-2"}]}]}`,
			expectedCode:     http.StatusOK,
		},
		{
			name: "Server error with unreadable config",
			config: map[string]*pb.Configuration{
				"gs://default/config": {},
			},
			endpoint:         "dashboard1/tabs/tab1/rows",
			expectedResponse: "Dashboard {\"dashboard1\"} or tab {\"tab1\"} not found\n",
			expectedCode:     http.StatusNotFound,
		},
	}
	RunTestsAgainstEndpoint(t, "/dashboards/", tests)
}
