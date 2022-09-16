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

package tabulator

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/testing/protocmp"

	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	tspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	"github.com/GoogleCloudPlatform/testgrid/pkg/updater"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs/fake"
)

func TestTabStatePath(t *testing.T) {
	path := newPathOrDie("gs://bucket/config")
	cases := []struct {
		name           string
		dashboardName  string
		tabName        string
		tabStatePrefix string
		expected       *gcs.Path
	}{
		{
			name:     "basically works",
			expected: path,
		},
		{
			name:          "invalid dashboard name errors",
			dashboardName: "---://foo",
			tabName:       "ok",
		},
		{
			name:          "invalid tab name errors",
			dashboardName: "cool",
			tabName:       "--??!f///",
		},
		{
			name:          "bucket change errors",
			dashboardName: "gs://honey-bucket/config",
			tabName:       "tab",
		},
		{
			name:          "normal behavior works",
			dashboardName: "dashboard",
			tabName:       "some-tab",
			expected:      newPathOrDie("gs://bucket/dashboard/some-tab"),
		},
		{
			name:           "target a subfolder works",
			tabStatePrefix: "tab-state",
			dashboardName:  "dashboard",
			tabName:        "some-tab",
			expected:       newPathOrDie("gs://bucket/tab-state/dashboard/some-tab"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := TabStatePath(*path, tc.tabStatePrefix, tc.dashboardName, tc.tabName)
			switch {
			case err != nil:
				if tc.expected != nil {
					t.Errorf("tabStatePath(%v, %v) got unexpected error: %v", tc.dashboardName, tc.tabName, err)
				}
			case tc.expected == nil:
				t.Errorf("tabStatePath(%v, %v) failed to receive an error", tc.dashboardName, tc.tabName)
			default:
				if diff := cmp.Diff(actual, tc.expected, cmp.AllowUnexported(gcs.Path{})); diff != "" {
					t.Errorf("tabStatePath(%v, %v) got unexpected diff (-have, +want):\n%s", tc.dashboardName, tc.tabName, diff)
				}
			}
		})
	}
}

func newPathOrDie(s string) *gcs.Path {
	p, err := gcs.NewPath(s)
	if err != nil {
		panic(err)
	}
	return p
}

func Test_DropEmptyColumns(t *testing.T) {
	testcases := []struct {
		name     string
		grid     []updater.InflatedColumn
		expected []updater.InflatedColumn
	}{
		{
			name:     "empty",
			grid:     []updater.InflatedColumn{},
			expected: []updater.InflatedColumn{},
		},
		{
			name:     "nil",
			grid:     nil,
			expected: []updater.InflatedColumn{},
		},
		{
			name: "drops empty column",
			grid: []updater.InflatedColumn{
				{
					Column: &statepb.Column{Name: "full"},
					Cells: map[string]updater.Cell{
						"first":  {Result: tspb.TestStatus_BUILD_PASSED},
						"second": {Result: tspb.TestStatus_PASS_WITH_SKIPS},
						"third":  {Result: tspb.TestStatus_FAIL},
					},
				},
				{
					Column: &statepb.Column{Name: "empty"},
					Cells: map[string]updater.Cell{
						"first":  {Result: tspb.TestStatus_NO_RESULT},
						"second": {Result: tspb.TestStatus_NO_RESULT},
						"third":  {Result: tspb.TestStatus_NO_RESULT},
					},
				},
				{
					Column: &statepb.Column{Name: "sparse"},
					Cells: map[string]updater.Cell{
						"first":  {Result: tspb.TestStatus_NO_RESULT},
						"second": {Result: tspb.TestStatus_TIMED_OUT},
						"third":  {Result: tspb.TestStatus_NO_RESULT},
					},
				},
			},
			expected: []updater.InflatedColumn{
				{
					Column: &statepb.Column{Name: "full"},
					Cells: map[string]updater.Cell{
						"first":  {Result: tspb.TestStatus_BUILD_PASSED},
						"second": {Result: tspb.TestStatus_PASS_WITH_SKIPS},
						"third":  {Result: tspb.TestStatus_FAIL},
					},
				},
				{
					Column: &statepb.Column{Name: "sparse"},
					Cells: map[string]updater.Cell{
						"first":  {Result: tspb.TestStatus_NO_RESULT},
						"second": {Result: tspb.TestStatus_TIMED_OUT},
						"third":  {Result: tspb.TestStatus_NO_RESULT},
					},
				},
			},
		},
		{
			name: "drop multiple columns in a row",
			grid: []updater.InflatedColumn{
				{
					Column: &statepb.Column{Name: "empty"},
					Cells: map[string]updater.Cell{
						"first":  {Result: tspb.TestStatus_NO_RESULT},
						"second": {Result: tspb.TestStatus_NO_RESULT},
						"third":  {Result: tspb.TestStatus_NO_RESULT},
					},
				},
				{
					Column: &statepb.Column{Name: "nothing"},
					Cells: map[string]updater.Cell{
						"first":  {Result: tspb.TestStatus_NO_RESULT},
						"second": {Result: tspb.TestStatus_NO_RESULT},
						"third":  {Result: tspb.TestStatus_NO_RESULT},
					},
				},
				{
					Column: &statepb.Column{Name: "full"},
					Cells: map[string]updater.Cell{
						"first":  {Result: tspb.TestStatus_BUILD_PASSED},
						"second": {Result: tspb.TestStatus_PASS_WITH_SKIPS},
						"third":  {Result: tspb.TestStatus_FAIL},
					},
				},
				{
					Column: &statepb.Column{Name: "zero"},
					Cells: map[string]updater.Cell{
						"first":  {Result: tspb.TestStatus_NO_RESULT},
						"second": {Result: tspb.TestStatus_NO_RESULT},
						"third":  {Result: tspb.TestStatus_NO_RESULT},
					},
				},
				{
					Column: &statepb.Column{Name: "nada"},
					Cells: map[string]updater.Cell{
						"first":  {Result: tspb.TestStatus_NO_RESULT},
						"second": {Result: tspb.TestStatus_NO_RESULT},
						"third":  {Result: tspb.TestStatus_NO_RESULT},
					},
				},
			},
			expected: []updater.InflatedColumn{
				{
					Column: &statepb.Column{Name: "full"},
					Cells: map[string]updater.Cell{
						"first":  {Result: tspb.TestStatus_BUILD_PASSED},
						"second": {Result: tspb.TestStatus_PASS_WITH_SKIPS},
						"third":  {Result: tspb.TestStatus_FAIL},
					},
				},
			},
		},
		{
			name: "don't drop everything",
			grid: []updater.InflatedColumn{
				{
					Column: &statepb.Column{Name: "first"},
					Cells: map[string]updater.Cell{
						"first":  {Result: tspb.TestStatus_NO_RESULT},
						"second": {Result: tspb.TestStatus_NO_RESULT},
						"third":  {Result: tspb.TestStatus_NO_RESULT},
					},
				},
				{
					Column: &statepb.Column{Name: "empty"},
					Cells: map[string]updater.Cell{
						"first":  {Result: tspb.TestStatus_NO_RESULT},
						"second": {Result: tspb.TestStatus_NO_RESULT},
						"third":  {Result: tspb.TestStatus_NO_RESULT},
					},
				},
				{
					Column: &statepb.Column{Name: "nada"},
					Cells: map[string]updater.Cell{
						"first":  {Result: tspb.TestStatus_NO_RESULT},
						"second": {Result: tspb.TestStatus_NO_RESULT},
						"third":  {Result: tspb.TestStatus_NO_RESULT},
					},
				},
			},
			expected: []updater.InflatedColumn{
				{
					Column: &statepb.Column{Name: "first"},
					Cells: map[string]updater.Cell{
						"first":  {Result: tspb.TestStatus_NO_RESULT},
						"second": {Result: tspb.TestStatus_NO_RESULT},
						"third":  {Result: tspb.TestStatus_NO_RESULT},
					},
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			actual := dropEmptyColumns(tc.grid)
			if diff := cmp.Diff(actual, tc.expected, protocmp.Transform()); diff != "" {
				t.Errorf("(-got, +want): %s", diff)
			}
		})
	}
}

func Test_Tabulate(t *testing.T) {
	testcases := []struct {
		name           string
		grid           *statepb.Grid
		dashCfg        *configpb.DashboardTab
		groupCfg       *configpb.TestGroup
		calculateStats bool
		useTabAlert    bool
		expected       *statepb.Grid
	}{
		{
			name:     "empty grid is tolerated",
			grid:     &statepb.Grid{},
			dashCfg:  &configpb.DashboardTab{},
			groupCfg: &configpb.TestGroup{},
			expected: &statepb.Grid{},
		},
		{
			name:     "nil grid is not tolerated",
			dashCfg:  &configpb.DashboardTab{},
			groupCfg: &configpb.TestGroup{},
		},
		{
			name: "nil config is not tolerated",
			grid: &statepb.Grid{},
		},
		{
			name: "basic grid",
			grid: buildGrid(t,
				updater.InflatedColumn{
					Column: &statepb.Column{Name: "okay"},
					Cells: map[string]updater.Cell{
						"first":  {Result: tspb.TestStatus_BUILD_PASSED},
						"second": {Result: tspb.TestStatus_BUILD_PASSED},
					},
				},
				updater.InflatedColumn{
					Column: &statepb.Column{Name: "still-ok"},
					Cells: map[string]updater.Cell{
						"first":  {Result: tspb.TestStatus_BUILD_PASSED},
						"second": {Result: tspb.TestStatus_BUILD_PASSED},
					},
				}),
			dashCfg: &configpb.DashboardTab{
				Name: "tab",
			},
			groupCfg: &configpb.TestGroup{},
			expected: &statepb.Grid{
				Columns: []*statepb.Column{
					{Name: "okay"},
					{Name: "still-ok"},
				},
				Rows: []*statepb.Row{
					{
						Name:    "first",
						Id:      "first",
						Results: []int32{int32(tspb.TestStatus_BUILD_PASSED), 2},
					},
					{
						Name:    "second",
						Id:      "second",
						Results: []int32{int32(tspb.TestStatus_BUILD_PASSED), 2},
					},
				},
			},
		},
		{
			name: "Filters out regex tabs",
			grid: buildGrid(t,
				updater.InflatedColumn{
					Column: &statepb.Column{Name: "okay"},
					Cells: map[string]updater.Cell{
						"first": {Result: tspb.TestStatus_BUILD_PASSED},
						"bad":   {Result: tspb.TestStatus_BUILD_PASSED},
					},
				},
				updater.InflatedColumn{
					Column: &statepb.Column{Name: "still-ok"},
					Cells: map[string]updater.Cell{
						"first": {Result: tspb.TestStatus_BUILD_PASSED},
						"bad":   {Result: tspb.TestStatus_BUILD_PASSED},
					},
				}),
			dashCfg: &configpb.DashboardTab{
				Name:        "tab",
				BaseOptions: "exclude-filter-by-regex=bad",
			},
			groupCfg: &configpb.TestGroup{},
			expected: &statepb.Grid{
				Columns: []*statepb.Column{
					{Name: "okay"},
					{Name: "still-ok"},
				},
				Rows: []*statepb.Row{
					{
						Name:    "first",
						Id:      "first",
						Results: []int32{int32(tspb.TestStatus_BUILD_PASSED), 2},
					},
				},
			},
		},
		{
			name: "Filters out regex tabs, and drops empty columns",
			grid: buildGrid(t,
				updater.InflatedColumn{
					Column: &statepb.Column{Name: "okay"},
					Cells: map[string]updater.Cell{
						"first": {Result: tspb.TestStatus_BUILD_PASSED},
					},
				},
				updater.InflatedColumn{
					Column: &statepb.Column{Name: "weird"},
					Cells: map[string]updater.Cell{
						"bad": {Result: tspb.TestStatus_BUILD_PASSED},
					},
				},
				updater.InflatedColumn{
					Column: &statepb.Column{Name: "still-ok"},
					Cells: map[string]updater.Cell{
						"first": {Result: tspb.TestStatus_BUILD_PASSED},
					},
				}),
			dashCfg: &configpb.DashboardTab{
				Name:        "tab",
				BaseOptions: "exclude-filter-by-regex=bad",
			},
			groupCfg: &configpb.TestGroup{},
			expected: &statepb.Grid{
				Columns: []*statepb.Column{
					{Name: "okay"},
					{Name: "still-ok"},
				},
				Rows: []*statepb.Row{
					{
						Name:    "first",
						Id:      "first",
						Results: []int32{int32(tspb.TestStatus_BUILD_PASSED), 2},
					},
				},
			},
		},
		{
			name: "calculate alerts using test group",
			grid: buildGrid(t,
				updater.InflatedColumn{
					Column: &statepb.Column{Name: "final"},
					Cells: map[string]updater.Cell{
						"okay":   {Result: tspb.TestStatus_BUILD_PASSED},
						"broken": {Result: tspb.TestStatus_BUILD_FAIL},
						"flaky":  {Result: tspb.TestStatus_BUILD_PASSED},
					},
				},
				updater.InflatedColumn{
					Column: &statepb.Column{Name: "middle"},
					Cells: map[string]updater.Cell{
						"okay":   {Result: tspb.TestStatus_BUILD_PASSED},
						"broken": {Result: tspb.TestStatus_BUILD_FAIL},
						"flaky":  {Result: tspb.TestStatus_BUILD_FAIL},
					},
				},
				updater.InflatedColumn{
					Column: &statepb.Column{Name: "initial"},
					Cells: map[string]updater.Cell{
						"okay":   {Result: tspb.TestStatus_BUILD_PASSED},
						"broken": {Result: tspb.TestStatus_BUILD_FAIL},
						"flaky":  {Result: tspb.TestStatus_BUILD_PASSED},
					},
				}),
			dashCfg: &configpb.DashboardTab{},
			groupCfg: &configpb.TestGroup{
				Name:                    "group",
				NumFailuresToAlert:      1,
				NumPassesToDisableAlert: 1,
			},
			useTabAlert: false,
			expected: &statepb.Grid{
				Columns: []*statepb.Column{
					{Name: "final"},
					{Name: "middle"},
					{Name: "initial"},
				},
				Rows: []*statepb.Row{
					{
						Name:    "okay",
						Id:      "okay",
						Results: []int32{int32(tspb.TestStatus_BUILD_PASSED), 3},
					},
					{
						Name:    "broken",
						Id:      "broken",
						Results: []int32{int32(tspb.TestStatus_BUILD_FAIL), 3},
						AlertInfo: &statepb.AlertInfo{
							FailCount: 3,
						},
					},
					{
						Name:    "flaky",
						Id:      "flaky",
						Results: []int32{int32(tspb.TestStatus_BUILD_PASSED), 1, int32(tspb.TestStatus_BUILD_FAIL), 1, int32(tspb.TestStatus_BUILD_PASSED), 1},
					},
				},
			},
		},
		{
			name: "calculate alerts using tab state",
			grid: buildGrid(t,
				updater.InflatedColumn{
					Column: &statepb.Column{Name: "final"},
					Cells: map[string]updater.Cell{
						"okay":   {Result: tspb.TestStatus_BUILD_PASSED},
						"broken": {Result: tspb.TestStatus_BUILD_FAIL},
						"flaky":  {Result: tspb.TestStatus_BUILD_PASSED},
					},
				},
				updater.InflatedColumn{
					Column: &statepb.Column{Name: "middle"},
					Cells: map[string]updater.Cell{
						"okay":   {Result: tspb.TestStatus_BUILD_PASSED},
						"broken": {Result: tspb.TestStatus_BUILD_FAIL},
						"flaky":  {Result: tspb.TestStatus_BUILD_FAIL},
					},
				},
				updater.InflatedColumn{
					Column: &statepb.Column{Name: "initial"},
					Cells: map[string]updater.Cell{
						"okay":   {Result: tspb.TestStatus_BUILD_PASSED},
						"broken": {Result: tspb.TestStatus_BUILD_FAIL},
						"flaky":  {Result: tspb.TestStatus_BUILD_PASSED},
					},
				}),
			dashCfg: &configpb.DashboardTab{
				Name: "tab",
				AlertOptions: &configpb.DashboardTabAlertOptions{
					NumFailuresToAlert:      1,
					NumPassesToDisableAlert: 1,
				},
			},
			groupCfg:    &configpb.TestGroup{},
			useTabAlert: true,
			expected: &statepb.Grid{
				Columns: []*statepb.Column{
					{Name: "final"},
					{Name: "middle"},
					{Name: "initial"},
				},
				Rows: []*statepb.Row{
					{
						Name:    "okay",
						Id:      "okay",
						Results: []int32{int32(tspb.TestStatus_BUILD_PASSED), 3},
					},
					{
						Name:    "broken",
						Id:      "broken",
						Results: []int32{int32(tspb.TestStatus_BUILD_FAIL), 3},
						AlertInfo: &statepb.AlertInfo{
							FailCount: 3,
						},
					},
					{
						Name:    "flaky",
						Id:      "flaky",
						Results: []int32{int32(tspb.TestStatus_BUILD_PASSED), 1, int32(tspb.TestStatus_BUILD_FAIL), 1, int32(tspb.TestStatus_BUILD_PASSED), 1},
					},
				},
			},
		},
		{
			name: "calculate stats",
			grid: buildGrid(t,
				updater.InflatedColumn{
					Column: &statepb.Column{Name: "final"},
					Cells: map[string]updater.Cell{
						"okay":   {Result: tspb.TestStatus_BUILD_PASSED},
						"broken": {Result: tspb.TestStatus_BUILD_FAIL},
						"flaky":  {Result: tspb.TestStatus_BUILD_PASSED},
					},
				},
				updater.InflatedColumn{
					Column: &statepb.Column{Name: "middle"},
					Cells: map[string]updater.Cell{
						"okay":   {Result: tspb.TestStatus_BUILD_PASSED},
						"broken": {Result: tspb.TestStatus_BUILD_FAIL},
						"flaky":  {Result: tspb.TestStatus_BUILD_FAIL},
					},
				},
				updater.InflatedColumn{
					Column: &statepb.Column{Name: "initial"},
					Cells: map[string]updater.Cell{
						"okay":   {Result: tspb.TestStatus_BUILD_PASSED},
						"broken": {Result: tspb.TestStatus_BUILD_FAIL},
						"flaky":  {Result: tspb.TestStatus_BUILD_PASSED},
					},
				}),
			dashCfg: &configpb.DashboardTab{
				Name: "tab",
				AlertOptions: &configpb.DashboardTabAlertOptions{
					NumFailuresToAlert:      1,
					NumPassesToDisableAlert: 1,
				},
				BrokenColumnThreshold: 0.5,
			},
			groupCfg:       &configpb.TestGroup{},
			useTabAlert:    true,
			calculateStats: true,
			expected: &statepb.Grid{
				Columns: []*statepb.Column{
					{
						Name: "final",
						Stats: &statepb.Stats{
							PassCount:  2,
							FailCount:  1,
							TotalCount: 3,
						},
					},
					{
						Name: "middle",
						Stats: &statepb.Stats{
							PassCount:  1,
							FailCount:  2,
							TotalCount: 3,
							Broken:     true,
						},
					},
					{
						Name: "initial",
						Stats: &statepb.Stats{
							PassCount:  2,
							FailCount:  1,
							TotalCount: 3,
						},
					},
				},
				Rows: []*statepb.Row{
					{
						Name:    "okay",
						Id:      "okay",
						Results: []int32{int32(tspb.TestStatus_BUILD_PASSED), 3},
					},
					{
						Name:    "broken",
						Id:      "broken",
						Results: []int32{int32(tspb.TestStatus_BUILD_FAIL), 3},
						AlertInfo: &statepb.AlertInfo{
							FailCount: 3,
						},
					},
					{
						Name:    "flaky",
						Id:      "flaky",
						Results: []int32{int32(tspb.TestStatus_BUILD_PASSED), 1, int32(tspb.TestStatus_BUILD_FAIL), 1, int32(tspb.TestStatus_BUILD_PASSED), 1},
					},
				},
			},
		},
		{
			name: "calculate stats, no broken threshold",
			grid: buildGrid(t,
				updater.InflatedColumn{
					Column: &statepb.Column{Name: "initial"},
					Cells: map[string]updater.Cell{
						"okay":   {Result: tspb.TestStatus_BUILD_PASSED},
						"broken": {Result: tspb.TestStatus_BUILD_FAIL},
					},
				}),
			dashCfg: &configpb.DashboardTab{
				Name: "tab",
				AlertOptions: &configpb.DashboardTabAlertOptions{
					NumFailuresToAlert:      1,
					NumPassesToDisableAlert: 1,
				},
			},
			groupCfg:       &configpb.TestGroup{},
			useTabAlert:    true,
			calculateStats: true,
			expected: &statepb.Grid{
				Columns: []*statepb.Column{
					{
						Name: "initial",
					},
				},
				Rows: []*statepb.Row{
					{
						Name:    "okay",
						Id:      "okay",
						Results: []int32{int32(tspb.TestStatus_BUILD_PASSED), 1},
					},
					{
						Name:    "broken",
						Id:      "broken",
						Results: []int32{int32(tspb.TestStatus_BUILD_FAIL), 1},
						AlertInfo: &statepb.AlertInfo{
							FailCount: 1,
						},
					},
				},
			},
		},
		{
			name: "calculate stats, calculate stats false",
			grid: buildGrid(t,
				updater.InflatedColumn{
					Column: &statepb.Column{Name: "initial"},
					Cells: map[string]updater.Cell{
						"okay":   {Result: tspb.TestStatus_BUILD_PASSED},
						"broken": {Result: tspb.TestStatus_BUILD_FAIL},
					},
				}),
			dashCfg: &configpb.DashboardTab{
				Name: "tab",
				AlertOptions: &configpb.DashboardTabAlertOptions{
					NumFailuresToAlert:      1,
					NumPassesToDisableAlert: 1,
				},
				BrokenColumnThreshold: 0.5,
			},
			groupCfg:       &configpb.TestGroup{},
			useTabAlert:    true,
			calculateStats: false,
			expected: &statepb.Grid{
				Columns: []*statepb.Column{
					{
						Name: "initial",
					},
				},
				Rows: []*statepb.Row{
					{
						Name:    "okay",
						Id:      "okay",
						Results: []int32{int32(tspb.TestStatus_BUILD_PASSED), 1},
					},
					{
						Name:    "broken",
						Id:      "broken",
						Results: []int32{int32(tspb.TestStatus_BUILD_FAIL), 1},
						AlertInfo: &statepb.AlertInfo{
							FailCount: 1,
						},
					},
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			actual, err := tabulate(ctx, logrus.New(), tc.grid, tc.dashCfg, tc.groupCfg, tc.calculateStats, tc.useTabAlert, nil) // TODO(slchase): add tests for not nil
			if tc.expected == nil {
				if err == nil {
					t.Error("Expected an error, but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			diff := cmp.Diff(actual, tc.expected, protocmp.Transform(),
				protocmp.IgnoreFields(&statepb.Row{}, "cell_ids", "icons", "messages", "user_property", "properties"), // mostly empty
				protocmp.IgnoreFields(&statepb.AlertInfo{}, "fail_time"),                                              // import not needed to determine if alert was set
				protocmp.SortRepeatedFields(&statepb.Grid{}, "rows"))                                                  // rows have no canonical order
			if diff != "" {
				t.Errorf("(-got, +want): %s", diff)
			}
		})
	}
}

func buildGrid(t *testing.T, cols ...updater.InflatedColumn) *statepb.Grid {
	t.Helper()
	var g statepb.Grid
	r := map[string]*statepb.Row{}
	for _, col := range cols {
		updater.AppendColumn(&g, r, col)
	}
	return &g
}

func Test_CreateTabState(t *testing.T) {
	var exampleGrid statepb.Grid
	updater.AppendColumn(&exampleGrid, map[string]*statepb.Row{}, updater.InflatedColumn{
		Column: &statepb.Column{Name: "full"},
		Cells: map[string]updater.Cell{
			"some data": {Result: tspb.TestStatus_BUILD_PASSED},
		},
	})

	testcases := []struct {
		name         string
		state        *statepb.Grid
		confirm      bool
		expectError  bool
		expectUpload bool
	}{
		{
			name:        "Fails if data is missing",
			expectError: true,
		},
		{
			name:        "Does not write without confirm",
			state:       &exampleGrid,
			confirm:     false,
			expectError: false,
		},
		{
			name:         "Writes data when upload is expected",
			state:        &exampleGrid,
			confirm:      true,
			expectUpload: true,
		},
	}

	expectedPath := newPathOrDie("gs://example/prefix/dashboard/tab")
	configPath := newPathOrDie("gs://example/config")

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := fake.UploadClient{
				Uploader: fake.Uploader{},
			}

			task := writeTask{
				dashboard: &configpb.Dashboard{
					Name: "dashboard",
				},
				tab: &configpb.DashboardTab{
					Name: "tab",
				},
				group: &configpb.TestGroup{
					Name: "testgroup",
				},
				data: tc.state,
			}

			err := createTabState(ctx, logrus.New(), client, task, *configPath, "prefix", tc.confirm, true, true, false)
			if tc.expectError == (err == nil) {
				t.Errorf("Wrong error: want %t, got %v", tc.expectError, err)
			}
			res, ok := client.Uploader[*expectedPath]
			uploaded := ok && (len(res.Buf) != 0)
			if uploaded != tc.expectUpload {
				t.Errorf("Wrong upload: want %t, got %v", tc.expectUpload, ok)
			}
		})
	}
}

func Test_MergeGrids(t *testing.T) {
	testcases := []struct {
		name    string
		current []updater.InflatedColumn
		add     []updater.InflatedColumn
		expect  []updater.InflatedColumn
	}{
		{
			name:   "Empty grids",
			expect: []updater.InflatedColumn{},
		},
		{
			name:    "Creating a grid",
			current: []updater.InflatedColumn{},
			add: []updater.InflatedColumn{
				{
					Column: &statepb.Column{
						Name:    "cool results",
						Started: 12345678,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_PASS},
					},
				},
			},
			expect: []updater.InflatedColumn{
				{
					Column: &statepb.Column{
						Name:    "cool results",
						Started: 12345678,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_PASS},
					},
				},
			},
		},
		{
			name: "Merge two results where existing is first",
			current: []updater.InflatedColumn{
				{
					Column: &statepb.Column{
						Name:    "old results",
						Started: 1,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_PASS},
					},
				},
			},
			add: []updater.InflatedColumn{
				{
					Column: &statepb.Column{
						Name:    "cool results",
						Started: 12345678,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_FAIL},
					},
				},
			},
			expect: []updater.InflatedColumn{
				{
					Column: &statepb.Column{
						Name:    "cool results",
						Started: 12345678,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_FAIL},
					},
				},
				{
					Column: &statepb.Column{
						Name:    "old results",
						Started: 1,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_PASS},
					},
				},
			},
		},
		{
			name: "Merge two results where new is first",
			current: []updater.InflatedColumn{
				{
					Column: &statepb.Column{
						Name:    "old results",
						Started: 123,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_PASS},
					},
				},
			},
			add: []updater.InflatedColumn{
				{
					Column: &statepb.Column{
						Name:    "finished results",
						Started: 1,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_FAIL},
					},
				},
			},
			expect: []updater.InflatedColumn{
				{
					Column: &statepb.Column{
						Name:    "old results",
						Started: 123,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_PASS},
					},
				},
				{
					Column: &statepb.Column{
						Name:    "finished results",
						Started: 1,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_FAIL},
					},
				},
			},
		},
		{
			name: "two identical results: merged with new result",
			current: []updater.InflatedColumn{
				{
					Column: &statepb.Column{
						Name:    "result",
						Build:   "build",
						Started: 123,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_RUNNING},
					},
				},
			},
			add: []updater.InflatedColumn{
				{
					Column: &statepb.Column{
						Name:    "result",
						Build:   "build",
						Started: 123,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_PASS},
					},
				},
			},
			expect: []updater.InflatedColumn{
				{
					Column: &statepb.Column{
						Name:    "result",
						Build:   "build",
						Started: 123,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_PASS},
					},
				},
			},
		},
		{
			name: "two similar results (diff build): not merged",
			current: []updater.InflatedColumn{
				{
					Column: &statepb.Column{
						Name:    "test",
						Build:   "building",
						Started: 123,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_PASS},
					},
				},
			},
			add: []updater.InflatedColumn{
				{
					Column: &statepb.Column{
						Name:    "test",
						Build:   "scaffold",
						Started: 123,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_PASS},
					},
				},
			},
			expect: []updater.InflatedColumn{
				{
					Column: &statepb.Column{
						Name:    "test",
						Build:   "building",
						Started: 123,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_PASS},
					},
				},
				{
					Column: &statepb.Column{
						Name:    "test",
						Build:   "scaffold",
						Started: 123,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_PASS},
					},
				},
			},
		},
		{
			name: "two similar results (diff name): not merged",
			current: []updater.InflatedColumn{
				{
					Column: &statepb.Column{
						Name:    "current",
						Build:   "build",
						Started: 123,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_PASS},
					},
				},
			},
			add: []updater.InflatedColumn{
				{
					Column: &statepb.Column{
						Name:    "add",
						Build:   "build",
						Started: 123,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_PASS},
					},
				},
			},
			expect: []updater.InflatedColumn{
				{
					Column: &statepb.Column{
						Name:    "current",
						Build:   "build",
						Started: 123,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_PASS},
					},
				},
				{
					Column: &statepb.Column{
						Name:    "add",
						Build:   "build",
						Started: 123,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_PASS},
					},
				},
			},
		},
		{
			name: "adds new info: keeps historical data",
			current: []updater.InflatedColumn{
				{
					Column: &statepb.Column{
						Name:    "fourth",
						Started: 1234,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_PASS},
					},
				},
				{
					Column: &statepb.Column{
						Name:    "third",
						Started: 123,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_PASS},
					},
				},
				{
					Column: &statepb.Column{
						Name:    "second",
						Started: 12,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_PASS},
					},
				},
				{
					Column: &statepb.Column{
						Name:    "first",
						Started: 1,
					},
					Cells: map[string]updater.Cell{
						"cell": {
							Result: tspb.TestStatus_UNKNOWN,
							Icon:   "...",
						},
					},
				},
			},
			add: []updater.InflatedColumn{
				{
					Column: &statepb.Column{
						Name:    "sixth",
						Started: 123456,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_PASS},
					},
				},
				{
					Column: &statepb.Column{
						Name:    "fifth",
						Started: 12345,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_PASS},
					},
				},
				{
					Column: &statepb.Column{
						Name:    "fourth",
						Started: 1234,
					},
					Cells: map[string]updater.Cell{
						"cell": {
							Result: tspb.TestStatus_UNKNOWN,
							Icon:   "...",
						},
					},
				},
			},
			expect: []updater.InflatedColumn{
				{
					Column: &statepb.Column{
						Name:    "sixth",
						Started: 123456,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_PASS},
					},
				},
				{
					Column: &statepb.Column{
						Name:    "fifth",
						Started: 12345,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_PASS},
					},
				},
				{
					Column: &statepb.Column{
						Name:    "fourth",
						Started: 1234,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_PASS},
					},
				},
				{
					Column: &statepb.Column{
						Name:    "third",
						Started: 123,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_PASS},
					},
				},
				{
					Column: &statepb.Column{
						Name:    "second",
						Started: 12,
					},
					Cells: map[string]updater.Cell{
						"cell": {Result: tspb.TestStatus_PASS},
					},
				},
				{
					Column: &statepb.Column{
						Name:    "first",
						Started: 1,
					},
					Cells: map[string]updater.Cell{
						"cell": {
							Result: tspb.TestStatus_UNKNOWN,
							Icon:   "...",
						},
					},
				},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			actual := mergeGrids(test.current, test.add)
			if diff := cmp.Diff(actual, test.expect, protocmp.Transform()); diff != "" {
				t.Errorf("Unexpected (-got, +want): %s", diff)
				t.Logf("Got %#v", actual)
			}
		})
	}

}
