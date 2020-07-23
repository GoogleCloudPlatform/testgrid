/*
Copyright 2020 The TestGrid Authors.

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
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/pb/state"
)

func blank(n int) []string {
	var out []string
	for i := 0; i < n; i++ {
		out = append(out, "")
	}
	return out
}

func TestInflateGrid(t *testing.T) {
	var hours []time.Time
	when := time.Now().Round(time.Hour)
	for i := 0; i < 24; i++ {
		hours = append(hours, when)
		when = when.Add(time.Hour)
	}

	millis := func(t time.Time) float64 {
		return float64(t.Unix() * 1000)
	}

	cases := []struct {
		name     string
		grid     state.Grid
		earliest time.Time
		latest   time.Time
		expected []inflatedColumn
	}{
		{
			name: "basically works",
		},
		{
			name: "preserve column data",
			grid: state.Grid{
				Columns: []*state.Column{
					{
						Build:      "build",
						Name:       "name",
						Started:    5,
						Extra:      []string{"extra", "fun"},
						HotlistIds: "hot topic",
					},
					{
						Build:      "second build",
						Name:       "second name",
						Started:    10,
						Extra:      []string{"more", "gooder"},
						HotlistIds: "hot pocket",
					},
				},
			},
			latest: hours[23],
			expected: []inflatedColumn{
				{
					column: &state.Column{
						Build:      "build",
						Name:       "name",
						Started:    5,
						Extra:      []string{"extra", "fun"},
						HotlistIds: "hot topic",
					},
					cells: map[string]cell{},
				},
				{
					column: &state.Column{
						Build:      "second build",
						Name:       "second name",
						Started:    10,
						Extra:      []string{"more", "gooder"},
						HotlistIds: "hot pocket",
					},
					cells: map[string]cell{},
				},
			},
		},
		{
			name: "preserve row data",
			grid: state.Grid{
				Columns: []*state.Column{
					{
						Build:   "b1",
						Name:    "n1",
						Started: 1,
					},
					{
						Build:   "b2",
						Name:    "n2",
						Started: 2,
					},
				},
				Rows: []*state.Row{
					{
						Name: "name",
						Results: []int32{
							int32(state.Row_FAIL), 2,
						},
						CellIds:  []string{"this", "that"},
						Messages: []string{"important", "notice"},
						Icons:    []string{"I1", "I2"},
						Metric:   []string{"this", "that"},
						Metrics: []*state.Metric{
							{
								Indices: []int32{0, 2},
								Values:  []float64{0.1, 0.2},
							},
							{
								Name:    "override",
								Indices: []int32{1, 1},
								Values:  []float64{1.1},
							},
						},
					},
					{
						Name: "second",
						Results: []int32{
							int32(state.Row_PASS), 2,
						},
						CellIds:  blank(2),
						Messages: blank(2),
						Icons:    blank(2),
						Metric:   blank(2),
					},
				},
			},
			latest: hours[23],
			expected: []inflatedColumn{
				{
					column: &state.Column{
						Build:   "b1",
						Name:    "n1",
						Started: 1,
					},
					cells: map[string]cell{
						"name": {
							result:  state.Row_FAIL,
							cellID:  "this",
							message: "important",
							icon:    "I1",
							metrics: map[string]float64{
								"this": 0.1,
							},
						},
						"second": {
							result: state.Row_PASS,
						},
					},
				},
				{
					column: &state.Column{
						Build:   "b2",
						Name:    "n2",
						Started: 2,
					},
					cells: map[string]cell{
						"name": {
							result:  state.Row_FAIL,
							cellID:  "that",
							message: "notice",
							icon:    "I2",
							metrics: map[string]float64{
								"this":     0.2,
								"override": 1.1,
							},
						},
						"second": {
							result: state.Row_PASS,
						},
					},
				},
			},
		},
		{
			name: "drop latest columns",
			grid: state.Grid{
				Columns: []*state.Column{
					{
						Build:   "latest1",
						Started: millis(hours[23]),
					},
					{
						Build:   "latest2",
						Started: millis(hours[20]) + 1000,
					},
					{
						Build:   "keep1",
						Started: millis(hours[20]) + 999,
					},
					{
						Build:   "keep2",
						Started: millis(hours[10]),
					},
				},
				Rows: []*state.Row{
					{
						Name:     "hello",
						CellIds:  blank(4),
						Messages: blank(4),
						Icons:    blank(4),
						Results: []int32{
							int32(state.Row_RUNNING), 1,
							int32(state.Row_PASS), 1,
							int32(state.Row_FAIL), 1,
							int32(state.Row_FLAKY), 1,
						},
					},
					{
						Name:     "world",
						CellIds:  blank(4),
						Messages: blank(4),
						Icons:    blank(4),
						Results: []int32{
							int32(state.Row_PASS_WITH_SKIPS), 4,
						},
					},
				},
			},
			latest: hours[20],
			expected: []inflatedColumn{
				{
					column: &state.Column{
						Build:   "keep1",
						Started: millis(hours[20]) + 999,
					},
					cells: map[string]cell{
						"hello": {result: state.Row_FAIL},
						"world": {result: state.Row_PASS_WITH_SKIPS},
					},
				},
				{
					column: &state.Column{
						Build:   "keep2",
						Started: millis(hours[10]),
					},
					cells: map[string]cell{
						"hello": {result: state.Row_FLAKY},
						"world": {result: state.Row_PASS_WITH_SKIPS},
					},
				},
			},
		},
		{
			name: "drop old columns",
			grid: state.Grid{
				Columns: []*state.Column{
					{
						Build:   "current1",
						Started: millis(hours[20]),
					},
					{
						Build:   "current2",
						Started: millis(hours[10]),
					},
					{
						Build:   "old1",
						Started: millis(hours[10]) - 1,
					},
					{
						Build:   "old2",
						Started: millis(hours[0]),
					},
				},
				Rows: []*state.Row{
					{
						Name:     "hello",
						CellIds:  blank(4),
						Messages: blank(4),
						Icons:    blank(4),
						Results: []int32{
							int32(state.Row_RUNNING), 1,
							int32(state.Row_PASS), 1,
							int32(state.Row_FAIL), 1,
							int32(state.Row_FLAKY), 1,
						},
					},
					{
						Name:     "world",
						CellIds:  blank(4),
						Messages: blank(4),
						Icons:    blank(4),
						Results: []int32{
							int32(state.Row_PASS_WITH_SKIPS), 4,
						},
					},
				},
			},
			latest:   hours[23],
			earliest: hours[10],
			expected: []inflatedColumn{
				{
					column: &state.Column{
						Build:   "current1",
						Started: millis(hours[20]),
					},
					cells: map[string]cell{
						"hello": {result: state.Row_RUNNING},
						"world": {result: state.Row_PASS_WITH_SKIPS},
					},
				},
				{
					column: &state.Column{
						Build:   "current2",
						Started: millis(hours[10]),
					},
					cells: map[string]cell{
						"hello": {result: state.Row_PASS},
						"world": {result: state.Row_PASS_WITH_SKIPS},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := inflateGrid(&tc.grid, tc.earliest, tc.latest)
			if !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("inflateGrid(%v) got %s, want %s", tc.grid, actual, tc.expected)
			}
		})

	}
}

func TestInflateRow(t *testing.T) {
	cases := []struct {
		name     string
		row      state.Row
		expected []cell
	}{
		{
			name: "basically works",
		},
		{
			name: "preserve cell ids",
			row: state.Row{
				CellIds:  []string{"cell-a", "cell-b"},
				Icons:    blank(2),
				Messages: blank(2),
				Results: []int32{
					int32(state.Row_PASS), 2,
				},
			},
			expected: []cell{
				{
					result: state.Row_PASS,
					cellID: "cell-a",
				},
				{
					result: state.Row_PASS,
					cellID: "cell-b",
				},
			},
		},
		{
			name: "only finished columns contain icons and messages",
			row: state.Row{
				CellIds: blank(8),
				Icons: []string{
					"F1", "~1", "~2",
				},
				Messages: []string{
					"fail", "flake-first", "flake-second",
				},
				Results: []int32{
					int32(state.Row_NO_RESULT), 2,
					int32(state.Row_FAIL), 1,
					int32(state.Row_NO_RESULT), 2,
					int32(state.Row_FLAKY), 2,
					int32(state.Row_NO_RESULT), 1,
				},
			},
			expected: []cell{
				{},
				{},
				{
					result:  state.Row_FAIL,
					icon:    "F1",
					message: "fail",
				},
				{},
				{},
				{
					result:  state.Row_FLAKY,
					icon:    "~1",
					message: "flake-first",
				},
				{
					result:  state.Row_FLAKY,
					icon:    "~2",
					message: "flake-second",
				},
				{},
			},
		},
		{
			name: "find metric name from row when missing",
			row: state.Row{
				CellIds:  blank(1),
				Icons:    blank(1),
				Messages: blank(1),
				Results: []int32{
					int32(state.Row_PASS), 1,
				},
				Metric: []string{"found-it"},
				Metrics: []*state.Metric{
					{
						Indices: []int32{0, 1},
						Values:  []float64{7},
					},
				},
			},
			expected: []cell{
				{
					result: state.Row_PASS,
					metrics: map[string]float64{
						"found-it": 7,
					},
				},
			},
		},
		{
			name: "prioritize local metric name",
			row: state.Row{
				CellIds:  blank(1),
				Icons:    blank(1),
				Messages: blank(1),
				Results: []int32{
					int32(state.Row_PASS), 1,
				},
				Metric: []string{"ignore-this"},
				Metrics: []*state.Metric{
					{
						Name:    "oh yeah",
						Indices: []int32{0, 1},
						Values:  []float64{7},
					},
				},
			},
			expected: []cell{
				{
					result: state.Row_PASS,
					metrics: map[string]float64{
						"oh yeah": 7,
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var actual []cell
			for r := range inflateRow(context.Background(), &tc.row) {
				actual = append(actual, r)
			}

			if !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("inflateRow(%v) got %v, want %v", tc.row, actual, tc.expected)
			}
		})
	}
}

func TestInflateMetic(t *testing.T) {
	point := func(v float64) *float64 {
		return &v
	}
	cases := []struct {
		name     string
		indices  []int32
		values   []float64
		expected []*float64
	}{
		{
			name: "basically works",
		},
		{
			name:    "documented example with both values and holes works",
			indices: []int32{0, 2, 6, 4},
			values:  []float64{0.1, 0.2, 6.1, 6.2, 6.3, 6.4},
			expected: []*float64{
				point(0.1),
				point(0.2),
				nil,
				nil,
				nil,
				nil,
				point(6.1),
				point(6.2),
				point(6.3),
				point(6.4),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var actual []*float64
			metric := state.Metric{
				Name:    tc.name,
				Indices: tc.indices,
				Values:  tc.values,
			}
			for v := range inflateMetric(context.Background(), &metric) {
				actual = append(actual, v)
			}

			if !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("inflateMetric(%v) got %v want %v", metric, actual, tc.expected)
			}
		})
	}
}

func TestInflateResults(t *testing.T) {
	cases := []struct {
		name     string
		results  []int32
		expected []state.Row_Result
	}{
		{
			name: "basically works",
		},
		{
			name: "first documented example with multiple values works",
			results: []int32{
				int32(state.Row_NO_RESULT), 3,
				int32(state.Row_PASS), 4,
			},
			expected: []state.Row_Result{
				state.Row_NO_RESULT,
				state.Row_NO_RESULT,
				state.Row_NO_RESULT,
				state.Row_PASS,
				state.Row_PASS,
				state.Row_PASS,
				state.Row_PASS,
			},
		},
		{
			name: "first item is the type",
			results: []int32{
				int32(state.Row_RUNNING), 1, // RUNNING == 4
			},
			expected: []state.Row_Result{
				state.Row_RUNNING,
			},
		},
		{
			name: "second item is the number of repetitions",
			results: []int32{
				int32(state.Row_PASS), 4, // Running == 1
			},
			expected: []state.Row_Result{
				state.Row_PASS,
				state.Row_PASS,
				state.Row_PASS,
				state.Row_PASS,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ch := inflateResults(context.Background(), tc.results)
			var actual []state.Row_Result
			for r := range ch {
				actual = append(actual, r)
			}
			if !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("inflateResults(%v) got %v, want %v", tc.results, actual, tc.expected)
			}
		})
	}
}
