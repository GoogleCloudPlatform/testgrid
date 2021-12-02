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

package gcs

import (
	"testing"
)

func TestCleanFilepath(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{
			path: "",
			want: "",
		},
		{
			path: "afile",
			want: "afile",
		},
		{
			path: "path/to/something",
			want: "path/to/something",
		},
		{
			path: "gs://path/to/something",
			want: "gs://path/to/something",
		},
		{
			path: "file:/path/to/something",
			want: "/path/to/something",
		},
		{
			path: "file://path/to/something",
			want: "/path/to/something",
		},
		{
			path: "file:///path/to/something",
			want: "/path/to/something",
		},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			var p Path
			err := p.Set(tc.path)
			if err != nil {
				t.Fatalf("bad path: %v", err)
			}
			if got := cleanFilepath(p); got != tc.want {
				t.Errorf("cleanFilepath(%v) got %q, want %q", p, got, tc.want)
			}
		})
	}
}
