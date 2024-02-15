/*
Copyright 2023 The TestGrid Authors.

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

package util

import (
	"testing"

	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestTokens(t *testing.T) {
	cases := []struct {
		name     string
		template *configpb.LinkTemplate
		want     []string
	}{
		{
			name:     "nil",
			template: nil,
			want:     nil,
		},
		{
			name:     "empty",
			template: &configpb.LinkTemplate{},
			want:     nil,
		},
		{
			name: "basically works",
			template: &configpb.LinkTemplate{
				Url: "https://test.com/<workflow-name>/<workflow-id>/<test-id>/<test-name>",
				Options: []*configpb.LinkOptionsTemplate{
					{
						Key:   "prefix",
						Value: "<gcs-prefix>",
					},
					{
						Key:   "build",
						Value: "<build-id>",
					},
					{
						Key:   "prop",
						Value: "<my-prop>",
					},
					{
						Key:   "foo",
						Value: "<custom-0>",
					},
				},
			},
			want: []string{WorkflowName, WorkflowID, TestID, TestName, GcsPrefix, BuildID, "<my-prop>", "<custom-0>"},
		},
		{
			name: "duplicate tokens",
			template: &configpb.LinkTemplate{
				Url: "https://test.com/<workflow-name>/<workflow-id>/<test-id>/<test-id>",
				Options: []*configpb.LinkOptionsTemplate{
					{
						Key:   "workflow",
						Value: "<workflow-name>",
					},
				},
			},
			want: []string{WorkflowName, WorkflowID, TestID},
		},
		{
			name: "encode",
			template: &configpb.LinkTemplate{
				Url: "https://test.com/<encode:<workflow-name>>",
				Options: []*configpb.LinkOptionsTemplate{
					{
						Key:   "workflow-id",
						Value: "<encode:<workflow-id>>",
					},
				},
			},
			want: []string{WorkflowName, WorkflowID},
		},
		{
			name: "invalid",
			template: &configpb.LinkTemplate{
				Url: "http://test.com/<oh-no",
			},
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Tokens(tc.template)
			if diff := cmp.Diff(tc.want, got, cmpopts.SortSlices(func(a, b string) bool { return a < b })); diff != "" {
				t.Errorf("Tokens(%v) differed (-want, +got): %s", tc.template, diff)
			}
		})
	}
}

func TestExpandTemplate(t *testing.T) {
	cases := []struct {
		name       string
		template   *configpb.LinkTemplate
		parameters map[string]string
		err        bool
		want       string
	}{
		{
			name:       "nil",
			template:   nil,
			parameters: nil,
			want:       "",
		},
		{
			name: "nil parameters",
			template: &configpb.LinkTemplate{
				Url: "https://test.com/<workflow-name>/<workflow-id>/<test-id>/<test-name>",
				Options: []*configpb.LinkOptionsTemplate{
					{
						Key:   "prefix",
						Value: "<gcs-prefix>",
					},
					{
						Key:   "build",
						Value: "<build-id>",
					},
					{
						Key:   "prop",
						Value: "<my-prop>",
					},
					{
						Key:   "foo",
						Value: "<custom-0>",
					},
				},
			},
			parameters: nil,
			want:       "https://test.com/%3Cworkflow-name%3E/%3Cworkflow-id%3E/%3Ctest-id%3E/%3Ctest-name%3E?build=%3Cbuild-id%3E&foo=%3Ccustom-0%3E&prefix=%3Cgcs-prefix%3E&prop=%3Cmy-prop%3E",
		},
		{
			name:       "empty",
			template:   &configpb.LinkTemplate{},
			parameters: map[string]string{},
			want:       "",
		},
		{
			name: "basically works",
			template: &configpb.LinkTemplate{
				Url: "https://test.com/<workflow-name>/<workflow-id>/<test-id>/<test-name>",
				Options: []*configpb.LinkOptionsTemplate{
					{
						Key:   "prefix",
						Value: "<gcs-prefix>",
					},
					{
						Key:   "build",
						Value: "<build-id>",
					},
					{
						Key:   "prop",
						Value: "<my-prop>",
					},
					{
						Key:   "foo",
						Value: "<custom-0>",
					},
				},
			},
			parameters: map[string]string{
				WorkflowName: "//my-workflow",
				WorkflowID:   "workflow-id-1",
				TestID:       "test-id-1",
				TestName:     "//path/to:my-test",
				GcsPrefix:    "my-bucket/has/results",
				BuildID:      "build-1",
				"<my-prop>":  "foo",
				"<custom-0>": "bar",
			},
			want: "https://test.com///my-workflow/workflow-id-1/test-id-1///path/to:my-test?build=build-1&foo=bar&prefix=my-bucket%2Fhas%2Fresults&prop=foo",
		},
		{
			name: "invalid url",
			template: &configpb.LinkTemplate{
				Url: "https://test.com%+",
				Options: []*configpb.LinkOptionsTemplate{
					{
						Key:   "prop",
						Value: "%+<my-prop>",
					},
				},
			},
			parameters: map[string]string{
				"<my-prop>": "foo",
			},
			err: true,
		},
		{
			name: "duplicate tokens",
			template: &configpb.LinkTemplate{
				Url: "https://test.com/<test-id>/<test-id>",
				Options: []*configpb.LinkOptionsTemplate{
					{
						Key:   "workflow",
						Value: "<workflow-name>",
					},
					{
						Key:   "workflow-name",
						Value: "<workflow-name>",
					},
				},
			},
			parameters: map[string]string{
				WorkflowName: "//my-workflow",
				WorkflowID:   "workflow-id-1",
				TestID:       "test-id-1",
				TestName:     "//path/to:my-test",
				GcsPrefix:    "my-bucket/has/results",
				BuildID:      "build-1",
				"<my-prop>":  "foo",
			},
			want: "https://test.com/test-id-1/test-id-1?workflow=%2F%2Fmy-workflow&workflow-name=%2F%2Fmy-workflow",
		},
		{
			name: "encode",
			template: &configpb.LinkTemplate{
				Url: "https://test.com/<encode:<workflow-name>>",
				Options: []*configpb.LinkOptionsTemplate{
					{
						Key:   "workflow",
						Value: "<encode:<workflow-name>>",
					},
				},
			},
			parameters: map[string]string{
				WorkflowName: "//my-workfl@w",
			},
			want: "https://test.com/%2F%2Fmy-workfl@w?workflow=%2F%2Fmy-workfl%40w",
		},
		{
			name: "invalid",
			template: &configpb.LinkTemplate{
				Url: "http://test.com/<workflow-name",
			},
			parameters: map[string]string{
				WorkflowName: "//my-workflow",
			},
			want: "http://test.com/%3Cworkflow-name",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExpandTemplate(tc.template, tc.parameters)
			if tc.err && err == nil {
				t.Errorf("ExpandTemplate() did not error as expected.")
			} else if !tc.err && err != nil {
				t.Errorf("ExpandTemplate() errored unexpectedly: %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("ExpandTemplate() differed (-want, +got): %s", diff)
			}
		})
	}
}

func TestExpandTemplateString(t *testing.T) {
	cases := []struct {
		name        string
		templateStr string
		parameters  map[string]string
		isQuery     bool
		want        string
	}{
		{
			name:        "empty",
			templateStr: "",
			parameters:  nil,
			want:        "",
		},
		{
			name:        "nil parameters",
			templateStr: "https://test.com/<workflow-name>/<workflow-id>/<test-id>/<test-name>",
			parameters:  nil,
			want:        "https://test.com/<workflow-name>/<workflow-id>/<test-id>/<test-name>",
		},
		{
			name:        "basically works",
			templateStr: "https://test.com/<workflow-name>/<workflow-id>/<test-id>/<test-name>/<my-prop>/<custom-0>",
			parameters: map[string]string{
				WorkflowName: "//my-workflow",
				WorkflowID:   "workflow-id-1",
				TestID:       "test-id-1",
				TestName:     "//path/to:my-test",
				"<my-prop>":  "foo",
				"<custom-0>": "magic",
			},
			want: "https://test.com///my-workflow/workflow-id-1/test-id-1///path/to:my-test/foo/magic",
		},
		{
			name:        "duplicate tokens",
			templateStr: "https://test.com/<test-id>/<test-id>",
			parameters: map[string]string{
				TestID: "test-id-1",
			},
			want: "https://test.com/test-id-1/test-id-1",
		},
		{
			name:        "encode",
			templateStr: "https://test.com/<encode:<workflow-name>>",
			parameters: map[string]string{
				WorkflowName: "//my-workfl@w",
			},
			want: "https://test.com/%2F%2Fmy-workfl@w",
		},
		{
			name:        "encode query",
			templateStr: "<encode:<workflow-name>>",
			parameters: map[string]string{
				WorkflowName: "//my-workfl@w",
			},
			isQuery: true,
			want:    "//my-workfl@w",
		},
		{
			name:        "invalid",
			templateStr: "http://test.com/<workflow-name",
			parameters: map[string]string{
				WorkflowName: "//my-workflow",
			},
			want: "http://test.com/<workflow-name",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := expandTemplateString(tc.templateStr, tc.parameters, tc.isQuery)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("expandTemplateString() differed (-want, +got): %s", diff)
			}
		})
	}
}
