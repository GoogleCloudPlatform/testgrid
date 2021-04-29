/*
Copyright 2021 The Kubernetes Authors.

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

	"github.com/google/go-cmp/cmp"
)

// TestLeastRecentlyUpdated in util/gcs/fake/sort_test
// TestTouch in util/gcs/fake/sort_test

func TestSort(t *testing.T) {
	cases := []struct {
		name   string
		builds []Build
		want   []Build
	}{
		{
			name: "basic",
		},
		{
			name: "sorted",
			builds: []Build{
				{baseName: "c"},
				{baseName: "b"},
				{baseName: "a"},
			},
			want: []Build{
				{baseName: "c"},
				{baseName: "b"},
				{baseName: "a"},
			},
		},
		{
			name: "resort",
			builds: []Build{
				{baseName: "b"},
				{baseName: "c"},
				{baseName: "a"},
			},
			want: []Build{
				{baseName: "c"},
				{baseName: "b"},
				{baseName: "a"},
			},
		},
		{
			name: "numerics",
			builds: []Build{
				{baseName: "a1b"},
				{baseName: "a10b"},
				{baseName: "a2b"},
				{baseName: "a3b"},
			},
			want: []Build{
				{baseName: "a10b"},
				{baseName: "a3b"},
				{baseName: "a2b"},
				{baseName: "a1b"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			Sort(tc.builds)
			if diff := cmp.Diff(tc.want, tc.builds, cmp.AllowUnexported(Build{}, Path{})); diff != "" {
				t.Errorf("Sort() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}
