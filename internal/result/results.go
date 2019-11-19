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

	"github.com/GoogleCloudPlatform/testgrid/pb/state"
)

const (
	// IgnoreRunning maps RUNNING to NO_RESULT
	IgnoreRunning = true
	// FailRunning maps RUNNING to FAIL
	FailRunning = false
)

// Coalesce reduces the result to PASS, NO_RESULT, FAIL or FLAKY.
func Coalesce(result state.Row_Result, ignoreRunning bool) state.Row_Result {
	// TODO(fejta): other result types, not used by k8s testgrid
	if result == state.Row_NO_RESULT || result == state.Row_RUNNING && ignoreRunning {
		return state.Row_NO_RESULT
	}
	if result == state.Row_FAIL || result == state.Row_RUNNING {
		return state.Row_FAIL
	}
	if result == state.Row_FLAKY {
		return result
	}
	return state.Row_PASS
}

// Iter returns a channel that outputs the result for each column, decoding the run-length-encoding.
func Iter(ctx context.Context, results []int32) <-chan state.Row_Result {
	out := make(chan state.Row_Result)
	go func() {
		defer close(out)
		for i := 0; i+1 < len(results); i += 2 {
			result := state.Row_Result(results[i])
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
func Map(ctx context.Context, rows []*state.Row) map[string]<-chan state.Row_Result {
	iters := map[string]<-chan state.Row_Result{}
	for _, r := range rows {
		iters[r.Name] = Iter(ctx, r.Results)
	}
	return iters
}
