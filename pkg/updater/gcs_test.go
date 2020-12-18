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
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/GoogleCloudPlatform/testgrid/metadata"
	"github.com/GoogleCloudPlatform/testgrid/metadata/junit"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	statuspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
)

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
		name     string
		ctx      context.Context
		nameCfg  nameConfig
		id       string
		headers  []string
		result   gcsResult
		expected *inflatedColumn
	}{
		{
			name: "basically works",
			expected: &inflatedColumn{
				column: &statepb.Column{},
				cells: map[string]cell{
					"Overall": {
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
					"Overall": {
						result:  statuspb.TestStatus_FAIL,
						icon:    "T",
						message: "Build did not complete within 24 hours",
					},
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
					"Overall": {
						result:  statuspb.TestStatus_RUNNING,
						icon:    "R",
						message: "Build still running...",
					},
				},
			},
		},
		{
			name: "failing job with only passing results has a failing overall message",
			nameCfg: nameConfig{
				format: "%s",
				parts:  []string{"Tests name"},
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
					"Overall": {
						result:  statuspb.TestStatus_FAIL,
						icon:    "F",
						message: "Build failed outside of test results",
						metrics: setElapsed(nil, 1),
					},
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
				parts:  []string{"Tests name"},
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
					"Overall": {
						result:  statuspb.TestStatus_FAIL,
						metrics: setElapsed(nil, 1),
					},
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
			name: "names formatted correctly",
			nameCfg: nameConfig{
				format: "%s - %s [%s]",
				parts:  []string{"Tests name", "extra", "part"},
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
					"Overall": {
						result:  statuspb.TestStatus_PASS,
						metrics: setElapsed(nil, 1),
					},
					"elapsed - first [second]": {
						result: statuspb.TestStatus_PASS,
					},
					"other - hey []": {
						result: statuspb.TestStatus_PASS,
					},
				},
			},
		},
		{
			name: "duplicate row names disambiguated",
			nameCfg: nameConfig{
				format: "%s - %s",
				parts:  []string{"Tests name", "extra"},
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
					"Overall": {
						result:  statuspb.TestStatus_PASS,
						metrics: setElapsed(nil, 1),
					},
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
			name: "cancelled context returns error",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			nameCfg: nameConfig{
				format: "%s - %s",
				parts:  []string{"Tests name", "extra"},
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
		},
		{
			name: "excessively duplicated rows overflows",
			nameCfg: nameConfig{
				format: "%s",
				parts:  []string{"Tests name"},
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
						"Overall": {
							result:  statuspb.TestStatus_PASS,
							metrics: setElapsed(nil, 1),
						},
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
			if tc.ctx == nil {
				tc.ctx = context.Background()
			}
			ctx, cancel := context.WithCancel(tc.ctx)
			defer cancel()
			log := logrus.WithField("test name", tc.name)
			actual, err := convertResult(ctx, log, tc.nameCfg, tc.id, tc.headers, tc.result)
			switch {
			case err != nil:
				if tc.expected != nil {
					t.Errorf("convertResult() got unexpected error: %v", err)
				}
			case tc.expected == nil:
				t.Error("convertResult() failed to return an error")
			default:
				if diff := cmp.Diff(actual, tc.expected, cmp.AllowUnexported(inflatedColumn{}, cell{}), protocmp.Transform()); diff != "" {
					t.Errorf("convertResult() got unexpected diff (-have, +want):\n", diff)
				}
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
			if !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("overallCell(%v) got %v, want %v", tc.result, actual, tc.expected)
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
