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
	"bytes"
	"compress/zlib"
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
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
		name     string
		grid     *statepb.Grid
		dashCfg  *configpb.DashboardTab
		dropCols bool
		expected *statepb.Grid
	}{
		{
			name:     "empty grid is tolerated",
			grid:     &statepb.Grid{},
			dashCfg:  &configpb.DashboardTab{},
			expected: &statepb.Grid{},
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
			name: "Filters out regex tabs, but does not yet drop columns",
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
			dropCols: false,
			expected: &statepb.Grid{
				Columns: []*statepb.Column{
					{Name: "okay"},
					{Name: "weird"},
					{Name: "still-ok"},
				},
				Rows: []*statepb.Row{
					{
						Name:    "first",
						Id:      "first",
						Results: []int32{int32(tspb.TestStatus_BUILD_PASSED), 1, int32(tspb.TestStatus_NO_RESULT), 1, int32(tspb.TestStatus_BUILD_PASSED), 1},
					},
				},
			},
		},
		{
			name: "Filters out regex tabs, dropping empty columns when asked",
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
			dropCols: true,
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
			name: "copies alerts while not dropping empty columns",
			grid: func() *statepb.Grid {
				var g statepb.Grid
				r := map[string]*statepb.Row{}
				updater.AppendColumn(&g, r, updater.InflatedColumn{
					Column: &statepb.Column{Name: "result"},
					Cells: map[string]updater.Cell{
						"bad": {Result: tspb.TestStatus_BUILD_FAIL},
					},
				})
				g.Rows[0].AlertInfo = &statepb.AlertInfo{
					FailCount: 999,
				}
				return &g
			}(),
			dashCfg: &configpb.DashboardTab{
				Name: "tab",
			},
			dropCols: false,
			expected: &statepb.Grid{
				Columns: []*statepb.Column{
					{Name: "result"},
				},
				Rows: []*statepb.Row{
					{
						Name:    "bad",
						Id:      "bad",
						Results: []int32{int32(tspb.TestStatus_BUILD_FAIL), 1},
						AlertInfo: &statepb.AlertInfo{
							FailCount: 999,
						},
					},
				},
			},
		},
		{
			name: "calculate alerts after dropping empty columns",
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
			dropCols: true,
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
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			actual, err := tabulate(ctx, logrus.New(), tc.grid, tc.dashCfg, &configpb.TestGroup{}, tc.dropCols)
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

	tabConfig := configpb.DashboardTab{
		Name: "no filters",
	}

	testcases := []struct {
		name          string
		existingState fake.Object
		confirm       bool
		expectError   bool
		expectUpload  bool
	}{
		{
			name:        "Fails if data is missing",
			expectError: true,
		},
		{
			name: "Does not write without confirm",
			existingState: func() fake.Object {
				return fake.Object{
					Data: string(compress(gridBuf(&exampleGrid))),
				}
			}(),
			confirm:     false,
			expectError: false,
		},
		{
			name: "Fails with uncompressed grid",
			existingState: func() fake.Object {
				return fake.Object{
					Data: string(gridBuf(&exampleGrid)),
				}
			}(),
			expectError: true,
		},
		{
			name: "Writes data when upload is expected",
			existingState: func() fake.Object {
				return fake.Object{
					Data: string(compress(gridBuf(&exampleGrid))),
				}
			}(),
			confirm:      true,
			expectUpload: true,
		},
	}

	fromPath := newPathOrDie("gs://example/from")
	toPath := newPathOrDie("gs://example/to")

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := fake.UploadClient{
				Client: fake.Client{
					Opener: fake.Opener{
						*fromPath: tc.existingState,
					},
				},
				Uploader: fake.Uploader{},
			}

			err := createTabState(ctx, logrus.New(), client, &tabConfig, &configpb.TestGroup{}, *fromPath, *toPath, tc.confirm, true)
			if tc.expectError == (err == nil) {
				t.Errorf("Wrong error: want %t, got %v", tc.expectError, err)
			}
			res, ok := client.Uploader[*toPath]
			uploaded := ok && (len(res.Buf) != 0)
			if uploaded != tc.expectUpload {
				t.Errorf("Wrong upload: want %t, got %v", tc.expectUpload, ok)
			}
		})
	}
}

func gridBuf(grid *statepb.Grid) []byte {
	buf, err := proto.Marshal(grid)
	if err != nil {
		panic(err)
	}
	return buf
}

func compress(buf []byte) []byte {
	var zbuf bytes.Buffer
	zw := zlib.NewWriter(&zbuf)
	if _, err := zw.Write(buf); err != nil {
		panic(err)
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	return zbuf.Bytes()
}
