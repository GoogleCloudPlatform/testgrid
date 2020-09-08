/*
Copyright 2019 The Kubernetes Authors.

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

package result

import (
	"context"

	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	statuspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
)

const (
	// IgnoreRunning maps RUNNING to NO_RESULT
	IgnoreRunning = true
	// FailRunning maps RUNNING to FAIL
	FailRunning = false
)

var (
	statusSeverity = map[statuspb.TestStatus]int{
		statuspb.TestStatus_NO_RESULT:         0,
		statuspb.TestStatus_PASS:              1,
		statuspb.TestStatus_BUILD_PASSED:      2,
		statuspb.TestStatus_PASS_WITH_ERRORS:  3,
		statuspb.TestStatus_PASS_WITH_SKIPS:   4,
		statuspb.TestStatus_RUNNING:           5,
		statuspb.TestStatus_CATEGORIZED_ABORT: 6,
		statuspb.TestStatus_UNKNOWN:           7,
		statuspb.TestStatus_CANCEL:            8,
		statuspb.TestStatus_BLOCKED:           9,
		statuspb.TestStatus_TOOL_FAIL:         10,
		statuspb.TestStatus_TIMED_OUT:         11,
		statuspb.TestStatus_CATEGORIZED_FAIL:  12,
		statuspb.TestStatus_BUILD_FAIL:        13,
		statuspb.TestStatus_FAIL:              14,
		statuspb.TestStatus_FLAKY:             15,
	}
)

func resultLte(rowResult, compareTo statuspb.TestStatus) bool {
	return statusSeverity[rowResult] <= statusSeverity[compareTo]
}

func resultGte(rowResult, compareTo statuspb.TestStatus) bool {
	return statusSeverity[rowResult] >= statusSeverity[compareTo]
}

// IsPassingResult returns true if the Row_Result is any of the passing results,
// including PASS_WITH_SKIPS, BUILD_PASSED, and more.
func IsPassingResult(rowResult statuspb.TestStatus) bool {
	return resultGte(rowResult, statuspb.TestStatus_PASS) && resultLte(rowResult, statuspb.TestStatus_PASS_WITH_SKIPS)
}

// IsFailingResult returns true if the Row_Result is any of the failing results,
// including CATEGORIZED_FAILURE, BUILD_FAIL, and more.
func IsFailingResult(rowResult statuspb.TestStatus) bool {
	return resultGte(rowResult, statuspb.TestStatus_TOOL_FAIL) && resultLte(rowResult, statuspb.TestStatus_FAIL)
}

// Coalesce reduces the result to PASS, NO_RESULT, FAIL or FLAKY.
func Coalesce(result statuspb.TestStatus, ignoreRunning bool) statuspb.TestStatus {
	// TODO(fejta): other result types, not used by k8s testgrid
	if result == statuspb.TestStatus_NO_RESULT || result == statuspb.TestStatus_RUNNING && ignoreRunning {
		return statuspb.TestStatus_NO_RESULT
	}
	if IsFailingResult(result) || result == statuspb.TestStatus_RUNNING {
		return statuspb.TestStatus_FAIL
	}
	if result == statuspb.TestStatus_FLAKY {
		return result
	}
	return statuspb.TestStatus_PASS
}

// Iter returns a channel that outputs the result for each column, decoding the run-length-encoding.
func Iter(ctx context.Context, results []int32) <-chan statuspb.TestStatus {
	out := make(chan statuspb.TestStatus)
	go func() {
		defer close(out)
		for i := 0; i+1 < len(results); i += 2 {
			result := statuspb.TestStatus(results[i])
			count := results[i+1]
			for count > 0 {
				select {
				case <-ctx.Done():
					return
				case out <- result:
					count--
				}
				select {
				case <-ctx.Done(): // In case we lost the race
					return
				default:
				}
			}
		}
	}()
	return out
}

// Map returns a per-column result output channel for each row.
func Map(ctx context.Context, rows []*statepb.Row) map[string]<-chan statuspb.TestStatus {
	iters := map[string]<-chan statuspb.TestStatus{}
	for _, r := range rows {
		iters[r.Name] = Iter(ctx, r.Results)
	}
	return iters
}
