/*
Copyright 2024 The TestGrid Authors.

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

package query

import (
	"testing"
)

func TestTranslateAtom(t *testing.T) {
	cases := []struct {
		name      string
		atom      string
		want      string
		wantError bool
	}{
		{
			name: "empty",
			atom: "",
			want: "",
		},
		{
			name: "basic",
			atom: `target:"//my-target"`,
			want: `id.target_id="//my-target"`,
		},
		{
			name:      "case-sensitive key",
			atom:      `TARGET:"//MY-TARGET"`,
			wantError: true,
		},
		{
			name: "multiple colons",
			atom: `target:"//path/to:my-target"`,
			want: `id.target_id="//path/to:my-target"`,
		},
		{
			name: "unquoted",
			atom: `target://my-target`,
			want: `id.target_id="//my-target"`,
		},
		{
			name: "partial quotes",
			atom: `target://my-target"`,
			want: `id.target_id="//my-target"`,
		},
		{
			name:      "not enough parts",
			atom:      "target",
			wantError: true,
		},
		{
			name:      "unknown atom",
			atom:      "label:foo",
			wantError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := translateAtom(tc.atom)
			if tc.want != got {
				t.Errorf("translateAtom(%q) differed; got %q, want %q", tc.atom, got, tc.want)
			}
			if err == nil && tc.wantError {
				t.Errorf("translateAtom(%q) did not error as expected", tc.atom)
			} else if err != nil && !tc.wantError {
				t.Errorf("translateAtom(%q) errored unexpectedly: %v", tc.atom, err)
			}
		})
	}
}

func TestTranslateQuery(t *testing.T) {
	cases := []struct {
		name      string
		query     string
		want      string
		wantError bool
	}{
		{
			name:  "empty",
			query: "",
			want:  "",
		},
		{
			name:  "basic",
			query: `target:"//my-target"`,
			want:  `id.target_id="//my-target"`,
		},
		{
			name:      "case-sensitive key",
			query:     `TARGET:"//MY-TARGET"`,
			wantError: true,
		},
		{
			name:  "multiple colons",
			query: `target:"//path/to:my-target"`,
			want:  `id.target_id="//path/to:my-target"`,
		},
		{
			name:      "unquoted",
			query:     `target://my-target`,
			wantError: true,
		},
		{
			name:      "partial quotes",
			query:     `target://my-target"`,
			wantError: true,
		},
		{
			name:      "invalid query",
			query:     `label:foo`,
			wantError: true,
		},
		{
			name:      "partial match",
			query:     `some_target:foo`,
			wantError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := TranslateQuery(tc.query)
			if tc.want != got {
				t.Errorf("translateQuery(%q) differed; got %q, want %q", tc.query, got, tc.want)
			}
			if tc.wantError && err == nil {
				t.Errorf("translateQuery(%q) did not error as expected", tc.query)
			} else if !tc.wantError && err != nil {
				t.Errorf("translateQuery(%q) errored unexpectedly: %v", tc.query, err)
			}
		})
	}
}
