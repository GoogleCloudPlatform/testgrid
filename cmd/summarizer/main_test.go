/*
Copyright 2019 The Kubernetes Authors.

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

package main

import (
	"testing"
	"time"

	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
)

func TestStaleHours(t *testing.T) {
	cases := []struct {
		name     string
		tab      configpb.DashboardTab
		expected time.Duration
	}{
		{
			name:     "zero without an alert",
			expected: 0,
		},
		{
			name: "use defined hours when set",
			tab: configpb.DashboardTab{
				AlertOptions: &configpb.DashboardTabAlertOptions{
					AlertStaleResultsHours: 4,
				},
			},
			expected: 4 * time.Hour,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual := staleHours(&tc.tab); actual != tc.expected {
				t.Errorf("actual %v != expected %v", actual, tc.expected)
			}
		})
	}
}
