/*
Copyright 2022 The TestGrid Authors.

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
	"regexp"
	"testing"

	evalpb "github.com/GoogleCloudPlatform/testgrid/pb/custom_evaluator"
	tspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
)

func TestStrReg(t *testing.T) {
	cases := []struct {
		expr string
		val  string
		res  map[string]*regexp.Regexp
		want bool
	}{
		{
			expr: `hello`,
			val:  "hello world",
			want: true,
		},
		{
			expr: `hello`,
			val:  "stranger",
		},
		{
			expr: `prefer cached regexps`,
			val:  "surprisingly this works",
			res: map[string]*regexp.Regexp{
				"prefer cached regexps": regexp.MustCompile("this"),
			},
			want: true,
		},
		{
			expr: `.**`,
			val:  "bad regexp should fail",
		},
	}

	orig := res
	defer func() { res = orig }()

	for _, tc := range cases {
		t.Run(tc.expr+" "+tc.val, func(t *testing.T) {
			res = tc.res
			if res == nil {
				res = map[string]*regexp.Regexp{}
			}
			if got := strReg(tc.val, tc.expr); got != tc.want {
				t.Errorf("strReg() got %t, want %t", got, tc.want)
			}
		})
	}
}

type fakeTestResult struct {
	properties map[string][]string
}

func (r fakeTestResult) Properties() map[string][]string {
	return r.properties
}

func (r fakeTestResult) Name() string {
	panic("boom")
}

func (r fakeTestResult) Errors() int {
	panic("boom")
}

func (r fakeTestResult) Failures() int {
	panic("boom")
}

func (r fakeTestResult) Exceptions() []string {
	panic("boom")
}

func makeResult(props map[string][]string) *fakeTestResult {
	return &fakeTestResult{
		properties: props,
	}
}

func TestCustomStatus(t *testing.T) {
	timedOut := tspb.TestStatus_TIMED_OUT
	abort := tspb.TestStatus_CATEGORIZED_ABORT
	cases := []struct {
		name  string
		rules []*evalpb.Rule
		tr    TestResult
		want  *tspb.TestStatus
	}{
		{
			name: "empty",
		},
		{
			name: "basic",
			rules: []*evalpb.Rule{
				{
					ComputedStatus: tspb.TestStatus_TIMED_OUT,
					TestResultComparisons: []*evalpb.TestResultComparison{
						{
							TestResultInfo: &evalpb.TestResultComparison_PropertyKey{
								PropertyKey: "foo",
							},
							Comparison: &evalpb.Comparison{
								Op: evalpb.Comparison_OP_EQ,
								ComparisonValue: &evalpb.Comparison_StringValue{
									StringValue: "goal",
								},
							},
						},
					},
				},
			},
			tr: makeResult(map[string][]string{
				"foo": {"goal"},
			}),

			want: &timedOut,
		},
		{
			name: "wrong string value",
			rules: []*evalpb.Rule{
				{
					ComputedStatus: tspb.TestStatus_TIMED_OUT,
					TestResultComparisons: []*evalpb.TestResultComparison{
						{
							TestResultInfo: &evalpb.TestResultComparison_PropertyKey{
								PropertyKey: "foo",
							},
							Comparison: &evalpb.Comparison{
								Op: evalpb.Comparison_OP_EQ,
								ComparisonValue: &evalpb.Comparison_StringValue{
									StringValue: "goal",
								},
							},
						},
					},
				},
			},
			tr: makeResult(map[string][]string{
				"foo": {"wrong-value"},
			}),
		},
		{
			name: "missing property",
			rules: []*evalpb.Rule{
				{
					ComputedStatus: tspb.TestStatus_TIMED_OUT,
					TestResultComparisons: []*evalpb.TestResultComparison{
						{
							TestResultInfo: &evalpb.TestResultComparison_PropertyKey{
								PropertyKey: "want",
							},
							Comparison: &evalpb.Comparison{
								Op: evalpb.Comparison_OP_EQ,
								ComparisonValue: &evalpb.Comparison_StringValue{
									StringValue: "goal",
								},
							},
						},
					},
				},
			},
			tr: makeResult(map[string][]string{
				"wrong-key": {"goal"},
			}),
		},
		{
			name: "match regex",
			rules: []*evalpb.Rule{
				{
					ComputedStatus: tspb.TestStatus_CATEGORIZED_ABORT,
					TestResultComparisons: []*evalpb.TestResultComparison{
						{
							TestResultInfo: &evalpb.TestResultComparison_PropertyKey{
								PropertyKey: "want",
							},
							Comparison: &evalpb.Comparison{
								Op: evalpb.Comparison_OP_REGEX,
								ComparisonValue: &evalpb.Comparison_StringValue{
									StringValue: ".*(No response|Error executing).*",
								},
							},
						},
					},
				},
			},
			tr: makeResult(map[string][]string{
				"want": {"prefix No response from server. Stopping testing"},
			}),
			want: &abort,
		},
		{
			name: "not match regex",
			rules: []*evalpb.Rule{
				{
					ComputedStatus: tspb.TestStatus_CATEGORIZED_ABORT,
					TestResultComparisons: []*evalpb.TestResultComparison{
						{
							TestResultInfo: &evalpb.TestResultComparison_PropertyKey{
								PropertyKey: "want",
							},
							Comparison: &evalpb.Comparison{
								Op: evalpb.Comparison_OP_REGEX,
								ComparisonValue: &evalpb.Comparison_StringValue{
									StringValue: ".*(No response|Error executing).*",
								},
							},
						},
					},
				},
			},
			tr: makeResult(map[string][]string{
				"want": {"Random stuff"},
			}),
		},
		{
			name: "no comparisons",
			rules: []*evalpb.Rule{
				{
					ComputedStatus: tspb.TestStatus_TIMED_OUT,
				},
			},
			tr: makeResult(map[string][]string{
				"foo": {"goal"},
			}),
			want: &timedOut,
		},
		{
			name: "no rules",
			tr: makeResult(map[string][]string{
				"foo": {"goal"},
			}),
		},
		// TODO(fejta): more test coverage
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CustomStatus(tc.rules, tc.tr)
			switch {
			case got == nil:
				if tc.want != nil {
					t.Errorf("customStatus() got nil, want %v", tc.want)
				}
			case tc.want == nil:
				t.Errorf("customStatus() should be nil, not %v", got)
			case *tc.want != *got:
				t.Errorf("customStatus() got %v, want %v", *got, *tc.want)

			}
		})
	}
}
