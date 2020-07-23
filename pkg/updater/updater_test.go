/*
Copyright 2018 The Kubernetes Authors.

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

package updater

import (
	"reflect"
	"sort"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	_ "github.com/GoogleCloudPlatform/testgrid/metadata/junit"
	"github.com/GoogleCloudPlatform/testgrid/pb/state"
)

func TestUpdate(t *testing.T) {
	// TODO(fejta): add coverage
}

func TestTestGroupPath(t *testing.T) {
	// TODO(fejta): add coverage
}

func TestUpdateGroup(t *testing.T) {
	// TODO(fejta): add coverage
}

func TestConstructGrid(t *testing.T) {
	// TODO(fejta): add coverage
}

func TestMarshalGrid(t *testing.T) {
	g1 := state.Grid{
		Columns: []*state.Column{
			{Build: "alpha"},
			{Build: "second"},
		},
	}
	g2 := state.Grid{
		Columns: []*state.Column{
			{Build: "first"},
			{Build: "second"},
		},
	}

	b1, e1 := marshalGrid(g1)
	b2, e2 := marshalGrid(g2)
	uncompressed, e1a := proto.Marshal(&g1)

	switch {
	case e1 != nil, e2 != nil:
		t.Errorf("unexpected error %v %v %v", e1, e2, e1a)
	}

	if reflect.DeepEqual(b1, b2) {
		t.Errorf("unexpected equality %v == %v", b1, b2)
	}

	if reflect.DeepEqual(b1, uncompressed) {
		t.Errorf("should be compressed but is not: %v", b1)
	}
}

func TestAppendMetric(t *testing.T) {
	cases := []struct {
		name     string
		metric   state.Metric
		idx      int32
		value    float64
		expected state.Metric
	}{
		{
			name: "basically works",
			expected: state.Metric{
				Indices: []int32{0, 1},
				Values:  []float64{0},
			},
		},
		{
			name:  "start metric at random column",
			idx:   7,
			value: 11,
			expected: state.Metric{
				Indices: []int32{7, 1},
				Values:  []float64{11},
			},
		},
		{
			name: "continue existing series",
			metric: state.Metric{
				Indices: []int32{6, 2},
				Values:  []float64{6.1, 6.2},
			},
			idx:   8,
			value: 88,
			expected: state.Metric{
				Indices: []int32{6, 3},
				Values:  []float64{6.1, 6.2, 88},
			},
		},
		{
			name: "start new series",
			metric: state.Metric{
				Indices: []int32{3, 2},
				Values:  []float64{6.1, 6.2},
			},
			idx:   8,
			value: 88,
			expected: state.Metric{
				Indices: []int32{3, 2, 8, 1},
				Values:  []float64{6.1, 6.2, 88},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			appendMetric(&tc.metric, tc.idx, tc.value)
			if diff := cmp.Diff(tc.metric, tc.expected, protocmp.Transform()); diff != "" {
				t.Errorf("appendMetric() got unexpected diff (-got +want):\n%s", diff)
			}
		})
	}
}

func TestAppendCell(t *testing.T) {
	cases := []struct {
		name  string
		row   state.Row
		cell  cell
		count int

		expected state.Row
	}{
		{
			name: "basically works",
			expected: state.Row{
				Results: []int32{0, 0},
			},
		},
		{
			name: "first result",
			cell: cell{
				result: state.Row_PASS,
			},
			count: 1,
			expected: state.Row{
				Results:  []int32{int32(state.Row_PASS), 1},
				CellIds:  []string{""},
				Messages: []string{""},
				Icons:    []string{""},
			},
		},
		{
			name: "all fields filled",
			cell: cell{
				result:  state.Row_PASS,
				cellID:  "cell-id",
				message: "hi",
				icon:    "there",
				metrics: map[string]float64{
					"pi":     3.14,
					"golden": 1.618,
				},
			},
			count: 1,
			expected: state.Row{
				Results:  []int32{int32(state.Row_PASS), 1},
				CellIds:  []string{"cell-id"},
				Messages: []string{"hi"},
				Icons:    []string{"there"},
				Metric: []string{
					"golden",
					"pi",
				},
				Metrics: []*state.Metric{
					{
						Name:    "pi",
						Indices: []int32{0, 1},
						Values:  []float64{3.14},
					},
					{
						Name:    "golden",
						Indices: []int32{0, 1},
						Values:  []float64{1.618},
					},
				},
			},
		},
		{
			name: "append same result",
			row: state.Row{
				Results: []int32{
					int32(state.Row_FLAKY), 3,
				},
				CellIds:  []string{"", "", ""},
				Messages: []string{"", "", ""},
				Icons:    []string{"", "", ""},
			},
			cell: cell{
				result:  state.Row_FLAKY,
				message: "echo",
				cellID:  "again and",
				icon:    "keeps going",
			},
			count: 2,
			expected: state.Row{
				Results:  []int32{int32(state.Row_FLAKY), 5},
				CellIds:  []string{"", "", "", "again and", "again and"},
				Messages: []string{"", "", "", "echo", "echo"},
				Icons:    []string{"", "", "", "keeps going", "keeps going"},
			},
		},
		{
			name: "append different result",
			row: state.Row{
				Results: []int32{
					int32(state.Row_FLAKY), 3,
				},
				CellIds:  []string{"", "", ""},
				Messages: []string{"", "", ""},
				Icons:    []string{"", "", ""},
			},
			cell: cell{
				result: state.Row_PASS,
			},
			count: 2,
			expected: state.Row{
				Results: []int32{
					int32(state.Row_FLAKY), 3,
					int32(state.Row_PASS), 2,
				},
				CellIds:  []string{"", "", "", "", ""},
				Messages: []string{"", "", "", "", ""},
				Icons:    []string{"", "", "", "", ""},
			},
		},
		{
			name: "append no result (results, cellIDs, no messages or icons)",
			row: state.Row{
				Results: []int32{
					int32(state.Row_FLAKY), 3,
				},
				CellIds:  []string{"", "", ""},
				Messages: []string{"", "", ""},
				Icons:    []string{"", "", ""},
			},
			cell: cell{
				result: state.Row_NO_RESULT,
			},
			count: 2,
			expected: state.Row{
				Results: []int32{
					int32(state.Row_FLAKY), 3,
					int32(state.Row_NO_RESULT), 2,
				},
				CellIds:  []string{"", "", "", "", ""},
				Messages: []string{"", "", ""},
				Icons:    []string{"", "", ""},
			},
		},
		{
			name: "add metric to series",
			row: state.Row{
				Results:  []int32{int32(state.Row_PASS), 5},
				CellIds:  []string{"", "", "", "", "c"},
				Messages: []string{"", "", "", "", "m"},
				Icons:    []string{"", "", "", "", "i"},
				Metric: []string{
					"continued-series",
					"new-series",
				},
				Metrics: []*state.Metric{
					{
						Name:    "continued-series",
						Indices: []int32{0, 5},
						Values:  []float64{0, 1, 2, 3, 4},
					},
					{
						Name:    "new-series",
						Indices: []int32{2, 2},
						Values:  []float64{2, 3},
					},
				},
			},
			cell: cell{
				result: state.Row_PASS,
				metrics: map[string]float64{
					"continued-series": 5.1,
					"new-series":       5.2,
				},
			},
			count: 1,
			expected: state.Row{
				Results:  []int32{int32(state.Row_PASS), 6},
				CellIds:  []string{"", "", "", "", "c", ""},
				Messages: []string{"", "", "", "", "m", ""},
				Icons:    []string{"", "", "", "", "i", ""},
				Metric: []string{
					"continued-series",
					"new-series",
				},
				Metrics: []*state.Metric{
					{
						Name:    "continued-series",
						Indices: []int32{0, 6},
						Values:  []float64{0, 1, 2, 3, 4, 5.1},
					},
					{
						Name:    "new-series",
						Indices: []int32{2, 2, 5, 1},
						Values:  []float64{2, 3, 5.2},
					},
				},
			},
		},
		{
			name:  "add a bunch of initial blank columns (eg a deleted row)",
			cell:  emptyCell,
			count: 7,
			expected: state.Row{
				Results: []int32{int32(state.Row_NO_RESULT), 7},
				CellIds: []string{"", "", "", "", "", "", ""},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			appendCell(&tc.row, tc.cell, tc.count)
			sort.SliceStable(tc.row.Metric, func(i, j int) bool {
				return tc.row.Metric[i] < tc.row.Metric[j]
			})
			sort.SliceStable(tc.row.Metrics, func(i, j int) bool {
				return tc.row.Metrics[i].Name < tc.row.Metrics[j].Name
			})
			sort.SliceStable(tc.expected.Metric, func(i, j int) bool {
				return tc.expected.Metric[i] < tc.expected.Metric[j]
			})
			sort.SliceStable(tc.expected.Metrics, func(i, j int) bool {
				return tc.expected.Metrics[i].Name < tc.expected.Metrics[j].Name
			})
			if diff := cmp.Diff(tc.row, tc.expected, protocmp.Transform()); diff != "" {
				t.Errorf("appendCell() got unexpected diff (-got +want):\n%s", diff)
			}
		})
	}
}

func TestAppendColumn(t *testing.T) {
	setupRow := func(row *state.Row, cells ...cell) *state.Row {
		for _, c := range cells {
			appendCell(row, c, 1)
		}
		return row
	}
	cases := []struct {
		name     string
		grid     state.Grid
		col      inflatedColumn
		expected state.Grid
	}{
		{
			name: "append first column",
			col:  inflatedColumn{column: &state.Column{Build: "10"}},
			expected: state.Grid{
				Columns: []*state.Column{
					{Build: "10"},
				},
			},
		},
		{
			name: "append additional column",
			grid: state.Grid{
				Columns: []*state.Column{
					{Build: "10"},
					{Build: "11"},
				},
			},
			col: inflatedColumn{column: &state.Column{Build: "20"}},
			expected: state.Grid{
				Columns: []*state.Column{
					{Build: "10"},
					{Build: "11"},
					{Build: "20"},
				},
			},
		},
		{
			name: "add rows to first column",
			col: inflatedColumn{
				column: &state.Column{Build: "10"},
				cells: map[string]cell{
					"hello": {
						result: state.Row_PASS,
						cellID: "yes",
						metrics: map[string]float64{
							"answer": 42,
						},
					},
					"world": {
						result:  state.Row_FAIL,
						message: "boom",
						icon:    "X",
					},
				},
			},
			expected: state.Grid{
				Columns: []*state.Column{
					{Build: "10"},
				},
				Rows: []*state.Row{
					setupRow(
						&state.Row{
							Name: "hello",
							Id:   "hello",
						},
						cell{
							result:  state.Row_PASS,
							cellID:  "yes",
							metrics: map[string]float64{"answer": 42},
						}),
					setupRow(&state.Row{
						Name: "world",
						Id:   "world",
					}, cell{
						result:  state.Row_FAIL,
						message: "boom",
						icon:    "X",
					}),
				},
			},
		},
		{
			name: "add empty cells",
			grid: state.Grid{
				Columns: []*state.Column{
					{Build: "10"},
					{Build: "11"},
					{Build: "12"},
				},
				Rows: []*state.Row{
					setupRow(
						&state.Row{Name: "deleted"},
						cell{result: state.Row_PASS},
						cell{result: state.Row_PASS},
						cell{result: state.Row_PASS},
					),
					setupRow(
						&state.Row{Name: "always"},
						cell{result: state.Row_PASS},
						cell{result: state.Row_PASS},
						cell{result: state.Row_PASS},
					),
				},
			},
			col: inflatedColumn{
				column: &state.Column{Build: "20"},
				cells: map[string]cell{
					"always": {result: state.Row_PASS},
					"new":    {result: state.Row_PASS},
				},
			},
			expected: state.Grid{
				Columns: []*state.Column{
					{Build: "10"},
					{Build: "11"},
					{Build: "12"},
					{Build: "20"},
				},
				Rows: []*state.Row{
					setupRow(
						&state.Row{Name: "deleted"},
						cell{result: state.Row_PASS},
						cell{result: state.Row_PASS},
						cell{result: state.Row_PASS},
						emptyCell,
					),
					setupRow(
						&state.Row{Name: "always"},
						cell{result: state.Row_PASS},
						cell{result: state.Row_PASS},
						cell{result: state.Row_PASS},
						cell{result: state.Row_PASS},
					),
					setupRow(
						&state.Row{
							Name: "new",
							Id:   "new",
						},
						emptyCell,
						emptyCell,
						emptyCell,
						cell{result: state.Row_PASS},
					),
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := map[string]*state.Row{}
			for _, r := range tc.grid.Rows {
				rows[r.Name] = r
			}
			appendColumn(&tc.grid, rows, tc.col)
			sort.SliceStable(tc.grid.Rows, func(i, j int) bool {
				return tc.grid.Rows[i].Name < tc.grid.Rows[j].Name
			})
			sort.SliceStable(tc.expected.Rows, func(i, j int) bool {
				return tc.expected.Rows[i].Name < tc.expected.Rows[j].Name
			})
			if diff := cmp.Diff(tc.grid, tc.expected, protocmp.Transform()); diff != "" {
				t.Errorf("appendColumn() got unexpected diff (-got +want):\n%s", diff)
			}
		})
	}
}

func TestAlertRow(t *testing.T) {
	var columns []*state.Column
	for i, id := range []string{"a", "b", "c", "d", "e", "f"} {
		columns = append(columns, &state.Column{
			Build:   id,
			Started: 100 - float64(i),
		})
	}
	cases := []struct {
		name      string
		row       state.Row
		failOpen  int
		passClose int
		expected  *state.AlertInfo
	}{
		{
			name: "never alert by default",
			row: state.Row{
				Results: []int32{
					int32(state.Row_FAIL), 6,
				},
			},
		},
		{
			name: "passes do not alert",
			row: state.Row{
				Results: []int32{
					int32(state.Row_PASS), 6,
				},
			},
			failOpen:  1,
			passClose: 3,
		},
		{
			name: "flakes do not alert",
			row: state.Row{
				Results: []int32{
					int32(state.Row_FLAKY), 6,
				},
			},
			failOpen: 1,
		},
		{
			name: "intermittent failures do not alert",
			row: state.Row{
				Results: []int32{
					int32(state.Row_FAIL), 2,
					int32(state.Row_PASS), 1,
					int32(state.Row_FAIL), 2,
				},
			},
			failOpen: 3,
		},
		{
			name: "new failures alert",
			row: state.Row{
				Results: []int32{
					int32(state.Row_FAIL), 3,
					int32(state.Row_PASS), 3,
				},
				Messages: []string{"hello", "no", "no again", "very wrong"},
				CellIds:  []string{"yes", "no", "no again", "very wrong"},
			},
			failOpen: 3,
			expected: alertInfo(3, "hello", "yes", columns[2], columns[3]),
		},
		{
			name: "too few passes do not close",
			row: state.Row{
				Results: []int32{
					int32(state.Row_PASS), 2,
					int32(state.Row_FAIL), 4,
				},
				Messages: []string{"nope", "no", "yay", "very wrong"},
				CellIds:  []string{"wrong", "no", "yep", "very wrong"},
			},
			failOpen:  1,
			passClose: 3,
			expected:  alertInfo(4, "yay", "yep", columns[5], nil),
		},
		{
			name: "flakes do not close",
			row: state.Row{
				Results: []int32{
					int32(state.Row_FLAKY), 2,
					int32(state.Row_FAIL), 4,
				},
				Messages: []string{"nope", "no", "yay", "very wrong"},
				CellIds:  []string{"wrong", "no", "yep", "very wrong"},
			},
			failOpen: 1,
			expected: alertInfo(4, "yay", "yep", columns[5], nil),
		},
		{
			name: "count failures after flaky passes",
			row: state.Row{
				Results: []int32{
					int32(state.Row_FAIL), 1,
					int32(state.Row_FLAKY), 1,
					int32(state.Row_FAIL), 1,
					int32(state.Row_PASS), 1,
					int32(state.Row_FAIL), 2,
				},
				Messages: []string{"nope", "no", "buu", "wrong", "this one"},
				CellIds:  []string{"wrong", "no", "buzz", "wrong2", "good job"},
			},
			failOpen:  2,
			passClose: 2,
			expected:  alertInfo(4, "this one", "good job", columns[5], nil),
		},
		{
			name: "close alert",
			row: state.Row{
				Results: []int32{
					int32(state.Row_PASS), 1,
					int32(state.Row_FAIL), 5,
				},
			},
			failOpen: 1,
		},
		{
			name: "track through empty results",
			row: state.Row{
				Results: []int32{
					int32(state.Row_FAIL), 1,
					int32(state.Row_NO_RESULT), 1,
					int32(state.Row_FAIL), 4,
				},
				Messages: []string{"yay", "no", "buu", "wrong", "nono"},
				CellIds:  []string{"yay-cell", "no", "buzz", "wrong2", "nada"},
			},
			failOpen:  5,
			passClose: 2,
			expected:  alertInfo(5, "yay", "yay-cell", columns[5], nil),
		},
		{
			name: "track passes through empty results",
			row: state.Row{
				Results: []int32{
					int32(state.Row_PASS), 1,
					int32(state.Row_NO_RESULT), 1,
					int32(state.Row_PASS), 1,
					int32(state.Row_FAIL), 3,
				},
			},
			failOpen:  1,
			passClose: 2,
		},
		{
			name: "running cells advance compressed index",
			row: state.Row{
				Results: []int32{
					int32(state.Row_RUNNING), 1,
					int32(state.Row_FAIL), 5,
				},
				Messages: []string{"running0", "fail1-expected", "fail2", "fail3", "fail4", "fail5"},
				CellIds:  []string{"wrong", "yep", "no2", "no3", "no4", "no5"},
			},
			failOpen: 1,
			expected: alertInfo(5, "fail1-expected", "yep", columns[5], nil),
		},
	}

	for _, tc := range cases {
		if actual := alertRow(columns, &tc.row, tc.failOpen, tc.passClose); !reflect.DeepEqual(actual, tc.expected) {
			t.Errorf("%s alert %s != expected %s", tc.name, actual, tc.expected)
		}
	}
}

func TestBuildID(t *testing.T) {
	cases := []struct {
		name     string
		build    string
		extra    string
		expected string
	}{
		{
			name: "return empty by default",
		},
		{
			name:     "favor extra if it exists",
			build:    "wrong",
			extra:    "right",
			expected: "right",
		},
		{
			name:     "build if no extra",
			build:    "yes",
			expected: "yes",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			col := state.Column{
				Build: tc.build,
			}
			if tc.extra != "" {
				col.Extra = append(col.Extra, tc.extra)
			}
			if actual := buildID(&col); actual != tc.expected {
				t.Errorf("%q != expected %q", actual, tc.expected)
			}
		})
	}
}

func TestStamp(t *testing.T) {
	cases := []struct {
		name     string
		col      *state.Column
		expected *timestamp.Timestamp
	}{
		{
			name: "0 returns nil",
		},
		{
			name: "no nanos",
			col: &state.Column{
				Started: 2,
			},
			expected: &timestamp.Timestamp{
				Seconds: 2,
				Nanos:   0,
			},
		},
		{
			name: "has nanos",
			col: &state.Column{
				Started: 1.1,
			},
			expected: &timestamp.Timestamp{
				Seconds: 1,
				Nanos:   1e8,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual := stamp(tc.col); !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("stamp %s != expected stamp %s", actual, tc.expected)
			}
		})
	}
}
