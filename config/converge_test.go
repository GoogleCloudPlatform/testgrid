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

package config

import (
	"reflect"
	"testing"

	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/golang/protobuf/proto"
)

func TestConverge(t *testing.T) {
	cases := []struct {
		name     string
		inputs   map[string]*configpb.Configuration
		expected *configpb.Configuration
	}{
		{
			name: "nil input; throws error",
		},
		{
			name: "Merge with no conflicts; no name changes",
			inputs: map[string]*configpb.Configuration{
				"halloween": {
					TestGroups: []*configpb.TestGroup{
						aValidTestGroupNamed("pumpkin-decorator"),
						aValidTestGroupNamed("cobweb-decorator"),
						aValidTestGroupNamed("candy-corn-emitter"),
					},
					Dashboards: []*configpb.Dashboard{
						{
							Name: "Decorations",
							DashboardTab: []*configpb.DashboardTab{
								{
									Name:          "Jack-O-Lanterns",
									TestGroupName: "pumpkin-decorator",
								},
								{
									Name:          "Spooky Cave",
									TestGroupName: "cobweb-decorator",
								},
							},
						},
						{
							Name: "Treats",
							DashboardTab: []*configpb.DashboardTab{
								{
									Name:          "Candy Corn Only (sorry)",
									TestGroupName: "candy-corn-emitter",
								},
							},
						},
					},
					DashboardGroups: []*configpb.DashboardGroup{
						{
							Name:           "Halloween",
							DashboardNames: []string{"Treats", "Decorations"},
						},
					},
				},
				"thanksgiving": {
					TestGroups: []*configpb.TestGroup{
						aValidTestGroupNamed("meal-preparer"),
					},
					Dashboards: []*configpb.Dashboard{
						{
							Name: "Thanksgiving",
							DashboardTab: []*configpb.DashboardTab{
								{
									Name:          "Meal Prep",
									TestGroupName: "meal-preparer",
								},
							},
						},
					},
				},
			},
			expected: &configpb.Configuration{
				TestGroups: []*configpb.TestGroup{
					aValidTestGroupNamed("pumpkin-decorator"),
					aValidTestGroupNamed("cobweb-decorator"),
					aValidTestGroupNamed("candy-corn-emitter"),
					aValidTestGroupNamed("meal-preparer"),
				},
				Dashboards: []*configpb.Dashboard{
					{
						Name: "Decorations",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name:          "Jack-O-Lanterns",
								TestGroupName: "pumpkin-decorator",
							},
							{
								Name:          "Spooky Cave",
								TestGroupName: "cobweb-decorator",
							},
						},
					},
					{
						Name: "Treats",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name:          "Candy Corn Only (sorry)",
								TestGroupName: "candy-corn-emitter",
							},
						},
					},
					{
						Name: "Thanksgiving",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name:          "Meal Prep",
								TestGroupName: "meal-preparer",
							},
						},
					},
				},
				DashboardGroups: []*configpb.DashboardGroup{
					{
						Name:           "Halloween",
						DashboardNames: []string{"Treats", "Decorations"},
					},
				},
			},
		},
		{
			name: "Merge with conflicts; renames with key-prefix",
			inputs: map[string]*configpb.Configuration{
				"halloween": {
					TestGroups: []*configpb.TestGroup{
						aValidTestGroupNamed("pumpkin-decorator"),
						aValidTestGroupNamed("candy-emitter"),
					},
					Dashboards: []*configpb.Dashboard{
						{
							Name: "Decorations",
							DashboardTab: []*configpb.DashboardTab{
								{
									Name:          "Living Room",
									TestGroupName: "pumpkin-decorator",
								},
							},
						},
						{
							Name: "Treats",
							DashboardTab: []*configpb.DashboardTab{
								{
									Name:          "Candy",
									TestGroupName: "candy-emitter",
								},
							},
						},
					},
					DashboardGroups: []*configpb.DashboardGroup{
						{
							Name:           "To-Do",
							DashboardNames: []string{"Treats", "Decorations"},
						},
					},
				},
				"crimbo": {
					TestGroups: []*configpb.TestGroup{
						aValidTestGroupNamed("tree-decorator"),
						aValidTestGroupNamed("candy-emitter"),
					},
					Dashboards: []*configpb.Dashboard{
						{
							Name: "Decorations",
							DashboardTab: []*configpb.DashboardTab{
								{
									Name:          "Living Room",
									TestGroupName: "tree-decorator",
								},
							},
						},
						{
							Name: "Treats",
							DashboardTab: []*configpb.DashboardTab{
								{
									Name:          "Candy",
									TestGroupName: "candy-emitter",
								},
							},
						},
					},
					DashboardGroups: []*configpb.DashboardGroup{
						{
							Name:           "To-Do",
							DashboardNames: []string{"Treats", "Decorations"},
						},
					},
				},
			},
			expected: &configpb.Configuration{
				TestGroups: []*configpb.TestGroup{
					aValidTestGroupNamed("tree-decorator"),
					aValidTestGroupNamed("candy-emitter"),
					aValidTestGroupNamed("pumpkin-decorator"),
					aValidTestGroupNamed("halloween-candy-emitter"),
				},
				Dashboards: []*configpb.Dashboard{
					{
						Name: "Decorations",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name:          "Living Room",
								TestGroupName: "tree-decorator",
							},
						},
					},
					{
						Name: "Treats",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name:          "Candy",
								TestGroupName: "candy-emitter",
							},
						},
					},
					{
						Name: "halloween-Decorations",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name:          "Living Room",
								TestGroupName: "pumpkin-decorator",
							},
						},
					},
					{
						Name: "halloween-Treats",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name:          "Candy",
								TestGroupName: "halloween-candy-emitter",
							},
						},
					},
				},
				DashboardGroups: []*configpb.DashboardGroup{
					{
						Name:           "To-Do",
						DashboardNames: []string{"Treats", "Decorations"},
					},
					{
						Name:           "halloween-To-Do",
						DashboardNames: []string{"halloween-Treats", "halloween-Decorations"},
					},
				},
			},
		},
	}

	for _, testcase := range cases {
		t.Run(testcase.name, func(t *testing.T) {
			for iname, input := range testcase.inputs {
				err := Validate(input)
				if err != nil {
					t.Logf("Warning! Input %s doesn't validate; the result might not either.", iname)
					t.Logf("Validation error: %v", err)
				}
			}

			result, err := Converge(testcase.inputs)

			if testcase.expected != nil {
				if err := Validate(result); err != nil {
					t.Errorf("Result doesn't validate: %v", result)
					t.Errorf("Validation error: %v", err)
				}

				if !proto.Equal(testcase.expected, result) {
					t.Errorf("Expected %v, but got %v", testcase.expected, result)
				}
			} else {
				if err == nil {
					t.Errorf("Expected an error, but got none.")
				}
			}

		})
	}
}

func TestRenameTestGroup(t *testing.T) {
	cases := []struct {
		name     string
		old      string
		new      string
		input    *configpb.Configuration
		expected *configpb.Configuration
	}{
		{
			name: "Old string isn't in TestGroup; no change",
			old:  "foo",
			new:  "bar",
			input: &configpb.Configuration{
				TestGroups: []*configpb.TestGroup{
					{
						Name: "foo-group",
					},
				},
			},
			expected: &configpb.Configuration{
				TestGroups: []*configpb.TestGroup{
					{
						Name: "foo-group",
					},
				},
			},
		},
		{
			name: "Changes Dashboard Tab references",
			old:  "foo",
			new:  "bar",
			input: &configpb.Configuration{
				TestGroups: []*configpb.TestGroup{
					{
						Name: "foo",
					},
				},
				Dashboards: []*configpb.Dashboard{
					{
						Name: "foo",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name:          "foo",
								TestGroupName: "foo",
							},
						},
					},
				},
			},
			expected: &configpb.Configuration{
				TestGroups: []*configpb.TestGroup{
					{
						Name: "bar",
					},
				},
				Dashboards: []*configpb.Dashboard{
					{
						Name: "foo",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name:          "foo",
								TestGroupName: "bar",
							},
						},
					},
				},
			},
		},
	}

	for _, testcase := range cases {
		t.Run(testcase.name, func(t *testing.T) {
			result := RenameTestGroup(testcase.old, testcase.new, testcase.input)

			if !proto.Equal(testcase.expected, result) {
				t.Errorf("Expected %v, but got %v", testcase.expected, result)
			}
		})
	}
}

func TestRenameDashboard(t *testing.T) {
	cases := []struct {
		name     string
		old      string
		new      string
		input    *configpb.Configuration
		expected *configpb.Configuration
	}{
		{
			name: "Old string isn't in Dashboard; do nothing",
			old:  "foo",
			new:  "bar",
			input: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "foo-group",
					},
				},
			},
			expected: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "foo-group",
					},
				},
			},
		},
		{
			name: "Changes Dashboard Group reference",
			old:  "foo",
			new:  "bar",
			input: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "foo",
					},
				},
				DashboardGroups: []*configpb.DashboardGroup{
					{
						Name:           "foo",
						DashboardNames: []string{"foo"},
					},
				},
			},
			expected: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "bar",
					},
				},
				DashboardGroups: []*configpb.DashboardGroup{
					{
						Name:           "foo",
						DashboardNames: []string{"bar"},
					},
				},
			},
		},
	}

	for _, testcase := range cases {
		t.Run(testcase.name, func(t *testing.T) {
			result := RenameDashboard(testcase.old, testcase.new, testcase.input)

			if !proto.Equal(testcase.expected, result) {
				t.Errorf("Expected %v, but got %v", testcase.expected, result)
			}
		})
	}
}

func TestRenameDashboardGroup(t *testing.T) {
	cases := []struct {
		name     string
		old      string
		new      string
		input    *configpb.Configuration
		expected *configpb.Configuration
	}{
		{
			name: "Old string isn't in Dashboard Group; do nothing",
			old:  "foo",
			new:  "bar",
			input: &configpb.Configuration{
				DashboardGroups: []*configpb.DashboardGroup{
					{Name: "foo-foo"},
				},
			},
			expected: &configpb.Configuration{
				DashboardGroups: []*configpb.DashboardGroup{
					{Name: "foo-foo"},
				},
			},
		},
		{
			name: "Renames Dashboard Group",
			old:  "foo",
			new:  "bar",
			input: &configpb.Configuration{
				DashboardGroups: []*configpb.DashboardGroup{
					{Name: "foo"},
				},
			},
			expected: &configpb.Configuration{
				DashboardGroups: []*configpb.DashboardGroup{
					{Name: "bar"},
				},
			},
		},
	}

	for _, testcase := range cases {
		t.Run(testcase.name, func(t *testing.T) {
			result := RenameDashboardGroup(testcase.old, testcase.new, testcase.input)

			if !proto.Equal(testcase.expected, result) {
				t.Errorf("Expected %v, but got %v", testcase.expected, result)
			}
		})
	}
}

func TestNegotiateConversions(t *testing.T) {
	cases := []struct {
		Name     string
		original []string
		new      []string
		expected map[string]string
	}{
		{
			Name: "No sets; no conflicts",
		},
		{
			Name:     "Sets with no conflict; no conflicts",
			original: []string{"one", "two", "three"},
			new:      []string{"un", "deux", "trois"},
			expected: map[string]string{},
		},
		{
			Name:     "Sets with conflicts; conflicts returned",
			original: []string{"one", "two", "three"},
			new:      []string{"three", "one", "four"},
			expected: map[string]string{
				"one":   "prefix-one",
				"three": "prefix-three",
			},
		},
		{
			Name:     "Sets with conflicts with prefix; modified prefix returned",
			original: []string{"ichi", "ni", "prefix-ichi"},
			new:      []string{"ichi", "ni", "prefix-ni"},
			expected: map[string]string{
				"ichi": "prefix-2-ichi",
				"ni":   "prefix-2-ni",
			},
		},
	}

	for _, test := range cases {
		t.Run(test.Name, func(t *testing.T) {
			originalMap := make(map[string]void)
			for _, a := range test.original {
				originalMap[a] = insert
			}

			newMap := make(map[string]void)
			for _, a := range test.new {
				newMap[a] = insert
			}

			result := negotiateConversions("prefix", originalMap, newMap)

			if !reflect.DeepEqual(test.expected, result) {
				t.Errorf("Expected %v, but got %v", test.expected, result)
			}
		})
	}
}

func aValidTestGroupNamed(name string) *configpb.TestGroup {
	return &configpb.TestGroup{
		Name:             name,
		GcsPrefix:        "gcs://some/path",
		DaysOfResults:    1,
		NumColumnsRecent: 3,
	}
}
