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

package updater

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/testing/protocmp"
	core "k8s.io/api/core/v1"

	"github.com/GoogleCloudPlatform/testgrid/metadata"
	"github.com/GoogleCloudPlatform/testgrid/metadata/junit"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	statuspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
)

func TestMergeCells(t *testing.T) {
	cases := []struct {
		name     string
		cells    []cell
		expected cell
	}{
		{
			name: "basically works",
			cells: []cell{
				{
					result:  statuspb.TestStatus_TOOL_FAIL,
					cellID:  "random",
					icon:    "religious",
					message: "empty",
					metrics: map[string]float64{
						"answer":   42,
						"question": 1,
					},
				},
			},
			expected: cell{
				result:  statuspb.TestStatus_TOOL_FAIL,
				cellID:  "random",
				icon:    "religious",
				message: "empty",
				metrics: map[string]float64{
					"answer":   42,
					"question": 1,
				},
			},
		},
		{
			name: "passes work and take first filled message",
			cells: []cell{
				{
					result: statuspb.TestStatus_PASS,
					icon:   "drop",
				},
				{
					result:  statuspb.TestStatus_BUILD_PASSED,
					message: "woah",
				},
				{
					result:  statuspb.TestStatus_PASS,
					message: "there",
				},
			},
			expected: cell{
				result:  statuspb.TestStatus_PASS,
				message: "3/3 runs passed: woah",
				icon:    "3/3",
			},
		},
		{
			name: "merge metrics",
			cells: []cell{
				{
					result: statuspb.TestStatus_PASS,
					metrics: map[string]float64{
						"common": 1,
						"first":  1,
					},
				},
				{
					result: statuspb.TestStatus_PASS,
					metrics: map[string]float64{
						"common": 2,
						"second": 2,
					},
				},
				{
					result: statuspb.TestStatus_PASS,
					metrics: map[string]float64{
						"common": 108, // total 111
						"third":  3,
					},
				},
			},
			expected: cell{
				result:  statuspb.TestStatus_PASS,
				message: "3/3 runs passed",
				icon:    "3/3",
				metrics: map[string]float64{
					"common": 37,
					"first":  1,
					"second": 2,
					"third":  3,
				},
			},
		},
		{
			name: "failures take highest failure, first failure message",
			cells: []cell{
				{
					result:  statuspb.TestStatus_TIMED_OUT,
					message: "agonizingly slow",
					icon:    "drop",
				},
				{
					result: statuspb.TestStatus_BUILD_FAIL,
					icon:   "drop",
				},
				{
					result: statuspb.TestStatus_CATEGORIZED_FAIL,
					icon:   "drop",
				},
			},
			expected: cell{
				result:  statuspb.TestStatus_BUILD_FAIL,
				icon:    "0/3",
				message: "0/3 runs passed: agonizingly slow",
			},
		},
		{
			name: "mix of passes and failures flake",
			cells: []cell{
				{
					result:  statuspb.TestStatus_PASS,
					message: "yay",
				},
				{
					result:  statuspb.TestStatus_FAIL,
					message: "boom",
				},
			},
			expected: cell{
				result:  statuspb.TestStatus_FLAKY,
				icon:    "1/2",
				message: "1/2 runs passed: boom",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := mergeCells(tc.cells...)
			if diff := cmp.Diff(actual, tc.expected, cmp.AllowUnexported(cell{})); diff != "" {
				t.Errorf("mergeCells() got unexpected diff (-have, +want):\n%s", diff)
			}
		})
	}
}

func TestSplitCells(t *testing.T) {
	const cellName = "foo"
	cases := []struct {
		name     string
		cells    []cell
		expected map[string]cell
	}{
		{
			name: "basically works",
		},
		{
			name:  "single item returns that item",
			cells: []cell{{message: "hi"}},
			expected: map[string]cell{
				"foo": {message: "hi"},
			},
		},
		{
			name: "multiple items have [1] starting from second",
			cells: []cell{
				{message: "first"},
				{message: "second"},
				{message: "third"},
			},
			expected: map[string]cell{
				"foo":     {message: "first"},
				"foo [1]": {message: "second"},
				"foo [2]": {message: "third"},
			},
		},
		{
			name: "many items eventually truncate",
			cells: func() []cell {
				var out []cell
				for i := 0; i < maxDuplicates*2; i++ {
					out = append(out, cell{icon: fmt.Sprintf("row %d", i)})
				}
				return out
			}(),
			expected: func() map[string]cell {
				out := map[string]cell{}
				out[cellName] = cell{icon: "row 0"}
				for i := 1; i < maxDuplicates; i++ {
					name := fmt.Sprintf("%s [%d]", cellName, i)
					out[name] = cell{icon: fmt.Sprintf("row %d", i)}
				}
				out[cellName+" [overflow]"] = overflowCell
				return out
			}(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := splitCells(cellName, tc.cells...)
			if diff := cmp.Diff(actual, tc.expected, cmp.AllowUnexported(cell{})); diff != "" {
				t.Errorf("splitCells() got unexpected diff (-have, +want):\n%s", diff)
			}
		})
	}
}

func TestConvertResult(t *testing.T) {
	pint := func(v int64) *int64 {
		return &v
	}
	pstr := func(s string) *string {
		return &s
	}
	yes := true
	now := time.Now().Unix()
	cases := []struct {
		name      string
		nameCfg   nameConfig
		id        string
		headers   []string
		metricKey string
		result    gcsResult
		opt       *groupOptions
		expected  *inflatedColumn
	}{
		{
			name: "basically works",
			opt:  &groupOptions{}, // turn off podinfo
			expected: &inflatedColumn{
				column: &statepb.Column{},
				cells: map[string]cell{
					overallRow: {
						result:  statuspb.TestStatus_FAIL,
						icon:    "T",
						message: "Build did not complete within 24 hours",
					},
				},
			},
		},
		{
			name:    "correct column information",
			headers: []string{"Commit", "hello", "spam", "do not have this one"},
			id:      "hello",
			result: gcsResult{
				started: gcs.Started{
					Started: metadata.Started{
						Timestamp: 300,
					},
				},
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Metadata: metadata.Metadata{
							"hello":             "world",
							"spam":              "eggs",
							metadata.JobVersion: "1.2.3",
						},
					},
				},
				podInfo: podInfoSuccessPodInfo,
			},
			expected: &inflatedColumn{
				column: &statepb.Column{
					Build:   "hello",
					Started: 300 * 1000,
					Extra: []string{
						"1.2.3",
						"world",
						"eggs",
						"missing",
					},
				},
				cells: map[string]cell{
					overallRow: {
						result:  statuspb.TestStatus_FAIL,
						icon:    "T",
						message: "Build did not complete within 24 hours",
					},
					podInfoRow: podInfoPassCell,
				},
			},
		},
		{
			name:    "running results do not have missing column headers",
			headers: []string{"Commit", "hello", "spam", "do not have this one"},
			id:      "hello",
			result: gcsResult{
				started: gcs.Started{
					Started: metadata.Started{
						Timestamp: now,
					},
				},
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Metadata: metadata.Metadata{
							"hello":             "world",
							"spam":              "eggs",
							metadata.JobVersion: "1.2.3",
						},
					},
				},
			},
			expected: &inflatedColumn{
				column: &statepb.Column{
					Build:   "hello",
					Started: float64(now * 1000),
					Extra: []string{
						"1.2.3",
						"world",
						"eggs",
						"", // not missing
					},
				},
				cells: map[string]cell{
					overallRow: {
						result:  statuspb.TestStatus_RUNNING,
						icon:    "R",
						message: "Build still running...",
					},
				},
			},
		},
		{
			name: "add job overall when multiJob",
			id:   "build",
			nameCfg: nameConfig{
				format:   "%s.%s",
				parts:    []string{jobName, testsName},
				multiJob: true,
			},
			result: gcsResult{
				started: gcs.Started{
					Started: metadata.Started{
						Timestamp: now,
					},
				},
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Timestamp: pint(now + 1),
					},
				},
				suites: []gcs.SuitesMeta{
					{
						Suites: junit.Suites{
							Suites: []junit.Suite{
								{
									Name: "this",
									Results: []junit.Result{
										{
											Name: "that",
										},
									},
								},
							},
						},
					},
				},
				job: "job-name",
			},
			expected: &inflatedColumn{
				column: &statepb.Column{
					Started: float64(now * 1000),
					Build:   "build",
				},
				cells: map[string]cell{
					overallRow: {
						result:  statuspb.TestStatus_FAIL,
						icon:    "F",
						message: "Build failed outside of test results",
						metrics: setElapsed(nil, 1),
						cellID:  "job-name/build",
					},
					podInfoRow: func() cell {
						c := podInfoMissingCell
						c.cellID = "job-name/build"
						return c
					}(),
					"job-name.Pod": func() cell {
						c := podInfoMissingCell
						c.cellID = "job-name/build"
						return c
					}(),
					"job-name.Overall": {
						result:  statuspb.TestStatus_FAIL,
						icon:    "F",
						message: "Build failed outside of test results",
						metrics: setElapsed(nil, 1),
						cellID:  "job-name/build",
					},
					"job-name.this.that": {
						result: statuspb.TestStatus_PASS,
						cellID: "job-name/build",
					},
				},
			},
		},
		{
			name: "inclue job name upon request",
			nameCfg: nameConfig{
				format: "%s.%s",
				parts:  []string{jobName, testsName},
			},
			result: gcsResult{
				started: gcs.Started{
					Started: metadata.Started{
						Timestamp: now,
					},
				},
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Timestamp: pint(now + 1),
					},
				},
				suites: []gcs.SuitesMeta{
					{
						Suites: junit.Suites{
							Suites: []junit.Suite{
								{
									Name: "this",
									Results: []junit.Result{
										{
											Name: "that",
										},
									},
								},
							},
						},
					},
				},
				job: "job-name",
			},
			expected: &inflatedColumn{
				column: &statepb.Column{
					Started: float64(now * 1000),
				},
				cells: map[string]cell{
					overallRow: {
						result:  statuspb.TestStatus_FAIL,
						icon:    "F",
						message: "Build failed outside of test results",
						metrics: setElapsed(nil, 1),
					},
					podInfoRow: podInfoMissingCell,
					"job-name.this.that": {
						result: statuspb.TestStatus_PASS,
					},
				},
			},
		},
		{
			name: "failing job with only passing results has a failing overall message",
			nameCfg: nameConfig{
				format: "%s",
				parts:  []string{testsName},
			},
			result: gcsResult{
				started: gcs.Started{
					Started: metadata.Started{
						Timestamp: now,
					},
				},
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Timestamp: pint(now + 1),
					},
				},
				suites: []gcs.SuitesMeta{
					{
						Suites: junit.Suites{
							Suites: []junit.Suite{
								{
									Name: "this",
									Results: []junit.Result{
										{
											Name: "that",
										},
									},
								},
							},
						},
					},
				},
			},
			expected: &inflatedColumn{
				column: &statepb.Column{
					Started: float64(now * 1000),
				},
				cells: map[string]cell{
					overallRow: {
						result:  statuspb.TestStatus_FAIL,
						icon:    "F",
						message: "Build failed outside of test results",
						metrics: setElapsed(nil, 1),
					},
					podInfoRow: podInfoMissingCell,
					"this.that": {
						result: statuspb.TestStatus_PASS,
					},
				},
			},
		},
		{
			name: "result fields parsed properly",
			nameCfg: nameConfig{
				format: "%s",
				parts:  []string{testsName},
			},
			result: gcsResult{
				started: gcs.Started{
					Started: metadata.Started{
						Timestamp: now,
					},
				},
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Timestamp: pint(now + 1),
					},
				},
				suites: []gcs.SuitesMeta{
					{
						Suites: junit.Suites{
							Suites: []junit.Suite{
								{
									Results: []junit.Result{
										{
											Name: "elapsed",
											Time: 5,
										},
										{
											Name:    "failed no message",
											Failure: pstr(""),
										},
										{
											Name:    "failed",
											Failure: pstr("boom"),
										},
										{
											Name:    "failed other message",
											Failure: pstr(""),
											Output:  pstr("irrelevant message"),
										},
										{
											Name:    "invisible skip",
											Skipped: pstr(""),
										},
										{
											Name:    "visible skip",
											Skipped: pstr("tl;dr"),
										},
										{
											Name:  "stderr message",
											Error: pstr("ouch"),
										},
										{
											Name:   "stdout message",
											Output: pstr("bellybutton"),
										},
									},
								},
							},
						},
					},
				},
			},
			expected: &inflatedColumn{
				column: &statepb.Column{
					Started: float64(now * 1000),
				},
				cells: map[string]cell{
					overallRow: {
						result:  statuspb.TestStatus_FAIL,
						metrics: setElapsed(nil, 1),
					},
					podInfoRow: podInfoMissingCell,
					"elapsed": {
						result:  statuspb.TestStatus_PASS,
						metrics: setElapsed(nil, 5),
					},
					"failed no message": {
						result: statuspb.TestStatus_FAIL,
					},
					"failed": {
						message: "boom",
						result:  statuspb.TestStatus_FAIL,
						icon:    "F",
					},
					"failed other message": {
						message: "irrelevant message",
						result:  statuspb.TestStatus_FAIL,
						icon:    "F",
					},
					// no invisible skip
					"visible skip": {
						result:  statuspb.TestStatus_PASS_WITH_SKIPS,
						message: "tl;dr",
						icon:    "S",
					},
					"stderr message": {
						message: "ouch",
						result:  statuspb.TestStatus_PASS,
					},
					"stdout message": {
						message: "bellybutton",
						result:  statuspb.TestStatus_PASS,
					},
				},
			},
		},
		{
			name: "icon set by metric key",
			nameCfg: nameConfig{
				format: "%s",
				parts:  []string{testsName},
			},
			metricKey: "food",
			result: gcsResult{
				started: gcs.Started{
					Started: metadata.Started{
						Timestamp: now,
					},
				},
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Timestamp: pint(now + 1),
					},
				},
				suites: []gcs.SuitesMeta{
					{
						Suites: junit.Suites{
							Suites: []junit.Suite{
								{
									Results: []junit.Result{
										{
											Name: "no properties",
										},
										{
											Name: "missing property",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{"random", "thing"},
												},
											},
										},
										{
											Name: "not a number",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{"food", "tasty"},
												},
											},
										},
										{
											Name: "short number",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{"food", "123"},
												},
											},
										},
										{
											Name: "large number",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{"food", "123456789"},
												},
											},
										},
										{
											Name: "many digits",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{"food", "1.567890"},
												},
											},
										},
										{
											Name: "multiple values",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{"food", "1"},
													{"food", "2"},
													{"food", "3"},
													{"food", "4"},
												},
											},
										},
										{
											Name: "preceds failure message",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{"food", "1"},
												},
											},
											Failure: pstr("boom"),
										},
										{
											Name: "preceds skip message",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{"food", "1"},
												},
											},
											Skipped: pstr("tl;dr"),
										},
									},
								},
							},
						},
					},
				},
			},
			expected: &inflatedColumn{
				column: &statepb.Column{
					Started: float64(now * 1000),
				},
				cells: map[string]cell{
					overallRow: {
						result:  statuspb.TestStatus_FAIL,
						metrics: setElapsed(nil, 1),
					},
					podInfoRow: podInfoMissingCell,
					"no properties": {
						result: statuspb.TestStatus_PASS,
					},
					"missing property": {
						result: statuspb.TestStatus_PASS,
					},
					"not a number": {
						result: statuspb.TestStatus_PASS,
					},
					"short number": {
						result: statuspb.TestStatus_PASS,
						icon:   "123",
						metrics: map[string]float64{
							"food": 123,
						},
					},
					"large number": {
						result: statuspb.TestStatus_PASS,
						icon:   "1.235e+08",
						metrics: map[string]float64{
							"food": 123456789,
						},
					},
					"many digits": {
						result: statuspb.TestStatus_PASS,
						icon:   "1.568",
						metrics: map[string]float64{
							"food": 1.567890,
						},
					},
					"multiple values": {
						result: statuspb.TestStatus_PASS,
						icon:   "2.5",
						metrics: map[string]float64{
							"food": 2.5,
						},
					},
					"preceds failure message": {
						result:  statuspb.TestStatus_FAIL,
						message: "boom",
						icon:    "1",
						metrics: map[string]float64{
							"food": 1,
						},
					},
					"preceds skip message": {
						result:  statuspb.TestStatus_PASS_WITH_SKIPS,
						message: "tl;dr",
						icon:    "1",
						metrics: map[string]float64{
							"food": 1,
						},
					},
				},
			},
		},
		{
			name: "names formatted correctly",
			nameCfg: nameConfig{
				format: "%s - %s [%s] (%s)",
				parts:  []string{testsName, "extra", "part", "property"},
			},
			result: gcsResult{
				started: gcs.Started{
					Started: metadata.Started{
						Timestamp: now,
					},
				},
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Timestamp: pint(now + 1),
						Passed:    &yes,
					},
				},
				suites: []gcs.SuitesMeta{
					{
						Suites: junit.Suites{
							Suites: []junit.Suite{
								{
									Results: []junit.Result{
										{
											Name: "elapsed",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{"property", "good-property"},
													{"ignore", "me"},
												},
											},
										},
									},
								},
							},
						},
						Metadata: map[string]string{
							"extra": "first",
							"part":  "second",
						},
					},
					{
						Suites: junit.Suites{
							Suites: []junit.Suite{
								{
									Results: []junit.Result{
										{
											Name: "other",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													// "property" missing
													{"ignore", "me"},
												},
											},
										},
									},
								},
							},
						},
						Metadata: map[string]string{
							"extra": "hey",
							// part missing
						},
					},
				},
			},
			expected: &inflatedColumn{
				column: &statepb.Column{
					Started: float64(now * 1000),
				},
				cells: map[string]cell{
					overallRow: {
						result:  statuspb.TestStatus_PASS,
						metrics: setElapsed(nil, 1),
					},
					podInfoRow: podInfoMissingCell,
					"elapsed - first [second] (good-property)": {
						result: statuspb.TestStatus_PASS,
					},
					"other - hey [] ()": {
						result: statuspb.TestStatus_PASS,
					},
				},
			},
		},
		{
			name: "duplicate row names can be merged",
			nameCfg: nameConfig{
				format: "%s - %s",
				parts:  []string{testsName, "extra"},
			},
			opt: &groupOptions{
				merge: true,
			},
			result: gcsResult{
				started: gcs.Started{
					Started: metadata.Started{
						Timestamp: now,
					},
				},
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Timestamp: pint(now + 1),
						Passed:    &yes,
					},
				},
				suites: []gcs.SuitesMeta{
					{
						Suites: junit.Suites{
							Suites: []junit.Suite{
								{
									Results: []junit.Result{
										{
											Name: "same",
											Time: 1,
										},
										{
											Name: "same",
											Time: 2,
										},
									},
								},
							},
						},
						Metadata: map[string]string{
							"extra": "same",
						},
					},
					{
						Suites: junit.Suites{
							Suites: []junit.Suite{
								{
									Results: []junit.Result{
										{
											Name:    "same",
											Time:    3,
											Failure: pstr("ugh"),
										},
									},
								},
							},
						},
						Metadata: map[string]string{
							"extra": "same",
						},
					},
				},
			},
			expected: &inflatedColumn{
				column: &statepb.Column{
					Started: float64(now * 1000),
				},
				cells: map[string]cell{
					overallRow: {
						result:  statuspb.TestStatus_PASS,
						metrics: setElapsed(nil, 1),
					},
					"same - same": {
						result:  statuspb.TestStatus_FLAKY,
						icon:    "2/3",
						message: "2/3 runs passed: ugh",
						metrics: setElapsed(nil, 2), // mean
					},
				},
			},
		},
		{
			name: "duplicate row names can be disambiguated",
			nameCfg: nameConfig{
				format: "%s - %s",
				parts:  []string{testsName, "extra"},
			},
			result: gcsResult{
				started: gcs.Started{
					Started: metadata.Started{
						Timestamp: now,
					},
				},
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Timestamp: pint(now + 1),
						Passed:    &yes,
					},
				},
				suites: []gcs.SuitesMeta{
					{
						Suites: junit.Suites{
							Suites: []junit.Suite{
								{
									Results: []junit.Result{
										{
											Name: "same",
											Time: 1,
										},
										{
											Name: "same",
											Time: 2,
										},
									},
								},
							},
						},
						Metadata: map[string]string{
							"extra": "same",
						},
					},
					{
						Suites: junit.Suites{
							Suites: []junit.Suite{
								{
									Results: []junit.Result{
										{
											Name: "same",
											Time: 3,
										},
									},
								},
							},
						},
						Metadata: map[string]string{
							"extra": "same",
						},
					},
				},
			},
			expected: &inflatedColumn{
				column: &statepb.Column{
					Started: float64(now * 1000),
				},
				cells: map[string]cell{
					overallRow: {
						result:  statuspb.TestStatus_PASS,
						metrics: setElapsed(nil, 1),
					},
					podInfoRow: podInfoMissingCell,
					"same - same": {
						result:  statuspb.TestStatus_PASS,
						metrics: setElapsed(nil, 1),
					},
					"same - same [1]": {
						result:  statuspb.TestStatus_PASS,
						metrics: setElapsed(nil, 2),
					},
					"same - same [2]": {
						result:  statuspb.TestStatus_PASS,
						metrics: setElapsed(nil, 3),
					},
				},
			},
		},
		{
			name: "excessively duplicated rows overflows",
			nameCfg: nameConfig{
				format: "%s",
				parts:  []string{testsName},
			},
			result: gcsResult{
				started: gcs.Started{
					Started: metadata.Started{
						Timestamp: now,
					},
				},
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Timestamp: pint(now + 1),
						Passed:    &yes,
					},
				},
				suites: []gcs.SuitesMeta{
					{
						Suites: junit.Suites{
							Suites: []junit.Suite{
								{
									Results: func() []junit.Result {
										var out []junit.Result
										under := junit.Result{Name: "under"}
										max := junit.Result{Name: "max"}
										over := junit.Result{Name: "over"}
										for i := 0; i < maxDuplicates; i++ {
											under.Time = float64(i)
											max.Time = float64(i)
											over.Time = float64(i)
											out = append(out, under, max, over)
										}
										max.Time = maxDuplicates
										over.Time = maxDuplicates
										out = append(out, max, over)
										over.Time++
										out = append(out, over)
										return out
									}(),
								},
							},
						},
						Metadata: map[string]string{
							"extra": "same",
						},
					},
				},
			},
			expected: &inflatedColumn{
				column: &statepb.Column{
					Started: float64(now * 1000),
				},
				cells: func() map[string]cell {
					out := map[string]cell{
						overallRow: {
							result:  statuspb.TestStatus_PASS,
							metrics: setElapsed(nil, 1),
						},
						podInfoRow: podInfoMissingCell,
					}
					under := cell{result: statuspb.TestStatus_PASS}
					max := cell{result: statuspb.TestStatus_PASS}
					over := cell{result: statuspb.TestStatus_PASS}
					out["under"] = under
					out["max"] = max
					out["over"] = over
					for i := 1; i < maxDuplicates; i++ {
						t := float64(i)
						under.metrics = setElapsed(nil, t)
						out[fmt.Sprintf("under [%d]", i)] = under
						max.metrics = setElapsed(nil, t)
						out[fmt.Sprintf("max [%d]", i)] = max
						over.metrics = setElapsed(nil, t)
						out[fmt.Sprintf("over [%d]", i)] = over
					}
					max.metrics = setElapsed(nil, maxDuplicates)
					out[`max [overflow]`] = overflowCell
					out[`over [overflow]`] = overflowCell
					return out
				}(),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			log := logrus.WithField("test name", tc.name)
			if tc.opt == nil {
				tc.opt = &groupOptions{
					analyzeProwJob: true,
				}
			}
			actual, err := convertResult(log, tc.nameCfg, tc.id, tc.headers, tc.metricKey, tc.result, *tc.opt)
			switch {
			case err != nil:
				if tc.expected != nil {
					t.Errorf("convertResult() got unexpected error: %v", err)
				}
			case tc.expected == nil:
				t.Error("convertResult() failed to return an error")
			default:
				if diff := cmp.Diff(actual, tc.expected, cmp.AllowUnexported(inflatedColumn{}, cell{}), protocmp.Transform()); diff != "" {
					t.Errorf("convertResult() got unexpected diff (-have, +want):\n%s", diff)
				}
			}
		})
	}
}

func TestPodInfoCell(t *testing.T) {
	cases := []struct {
		name     string
		podInfo  gcs.PodInfo
		expected cell
	}{
		{
			name:     "basically works",
			expected: podInfoMissingCell,
		},
		{
			name:     "passing result works",
			podInfo:  podInfoSuccessPodInfo,
			expected: podInfoPassCell,
		},
		{
			name:    "no pod utils works",
			podInfo: gcs.PodInfo{Pod: &core.Pod{}},
			expected: cell{
				message: gcs.NoPodUtils,
				icon:    "E",
				result:  statuspb.TestStatus_PASS,
			},
		},
		{
			name: "failure works",
			podInfo: gcs.PodInfo{
				Pod: &core.Pod{
					Status: core.PodStatus{
						Conditions: []core.PodCondition{
							{
								Type:    core.PodScheduled,
								Status:  core.ConditionFalse,
								Message: "hi there",
							},
						},
					},
				},
			},
			expected: cell{
				message: "pod did not schedule: hi there",
				icon:    "F",
				result:  statuspb.TestStatus_FAIL,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := podInfoCell(tc.podInfo)
			if diff := cmp.Diff(actual, tc.expected, cmp.AllowUnexported(cell{})); diff != "" {
				t.Errorf("podInfoCell(%s) got unexpected diff (-have, +want):\n%s", tc.podInfo, diff)
			}
		})
	}
}

func TestOverallCell(t *testing.T) {
	pint := func(v int64) *int64 {
		return &v
	}
	yes := true
	var no bool
	cases := []struct {
		name     string
		result   gcsResult
		expected cell
	}{
		{
			name: "result timed out",
			result: gcsResult{
				started: gcs.Started{
					Started: metadata.Started{
						Timestamp: time.Now().Add(-25 * time.Hour).Unix(),
					},
				},
			},
			expected: cell{
				result:  statuspb.TestStatus_FAIL,
				message: "Build did not complete within 24 hours",
				icon:    "T",
			},
		},
		{
			name: "passed result passes",
			result: gcsResult{
				started: gcs.Started{
					Started: metadata.Started{
						Timestamp: 100,
					},
				},
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Timestamp: pint(250),
						Passed:    &yes,
					},
				},
			},
			expected: cell{
				result:  statuspb.TestStatus_PASS,
				metrics: setElapsed(nil, 150),
			},
		},
		{
			name: "failed via deprecated result",
			result: gcsResult{
				started: gcs.Started{
					Started: metadata.Started{
						Timestamp: 100,
					},
				},
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Timestamp: pint(250),
						Result:    "BOOM",
					},
				},
			},
			expected: cell{
				result:  statuspb.TestStatus_FAIL,
				icon:    "E",
				message: `finished.json missing "passed": false`,
				metrics: setElapsed(nil, 150),
			},
		},
		{
			name: "passed via deprecated result",
			result: gcsResult{
				started: gcs.Started{
					Started: metadata.Started{
						Timestamp: 100,
					},
				},
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Timestamp: pint(250),
						Result:    "SUCCESS",
					},
				},
			},
			expected: cell{
				result:  statuspb.TestStatus_PASS,
				icon:    "E",
				message: `finished.json missing "passed": true`,
				metrics: setElapsed(nil, 150),
			},
		},
		{
			name: "failed result fails",
			result: gcsResult{
				started: gcs.Started{
					Started: metadata.Started{
						Timestamp: 100,
					},
				},
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Timestamp: pint(250),
						Passed:    &no,
					},
				},
			},
			expected: cell{
				result:  statuspb.TestStatus_FAIL,
				metrics: setElapsed(nil, 150),
			},
		},
		{
			name: "missing passed field is a failure",
			result: gcsResult{
				started: gcs.Started{
					Started: metadata.Started{
						Timestamp: 100,
					},
				},
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Timestamp: pint(250),
					},
				},
			},
			expected: cell{
				result:  statuspb.TestStatus_FAIL,
				metrics: setElapsed(nil, 150),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := overallCell(tc.result)
			if diff := cmp.Diff(actual, tc.expected, cmp.AllowUnexported(cell{})); diff != "" {
				t.Errorf("overallCell(%v) got unexpected diff:\n%s", tc.result, diff)
			}
		})
	}
}

func TestSetElapsed(t *testing.T) {
	cases := []struct {
		name     string
		metrics  map[string]float64
		seconds  float64
		expected map[string]float64
	}{
		{
			name:    "nil map works",
			seconds: 10,
			expected: map[string]float64{
				elapsedKey: 10 / 60.0,
			},
		},
		{
			name: "existing keys preserved",
			metrics: map[string]float64{
				"hello": 7,
			},
			seconds: 5,
			expected: map[string]float64{
				"hello":    7,
				elapsedKey: 5 / 60.0,
			},
		},
		{
			name: "override existing value",
			metrics: map[string]float64{
				elapsedKey: 3 / 60.0,
			},
			seconds: 10,
			expected: map[string]float64{
				elapsedKey: 10 / 60.0,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := setElapsed(tc.metrics, tc.seconds)
			if !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("setElapsed(%v, %v) got %v, want %v", tc.metrics, tc.seconds, actual, tc.expected)
			}
		})
	}
}

func TestFlattenResults(t *testing.T) {
	pstr := func(s string) *string {
		return &s
	}
	cases := []struct {
		name     string
		suites   []junit.Suite
		expected []junit.Result
	}{
		{
			name: "basically works",
		},
		{
			name: "results from multiple suites",
			suites: []junit.Suite{
				{
					Name: "suite1",
					Results: []junit.Result{
						{
							Name:   "resultA",
							Output: pstr("hello"),
						},
						{
							Name:  "resultB",
							Error: pstr("bonk"),
						},
					},
				},
				{
					Name: "suite-two",
					Results: []junit.Result{
						{
							Name: "resultX",
						},
					},
				},
			},
			expected: []junit.Result{
				{
					Name:   "suite1.resultA",
					Output: pstr("hello"),
				},
				{
					Name:  "suite1.resultB",
					Error: pstr("bonk"),
				},
				{
					Name: "suite-two.resultX",
				},
			},
		},
		{
			name: "find results deeply nested in suites",
			suites: []junit.Suite{
				{
					Name: "must",
					Suites: []junit.Suite{
						{
							Name: "go",
							Suites: []junit.Suite{
								{
									Name: "deeper",
									Results: []junit.Result{
										{
											Name:    "leaf",
											Skipped: pstr("first"),
										},
									},
								},
							},
							Results: []junit.Result{
								{
									Name:    "branch",
									Skipped: pstr("second"),
								},
							},
						},
					},
					Results: []junit.Result{
						{
							Name:    "trunk",
							Skipped: pstr("third"),
						},
					},
				},
			},
			expected: []junit.Result{
				{
					Name:    "must.go.deeper.leaf",
					Skipped: pstr("first"),
				},
				{
					Name:    "must.go.branch",
					Skipped: pstr("second"),
				},
				{
					Name:    "must.trunk",
					Skipped: pstr("third"),
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := flattenResults(tc.suites...)
			if !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("flattenResults(%v) got %v, want %v", tc.suites, actual, tc.expected)
			}
		})
	}
}

func TestDotName(t *testing.T) {
	cases := []struct {
		name     string
		left     string
		right    string
		expected string
	}{
		{
			name: "basically works",
		},
		{
			name:     "left.right",
			left:     "left",
			right:    "right",
			expected: "left.right",
		},
		{
			name:     "only left",
			left:     "left",
			expected: "left",
		},
		{
			name:     "only right",
			right:    "right",
			expected: "right",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual := dotName(tc.left, tc.right); actual != tc.expected {
				t.Errorf("dotName(%q, %q) got %q, want %q", tc.left, tc.right, actual, tc.expected)
			}
		})
	}
}
