/*
Copyright 2019 The Testgrid Authors.

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

package metadata

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestMeta(t *testing.T) {
	world := "world"
	const key = "target-key"
	cases := []struct {
		name    string
		in      Metadata
		call    func(actual Metadata) (interface{}, bool)
		val     interface{}
		present bool
	}{
		{
			name: "can match string",
			in: Metadata{
				key: world,
			},
			call: func(actual Metadata) (interface{}, bool) {
				return actual.String(key)
			},
			val:     &world,
			present: true,
		},
		{
			name: "detect value is not a string",
			in: Metadata{
				key: Metadata{"super": "fancy"},
			},
			call: func(actual Metadata) (interface{}, bool) {
				return actual.String(key)
			},
			val:     (*string)(nil),
			present: true,
		},
		{
			name: "can match metadata",
			in: Metadata{
				key: Metadata{
					"super": "fancy",
					"one":   1,
				},
			},
			call: func(actual Metadata) (interface{}, bool) {
				return actual.Meta(key)
			},
			val: &Metadata{
				"super": "fancy",
				"one":   1.0, // LOL json
			},
			present: true,
		},
		{
			name: "detect value is not metadata",
			in: Metadata{
				key: "not metadata",
			},
			call: func(actual Metadata) (interface{}, bool) {
				return actual.Meta(key)
			},
			val:     (*Metadata)(nil),
			present: true,
		},
		{
			name: "detect key absence for string",
			in: Metadata{
				"random-key": "hello",
			},
			call: func(actual Metadata) (interface{}, bool) {
				return actual.String(key)
			},
			val: (*string)(nil),
		},
		{
			name: "detect key absence for metadata",
			in: Metadata{
				"random-key": Metadata{},
			},
			call: func(actual Metadata) (interface{}, bool) {
				return actual.Meta(key)
			},
			val: (*Metadata)(nil),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := json.Marshal(tc.in)
			if err != nil {
				t.Errorf("marshal: %v", err)
			}
			var actual Metadata
			if err := json.Unmarshal(out, &actual); err != nil {
				t.Errorf("unmarshal: %v", err)
			}
			val, present := tc.call(actual)
			if !reflect.DeepEqual(val, tc.val) {
				t.Errorf("%#v != expected %#v", val, tc.val) // Remember json doesn't have ints
			}
			if present != tc.present {
				t.Errorf("present %t != expected %t", present, tc.present)
			}
		})
	}
}

func TestVersion(t *testing.T) {
	cases := []struct {
		name     string
		started  Started
		finished Finished
		expected string
	}{
		{
			name:     "missing by default",
			expected: missing,
		},
		{
			name: "DEPRECATED: finished job version over started",
			started: Started{
				DeprecatedJobVersion: "wrong",
			},
			finished: Finished{
				DeprecatedJobVersion: "right",
			},
			expected: "right",
		},
		{
			name: "DEPRECATED: job version over repo version",
			started: Started{
				DeprecatedJobVersion:  "yes-job",
				DeprecatedRepoVersion: "no-repo",
			},
			expected: "yes-job",
		},
		{
			name: "DEPRECATED: started repo version over finished",
			started: Started{
				DeprecatedRepoVersion: "right",
			},
			finished: Finished{
				DeprecatedRepoVersion: "wrong-finish",
			},
			expected: "right",
		},
		{
			name: "job-version over repo-commit",
			started: Started{
				RepoCommit: "nope",
			},
			finished: Finished{
				Metadata: Metadata{
					JobVersion: "yes",
				},
			},
			expected: "yes",
		},
		{
			name: "find repo-commit",
			started: Started{
				RepoCommit: "yay",
			},
			expected: "yay",
		},
		{
			name: "truncate to 9 chars",
			started: Started{
				RepoCommit: "12345678900000000",
			},
			expected: "123456789",
		},
		{
			name: "drop part before the first +",
			started: Started{
				RepoCommit: "ignore+hello+yes",
			},
			expected: "hello+yes",
		},
		{
			name: "trucate part after +",
			started: Started{
				RepoCommit: "deadbeef+12345678900000000",
			},
			expected: "123456789",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual := Version(tc.started, tc.finished); actual != tc.expected {
				t.Errorf("actual %s != expected %s", actual, tc.expected)
			}
		})
	}
}

func TestSetVersion(t *testing.T) {
	cases := []struct {
		name       string
		repoCommit string
		jobVersion string
		started    bool
		finished   bool
		expected   string
	}{
		{
			name:       "basically works",
			repoCommit: "repo",
			jobVersion: "job",
			started:    true,
			finished:   true,
			expected:   "job",
		},
		{
			name:     "missing by default",
			started:  true,
			finished: true,
			expected: missing,
		},
		{
			name:       "can pass in nothing",
			repoCommit: "ignore",
			jobVersion: "me",
			expected:   missing,
		},
		{
			name:       "match repo commit when job version not set",
			repoCommit: "deadbeef",
			started:    true,
			finished:   true,
			expected:   "deadbeef",
		},
		{
			name:       "match repo commit when just started",
			repoCommit: "aaa",
			jobVersion: "ignore",
			started:    true,
			expected:   "aaa",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var s Started
			var f Finished
			var ps *Started
			var pf *Finished
			if tc.started {
				ps = &s
			}
			if tc.finished {
				pf = &f
			}
			SetVersion(ps, pf, tc.repoCommit, tc.jobVersion)
			if actual := Version(s, f); actual != tc.expected {
				t.Errorf("actual %s != expected %s", actual, tc.expected)
			}
		})
	}

}
