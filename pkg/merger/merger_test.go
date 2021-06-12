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

package merger

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/google/go-cmp/cmp"
	"io"
	"io/ioutil"
	"testing"

	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"

	"cloud.google.com/go/storage"
	"github.com/golang/protobuf/proto"
)

func newPathOrDie(s string) *gcs.Path {
	p, err := gcs.NewPath(s)
	if err != nil {
		panic(err)
	}
	return p
}

func Test_ParseAndCheck(t *testing.T) {
	tc := []struct {
		name         string
		input        []byte
		expectedList MergeList
		expectError  bool
	}{
		{
			name:        "Empty MergeList will return an error",
			expectError: true,
		},
		{
			name: "Parses YAML examples",
			input: []byte(`target: "gs://path/to/write/config"
sources:
- name: "red"
  location: "gs://example/red-team/config"
  contact: "red-admin@example.com"
- name: "blue"
  location: "gs://example/blue-team/config"
  contact: "blue.team.contact@example.com"`),
			expectedList: MergeList{
				Target: "gs://path/to/write/config",
				Path:   newPathOrDie("gs://path/to/write/config"),
				Sources: []Source{
					{
						Name:     "red",
						Location: "gs://example/red-team/config",
						Path:     newPathOrDie("gs://example/red-team/config"),
						Contact:  "red-admin@example.com",
					},
					{
						Name:     "blue",
						Location: "gs://example/blue-team/config",
						Path:     newPathOrDie("gs://example/blue-team/config"),
						Contact:  "blue.team.contact@example.com",
					},
				},
			},
		},
		{
			name: "Tolerate missing contacts",
			input: []byte(`target: "gs://path/to/write/config"
sources:
- name: "red"
  location: "gs://example/red-team/config"
- name: "blue"
  location: "gs://example/blue-team/config"`),
			expectedList: MergeList{
				Target: "gs://path/to/write/config",
				Path:   newPathOrDie("gs://path/to/write/config"),
				Sources: []Source{
					{
						Name:     "red",
						Location: "gs://example/red-team/config",
						Path:     newPathOrDie("gs://example/red-team/config"),
					},
					{
						Name:     "blue",
						Location: "gs://example/blue-team/config",
						Path:     newPathOrDie("gs://example/blue-team/config"),
					},
				},
			},
		},
		{
			name: "Target is local filesystem path",
			input: []byte(`target: "/tmp/config"
sources:
- name: "red"
  location: "gs://example/red-team/config"
  contact: "red-admin@example.com"
- name: "blue"
  location: "gs://example/blue-team/config"
  contact: "blue.team.contact@example.com"`),
			expectedList: MergeList{
				Target: "/tmp/config",
				Path:   newPathOrDie("/tmp/config"),
				Sources: []Source{
					{
						Name:     "red",
						Location: "gs://example/red-team/config",
						Path:     newPathOrDie("gs://example/red-team/config"),
						Contact:  "red-admin@example.com",
					},
					{
						Name:     "blue",
						Location: "gs://example/blue-team/config",
						Path:     newPathOrDie("gs://example/blue-team/config"),
						Contact:  "blue.team.contact@example.com",
					},
				},
			},
		},
		{
			name: "Target is file:// path",
			input: []byte(`target: "file://tmp/config"
sources:
- name: "red"
  location: "gs://example/red-team/config"
  contact: "red-admin@example.com"
- name: "blue"
  location: "gs://example/blue-team/config"
  contact: "blue.team.contact@example.com"`),
			expectedList: MergeList{
				Target: "file://tmp/config",
				Path:   newPathOrDie("file://tmp/config"),
				Sources: []Source{
					{
						Name:     "red",
						Location: "gs://example/red-team/config",
						Path:     newPathOrDie("gs://example/red-team/config"),
						Contact:  "red-admin@example.com",
					},
					{
						Name:     "blue",
						Location: "gs://example/blue-team/config",
						Path:     newPathOrDie("gs://example/blue-team/config"),
						Contact:  "blue.team.contact@example.com",
					},
				},
			},
		},
		{
			name: "Target is invalid path",
			input: []byte(`target: "foo://config"
sources:
- name: "red"
  location: "gs://example/red-team/config"
  contact: "red-admin@example.com"
- name: "blue"
  location: "gs://example/blue-team/config"
  contact: "blue.team.contact@example.com"`),
			expectError: true,
		},
		{
			name: "Source contains a local filesystem path",
			input: []byte(`target: "gs://path/to/write/config"
sources:
- name: "red"
  location: "/tmp/config"
  contact: "red-admin@example.com"
- name: "blue"
  location: "file://example/blue-team/config"
  contact: "blue.team.contact@example.com"`),
			expectedList: MergeList{
				Target: "gs://path/to/write/config",
				Path:   newPathOrDie("gs://path/to/write/config"),
				Sources: []Source{
					{
						Name:     "red",
						Location: "/tmp/config",
						Path:     newPathOrDie("/tmp/config"),
						Contact:  "red-admin@example.com",
					},
					{
						Name:     "blue",
						Location: "file://example/blue-team/config",
						Path:     newPathOrDie("file://example/blue-team/config"),
						Contact:  "blue.team.contact@example.com",
					},
				},
			},
		},
		{
			name: "Source contains an invalid path, returns error",
			input: []byte(`target: "gs://path/to/write/config"
sources:
- name: "red"
  location: "foo://config"
  contact: "red-admin@example.com"
- name: "blue"
  location: "gs://example/blue-team/config"
  contact: "blue.team.contact@example.com"`),
			expectError: true,
		},
		{
			name: "Contains a duplicated name, returns error",
			input: []byte(`target: "gs://path/to/write/config"
sources:
- name: "red"
  location: "gs://example/red-team/config"
- name: "red"
  location: "gs://example/new-red-team/config"`),
			expectError: true,
		},
	}

	for _, test := range tc {
		t.Run(test.name, func(t *testing.T) {
			resultList, err := ParseAndCheck(test.input)
			if test.expectError {
				if err == nil {
					t.Fatal("Expected error, but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("Unexpected error %v", err)
			}
			if diff := cmp.Diff(test.expectedList, resultList, cmp.AllowUnexported(gcs.Path{})); diff != "" {
				t.Errorf("ParseAndCheck(%q) differed (-got, +want): %s", test.input, diff)
			}
		})
	}
}

func Test_MergeAndUpdate(t *testing.T) {
	cases := []struct {
		name                string
		paths               map[string]*gcs.Path
		uploadInjectedError error
		skipValidate        bool
		confirm             bool
		expectError         bool
		expectUpload        bool
	}{
		{
			name:        "No paths to read from; fails",
			confirm:     true,
			expectError: true,
		},
		{
			name: "Intended upload; succeeds",
			paths: map[string]*gcs.Path{
				"first": newPathOrDie("gs://valid/config"),
			},
			confirm:      true,
			expectUpload: true,
		},
		{
			name: "Given nil path; fails",
			paths: map[string]*gcs.Path{
				"first":  newPathOrDie("gs://valid/config"),
				"second": nil,
			},
			confirm:     true,
			expectError: true,
		},
		{
			name: "Open fails; fails",
			paths: map[string]*gcs.Path{
				"first":  newPathOrDie("gs://valid/config"),
				"second": newPathOrDie("gs://read/error"),
			},
			confirm:     true,
			expectError: true,
		},
		{
			name: "Validate fails; skips and succeeds",
			paths: map[string]*gcs.Path{
				"first":  newPathOrDie("gs://valid/config"),
				"second": newPathOrDie("gs://invalid/config"),
			},
			confirm:      true,
			expectUpload: true,
		},
		{
			name: "Validate fails for all targets; fails",
			paths: map[string]*gcs.Path{
				"first": newPathOrDie("gs://invalid/config"),
			},
			confirm:     true,
			expectError: true,
		},
		{
			name: "Upload fails; fails",
			paths: map[string]*gcs.Path{
				"first": newPathOrDie("gs://valid/config"),
			},
			uploadInjectedError: errors.New("upload error"),
			confirm:             true,
			expectError:         true,
		},
		{
			name: "no-confirm; succeeds with no upload",
			paths: map[string]*gcs.Path{
				"first": newPathOrDie("gs://valid/config"),
			},
		},
		{
			name: "skip-validate with invalid proto; succeeds",
			paths: map[string]*gcs.Path{
				"second": newPathOrDie("gs://invalid/config"),
			},
			skipValidate: true,
			confirm:      true,
			expectUpload: true,
		},
	}

	for _, tc := range cases {
		existingData := fakeOpener{
			"gs://valid/config": configInFake(&configpb.Configuration{
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
						GcsPrefix:        "tests_live_here",
						DaysOfResults:    1,
						NumColumnsRecent: 1,
					},
				},
			}),
			"gs://invalid/config": configInFake(&configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{Name: "dash_1"},
					{Name: "dash_1"},
				},
			}),
			"gs://read/error": fakeObject{
				err: errors.New("read error"),
			},
		}

		t.Run(tc.name, func(t *testing.T) {
			client := fakeMergeClient{
				fakeUploader: fakeUploader{
					err: tc.uploadInjectedError,
				},
				fakeOpener: existingData,
			}

			mergeList := MergeList{
				Target:  "gs://result/config",
				Path:    newPathOrDie("gs://result/config"),
				Sources: nil,
			}

			for name, path := range tc.paths {
				mergeList.Sources = append(mergeList.Sources, Source{
					Name: name,
					Path: path,
				})
			}

			resultErr := MergeAndUpdate(context.Background(), &client, mergeList, tc.skipValidate, tc.confirm)

			if tc.expectUpload && !client.uploaded {
				t.Errorf("Expected upload, but there was none")
			}

			if !tc.expectUpload && client.uploaded {
				t.Errorf("Unexpected upload")
			}

			if tc.expectError && resultErr == nil {
				t.Errorf("Expected error, but got none")
			}

			if !tc.expectError && resultErr != nil {
				t.Errorf("Unexpected error %v", resultErr)
			}
		})
	}
}

type fakeMergeClient struct {
	fakeOpener
	fakeUploader
}

type fakeOpener map[string]fakeObject

func (fo fakeOpener) Open(_ context.Context, path gcs.Path) (io.ReadCloser, error) {
	o, ok := fo[path.String()]
	if !ok {
		return nil, fmt.Errorf("wrap not exist: %w", storage.ErrObjectNotExist)
	}
	if o.err != nil {
		return nil, fmt.Errorf("injected open error: %w", o.err)
	}
	return ioutil.NopCloser(bytes.NewReader(o.buf)), nil
}

type fakeObject struct {
	buf []byte
	err error
}

func configInFake(cfg *configpb.Configuration) (fo fakeObject) {
	b, err := proto.Marshal(cfg)
	if err != nil {
		panic(err)
	}
	fo.buf = b
	return
}

type fakeUploader struct {
	uploaded bool
	err      error
}

func (fu *fakeUploader) Upload(context.Context, gcs.Path, []byte, bool, string) (*storage.ObjectAttrs, error) {
	if fu.err != nil {
		return nil, fmt.Errorf("injected upload error: %w", fu.err)
	}
	fu.uploaded = true
	return nil, nil
}
