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

package naiveanalyzer

import (
	"reflect"
	"testing"

	summarypb "github.com/GoogleCloudPlatform/testgrid/pb/summary"
	"github.com/GoogleCloudPlatform/testgrid/pkg/summarizer/common"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
)

func getTypicalGridMetricsArray() []*common.GridMetrics {
	return []*common.GridMetrics{
		{
			Name:             "//test1 - [env1]",
			Passed:           1,
			Failed:           1,
			FlakyCount:       1,
			AverageFlakiness: 50.0,
			FailedInfraCount: 1,
		},
	}
}

func createTimestamp(time int) *timestamp.Timestamp {
	timestamp := &timestamp.Timestamp{
		Seconds: int64(time),
	}
	return timestamp
}

func TestGetFlakiness(t *testing.T) {
	cases := []struct {
		name      string
		metrics   []*common.GridMetrics
		minRuns   int
		startDate int
		endDate   int
		tab       string
		expected  *summarypb.HealthinessInfo
	}{
		{
			name:      "typical case returns expected healthiness",
			metrics:   getTypicalGridMetricsArray(),
			minRuns:   -1,
			startDate: 0,
			endDate:   2,
			tab:       "tab1",
			expected: &summarypb.HealthinessInfo{
				Start: createTimestamp(0),
				End:   createTimestamp(2),
				Tests: []*summarypb.TestInfo{
					{
						DisplayName:        "//test1 - [env1]",
						TotalNonInfraRuns:  2,
						TotalRunsWithInfra: 3,
						PassedNonInfraRuns: 1,
						FailedNonInfraRuns: 1,
						FailedInfraRuns:    1,
						Flakiness:          50,
					},
				},
				AverageFlakiness: 50,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			analyzer := NaiveAnalyzer{}
			if actual := analyzer.GetFlakiness(tc.metrics, tc.minRuns, tc.startDate, tc.endDate, tc.tab); !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("\nactual %+v \n!= \nexpected %+v", actual, tc.expected)
			}
		})
	}
}

func TestCreateHealthiness(t *testing.T) {
	cases := []struct {
		name         string
		startDate    int
		endDate      int
		testInfoList []*summarypb.TestInfo
		expected     *summarypb.HealthinessInfo
	}{
		{
			name:         "typical inputs return correct Healthiness output",
			startDate:    0,
			endDate:      2,
			testInfoList: []*summarypb.TestInfo{},
			expected: &summarypb.HealthinessInfo{
				Start: createTimestamp(0),
				End:   createTimestamp(2),
				Tests: []*summarypb.TestInfo{},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual := createHealthiness(tc.startDate, tc.endDate, tc.testInfoList); !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("\nactual: %+v \n!= \nexpected: %+v", actual, tc.expected)
			}
		})
	}
}

func TestCalculateNaiveFlakiness(t *testing.T) {
	cases := []struct {
		name             string
		test             *common.GridMetrics
		minRuns          int
		expectedTestInfo *summarypb.TestInfo
		expectedSuccess  bool
	}{
		{
			name:             "correctly filters GridMetrics with less than minRuns",
			test:             &common.GridMetrics{},
			minRuns:          1000, // arbitrarily large number so that it should get filtered
			expectedTestInfo: &summarypb.TestInfo{},
			expectedSuccess:  false,
		},
		{
			name: "typical GridMetrics returns correct TestInfo",
			test: &common.GridMetrics{
				Passed:           3,
				Failed:           2,
				FlakyCount:       8,
				AverageFlakiness: 50.0,
				FailedInfraCount: 4,
			},
			minRuns: -1,
			expectedTestInfo: &summarypb.TestInfo{
				DisplayName:        "",
				Flakiness:          40.0,
				TotalNonInfraRuns:  5,
				TotalRunsWithInfra: 9,
				PassedNonInfraRuns: 3,
				FailedNonInfraRuns: 2,
				FailedInfraRuns:    4,
			},
			expectedSuccess: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actualTest, actualSuccess := calculateNaiveFlakiness(tc.test, tc.minRuns); !proto.Equal(actualTest, tc.expectedTestInfo) || tc.expectedSuccess != actualSuccess {
				t.Errorf("\ntestInfo:\nactual: %v vs. expected: %v\nsuccess:\nactual: %v vs. expected: %v", actualTest, tc.expectedTestInfo, actualSuccess, tc.expectedSuccess)
			}
		})
	}
}

func TestIntToTimestamp(t *testing.T) {
	cases := []struct {
		name     string
		seconds  int
		expected *timestamp.Timestamp
	}{
		{
			name:    "typical input returns correct timestamp",
			seconds: 2,
			expected: &timestamp.Timestamp{
				Seconds: int64(2),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual := intToTimestamp(tc.seconds); !proto.Equal(actual, tc.expected) {
				t.Errorf("actual %+v != expected %+v", actual, tc.expected)
			}
		})
	}
}
