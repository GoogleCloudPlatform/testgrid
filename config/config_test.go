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
	"reflect"
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

func TestValidateUnique(t *testing.T) {
	tests := []struct {
		name         string
		input        []string
		expectedErrs []error
	}{
		{
			name:  "No names",
			input: []string{},
		},
		{
			name:  "Unique names",
			input: []string{"test_group_1", "test_group_2", "test_group_3"},
		},
		{
			name:  "Duplicate name; error",
			input: []string{"test_group_1", "test_group_1"},
			expectedErrs: []error{
				DuplicateNameError{"testgroup1", "TestGroup"},
			},
		},
		{
			name:  "Duplicate name after normalization; error",
			input: []string{"test_group_1", "TEST GROUP 1"},
			expectedErrs: []error{
				DuplicateNameError{"testgroup1", "TestGroup"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateUnique(test.input, "TestGroup")
			if err == nil {
				if len(test.expectedErrs) > 0 {
					t.Fatalf("Expected %v, but got no error", test.expectedErrs)
				}
			} else {
				if len(test.expectedErrs) == 0 {
					t.Fatalf("Unexpected Error: %v", err)
				}

				if mErr, ok := err.(*multierror.Error); ok {
					if !reflect.DeepEqual(test.expectedErrs, mErr.Errors) {
						t.Fatalf("Expected %v, but got: %v", test.expectedErrs, mErr.Errors)
					}
				} else {
					t.Fatalf("Expected %v, but got: %v", test.expectedErrs, err)
				}
			}
		})
	}
}

func TestValidateAllUnique(t *testing.T) {
	cases := []struct {
		name string
		c    configpb.Configuration
		pass bool
	}{
		{
			name: "basically works",
			pass: true,
		},
		{
			name: "everything works",
			c: configpb.Configuration{
				TestGroups: []*configpb.TestGroup{
					{
						Name: "test_group_1",
					},
				},
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dash",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name: "tab_1",
							},
						},
					},
				},
				DashboardGroups: []*configpb.DashboardGroup{
					{
						Name: "dashboard_group_1",
					},
				},
			},
			pass: true,
		},
		{
			name: "reject empty group names",
			c: configpb.Configuration{
				TestGroups: []*configpb.TestGroup{
					{},
				},
			},
		},
		{
			name: "reject empty dashboard names",
			c: configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{},
				},
			},
		},
		{
			name: "reject empty tab names",
			c: configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dashboard_1",
						DashboardTab: []*configpb.DashboardTab{
							{},
						},
					},
				},
			},
		},
		{
			name: "reject empty dashboard group names",
			c: configpb.Configuration{
				DashboardGroups: []*configpb.DashboardGroup{
					{},
				},
			},
		},
		{
			name: "dashboard group names cannot match a dashboard name",
			c: configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "foo",
					},
				},
				DashboardGroups: []*configpb.DashboardGroup{
					{
						Name: "foo",
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAllUnique(tc.c)
			switch {
			case err != nil:
				if tc.pass {
					t.Errorf("got unexpected error: %v", err)
				}
			case !tc.pass:
				t.Error("failed to get an error")
			}

		})
	}
}

func TestValidateReferencesExist_Simple(t *testing.T) {
	cases := []struct {
		name string
		c    configpb.Configuration
		pass bool
	}{
		{
			name: "basically works",
			pass: true,
		},
		{
			name: "accept filled column headers",
			c: configpb.Configuration{
				TestGroups: []*configpb.TestGroup{
					{
						Name: "test_group",
						ColumnHeader: []*configpb.TestGroup_ColumnHeader{
							{
								ColumnHeaderSource: &configpb.TestGroup_ColumnHeader_ConfigurationValue{
									ConfigurationValue: "sad",
								},
							},
						},
					},
				},
			},
			pass: true,
		},
		{
			name: "reject empty column headers",
			c: configpb.Configuration{
				TestGroups: []*configpb.TestGroup{
					{
						Name: "test_group",
						ColumnHeader: []*configpb.TestGroup_ColumnHeader{
							{},
						},
					},
				},
			},
		},
		{
			name: "accept balanced name formats",
			c: configpb.Configuration{
				TestGroups: []*configpb.TestGroup{
					{
						Name: "simple",
						TestNameConfig: &configpb.TestNameConfig{
							NameFormat: "hello world",
						},
					},
					{
						Name: "complex",
						TestNameConfig: &configpb.TestNameConfig{
							NameFormat: "hello %s you are %s",
							NameElements: []*configpb.TestNameConfig_NameElement{
								{
									Labels: "world",
								},
								{
									Labels: "great",
								},
							},
						},
					},
				},
			},
			pass: true,
		},
		{
			name: "reject unbalanced name formats",
			c: configpb.Configuration{
				TestGroups: []*configpb.TestGroup{
					{
						Name: "bad",
						TestNameConfig: &configpb.TestNameConfig{
							NameFormat: "sorry %s but this is just too %s to tell you",
							NameElements: []*configpb.TestNameConfig_NameElement{
								{
									Labels: "charlie",
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, tg := range tc.c.TestGroups {
				// Auto-setup valid dashboard references, we are not testing this.
				tc.c.Dashboards = append(tc.c.Dashboards, &configpb.Dashboard{
					Name: tg.Name,
					DashboardTab: []*configpb.DashboardTab{
						{
							Name:          tg.Name,
							TestGroupName: tg.Name,
						},
					},
				})
			}
			err := validateReferencesExist(tc.c)
			switch {
			case err != nil:
				if tc.pass {
					t.Errorf("got unexpected error: %v", err)
				}
			case !tc.pass:
				t.Error("failed to get an error")
			}
		})
	}

}

func TestValidateReferencesExist(t *testing.T) {
	tests := []struct {
		name         string
		input        configpb.Configuration
		expectedErrs []error
	}{
		{
			name: "Dashboard Tabs must reference an existing Test Group",
			input: configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dashboard_1",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name:          "tab_1",
								TestGroupName: "test_group_1",
							},
							{
								Name:          "tab_2",
								TestGroupName: "test_group_2",
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
			expectedErrs: []error{
				MissingEntityError{"test_group_2", "TestGroup"},
			},
		},
		{
			name: "Test Groups must have an associated Dashboard Tab",
			input: configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name:         "dashboard_1",
						DashboardTab: []*configpb.DashboardTab{},
					},
				},
				TestGroups: []*configpb.TestGroup{
					{
						Name: "test_group_1",
					},
				},
			},
			expectedErrs: []error{
				ConfigError{"test_group_1", "TestGroup", "Each Test Group must be referenced by at least 1 Dashboard Tab."},
			},
		},
		{
			name: "Dashboard Groups must reference existing Dashboards",
			input: configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dashboard_1",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name:          "tab_1",
								TestGroupName: "test_group_1",
							},
						},
					},
				},
				TestGroups: []*configpb.TestGroup{
					{
						Name: "test_group_1",
					},
				},
				DashboardGroups: []*configpb.DashboardGroup{
					{
						Name:           "dashboard_group_1",
						DashboardNames: []string{"dashboard_1", "dashboard_2", "dashboard_3"},
					},
				},
			},
			expectedErrs: []error{
				MissingEntityError{"dashboard_2", "Dashboard"},
				MissingEntityError{"dashboard_3", "Dashboard"},
			},
		},
		{
			name: "A Dashboard can belong to at most 1 Dashboard Group",
			input: configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dashboard_1",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name:          "tab_1",
								TestGroupName: "test_group_1",
							},
						},
					},
				},
				TestGroups: []*configpb.TestGroup{
					{
						Name: "test_group_1",
					},
				},
				DashboardGroups: []*configpb.DashboardGroup{
					{
						Name:           "dashboard_group_1",
						DashboardNames: []string{"dashboard_1"},
					},
					{
						Name:           "dashboard_group_2",
						DashboardNames: []string{"dashboard_1"},
					},
				},
			},
			expectedErrs: []error{
				ConfigError{"dashboard_1", "Dashboard", "A Dashboard cannot be in more than 1 Dashboard Group."},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateReferencesExist(test.input)
			if err != nil && len(test.expectedErrs) == 0 {
				t.Fatalf("Unexpected Error: %v", err)
			}

			if len(test.expectedErrs) != 0 {
				if err == nil {
					t.Fatalf("Expected %v, but got no error", test.expectedErrs)
				}

				if mErr, ok := err.(*multierror.Error); ok {
					if !reflect.DeepEqual(test.expectedErrs, mErr.Errors) {
						t.Fatalf("Expected %v, but got: %v", test.expectedErrs, mErr.Errors)
					}
				} else {
					t.Fatalf("Expected %v, but got: %v", test.expectedErrs, err)
				}
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name         string
		input        configpb.Configuration
		expectedErrs []error
	}{
		{
			name:         "Null input; returns error",
			expectedErrs: []error{MissingFieldError{"TestGroups"}},
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
			expectedErrs: []error{
				MissingFieldError{"TestGroups"},
			},
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
			expectedErrs: []error{
				MissingFieldError{"Dashboards"},
			},
		},
		{
			name: "Complete Minimal Config",
			input: configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dashboard_1",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name:          "tab_1",
								TestGroupName: "test_group_1",
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
		},
		{
			name: "Dashboards and Dashboard Groups cannot share names.",
			input: configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "name_1",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name:          "tab_1",
								TestGroupName: "test_group_1",
							},
						},
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
			expectedErrs: []error{
				DuplicateNameError{"name1", "Dashboard/DashboardGroup"},
			},
		},
		{
			name: "Dashboard Tabs must reference an existing Test Group",
			input: configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dashboard_1",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name:          "tab_1",
								TestGroupName: "test_group_1",
							},
							{
								Name:          "tab_2",
								TestGroupName: "test_group_2",
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
			expectedErrs: []error{
				MissingEntityError{"test_group_2", "TestGroup"},
			},
		},
		{
			name: "Test Groups must have an associated Dashboard Tab",
			input: configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name:         "dashboard_1",
						DashboardTab: []*configpb.DashboardTab{},
					},
				},
				TestGroups: []*configpb.TestGroup{
					{
						Name: "test_group_1",
					},
				},
			},
			expectedErrs: []error{
				ConfigError{"test_group_1", "TestGroup", "Each Test Group must be referenced by at least 1 Dashboard Tab."},
			},
		},
		{
			name: "Dashboard Groups must reference existing Dashboards",
			input: configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dashboard_1",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name:          "tab_1",
								TestGroupName: "test_group_1",
							},
						},
					},
				},
				TestGroups: []*configpb.TestGroup{
					{
						Name: "test_group_1",
					},
				},
				DashboardGroups: []*configpb.DashboardGroup{
					{
						Name:           "dashboard_group_1",
						DashboardNames: []string{"dashboard_1", "dashboard_2", "dashboard_3"},
					},
				},
			},
			expectedErrs: []error{
				MissingEntityError{"dashboard_2", "Dashboard"},
				MissingEntityError{"dashboard_3", "Dashboard"},
			},
		},
		{
			name: "A Dashboard can belong to at most 1 Dashboard Group",
			input: configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dashboard_1",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name:          "tab_1",
								TestGroupName: "test_group_1",
							},
						},
					},
				},
				TestGroups: []*configpb.TestGroup{
					{
						Name: "test_group_1",
					},
				},
				DashboardGroups: []*configpb.DashboardGroup{
					{
						Name:           "dashboard_group_1",
						DashboardNames: []string{"dashboard_1"},
					},
					{
						Name:           "dashboard_group_2",
						DashboardNames: []string{"dashboard_1"},
					},
				},
			},
			expectedErrs: []error{
				ConfigError{"dashboard_1", "Dashboard", "A Dashboard cannot be in more than 1 Dashboard Group."},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Validate(test.input)
			if err != nil && len(test.expectedErrs) == 0 {
				t.Fatalf("Unexpected Error: %v", err)
			}

			if len(test.expectedErrs) != 0 {
				if err == nil {
					t.Fatalf("Expected %v, but got no error", test.expectedErrs)
				}

				if mErr, ok := err.(*multierror.Error); ok {
					if !reflect.DeepEqual(test.expectedErrs, mErr.Errors) {
						t.Fatalf("Expected %v, but got: %v", test.expectedErrs, mErr.Errors)
					}
				} else {
					t.Fatalf("Expected %v, but got: %v", test.expectedErrs, err)
				}
			}
		})
	}
}
