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
	"encoding/xml"
	"testing"

	"github.com/google/go-cmp/cmp"
)

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
			switch {
			case err != nil:
				if tc.expected != nil {
					t.Errorf("Parse(%v) got unexpected error: %v", tc.buf, err)
				}
			case tc.expected == nil:
				t.Errorf("Parse(%v) got %v, wanted an error", tc.buf, actual)
			default:
				if diff := cmp.Diff(actual, *tc.expected); diff != "" {
					t.Errorf("Parse(%v) got unexpected diff:\n%s", tc.buf, diff)
				}
			}
		})
	}
}
