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

// Package analyzers represents ways to analyze healthiness and flakiness of tests
package analyzers

import (
	summarypb "github.com/GoogleCloudPlatform/testgrid/pb/summary"
	"github.com/GoogleCloudPlatform/testgrid/pkg/summarizer/common"
	"github.com/golang/protobuf/ptypes/timestamp"
)

// IntString is for sorting, primarily intended for map[string]int as implemented below
type IntString struct {
	s string
	i int
}

// BaseAnalyzer implements functions that calculate flakiness as a ratio of failed tests to total tests
type BaseAnalyzer struct {
}

// GetFlakiness returns a HealthinessInfo message with data to display flakiness as a ratio of failed tests
// to total tests
func (na *BaseAnalyzer) GetFlakiness(gridMetrics []*common.GridMetrics, minRuns int, startDate int, endDate int, tab string) *summarypb.HealthinessInfo {
	testInfoList := []*summarypb.TestInfo{}
	for _, test := range gridMetrics {
		testInfo, success := calculateNaiveFlakiness(test, minRuns)
		if !success {
			continue
		}
		// TODO (itsazhuhere@): Introduce name parsing into test name and env
		testInfo.DisplayName = test.Name
		testInfoList = append(testInfoList, testInfo)
	}
	// Populate Healthiness with above calculated information
	healthiness := createHealthiness(startDate, endDate, testInfoList)
	return healthiness
}

func createHealthiness(startDate int, endDate int, testInfoList []*summarypb.TestInfo) *summarypb.HealthinessInfo {
	healthiness := &summarypb.HealthinessInfo{
		Start: intToTimestamp(startDate),
		End:   intToTimestamp(endDate),
		Tests: testInfoList,
	}

	var averageFlakiness float32
	for _, testInfo := range healthiness.Tests {
		averageFlakiness += testInfo.Flakiness
	}
	totalTests := int32(len(healthiness.Tests))
	if totalTests > 0 {
		healthiness.AverageFlakiness = averageFlakiness / float32(totalTests)
	}
	return healthiness
}

func calculateNaiveFlakiness(test *common.GridMetrics, minRuns int) (*summarypb.TestInfo, bool) {
	failedCount := int32(test.Failed)
	totalCount := int32(test.Passed) + int32(test.Failed)
	totalCountWithInfra := totalCount + int32(test.FailedInfraCount)
	if totalCount <= 0 || totalCount < int32(minRuns) {
		return &summarypb.TestInfo{}, false
	}
	// Convert from map[string]int to map[string]int32
	infraFailures := map[string]int32{}
	for key, value := range test.InfraFailures {
		infraFailures[key] = int32(value)
	}

	flakiness := 100 * float32(failedCount) / float32(totalCount)
	testInfo := &summarypb.TestInfo{
		Flakiness:          flakiness,
		TotalNonInfraRuns:  totalCount,
		TotalRunsWithInfra: totalCountWithInfra,
		PassedNonInfraRuns: int32(test.Passed),
		FailedNonInfraRuns: int32(test.Failed),
		FailedInfraRuns:    int32(test.FailedInfraCount),
		InfraFailures:      infraFailures,
	}
	return testInfo, true

}

func intToTimestamp(seconds int) *timestamp.Timestamp {
	timestamp := &timestamp.Timestamp{
		Seconds: int64(seconds),
	}
	return timestamp
}
