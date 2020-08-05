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

	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	summarypb "github.com/GoogleCloudPlatform/testgrid/pb/summary"
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
		name      string
		grid      *statepb.Grid
		startTime int
		endTime   int
		expected  []*common.GridMetrics
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
							statepb.Row_Result_value["PASS"], 1,
							statepb.Row_Result_value["FAIL"], 1,
							statepb.Row_Result_value["FLAKY"], 1,
							statepb.Row_Result_value["CATEGORIZED_FAIL"], 1,
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
							statepb.Row_Result_value["NO_RESULT"], 4,
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
			startTime: 0,
			endTime:   2,
			expected:  []*common.GridMetrics{},
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
							statepb.Row_Result_value["PASS"], 1,
							statepb.Row_Result_value["NO_RESULT"], 2,
							statepb.Row_Result_value["FAIL"], 2,
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
			expected: []*common.GridMetrics{
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
							statepb.Row_Result_value["PASS"], 1,
							statepb.Row_Result_value["NO_RESULT"], 2,
							statepb.Row_Result_value["FAIL"], 2,
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
			expected: []*common.GridMetrics{
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
