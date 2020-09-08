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
	"regexp"

	"github.com/GoogleCloudPlatform/testgrid/internal/result"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	summarypb "github.com/GoogleCloudPlatform/testgrid/pb/summary"
	statuspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	"github.com/GoogleCloudPlatform/testgrid/pkg/summarizer/analyzers"
	"github.com/GoogleCloudPlatform/testgrid/pkg/summarizer/common"
)

const (
	minRuns = 0
	// DefaultInterval is the default number of days of analysis
	DefaultInterval = 7
)

var (
	infraRegex      = regexp.MustCompile(`^\w+$`)
	testMethodRegex = regexp.MustCompile(`@TESTGRID@`)
)

type flakinessAnalyzer interface {
	GetFlakiness(gridMetrics []*common.GridMetrics, relevantFilteredStatus map[string][]statuspb.TestStatus, minRuns int, startDate int, endDate int, tab string) *summarypb.HealthinessInfo
}

// CalculateHealthiness extracts the test run data from each row (which represents a test)
// of the Grid and then analyzes it with an implementation of flakinessAnalyzer, which has
// implementations in the subdir naive and can be injected as needed.
func CalculateHealthiness(grid *statepb.Grid, startTime int, endTime int, tab string) *summarypb.HealthinessInfo {
	gridMetrics, relevantFilteredStatus := parseGrid(grid, startTime, endTime)
	analyzer := analyzers.FlipAnalyzer{
		RelevantStatus: relevantFilteredStatus,
	}
	return analyzer.GetFlakiness(gridMetrics, minRuns, startTime, endTime, tab)
}

// CalculateTrend populates the ChangeFromLastInterval fields of each TestInfo by comparing
// the current flakiness to the flakiness calculated for the last interval. Interval length
// is a config value that is 7 days by default. The Trend enum defaults to UNKNOWN, so there
// is no need to explicitly assign UNKNOWN when a test appears in currentHealthiness but not
// in previousHealthiness.
func CalculateTrend(currentHealthiness, previousHealthiness *summarypb.HealthinessInfo) {
	previousFlakiness := map[string]float32{}
	// Create a map for faster lookup and avoiding repeated iteration through Tests
	for _, test := range previousHealthiness.Tests {
		previousFlakiness[test.DisplayName] = test.Flakiness
	}

	for i, test := range currentHealthiness.Tests {
		if value, ok := previousFlakiness[test.DisplayName]; ok {
			currentHealthiness.Tests[i].ChangeFromLastInterval = getTrend(test.Flakiness, value)
			currentHealthiness.Tests[i].PreviousFlakiness = []float32{value}
		}
	}
}

func getTrend(currentFlakiness, previousFlakiness float32) summarypb.TestInfo_Trend {
	if currentFlakiness < previousFlakiness {
		return summarypb.TestInfo_DOWN
	}
	if currentFlakiness > previousFlakiness {
		return summarypb.TestInfo_UP
	}
	return summarypb.TestInfo_NO_CHANGE
}

func parseGrid(grid *statepb.Grid, startTime int, endTime int) ([]*common.GridMetrics, map[string][]analyzers.StatusCategory) {
	// Get the relevant data for flakiness from each Grid (which represents
	// a dashboard tab) as a list of GridMetrics structs

	// TODO (itsazhuhere@): consider refactoring/using summary.go's gridMetrics function
	// as it does very similar data collection.

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Multiply by 1000 because currently Column.Started is in milliseconds; this is used
	// for comparisons later. startTime and endTime will be used in a Timestamp later that
	// requires seconds, so we would like to impact that at little as possible.
	startTime *= 1000
	endTime *= 1000

	// We create maps because result.Map returns a map where we can access each result
	// through the test name, and at each instance we can increment our types.Result
	// using the same key. At the end we can filter out those types.Result that had
	// 0 of all counts.
	gridMetricsMap := make(map[string]*common.GridMetrics, 0)
	gridRows := make(map[string]*statepb.Row)

	// For each filtered test, status of non-infra-failure tests
	rowStatuses := make(map[string][]analyzers.StatusCategory)

	for i, row := range grid.Rows {
		gridRows[row.Name] = grid.Rows[i]
		gridMetricsMap[row.Name] = common.NewGridMetrics(row.Name)
		rowStatuses[row.Name] = []analyzers.StatusCategory{}
	}

	// result.Map is written in a way that assumes each test/row name is unique
	rowResults := result.Map(ctx, grid.Rows)
	failingColumns := failingColumns(ctx, len(grid.Columns), grid.Rows)

	for key, ch := range rowResults {
		if !isValidTestName(key) {
			continue
		}
		rowToMessageIndex := 0
		i := -1
		for nextRowResult := range ch {
			i++
			if i >= len(grid.Columns) {
				break
			}
			rowResult := result.Coalesce(nextRowResult, result.FailRunning)

			// We still need to increment rowToMessageIndex even if we want to skip counting
			// this column.
			if !isWithinTimeFrame(grid.Columns[i], startTime, endTime) {
				switch rowResult {
				case statuspb.TestStatus_NO_RESULT:
					// Ignore NO_RESULT (e.g. blank cell)
				default:
					rowToMessageIndex++
				}
				continue
			}
			switch rowResult {
			case statuspb.TestStatus_NO_RESULT:
				continue
			case statuspb.TestStatus_FAIL:
				message := gridRows[key].Messages[rowToMessageIndex]
				if isInfraFailure(message) {
					gridMetricsMap[key].FailedInfraCount++
					gridMetricsMap[key].InfraFailures[message] = gridMetricsMap[key].InfraFailures[message] + 1
				} else {
					gridMetricsMap[key].Failed++
					if !failingColumns[i] {
						rowStatuses[key] = append(rowStatuses[key], analyzers.StatusFail)
					}
				}
			case statuspb.TestStatus_PASS:
				gridMetricsMap[key].Passed++
				rowStatuses[key] = append(rowStatuses[key], analyzers.StatusPass)
			case statuspb.TestStatus_FLAKY:
				rowStatuses[key] = append(rowStatuses[key], analyzers.StatusFlaky)
				getValueOfFlakyMetric(gridMetricsMap[key])
			}
			rowToMessageIndex++
		}
	}
	gridMetrics := make([]*common.GridMetrics, 0)
	for _, metric := range gridMetricsMap {
		if metric.Failed > 0 || metric.Passed > 0 || metric.FlakyCount > 0 {
			gridMetrics = append(gridMetrics, metric)
		}
	}
	return gridMetrics, rowStatuses
}

// failingColumns iterates over the grid in column-major order
// and returns a slice of bool indicating whether a column is 100% failing.
func failingColumns(ctx context.Context, numColumns int, rows []*statepb.Row) []bool {
	// Convert to map of iterators to handle run-length encoding.
	rowResults := result.Map(ctx, rows)
	out := make([]bool, numColumns)
	if len(rows) <= 1 {
		// If we only have one test, don't do this metric.
		return out
	}
	for i := 0; i < numColumns; i++ {
		out[i] = true
		for _, row := range rowResults {
			rr, more := <-row
			if !more {
				continue
			}
			crr := result.Coalesce(rr, true)
			if crr == statuspb.TestStatus_PASS || crr == statuspb.TestStatus_FLAKY {
				out[i] = false
			}
		}
	}
	return out
}

func isInfraFailure(message string) bool {
	return (message != "" && infraRegex.MatchString(message))
}

func getValueOfFlakyMetric(gridMetrics *common.GridMetrics) {
	// TODO (itszhuhere@): add a way to get exact flakiness from a Row_FLAKY cell
	// For now we will leave it as 50%, because:
	// a) gridMetrics.flakiness and .flakyCount are currently not used by anything
	// and
	// b) there's no easy way to get the exact flakiness measurement from prow or whatever else
	// and potentially
	// c) GKE does not currently enable retry on flakes so it isn't as important right now
	// Keep in mind that flakiness is measured as out of 100, i.e. 23 not .23
	flakiness := 50.0
	gridMetrics.FlakyCount++
	// Formula for adding one new value to mean is mean + (newValue - mean) / newCount
	gridMetrics.AverageFlakiness += (flakiness - gridMetrics.AverageFlakiness) / float64(gridMetrics.FlakyCount)
}

func isWithinTimeFrame(column *statepb.Column, startTime, endTime int) bool {
	return column.Started >= float64(startTime) && column.Started <= float64(endTime)
}

func isValidTestName(name string) bool {
	// isValidTestName filters out test names, currently only tests with
	// @TESTGRID@ which would otherwise make some summaries unnecessarily large
	return !testMethodRegex.MatchString(name)
}
