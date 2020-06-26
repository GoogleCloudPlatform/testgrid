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

package summarizer

import (
	"reflect"
	"testing"

	"github.com/GoogleCloudPlatform/testgrid/pb/state"
	"github.com/GoogleCloudPlatform/testgrid/pkg/summarizer/common"
)

func TestIsWithinTimeFrame(t *testing.T) {
	cases := []struct {
		name      string
		column    *state.Column
		startTime int
		endTime   int
		expected  bool
	}{
		{
			name: "column within time frame returns true",
			column: &state.Column{
				Started: 1.0,
			},
			startTime: 0,
			endTime:   2,
			expected:  true,
		},
		{
			name: "column before time frame returns false",
			column: &state.Column{
				Started: 1.0,
			},
			startTime: 3,
			endTime:   7,
			expected:  false,
		},
		{
			name: "column after time frame returns false",
			column: &state.Column{
				Started: 4.0,
			},
			startTime: 0,
			endTime:   2,
			expected:  false,
		},
		{
			name: "function is inclusive with column at start time",
			column: &state.Column{
				Started: 0.0,
			},
			startTime: 0,
			endTime:   2,
			expected:  true,
		},
		{
			name: "function is inclusive with column at end time",
			column: &state.Column{
				Started: 2.0,
			},
			startTime: 0,
			endTime:   2,
			expected:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual := isWithinTimeFrame(tc.column, tc.startTime, tc.endTime); actual != tc.expected {
				t.Errorf("isWithinTimeFrame returned %t for %d < %f <= %d", actual, tc.startTime, tc.column.Started, tc.endTime)
			}
		})
	}
}

func TestParseGrid(t *testing.T) {
	cases := []struct {
		name      string
		grid      *state.Grid
		startTime int
		endTime   int
		expected  []*common.GridMetrics
	}{
		{
			name: "grid with all analyzed result types produces correct result list",
			grid: &state.Grid{
				Columns: []*state.Column{
					{Started: 0},
					{Started: 1},
					{Started: 2},
					{Started: 2},
				},
				Rows: []*state.Row{
					{
						Name: "test_1",
						Results: []int32{
							state.Row_Result_value["PASS"], 1,
							state.Row_Result_value["FAIL"], 1,
							state.Row_Result_value["FLAKY"], 1,
							state.Row_Result_value["FAIL"], 1,
						},
						Messages: []string{
							"",
							"",
							"",
							"infra_fail_1",
						},
					},
				},
			},
			startTime: 0,
			endTime:   2,
			expected: []*common.GridMetrics{
				{
					Name:             "test_1",
					Passed:           1,
					Failed:           1,
					FlakyCount:       1,
					AverageFlakiness: 50.0,
					FailedInfraCount: 1,
					InfraFailures: map[string]int{
						"infra_fail_1": 1,
					},
				},
			},
		},
		{
			name: "grid with no analyzed results produces empty result list",
			grid: &state.Grid{
				Columns: []*state.Column{
					{Started: -1},
					{Started: 1},
					{Started: 2},
					{Started: 2},
				},
				Rows: []*state.Row{
					{
						Name: "test_1",
						Results: []int32{
							state.Row_Result_value["NO_RESULT"], 4,
						},
						Messages: []string{
							"this message should not show up in results_0",
							"this message should not show up in results_1",
							"this message should not show up in results_2",
							"this message should not show up in results_3",
						},
					},
				},
			},
			startTime: 0,
			endTime:   2,
			expected:  []*common.GridMetrics{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual := parseGrid(tc.grid, tc.startTime, tc.endTime); !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("\nactual %+v \n!= \nexpected %+v", actual[0], tc.expected[0])
			}
		})
	}
}

func TestCategorizeFailure(t *testing.T) {
	cases := []struct {
		name     string
		message  string
		expected *common.GridMetrics
	}{
		{
			name:    "typical matched string increments counts correctly",
			message: "whatever_valid_word_character_string_with_no_spaces",
			expected: &common.GridMetrics{
				FailedInfraCount: 1,
				InfraFailures: map[string]int{
					"whatever_valid_word_character_string_with_no_spaces": 1,
				},
			},
		},
		{
			name:    "unmatched string increments Failed and not other counts",
			message: "message with spaces should no get matched",
			expected: &common.GridMetrics{
				Failed:        1,
				InfraFailures: map[string]int{},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkMetric := &common.GridMetrics{}
			checkMetric.InfraFailures = map[string]int{}
			categorizeFailure(checkMetric, tc.message)
			if !reflect.DeepEqual(checkMetric, tc.expected) {
				t.Errorf("actual: %+v != expected: %+v", checkMetric, tc.expected)
			}
		})
	}
}
