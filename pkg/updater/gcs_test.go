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
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	statuspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
)

func TestMergeCells(t *testing.T) {
	cases := []struct {
		name     string
		flaky    bool
		cells    []Cell
		expected Cell
	}{
		{
			name: "basically works",
			cells: []Cell{
				{
					Result:  statuspb.TestStatus_TOOL_FAIL,
					CellID:  "random",
					Icon:    "religious",
					Message: "empty",
					Metrics: map[string]float64{
						"answer":   42,
						"question": 1,
					},
				},
			},
			expected: Cell{
				Result:  statuspb.TestStatus_TOOL_FAIL,
				CellID:  "random",
				Icon:    "religious",
				Message: "empty",
				Metrics: map[string]float64{
					"answer":   42,
					"question": 1,
				},
			},
		},
		{
			name: "passes work and take highest filled message",
			cells: []Cell{
				{
					Result: statuspb.TestStatus_PASS,
					Icon:   "drop",
				},
				{
					Result:  statuspb.TestStatus_BUILD_PASSED,
					Message: "woah",
				},
				{
					Result: statuspb.TestStatus_PASS_WITH_ERRORS, // highest but empty
				},
				{
					Result:  statuspb.TestStatus_PASS_WITH_SKIPS, // highest with message
					Message: "already got one",
				},
				{
					Result:  statuspb.TestStatus_PASS,
					Message: "there",
				},
			},
			expected: Cell{
				Result:  statuspb.TestStatus_PASS_WITH_ERRORS,
				Message: "5/5 runs passed: already got one",
				Icon:    "5/5",
			},
		},
		{
			name: "merge metrics",
			cells: []Cell{
				{
					Result: statuspb.TestStatus_PASS,
					Metrics: map[string]float64{
						"common": 1,
						"first":  1,
					},
				},
				{
					Result: statuspb.TestStatus_PASS,
					Metrics: map[string]float64{
						"common": 2,
						"second": 2,
					},
				},
				{
					Result: statuspb.TestStatus_PASS,
					Metrics: map[string]float64{
						"common": 108, // total 111
						"third":  3,
					},
				},
			},
			expected: Cell{
				Result:  statuspb.TestStatus_PASS,
				Message: "3/3 runs passed",
				Icon:    "3/3",
				Metrics: map[string]float64{
					"common": 37,
					"first":  1,
					"second": 2,
					"third":  3,
				},
			},
		},
		{
			name: "issues",
			cells: []Cell{
				{
					Result: statuspb.TestStatus_PASS,
					Issues: []string{"a", "b", "common"},
				},
				{
					Result: statuspb.TestStatus_PASS,
					Issues: []string{"common", "c"},
				},
				{
					Result: statuspb.TestStatus_PASS,
					Issues: []string{"common", "d"},
				},
			},
			expected: Cell{
				Result:  statuspb.TestStatus_PASS,
				Message: "3/3 runs passed",
				Icon:    "3/3",
				Issues: []string{
					"a",
					"b",
					"c",
					"common",
					"d",
				},
			},
		},
		{
			name: "failures take highest failure and highest failure message",
			cells: []Cell{
				{
					Result:  statuspb.TestStatus_TIMED_OUT,
					Message: "agonizingly slow",
					Icon:    "drop",
				},
				{
					Result:  statuspb.TestStatus_CATEGORIZED_FAIL, //highest with message
					Icon:    "drop",
					Message: "categorically wrong",
				},
				{
					Result: statuspb.TestStatus_BUILD_FAIL, // highest
					Icon:   "drop",
				},
			},
			expected: Cell{
				Result:  statuspb.TestStatus_BUILD_FAIL,
				Icon:    "0/3",
				Message: "0/3 runs passed: categorically wrong",
			},
		},
		{
			name:  "mix of passes and failures flake upon request",
			flaky: true,
			cells: []Cell{
				{
					Result:  statuspb.TestStatus_PASS,
					Message: "yay",
				},
				{
					Result:  statuspb.TestStatus_FAIL,
					Message: "boom",
				},
			},
			expected: Cell{
				Result:  statuspb.TestStatus_FLAKY,
				Icon:    "1/2",
				Message: "1/2 runs passed: boom",
			},
		},
		{
			name: "mix of passes and failures will fail upon request",
			cells: []Cell{
				{
					Result:  statuspb.TestStatus_PASS,
					Message: "yay",
				},
				{
					Result:  statuspb.TestStatus_TOOL_FAIL,
					Message: "boom",
				},
				{
					Result:  statuspb.TestStatus_FAIL, // highest result.GTE
					Message: "bang",
				},
				{
					Result:  statuspb.TestStatus_BUILD_FAIL,
					Message: "missing ;",
				},
			},
			expected: Cell{
				Result:  statuspb.TestStatus_FAIL,
				Icon:    "1/4",
				Message: "1/4 runs passed: bang",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MergeCells(tc.flaky, tc.cells...)
			if diff := cmp.Diff(tc.expected, got); diff != "" {
				t.Errorf("MergeCells() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSplitCells(t *testing.T) {
	const cellName = "foo"
	cases := []struct {
		name     string
		cells    []Cell
		expected map[string]Cell
	}{
		{
			name: "basically works",
		},
		{
			name:  "single item returns that item",
			cells: []Cell{{Message: "hi"}},
			expected: map[string]Cell{
				"foo": {Message: "hi"},
			},
		},
		{
			name: "multiple items have [1] starting from second",
			cells: []Cell{
				{Message: "first"},
				{Message: "second"},
				{Message: "third"},
			},
			expected: map[string]Cell{
				"foo":     {Message: "first"},
				"foo [1]": {Message: "second"},
				"foo [2]": {Message: "third"},
			},
		},
		{
			name: "many items eventually truncate",
			cells: func() []Cell {
				var out []Cell
				for i := 0; i < maxDuplicates*2; i++ {
					out = append(out, Cell{Icon: fmt.Sprintf("row %d", i)})
				}
				return out
			}(),
			expected: func() map[string]Cell {
				out := map[string]Cell{}
				out[cellName] = Cell{Icon: "row 0"}
				for i := 1; i < maxDuplicates; i++ {
					name := fmt.Sprintf("%s [%d]", cellName, i)
					out[name] = Cell{Icon: fmt.Sprintf("row %d", i)}
				}
				out[cellName+" [overflow]"] = overflowCell
				return out
			}(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := SplitCells(cellName, tc.cells...)
			if diff := cmp.Diff(actual, tc.expected); diff != "" {
				t.Errorf("SplitCells() got unexpected diff (-have, +want):\n%s", diff)
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
		name     string
		nameCfg  nameConfig
		id       string
		headers  []string
		result   gcsResult
		opt      groupOptions
		expected InflatedColumn
	}{
		{
			name: "basically works",
			expected: InflatedColumn{
				Column: &statepb.Column{},
				Cells: map[string]Cell{
					"." + overallRow: {
						Result:  statuspb.TestStatus_FAIL,
						Icon:    "T",
						Message: "Build did not complete within 24 hours",
					},
				},
			},
		},
		{
			name: "pass dynamic email list",
			result: gcsResult{
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Metadata: metadata.Metadata{
							EmailListKey: []string{"world"},
						},
					},
				},
			},
			expected: InflatedColumn{
				Column: &statepb.Column{
					EmailAddresses: []string{"world"},
				},
				Cells: map[string]Cell{
					"." + overallRow: {
						Result:  statuspb.TestStatus_FAIL,
						Icon:    "T",
						Message: "Build did not complete within 24 hours",
					},
				},
			},
		},
		{
			name: "pass dynamic email list as list of interaces",
			result: gcsResult{
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Metadata: metadata.Metadata{
							EmailListKey: []interface{}{"world"},
						},
					},
				},
			},
			expected: InflatedColumn{
				Column: &statepb.Column{
					EmailAddresses: []string{"world"},
				},
				Cells: map[string]Cell{
					"." + overallRow: {
						Result:  statuspb.TestStatus_FAIL,
						Icon:    "T",
						Message: "Build did not complete within 24 hours",
					},
				},
			},
		},
		{
			name: "multiple dynamic email list",
			result: gcsResult{
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Metadata: metadata.Metadata{
							EmailListKey: []string{"world", "olam"},
						},
					},
				},
			},
			expected: InflatedColumn{
				Column: &statepb.Column{
					EmailAddresses: []string{"world", "olam"},
				},
				Cells: map[string]Cell{
					"." + overallRow: {
						Result:  statuspb.TestStatus_FAIL,
						Icon:    "T",
						Message: "Build did not complete within 24 hours",
					},
				},
			},
		},
		{
			name: "not a list",
			result: gcsResult{
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Metadata: metadata.Metadata{
							EmailListKey: "olam",
						},
					},
				},
			},
			expected: InflatedColumn{
				Column: &statepb.Column{
					EmailAddresses: []string{},
				},
				Cells: map[string]Cell{
					"." + overallRow: {
						Result:  statuspb.TestStatus_FAIL,
						Icon:    "T",
						Message: "Build did not complete within 24 hours",
					},
				},
			},
		},
		{
			name: "invalid email list",
			result: gcsResult{
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Metadata: metadata.Metadata{
							EmailListKey: []interface{}{1},
						},
					},
				},
			},
			expected: InflatedColumn{
				Column: &statepb.Column{
					EmailAddresses: []string{},
				},
				Cells: map[string]Cell{
					"." + overallRow: {
						Result:  statuspb.TestStatus_FAIL,
						Icon:    "T",
						Message: "Build did not complete within 24 hours",
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
			expected: InflatedColumn{
				Column: &statepb.Column{
					Build:   "hello",
					Hint:    "hello",
					Started: 300 * 1000,
					Extra: []string{
						"1.2.3",
						"world",
						"eggs",
						"missing",
					},
				},
				Cells: map[string]Cell{
					"." + overallRow: {
						Result:  statuspb.TestStatus_FAIL,
						Icon:    "T",
						Message: "Build did not complete within 24 hours",
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
			expected: InflatedColumn{
				Column: &statepb.Column{
					Build:   "hello",
					Hint:    "hello",
					Started: float64(now * 1000),
					Extra: []string{
						"1.2.3",
						"world",
						"eggs",
						"", // not missing
					},
				},
				Cells: map[string]Cell{
					"." + overallRow: {
						Result:  statuspb.TestStatus_RUNNING,
						Icon:    "R",
						Message: "Build still running...",
					},
				},
			},
		},
		{
			name: "add overall when multiJob",
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
						Suites: &junit.Suites{
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
			expected: InflatedColumn{
				Column: &statepb.Column{
					Started: float64(now * 1000),
					Build:   "build",
					Hint:    "build",
				},
				Cells: map[string]Cell{
					overallRow: {
						Result:  statuspb.TestStatus_FAIL,
						Icon:    "F",
						Message: "Build failed outside of test results",
						Metrics: setElapsed(nil, 1),
						CellID:  "job-name/build",
					},
					"job-name.Overall": {
						Result:  statuspb.TestStatus_FAIL,
						Icon:    "F",
						Message: "Build failed outside of test results",
						Metrics: setElapsed(nil, 1),
						CellID:  "job-name/build",
					},
					"job-name.this.that": {
						Result: statuspb.TestStatus_PASS,
						CellID: "job-name/build",
					},
				},
			},
		},
		{
			name: "include job name upon request",
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
						Suites: &junit.Suites{
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
			expected: InflatedColumn{
				Column: &statepb.Column{
					Started: float64(now * 1000),
				},
				Cells: map[string]Cell{
					"job-name." + overallRow: {
						Result:  statuspb.TestStatus_FAIL,
						Icon:    "F",
						Message: "Build failed outside of test results",
						Metrics: setElapsed(nil, 1),
					},
					"job-name.this.that": {
						Result: statuspb.TestStatus_PASS,
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
						Suites: &junit.Suites{
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
			expected: InflatedColumn{
				Column: &statepb.Column{
					Started: float64(now * 1000),
				},
				Cells: map[string]Cell{
					"." + overallRow: {
						Result:  statuspb.TestStatus_FAIL,
						Icon:    "F",
						Message: "Build failed outside of test results",
						Metrics: setElapsed(nil, 1),
					},
					"this.that": {
						Result: statuspb.TestStatus_PASS,
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
						Suites: &junit.Suites{
							Suites: []junit.Suite{
								{
									Results: []junit.Result{
										{
											Name: "elapsed",
											Time: 5,
										},
										{
											Name:    "failed no message",
											Failure: &junit.Failure{Value: *pstr("")},
										},
										{
											Name:    "failed",
											Failure: &junit.Failure{Value: *pstr("boom")},
										},
										{
											Name:    "failed other message",
											Failure: &junit.Failure{Value: *pstr("")},
											Output:  pstr("irrelevant message"),
										},
										{
											Name:    "errored no message",
											Failure: &junit.Failure{Value: *pstr("")},
										},
										{
											Name:    "errored",
											Failure: &junit.Failure{Value: *pstr("oh no")},
										},
										{
											Name:    "invisible skip",
											Skipped: &junit.Skipped{Value: *pstr("")},
										},
										{
											Name:    "visible skip",
											Skipped: &junit.Skipped{Value: *pstr("tl;dr")},
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
			expected: InflatedColumn{
				Column: &statepb.Column{
					Started: float64(now * 1000),
				},
				Cells: map[string]Cell{
					"." + overallRow: {
						Result:  statuspb.TestStatus_FAIL,
						Metrics: setElapsed(nil, 1),
					},
					"elapsed": {
						Result:  statuspb.TestStatus_PASS,
						Metrics: setElapsed(nil, 5),
					},
					"failed no message": {
						Result: statuspb.TestStatus_FAIL,
					},
					"failed": {
						Message: "boom",
						Result:  statuspb.TestStatus_FAIL,
						Icon:    "F",
					},
					"failed other message": {
						Message: "irrelevant message",
						Result:  statuspb.TestStatus_FAIL,
						Icon:    "F",
					},
					"errored no message": {
						Result: statuspb.TestStatus_FAIL,
					},
					"errored": {
						Message: "oh no",
						Result:  statuspb.TestStatus_FAIL,
						Icon:    "F",
					},
					// no invisible skip
					"visible skip": {
						Result:  statuspb.TestStatus_PASS_WITH_SKIPS,
						Message: "tl;dr",
						Icon:    "S",
					},
					"stderr message": {
						Message: "ouch",
						Result:  statuspb.TestStatus_PASS,
					},
					"stdout message": {
						Message: "bellybutton",
						Result:  statuspb.TestStatus_PASS,
					},
				},
			},
		},
		{
			name: "metricKey",
			nameCfg: nameConfig{
				format: "%s",
				parts:  []string{testsName},
			},
			opt: groupOptions{
				metricKey: "food",
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
						Suites: &junit.Suites{
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
													{Name: "random", Value: "thing"},
												},
											},
										},
										{
											Name: "not a number",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{Name: "food", Value: "tasty"},
												},
											},
										},
										{
											Name: "short number",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{Name: "food", Value: "123"},
												},
											},
										},
										{
											Name: "large number",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{Name: "food", Value: "123456789"},
												},
											},
										},
										{
											Name: "many digits",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{Name: "food", Value: "1.567890"},
												},
											},
										},
										{
											Name: "multiple values",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{Name: "food", Value: "1"},
													{Name: "food", Value: "2"},
													{Name: "food", Value: "3"},
													{Name: "food", Value: "4"},
												},
											},
										},
										{
											Name: "preceds failure message",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{Name: "food", Value: "1"},
												},
											},
											Failure: &junit.Failure{Value: *pstr("boom")},
										},
										{
											Name: "preceds skip message",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{Name: "food", Value: "1"},
												},
											},
											Skipped: &junit.Skipped{Value: *pstr("tl;dr")},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: InflatedColumn{
				Column: &statepb.Column{
					Started: float64(now * 1000),
				},
				Cells: map[string]Cell{
					"." + overallRow: {
						Result:  statuspb.TestStatus_FAIL,
						Metrics: setElapsed(nil, 1),
					},
					"no properties": {
						Result: statuspb.TestStatus_PASS,
					},
					"missing property": {
						Result: statuspb.TestStatus_PASS,
					},
					"not a number": {
						Result: statuspb.TestStatus_PASS,
					},
					"short number": {
						Result: statuspb.TestStatus_PASS,
						Icon:   "123",
						Metrics: map[string]float64{
							"food": 123,
						},
					},
					"large number": {
						Result: statuspb.TestStatus_PASS,
						Icon:   "1.235e+08",
						Metrics: map[string]float64{
							"food": 123456789,
						},
					},
					"many digits": {
						Result: statuspb.TestStatus_PASS,
						Icon:   "1.568",
						Metrics: map[string]float64{
							"food": 1.567890,
						},
					},
					"multiple values": {
						Result: statuspb.TestStatus_PASS,
						Icon:   "2.5",
						Metrics: map[string]float64{
							"food": 2.5,
						},
					},
					"preceds failure message": {
						Result:  statuspb.TestStatus_FAIL,
						Message: "boom",
						Icon:    "1",
						Metrics: map[string]float64{
							"food": 1,
						},
					},
					"preceds skip message": {
						Result:  statuspb.TestStatus_PASS_WITH_SKIPS,
						Message: "tl;dr",
						Icon:    "1",
						Metrics: map[string]float64{
							"food": 1,
						},
					},
				},
			},
		},
		{
			name: "annotations",
			nameCfg: nameConfig{
				format: "%s",
				parts:  []string{testsName},
			},
			opt: groupOptions{
				annotations: []*configpb.TestGroup_TestAnnotation{
					{
						ShortText: "steak",
						ShortTextMessageSource: &configpb.TestGroup_TestAnnotation_PropertyName{
							PropertyName: "fries",
						},
					},
				},
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
						Suites: &junit.Suites{
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
													{Name: "random", Value: "thing"},
												},
											},
										},
										{
											Name: "present",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{Name: "fries", Value: "irrelevant"},
												},
											},
										},
										{
											Name: "empty",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{Name: "fries", Value: ""},
												},
											},
										},
										{
											Name: "multiple",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{Name: "fries", Value: "shoestring"},
													{Name: "fries", Value: "curly"},
												},
											},
										},
										{
											Name:    "annotation over failure",
											Failure: &junit.Failure{Value: *pstr("boom")},
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{Name: "fries", Value: "irrelevant"},
												},
											},
										},
										{
											Name:    "annotation over error",
											Errored: &junit.Errored{Value: *pstr("boom")},
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{Name: "fries", Value: "irrelevant"},
												},
											},
										},
										{
											Name:    "annotation over skip",
											Skipped: &junit.Skipped{Value: *pstr("boom")},
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{Name: "fries", Value: "irrelevant"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: InflatedColumn{
				Column: &statepb.Column{
					Started: float64(now * 1000),
				},
				Cells: map[string]Cell{
					"." + overallRow: {
						Result:  statuspb.TestStatus_PASS,
						Metrics: setElapsed(nil, 1),
					},
					"no properties": {
						Result: statuspb.TestStatus_PASS,
					},
					"missing property": {
						Result: statuspb.TestStatus_PASS,
					},
					"present": {
						Result: statuspb.TestStatus_PASS,
						Icon:   "steak",
					},
					"empty": {
						Result: statuspb.TestStatus_PASS,
						Icon:   "steak",
					},
					"multiple": {
						Result: statuspb.TestStatus_PASS,
						Icon:   "steak",
					},
					"annotation over failure": {
						Result:  statuspb.TestStatus_FAIL,
						Icon:    "steak",
						Message: "boom",
					},
					"annotation over error": {
						Result:  statuspb.TestStatus_FAIL,
						Icon:    "steak",
						Message: "boom",
					},
					"annotation over skip": {
						Result:  statuspb.TestStatus_PASS_WITH_SKIPS,
						Icon:    "steak",
						Message: "boom",
					},
				},
			},
		},
		{
			name: "userKey",
			nameCfg: nameConfig{
				format: "%s",
				parts:  []string{testsName},
			},
			opt: groupOptions{
				userKey: "fries",
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
						Suites: &junit.Suites{
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
													{Name: "random", Value: "thing"},
												},
											},
										},
										{
											Name: "present",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{Name: "fries", Value: "curly"},
												},
											},
										},
										{
											Name: "choose first",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{Name: "fries", Value: "shoestring"},
													{Name: "fries", Value: "curly"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: InflatedColumn{
				Column: &statepb.Column{
					Started: float64(now * 1000),
				},
				Cells: map[string]Cell{
					"." + overallRow: {
						Result:  statuspb.TestStatus_PASS,
						Metrics: setElapsed(nil, 1),
					},
					"no properties": {
						Result: statuspb.TestStatus_PASS,
					},
					"missing property": {
						Result: statuspb.TestStatus_PASS,
					},
					"present": {
						Result:       statuspb.TestStatus_PASS,
						UserProperty: "curly",
					},
					"choose first": {
						Result:       statuspb.TestStatus_PASS,
						UserProperty: "shoestring",
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
						Suites: &junit.Suites{
							Suites: []junit.Suite{
								{
									Results: []junit.Result{
										{
											Name: "elapsed",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													{Name: "property", Value: "good-property"},
													{Name: "ignore", Value: "me"},
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
						Suites: &junit.Suites{
							Suites: []junit.Suite{
								{
									Results: []junit.Result{
										{
											Name: "other",
											Properties: &junit.Properties{
												PropertyList: []junit.Property{
													// "property" missing
													{Name: "ignore", Value: "me"},
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
			expected: InflatedColumn{
				Column: &statepb.Column{
					Started: float64(now * 1000),
				},
				Cells: map[string]Cell{
					"." + overallRow: {
						Result:  statuspb.TestStatus_PASS,
						Metrics: setElapsed(nil, 1),
					},
					"elapsed - first [second] (good-property)": {
						Result: statuspb.TestStatus_PASS,
					},
					"other - hey [] ()": {
						Result: statuspb.TestStatus_PASS,
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
			opt: groupOptions{
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
						Suites: &junit.Suites{
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
						Suites: &junit.Suites{
							Suites: []junit.Suite{
								{
									Results: []junit.Result{
										{
											Name:    "same",
											Time:    3,
											Failure: &junit.Failure{Value: *pstr("ugh")},
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
			expected: InflatedColumn{
				Column: &statepb.Column{
					Started: float64(now * 1000),
				},
				Cells: map[string]Cell{
					"." + overallRow: {
						Result:  statuspb.TestStatus_PASS,
						Metrics: setElapsed(nil, 1),
					},
					"same - same": {
						Result:  statuspb.TestStatus_FLAKY,
						Icon:    "2/3",
						Message: "2/3 runs passed: ugh",
						Metrics: setElapsed(nil, 2), // mean
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
						Suites: &junit.Suites{
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
						Suites: &junit.Suites{
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
			expected: InflatedColumn{
				Column: &statepb.Column{
					Started: float64(now * 1000),
				},
				Cells: map[string]Cell{
					"." + overallRow: {
						Result:  statuspb.TestStatus_PASS,
						Metrics: setElapsed(nil, 1),
					},
					"same - same": {
						Result:  statuspb.TestStatus_PASS,
						Metrics: setElapsed(nil, 1),
					},
					"same - same [1]": {
						Result:  statuspb.TestStatus_PASS,
						Metrics: setElapsed(nil, 2),
					},
					"same - same [2]": {
						Result:  statuspb.TestStatus_PASS,
						Metrics: setElapsed(nil, 3),
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
						Suites: &junit.Suites{
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
			expected: InflatedColumn{
				Column: &statepb.Column{
					Started: float64(now * 1000),
				},
				Cells: func() map[string]Cell {
					out := map[string]Cell{
						"." + overallRow: {
							Result:  statuspb.TestStatus_PASS,
							Metrics: setElapsed(nil, 1),
						},
					}
					under := Cell{Result: statuspb.TestStatus_PASS}
					max := Cell{Result: statuspb.TestStatus_PASS}
					over := Cell{Result: statuspb.TestStatus_PASS}
					out["under"] = under
					out["max"] = max
					out["over"] = over
					for i := 1; i < maxDuplicates; i++ {
						t := float64(i)
						under.Metrics = setElapsed(nil, t)
						out[fmt.Sprintf("under [%d]", i)] = under
						max.Metrics = setElapsed(nil, t)
						out[fmt.Sprintf("max [%d]", i)] = max
						over.Metrics = setElapsed(nil, t)
						out[fmt.Sprintf("over [%d]", i)] = over
					}
					max.Metrics = setElapsed(nil, maxDuplicates)
					out[`max [overflow]`] = overflowCell
					out[`over [overflow]`] = overflowCell
					return out
				}(),
			},
		},
		{
			name: "can add missing podInfo",
			opt: groupOptions{
				analyzeProwJob: true,
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
			},
			expected: InflatedColumn{
				Column: &statepb.Column{
					Started: float64(now * 1000),
				},
				Cells: map[string]Cell{
					"." + overallRow: {
						Result:  statuspb.TestStatus_PASS,
						Metrics: setElapsed(nil, 1),
					},
					"." + podInfoRow: podInfoMissingCell,
				},
			},
		},
		{
			name: "can add interesting pod info",
			opt: groupOptions{
				analyzeProwJob: true,
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
				podInfo: gcs.PodInfo{
					Pod: &core.Pod{
						Status: core.PodStatus{Phase: core.PodSucceeded},
					},
				},
			},
			expected: InflatedColumn{
				Column: &statepb.Column{
					Started: float64(now * 1000),
				},
				Cells: map[string]Cell{
					"." + overallRow: {
						Result:  statuspb.TestStatus_PASS,
						Metrics: setElapsed(nil, 1),
					},
					"." + podInfoRow: podInfoPassCell,
				},
			},
		},
		{
			name: "do not add missing podinfo when still running",
			opt: groupOptions{
				analyzeProwJob: true,
			},
			result: gcsResult{
				started: gcs.Started{
					Started: metadata.Started{
						Timestamp: now,
					},
				},
			},
			expected: InflatedColumn{
				Column: &statepb.Column{
					Started: float64(now * 1000),
				},
				Cells: map[string]Cell{
					"." + overallRow: {
						Result:  statuspb.TestStatus_RUNNING,
						Icon:    "R",
						Message: "Build still running...",
					},
				},
			},
		},
		{
			name: "add intersting podinfo even still running",
			opt: groupOptions{
				analyzeProwJob: true,
			},
			result: gcsResult{
				started: gcs.Started{
					Started: metadata.Started{
						Timestamp: now,
					},
				},
				podInfo: gcs.PodInfo{
					Pod: &core.Pod{
						Status: core.PodStatus{Phase: core.PodSucceeded},
					},
				},
			},
			expected: InflatedColumn{
				Column: &statepb.Column{
					Started: float64(now * 1000),
				},
				Cells: map[string]Cell{
					"." + overallRow: {
						Result:  statuspb.TestStatus_RUNNING,
						Icon:    "R",
						Message: "Build still running...",
					},
					"." + podInfoRow: podInfoPassCell,
				},
			},
		},
		{
			name: "addCellID",
			nameCfg: nameConfig{
				format: "%s",
				parts:  []string{testsName},
			},
			opt: groupOptions{
				addCellID: true,
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
						Suites: &junit.Suites{
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
			id: "McLovin",
			expected: InflatedColumn{
				Column: &statepb.Column{
					Started: float64(now * 1000),
					Build:   "McLovin",
					Hint:    "McLovin",
				},
				Cells: map[string]Cell{
					"." + overallRow: {
						Result:  statuspb.TestStatus_PASS,
						Metrics: setElapsed(nil, 1),
						CellID:  "McLovin",
					},
					"this.that": {
						Result: statuspb.TestStatus_PASS,
						CellID: "McLovin",
					},
				},
			},
		},
		{
			name: "override build key",
			nameCfg: nameConfig{
				format: "%s",
				parts:  []string{testsName},
			},
			opt: groupOptions{
				buildKey: "my-build-id",
			},
			result: gcsResult{
				started: gcs.Started{
					Started: metadata.Started{
						Timestamp: now,
					},
				},
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Metadata: metadata.Metadata{
							"my-build-id": "id-1",
						},
						Timestamp: pint(now + 1),
						Passed:    &yes,
					},
				},
			},
			id: "McLovin",
			expected: InflatedColumn{
				Column: &statepb.Column{
					Started: float64(now * 1000),
					Build:   "id-1",
					Hint:    "McLovin",
				},
				Cells: map[string]Cell{
					"." + overallRow: {
						Result:  statuspb.TestStatus_PASS,
						Metrics: setElapsed(nil, 1),
					},
				},
			},
		},
		{
			name: "override build key missing",
			nameCfg: nameConfig{
				format: "%s",
				parts:  []string{testsName},
			},
			opt: groupOptions{
				buildKey: "my-build-id",
			},
			result: gcsResult{
				started: gcs.Started{
					Started: metadata.Started{
						Timestamp: now,
					},
				},
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Metadata: metadata.Metadata{
							"something": "hello",
						},
						Timestamp: pint(now + 1),
						Passed:    &yes,
					},
				},
			},
			id: "McLovin",
			expected: InflatedColumn{
				Column: &statepb.Column{
					Started: float64(now * 1000),
					Build:   "",
					Hint:    "McLovin",
				},
				Cells: map[string]Cell{
					"." + overallRow: {
						Result:  statuspb.TestStatus_PASS,
						Metrics: setElapsed(nil, 1),
					},
				},
			},
		},
		{
			name: "Ginkgo V2 skipped with message",
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
						Suites: &junit.Suites{
							Suites: []junit.Suite{
								{
									Results: []junit.Result{
										{
											Name:    "visible skip non-default msg",
											Skipped: &junit.Skipped{Message: *pstr("non-default message")},
										},
										{
											Name:    "invisible skip default msg",
											Skipped: &junit.Skipped{Message: *pstr("skipped")},
										},
										{
											Name:    "invisible skip msg empty",
											Skipped: &junit.Skipped{Message: *pstr("")},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: InflatedColumn{
				Column: &statepb.Column{
					Started: float64(now * 1000),
				},
				Cells: map[string]Cell{
					"." + overallRow: {
						Result:  statuspb.TestStatus_PASS,
						Metrics: setElapsed(nil, 1),
					},
					"visible skip non-default msg": {
						Result:  statuspb.TestStatus_PASS_WITH_SKIPS,
						Message: "non-default message",
						Icon:    "S",
					},
				},
			},
		},
		{
			name: "ignore_skip works",
			opt: groupOptions{
				ignoreSkip: true,
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
						Suites: &junit.Suites{
							Suites: []junit.Suite{
								{
									Results: []junit.Result{
										{
											Name:    "visible skip non-default msg",
											Skipped: &junit.Skipped{Message: *pstr("non-default message")},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: InflatedColumn{
				Column: &statepb.Column{
					Started: float64(now * 1000),
				},
				Cells: map[string]Cell{
					"." + overallRow: {
						Result:  statuspb.TestStatus_PASS,
						Metrics: setElapsed(nil, 1),
					},
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			log := logrus.WithField("test name", tc.name)
			actual := convertResult(log, tc.nameCfg, tc.id, tc.headers, tc.result, tc.opt)
			if diff := cmp.Diff(tc.expected, actual, protocmp.Transform()); diff != "" {
				t.Errorf("convertResult() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestIgnoreStatus(t *testing.T) {
	cases := []struct {
		opt    groupOptions
		status statuspb.TestStatus
		want   bool
	}{
		{
			opt:    groupOptions{},
			status: statuspb.TestStatus_NO_RESULT,
			want:   true,
		},
		{
			opt:    groupOptions{},
			status: statuspb.TestStatus_PASS,
			want:   false,
		},
		{
			opt:    groupOptions{},
			status: statuspb.TestStatus_PASS_WITH_SKIPS,
			want:   false,
		},
		{
			opt:    groupOptions{},
			status: statuspb.TestStatus_RUNNING,
			want:   false,
		},
		{
			opt:    groupOptions{ignoreSkip: true},
			status: statuspb.TestStatus_PASS,
			want:   false,
		},
		{
			opt:    groupOptions{ignoreSkip: true},
			status: statuspb.TestStatus_PASS_WITH_SKIPS,
			want:   true,
		},
		{
			opt:    groupOptions{ignoreSkip: true},
			status: statuspb.TestStatus_RUNNING,
			want:   false,
		},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("ignoreStatus(%+v, %v)", tc.opt, tc.status), func(t *testing.T) {
			if got := ignoreStatus(tc.opt, tc.status); got != tc.want {
				t.Errorf("ignoreStatus(%v, %v) got %v, want %v", tc.opt, tc.status, got, tc.want)
			}
		})
	}
}

func TestPodInfoCell(t *testing.T) {
	now := time.Now().Add(-time.Second)

	cases := []struct {
		name     string
		result   gcsResult
		expected Cell
	}{
		{
			name: "basically works",
			result: gcsResult{
				started: gcs.Started{
					Started: metadata.Started{
						Timestamp: now.Unix(),
					},
				},
			},
			expected: podInfoMissingCell,
		},
		{
			name: "wait for pod from running pod",
			result: gcsResult{
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Timestamp: func() *int64 {
							when := now.Unix()
							return &when
						}(),
					},
				},
			},
			expected: podInfoMissingCell,
		},
		{
			name: "wait for pod info from finished pod",
			result: gcsResult{
				started: gcs.Started{
					Started: metadata.Started{
						Timestamp: now.Add(-24 * time.Hour).Unix(),
					},
				},
			},
			expected: func() Cell {
				c := podInfoMissingCell
				c.Result = statuspb.TestStatus_PASS_WITH_SKIPS
				return c
			}(),
		},
		{
			name: "finished pod without podinfo",
			result: gcsResult{
				finished: gcs.Finished{
					Finished: metadata.Finished{
						Timestamp: func() *int64 {
							when := now.Add(-time.Hour).Unix()
							return &when
						}(),
					},
				},
			},
			expected: func() Cell {
				c := podInfoMissingCell
				c.Result = statuspb.TestStatus_PASS_WITH_SKIPS
				return c
			}(),
		},
		{
			name: "passing pod",
			result: gcsResult{
				podInfo: podInfoSuccessPodInfo,
			},
			expected: podInfoPassCell,
		},
		{
			name: "no pod utils",
			result: gcsResult{
				podInfo: gcs.PodInfo{Pod: &core.Pod{}},
			},
			expected: Cell{
				Message: gcs.NoPodUtils,
				Icon:    "E",
				Result:  statuspb.TestStatus_PASS,
			},
		},
		{
			name: "pod failure",
			result: gcsResult{
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
			},
			expected: Cell{
				Message: "pod did not schedule: hi there",
				Icon:    "F",
				Result:  statuspb.TestStatus_FAIL,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := podInfoCell(tc.result)
			if diff := cmp.Diff(actual, tc.expected); diff != "" {
				t.Errorf("podInfoCell(%v) got unexpected diff (-have, +want):\n%s", tc.result, diff)
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
		expected Cell
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
			expected: Cell{
				Result:  statuspb.TestStatus_FAIL,
				Message: "Build did not complete within 24 hours",
				Icon:    "T",
			},
		},
		{
			name: "missing results fail",
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
				malformed: []string{
					"podinfo.json",
				},
			},
			expected: Cell{
				Result:  statuspb.TestStatus_FAIL,
				Message: "Malformed artifacts: podinfo.json",
				Icon:    "E",
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
			expected: Cell{
				Result:  statuspb.TestStatus_PASS,
				Metrics: setElapsed(nil, 150),
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
			expected: Cell{
				Result:  statuspb.TestStatus_FAIL,
				Icon:    "E",
				Message: `finished.json missing "passed": false`,
				Metrics: setElapsed(nil, 150),
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
			expected: Cell{
				Result:  statuspb.TestStatus_PASS,
				Icon:    "E",
				Message: `finished.json missing "passed": true`,
				Metrics: setElapsed(nil, 150),
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
			expected: Cell{
				Result:  statuspb.TestStatus_FAIL,
				Metrics: setElapsed(nil, 150),
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
			expected: Cell{
				Result:  statuspb.TestStatus_FAIL,
				Metrics: setElapsed(nil, 150),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := overallCell(tc.result)
			if diff := cmp.Diff(actual, tc.expected); diff != "" {
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
				ElapsedKey: 10 / 60.0,
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
				ElapsedKey: 5 / 60.0,
			},
		},
		{
			name: "override existing value",
			metrics: map[string]float64{
				ElapsedKey: 3 / 60.0,
			},
			seconds: 10,
			expected: map[string]float64{
				ElapsedKey: 10 / 60.0,
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
											Skipped: &junit.Skipped{Value: *pstr("first")},
										},
									},
								},
							},
							Results: []junit.Result{
								{
									Name:    "branch",
									Skipped: &junit.Skipped{Value: *pstr("second")},
								},
							},
						},
					},
					Results: []junit.Result{
						{
							Name:    "trunk",
							Skipped: &junit.Skipped{Value: *pstr("third")},
						},
					},
				},
			},
			expected: []junit.Result{
				{
					Name:    "must.go.deeper.leaf",
					Skipped: &junit.Skipped{Value: *pstr("first")},
				},
				{
					Name:    "must.go.branch",
					Skipped: &junit.Skipped{Value: *pstr("second")},
				},
				{
					Name:    "must.trunk",
					Skipped: &junit.Skipped{Value: *pstr("third")},
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
