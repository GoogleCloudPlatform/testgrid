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
	"net/url"
	"reflect"
	"testing"

	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	statuspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestFilterGrid(t *testing.T) {
	cases := []struct {
		name        string
		baseOptions string
		rows        []*statepb.Row
		expected    []*statepb.Row
		expectError bool
	}{
		{
			name: "basically works",
		},
		{
			name:        "bad options returns error",
			baseOptions: "%z",
			expectError: true,
		},
		{
			name: "everything works",
			baseOptions: url.Values{
				includeFilter: []string{"foo"},
				excludeFilter: []string{"bar"},
			}.Encode(),
			rows: []*statepb.Row{
				{
					Name:    "include-food",
					Results: []int32{int32(statuspb.TestStatus_PASS), 10},
				},
				{
					Name:    "exclude-included-bart",
					Results: []int32{int32(statuspb.TestStatus_PASS), 10},
				},
				{
					Name: "ignore-included-stale",
					Results: []int32{
						int32(statuspb.TestStatus_NO_RESULT), 5,
						int32(statuspb.TestStatus_PASS_WITH_SKIPS), 10,
					},
				},
			},
			expected: []*statepb.Row{
				{
					Name:    "include-food",
					Results: []int32{int32(statuspb.TestStatus_PASS), 10},
				},
			},
		},
		{
			name: "must match all includes",
			baseOptions: url.Values{
				includeFilter: []string{"foo", "spam"},
			}.Encode(),
			rows: []*statepb.Row{
				{
					Name: "spam-is-food",
				},
				{
					Name: "spam-musubi",
				},
				{
					Name: "unagi-food",
				},
				{
					Name: "email-spam-is-not-food",
				},
			},
			expected: []*statepb.Row{
				{
					Name: "spam-is-food",
				},
				{
					Name: "email-spam-is-not-food",
				},
			},
		},
		{
			name: "exclude any exclusions",
			baseOptions: url.Values{
				excludeFilter: []string{"not", "nope"},
			}.Encode(),
			rows: []*statepb.Row{
				{
					Name: "yes please",
				},
				{
					Name: "leslie knope",
				},
				{
					Name: "not eating",
				},
				{
					Name: "fluffy waffles",
				},
			},
			expected: []*statepb.Row{
				{
					Name: "yes please",
				},
				{
					Name: "fluffy waffles",
				},
			},
		},
		{
			name: "bad inclusion regexp errors",
			baseOptions: url.Values{
				includeFilter: []string{"this.("},
			}.Encode(),
			expectError: true,
		},
		{
			name: "bad exclude regexp errors",
			baseOptions: url.Values{
				excludeFilter: []string{"this.("},
			}.Encode(),
			expectError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, r := range tc.rows {
				if r.Results == nil {
					r.Results = []int32{int32(statuspb.TestStatus_PASS), 100}
				}
			}
			for _, r := range tc.expected {
				if r.Results == nil {
					r.Results = []int32{int32(statuspb.TestStatus_PASS), 100}
				}
			}
			actual, err := filterGrid(tc.baseOptions, tc.rows)
			if !tc.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tc.expectError && err == nil {
				t.Error("failed to return an error")
			}
			if diff := cmp.Diff(tc.expected, actual, protocmp.Transform()); diff != "" {
				t.Errorf("(-want, +got): %s", diff)
			}
		})
	}
}

func TestIncludeRows(t *testing.T) {
	cases := []struct {
		name     string
		names    []string
		include  string
		expected []string
		err      bool
	}{
		{
			name: "basically works",
		},
		{
			name:    "bad regex errors",
			include: "^[a-z",
			err:     true,
		},
		{
			name:    "return nothing rows when nothing matches",
			names:   []string{"hello", "world"},
			include: "dog",
		},
		{
			name:     "include only matching rows",
			include:  "fun",
			names:    []string{"apply", "function", "to", "funny", "bone"},
			expected: []string{"function", "funny"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rows []*statepb.Row
			for _, n := range tc.names {
				rows = append(rows, &statepb.Row{Name: n})
			}
			actualRows, err := includeRows(rows, tc.include)
			var actual []string
			for _, r := range actualRows {
				actual = append(actual, r.Name)
			}
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Error("failed to return expected error")
			case !reflect.DeepEqual(actual, tc.expected):
				t.Errorf("actual %s != expected %s", actual, tc.expected)
			}
		})
	}
}

func TestExcludeRows(t *testing.T) {
	cases := []struct {
		name     string
		names    []string
		exclude  string
		expected []string
		err      bool
	}{
		{
			name: "basically works",
		},
		{
			name:    "bad regex errors",
			exclude: "^[a-z",
			err:     true,
		},
		{
			name:     "return all rows when nothing matches",
			names:    []string{"hello", "world"},
			exclude:  "dog",
			expected: []string{"hello", "world"},
		},
		{
			name:     "drop matching rows",
			exclude:  "fun",
			names:    []string{"apply", "function", "to", "funny", "bone"},
			expected: []string{"apply", "to", "bone"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rows []*statepb.Row
			for _, n := range tc.names {
				rows = append(rows, &statepb.Row{Name: n})
			}
			actualRows, err := excludeRows(rows, tc.exclude)
			var actual []string
			for _, r := range actualRows {
				actual = append(actual, r.Name)
			}
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Error("failed to return expected error")
			case !reflect.DeepEqual(actual, tc.expected):
				t.Errorf("actual %s != expected %s", actual, tc.expected)
			}
		})
	}

}
