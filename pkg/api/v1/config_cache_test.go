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

package v1

import (
	"testing"

	"github.com/GoogleCloudPlatform/testgrid/config/snapshot"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestGenerateNormalCache(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *snapshot.Config
		expected *cachedConfig
	}{
		{
			name:     "Generate nothing for nil config",
			expected: &cachedConfig{},
		},
		{
			name: "Generate empty sets for empty config",
			cfg:  &snapshot.Config{},
			expected: &cachedConfig{
				NormalDashboardGroup: map[string]string{},
				NormalDashboard:      map[string]string{},
				NormalTestGroup:      map[string]string{},
				NormalDashboardTab:   map[string]map[string]string{},
			},
		},
		{
			name: "Generate normalized names for all things",
			cfg: &snapshot.Config{
				DashboardGroups: map[string]*configpb.DashboardGroup{
					"My Dashboard Group": {Name: "My Dashboard Group"},
				},
				Dashboards: map[string]*configpb.Dashboard{
					"Chessboard ♟️♙": {
						Name: "Chessboard ♟️♙",
						DashboardTab: []*configpb.DashboardTab{
							{Name: "Pawns ♟️"},
							{Name: "Knights ♞"},
						},
					},
				},
				Groups: map[string]*configpb.TestGroup{
					"Groups: 一緒にあるいくつかのこと": {Name: "Groups: 一緒にあるいくつかのこと"},
				},
			},
			expected: &cachedConfig{
				NormalDashboardGroup: map[string]string{
					"mydashboardgroup": "My Dashboard Group",
				},
				NormalDashboard: map[string]string{
					"chessboard": "Chessboard ♟️♙",
				},
				NormalTestGroup: map[string]string{
					"groups": "Groups: 一緒にあるいくつかのこと",
				},
				NormalDashboardTab: map[string]map[string]string{
					"chessboard": {
						"pawns":   "Pawns ♟️",
						"knights": "Knights ♞",
					},
				},
			},
		},
		{
			name: "Tolerate dashboards with the same normalized tab name",
			cfg: &snapshot.Config{
				Dashboards: map[string]*configpb.Dashboard{
					"Black Pieces": {
						Name: "Black Pieces",
						DashboardTab: []*configpb.DashboardTab{
							{Name: "Pawns ♟️"},
							{Name: "Knights ♞"},
						},
					},
					"White Pieces": {
						Name: "White Pieces",
						DashboardTab: []*configpb.DashboardTab{
							{Name: "Pawns ♙"},
							{Name: "Knights ♘"},
						},
					},
				},
			},
			expected: &cachedConfig{
				NormalDashboardGroup: map[string]string{},
				NormalTestGroup:      map[string]string{},
				NormalDashboard: map[string]string{
					"blackpieces": "Black Pieces",
					"whitepieces": "White Pieces",
				},
				NormalDashboardTab: map[string]map[string]string{
					"blackpieces": {
						"pawns":   "Pawns ♟️",
						"knights": "Knights ♞",
					},
					"whitepieces": {
						"pawns":   "Pawns ♙",
						"knights": "Knights ♘",
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cache := cachedConfig{
				Config: tc.cfg,
			}
			cache.generateNormalCache()

			if diff := cmp.Diff(&cache, tc.expected, cmpopts.IgnoreFields(cachedConfig{}, "Config", "Mutex")); diff != "" {
				t.Errorf("Unexpected Diff: (-got, +want): %s", diff)
			}
		})
	}
}

func TestConfigPath(t *testing.T) {
	tests := []struct {
		name          string
		defaultBucket string
		scopeParam    string
		expected      *gcs.Path
		expectDefault bool
	}{
		{
			name:          "Defaults to default",
			defaultBucket: "gs://example",
			expected:      getPathOrDie(t, "gs://example/config"),
			expectDefault: true,
		},
		{
			name:          "Use config if specified",
			defaultBucket: "gs://wrong",
			scopeParam:    "gs://example/path",
			expected:      getPathOrDie(t, "gs://example/path/config"),
		},
		{
			name:       "Do not require a default",
			scopeParam: "gs://example/path",
			expected:   getPathOrDie(t, "gs://example/path/config"),
		},
		{
			name: "Return error if no way to find config",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := Server{
				DefaultBucket: test.defaultBucket,
			}

			result, isDefault, err := s.configPath(test.scopeParam)
			if test.expected == nil && err == nil {
				t.Fatalf("Expected an error, but got none")
			}

			if test.expectDefault != isDefault {
				t.Errorf("Default Flag: Want %t, got %t", test.expectDefault, isDefault)
			}

			if test.expected != nil && result.String() != test.expected.String() {
				t.Errorf("Want %s, but got %s", test.expected.String(), result.String())
			}
		})
	}
}
