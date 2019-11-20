/*
Copyright 2018 The Kubernetes Authors.

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

package main

import (
	"reflect"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"

	"github.com/GoogleCloudPlatform/testgrid/metadata/junit"
	"github.com/GoogleCloudPlatform/testgrid/pb/state"
)

func TestExtractRows(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		metadata map[string]string
		rows     map[string][]Row
		err      bool
	}{
		{
			name:    "not xml",
			content: `{"super": 123}`,
			err:     true,
		},
		{
			name:    "not junit",
			content: `<amazing><content/></amazing>`,
			err:     true,
		},
		{
			name: "basic testsuite",
			content: `
			  <testsuite>
			    <testcase name="good"/>
			    <testcase name="bad"><failure/></testcase>
			    <testcase name="skip"><skipped/></testcase>
			  </testsuite>`,
			rows: map[string][]Row{
				"good": {
					{
						Result: state.Row_PASS,
						Metadata: map[string]string{
							"Tests name": "good",
						},
					},
				},
				"bad": {
					{
						Result: state.Row_FAIL,
						Metadata: map[string]string{
							"Tests name": "bad",
						},
					},
				},
			},
		},
		{
			name: "basic testsuites",
			content: `
			  <testsuites>
			  <testsuite>
			    <testcase name="good"/>
			  </testsuite>
			  <testsuite>
			    <testcase name="bad"><failure/></testcase>
			    <testcase name="skip"><skipped/></testcase>
			  </testsuite>
			  </testsuites>`,
			rows: map[string][]Row{
				"good": {
					{
						Result: state.Row_PASS,
						Metadata: map[string]string{
							"Tests name": "good",
						},
					},
				},
				"bad": {
					{
						Result: state.Row_FAIL,
						Metadata: map[string]string{
							"Tests name": "bad",
						},
					},
				},
			},
		},
		{
			name: "suite name",
			content: `
			  <testsuite name="hello">
			    <testcase name="world" />
			  </testsuite>`,
			rows: map[string][]Row{
				"hello.world": {
					{
						Result: state.Row_PASS,
						Metadata: map[string]string{
							"Tests name": "hello.world",
						},
					},
				},
			},
		},
		{
			name: "duplicate target names",
			content: `
			  <testsuite>
			    <testcase name="multi">
			      <failure>doh</failure>
		            </testcase>
			    <testcase name="multi" />
			  </testsuite>`,
			rows: map[string][]Row{
				"multi": {
					{
						Result: state.Row_FAIL,
						Metadata: map[string]string{
							"Tests name": "multi",
						},
					},
					{
						Result: state.Row_PASS,
						Metadata: map[string]string{
							"Tests name": "multi",
						},
					},
				},
			},
		},
		{
			name: "basic timing",
			content: `
			  <testsuite>
			    <testcase name="slow" time="100.1" />
			    <testcase name="slow-failure" time="123456789">
			      <failure>terrible</failure>
			    </testcase>
			    <testcase name="fast" time="0.0001" />
			    <testcase name="nothing-elapsed" time="0" />
			  </testsuite>`,
			rows: map[string][]Row{
				"slow": {
					{
						Result:  state.Row_PASS,
						Metrics: map[string]float64{elapsedKey: 100.1},
						Metadata: map[string]string{
							"Tests name": "slow",
						},
					},
				},
				"slow-failure": {
					{
						Result:  state.Row_FAIL,
						Metrics: map[string]float64{elapsedKey: 123456789},
						Metadata: map[string]string{
							"Tests name": "slow-failure",
						},
					},
				},
				"fast": {
					{
						Result:  state.Row_PASS,
						Metrics: map[string]float64{elapsedKey: 0.0001},
						Metadata: map[string]string{
							"Tests name": "fast",
						},
					},
				},
				"nothing-elapsed": {
					{
						Result: state.Row_PASS,
						Metadata: map[string]string{
							"Tests name": "nothing-elapsed",
						},
					},
				},
			},
		},
		{
			name: "add metadata",
			content: `
			  <testsuite>
			    <testcase name="fancy" />
			    <testcase name="ketchup" />
			  </testsuite>`,
			metadata: map[string]string{
				"Context":   "debian",
				"Timestamp": "1234",
				"Thread":    "7",
			},
			rows: map[string][]Row{
				"fancy": {
					{
						Result: state.Row_PASS,
						Metadata: map[string]string{
							"Tests name": "fancy",
							"Context":    "debian",
							"Timestamp":  "1234",
							"Thread":     "7",
						},
					},
				},
				"ketchup": {
					{
						Result: state.Row_PASS,
						Metadata: map[string]string{
							"Tests name": "ketchup",
							"Context":    "debian",
							"Timestamp":  "1234",
							"Thread":     "7",
						},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := map[string][]Row{}

			suites, err := junit.Parse([]byte(tc.content))
			if err == nil {
				rows = extractRows(suites, tc.metadata)
			}
			switch {
			case err == nil && tc.err:
				t.Error("failed to raise an error")
			case err != nil && !tc.err:
				t.Errorf("unexpected err: %v", err)
			case len(rows) > len(tc.rows):
				t.Errorf("extra rows: actual %v != expected %v", rows, tc.rows)
			default:
				for target, expectedRows := range tc.rows {
					actualRows, ok := rows[target]
					if !ok {
						t.Errorf("missing row %s", target)
						continue
					} else if len(actualRows) != len(expectedRows) {
						t.Errorf("bad results for %s: actual %v != expected %v", target, actualRows, expectedRows)
						continue
					}
					for i, er := range expectedRows {
						ar := actualRows[i]
						if er.Result != ar.Result {
							t.Errorf("%s %d actual %v != expected %v", target, i, ar.Result, er.Result)
						}

						if len(ar.Metrics) > len(er.Metrics) {
							t.Errorf("extra %s %d metrics: actual %v != expected %v", target, i, ar.Metrics, er.Metrics)
						} else {
							for m, ev := range er.Metrics {
								if av, ok := ar.Metrics[m]; !ok {
									t.Errorf("%s %d missing %s metric", target, i, m)
								} else if ev != av {
									t.Errorf("%s %d bad %s metric: actual %f != expected %f", target, i, m, av, ev)
								}
							}
						}

						if len(ar.Metadata) > len(er.Metadata) {
							t.Errorf("extra %s %d metadata: actual %v != expected %v", target, i, ar.Metadata, er.Metadata)
						} else {
							for m, ev := range er.Metadata {
								if av, ok := ar.Metadata[m]; !ok {
									t.Errorf("%s %d missing %s metadata", target, i, m)
								} else if ev != av {
									t.Errorf("%s %d bad %s metadata: actual %s != expected %s", target, i, m, av, ev)
								}
							}
						}
					}
				}
			}
		})
	}
}

func TestMarshalGrid(t *testing.T) {
	g1 := state.Grid{
		Columns: []*state.Column{
			{Build: "alpha"},
			{Build: "second"},
		},
	}
	g2 := state.Grid{
		Columns: []*state.Column{
			{Build: "first"},
			{Build: "second"},
		},
	}

	b1, e1 := marshalGrid(g1)
	b2, e2 := marshalGrid(g2)
	uncompressed, e1a := proto.Marshal(&g1)

	switch {
	case e1 != nil, e2 != nil:
		t.Errorf("unexpected error %v %v %v", e1, e2, e1a)
	}

	if reflect.DeepEqual(b1, b2) {
		t.Errorf("unexpected equality %v == %v", b1, b2)
	}

	if reflect.DeepEqual(b1, uncompressed) {
		t.Errorf("should be compressed but is not: %v", b1)
	}
}

func TestAlertRow(t *testing.T) {
	var columns []*state.Column
	for i, id := range []string{"a", "b", "c", "d", "e", "f"} {
		columns = append(columns, &state.Column{
			Build:   id,
			Started: 100 - float64(i),
		})
	}
	cases := []struct {
		name      string
		row       state.Row
		failOpen  int
		passClose int
		expected  *state.AlertInfo
	}{
		{
			name: "never alert by default",
			row: state.Row{
				Results: []int32{
					int32(state.Row_FAIL), 6,
				},
			},
		},
		{
			name: "passes do not alert",
			row: state.Row{
				Results: []int32{
					int32(state.Row_PASS), 6,
				},
			},
			failOpen:  1,
			passClose: 3,
		},
		{
			name: "flakes do not alert",
			row: state.Row{
				Results: []int32{
					int32(state.Row_FLAKY), 6,
				},
			},
			failOpen: 1,
		},
		{
			name: "intermittent failures do not alert",
			row: state.Row{
				Results: []int32{
					int32(state.Row_FAIL), 2,
					int32(state.Row_PASS), 1,
					int32(state.Row_FAIL), 2,
				},
			},
			failOpen: 3,
		},
		{
			name: "new failures alert",
			row: state.Row{
				Results: []int32{
					int32(state.Row_FAIL), 3,
					int32(state.Row_PASS), 3,
				},
				Messages: []string{"hello", "no", "no again", "very wrong"},
				CellIds:  []string{"yes", "no", "no again", "very wrong"},
			},
			failOpen: 3,
			expected: alertInfo(3, "hello", "yes", columns[2], columns[3]),
		},
		{
			name: "too few passes do not close",
			row: state.Row{
				Results: []int32{
					int32(state.Row_PASS), 2,
					int32(state.Row_FAIL), 4,
				},
				Messages: []string{"nope", "no", "yay", "very wrong"},
				CellIds:  []string{"wrong", "no", "yep", "very wrong"},
			},
			failOpen:  1,
			passClose: 3,
			expected:  alertInfo(4, "yay", "yep", columns[5], nil),
		},
		{
			name: "flakes do not close",
			row: state.Row{
				Results: []int32{
					int32(state.Row_FLAKY), 2,
					int32(state.Row_FAIL), 4,
				},
				Messages: []string{"nope", "no", "yay", "very wrong"},
				CellIds:  []string{"wrong", "no", "yep", "very wrong"},
			},
			failOpen: 1,
			expected: alertInfo(4, "yay", "yep", columns[5], nil),
		},
		{
			name: "count failures after flaky passes",
			row: state.Row{
				Results: []int32{
					int32(state.Row_FAIL), 1,
					int32(state.Row_FLAKY), 1,
					int32(state.Row_FAIL), 1,
					int32(state.Row_PASS), 1,
					int32(state.Row_FAIL), 2,
				},
				Messages: []string{"nope", "no", "buu", "wrong", "this one"},
				CellIds:  []string{"wrong", "no", "buzz", "wrong2", "good job"},
			},
			failOpen:  2,
			passClose: 2,
			expected:  alertInfo(4, "this one", "good job", columns[5], nil),
		},
		{
			name: "close alert",
			row: state.Row{
				Results: []int32{
					int32(state.Row_PASS), 1,
					int32(state.Row_FAIL), 5,
				},
			},
			failOpen: 1,
		},
		{
			name: "track through empty results",
			row: state.Row{
				Results: []int32{
					int32(state.Row_FAIL), 1,
					int32(state.Row_NO_RESULT), 1,
					int32(state.Row_FAIL), 4,
				},
				Messages: []string{"yay", "no", "buu", "wrong", "nono"},
				CellIds:  []string{"yay-cell", "no", "buzz", "wrong2", "nada"},
			},
			failOpen:  5,
			passClose: 2,
			expected:  alertInfo(5, "yay", "yay-cell", columns[5], nil),
		},
		{
			name: "track passes through empty results",
			row: state.Row{
				Results: []int32{
					int32(state.Row_PASS), 1,
					int32(state.Row_NO_RESULT), 1,
					int32(state.Row_PASS), 1,
					int32(state.Row_FAIL), 3,
				},
			},
			failOpen:  1,
			passClose: 2,
		},
	}

	for _, tc := range cases {
		if actual := alertRow(columns, &tc.row, tc.failOpen, tc.passClose); !reflect.DeepEqual(actual, tc.expected) {
			t.Errorf("%s alert %s != expected %s", tc.name, actual, tc.expected)
		}
	}
}

func TestBuildID(t *testing.T) {
	cases := []struct {
		name     string
		build    string
		extra    string
		expected string
	}{
		{
			name: "return empty by default",
		},
		{
			name:     "favor extra if it exists",
			build:    "wrong",
			extra:    "right",
			expected: "right",
		},
		{
			name:     "build if no extra",
			build:    "yes",
			expected: "yes",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			col := state.Column{
				Build: tc.build,
			}
			if tc.extra != "" {
				col.Extra = append(col.Extra, tc.extra)
			}
			if actual := buildID(&col); actual != tc.expected {
				t.Errorf("%q != expected %q", actual, tc.expected)
			}
		})
	}
}

func TestStamp(t *testing.T) {
	cases := []struct {
		name     string
		col      *state.Column
		expected *timestamp.Timestamp
	}{
		{
			name: "0 returns nil",
		},
		{
			name: "no nanos",
			col: &state.Column{
				Started: 2,
			},
			expected: &timestamp.Timestamp{
				Seconds: 2,
				Nanos:   0,
			},
		},
		{
			name: "has nanos",
			col: &state.Column{
				Started: 1.1,
			},
			expected: &timestamp.Timestamp{
				Seconds: 1,
				Nanos:   1e8,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual := stamp(tc.col); !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("stamp %s != expected stamp %s", actual, tc.expected)
			}
		})
	}
}
