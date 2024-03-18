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
	"flag"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"cloud.google.com/go/storage"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	statuspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
)

// TODO(fejta): rename everything to InflatedColumn
type inflatedColumn = InflatedColumn

// TODO(fejta): rename everything to Cell
type cell = Cell

func blank(n int) []string {
	var out []string
	for i := 0; i < n; i++ {
		out = append(out, "")
	}
	return out
}

var benchPath gcs.Path

func init() {
	flag.Var(&benchPath, "bench-path", "Path to ./foo/local-state or gs://bucket/grid/test-group")
}

func BenchmarkInflateGrid(b *testing.B) {
	ctx := context.Background()
	if benchPath.Object() == "" {
		b.Skip("No grid-path specified")
	}
	var storageClient *storage.Client
	if benchPath.Bucket() != "" {
		s, err := gcs.ClientWithCreds(context.Background())
		if err != nil {
			b.Fatalf("Cannot create GCS client: %v", err)
		}
		storageClient = s
	}
	client := gcs.NewClient(storageClient)
	grid, _, err := gcs.DownloadGrid(ctx, client, benchPath)
	if err != nil {
		b.Fatalf("Failed to download %s: %v", benchPath, err)
	}
	latest := time.Now().Add(time.Hour)
	earliest := time.Unix(0, 0)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			InflateGrid(ctx, grid, earliest, latest)
		}
	})
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
		name       string
		ctx        context.Context
		grid       *statepb.Grid
		earliest   time.Time
		latest     time.Time
		expected   []inflatedColumn
		wantIssues map[string][]string
		err        bool
	}{
		{
			name: "basically works",
			grid: &statepb.Grid{},
		},
		{
			name: "preserve column data",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			grid: &statepb.Grid{
				Columns: []*statepb.Column{
					{
						Build:      "build",
						Hint:       "xyzpdq",
						Name:       "name",
						Started:    5,
						Extra:      []string{"extra", "fun"},
						HotlistIds: "hot topic",
					},
					{
						Build:      "second build", // Also becomes Hint
						Name:       "second name",
						Started:    10,
						Extra:      []string{"more", "gooder"},
						HotlistIds: "hot pocket",
					},
				},
			},
			err: true,
		},
		{
			name: "preserve column data",
			grid: &statepb.Grid{
				Columns: []*statepb.Column{
					{
						Build:      "build",
						Hint:       "xyzpdq",
						Name:       "name",
						Started:    5,
						Extra:      []string{"extra", "fun"},
						HotlistIds: "hot topic",
					},
					{
						Build:      "second build", // Also becomes Hint
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
					Column: &statepb.Column{
						Build:      "build",
						Hint:       "xyzpdq",
						Name:       "name",
						Started:    5,
						Extra:      []string{"extra", "fun"},
						HotlistIds: "hot topic",
					},
					Cells: map[string]cell{},
				},
				{
					Column: &statepb.Column{
						Build:      "second build",
						Hint:       "second build",
						Name:       "second name",
						Started:    10,
						Extra:      []string{"more", "gooder"},
						HotlistIds: "hot pocket",
					},
					Cells: map[string]cell{},
				},
			},
		},
		{
			name: "preserve row data",
			grid: &statepb.Grid{
				Columns: []*statepb.Column{
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
				Rows: []*statepb.Row{
					{
						Name: "name",
						Results: []int32{
							int32(statuspb.TestStatus_FAIL), 2,
						},
						CellIds:      []string{"this", "that"},
						Messages:     []string{"important", "notice"},
						Icons:        []string{"I1", "I2"},
						Metric:       []string{"this", "that"},
						UserProperty: []string{"hello", "there"},
						Metrics: []*statepb.Metric{
							{
								Indices: []int32{0, 2}, // both columns
								Values:  []float64{0.1, 0.2},
							},
							{
								Name:    "override",
								Indices: []int32{1, 1}, // only second
								Values:  []float64{1.1},
							},
						},
						Issues: []string{"fun", "times"},
					},
					{
						Name: "second",
						Results: []int32{
							int32(statuspb.TestStatus_PASS), 2,
						},
						CellIds:      blank(2),
						Messages:     blank(2),
						Icons:        blank(2),
						Metric:       blank(2),
						UserProperty: blank(2),
					},
					{
						Name: "sparse",
						Results: []int32{
							int32(statuspb.TestStatus_NO_RESULT), 1,
							int32(statuspb.TestStatus_FLAKY), 1,
						},
						CellIds:      []string{"that-sparse"},
						Messages:     []string{"notice-sparse"},
						Icons:        []string{"I2-sparse"},
						UserProperty: []string{"there-sparse"},
					},
					{
						Name:   "issued",
						Issues: []string{"three", "4"},
					},
				},
			},
			latest: hours[23],
			expected: []inflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "b1",
						Hint:    "b1",
						Name:    "n1",
						Started: 1,
					},
					Cells: map[string]cell{
						"name": {
							Result:  statuspb.TestStatus_FAIL,
							CellID:  "this",
							Message: "important",
							Icon:    "I1",
							Metrics: map[string]float64{
								"this": 0.1,
							},
							UserProperty: "hello",
						},
						"second": {
							Result: statuspb.TestStatus_PASS,
						},
						"sparse": {},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "b2",
						Hint:    "b2",
						Name:    "n2",
						Started: 2,
					},
					Cells: map[string]cell{
						"name": {
							Result:  statuspb.TestStatus_FAIL,
							CellID:  "that",
							Message: "notice",
							Icon:    "I2",
							Metrics: map[string]float64{
								"this":     0.2,
								"override": 1.1,
							},
							UserProperty: "there",
						},
						"second": {
							Result: statuspb.TestStatus_PASS,
						},
						"sparse": {
							Result:       statuspb.TestStatus_FLAKY,
							CellID:       "that-sparse",
							Message:      "notice-sparse",
							Icon:         "I2-sparse",
							UserProperty: "there-sparse",
						},
					},
				},
			},
			wantIssues: map[string][]string{
				"issued": {"three", "4"},
				"name":   {"fun", "times"},
			},
		},
		{
			name: "drop latest columns",
			grid: &statepb.Grid{
				Columns: []*statepb.Column{
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
				Rows: []*statepb.Row{
					{
						Name:     "hello",
						CellIds:  blank(4),
						Messages: blank(4),
						Icons:    blank(4),
						Results: []int32{
							int32(statuspb.TestStatus_RUNNING), 1,
							int32(statuspb.TestStatus_PASS), 1,
							int32(statuspb.TestStatus_FAIL), 1,
							int32(statuspb.TestStatus_FLAKY), 1,
						},
					},
					{
						Name:     "world",
						CellIds:  blank(4),
						Messages: blank(4),
						Icons:    blank(4),
						Results: []int32{
							int32(statuspb.TestStatus_PASS_WITH_SKIPS), 4,
						},
					},
				},
			},
			latest: hours[20],
			expected: []inflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "keep1",
						Hint:    "keep1",
						Started: millis(hours[20]) + 999,
					},
					Cells: map[string]cell{
						"hello": {Result: statuspb.TestStatus_FAIL},
						"world": {Result: statuspb.TestStatus_PASS_WITH_SKIPS},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "keep2",
						Hint:    "keep2",
						Started: millis(hours[10]),
					},
					Cells: map[string]cell{
						"hello": {Result: statuspb.TestStatus_FLAKY},
						"world": {Result: statuspb.TestStatus_PASS_WITH_SKIPS},
					},
				},
			},
		},
		{
			name: "unsorted", // drop old and new
			grid: &statepb.Grid{
				Columns: []*statepb.Column{
					{
						Build:   "current1",
						Started: millis(hours[20]),
					},
					{
						Build:   "old1",
						Started: millis(hours[10]) - 1,
					},
					{
						Build:   "new1",
						Started: millis(hours[22]),
					},
					{
						Build:   "current3",
						Started: millis(hours[19]),
					},
					{
						Build:   "new2",
						Started: millis(hours[23]),
					},
					{
						Build:   "old2",
						Started: millis(hours[0]),
					},
					{
						Build:   "current2",
						Started: millis(hours[10]),
					},
				},
				Rows: []*statepb.Row{
					{
						Name:     "hello",
						CellIds:  blank(7),
						Messages: blank(7),
						Icons:    blank(7),
						Results: []int32{
							int32(statuspb.TestStatus_RUNNING), 1,
							int32(statuspb.TestStatus_PASS), 2,
							int32(statuspb.TestStatus_FAIL), 1,
							int32(statuspb.TestStatus_PASS), 2,
							int32(statuspb.TestStatus_FLAKY), 1,
						},
					},
					{
						Name:     "world",
						CellIds:  blank(7),
						Messages: blank(7),
						Icons:    blank(7),
						Results: []int32{
							int32(statuspb.TestStatus_PASS_WITH_SKIPS), 7,
						},
					},
				},
			},
			latest:   hours[21],
			earliest: hours[10],
			expected: []inflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "current1",
						Hint:    "current1",
						Started: millis(hours[20]),
					},
					Cells: map[string]cell{
						"hello": {Result: statuspb.TestStatus_RUNNING},
						"world": {Result: statuspb.TestStatus_PASS_WITH_SKIPS},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "current3",
						Hint:    "current3",
						Started: millis(hours[19]),
					},
					Cells: map[string]cell{
						"hello": {Result: statuspb.TestStatus_FAIL},
						"world": {Result: statuspb.TestStatus_PASS_WITH_SKIPS},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "current2",
						Hint:    "current2",
						Started: millis(hours[10]),
					},
					Cells: map[string]cell{
						"hello": {Result: statuspb.TestStatus_FLAKY},
						"world": {Result: statuspb.TestStatus_PASS_WITH_SKIPS},
					},
				},
			},
		},
		{
			name: "drop old columns",
			grid: &statepb.Grid{
				Columns: []*statepb.Column{
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
				Rows: []*statepb.Row{
					{
						Name:     "hello",
						CellIds:  blank(4),
						Messages: blank(4),
						Icons:    blank(4),
						Results: []int32{
							int32(statuspb.TestStatus_RUNNING), 1,
							int32(statuspb.TestStatus_PASS), 1,
							int32(statuspb.TestStatus_FAIL), 1,
							int32(statuspb.TestStatus_FLAKY), 1,
						},
					},
					{
						Name:     "world",
						CellIds:  blank(4),
						Messages: blank(4),
						Icons:    blank(4),
						Results: []int32{
							int32(statuspb.TestStatus_PASS_WITH_SKIPS), 4,
						},
					},
				},
			},
			latest:   hours[23],
			earliest: hours[10],
			expected: []inflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "current1",
						Hint:    "current1",
						Started: millis(hours[20]),
					},
					Cells: map[string]cell{
						"hello": {Result: statuspb.TestStatus_RUNNING},
						"world": {Result: statuspb.TestStatus_PASS_WITH_SKIPS},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "current2",
						Hint:    "current2",
						Started: millis(hours[10]),
					},
					Cells: map[string]cell{
						"hello": {Result: statuspb.TestStatus_PASS},
						"world": {Result: statuspb.TestStatus_PASS_WITH_SKIPS},
					},
				},
			},
		},
		{
			name: "keep newest old column when none newer",
			grid: &statepb.Grid{
				Columns: []*statepb.Column{
					{
						Build:   "drop-latest1",
						Started: millis(hours[23]),
					},
					{
						Build:   "keep-old1",
						Started: millis(hours[10]) - 1,
					},
					{
						Build:   "drop-old2",
						Started: millis(hours[0]),
					},
				},
				Rows: []*statepb.Row{
					{
						Name:     "hello",
						CellIds:  blank(4),
						Messages: blank(4),
						Icons:    blank(4),
						Results: []int32{
							int32(statuspb.TestStatus_RUNNING), 1,
							int32(statuspb.TestStatus_FAIL), 1,
							int32(statuspb.TestStatus_FLAKY), 1,
						},
					},
					{
						Name:     "world",
						CellIds:  blank(4),
						Messages: blank(4),
						Icons:    blank(4),
						Results: []int32{
							int32(statuspb.TestStatus_PASS_WITH_SKIPS), 3,
						},
					},
				},
			},
			latest:   hours[20],
			earliest: hours[10],
			expected: []inflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "keep-old1",
						Hint:    "keep-old1",
						Started: millis(hours[10]) - 1,
					},
					Cells: map[string]cell{
						"hello": {Result: statuspb.TestStatus_FAIL},
						"world": {Result: statuspb.TestStatus_PASS_WITH_SKIPS},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantIssues == nil {
				tc.wantIssues = map[string][]string{}
			}
			if tc.ctx == nil {
				tc.ctx = context.Background()
			}
			actual, issues, err := InflateGrid(tc.ctx, tc.grid, tc.earliest, tc.latest)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("InflatedGrid() got unexpected error: %v", err)
				}
			case tc.err:
				t.Error("InflateGrid() failed to return an error")
			default:
				if diff := cmp.Diff(tc.expected, actual, cmp.AllowUnexported(inflatedColumn{}, cell{}), protocmp.Transform()); diff != "" {
					t.Errorf("InflateGrid() got unexpected diff (-want +got):\n%s", diff)
				}
				if diff := cmp.Diff(tc.wantIssues, issues); diff != "" {
					t.Errorf("InflateGrid() got unexpected issue diff (-want +got):\n%s", diff)
				}
			}
		})

	}
}

func TestInflateRow(t *testing.T) {
	cases := []struct {
		name     string
		row      *statepb.Row
		expected []cell
	}{
		{
			name: "basically works",
		},
		{
			name: "preserve cell ids",
			row: &statepb.Row{
				CellIds:  []string{"cell-a", "cell-b", "cell-d"},
				Icons:    blank(3),
				Messages: blank(3),
				Results: []int32{
					int32(statuspb.TestStatus_PASS), 2,
					int32(statuspb.TestStatus_NO_RESULT), 1,
					int32(statuspb.TestStatus_PASS), 1,
					int32(statuspb.TestStatus_NO_RESULT), 1,
				},
			},
			expected: []cell{
				{
					Result: statuspb.TestStatus_PASS,
					CellID: "cell-a",
				},
				{
					Result: statuspb.TestStatus_PASS,
					CellID: "cell-b",
				},
				{
					Result: statuspb.TestStatus_NO_RESULT,
				},
				{
					Result: statuspb.TestStatus_PASS,
					CellID: "cell-d",
				},
				{
					Result: statuspb.TestStatus_NO_RESULT,
				},
			},
		},
		{
			name: "only finished columns contain icons and messages",
			row: &statepb.Row{
				CellIds: blank(8),
				Icons: []string{
					"F1", "~1", "~2",
				},
				Messages: []string{
					"fail", "flake-first", "flake-second",
				},
				Results: []int32{
					int32(statuspb.TestStatus_NO_RESULT), 2,
					int32(statuspb.TestStatus_FAIL), 1,
					int32(statuspb.TestStatus_NO_RESULT), 2,
					int32(statuspb.TestStatus_FLAKY), 2,
					int32(statuspb.TestStatus_NO_RESULT), 1,
				},
			},
			expected: []cell{
				{},
				{},
				{
					Result:  statuspb.TestStatus_FAIL,
					Icon:    "F1",
					Message: "fail",
				},
				{},
				{},
				{
					Result:  statuspb.TestStatus_FLAKY,
					Icon:    "~1",
					Message: "flake-first",
				},
				{
					Result:  statuspb.TestStatus_FLAKY,
					Icon:    "~2",
					Message: "flake-second",
				},
				{},
			},
		},
		{
			name: "find metric name from row when missing",
			row: &statepb.Row{
				CellIds:  blank(1),
				Icons:    blank(1),
				Messages: blank(1),
				Results: []int32{
					int32(statuspb.TestStatus_PASS), 1,
				},
				Metric: []string{"found-it"},
				Metrics: []*statepb.Metric{
					{
						Indices: []int32{0, 1},
						Values:  []float64{7},
					},
				},
			},
			expected: []cell{
				{
					Result: statuspb.TestStatus_PASS,
					Metrics: map[string]float64{
						"found-it": 7,
					},
				},
			},
		},
		{
			name: "prioritize local metric name",
			row: &statepb.Row{
				CellIds:  blank(1),
				Icons:    blank(1),
				Messages: blank(1),
				Results: []int32{
					int32(statuspb.TestStatus_PASS), 1,
				},
				Metric: []string{"ignore-this"},
				Metrics: []*statepb.Metric{
					{
						Name:    "oh yeah",
						Indices: []int32{0, 1},
						Values:  []float64{7},
					},
				},
			},
			expected: []cell{
				{
					Result: statuspb.TestStatus_PASS,
					Metrics: map[string]float64{
						"oh yeah": 7,
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var actual []cell
			nextCell := inflateRow(tc.row)
			for c := nextCell(); c != nil; c = nextCell() {
				actual = append(actual, *c)
			}

			if diff := cmp.Diff(actual, tc.expected, cmp.AllowUnexported(cell{}), protocmp.Transform()); diff != "" {
				t.Errorf("inflateRow() got unexpected diff (-have, +want):\n%s", diff)
			}
		})
	}
}

func TestInflateMetric(t *testing.T) {
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
			metric := &statepb.Metric{
				Name:    tc.name,
				Indices: tc.indices,
				Values:  tc.values,
			}
			nextMetric := inflateMetric(metric)
			for val, ok := nextMetric(); ok; val, ok = nextMetric() {
				actual = append(actual, val)
			}

			if diff := cmp.Diff(tc.expected, actual); diff != "" {
				t.Errorf("inflateMetric(%v) got unexpected diff (-want +got):\n%s", metric, diff)
			}
		})
	}
}

func TestInflateResults(t *testing.T) {
	cases := []struct {
		name     string
		results  []int32
		expected []statuspb.TestStatus
	}{
		{
			name: "basically works",
		},
		{
			name: "first documented example with multiple values works",
			results: []int32{
				int32(statuspb.TestStatus_NO_RESULT), 3,
				int32(statuspb.TestStatus_PASS), 4,
			},
			expected: []statuspb.TestStatus{
				statuspb.TestStatus_NO_RESULT,
				statuspb.TestStatus_NO_RESULT,
				statuspb.TestStatus_NO_RESULT,
				statuspb.TestStatus_PASS,
				statuspb.TestStatus_PASS,
				statuspb.TestStatus_PASS,
				statuspb.TestStatus_PASS,
			},
		},
		{
			name: "first item is the type",
			results: []int32{
				int32(statuspb.TestStatus_RUNNING), 1, // RUNNING == 4
			},
			expected: []statuspb.TestStatus{
				statuspb.TestStatus_RUNNING,
			},
		},
		{
			name: "second item is the number of repetitions",
			results: []int32{
				int32(statuspb.TestStatus_PASS), 4, // Running == 1
			},
			expected: []statuspb.TestStatus{
				statuspb.TestStatus_PASS,
				statuspb.TestStatus_PASS,
				statuspb.TestStatus_PASS,
				statuspb.TestStatus_PASS,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nextResult := inflateResults(tc.results)
			var actual []statuspb.TestStatus
			for cur := nextResult(); cur != nil; cur = nextResult() {
				actual = append(actual, *cur)
			}
			if !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("inflateResults(%v) got %v, want %v", tc.results, actual, tc.expected)
			}
		})
	}
}
