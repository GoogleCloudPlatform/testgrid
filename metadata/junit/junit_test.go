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

package junit

import (
	"bytes"
	"encoding/xml"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestMessage(t *testing.T) {
	pstr := func(s string) *string {
		return &s
	}

	cases := []struct {
		name     string
		jr       Result
		max      int
		expected string
	}{
		{
			name: "basically works",
		},
		{
			name: "errored takes top priority with message and value",
			jr: Result{
				Errored: &Errored{Message: "errored-0-msg", Value: *pstr("errored-0")},
				Failure: &Failure{Message: "failure-1-msg", Value: *pstr("failure-1")},
				Skipped: &Skipped{Message: "skipped-2-msg", Value: *pstr("skipped-2")},
				Error:   pstr("error-3"),
				Output:  pstr("output-4"),
			},
			expected: "errored-0-msg\nerrored-0",
		},
		{
			name: "errored takes top priority with message",
			jr: Result{
				Errored: &Errored{Message: "errored-0-msg"},
				Failure: &Failure{Message: "failure-1-msg"},
				Skipped: &Skipped{Message: "skipped-2-msg"},
				Error:   pstr("error-3"),
				Output:  pstr("output-4"),
			},
			expected: "errored-0-msg",
		},
		{
			name: "errored takes top priority with value",
			jr: Result{
				Errored: &Errored{Value: *pstr("errored-0")},
				Failure: &Failure{Value: *pstr("failure-1")},
				Skipped: &Skipped{Value: *pstr("skipped-2")},
				Error:   pstr("error-3"),
				Output:  pstr("output-4"),
			},
			expected: "errored-0",
		},

		{
			name: "failure priorized over skipped, error and output with message and value",
			jr: Result{
				Failure: &Failure{Message: "failure-0-msg", Value: *pstr("failure-0")},
				Skipped: &Skipped{Message: "skipped-1-msg", Value: *pstr("skipped-1")},
				Error:   pstr("error-2"),
				Output:  pstr("output-3"),
			},
			expected: "failure-0-msg\nfailure-0",
		},
		{
			name: "failure priorized over skipped, error and output with message",
			jr: Result{
				Failure: &Failure{Message: "failure-0-msg"},
				Skipped: &Skipped{Message: "skipped-1-msg"},
				Error:   pstr("error-2"),
				Output:  pstr("output-3"),
			},
			expected: "failure-0-msg",
		},
		{
			name: "failure priorized over skipped, error and output with value",
			jr: Result{
				Failure: &Failure{Value: *pstr("failure-0")},
				Skipped: &Skipped{Value: *pstr("skipped-1")},
				Error:   pstr("error-2"),
				Output:  pstr("output-3"),
			},
			expected: "failure-0",
		},
		{
			name: "skipped prioritized over error, output with message and value",
			jr: Result{
				Skipped: &Skipped{Message: "skipped-1-msg", Value: *pstr("skipped-1")},
				Error:   pstr("error-2"),
				Output:  pstr("output-3"),
			},
			expected: "skipped-1-msg\nskipped-1",
		},
		{
			name: "skipped prioritized over error, output with message",
			jr: Result{
				Skipped: &Skipped{Message: "skipped-1-msg"},
				Error:   pstr("error-2"),
				Output:  pstr("output-3"),
			},
			expected: "skipped-1-msg",
		},
		{
			name: "skipped prioritized over error, output with value",
			jr: Result{
				Skipped: &Skipped{Value: *pstr("skipped-1")},
				Error:   pstr("error-2"),
				Output:  pstr("output-3"),
			},
			expected: "skipped-1",
		},
		{
			name: "error has higher priority than output",
			jr: Result{
				Error:  pstr("error-2"),
				Output: pstr("output-3"),
			},
			expected: "error-2",
		},
		{
			name: "return output when set",
			jr: Result{
				Output: pstr("output-3"),
			},
			expected: "output-3",
		},
		{
			name: "truncate long messages upon request",
			jr: Result{
				Output: pstr("four by four"),
			},
			max:      8,
			expected: "four...four",
		},
		{
			name: "handle invalid UTF-8 strings",
			jr: Result{
				Output: pstr("a\xc5z"),
			},
			max:      8,
			expected: "invalid utf8: a?z",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual, expected := tc.jr.Message(tc.max), tc.expected; actual != expected {
				t.Errorf("jr.Message(%d) got %q, want %q", tc.max, actual, expected)
			}
		})
	}
}

func TestParse(t *testing.T) {
	pstr := func(s string) *string {
		return &s
	}
	cases := []struct {
		name     string
		buf      []byte
		expected *Suites
	}{
		{
			name:     "parse empty file as empty suite",
			expected: &Suites{},
		},
		{
			name: "not xml fails",
			buf:  []byte("<hello"),
		},
		{
			name: "parse testsuite correctly",
			buf:  []byte(`<testsuite><testcase name="hi"/></testsuite>`),
			expected: &Suites{
				Suites: []Suite{
					{
						XMLName: xml.Name{Local: "testsuite"},
						Results: []Result{
							{Name: "hi"},
						},
					},
				},
			},
		},
		{
			name: "parse testsuites correctly",
			buf: []byte(`
                        <testsuites>
                            <testsuite name="fun" tests="10" disabled="1" skipped="2" errors="3" failures="4" time="100.1" timestamp="2023-08-28T17:15:04">
                                <testsuite name="knee">
				    <properties>
					<property name="SuiteSucceeded" value="true"></property>
                                    </properties>
                                    <testcase name="bone" time="6" />
                                    <testcase name="head" time="3" >
										<failure type="failure" message="failure message attribute"> failure message body </failure>
									</testcase>
                                    <testcase name="neck" time="2" >
										<error type="error" message="error message attribute"> error message body </error>
									</testcase>
                                </testsuite>
                                <testcase name="word" classname="E2E Suite" status="skipped" time="7"></testcase>
                            </testsuite>
                        </testsuites>
                        `),
			expected: &Suites{
				XMLName: xml.Name{Local: "testsuites"},
				Suites: []Suite{
					{
						XMLName: xml.Name{Local: "testsuite"},
						Name:    "fun",
						Failures:  4,
                                                Tests:     10,
                                                Disabled:  1,
                                                Skipped:   2,
                                                Errors:    3,
						Time:      100.1,
                                                TimeStamp: "2023-08-28T17:15:04",
                                                Suites: []Suite{
							{
								XMLName: xml.Name{Local: "testsuite"},
								Name:    "knee",
								Properties: &Properties{
                                                                        []Property{
                                                                                {
                                                                                        Name:  "SuiteSucceeded",
                                                                                        Value: "true",
                                                                                },
                                                                        },
                                                                },
								Results: []Result{
									{
										Name: "bone",
										Time: 6,
									},
									{
										Name:    "head",
										Time:    3,
										Failure: &Failure{Type: "failure", Message: "failure message attribute", Value: *pstr(" failure message body ")},
									},
									{
										Name:    "neck",
										Time:    2,
										Errored: &Errored{Type: "error", Message: "error message attribute", Value: *pstr(" error message body ")},
									},
								},
							},
						},
						Results: []Result{
							{
								Name: "word",
								Time: 7,
								ClassName: "E2E Suite",
								Status: "skipped",
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := Parse(tc.buf)
			str := string(tc.buf)
			switch {
			case err != nil:
				if tc.expected != nil {
					t.Errorf("Parse(%q) got unexpected error: %v", str, err)
				}
			case tc.expected == nil:
				t.Errorf("Parse(%q) got %v, wanted an error", str, actual)
			default:
				if diff := cmp.Diff(actual, tc.expected); diff != "" {
					t.Errorf("Parse(%q) got unexpected diff:\n%s", str, diff)
				}
			}
			if err == nil {
				streamActual, err := ParseStream(bytes.NewReader(tc.buf))
				if err != nil {
					t.Errorf("ParseStream() got unexpected error: %v", err)
				}
				if diff := cmp.Diff(actual, streamActual); diff != "" {
					t.Errorf("ParseStream() got unexpected diff:\n%s", diff)
				}
			}
		})
	}
}
