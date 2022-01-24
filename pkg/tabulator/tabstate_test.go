/*
Copyright 2022 The TestGrid Authors.

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

package tabulator

import (
	"testing"

	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/google/go-cmp/cmp"
)

func TestTabStatePath(t *testing.T) {
	path := newPathOrDie("gs://bucket/config")
	cases := []struct {
		name           string
		dashboardName  string
		tabName        string
		tabStatePrefix string
		expected       *gcs.Path
	}{
		{
			name:     "basically works",
			expected: path,
		},
		{
			name:          "invalid dashboard name errors",
			dashboardName: "---://foo",
			tabName:       "ok",
		},
		{
			name:          "invalid tab name errors",
			dashboardName: "cool",
			tabName:       "--??!f///",
		},
		{
			name:          "bucket change errors",
			dashboardName: "gs://honey-bucket/config",
			tabName:       "tab",
		},
		{
			name:          "normal behavior works",
			dashboardName: "dashboard",
			tabName:       "some-tab",
			expected:      newPathOrDie("gs://bucket/dashboard/some-tab"),
		},
		{
			name:           "target a subfolder works",
			tabStatePrefix: "tab-state",
			dashboardName:  "dashboard",
			tabName:        "some-tab",
			expected:       newPathOrDie("gs://bucket/tab-state/dashboard/some-tab"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := tabStatePath(*path, tc.tabStatePrefix, tc.dashboardName, tc.tabName)
			switch {
			case err != nil:
				if tc.expected != nil {
					t.Errorf("tabStatePath(%v, %v) got unexpected error: %v", tc.dashboardName, tc.tabName, err)
				}
			case tc.expected == nil:
				t.Errorf("tabStatePath(%v, %v) failed to receive an error", tc.dashboardName, tc.tabName)
			default:
				if diff := cmp.Diff(actual, tc.expected, cmp.AllowUnexported(gcs.Path{})); diff != "" {
					t.Errorf("tabStatePath(%v, %v) got unexpected diff (-have, +want):\n%s", tc.dashboardName, tc.tabName, diff)
				}
			}
		})
	}
}

func newPathOrDie(s string) *gcs.Path {
	p, err := gcs.NewPath(s)
	if err != nil {
		panic(err)
	}
	return p
}
