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
		name       string
		atom       keyValue
		want       string
		wantTarget string
	}{
		{
			name:       "label atom",
			atom:       keyValue{"label", "foo"},
			want:       `invocation_attributes.labels:"foo"`,
			wantTarget: `invocation.invocation_attributes.labels:"foo"`,
		},
		{
			name:       "target atom",
			atom:       keyValue{"target", "//my-target"},
			want:       `id.target_id="//my-target"`,
			wantTarget: `id.target_id="//my-target"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Invocation queries (queryTarget = false)
			got, err := translateAtom(tc.atom, false)
			if err != nil {
				t.Fatalf("translateAtom(%q, %t) errored: %v", tc.atom, false, err)
			}
			if tc.want != got {
				t.Errorf("translateAtom(%q, %t) differed; got %q, want %q", tc.atom, false, got, tc.want)
			}
			// Configured Target queries (queryTarget = true)
			gotTarget, err := translateAtom(tc.atom, true)
			if err != nil {
				t.Fatalf("translateAtom(%q, %t) errored: %v", tc.atom, true, err)
			}
			if tc.wantTarget != gotTarget {
				t.Errorf("translateAtom(%q, %t) differed; got %q, want %q", tc.atom, true, gotTarget, tc.wantTarget)
			}
		})
	}
}

func TestTranslateAtom_Error(t *testing.T) {
	cases := []struct {
		name string
		atom keyValue
	}{
		{
			name: "empty",
			atom: keyValue{"", ""},
		},
		{
			name: "case-sensitive key",
			atom: keyValue{"TARGET", "//MY-TARGET"},
		},
		{
			name: "missing key",
			atom: keyValue{"", "//path/to:my-target"},
		},
		{
			name: "missing value",
			atom: keyValue{"target", ""},
		},
		{
			name: "unknown atom",
			atom: keyValue{"custom-property", "foo"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Invocation queries (queryTarget = false)
			_, err := translateAtom(tc.atom, false)
			if err == nil {
				t.Fatalf("translateAtom(%q, %t): want error but got none", tc.atom, false)
			}
			// Configured Target queries (queryTarget = true)
			_, err = translateAtom(tc.atom, true)
			if err == nil {
				t.Fatalf("translateAtom(%q, %t): want error but got none", tc.atom, true)
			}
		})
	}
}

func TestTranslateQuery(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{
			name:  "empty",
			query: "",
			want:  "",
		},
		{
			name:  "label",
			query: `label:"foo"`,
			want:  `invocation_attributes.labels:"foo"`,
		},
		{
			name:  "target",
			query: `target:"//my-target"`,
			want:  `id.target_id="//my-target"`,
		},
		{
			name:  "multiple labels",
			query: `label:"foo" label:"bar"`,
			want:  `invocation_attributes.labels:"foo" invocation_attributes.labels:"bar"`,
		},
		{
			name:  "mixed",
			query: `label:"foo" target:"//my-target" label:"bar"`,
			want:  `invocation.invocation_attributes.labels:"foo" id.target_id="//my-target" invocation.invocation_attributes.labels:"bar"`,
		},
		{
			name:  "multiple colons",
			query: `target:"//path/to:my-target"`,
			want:  `id.target_id="//path/to:my-target"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := TranslateQuery(tc.query)
			if err != nil {
				t.Fatalf("translateQuery(%q) errored: %v", tc.query, err)
			}
			if tc.want != got {
				t.Errorf("translateQuery(%q) differed; got %q, want %q", tc.query, got, tc.want)
			}
		})
	}
}

func TestTranslateQuery_Error(t *testing.T) {
	cases := []struct {
		name  string
		query string
	}{
		{
			name:  "case-sensitive key",
			query: `TARGET:"//MY-TARGET"`,
		},
		{
			name:  "unquoted",
			query: `target://my-target`,
		},
		{
			name:  "partial quotes",
			query: `target://my-target"`,
		},
		{
			name:  "invalid query",
			query: `label:foo`,
		},
		{
			name:  "partial match",
			query: `some_target:foo`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := TranslateQuery(tc.query)
			if err == nil {
				t.Fatalf("translateQuery(%q): want error, got none", tc.query)
			}
		})
	}
}
