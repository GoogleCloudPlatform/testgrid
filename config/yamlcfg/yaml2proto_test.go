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
- name: dashboard_1`

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
- name: dashboard_1
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
- name: dashboard_1
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
- name: dashboard_1
  dashboard_tab:
  - name: tab_1
test_groups:
- name: testgroup_1
  num_columns_recent: 4`,
			expectedTestGroup: 4,
			expectedDashTab:   20,
		},
		{
			name: "Doesn't inherit imbedded defaults",
			yaml: `default_test_group:
  num_columns_recent: 5
default_dashboard_tab:
  num_columns_recent: 6
dashboards:
- name: dashboard_1
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
		input    config.Configuration
		expected []byte
	}{
		{
			name:  "Empty input; error",
			input: config.Configuration{},
		},
		{
			name: "Dashboard Tab & Group",
			input: config.Configuration{
				Dashboards: []*config.Dashboard{
					{
						Name: "dashboard_1",
						DashboardTab: []*config.DashboardTab{
							{
								Name:          "tab_1",
								TestGroupName: "testgroup_1",
							},
						},
					},
				},
				TestGroups: []*config.TestGroup{
					{Name: "testgroup_1"},
				},
			},
			expected: []byte(`dashboards:
- dashboard_tab:
  - name: tab_1
    test_group_name: testgroup_1
  name: dashboard_1
test_groups:
- name: testgroup_1
`),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := MarshalYAML(test.input)
			if test.expected == nil && err == nil {
				t.Errorf("Expected error, but got none")
			}
			if !bytes.Equal(result, test.expected) {
				t.Errorf("Expected: %v\n, Got: %v\nError: %v\n", test.expected, result, err)
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
