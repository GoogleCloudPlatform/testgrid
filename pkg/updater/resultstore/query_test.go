/*
Copyright 2023 The TestGrid Authors.

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

package resultstore

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestToken(t *testing.T) {
	cases := []struct {
		token      string
		want       string // Expected token when searching invocations
		wantTarget string // Expected token when searching targets
	}{
		// Empty
		{
			token:      "",
			want:       "",
			wantTarget: "",
		},
		{
			token:      "  ",
			want:       "",
			wantTarget: "",
		},
		{
			token:      ":value",
			want:       "",
			wantTarget: "",
		},
		// Logic
		{
			token:      "and",
			want:       "AND",
			wantTarget: "AND",
		},
		{
			token:      "And",
			want:       "AND",
			wantTarget: "AND",
		},
		{
			token:      "not",
			want:       "NOT",
			wantTarget: "NOT",
		},
		{
			token:      "NOT",
			want:       "NOT",
			wantTarget: "NOT",
		},
		{
			token:      "oR ",
			want:       "OR",
			wantTarget: "OR",
		},
		{
			token:      "OR ",
			want:       "OR",
			wantTarget: "OR",
		},
		// Label
		{
			token:      "label:foo",
			want:       "invocation_attributes.labels:\"foo\"",
			wantTarget: "invocation.invocation_attributes.labels:\"foo\"",
		},
		// User
		{
			token:      "user:someone",
			want:       "invocation_attributes.users:\"someone\"",
			wantTarget: "invocation.invocation_attributes.users:\"someone\"",
		},
		// Property
		{
			token:      "some_property:foo",
			want:       "propertyEquals(some_property, \"foo\")",
			wantTarget: "(invocationPropertyEquals(some_property, \"foo\") OR configurationPropertyEquals(some_property, \"foo\"))",
		},
		{
			token:      "SUBMIT_STATUS:POST_SUBMIT",
			want:       "propertyEquals(SUBMIT_STATUS, \"POST_SUBMIT\")",
			wantTarget: "(invocationPropertyEquals(SUBMIT_STATUS, \"POST_SUBMIT\") OR configurationPropertyEquals(SUBMIT_STATUS, \"POST_SUBMIT\"))",
		},
		// Negative tokens
		{
			token:      "-AND",
			want:       "AND",
			wantTarget: "AND",
		},
		{
			token:      "-user:someone",
			want:       "NOT invocation_attributes.users:\"someone\"",
			wantTarget: "NOT invocation.invocation_attributes.users:\"someone\"",
		},
		{
			token:      "-special_type_target:some/special_type:target",
			want:       "NOT propertyEquals(special_type_target, \"some/special_type:target\")",
			wantTarget: "NOT (invocationPropertyEquals(special_type_target, \"some/special_type:target\") OR configurationPropertyEquals(special_type_target, \"some/special_type:target\"))",
		},
		// Capitalization
		{
			token:      "label:FOO",
			want:       "invocation_attributes.labels:\"FOO\"",
			wantTarget: "invocation.invocation_attributes.labels:\"FOO\"",
		},
		{
			token:      "LABEL:foo",
			want:       "invocation_attributes.labels:\"foo\"",
			wantTarget: "invocation.invocation_attributes.labels:\"foo\"",
		},
		// Quotes
		{
			token:      "label:\"foo\"",
			want:       "invocation_attributes.labels:\"foo\"",
			wantTarget: "invocation.invocation_attributes.labels:\"foo\"",
		},
		{
			token:      "special_type_target:\"some/special_type:target\"",
			want:       "propertyEquals(special_type_target, \"some/special_type:target\")",
			wantTarget: "(invocationPropertyEquals(special_type_target, \"some/special_type:target\") OR configurationPropertyEquals(special_type_target, \"some/special_type:target\"))",
		},
		// Targets
		{
			token:      "exact_target://path/to:foo",
			want:       "id.target_id=\"//path/to:foo\"",
			wantTarget: "id.target_id=\"//path/to:foo\"",
		},
		{
			token:      "exact_target:non_blaze_target",
			want:       "id.target_id=\"non_blaze_target\"",
			wantTarget: "id.target_id=\"non_blaze_target\"",
		},
		{
			token:      "target://path/to:foo",
			want:       "id.target_id=\"//path/to:foo\"",
			wantTarget: "id.target_id=\"//path/to:foo\"",
		},
	}

	for _, tc := range cases {
		t.Run(tc.token, func(t *testing.T) {
			// Check token is correct for invocation searches.
			got, err := complexToken(tc.token, false)
			if err != nil {
				t.Fatalf("complexToken(%q) errored unexpectedly: %v", tc.token, err)
			}
			if got != tc.want {
				t.Errorf("complexToken(%q, false): got %q, want %q", tc.token, got, tc.want)
			}
			// Same check for target searches (which require different tokens).
			gotTarget, err := complexToken(tc.token, true)
			if err != nil {
				t.Fatalf("complexToken(%q, true) errored unexpectedly: %v", tc.token, err)
			}
			if gotTarget != tc.wantTarget {
				t.Errorf("complexToken(%q, true): got %q, want %q", tc.token, gotTarget, tc.wantTarget)
			}
		})
	}
}

func TestTokenInvalid(t *testing.T) {
	cases := []struct {
		token  string
		reason ConvertReason
	}{
		{
			token:  "before:2020-01-01",
			reason: BeforeAfter,
		},
		{
			token:  "after:2020-01-01T-08:00",
			reason: BeforeAfter,
		},
		{
			token:  "bare-label",
			reason: Bare,
		},
		{
			token:  "-bare-label",
			reason: Bare,
		},
	}

	for _, tc := range cases {
		t.Run(tc.token, func(t *testing.T) {
			got, err := complexToken(tc.token, false)
			if err == nil {
				t.Fatalf("complexToken(%q, false) expected error but got %q", tc.token, got)
			}
			if qerr, ok := err.(*QueryError); !ok {
				t.Fatalf("unable to convert error (type %T) to type checks.QueryError: %v", err, err)
			} else if qerr.Reason != tc.reason {
				t.Fatalf("want reason %v, got %v", tc.reason, qerr.Reason)
			}
		})
	}
}

func TestIsTargetToken(t *testing.T) {
	cases := []struct {
		token string
		want  bool
	}{
		{
			token: "",
			want:  false,
		},
		{
			token: "  ",
			want:  false,
		},
		{
			token: ":value",
			want:  false,
		},
		{
			token: "NOT",
			want:  false,
		},
		{
			token: "label:foo",
			want:  false,
		},
		{
			token: "user:someone",
			want:  false,
		},
		{
			token: "special_type_target:some/special_type:target",
			want:  false,
		},
		{
			token: "some_property:foo",
			want:  false,
		},
		{
			token: "-user:someone",
			want:  false,
		},
		{
			token: "-special_type_target:some/special_type:target",
			want:  false,
		},
		{
			token: "label:\"foo\"",
			want:  false,
		},
		{
			token: "special_type_target:\"some/special_type:target\"",
			want:  false,
		},
		{
			token: "exact_target://path/to:foo",
			want:  true,
		},
		{
			token: "exact_target:non_blaze_target",
			want:  true,
		},
		{
			token: "target://path/to:foo",
			want:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.token, func(t *testing.T) {
			if got := isTargetToken(tc.token); got != tc.want {
				t.Errorf("isTargetToken(%q): got %t, want %t", tc.token, got, tc.want)
			}
		})
	}
}

func TestQuery(t *testing.T) {
	cases := []struct {
		query string
		want  string
	}{
		{
			query: "",
			want:  "",
		},
		{
			query: "   ",
			want:  "",
		},
		{
			query: "label:foo",
			want:  "invocation_attributes.labels:\"foo\"",
		},
		{
			query: "  label:foo  ",
			want:  "invocation_attributes.labels:\"foo\"",
		},
		{
			query: "label:foo user:someone",
			want:  "invocation_attributes.labels:\"foo\" invocation_attributes.users:\"someone\"",
		},
		{
			query: "-label:foo label:bar",
			want:  "NOT invocation_attributes.labels:\"foo\" invocation_attributes.labels:\"bar\"",
		},
		{
			query: "label:foo or label:bar",
			want:  "invocation_attributes.labels:\"foo\" OR invocation_attributes.labels:\"bar\"",
		},
		{
			query: "-status:built label:foo",
			want:  "invocation_attributes.labels:\"foo\"",
		},
		{
			query: "status:built special_type_target:some/special_type:target",
			want:  "propertyEquals(special_type_target, \"some/special_type:target\")",
		},
		{
			query: "label:\"foo\" special_type_target:some/special_type:target",
			want:  "invocation_attributes.labels:\"foo\" propertyEquals(special_type_target, \"some/special_type:target\")",
		},
		{
			query: "label:foo special_type_target:some/special_type:target target://my/test:target",
			want:  "invocation.invocation_attributes.labels:\"foo\" (invocationPropertyEquals(special_type_target, \"some/special_type:target\") OR configurationPropertyEquals(special_type_target, \"some/special_type:target\")) id.target_id=\"//my/test:target\"",
		},
		{
			query: "label:\"foo bar\" label:'baz baz'",
			want:  "invocation_attributes.labels:\"foo bar\" invocation_attributes.labels:\"baz baz\"",
		},
		{
			query: "exact_target://oh/no:bad label:foo",
			want:  "id.target_id=\"//oh/no:bad\" invocation.invocation_attributes.labels:\"foo\"",
		},
		{
			query: "target://oh/no:bad label:foo",
			want:  "id.target_id=\"//oh/no:bad\" invocation.invocation_attributes.labels:\"foo\"",
		},
		{
			query: "label:something SUBMIT_STATUS:POST_SUBMIT",
			want:  "invocation_attributes.labels:\"something\" propertyEquals(SUBMIT_STATUS, \"POST_SUBMIT\")",
		},
		{
			query: "label:foo label:bar bare-label",
			want:  "invocation_attributes.labels:\"foo\" invocation_attributes.labels:\"bar\" invocation_attributes.labels:\"bare-label\"",
		},
		{
			query: "label: bare-label",
			want:  "invocation_attributes.labels:\"label:\" invocation_attributes.labels:\"bare-label\"",
		},
		{
			query: "label: -bare-label",
			want:  "invocation_attributes.labels:\"label:\" NOT invocation_attributes.labels:\"bare-label\"",
		},
		{
			query: "label:foo empty:",
			want:  "invocation_attributes.labels:\"foo\" invocation_attributes.labels:\"empty:\"",
		},
		{
			query: "(label:foo label:bar)",
			want:  "( invocation_attributes.labels:\"foo\" invocation_attributes.labels:\"bar\" )",
		},
		{
			query: "label:\"foo (bar)\"",
			want:  "invocation_attributes.labels:\"foo (bar)\"",
		},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			got, err := ComplexQuery(tc.query)
			if err != nil {
				t.Errorf("complexQuery(%q) errored unexpectedly: %v", tc.query, err)
			}
			if got != tc.want {
				t.Errorf("complexQuery(%q): got %q, want %q", tc.query, got, tc.want)
			}
		})
	}
}

func TestQueryInvalid(t *testing.T) {
	cases := []struct {
		query string
	}{
		{"label:foo before:2020-01-01"},
		{"after:2020-01-01"},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			got, err := ComplexQuery(tc.query)
			if err == nil {
				t.Errorf("complexQuery(%q) expected error but got %q", tc.query, got)
			}
		})
	}
}

func TestSearch(t *testing.T) {
	invocQuery := "invocation_attributes.labels:\"foo\""
	targetQuery := "id.target_id=\"//my/test/path:test\""
	sampleInvocs := map[string]fakePage{
		"": {
			nextPageToken: "page-1",
			invocationIDs: []string{"a1", "a2", "a3"},
		},
		"page-1": {
			nextPageToken: "page-2",
			invocationIDs: []string{"b1", "b2"},
		},
		"page-2": {
			nextPageToken: "",
			invocationIDs: []string{"c1", "c2", "c3"},
		},
	}
	cases := []struct {
		name   string
		query  string
		client fakeResultStoreClient
		max    int
		want   []string
		err    bool
	}{
		{
			name:   "empty query",
			query:  "",
			client: fakeResultStoreClient{invocations: sampleInvocs},
			err:    true,
		},
		{
			name:  "empty client",
			query: invocQuery,
			err:   true,
		},
		{
			name:   "negative max",
			query:  invocQuery,
			client: fakeResultStoreClient{invocations: sampleInvocs},
			max:    -1,
			err:    true,
		},
		{
			name:   "no invoc results",
			query:  invocQuery,
			client: fakeResultStoreClient{invocations: map[string]fakePage{}},
			err:    true,
		},
		{
			name:   "no target results",
			query:  targetQuery,
			client: fakeResultStoreClient{invocations: map[string]fakePage{}},
			err:    true,
		},
		{
			name:   "invoc results",
			query:  invocQuery,
			client: fakeResultStoreClient{invocations: sampleInvocs},
			want:   []string{"a1", "a2", "a3", "b1", "b2", "c1", "c2", "c3"},
		},
		{
			name:   "target results",
			query:  targetQuery,
			client: fakeResultStoreClient{invocations: sampleInvocs},
			want:   []string{"a1", "a2", "a3", "b1", "b2", "c1", "c2", "c3"},
		},
		{
			name:   "max",
			query:  targetQuery,
			client: fakeResultStoreClient{invocations: sampleInvocs},
			max:    5,
			want:   []string{"a1", "a2", "a3", "b1", "b2"},
		},
		{
			name:   "max smaller than page",
			query:  targetQuery,
			client: fakeResultStoreClient{invocations: sampleInvocs},
			max:    1,
			want:   []string{"a1", "a2", "a3"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _, err := search(context.Background(), tc.query, tc.client, "fake-project-id", tc.max)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("Search(%s) differed (-want, +got): %s", tc.query, diff)
			}
			if tc.err && err == nil {
				t.Errorf("Search(%q) did not error as expected", tc.query)
			}
			if !tc.err && err != nil {
				t.Errorf("Search(%q) errored unexpectedly: %v", tc.query, err)
			}
		})
	}
}

func TestNewSearches(t *testing.T) {
	sampleInvocs := map[string]fakePage{
		"": {
			nextPageToken: "page-1",
			invocationIDs: []string{"a1", "a2", "a3"},
		},
		"page-1": {
			nextPageToken: "page-2",
			invocationIDs: []string{"b1", "b2"},
		},
		"page-2": {
			nextPageToken: "",
			invocationIDs: []string{"c1", "c2", "c3"},
		},
	}
	cases := []struct {
		name          string
		client        fakeResultStoreClient
		pageSize      int
		pageToken     string
		want          []string
		wantPageToken string
		err           bool
	}{
		{
			name: "empty client",
			err:  true,
		},
		{
			name:   "no results",
			client: fakeResultStoreClient{invocations: map[string]fakePage{}},
			err:    true,
		},
		{
			name:     "negative pageSize",
			client:   fakeResultStoreClient{invocations: sampleInvocs},
			pageSize: -1,
			err:      true,
		},
		{
			name:          "zero pageSize",
			client:        fakeResultStoreClient{invocations: sampleInvocs},
			pageSize:      0,
			want:          []string{"a1", "a2", "a3"},
			wantPageToken: "page-1",
		},
		{
			name:          "empty pageToken",
			client:        fakeResultStoreClient{invocations: sampleInvocs},
			pageSize:      3,
			want:          []string{"a1", "a2", "a3"},
			wantPageToken: "page-1",
		},
		{
			name:          "pageToken",
			client:        fakeResultStoreClient{invocations: sampleInvocs},
			pageSize:      3,
			pageToken:     "page-1",
			want:          []string{"b1", "b2"},
			wantPageToken: "page-2",
		},
		{
			name:          "last pageToken",
			client:        fakeResultStoreClient{invocations: sampleInvocs},
			pageSize:      3,
			pageToken:     "page-2",
			want:          []string{"c1", "c2", "c3"},
			wantPageToken: "",
		},
		{
			name:          "missing pageToken",
			client:        fakeResultStoreClient{invocations: sampleInvocs},
			pageSize:      3,
			pageToken:     "missing-page",
			want:          nil,
			wantPageToken: "",
			err:           true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Test invocationSearch()
			got, _, gotPageToken, err := invocationSearch(context.Background(), "fake-query", tc.client, "fake-project-id", tc.pageSize, tc.pageToken)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("invocationSearch(%d, %s)) differed (-want, +got): %s", tc.pageSize, tc.pageToken, diff)
			}
			if tc.wantPageToken != gotPageToken {
				t.Errorf("invocationSearch(%d, %s) page token differed; want %q, got %q", tc.pageSize, tc.pageToken, tc.wantPageToken, gotPageToken)
			}
			if tc.err && err == nil {
				t.Errorf("invocationSearch(%d, %s)) did not error as expected", tc.pageSize, tc.pageToken)
			}
			if !tc.err && err != nil {
				t.Errorf("invocationSearch(%d, %s)) errored unexpectedly: %v", tc.pageSize, tc.pageToken, err)
			}

			// Test targetSearch()
			got, gotPageToken, err = targetSearch(context.Background(), "fake-query", tc.client, "fake-project-id", tc.pageSize, tc.pageToken)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("targetSearch(%d, %s)) differed (-want, +got): %s", tc.pageSize, tc.pageToken, diff)
			}
			if tc.wantPageToken != gotPageToken {
				t.Errorf("targetSearch(%d, %s) page token differed; want %q, got %q", tc.pageSize, tc.pageToken, tc.wantPageToken, gotPageToken)
			}
			if tc.err && err == nil {
				t.Errorf("targetSearch(%d, %s)) did not error as expected", tc.pageSize, tc.pageToken)
			}
			if !tc.err && err != nil {
				t.Errorf("targetSearch(%d, %s)) errored unexpectedly: %v", tc.pageSize, tc.pageToken, err)
			}
		})
	}
}
