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
	"context"
	"reflect"
	"testing"

	apipb "github.com/GoogleCloudPlatform/testgrid/pb/api/v1"

	pb "github.com/GoogleCloudPlatform/testgrid/pb/config"

	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

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
	tests := []struct {
		name        string
		config      map[string]*pb.Configuration
		grid        map[string]*statepb.Grid
		req         *apipb.ListHeadersRequest
		want        *apipb.ListHeadersResponse
		expectError bool
	}{
		{
			name: "Returns an error when there's no dashboard resource",
			config: map[string]*pb.Configuration{
				"gs://default/config": {},
			},
			req: &apipb.ListHeadersRequest{
				Dashboard: "missing",
				Tab:       "some tab",
			},
			expectError: true,
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
			req: &apipb.ListHeadersRequest{
				Dashboard: "dashboard1",
				Tab:       "missing",
			},
			expectError: true,
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
			req: &apipb.ListHeadersRequest{
				Dashboard: "Dashboard1",
				Tab:       "tab 1",
			},
			want: &apipb.ListHeadersResponse{},
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
							Started: 1635693255000, // Milliseconds
							Extra:   []string{""},
						},
						{
							Build:   "80",
							Hint:    "80",
							Started: 1635779655000, // Milliseconds
							Extra:   []string{"build80"},
						},
					},
				},
			},
			req: &apipb.ListHeadersRequest{
				Dashboard: "dashboard1",
				Tab:       "tab1",
			},
			want: &apipb.ListHeadersResponse{
				Headers: []*apipb.ListHeadersResponse_Header{
					{
						Build:   "99",
						Started: &timestamppb.Timestamp{Seconds: 1635693255},
						Extra:   []string{""},
					},
					{
						Build:   "80",
						Started: &timestamppb.Timestamp{Seconds: 1635779655},
						Extra:   []string{"build80"},
					},
				},
			},
		},
		{
			name: "Returns correct timestamps from a tab",
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
							Started: 1, // Milliseconds
						},
						{
							Build:   "80",
							Hint:    "80",
							Started: 1635779655123, // Milliseconds
						},
					},
				},
			},
			req: &apipb.ListHeadersRequest{
				Dashboard: "dashboard 1",
				Tab:       "tab 1",
			},
			want: &apipb.ListHeadersResponse{
				Headers: []*apipb.ListHeadersResponse_Header{
					{
						Build:   "99",
						Started: &timestamppb.Timestamp{Nanos: 1000000},
					},
					{
						Build:   "80",
						Started: &timestamppb.Timestamp{Seconds: 1635779655, Nanos: 123000000},
					},
				},
			},
		},
		{
			name: "Server error with unreadable config",
			config: map[string]*pb.Configuration{
				"gs://default/config": {},
			},
			req: &apipb.ListHeadersRequest{
				Dashboard: "dashboard 1",
				Tab:       "tab 1",
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := setupTestServer(t, tc.config, tc.grid)
			got, err := server.ListHeaders(context.Background(), tc.req)
			switch {
			case err != nil:
				if !tc.expectError {
					t.Errorf("got unexpected error: %v", err)
				}
			case tc.expectError:
				t.Error("failed to receive an error")
			default:
				if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
					t.Errorf("got unexpected diff (-want +got):\n%s", diff)
				}
			}
		})

	}

}

func TestListRows(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]*pb.Configuration
		grid   map[string]*statepb.Grid
		patch  func(*Server)
		req    *apipb.ListRowsRequest
		want   *apipb.ListRowsResponse
		err    bool
	}{
		{
			name: "Returns an error when there's no dashboard resource",
			config: map[string]*pb.Configuration{
				"gs://default/config": {},
			},
			req: &apipb.ListRowsRequest{
				Scope:     "gs://default",
				Dashboard: "missing",
				Tab:       "irrelevant",
			},
			err: true,
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
			req: &apipb.ListRowsRequest{
				Scope:     "gs://default",
				Dashboard: "Dashboard1",
				Tab:       "irrelevant",
			},
			err: true,
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
			req: &apipb.ListRowsRequest{
				Scope:     "gs://default",
				Dashboard: "dashboard1",
				Tab:       "tab1",
			},
			want: &apipb.ListRowsResponse{},
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
			req: &apipb.ListRowsRequest{
				Scope:     "gs://default",
				Dashboard: "dashboard1",
				Tab:       "tab1",
			},
			want: &apipb.ListRowsResponse{
				Rows: []*apipb.ListRowsResponse_Row{
					{
						Name: "tabrow1",
						Cells: []*apipb.ListRowsResponse_Cell{
							{
								Result: 1,
								CellId: "cell-1",
							},
							{
								Result: 1,
								CellId: "cell-2",
							},
						},
					},
				},
			},
		},
		{
			name: "Returns tab from tab state",
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
				"gs://default/look-ma-tabs/Dashboard1/tab%201": {
					Rows: []*statepb.Row{
						{
							Name:     "tabrow1",
							Id:       "tabrow1",
							Results:  []int32{1, 2},
							CellIds:  []string{"cell-1", "cell-2"},
							Messages: []string{"tab soda", "", "", ""},
							Icons:    []string{"", "", "", ""},
						},
					},
				},
			},
			patch: func(s *Server) {
				s.TabPathPrefix = "look-ma-tabs"
			},
			req: &apipb.ListRowsRequest{
				Scope:     "gs://default",
				Dashboard: "dashboard1",
				Tab:       "tab1",
			},
			want: &apipb.ListRowsResponse{
				Rows: []*apipb.ListRowsResponse_Row{
					{
						Name: "tabrow1",
						Cells: []*apipb.ListRowsResponse_Cell{
							{
								Result:  1,
								CellId:  "cell-1",
								Message: "tab soda",
							},
							{
								Result: 1,
								CellId: "cell-2",
							},
						},
					},
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := setupTestServer(t, tc.config, tc.grid)
			if tc.patch != nil {
				tc.patch(&server)
			}
			got, err := server.ListRows(context.Background(), tc.req)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("ListRows() got unexpected error: %v", err)
				}
			case tc.err:
				t.Error("ListRows() failed to receive an error")
			default:
				if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
					t.Errorf("ListRows() got unexpected diff (-want +got):\n%s", diff)
				}
			}
		})
	}
}
