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

// A script to quickly check two TestGrid state.protos do not wildly differ.
// Assume that if the column and row names are approx. equivalent, the state
// is probably reasonable.

package main

import (
	"context"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/testgrid/util/gcs"

	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
)

func newPathOrDie(s string) gcs.Path {
	p, err := gcs.NewPath(s)
	if err != nil {
		panic(err)
	}
	return *p
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name        string
		first       gcs.Path
		second      gcs.Path
		diffRatioOK float64
		err         bool
	}{
		{
			name:   "empty paths, error",
			first:  newPathOrDie(""),
			second: newPathOrDie(""),
			err:    true,
		},
		{
			name:   "empty first path, error",
			first:  newPathOrDie(""),
			second: newPathOrDie("gs://path/to/second"),
			err:    true,
		},
		{
			name:   "empty second path, error",
			first:  newPathOrDie("gs://path/to/first"),
			second: newPathOrDie(""),
			err:    true,
		},
		{
			name:   "paths basically work",
			first:  newPathOrDie("gs://path/to/first"),
			second: newPathOrDie("gs://path/to/second"),
		},
		{
			name:        "reject negative ratio",
			first:       newPathOrDie("gs://path/to/first"),
			second:      newPathOrDie("gs://path/to/second"),
			diffRatioOK: -0.5,
			err:         true,
		},
		{
			name:        "reject ratio over 1.0",
			first:       newPathOrDie("gs://path/to/first"),
			second:      newPathOrDie("gs://path/to/second"),
			diffRatioOK: 1.5,
			err:         true,
		},
		{
			name:        "ratio basically works",
			first:       newPathOrDie("gs://path/to/first"),
			second:      newPathOrDie("gs://path/to/second"),
			diffRatioOK: 0.5,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opt := options{
				first:       tc.first,
				second:      tc.second,
				diffRatioOK: tc.diffRatioOK,
			}
			err := opt.validate()
			if tc.err && err == nil {
				t.Fatalf("validate() (%v), expected error but got none", opt)
			}
			if !tc.err {
				if err != nil {
					t.Fatalf("validate() (%v) got unexpected error %v", opt, err)
				}
				// Also check that paths end in a '/'.
				if !strings.HasSuffix(opt.first.String(), "/") {
					t.Errorf("first path %q should end in '/'.", tc.first.String())
				}
				if !strings.HasSuffix(opt.second.String(), "/") {
					t.Errorf("second path %q should end in '/'.", tc.second.String())
				}
			}
		})
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		name        string
		first       *statepb.Grid
		second      *statepb.Grid
		diffRatioOK float64
		diffed      bool
	}{
		{
			name:   "nil grids, same",
			diffed: false,
		},
		{
			name:   "empty grids, same",
			first:  &statepb.Grid{},
			second: &statepb.Grid{},
			diffed: false,
		},
		{
			name: "basic grids, same",
			first: &statepb.Grid{
				Rows: []*statepb.Row{
					{Name: "row1"},
					{Name: "row2"},
				},
				Columns: []*statepb.Column{
					{Build: "col1"},
					{Build: "col2"},
				},
			},
			second: &statepb.Grid{
				Rows: []*statepb.Row{
					{Name: "row1"},
					{Name: "row2"},
				},
				Columns: []*statepb.Column{
					{Build: "col1"},
					{Build: "col2"},
				},
			},
			diffed: false,
		},
		{
			name: "different rows, diff",
			first: &statepb.Grid{
				Rows: []*statepb.Row{
					{Name: "row1"},
				},
				Columns: []*statepb.Column{
					{Build: "col1"},
					{Build: "col2"},
				},
			},
			second: &statepb.Grid{
				Rows: []*statepb.Row{
					{Name: "row2"},
				},
				Columns: []*statepb.Column{
					{Build: "col1"},
					{Build: "col2"},
				},
			},
			diffed: true,
		},
		{
			name: "different rows lower than ratio, same",
			first: &statepb.Grid{
				Rows: []*statepb.Row{
					{Name: "row1"},
					{Name: "row2"},
				},
				Columns: []*statepb.Column{
					{Build: "col1"},
					{Build: "col2"},
				},
			},
			second: &statepb.Grid{
				Rows: []*statepb.Row{
					{Name: "row1"},
				},
				Columns: []*statepb.Column{
					{Build: "col1"},
					{Build: "col2"},
				},
			},
			diffRatioOK: 0.6,
		},
		{
			name: "different columns, diff",
			first: &statepb.Grid{
				Rows: []*statepb.Row{
					{Name: "row1"},
					{Name: "row2"},
				},
				Columns: []*statepb.Column{
					{Build: "col1"},
					{Build: "col2"},
				},
			},
			second: &statepb.Grid{
				Rows: []*statepb.Row{
					{Name: "row1"},
					{Name: "row2"},
				},
				Columns: []*statepb.Column{
					{Build: "col3"},
					{Build: "col4"},
				},
			},
			diffed: true,
		},
		{
			name: "different columns lower than ratio, same",
			first: &statepb.Grid{
				Rows: []*statepb.Row{
					{Name: "row1"},
					{Name: "row2"},
				},
				Columns: []*statepb.Column{
					{Build: "col1"},
					{Build: "col2"},
				},
			},
			second: &statepb.Grid{
				Rows: []*statepb.Row{
					{Name: "row1"},
					{Name: "row2"},
				},
				Columns: []*statepb.Column{
					{Build: "col1"},
				},
			},
			diffRatioOK: 0.6,
			diffed:      false,
		},
		{
			name: "different grids, diff",
			first: &statepb.Grid{
				Rows: []*statepb.Row{
					{Name: "row1"},
					{Name: "row2"},
				},
				Columns: []*statepb.Column{
					{Build: "col1"},
					{Build: "col2"},
				},
			},
			second: &statepb.Grid{
				Rows: []*statepb.Row{
					{Name: "row1"},
				},
				Columns: []*statepb.Column{
					{Build: "col1"},
				},
			},
			diffed: true,
		},
		{
			name: "different grids lower than ratio, same",
			first: &statepb.Grid{
				Rows: []*statepb.Row{
					{Name: "row1"},
					{Name: "row2"},
				},
				Columns: []*statepb.Column{
					{Build: "col1"},
					{Build: "col2"},
				},
			},
			second: &statepb.Grid{
				Rows: []*statepb.Row{
					{Name: "row1"},
				},
				Columns: []*statepb.Column{
					{Build: "col1"},
				},
			},
			diffRatioOK: 0.6,
			diffed:      false,
		},
		{
			name: "diff cols but empty rows, same",
			first: &statepb.Grid{
				Rows: []*statepb.Row{},
				Columns: []*statepb.Column{
					{Build: "col1"},
				},
			},
			second: &statepb.Grid{
				Rows: []*statepb.Row{},
				Columns: []*statepb.Column{
					{Build: "col1"},
					{Build: "col2"},
				},
			},
			diffed: false,
		},
		{
			name: "diff rows but empty cols, same",
			first: &statepb.Grid{
				Rows: []*statepb.Row{
					{Name: "row1"},
					{Name: "row2"},
				},
				Columns: []*statepb.Column{},
			},
			second: &statepb.Grid{
				Rows: []*statepb.Row{
					{Name: "row1"},
				},
				Columns: []*statepb.Column{},
			},
			diffed: false,
		},
		{
			name: "first empty, second has one column, same",
			first: &statepb.Grid{
				Rows:    []*statepb.Row{},
				Columns: []*statepb.Column{},
			},
			second: &statepb.Grid{
				Rows: []*statepb.Row{
					{Name: "row1"},
					{Name: "row2"},
					{Name: "row3"},
				},
				Columns: []*statepb.Column{
					{Build: "col1"},
				},
			},
			diffed: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if diffed, _, _ := compare(ctx, tc.first, tc.second, tc.diffRatioOK, 5); diffed != tc.diffed {
				t.Errorf("compare(%s, %s) not as expected; got %t, want %t", tc.first, tc.second, diffed, tc.diffed)
			}
		})
	}
}

func TestReportDiff(t *testing.T) {
	cases := []struct {
		name        string
		first       map[string]int
		second      map[string]int
		diffRatioOK float64
		diffed      bool
		reasons     diffReasons
	}{
		{
			name:   "nil",
			diffed: false,
		},
		{
			name: "same",
			first: map[string]int{
				"some": 1,
				"more": 2,
			},
			second: map[string]int{
				"some": 1,
				"more": 2,
			},
			diffed: false,
		},
		{
			name: "different",
			first: map[string]int{
				"some": 1,
				"more": 1,
			},
			second: map[string]int{
				"hi":    1,
				"hello": 1,
				"yay":   1,
			},
			diffed:  true,
			reasons: diffReasons{other: true},
		},
		{
			name: "different above ratio",
			first: map[string]int{
				"some": 1,
				"more": 1,
			},
			second: map[string]int{
				"some": 1,
				"more": 1,
				"hi":   1,
			},
			diffRatioOK: 0.3,
			diffed:      true,
			reasons:     diffReasons{other: true},
		},
		{
			name: "different below ratio",
			first: map[string]int{
				"some": 1,
				"more": 1,
			},
			second: map[string]int{
				"some": 1,
				"more": 1,
				"hi":   1,
			},
			diffRatioOK: 0.6,
			diffed:      false,
			reasons:     diffReasons{},
		},
		{
			name: "first has duplicates",
			first: map[string]int{
				"some": 3,
				"more": 5,
			},
			second: map[string]int{
				"some": 1,
				"more": 1,
			},
			diffed:  true,
			reasons: diffReasons{firstHasDuplicates: true},
		},
		{
			name: "second has duplicates",
			first: map[string]int{
				"some": 1,
				"more": 1,
			},
			second: map[string]int{
				"some": 3,
				"more": 5,
			},
			diffed:  true,
			reasons: diffReasons{secondHasDuplicates: true},
		},
		{
			name: "first has duplicates below ratio",
			first: map[string]int{
				"some": 1,
				"more": 2,
			},
			second: map[string]int{
				"some": 1,
				"more": 1,
			},
			diffRatioOK: 0.6,
			diffed:      false,
			reasons:     diffReasons{},
		},
		{
			name: "second has duplicates below ratio",
			first: map[string]int{
				"some": 1,
				"more": 1,
			},
			second: map[string]int{
				"some": 1,
				"more": 2,
			},
			diffRatioOK: 0.6,
			diffed:      false,
			reasons:     diffReasons{},
		},
		{
			name: "first has duplicates above ratio",
			first: map[string]int{
				"some": 2,
				"more": 2,
			},
			second: map[string]int{
				"some": 1,
				"more": 1,
			},
			diffRatioOK: 0.3,
			diffed:      true,
			reasons:     diffReasons{firstHasDuplicates: true},
		},
		{
			name: "second has duplicates above ratio",
			first: map[string]int{
				"some": 1,
				"more": 1,
			},
			second: map[string]int{
				"some": 2,
				"more": 2,
			},
			diffRatioOK: 0.3,
			diffed:      true,
			reasons:     diffReasons{secondHasDuplicates: true},
		},
		{
			name: "second has duplicates and differences above ratio",
			first: map[string]int{
				"some": 1,
				"more": 1,
			},
			second: map[string]int{
				"some": 2,
				"more": 2,
				"hi":   5,
			},
			diffRatioOK: 0.3,
			diffed:      true,
			reasons:     diffReasons{other: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diffed, reasons := reportDiff(tc.first, tc.second, "thing", tc.diffRatioOK)
			if diffed != tc.diffed {
				t.Errorf("reportDiff(%v, %v, %f) diffed wrong; got %t, want %t", tc.first, tc.second, tc.diffRatioOK, diffed, tc.diffed)
			}
			if reasons != tc.reasons {
				t.Errorf("reportDiff(%v, %v, %f) has wrong diff reasons; got %t, want %t", tc.first, tc.second, tc.diffRatioOK, reasons, tc.reasons)
			}
		})
	}
}

func TestCompareKeys(t *testing.T) {
	cases := []struct {
		name        string
		first       map[string]int
		second      map[string]int
		diffRatioOK float64
		diffed      bool
	}{
		{
			name:   "nil",
			diffed: false,
		},
		{
			name:   "empty",
			first:  map[string]int{},
			second: map[string]int{},
			diffed: false,
		},
		{
			name: "maps match",
			first: map[string]int{
				"some": 1,
				"more": 2,
			},
			second: map[string]int{
				"some": 1,
				"more": 2,
			},
			diffed: false,
		},
		{
			name: "non-matching keys",
			first: map[string]int{
				"some": 5,
				"more": 3,
			},
			second: map[string]int{
				"hi":    1,
				"hello": 1,
				"other": 1,
			},
			diffed: true,
		},
		{
			name: "duplicate keys",
			first: map[string]int{
				"some": 5,
				"more": 3,
			},
			second: map[string]int{
				"some": 1,
				"more": 1,
			},
			diffed: false,
		},
		{
			name: "mostly duplicate keys below ratio",
			first: map[string]int{
				"some": 5,
				"more": 3,
			},
			second: map[string]int{
				"some": 1,
				"more": 1,
				"hi":   1,
			},
			diffRatioOK: 0.6,
			diffed:      false,
		},
		{
			name: "mostly duplicate keys above ratio",
			first: map[string]int{
				"some": 5,
				"more": 3,
			},
			second: map[string]int{
				"some": 1,
				"more": 1,
				"hi":   1,
			},
			diffRatioOK: 0.3,
			diffed:      true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if diffed := compareKeys(tc.first, tc.second, tc.diffRatioOK); diffed != tc.diffed {
				t.Errorf("compareKeys(%v, %v, %f) not as expected; got %t, want %t", tc.first, tc.second, tc.diffRatioOK, diffed, tc.diffed)
			}
		})
	}
}
