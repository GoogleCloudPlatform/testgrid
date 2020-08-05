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
)

const (
	// IgnoreRunning maps RUNNING to NO_RESULT
	IgnoreRunning = true
	// FailRunning maps RUNNING to FAIL
	FailRunning = false
)

var (
	statusSeverity = map[statepb.Row_Result]int{
		statepb.Row_NO_RESULT:         0,
		statepb.Row_PASS:              1,
		statepb.Row_BUILD_PASSED:      2,
		statepb.Row_PASS_WITH_ERRORS:  3,
		statepb.Row_PASS_WITH_SKIPS:   4,
		statepb.Row_RUNNING:           5,
		statepb.Row_CATEGORIZED_ABORT: 6,
		statepb.Row_UNKNOWN:           7,
		statepb.Row_CANCEL:            8,
		statepb.Row_BLOCKED:           9,
		statepb.Row_TOOL_FAIL:         10,
		statepb.Row_TIMED_OUT:         11,
		statepb.Row_CATEGORIZED_FAIL:  12,
		statepb.Row_BUILD_FAIL:        13,
		statepb.Row_FAIL:              14,
		statepb.Row_FLAKY:             15,
	}
)

func resultLte(rowResult, compareTo statepb.Row_Result) bool {
	return statusSeverity[rowResult] <= statusSeverity[compareTo]
}

func resultGte(rowResult, compareTo statepb.Row_Result) bool {
	return statusSeverity[rowResult] >= statusSeverity[compareTo]
}

// IsPassingResult returns true if the Row_Result is any of the passing results,
// including PASS_WITH_SKIPS, BUILD_PASSED, and more.
func IsPassingResult(rowResult statepb.Row_Result) bool {
	return resultGte(rowResult, statepb.Row_PASS) && resultLte(rowResult, statepb.Row_PASS_WITH_SKIPS)
}

// IsFailingResult returns true if the Row_Result is any of the failing results,
// including CATEGORIZED_FAILURE, BUILD_FAIL, and more.
func IsFailingResult(rowResult statepb.Row_Result) bool {
	return resultGte(rowResult, statepb.Row_TOOL_FAIL) && resultLte(rowResult, statepb.Row_FAIL)
}

// Coalesce reduces the result to PASS, NO_RESULT, FAIL or FLAKY.
func Coalesce(result statepb.Row_Result, ignoreRunning bool) statepb.Row_Result {
	// TODO(fejta): other result types, not used by k8s testgrid
	if result == statepb.Row_NO_RESULT || result == statepb.Row_RUNNING && ignoreRunning {
		return statepb.Row_NO_RESULT
	}
	if IsFailingResult(result) || result == statepb.Row_RUNNING {
		return statepb.Row_FAIL
	}
	if result == statepb.Row_FLAKY {
		return result
	}
	return statepb.Row_PASS
}

// Iter returns a channel that outputs the result for each column, decoding the run-length-encoding.
func Iter(ctx context.Context, results []int32) <-chan statepb.Row_Result {
	out := make(chan statepb.Row_Result)
	go func() {
		defer close(out)
		for i := 0; i+1 < len(results); i += 2 {
			result := statepb.Row_Result(results[i])
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
func Map(ctx context.Context, rows []*statepb.Row) map[string]<-chan statepb.Row_Result {
	iters := map[string]<-chan statepb.Row_Result{}
	for _, r := range rows {
		iters[r.Name] = Iter(ctx, r.Results)
	}
	return iters
}
