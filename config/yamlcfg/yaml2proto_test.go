/*
Copyright 2016 The Kubernetes Authors.

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

package yamlcfg

import (
	"bytes"
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/google/go-cmp/cmp"
)

func TestYaml2Proto_IsExternal_And_UseKuberClient_False(t *testing.T) {
	yaml :=
		`default_test_group:
  name: default
default_dashboard_tab:
  name: default
test_groups:
- name: testgroup_1
dashboards:
- name: dash_1`

	defaults, err := LoadDefaults([]byte(yaml))
	if err != nil {
		t.Fatalf("Convert Error: %v\n Results: %v", err, defaults)
	}

	var cfg config.Configuration

	if err := Update(&cfg, []byte(yaml), &defaults); err != nil {
		t.Errorf("Convert Error: %v\n", err)
	}

	for _, testgroup := range cfg.TestGroups {
		if !testgroup.IsExternal {
			t.Errorf("IsExternal should always be true!")
		}

		if !testgroup.UseKubernetesClient {
			t.Errorf("UseKubernetesClient should always be true!")
		}
	}
}

func TestUpdateDefaults_Validity(t *testing.T) {
	tests := []struct {
		name            string
		yaml            string
		expectError     bool
		expectedMissing string
	}{
		{
			name:            "Empty file; returns error",
			yaml:            "",
			expectError:     true,
			expectedMissing: "DefaultTestGroup",
		},
		{
			name: "Only test group; returns error",
			yaml: `default_test_group:
  name: default`,
			expectError:     true,
			expectedMissing: "DefaultDashboardTab",
		},
		{
			name:            "Malformed YAML; returns error",
			yaml:            "{{{",
			expectError:     true,
			expectedMissing: "",
		},
		{
			name: "Set Default",
			yaml: `default_test_group:
  name: default
default_dashboard_tab:
  name: default`,
			expectedMissing: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadDefaults([]byte(test.yaml))

			if test.expectError {
				if err == nil {
					t.Errorf("Expected error, but got none")
				} else if test.expectedMissing != "" {
					e, isMissingFieldError := err.(MissingFieldError)
					if test.expectedMissing != "" && !isMissingFieldError {
						t.Errorf("Expected a MissingFieldError, but got %v", err)
					} else if e.Field != test.expectedMissing {
						t.Errorf("Unexpected Missing field; got %s, expected %s", e.Field, test.expectedMissing)
					}
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected Error: %v", err)
				}
			}
		})
	}
}

func TestUpdate_DefaultInherits(t *testing.T) {
	defaultYaml := `default_test_group:
  num_columns_recent: 10
  ignore_skip: true
  ignore_pending: true
default_dashboard_tab:
  num_columns_recent: 20`

	tests := []struct {
		name              string
		yaml              string
		expectedTestGroup int32
		expectedDashTab   int32
	}{
		{
			name: "Default Settings",
			yaml: `dashboards:
- name: dash_1
  dashboard_tab:
  - name: tab_1
test_groups:
- name: testgroup_1`,
			expectedTestGroup: 10,
			expectedDashTab:   20,
		},
		{
			name: "DashboardTab Inheritance",
			yaml: `dashboards:
- name: dash_1
  dashboard_tab:
  - name: tab_1
    num_columns_recent: 3
test_groups:
- name: testgroup_1`,
			expectedTestGroup: 10,
			expectedDashTab:   3,
		},
		{
			name: "TestGroup Inheritance",
			yaml: `dashboards:
- name: dash_1
  dashboard_tab:
  - name: tab_1
test_groups:
- name: testgroup_1
  num_columns_recent: 4`,
			expectedTestGroup: 4,
			expectedDashTab:   20,
		},
		{
			// TODO: Prevent this case.
			name: "Doesn't inherit imbedded defaults",
			yaml: `default_test_group:
  num_columns_recent: 5
default_dashboard_tab:
  num_columns_recent: 6
dashboards:
- name: dash_1
  dashboard_tab:
  - name: tab_1
test_groups:
- name: testgroup_1`,
			expectedTestGroup: 10,
			expectedDashTab:   20,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var cfg config.Configuration
			defaults, err := LoadDefaults([]byte(defaultYaml))
			if err != nil {
				t.Fatalf("Unexpected error with default yaml: %v", err)
			}

			if err := Update(&cfg, []byte(test.yaml), &defaults); err != nil {
				t.Fatalf("Unexpected error with Update: %v", err)
			}

			if cfg.TestGroups[0].NumColumnsRecent != test.expectedTestGroup {
				t.Errorf("Wrong inheritance for TestGroup: got %d, expected %d",
					cfg.TestGroups[0].NumColumnsRecent, test.expectedTestGroup)
			}

			if cfg.TestGroups[0].IgnorePending != true {
				t.Error("Wrong inheritance for TestGroup.IgnorePending: got false, expected true")
			}

			if cfg.TestGroups[0].IgnoreSkip != true {
				t.Error("Wrong inheritance for TestGroup.IgnoreSkip: got false, expected true")
			}

			if cfg.Dashboards[0].DashboardTab[0].NumColumnsRecent != test.expectedDashTab {
				t.Errorf("Wrong inheritance for Dashboard Tab: got %d, expected %d",
					cfg.Dashboards[0].DashboardTab[0].NumColumnsRecent, test.expectedDashTab)
			}

		})
	}
}

func Test_MarshalYAML(t *testing.T) {
	tests := []struct {
		name     string
		input    *config.Configuration
		expected []byte
	}{
		{
			name:  "Nil input; error",
			input: nil,
		},
		{
			name:  "Empty input; error",
			input: &config.Configuration{},
		},
		{
			name: "Dashboard Tab & Group",
			input: &config.Configuration{
				Dashboards: []*config.Dashboard{
					{
						Name: "dash_1",
						DashboardTab: []*config.DashboardTab{
							{
								Name:          "tab_1",
								TestGroupName: "testgroup_1",
							},
						},
					},
				},
				TestGroups: []*config.TestGroup{
					{
						Name: "testgroup_1",
					},
				},
			},
			expected: []byte(`dashboards:
- dashboard_tab:
  - name: tab_1
    test_group_name: testgroup_1
  name: dash_1
test_groups:
- days_of_results: 1
  gcs_prefix: fake path
  name: testgroup_1
  num_columns_recent: 1
`),
		},
		{
			name: "reject empty column headers",
			input: &config.Configuration{
				TestGroups: []*config.TestGroup{
					{
						Name: "test_group",
						ColumnHeader: []*config.TestGroup_ColumnHeader{
							{},
						},
					},
				},
				Dashboards: []*config.Dashboard{
					{
						Name: "dash",
						DashboardTab: []*config.DashboardTab{
							{
								Name:          "tab",
								TestGroupName: "test_group",
							},
						},
					},
				},
			},
		},
		{
			name: "reject multiple column header values",
			input: &config.Configuration{
				TestGroups: []*config.TestGroup{
					{
						Name: "test_group",
						ColumnHeader: []*config.TestGroup_ColumnHeader{
							{
								ConfigurationValue: "yay",
								Label:              "lab",
							},
						},
					},
				},
				Dashboards: []*config.Dashboard{
					{
						Name: "dash",
						DashboardTab: []*config.DashboardTab{
							{
								Name:          "tab",
								TestGroupName: "test_group",
							},
						},
					},
				},
			},
		},
		{
			name: "column headers configuration_value work",
			input: &config.Configuration{
				TestGroups: []*config.TestGroup{
					{
						Name: "test_group",
						ColumnHeader: []*config.TestGroup_ColumnHeader{
							{
								ConfigurationValue: "yay",
							},
							{
								Label: "lab",
							},
							{
								Property: "prop",
							},
						},
					},
				},
				Dashboards: []*config.Dashboard{
					{
						Name: "dash",
						DashboardTab: []*config.DashboardTab{
							{
								Name:          "tab",
								TestGroupName: "test_group",
							},
						},
					},
				},
			},
			expected: []byte(`dashboards:
- dashboard_tab:
  - name: tab
    test_group_name: test_group
  name: dash
test_groups:
- column_header:
  - configuration_value: yay
  - label: lab
  - property: prop
  days_of_results: 1
  gcs_prefix: fake path
  name: test_group
  num_columns_recent: 1
`),
		},
		{
			name: "name elements work correctly",
			input: &config.Configuration{
				TestGroups: []*config.TestGroup{
					{
						Name: "test_group",
						TestNameConfig: &config.TestNameConfig{
							NameFormat: "labels:%s target_config:%s build_target:%s tags:%s test_property:%s",
							NameElements: []*config.TestNameConfig_NameElement{
								{
									Labels: "labels",
								},
								{
									TargetConfig: "target config",
								},
								{
									BuildTarget: true,
								},
								{
									Tags: "tags",
								},
								{
									TestProperty: "test property",
								},
							},
						},
					},
				},
				Dashboards: []*config.Dashboard{
					{
						Name: "dash",
						DashboardTab: []*config.DashboardTab{
							{
								Name:          "tab",
								TestGroupName: "test_group",
							},
						},
					},
				},
			},
			expected: []byte(`dashboards:
- dashboard_tab:
  - name: tab
    test_group_name: test_group
  name: dash
test_groups:
- days_of_results: 1
  gcs_prefix: fake path
  name: test_group
  num_columns_recent: 1
  test_name_config:
    name_elements:
    - labels: labels
    - target_config: target config
    - build_target: true
    - tags: tags
    - test_property: test property
    name_format: labels:%s target_config:%s build_target:%s tags:%s test_property:%s
`),
		},
		{
			name: "reject unbalanced name format",
			input: &config.Configuration{
				TestGroups: []*config.TestGroup{
					{
						Name: "test_group",
						TestNameConfig: &config.TestNameConfig{
							NameFormat: "one:%s two:%s",
							NameElements: []*config.TestNameConfig_NameElement{
								{
									Labels: "labels",
								},
								{
									TargetConfig: "target config",
								},
								{
									BuildTarget: true,
								},
							},
						},
					},
				},
				Dashboards: []*config.Dashboard{
					{
						Name: "dash",
						DashboardTab: []*config.DashboardTab{
							{
								Name:          "tab",
								TestGroupName: "test_group",
							},
						},
					},
				},
			},
		},
		{
			name: "basic group values work",
			input: &config.Configuration{
				TestGroups: []*config.TestGroup{
					{
						Name:                    "test_group",
						AlertStaleResultsHours:  5,
						CodeSearchPath:          "github.com/kubernetes/example",
						IgnorePending:           true,
						IgnoreSkip:              true,
						IsExternal:              true,
						NumFailuresToAlert:      4,
						NumPassesToDisableAlert: 6,
						TestsNamePolicy:         2,
						UseKubernetesClient:     true,
					},
				},
				Dashboards: []*config.Dashboard{
					{
						Name: "dash",
						DashboardTab: []*config.DashboardTab{
							{
								Name:          "tab",
								TestGroupName: "test_group",
							},
						},
					},
				},
			},
			expected: []byte(`dashboards:
- dashboard_tab:
  - name: tab
    test_group_name: test_group
  name: dash
test_groups:
- alert_stale_results_hours: 5
  code_search_path: github.com/kubernetes/example
  days_of_results: 1
  gcs_prefix: fake path
  ignore_pending: true
  ignore_skip: true
  is_external: true
  name: test_group
  num_columns_recent: 1
  num_failures_to_alert: 4
  num_passes_to_disable_alert: 6
  tests_name_policy: 2
  use_kubernetes_client: true
`),
		},
		{
			name: "basic dashboard values work",
			input: &config.Configuration{
				TestGroups: []*config.TestGroup{
					{
						Name: "test_group",
					},
				},
				Dashboards: []*config.Dashboard{
					{
						Name: "dash",
						DashboardTab: []*config.DashboardTab{
							{
								Name:          "tab",
								TestGroupName: "test_group",
								AttachBugTemplate: &config.LinkTemplate{
									Url:     "yes",
									Options: []*config.LinkOptionsTemplate{},
								},
								CodeSearchPath: "find",
								CodeSearchUrlTemplate: &config.LinkTemplate{
									Url: "woo",
								},
								FileBugTemplate: &config.LinkTemplate{
									Url: "bar",
									Options: []*config.LinkOptionsTemplate{
										{
											Key:   "title",
											Value: "yay <test-name>",
										},
										{
											Key:   "body",
											Value: "woo <test-url>",
										},
									},
								},
								NumColumnsRecent: 10,
								OpenTestTemplate: &config.LinkTemplate{
									Url: "foo",
								},
								OpenBugTemplate: &config.LinkTemplate{
									Url: "ugh",
								},
								ResultsText: "wee",
								ResultsUrlTemplate: &config.LinkTemplate{
									Url: "soup",
								},
							},
						},
					},
				},
			},
			expected: []byte(`dashboards:
- dashboard_tab:
  - attach_bug_template:
      url: "yes"
    code_search_path: find
    code_search_url_template:
      url: woo
    file_bug_template:
      options:
      - key: title
        value: yay <test-name>
      - key: body
        value: woo <test-url>
      url: bar
    name: tab
    num_columns_recent: 10
    open_bug_template:
      url: ugh
    open_test_template:
      url: foo
    results_text: wee
    results_url_template:
      url: soup
    test_group_name: test_group
  name: dash
test_groups:
- days_of_results: 1
  gcs_prefix: fake path
  name: test_group
  num_columns_recent: 1
`),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Add required TestGroup fields, not validating them in these tests.
			if len(test.input.GetTestGroups()) != 0 {
				test.input.GetTestGroups()[0].DaysOfResults = 1
				test.input.GetTestGroups()[0].NumColumnsRecent = 1
				test.input.GetTestGroups()[0].GcsPrefix = "fake path"
			}
			result, err := MarshalYAML(test.input)
			if test.expected == nil && err == nil {
				t.Errorf("Expected error, but got none")
			}
			if !bytes.Equal(result, test.expected) {
				t.Errorf("Expected: %s\n, Got: %s\nError: %v\n", string(test.expected), string(result), err)
			}
		})
	}
}

func Test_ReadConfig(t *testing.T) {
	tests := []struct {
		name          string
		files         map[string]string
		useDir        bool
		expected      config.Configuration
		expectFailure bool
	}{
		{
			name: "Reads file",
			files: map[string]string{
				"1*.yaml": "dashboards:\n- name: Foo\n",
			},
			expected: config.Configuration{
				Dashboards: []*config.Dashboard{
					{Name: "Foo"},
				},
			},
		},
		{
			name: "Reads files in directory",
			files: map[string]string{
				"1*.yaml": "dashboards:\n- name: Foo\n",
				"2*.yaml": "dashboards:\n- name: Bar\n",
			},
			useDir: true,
			expected: config.Configuration{
				Dashboards: []*config.Dashboard{
					{Name: "Foo"},
					{Name: "Bar"},
				},
			},
		},
		{
			name: "Invalid YAML: fails",
			files: map[string]string{
				"1*.yaml": "gibberish",
			},
			expectFailure: true,
		},
		{
			name: "Won't read non-YAML",
			files: map[string]string{
				"1*.yml": "dashboards:\n- name: Foo\n",
				"2*.txt": "dashboards:\n- name: Bar\n",
			},
			expected: config.Configuration{
				Dashboards: []*config.Dashboard{
					{Name: "Foo"},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inputs := make([]string, 0)
			directory, err := ioutil.TempDir("", "")
			if err != nil {
				t.Fatalf("Error in creating temporary dir: %v", err)
			}
			defer os.RemoveAll(directory)

			for fileName, fileContents := range test.files {
				file, err := ioutil.TempFile(directory, fileName)
				if err != nil {
					t.Fatalf("Error in creating temporary file %s: %v", fileName, err)
				}
				if _, err := file.WriteString(fileContents); err != nil {
					t.Fatalf("Error in writing temporary file %s: %v", fileName, err)
				}
				inputs = append(inputs, file.Name())
				if err := file.Close(); err != nil {
					t.Fatalf("Error in closing temporary file %s: %v", fileName, err)
				}
			}

			var result config.Configuration
			var readErr error
			if test.useDir {
				result, readErr = ReadConfig([]string{directory}, "")
			} else {
				result, readErr = ReadConfig(inputs, "")
			}

			if test.expectFailure && readErr == nil {
				t.Error("Expected error, but got none")
			}
			if !test.expectFailure && readErr != nil {
				t.Errorf("Unexpected error: %v", readErr)
			}
			if !test.expectFailure && !reflect.DeepEqual(result, test.expected) {
				t.Errorf("Mismatched results: got %v, expected %v", result, test.expected)
			}
		})
	}
}

func Test_getDefaults(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		want  []string
		err   bool
	}{
		{
			name: "empty paths",
		},
		{
			name:  "simple case",
			paths: []string{"foo/config.yaml", "foo/default.yaml"},
			want:  []string{"foo/default.yaml"},
			err:   false,
		},
		{
			name:  "no defaults",
			paths: []string{"foo/config.yaml"},
			err:   false,
		},
		{
			name:  "two defaults",
			paths: []string{"foo/config.yaml", "foo/default.yml", "foo/default.yaml"},
			err:   true,
		},
		{
			name:  "multiple defaults",
			paths: []string{"foo/default.yaml", "bar/default.yaml"},
			want:  []string{"foo/default.yaml", "bar/default.yaml"},
			err:   false,
		},
		{
			name:  "subdirs",
			paths: []string{"foo/bar/default.yaml"},
			want:  []string{"foo/bar/default.yaml"},
			err:   false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := getDefaults(test.paths)
			if test.err && err == nil {
				t.Fatalf("expected error, but no error was received")
			}
			if err != nil && !test.err {
				t.Fatalf("expected no error, but received error %v", err)
			}
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Fatalf("returned with incorrect value, %v", diff)
			}
		})
	}
}

func Test_PathDefault(t *testing.T) {
	overallDefaults := DefaultConfiguration{
		DefaultTestGroup: &config.TestGroup{
			NumColumnsRecent: 5,
		},
	}
	localDefaults := DefaultConfiguration{
		DefaultTestGroup: &config.TestGroup{
			NumColumnsRecent: 10,
		},
	}

	tests := []struct {
		name         string
		path         string
		defaultFiles map[string]DefaultConfiguration
		defaults     DefaultConfiguration
		want         DefaultConfiguration
	}{
		{
			name: "empty works",
		},
		{
			name: "basically works",
			path: "foo/config.yaml",
			defaultFiles: map[string]DefaultConfiguration{
				"foo": localDefaults,
			},
			defaults: overallDefaults,
			want:     localDefaults,
		},
		{
			name: "path not in map uses overall defaults",
			path: "config.yaml",
			defaultFiles: map[string]DefaultConfiguration{
				"foo": localDefaults,
			},
			defaults: overallDefaults,
			want:     overallDefaults,
		},
		{
			name: "path in subdirectory of local default uses overall defaults",
			path: "foo/bar/config.yaml",
			defaultFiles: map[string]DefaultConfiguration{
				"foo": localDefaults,
			},
			defaults: overallDefaults,
			want:     overallDefaults,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := pathDefault(test.path, test.defaultFiles, test.defaults); test.want != got {
				t.Fatalf("pathDefault(%s) incorrect; got %v, want %v", test.path, got, test.want)
			}
		})
	}
}
