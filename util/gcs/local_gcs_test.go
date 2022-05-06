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
	"context"
	"os"
	"path"
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
		{
			path: "file:/base/valid-in gcs!",
			want: `/base/valid-in gcs!`,
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

func TestUpload_UploadsToLocalStorage(t *testing.T) {
	tests := []struct {
		name        string
		destination string
		ExpectErr   bool
		ExpectStat  bool
	}{
		{
			name:       "won't upload nothing",
			ExpectErr:  true,
			ExpectStat: true,
		},
		{
			name:        "uploads files",
			destination: "foo",
			ExpectStat:  true,
		},
		{
			name:        "uploads nested files",
			destination: "foo/bar",
			ExpectStat:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client := NewLocalClient()
			base := t.TempDir()

			location, err := NewPath(path.Join(base, tc.destination))
			if err != nil || location == nil {
				t.Fatalf("Fatal error with location %v; %v", location, err)
			}

			_, err = client.Upload(ctx, *location, []byte("some-content"), DefaultACL, NoCache)
			if tc.ExpectErr && err == nil {
				t.Error("Expected error, but got none")
			}
			if !tc.ExpectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if tc.ExpectStat {
				_, err = os.Stat(path.Join(base, tc.destination))
				if err != nil {
					t.Errorf("Stat failed: %v", err)
				}
			}
		})
	}
}
