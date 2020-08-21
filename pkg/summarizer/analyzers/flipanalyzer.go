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
)

const analyzerName = "flipanalyzer"

// FlipAnalyzer implements functions that calculate flakiness as a ratio of failed tests to total tests
type FlipAnalyzer struct {
	RelevantStatus map[string][]StatusCategory
}

// StatusCategory is a simiplified status that allows only "Pass", "Fail", and "Flaky"
type StatusCategory int32

// StatusPass, StatusFail, and StatusFlaky are the status categories this analyzer works with.
const (
	StatusPass StatusCategory = iota
	StatusFail
	StatusFlaky
)

// GetFlakiness returns a HealthinessInfo message
func (ea *FlipAnalyzer) GetFlakiness(gridMetrics []*common.GridMetrics, minRuns int, startDate int, endDate int, tab string) *summarypb.HealthinessInfo {
	// Delegate to a BaseAnalyzer and change the flakiness scores.
	// (And average flakiness)
	var ba BaseAnalyzer
	healthinessInfo := ba.GetFlakiness(gridMetrics, minRuns, startDate, endDate, tab)
	var averageFlakiness float32
	for _, test := range healthinessInfo.Tests {
		test.Flakiness = calculateFlipFlakiness(ea.RelevantStatus[test.DisplayName])
		averageFlakiness += test.Flakiness
	}
	if len(healthinessInfo.Tests) == 0 {
		healthinessInfo.AverageFlakiness = 0
	} else {
		healthinessInfo.AverageFlakiness = averageFlakiness / float32(len(healthinessInfo.Tests))
	}
	return healthinessInfo
}

const ignoreFailuresInARow = 3

func consecutiveFailures(statuses []StatusCategory, i int) int {
	var result int
	for i < len(statuses) && statuses[i] == StatusFail {
		result++
		i++
	}

	return result
}

// calculateFlipFlakiness gets a calculation of flakiness based on number of flips to failing rather than number of failures
// statuses should have already filtered to the correct time horizon and removed infra failures
// Returns a percentage between 0 and 100
func calculateFlipFlakiness(statuses []StatusCategory) float32 {
	var flips int
	var considered int
	lastPassing := true // No flakes if we pass 100%
	var i int
	for i < len(statuses) {
		cf := consecutiveFailures(statuses, i)
		if cf >= ignoreFailuresInARow {
			// Ignore the run of failures
			i += cf
			if i >= len(statuses) {
				break
			}
		}
		s := statuses[i]
		considered++
		if s == StatusPass {
			lastPassing = true
		} else if s == StatusFlaky {
			// Consider this as always a flip (because there was a flip involved), but it did pass.
			flips++
			lastPassing = true
		} else {
			// Failing
			if lastPassing {
				flips++
			}
			lastPassing = false
		}
		i++
	}
	if considered == 0 {
		return 0
	}
	return 100 * float32(flips) / float32(considered)

}
