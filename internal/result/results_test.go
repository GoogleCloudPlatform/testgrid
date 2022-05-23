/*
Copyright 2020 The Kubernetes Authors.

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
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	statuspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
)

func TestPassing(t *testing.T) {
	cases := []struct {
		status   statuspb.TestStatus
		expected bool
	}{
		{
			status: statuspb.TestStatus_NO_RESULT,
		},
		{
			status:   statuspb.TestStatus_BUILD_PASSED,
			expected: true,
		},
		{
			status:   statuspb.TestStatus_PASS,
			expected: true,
		},
		{
			status:   statuspb.TestStatus_PASS_WITH_SKIPS,
			expected: true,
		},
		{
			status:   statuspb.TestStatus_PASS_WITH_ERRORS,
			expected: true,
		},
		{
			status: statuspb.TestStatus_RUNNING,
		},
		{
			status: statuspb.TestStatus_CATEGORIZED_ABORT,
		},
		{
			status: statuspb.TestStatus_FLAKY,
		},
		{
			status: statuspb.TestStatus_FAIL,
		},
	}

	for _, tc := range cases {
		t.Run(tc.status.String(), func(t *testing.T) {
			if actual := Passing(tc.status); actual != tc.expected {
				t.Errorf("Passing(%v) got %t, want %t", tc.status, actual, tc.expected)
			}
		})
	}
}

func TestFailing(t *testing.T) {
	cases := []struct {
		status   statuspb.TestStatus
		expected bool
	}{
		{
			status: statuspb.TestStatus_NO_RESULT,
		},
		{
			status: statuspb.TestStatus_BUILD_PASSED,
		},
		{
			status: statuspb.TestStatus_PASS,
		},
		{
			status: statuspb.TestStatus_RUNNING,
		},
		{
			status: statuspb.TestStatus_CATEGORIZED_ABORT,
		},
		{
			status: statuspb.TestStatus_UNKNOWN,
		},
		{
			status: statuspb.TestStatus_CANCEL,
		},
		{
			status: statuspb.TestStatus_FLAKY,
		},
		{
			status:   statuspb.TestStatus_TOOL_FAIL,
			expected: true,
		},
		{
			status:   statuspb.TestStatus_TIMED_OUT,
			expected: true,
		},
		{
			status:   statuspb.TestStatus_CATEGORIZED_FAIL,
			expected: true,
		},
		{
			status:   statuspb.TestStatus_BUILD_FAIL,
			expected: true,
		},
		{
			status:   statuspb.TestStatus_FAIL,
			expected: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.status.String(), func(t *testing.T) {
			if actual := Failing(tc.status); actual != tc.expected {
				t.Errorf("Failing(%v) got %t, want %t", tc.status, actual, tc.expected)
			}
		})
	}
}

func TestCoalesce(t *testing.T) {
	cases := []struct {
		status        statuspb.TestStatus
		ignoreRunning bool
		expected      statuspb.TestStatus
	}{
		{
			status:        statuspb.TestStatus_NO_RESULT,
			ignoreRunning: true,
			expected:      statuspb.TestStatus_NO_RESULT,
		},
		{
			status:   statuspb.TestStatus_NO_RESULT,
			expected: statuspb.TestStatus_NO_RESULT,
		},
		{
			status:   statuspb.TestStatus_BUILD_PASSED,
			expected: statuspb.TestStatus_PASS,
		},
		{
			status:   statuspb.TestStatus_PASS,
			expected: statuspb.TestStatus_PASS,
		},
		{
			status:        statuspb.TestStatus_PASS_WITH_ERRORS,
			ignoreRunning: true,
			expected:      statuspb.TestStatus_PASS,
		},
		{
			status:        statuspb.TestStatus_RUNNING,
			ignoreRunning: true,
			expected:      statuspb.TestStatus_NO_RESULT,
		},
		{
			status:   statuspb.TestStatus_RUNNING,
			expected: statuspb.TestStatus_UNKNOWN,
		},
		{
			status:   statuspb.TestStatus_CANCEL,
			expected: statuspb.TestStatus_UNKNOWN,
		},
		{
			status:        statuspb.TestStatus_TOOL_FAIL,
			ignoreRunning: true,
			expected:      statuspb.TestStatus_FAIL,
		},
		{
			status:   statuspb.TestStatus_TOOL_FAIL,
			expected: statuspb.TestStatus_FAIL,
		},
		{
			status:        statuspb.TestStatus_CATEGORIZED_FAIL,
			ignoreRunning: true,
			expected:      statuspb.TestStatus_FAIL,
		},
		{
			status:   statuspb.TestStatus_FAIL,
			expected: statuspb.TestStatus_FAIL,
		},
	}

	for _, tc := range cases {
		name := fmt.Sprintf("Coalesce(%v,%t)", tc.status, tc.ignoreRunning)
		t.Run(name, func(t *testing.T) {
			if actual := Coalesce(tc.status, tc.ignoreRunning); actual != tc.expected {
				t.Errorf("got %v, want %v", actual, tc.expected)
			}
		})
	}
}

func TestIter(t *testing.T) {
	stoi := func(s statuspb.TestStatus) int32 { return int32(s) }
	cases := []struct {
		name     string
		ctx      context.Context
		results  []int32
		expected []statuspb.TestStatus
	}{
		{
			name: "basically works",
		},
		{
			name: "works correctly",
			results: []int32{
				stoi(statuspb.TestStatus_FAIL), 1,
				stoi(statuspb.TestStatus_PASS), 2,
				stoi(statuspb.TestStatus_FLAKY), 3,
			},
			expected: []statuspb.TestStatus{
				statuspb.TestStatus_FAIL,
				statuspb.TestStatus_PASS,
				statuspb.TestStatus_PASS,
				statuspb.TestStatus_FLAKY,
				statuspb.TestStatus_FLAKY,
				statuspb.TestStatus_FLAKY,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ctx == nil {
				tc.ctx = context.Background()
			}
			var actual []statuspb.TestStatus
			f := Iter(tc.results)
			for {
				item, more := f()
				if !more {
					break
				}
				actual = append(actual, item)
			}
			if !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("Iter(%v) got %v, want %v", tc.results, actual, tc.expected)
			}
		})
	}
}

const totalResults = 10e6

func benchmarkResults(remain int32) []int32 {
	rand.Seed(42)

	var statuses []int32
	for num := range statuspb.TestStatus_name {
		statuses = append(statuses, num)
	}
	var results []int32
	n := int32(len(statuses))
	for remain > 0 {
		result := rand.Int31() % n
		count := rand.Int31() % 100
		if count > remain {
			count = remain
		}
		results = append(results, result, count)
		remain -= count
	}
	return results
}

func BenchmarkIter(b *testing.B) {

	loopCh := func(results []int32) int32 {
		var n int32
		ch := iterSlow(context.Background(), results)
		for range ch {
			n++
		}
		return n
	}

	loopF := func(results []int32) int32 {
		var n int32
		f := iterFast(results)
		for {
			_, more := f()
			if !more {
				break
			}
			n++
		}
		return n
	}

	const (
		few  = 1e6
		many = 10e6
	)

	cases := []struct {
		name    string
		results int32
		loop    func([]int32) int32
	}{
		{
			name:    "few chan",
			results: few,
			loop:    loopCh,
		},
		{
			name:    "few func",
			results: few,
			loop:    loopF,
		},
		{
			name:    "many chan",
			results: many,
			loop:    loopCh,
		},
		{
			name:    "many func",
			results: many,
			loop:    loopF,
		},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			results := benchmarkResults(int32(tc.results))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				n := tc.loop(results)
				if n != tc.results {
					b.Fatalf("Got %d results, wanted %d", n, tc.results)
				}
			}
		})
	}
}
