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
	"context"
	"reflect"
	"testing"

	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	summarypb "github.com/GoogleCloudPlatform/testgrid/pb/summary"
	statuspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	"github.com/GoogleCloudPlatform/testgrid/pkg/summarizer/analyzers"
	"github.com/GoogleCloudPlatform/testgrid/pkg/summarizer/common"
	"github.com/golang/protobuf/proto"
)

func TestCalculateTrend(t *testing.T) {
	cases := []struct {
		name                string
		currentHealthiness  *summarypb.HealthinessInfo
		previousHealthiness *summarypb.HealthinessInfo
		expected            *summarypb.HealthinessInfo
	}{
		{
			name: "typical input assigns correct ChangeFromLastInterval's",
			currentHealthiness: &summarypb.HealthinessInfo{
				Tests: []*summarypb.TestInfo{
					{
						DisplayName: "test2_should_be_DOWN",
						Flakiness:   30.0,
					},
					{
						DisplayName: "test1_should_be_UP",
						Flakiness:   70.0,
					},
					{
						DisplayName: "test3_should_be_NO_CHANGE",
						Flakiness:   50.0,
					},
				},
			},
			previousHealthiness: &summarypb.HealthinessInfo{
				Tests: []*summarypb.TestInfo{
					{
						DisplayName: "test1_should_be_UP",
						Flakiness:   50.0,
					},
					{
						DisplayName: "test2_should_be_DOWN",
						Flakiness:   50.0,
					},
					{
						DisplayName: "test3_should_be_NO_CHANGE",
						Flakiness:   50.0,
					},
				},
			},
			expected: &summarypb.HealthinessInfo{
				Tests: []*summarypb.TestInfo{
					{
						DisplayName:            "test2_should_be_DOWN",
						Flakiness:              30.0,
						PreviousFlakiness:      []float32{50.0},
						ChangeFromLastInterval: summarypb.TestInfo_DOWN,
					},
					{
						DisplayName:            "test1_should_be_UP",
						Flakiness:              70.0,
						PreviousFlakiness:      []float32{50.0},
						ChangeFromLastInterval: summarypb.TestInfo_UP,
					},
					{
						DisplayName:            "test3_should_be_NO_CHANGE",
						Flakiness:              50.0,
						PreviousFlakiness:      []float32{50.0},
						ChangeFromLastInterval: summarypb.TestInfo_NO_CHANGE,
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if CalculateTrend(tc.currentHealthiness, tc.previousHealthiness); !proto.Equal(tc.currentHealthiness, tc.expected) {
				for _, expectedTest := range tc.expected.Tests {
					// Linear search because the test cases are so small
					for _, actualTest := range tc.currentHealthiness.Tests {
						if actualTest.DisplayName != expectedTest.DisplayName {
							continue
						}
						actual := actualTest.ChangeFromLastInterval
						expected := expectedTest.ChangeFromLastInterval
						if actual == expected {
							continue
						}

						actualValue := int(actualTest.ChangeFromLastInterval)
						expectedValue := int(expectedTest.ChangeFromLastInterval)
						t.Logf("test: %s has trend of: %s (value: %d) but expected %s (value: %d)",
							actualTest.DisplayName, actual, actualValue, expected, expectedValue)
					}
				}
				t.Fail()
			}
		})
	}
}

func TestGetTrend(t *testing.T) {
	cases := []struct {
		name              string
		currentFlakiness  float32
		previousFlakiness float32
		expected          summarypb.TestInfo_Trend
	}{
		{
			name:              "lower currentFlakiness returns TestInfo_DOWN",
			currentFlakiness:  10.0,
			previousFlakiness: 20.0,
			expected:          summarypb.TestInfo_DOWN,
		},
		{
			name:              "higher currentFlakiness returns TestInfo_UP",
			currentFlakiness:  20.0,
			previousFlakiness: 10.0,
			expected:          summarypb.TestInfo_UP,
		},
		{
			name:              "equal currentFlakiness and previousFlakiness returns TestInfo_NO_CHANGE",
			currentFlakiness:  5.0,
			previousFlakiness: 5.0,
			expected:          summarypb.TestInfo_NO_CHANGE,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual := getTrend(tc.currentFlakiness, tc.previousFlakiness); actual != tc.expected {
				t.Errorf("getTrend returned actual: %d !+ expected: %d for inputs (%f, %f)", actual, tc.expected, tc.currentFlakiness, tc.previousFlakiness)
			}
		})
	}
}

func TestIsWithinTimeFrame(t *testing.T) {
	cases := []struct {
		name      string
		column    *statepb.Column
		startTime int
		endTime   int
		expected  bool
	}{
		{
			name: "column within time frame returns true",
			column: &statepb.Column{
				Started: 1.0,
			},
			startTime: 0,
			endTime:   2,
			expected:  true,
		},
		{
			name: "column before time frame returns false",
			column: &statepb.Column{
				Started: 1.0,
			},
			startTime: 3,
			endTime:   7,
			expected:  false,
		},
		{
			name: "column after time frame returns false",
			column: &statepb.Column{
				Started: 4.0,
			},
			startTime: 0,
			endTime:   2,
			expected:  false,
		},
		{
			name: "function is inclusive with column at start time",
			column: &statepb.Column{
				Started: 0.0,
			},
			startTime: 0,
			endTime:   2,
			expected:  true,
		},
		{
			name: "function is inclusive with column at end time",
			column: &statepb.Column{
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
		name                   string
		grid                   *statepb.Grid
		startTime              int
		endTime                int
		expectedMetrics        []*common.GridMetrics
		expectedFilteredStatus map[string][]analyzers.StatusCategory
	}{
		{
			name: "grid with all analyzed result types produces correct result list",
			grid: &statepb.Grid{
				Columns: []*statepb.Column{
					{Started: 0},
					{Started: 1000},
					{Started: 2000},
					{Started: 2000},
				},
				Rows: []*statepb.Row{
					{
						Name: "test_1",
						Results: []int32{
							statuspb.TestStatus_value["PASS"], 1,
							statuspb.TestStatus_value["FAIL"], 1,
							statuspb.TestStatus_value["FLAKY"], 1,
							statuspb.TestStatus_value["CATEGORIZED_FAIL"], 1,
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
			expectedMetrics: []*common.GridMetrics{
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
			expectedFilteredStatus: map[string][]analyzers.StatusCategory{
				"test_1": {
					analyzers.StatusPass, analyzers.StatusFail, analyzers.StatusFlaky,
				},
			},
		},
		{
			name: "grid with failing columns produces correct status list",
			grid: &statepb.Grid{
				Columns: []*statepb.Column{
					{Started: 0},
					{Started: 1000},
					{Started: 2000},
					{Started: 2000},
				},
				Rows: []*statepb.Row{
					{
						Name: "test_1",
						Results: []int32{
							statuspb.TestStatus_value["PASS"], 1,
							statuspb.TestStatus_value["FAIL"], 1,
							statuspb.TestStatus_value["FLAKY"], 1,
							statuspb.TestStatus_value["CATEGORIZED_FAIL"], 1,
						},
						Messages: []string{
							"",
							"",
							"",
							"infra_fail_1",
						},
					},
					{
						Name: "test_2",
						Results: []int32{
							statuspb.TestStatus_value["PASS"], 1,
							statuspb.TestStatus_value["FAIL"], 1,
							statuspb.TestStatus_value["FAIL"], 1,
							statuspb.TestStatus_value["CATEGORIZED_FAIL"], 1,
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
			expectedMetrics: []*common.GridMetrics{
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
				{
					Name:             "test_2",
					Passed:           1,
					Failed:           2,
					FlakyCount:       0,
					AverageFlakiness: 2 / 3,
					FailedInfraCount: 1,
					InfraFailures: map[string]int{
						"infra_fail_1": 1,
					},
				},
			},
			expectedFilteredStatus: map[string][]analyzers.StatusCategory{
				"test_1": {
					analyzers.StatusPass, analyzers.StatusFlaky,
				},
				"test_2": {
					analyzers.StatusPass, analyzers.StatusFail,
				},
			},
		},
		{
			name: "grid with no analyzed results produces empty result list",
			grid: &statepb.Grid{
				Columns: []*statepb.Column{
					{Started: -1000},
					{Started: 1000},
					{Started: 2000},
					{Started: 2000},
				},
				Rows: []*statepb.Row{
					{
						Name: "test_1",
						Results: []int32{
							statuspb.TestStatus_value["NO_RESULT"], 4,
						},
						Messages: []string{
							"this_message_should_not_show_up_in_results_0",
							"this_message_should_not_show_up_in_results_1",
							"this_message_should_not_show_up_in_results_2",
							"this_message_should_not_show_up_in_results_3",
						},
					},
				},
			},
			startTime:       0,
			endTime:         2,
			expectedMetrics: []*common.GridMetrics{},
			expectedFilteredStatus: map[string][]analyzers.StatusCategory{
				"test_1": {},
			},
		},
		{
			name: "grid with some non-analyzed results properly assigns correct messages",
			grid: &statepb.Grid{
				Columns: []*statepb.Column{
					{Started: 0},
					{Started: 1000},
					{Started: 1000},
					{Started: 2000},
					{Started: 2000},
				},
				Rows: []*statepb.Row{
					{
						Name: "test_1",
						Results: []int32{
							statuspb.TestStatus_value["PASS"], 1,
							statuspb.TestStatus_value["NO_RESULT"], 2,
							statuspb.TestStatus_value["FAIL"], 2,
						},
						Messages: []string{
							"this_message_should_not_show_up_in_results",
							"this_message_should_show_up_as_an_infra_failure",
							"",
						},
					},
				},
			},
			startTime: 0,
			endTime:   2,
			expectedMetrics: []*common.GridMetrics{
				{
					Name:             "test_1",
					Passed:           1,
					Failed:           1,
					FlakyCount:       0,
					AverageFlakiness: 0.0,
					FailedInfraCount: 1,
					InfraFailures: map[string]int{
						"this_message_should_show_up_as_an_infra_failure": 1,
					},
				},
			},
			expectedFilteredStatus: map[string][]analyzers.StatusCategory{
				"test_1": {
					analyzers.StatusPass, analyzers.StatusFail,
				},
			},
		},
		{
			name: "grid with columns outside of time frame correctly assigns messages",
			grid: &statepb.Grid{
				Columns: []*statepb.Column{
					{Started: 0},
					{Started: 1000},
					{Started: 1000},
					{Started: 7000},
					{Started: 2000},
				},
				Rows: []*statepb.Row{
					{
						Name: "test_1",
						Results: []int32{
							statuspb.TestStatus_value["PASS"], 1,
							statuspb.TestStatus_value["NO_RESULT"], 2,
							statuspb.TestStatus_value["FAIL"], 2,
						},
						Messages: []string{
							"this_message_should_not_show_up_in_results",
							"this_message_should_not_show_up_in_results",
							"this_message_should_show_up_as_an_infra_failure",
						},
					},
				},
			},
			startTime: 0,
			endTime:   2,
			expectedMetrics: []*common.GridMetrics{
				{
					Name:             "test_1",
					Passed:           1,
					Failed:           0,
					FlakyCount:       0,
					AverageFlakiness: 0.0,
					FailedInfraCount: 1,
					InfraFailures: map[string]int{
						"this_message_should_show_up_as_an_infra_failure": 1,
					},
				},
			},
			expectedFilteredStatus: map[string][]analyzers.StatusCategory{
				"test_1": {
					analyzers.StatusPass,
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actualMetrics, actualFS := parseGrid(tc.grid, tc.startTime, tc.endTime)
			if !reflect.DeepEqual(actualMetrics, tc.expectedMetrics) {
				t.Errorf("Metrics disagree:\nactual %+v \n!= \nexpected %+v", actualMetrics[0], tc.expectedMetrics[0])
			}
			if !reflect.DeepEqual(actualFS, tc.expectedFilteredStatus) {
				t.Errorf("Status disagree:\nactual %+v \n!= \nexpected %+v", actualFS, tc.expectedFilteredStatus)
			}
		})
	}
}

func TestIsInfraFailure(t *testing.T) {
	cases := []struct {
		name     string
		message  string
		expected bool
	}{
		{
			name:     "typical matched string increments counts correctly",
			message:  "whatever_valid_word_character_string_with_no_spaces",
			expected: true,
		},
		{
			name:     "unmatched string increments Failed and not other counts",
			message:  "message with spaces should no get matched",
			expected: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !(isInfraFailure(tc.message) == tc.expected) {
				t.Errorf("isInfraFailure(%v) != Expected %v", tc.message, tc.expected)

			}
		})
	}
}

func TestIsValidTestName(t *testing.T) {
	cases := []struct {
		name     string
		testName string
		expected bool
	}{
		{
			name:     "regular name returns true",
			testName: "valid_test",
			expected: true,
		},
		{
			name:     "name with substring '@TESTGRID@' returns false",
			testName: "invalid_test_@TESTGRID@",
			expected: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual := isValidTestName(tc.testName); actual != tc.expected {
				t.Errorf("isValidTestName returned %t for the name %s, but expected %t", actual, tc.testName, tc.expected)
			}
		})
	}
}

func TestFailingColumns(t *testing.T) {
	p := statuspb.TestStatus_value["PASS"]
	f := statuspb.TestStatus_value["FAIL"]
	fl := statuspb.TestStatus_value["FLAKY"]
	cases := []struct {
		name       string
		rows       []*statepb.Row
		numColumns int
		expected   []bool
	}{
		{
			name: "Some failing columns",
			rows: []*statepb.Row{
				{
					Name: "//test1 - [env1]",
					Results: []int32{
						p, 1, f, 1, p, 1, p, 1, f, 1,
					},
				},
				{
					Name: "//test2 - [env1]",
					Results: []int32{
						p, 1, f, 1, p, 1, p, 1, f, 1,
					},
				},
				{
					Name: "//test3 - [env1]",
					Results: []int32{
						p, 1, f, 1, p, 1, p, 1, fl, 1,
					},
				},
				{
					Name: "//test4 - [env1]",
					Results: []int32{
						p, 1, f, 1, p, 1, p, 1, f, 1,
					},
				},
			},
			numColumns: 5,
			expected:   []bool{false, true, false, false, false},
		},
		{
			name: "Unequal Length rows",
			rows: []*statepb.Row{
				{
					Name: "//test1 - [env1]",
					Results: []int32{
						p, 1, f, 1, p, 1,
					},
				},
				{
					Name: "//test2 - [env1]",
					Results: []int32{
						p, 1, f, 1,
					},
				},
				{
					Name: "//test3 - [env1]",
					Results: []int32{
						p, 1, f, 1, p, 1, p, 1,
					},
				},
			},
			numColumns: 3,
			expected:   []bool{false, true, false},
		},
		{
			name: "Only one test",
			rows: []*statepb.Row{
				{
					Name: "//test1 - [env1]",
					Results: []int32{
						p, 1, f, 1, p, 1,
					},
				},
			},
			numColumns: 3,
			expected:   []bool{false, false, false},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual := failingColumns(context.Background(), tc.numColumns, tc.rows); !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("failingColumns(ctx, %v %v)=%v; expected %v", tc.numColumns, tc.rows, actual, tc.expected)
			}
		})
	}
}
