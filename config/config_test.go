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

package config

import (
	"testing"

	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
)

func TestUpdate_Validate(t *testing.T) {
	tests := []struct {
		name            string
		input           configpb.Configuration
		expectedMissing string
	}{
		{
			name:            "Null input; returns error",
			expectedMissing: "TestGroups",
		},
		{
			name: "Dashboard Only; returns error",
			input: configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dashboard_1",
					},
				},
			},
			expectedMissing: "TestGroups",
		},
		{
			name: "Test Group Only; returns error",
			input: configpb.Configuration{
				TestGroups: []*configpb.TestGroup{
					{
						Name: "test_group_1",
					},
				},
			},
			expectedMissing: "Dashboards",
		},
		{
			name: "Complete Minimal Config",
			input: configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dashboard_1",
					},
				},
				TestGroups: []*configpb.TestGroup{
					{
						Name: "test_group_1",
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Validate(test.input)
			if err != nil && test.expectedMissing == "" {
				t.Fatalf("Unexpected Error: %v", err)
			}

			if test.expectedMissing != "" {
				if err == nil {
					t.Errorf("Expected MissingFieldError(%s), but got no error", test.expectedMissing)
				} else if e, ok := err.(MissingFieldError); !ok || e.Field != test.expectedMissing {
					t.Errorf("Expected MissingFieldError(%s), but got %v", test.expectedMissing, err)
				}
			}
		})
	}
}
