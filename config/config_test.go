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
	"errors"
	"reflect"
	"strings"
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
		c    *configpb.Configuration
		pass bool
	}{
		{
			name: "reject nil config.Configuration",
			c:    nil,
			pass: false,
		},
		{
			name: "everything works",
			c: &configpb.Configuration{
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
						Name: "dash_group_1",
					},
				},
			},
			pass: true,
		},
		{
			name: "reject empty group names",
			c: &configpb.Configuration{
				TestGroups: []*configpb.TestGroup{
					{},
				},
			},
		},
		{
			name: "reject empty dashboard names",
			c: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{},
				},
			},
		},
		{
			name: "reject empty tab names",
			c: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dash_1",
						DashboardTab: []*configpb.DashboardTab{
							{},
						},
					},
				},
			},
		},
		{
			name: "reject empty dashboard group names",
			c: &configpb.Configuration{
				DashboardGroups: []*configpb.DashboardGroup{
					{},
				},
			},
		},
		{
			name: "dashboard group names cannot match a dashboard name",
			c: &configpb.Configuration{
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

func TestValidateReferencesExist(t *testing.T) {
	tests := []struct {
		name         string
		input        *configpb.Configuration
		expectedErrs []error
	}{
		{
			name:         "reject nil config.Configuration",
			input:        nil,
			expectedErrs: []error{errors.New("got an empty config.Configuration")},
		},
		{
			name: "Dashboard Tabs must reference an existing Test Group",
			input: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dash_1",
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
			input: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name:         "dash_1",
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
			input: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dash_1",
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
						Name:           "dash_group_1",
						DashboardNames: []string{"dash_1", "dash_2", "dash_3"},
					},
				},
			},
			expectedErrs: []error{
				MissingEntityError{"dash_2", "Dashboard"},
				MissingEntityError{"dash_3", "Dashboard"},
			},
		},
		{
			name: "A Dashboard can belong to at most 1 Dashboard Group",
			input: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dash_1",
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
						Name:           "dash_group_1",
						DashboardNames: []string{"dash_1"},
					},
					{
						Name:           "dash_group_2",
						DashboardNames: []string{"dash_1"},
					},
				},
			},
			expectedErrs: []error{
				ConfigError{"dash_1", "Dashboard", "A Dashboard cannot be in more than 1 Dashboard Group."},
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

func TestValidateName(t *testing.T) {
	stringOfLength := func(length int) string {
		var sb strings.Builder
		for i := 0; i < length; i++ {
			sb.WriteRune('a')
		}
		return sb.String()
	}

	tests := []struct {
		name  string
		input string
		pass  bool
	}{
		{
			name:  "Names can't be empty",
			input: "",
		},
		{
			name:  "Invalid characters are filtered out",
			input: "___%%%***!!!???'''|||@@@###$$$^^^///\\\\\\",
		},
		{
			name:  "Names can't be too short",
			input: "q",
		},
		{
			name:  "Names must contain 3+ alphanumeric characters",
			input: "?rs=%%",
		},
		{
			name:  "Names can't be too long",
			input: stringOfLength(2049),
		},
		{
			name:  "Names can't start with dashboard",
			input: "dashboard",
		},
		{
			name:  "Names can't start with summary",
			input: "_summary_",
		},
		{
			name:  "Names can't start with alerter",
			input: "ALERTER",
		},
		{
			name:  "Names can't start with bugs",
			input: "bugs-1-2-3",
		},
		{
			name:  "Names may contain forbidden prefixes in the middle",
			input: "file-bugs-for-alerter",
			pass:  true,
		},
		{
			name:  "Valid name",
			input: "some-test-group",
			pass:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateName(test.input)
			pass := err == nil
			if pass != test.pass {
				t.Fatalf("name %s got pass = %v, want pass = %v", test.input, pass, test.pass)
			}
		})
	}
}

func TestValidateTestGroup(t *testing.T) {
	tests := []struct {
		name      string
		testGroup *configpb.TestGroup
		pass      bool
	}{
		{
			name:      "Nil TestGroup fails",
			pass:      false,
			testGroup: nil,
		},
		{
			name: "Minimal config passes",
			pass: true,
			testGroup: &configpb.TestGroup{
				Name:             "test_group",
				DaysOfResults:    1,
				GcsPrefix:        "fake path",
				NumColumnsRecent: 1,
			},
		},
		{
			name: "Must have days_of_results",
			testGroup: &configpb.TestGroup{
				Name:             "test_group",
				GcsPrefix:        "fake path",
				NumColumnsRecent: 1,
			},
		},
		{
			name: "days_of_results must be positive",
			testGroup: &configpb.TestGroup{
				Name:             "test_group",
				DaysOfResults:    -1,
				GcsPrefix:        "fake path",
				NumColumnsRecent: 1,
			},
		},
		{
			name: "Must have gcs_prefix",
			testGroup: &configpb.TestGroup{
				Name:             "test_group",
				DaysOfResults:    1,
				NumColumnsRecent: 1,
			},
		},
		{
			name: "Must have num_columns_recent",
			testGroup: &configpb.TestGroup{
				Name:          "test_group",
				DaysOfResults: 1,
				GcsPrefix:     "fake path",
			},
		},
		{
			name: "num_columns_recent must be positive",
			testGroup: &configpb.TestGroup{
				Name:             "test_group",
				DaysOfResults:    1,
				GcsPrefix:        "fake path",
				NumColumnsRecent: -1,
			},
		},
		{
			name: "test_method_match_regex must compile",
			testGroup: &configpb.TestGroup{
				Name:                 "test_group",
				DaysOfResults:        1,
				GcsPrefix:            "fake path",
				NumColumnsRecent:     1,
				TestMethodMatchRegex: "[.*",
			},
		},
		{
			name: "Notifications must have a summary",
			testGroup: &configpb.TestGroup{
				Name:             "test_group",
				DaysOfResults:    1,
				GcsPrefix:        "fake path",
				NumColumnsRecent: 1,
				Notifications: []*configpb.Notification{
					{},
				},
			},
		},
		{
			name: "Test Annotations must have property_name",
			testGroup: &configpb.TestGroup{
				Name:             "test_group",
				DaysOfResults:    1,
				GcsPrefix:        "fake path",
				NumColumnsRecent: 1,
				TestAnnotations: []*configpb.TestGroup_TestAnnotation{
					{
						ShortText: "a",
					},
				},
			},
		},
		{
			name: "Test Annotation short_text has to be at least 1 character",
			testGroup: &configpb.TestGroup{
				Name:             "test_group",
				DaysOfResults:    1,
				GcsPrefix:        "fake path",
				NumColumnsRecent: 1,
				TestAnnotations: []*configpb.TestGroup_TestAnnotation{
					{
						ShortTextMessageSource: &configpb.TestGroup_TestAnnotation_PropertyName{
							PropertyName: "something",
						},
						ShortText: "",
					},
				},
			},
		},
		{
			name: "Test Annotation short_text has to be at most 5 characters",
			testGroup: &configpb.TestGroup{
				Name:             "test_group",
				DaysOfResults:    1,
				GcsPrefix:        "fake path",
				NumColumnsRecent: 1,
				TestAnnotations: []*configpb.TestGroup_TestAnnotation{
					{
						ShortTextMessageSource: &configpb.TestGroup_TestAnnotation_PropertyName{
							PropertyName: "something",
						},
						ShortText: "abcdef",
					},
				},
			},
		},
		{
			name: "fallback_grouping_configuration_value requires fallback_group = configuration_value",
			testGroup: &configpb.TestGroup{
				Name:                               "test_group",
				DaysOfResults:                      1,
				GcsPrefix:                          "fake path",
				NumColumnsRecent:                   1,
				FallbackGroupingConfigurationValue: "something",
			},
		},
		{
			name: "fallback_grouping = configuration_value requires fallback_grouping_configuration_value",
			testGroup: &configpb.TestGroup{
				Name:             "test_group",
				DaysOfResults:    1,
				GcsPrefix:        "fake path",
				NumColumnsRecent: 1,
				FallbackGrouping: configpb.TestGroup_FALLBACK_GROUPING_CONFIGURATION_VALUE,
			},
		},
		{
			name: "Complex config passes",
			pass: true,
			testGroup: &configpb.TestGroup{
				// Basic config
				Name:             "test_group",
				DaysOfResults:    1,
				GcsPrefix:        "fake path",
				NumColumnsRecent: 1,
				// Regexes compile
				TestMethodMatchRegex: "test.*",
				// Simple notification
				Notifications: []*configpb.Notification{
					{
						Summary: "I'm a notification!",
					},
				},
				// Fallback grouping based on a configuration value
				FallbackGrouping:                   configpb.TestGroup_FALLBACK_GROUPING_CONFIGURATION_VALUE,
				FallbackGroupingConfigurationValue: "something",
				// Simple test annotation based on a property
				TestAnnotations: []*configpb.TestGroup_TestAnnotation{
					{
						ShortTextMessageSource: &configpb.TestGroup_TestAnnotation_PropertyName{
							PropertyName: "something",
						},
						ShortText: "abc",
					},
				},
			},
		},
		{
			name: "accept filled column headers",
			pass: true,
			testGroup: &configpb.TestGroup{
				Name:             "test_group",
				DaysOfResults:    1,
				GcsPrefix:        "fake path",
				NumColumnsRecent: 1,
				ColumnHeader: []*configpb.TestGroup_ColumnHeader{
					{
						Label: "lab",
					},
					{
						Property: "prop",
					},
					{
						ConfigurationValue: "yay",
					},
				},
			},
		},
		{
			name: "reject column headers with label and configuration_value",
			testGroup: &configpb.TestGroup{
				Name:             "test_group",
				DaysOfResults:    1,
				GcsPrefix:        "fake path",
				NumColumnsRecent: 1,
				ColumnHeader: []*configpb.TestGroup_ColumnHeader{
					{
						Label:              "labtoo",
						ConfigurationValue: "foo",
					},
				},
			},
		},
		{
			name: "reject column headers with configuration_value and property",
			testGroup: &configpb.TestGroup{
				Name:             "test_group",
				DaysOfResults:    1,
				GcsPrefix:        "fake path",
				NumColumnsRecent: 1,
				ColumnHeader: []*configpb.TestGroup_ColumnHeader{
					{
						ConfigurationValue: "bar",
						Property:           "proptoo",
					},
				},
			},
		},
		{
			name: "reject column headers with label and property",
			testGroup: &configpb.TestGroup{
				Name:             "test_group",
				DaysOfResults:    1,
				GcsPrefix:        "fake path",
				NumColumnsRecent: 1,
				ColumnHeader: []*configpb.TestGroup_ColumnHeader{
					{
						Label:    "labtoo",
						Property: "proptoo",
					},
				},
			},
		},
		{
			name: "reject empty column headers",
			testGroup: &configpb.TestGroup{
				Name:             "test_group",
				DaysOfResults:    1,
				GcsPrefix:        "fake path",
				NumColumnsRecent: 1,
				ColumnHeader: []*configpb.TestGroup_ColumnHeader{
					{},
				},
			},
		},
		{
			name: "reject unformatted name format",
			testGroup: &configpb.TestGroup{
				Name:             "simple",
				DaysOfResults:    1,
				GcsPrefix:        "fake path",
				NumColumnsRecent: 1,
				TestNameConfig: &configpb.TestNameConfig{
					NameFormat: "hello world",
				},
			},
		},
		{
			name: "accept complex and balanced name formats",
			pass: true,
			testGroup: &configpb.TestGroup{
				Name:             "complex",
				DaysOfResults:    1,
				GcsPrefix:        "fake path",
				NumColumnsRecent: 1,
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
		{
			name: "reject unbalanced name formats",
			testGroup: &configpb.TestGroup{
				Name:             "bad",
				DaysOfResults:    1,
				GcsPrefix:        "fake path",
				NumColumnsRecent: 1,
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
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTestGroup(test.testGroup)
			pass := err == nil
			if test.pass != pass {
				t.Fatalf("test group config got pass = %v, want pass = %v: %v", pass, test.pass, err)
			}
		})
	}
}

func TestInvalidEmails(t *testing.T) {
	tests := []struct {
		name      string
		addresses string
		pass      bool
	}{
		{
			name:      "Addresses can't be blank",
			addresses: "",
		},
		{
			name:      "Comma-separated addresses can't be blank",
			addresses: ",",
		},
		{
			name:      "Comma-separated addresses still can't be blank",
			addresses: ",thing@email.com",
		},
		{
			name:      "no username",
			addresses: "@email.com",
		},
		{
			name:      "no domain name",
			addresses: "username",
		},
		{
			name:      "@ but no domain name",
			addresses: "username@",
		},
		{
			name:      "too many @'s",
			addresses: "hey@hello@greetings.com",
		},
		{
			name:      "Valid Address",
			addresses: "hey@greetings.com",
			pass:      true,
		},
		{
			name:      "Multiple Valid Addresses",
			addresses: "hey@greetings.com,something@mail.com",
			pass:      true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateEmails(test.addresses)
			pass := err == nil
			if test.pass != pass {
				t.Fatalf("addresses (%s) got pass = %v, want pass = %v: %v", test.addresses, pass, test.pass, err)
			}
		})
	}
}

func TestValidateDashboardTab(t *testing.T) {
	tests := []struct {
		name string
		tab  *configpb.DashboardTab
		pass bool
	}{
		{
			name: "nil DashboardTab fails",
			tab:  nil,
			pass: false,
		},
		{
			name: "Dashboard Tabs must specify a Test Group Name",
			tab: &configpb.DashboardTab{
				Name: "tabby",
			},
		},
		{
			name: "Dashboard Tabs must specify a Test Group Name",
			tab: &configpb.DashboardTab{
				Name:          "Summary",
				TestGroupName: "test_group_1",
			},
		},
		{
			name: "Tabular Names Regex must compile",
			tab: &configpb.DashboardTab{
				Name:              "tabby",
				TestGroupName:     "test_group_1",
				TabularNamesRegex: "([1!]",
			},
		},
		{
			name: "Tabular Names Regex must have at least one capture group",
			tab: &configpb.DashboardTab{
				Name:              "tabby",
				TestGroupName:     "test_group_1",
				TabularNamesRegex: ".*",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateDashboardTab(test.tab)
			pass := err == nil
			if pass != test.pass {
				t.Fatalf("Invalid dashboard tab config: %v", err)
			}
		})
	}
}

func TestUpdate_Validate(t *testing.T) {
	tests := []struct {
		name         string
		input        *configpb.Configuration
		expectedErrs []error
	}{
		{
			name:         "Nil input; returns error",
			input:        nil,
			expectedErrs: []error{errors.New("got an empty config.Configuration")},
		},
		{
			name: "Dashboard Only; returns error",
			input: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dash_1",
					},
				},
			},
			expectedErrs: []error{
				MissingFieldError{"TestGroups"},
			},
		},
		{
			name: "Test Group Only; returns error",
			input: &configpb.Configuration{
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
			input: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dash_1",
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
						Name:             "test_group_1",
						GcsPrefix:        "fake GcsPrefix",
						DaysOfResults:    1,
						NumColumnsRecent: 1,
					},
				},
			},
		},
		{
			name: "Empty Dashboard; returns error",
			input: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dash_1",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name:          "tab_1",
								TestGroupName: "test_group_1",
							},
						},
					},
					{
						Name: "dash_2",
					},
				},
				TestGroups: []*configpb.TestGroup{
					{
						Name:             "test_group_1",
						GcsPrefix:        "fake GcsPrefix",
						DaysOfResults:    1,
						NumColumnsRecent: 1,
					},
				},
			},
			expectedErrs: []error{
				ConfigError{"dash_2", "Dashboard", "contains no tabs"},
			},
		},
		{
			name: "Dashboards and Dashboard Groups cannot share names.",
			input: &configpb.Configuration{
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
						Name:             "test_group_1",
						GcsPrefix:        "fake GcsPrefix",
						DaysOfResults:    1,
						NumColumnsRecent: 1,
					},
				},
			},
			expectedErrs: []error{
				DuplicateNameError{"name1", "Dashboard/DashboardGroup"},
			},
		},
		{
			name: "Dashboard Tabs must reference an existing Test Group",
			input: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dash_1",
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
						Name:             "test_group_1",
						GcsPrefix:        "fake GcsPrefix",
						DaysOfResults:    1,
						NumColumnsRecent: 1,
					},
				},
			},
			expectedErrs: []error{
				MissingEntityError{"test_group_2", "TestGroup"},
			},
		},
		{
			name: "Test Groups must have an associated Dashboard Tab",
			input: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dash_1",
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
						Name:             "test_group_1",
						GcsPrefix:        "fake GcsPrefix",
						DaysOfResults:    1,
						NumColumnsRecent: 1,
					},
					{
						Name:             "test_group_2",
						GcsPrefix:        "fake GcsPrefix",
						DaysOfResults:    1,
						NumColumnsRecent: 1,
					},
				},
			},
			expectedErrs: []error{
				ConfigError{"test_group_2", "TestGroup", "Each Test Group must be referenced by at least 1 Dashboard Tab."},
			},
		},
		{
			name: "Dashboard Groups must reference existing Dashboards",
			input: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dash_1",
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
						Name:             "test_group_1",
						GcsPrefix:        "fake GcsPrefix",
						DaysOfResults:    1,
						NumColumnsRecent: 1,
					},
				},
				DashboardGroups: []*configpb.DashboardGroup{
					{
						Name:           "dash_group_1",
						DashboardNames: []string{"dash_1", "dash_2", "dash_3"},
					},
				},
			},
			expectedErrs: []error{
				MissingEntityError{"dash_2", "Dashboard"},
				MissingEntityError{"dash_3", "Dashboard"},
			},
		},
		{
			name: "A Dashboard can belong to at most 1 Dashboard Group",
			input: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dash_1",
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
						Name:             "test_group_1",
						GcsPrefix:        "fake GcsPrefix",
						DaysOfResults:    1,
						NumColumnsRecent: 1,
					},
				},
				DashboardGroups: []*configpb.DashboardGroup{
					{
						Name:           "dash_group_1",
						DashboardNames: []string{"dash_1"},
					},
					{
						Name:           "dash_group_2",
						DashboardNames: []string{"dash_1"},
					},
				},
			},
			expectedErrs: []error{
				ConfigError{"dash_1", "Dashboard", "A Dashboard cannot be in more than 1 Dashboard Group."},
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
