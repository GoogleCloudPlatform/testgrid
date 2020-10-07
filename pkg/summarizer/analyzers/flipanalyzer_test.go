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

package analyzers

import (
	"testing"

	summarypb "github.com/GoogleCloudPlatform/testgrid/pb/summary"
	"github.com/GoogleCloudPlatform/testgrid/pkg/summarizer/common"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestGetFlakinessFlip(t *testing.T) {
	cases := []struct {
		name                   string
		metrics                []*common.GridMetrics
		relevantFilteredStatus map[string][]StatusCategory
		minRuns                int
		startDate              int
		endDate                int
		tab                    string
		expected               *summarypb.HealthinessInfo
	}{
		{
			name: "case where naive and enhanced flakiness disagree",
			metrics: []*common.GridMetrics{
				{
					Name:             "//test1 - [env1]",
					Passed:           3,
					Failed:           3,
					FlakyCount:       0,
					AverageFlakiness: 50.0,
					FailedInfraCount: 1,
				},
			},
			relevantFilteredStatus: map[string][]StatusCategory{
				"//test1 - [env1]": {
					StatusFail, StatusFail, StatusFail, StatusPass, StatusPass, StatusPass,
				},
			},
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
						TotalNonInfraRuns:  6,
						TotalRunsWithInfra: 7,
						PassedNonInfraRuns: 3,
						FailedNonInfraRuns: 3,
						FailedInfraRuns:    1,
						Flakiness:          0,
					},
				},
				AverageFlakiness: 0,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			analyzer := FlipAnalyzer{
				RelevantStatus: tc.relevantFilteredStatus,
			}
			actual := analyzer.GetFlakiness(tc.metrics, tc.minRuns, tc.startDate, tc.endDate, tc.tab)
			if diff := cmp.Diff(tc.expected, actual, protocmp.Transform()); diff != "" {
				t.Errorf("\nGetFlakiness produced unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

func almostEqual(a, b float32) bool {
	return (a-b < .01) && (b-a < .01)
}

func TestCalculateFlipFlakiness(t *testing.T) {
	p := StatusPass
	f := StatusFail
	fl := StatusFlaky
	cases := []struct {
		name     string
		results  []StatusCategory
		expected float32
	}{
		{
			name:     "Empty List",
			results:  []StatusCategory{},
			expected: 0,
		},
		{
			name:     "All passing",
			results:  []StatusCategory{p, p, p},
			expected: 0,
		},
		{
			name:     "Multiple flakes",
			results:  []StatusCategory{p, f, p, p, fl, p, p, p, p, f},
			expected: 30,
		},
		{
			name:     "Short run",
			results:  []StatusCategory{p, f, f, p, p, p, p, f, p, p},
			expected: 20,
		},
		{
			name:     "Long run",
			results:  []StatusCategory{p, f, f, f, f, f, p, f, p, f},
			expected: 40,
		},
		{
			name:     "Long run interupted by flakes",
			results:  []StatusCategory{p, f, f, fl, f, f, p, f, p, f},
			expected: 50,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual := calculateFlipFlakiness(tc.results); !almostEqual(actual, tc.expected) {
				t.Errorf("enhancedFlakiness(%+v)=%v; expected %v", tc.results, actual, tc.expected)
			}
		})
	}
}
