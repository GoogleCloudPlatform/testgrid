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
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"strings"
	"testing"

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
					{Name: "col1"},
					{Name: "col2"},
				},
			},
			second: &statepb.Grid{
				Rows: []*statepb.Row{
					{Name: "row1"},
					{Name: "row2"},
				},
				Columns: []*statepb.Column{
					{Name: "col1"},
					{Name: "col2"},
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
					{Name: "col1"},
					{Name: "col2"},
				},
			},
			second: &statepb.Grid{
				Rows: []*statepb.Row{
					{Name: "row2"},
				},
				Columns: []*statepb.Column{
					{Name: "col1"},
					{Name: "col2"},
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
					{Name: "col1"},
					{Name: "col2"},
				},
			},
			second: &statepb.Grid{
				Rows: []*statepb.Row{
					{Name: "row1"},
				},
				Columns: []*statepb.Column{
					{Name: "col1"},
					{Name: "col2"},
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
					{Name: "col1"},
				},
			},
			second: &statepb.Grid{
				Rows: []*statepb.Row{
					{Name: "row1"},
					{Name: "row2"},
				},
				Columns: []*statepb.Column{
					{Name: "col2"},
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
					{Name: "col1"},
					{Name: "col2"},
				},
			},
			second: &statepb.Grid{
				Rows: []*statepb.Row{
					{Name: "row1"},
					{Name: "row2"},
				},
				Columns: []*statepb.Column{
					{Name: "col1"},
				},
			},
			diffRatioOK: 0.6,
		},
		{
			name: "different grids, diff",
			first: &statepb.Grid{
				Rows: []*statepb.Row{
					{Name: "row1"},
					{Name: "row2"},
				},
				Columns: []*statepb.Column{
					{Name: "col1"},
					{Name: "col2"},
				},
			},
			second: &statepb.Grid{
				Rows: []*statepb.Row{
					{Name: "row1"},
				},
				Columns: []*statepb.Column{
					{Name: "col1"},
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
					{Name: "col1"},
					{Name: "col2"},
				},
			},
			second: &statepb.Grid{
				Rows: []*statepb.Row{
					{Name: "row1"},
				},
				Columns: []*statepb.Column{
					{Name: "col1"},
				},
			},
			diffRatioOK: 0.6,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if diffed := compare(ctx, tc.first, tc.second, tc.diffRatioOK, false); diffed != tc.diffed {
				t.Errorf("compare(%s, %s) not as expected; got %t, want %t")
			}
		})
	}
}
