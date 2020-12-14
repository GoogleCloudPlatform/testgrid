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
			name: "failure takes top priority",
			jr: Result{
				Failure: pstr("failure-0"),
				Skipped: pstr("skipped-1"),
				Error:   pstr("error-2"),
				Output:  pstr("output-3"),
			},
			expected: "failure-0",
		},
		{
			name: "skipped prioritized over error, output",
			jr: Result{
				Skipped: pstr("skipped-1"),
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
			buf:  []byte("hello"),
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
                            <testsuite name="fun">
                                <testsuite name="knee">
                                    <testcase name="bone" time="6" />
                                </testsuite>
                                <testcase name="word" time="7" />
                            </testsuite>
                        </testsuites>
                        `),
			expected: &Suites{
				XMLName: xml.Name{Local: "testsuites"},
				Suites: []Suite{
					{
						XMLName: xml.Name{Local: "testsuite"},
						Name:    "fun",
						Suites: []Suite{
							{
								XMLName: xml.Name{Local: "testsuite"},
								Name:    "knee",
								Results: []Result{
									{
										Name: "bone",
										Time: 6,
									},
								},
							},
						},
						Results: []Result{
							{
								Name: "word",
								Time: 7,
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
			if len(tc.buf) > 0 && err == nil {
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
