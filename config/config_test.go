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
	multierror "github.com/hashicorp/go-multierror"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "normal",
			expected: "normal",
		},
		{
			input:    "UPPER",
			expected: "upper",
		},
		{
			input:    "pun-_*ctuation Y_E_A_H!",
			expected: "punctuationyeah",
		},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got := normalize(test.input)
			if got != test.expected {
				t.Fatalf("got %s, want %s", got, test.expected)
			}
		})
	}
}

func TestUpdate_Validate(t *testing.T) {
	tests := []struct {
		name            string
		input           configpb.Configuration
		expectedMissing string
		expectedDup     map[string]bool
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
		{
			name: "No Duplicate Test Groups",
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
					{
						Name: "TEST_GROUP_1",
					},
				},
			},
			expectedDup: map[string]bool{"testgroup1": true},
		},
		{
			name: "No Duplicate Tabs in Dashboard",
			input: configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dashboard_1",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name: "tab_1",
							},
							{
								Name: "TAB_1",
							},
							{
								Name: "tab_2",
							},
						},
					},
					{
						Name: "dashboard_2",
						// Same tab name in a different dashboard is fine.
						DashboardTab: []*configpb.DashboardTab{
							{
								Name: "tab_2",
							},
						},
					},
				},
				TestGroups: []*configpb.TestGroup{
					{
						Name: "test_group_1",
					},
				},
			},
			expectedDup: map[string]bool{"tab1": true},
		},
		{
			name: "No Duplicate Dashboard or Dashboard Group",
			input: configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "name_1",
					},
				},
				DashboardGroups: []*configpb.DashboardGroup{
					{
						Name: "name_1",
					},
				},
				TestGroups: []*configpb.TestGroup{
					{
						Name: "test_group_1",
					},
				},
			},
			expectedDup: map[string]bool{"name1": true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Validate(test.input)
			if err != nil && test.expectedMissing == "" && len(test.expectedDup) == 0 {
				t.Fatalf("Unexpected Error: %v", err)
			}

			if test.expectedMissing != "" {
				if err == nil {
					t.Errorf("Expected MissingFieldError(%s), but got no error", test.expectedMissing)
				} else if e, ok := err.(MissingFieldError); !ok || e.Field != test.expectedMissing {
					t.Errorf("Expected MissingFieldError(%s), but got %v", test.expectedMissing, err)
				}
			}

			if len(test.expectedDup) != 0 {
				if err == nil {
					t.Errorf("Expected DuplicateNameError(%v), but got no error", test.expectedDup)
				}

				if mErr, ok := err.(*multierror.Error); ok {
					for _, e := range mErr.Errors {
						if dupErr, ok := e.(DuplicateNameError); !ok || !test.expectedDup[dupErr.Name] {
							t.Errorf("Expected DuplicateNameError(%s), but got %v", test.expectedDup, dupErr)
						}
					}
				} else {
					t.Errorf("Expected DuplicateNameError(%s), but got %v", test.expectedDup, err)
				}
			}
		})
	}
}
