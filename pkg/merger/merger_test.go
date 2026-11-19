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
	"io"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/GoogleCloudPlatform/testgrid/config"
	"github.com/GoogleCloudPlatform/testgrid/config/snapshot"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs/fake"
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

			_, resultErr := MergeAndUpdate(context.Background(), &client, nil, mergeList, tc.skipValidate, tc.confirm)

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
func Test_RecordLastModified(t *testing.T) {
	cases := []struct {
		name          string
		attrs         *storage.ReaderObjectAttrs
		mets          *Metrics
		source        string
		expectAtLeast int64
	}{
		{
			name:   "nil attrs; succeeds",
			attrs:  nil,
			mets:   &Metrics{},
			source: "fake-source",
		},
		{
			name:   "nil mets; succeeds",
			attrs:  &storage.ReaderObjectAttrs{},
			mets:   nil,
			source: "fake-source",
		},
		{
			name:   "nil; succeeds",
			attrs:  nil,
			mets:   nil,
			source: "",
		}, {
			name: "non-nil mets and attrs; succeeds",
			attrs: &storage.ReaderObjectAttrs{
				LastModified: time.Now().Add(-5 * time.Second),
			},
			mets:          &Metrics{},
			source:        "",
			expectAtLeast: 5,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.attrs == nil || tc.mets == nil {
				recordLastModified(tc.attrs, tc.mets, tc.source)
			} else {
				var fakeMetric fakeInt64
				tc.mets.LastModified = &fakeMetric
				recordLastModified(tc.attrs, tc.mets, tc.source)
				if fakeMetric.last < tc.expectAtLeast {
					t.Errorf("want at least %d, got %d", tc.expectAtLeast, fakeMetric.last)
				}
			}
		})
	}
}

type fakeInt64 struct {
	mu   sync.Mutex
	set  bool
	last int64
}

func (f *fakeInt64) Name() string {
	return "fakeInt64"
}

func (f *fakeInt64) Set(n int64, _ ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.last = n
	f.set = true
}

type unusedConditionalClient struct {
	gcs.ConditionalClient
}

type fakeMergeClient struct {
	unusedConditionalClient
	fakeOpener
	fakeUploader
}

func Test_MergeUpdateListen_OnInit(t *testing.T) {
	redCfg := validConfig("red_dash", "red_tab", "red_tg", "red_dg")
	blueCfg := validConfig("blue_dash", "blue_tab", "blue_tg", "blue_dg")
	conflictBlueCfg := validConfig("shared_dash", "tab_1", "shared_tg", "shared_dg")
	conflictRedCfg := validConfig("shared_dash", "tab_1", "shared_tg", "shared_dg")
	invalidCfg := &configpb.Configuration{
		Dashboards: []*configpb.Dashboard{
			{Name: "dash_1"},
		},
	}

	targetPath := newPathOrDie("gs://result/config")
	redPath := newPathOrDie("gs://red/config")
	bluePath := newPathOrDie("gs://blue/config")

	cases := []struct {
		name                string
		list                MergeList
		setupClient         func(*fake.ConditionalClient)
		uploadInjectedError error
		skipValidate        bool
		confirm             bool
		expectError         bool
		expectUpload        bool
		wantSnap            *snapshot.Config
		wantUploaded        *configpb.Configuration
	}{
		{
			name: "No paths to read from; fails",
			list: MergeList{
				Target: targetPath.String(),
				Path:   targetPath,
			},
			confirm:     true,
			expectError: true,
		},
		{
			name: "Given nil path; fails",
			list: MergeList{
				Target: targetPath.String(),
				Path:   targetPath,
				Sources: []Source{
					{Name: "red", Path: redPath},
					{Name: "blue", Path: nil},
				},
			},
			setupClient: func(c *fake.ConditionalClient) {
				setFakeObject(c, *redPath, redCfg, 1, 0, nil)
			},
			confirm:     true,
			expectError: true,
		},
		{
			name: "Confirm with nil target path; fails",
			list: MergeList{
				Sources: []Source{
					{Name: "red", Path: redPath},
				},
			},
			setupClient: func(c *fake.ConditionalClient) {
				setFakeObject(c, *redPath, redCfg, 1, 0, nil)
			},
			confirm:     true,
			expectError: true,
		},
		{
			name: "Open fails; fails",
			list: MergeList{
				Target: targetPath.String(),
				Path:   targetPath,
				Sources: []Source{
					{Name: "red", Path: redPath},
					{Name: "blue", Path: bluePath},
				},
			},
			setupClient: func(c *fake.ConditionalClient) {
				setFakeObject(c, *redPath, redCfg, 1, 0, nil)
				setFakeObject(c, *bluePath, nil, 1, 0, errors.New("read error"))
			},
			confirm:     true,
			expectError: true,
		},
		{
			name: "Config exceeds max size; fails",
			list: MergeList{
				Target: targetPath.String(),
				Path:   targetPath,
				Sources: []Source{
					{Name: "red", Path: redPath},
				},
			},
			setupClient: func(c *fake.ConditionalClient) {
				setFakeObject(c, *redPath, redCfg, 1, int64(config.MaxConfigSize)+1, nil)
			},
			confirm:     true,
			expectError: true,
		},
		{
			name: "Multiple configs fail concurrently; fails without blocking",
			list: MergeList{
				Target: targetPath.String(),
				Path:   targetPath,
				Sources: []Source{
					{Name: "red", Path: redPath},
					{Name: "blue", Path: bluePath},
				},
			},
			setupClient: func(c *fake.ConditionalClient) {
				setFakeObject(c, *redPath, redCfg, 1, int64(config.MaxConfigSize)+1, nil)
				setFakeObject(c, *bluePath, blueCfg, 1, int64(config.MaxConfigSize)+1, nil)
			},
			confirm:     true,
			expectError: true,
		},
		{
			name: "Upload fails; fails",
			list: MergeList{
				Target: targetPath.String(),
				Path:   targetPath,
				Sources: []Source{
					{Name: "red", Path: redPath},
				},
			},
			setupClient: func(c *fake.ConditionalClient) {
				setFakeObject(c, *redPath, redCfg, 1, 0, nil)
			},
			uploadInjectedError: errors.New("upload error"),
			confirm:             true,
			expectError:         true,
		},
		{
			name: "Intended upload for single valid config; succeeds",
			list: MergeList{
				Target: targetPath.String(),
				Path:   targetPath,
				Sources: []Source{
					{Name: "red", Path: redPath},
				},
			},
			setupClient: func(c *fake.ConditionalClient) {
				setFakeObject(c, *redPath, redCfg, 1, 0, nil)
			},
			confirm:      true,
			expectUpload: true,
			wantSnap:     protoToSnapshot(redCfg),
			wantUploaded: redCfg,
		},
		{
			name: "no-confirm; succeeds with no upload",
			list: MergeList{
				Target: targetPath.String(),
				Path:   targetPath,
				Sources: []Source{
					{Name: "red", Path: redPath},
				},
			},
			setupClient: func(c *fake.ConditionalClient) {
				setFakeObject(c, *redPath, redCfg, 1, 0, nil)
			},
			confirm:      false,
			expectUpload: false,
			wantSnap:     protoToSnapshot(redCfg),
		},
		{
			name: "Merges multiple valid configs without conflicts",
			list: MergeList{
				Target: targetPath.String(),
				Path:   targetPath,
				Sources: []Source{
					{Name: "red", Path: redPath},
					{Name: "blue", Path: bluePath},
				},
			},
			setupClient: func(c *fake.ConditionalClient) {
				setFakeObject(c, *redPath, redCfg, 1, 0, nil)
				setFakeObject(c, *bluePath, blueCfg, 1, 0, nil)
			},
			confirm:      true,
			expectUpload: true,
			wantSnap: &snapshot.Config{
				DashboardGroups: map[string]*configpb.DashboardGroup{
					"blue_dg": {Name: "blue_dg", DashboardNames: []string{"blue_dash"}},
					"red_dg":  {Name: "red_dg", DashboardNames: []string{"red_dash"}},
				},
				Dashboards: map[string]*configpb.Dashboard{
					"blue_dash": {
						Name: "blue_dash",
						DashboardTab: []*configpb.DashboardTab{
							{Name: "blue_tab", TestGroupName: "blue_tg"},
						},
					},
					"red_dash": {
						Name: "red_dash",
						DashboardTab: []*configpb.DashboardTab{
							{Name: "red_tab", TestGroupName: "red_tg"},
						},
					},
				},
				Groups: map[string]*configpb.TestGroup{
					"blue_tg": validTestGroup("blue_tg"),
					"red_tg":  validTestGroup("red_tg"),
				},
			},
			wantUploaded: &configpb.Configuration{
				DashboardGroups: []*configpb.DashboardGroup{
					{Name: "blue_dg", DashboardNames: []string{"blue_dash"}},
					{Name: "red_dg", DashboardNames: []string{"red_dash"}},
				},
				Dashboards: []*configpb.Dashboard{
					{
						Name: "blue_dash",
						DashboardTab: []*configpb.DashboardTab{
							{Name: "blue_tab", TestGroupName: "blue_tg"},
						},
					},
					{
						Name: "red_dash",
						DashboardTab: []*configpb.DashboardTab{
							{Name: "red_tab", TestGroupName: "red_tg"},
						},
					},
				},
				TestGroups: []*configpb.TestGroup{
					validTestGroup("blue_tg"),
					validTestGroup("red_tg"),
				},
			},
		},
		{
			name: "Merges multiple valid configs with conflicts; prefixes names",
			list: MergeList{
				Target: targetPath.String(),
				Path:   targetPath,
				Sources: []Source{
					{Name: "red", Path: redPath},
					{Name: "blue", Path: bluePath},
				},
			},
			setupClient: func(c *fake.ConditionalClient) {
				setFakeObject(c, *bluePath, conflictBlueCfg, 1, 0, nil)
				setFakeObject(c, *redPath, conflictRedCfg, 1, 0, nil)
			},
			confirm:      true,
			expectUpload: true,
			wantSnap: &snapshot.Config{
				DashboardGroups: map[string]*configpb.DashboardGroup{
					"shared_dg":     {Name: "shared_dg", DashboardNames: []string{"shared_dash"}},
					"red-shared_dg": {Name: "red-shared_dg", DashboardNames: []string{"red-shared_dash"}},
				},
				Dashboards: map[string]*configpb.Dashboard{
					"shared_dash": {
						Name: "shared_dash",
						DashboardTab: []*configpb.DashboardTab{
							{Name: "tab_1", TestGroupName: "shared_tg"},
						},
					},
					"red-shared_dash": {
						Name: "red-shared_dash",
						DashboardTab: []*configpb.DashboardTab{
							{Name: "tab_1", TestGroupName: "red-shared_tg"},
						},
					},
				},
				Groups: map[string]*configpb.TestGroup{
					"shared_tg":     validTestGroup("shared_tg"),
					"red-shared_tg": validTestGroup("red-shared_tg"),
				},
			},
		},
		{
			name: "Validate fails for one source; skips invalid and succeeds",
			list: MergeList{
				Target: targetPath.String(),
				Path:   targetPath,
				Sources: []Source{
					{Name: "red", Path: redPath},
					{Name: "blue", Path: bluePath},
				},
			},
			setupClient: func(c *fake.ConditionalClient) {
				setFakeObject(c, *redPath, redCfg, 1, 0, nil)
				setFakeObject(c, *bluePath, invalidCfg, 1, 0, nil)
			},
			confirm:      true,
			expectUpload: true,
			wantSnap:     protoToSnapshot(redCfg),
			wantUploaded: redCfg,
		},
		{
			name: "Validate fails for all targets; fails",
			list: MergeList{
				Target: targetPath.String(),
				Path:   targetPath,
				Sources: []Source{
					{Name: "blue", Path: bluePath},
				},
			},
			setupClient: func(c *fake.ConditionalClient) {
				setFakeObject(c, *bluePath, invalidCfg, 1, 0, nil)
			},
			confirm:     true,
			expectError: true,
		},
		{
			name: "skip-validate with invalid proto; succeeds",
			list: MergeList{
				Target: targetPath.String(),
				Path:   targetPath,
				Sources: []Source{
					{Name: "blue", Path: bluePath},
				},
			},
			setupClient: func(c *fake.ConditionalClient) {
				setFakeObject(c, *bluePath, invalidCfg, 1, 0, nil)
			},
			skipValidate: true,
			confirm:      true,
			expectUpload: true,
			wantSnap:     protoToSnapshot(invalidCfg),
			wantUploaded: invalidCfg,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Short timeout keeps retry-backoff failure cases fast
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()

			client := newFakeConditionalClient()
			if tc.setupClient != nil {
				tc.setupClient(client)
			}
			if tc.uploadInjectedError != nil {
				client.Uploader[*targetPath] = fake.Upload{Err: tc.uploadInjectedError}
			}

			snaps, err := MergeUpdateListen(ctx, client, nil, tc.list, tc.skipValidate, tc.confirm)
			if tc.expectError {
				if err == nil {
					t.Fatalf("MergeUpdateListen() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("MergeUpdateListen() unexpected error: %v", err)
			}

			client.Lock.RLock()
			uploaded, hasUpload := client.Uploader[*targetPath]
			client.Lock.RUnlock()

			if tc.expectUpload && !hasUpload {
				t.Errorf("Expected upload to %s, but there was none", targetPath)
			}
			if !tc.expectUpload && hasUpload {
				t.Errorf("Unexpected upload to %s", targetPath)
			}
			if tc.wantUploaded != nil && hasUpload {
				var gotCfg configpb.Configuration
				if err := proto.Unmarshal(uploaded.Buf, &gotCfg); err != nil {
					t.Fatalf("proto.Unmarshal(uploaded) error: %v", err)
				}
				if diff := cmp.Diff(tc.wantUploaded, &gotCfg, protocmp.Transform()); diff != "" {
					t.Errorf("Uploaded configuration diff (-want +got):\n%s", diff)
				}
			}

			select {
			case gotSnap, ok := <-snaps:
				if !ok {
					t.Fatalf("snaps channel closed before initial snapshot")
				}
				if diff := cmp.Diff(tc.wantSnap, gotSnap, protocmp.Transform()); diff != "" {
					t.Errorf("Initial snapshot diff (-want +got):\n%s", diff)
				}
			case <-time.After(time.Second):
				t.Fatalf("Timed out waiting for initial snapshot")
			}

			// Cancelling context should close the output channel cleanly.
			cancel()
			select {
			case _, ok := <-snaps:
				if ok {
					t.Errorf("Expected channel to close after cancel, got unexpected snapshot")
				}
			case <-time.After(time.Second):
				t.Errorf("Timed out waiting for channel to close after cancel")
			}
		})
	}
}

func Test_MergeUpdateListen_OnChange(t *testing.T) {
	targetPath := newPathOrDie("gs://result/config")
	redPath := newPathOrDie("gs://red/config")
	bluePath := newPathOrDie("gs://blue/config")

	t.Run("Updates merged config when sources change", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client := newFakeConditionalClient()
		redV1 := validConfig("red_dash_1", "red_tab", "red_tg_1", "red_dg")
		blueV1 := validConfig("blue_dash_1", "blue_tab", "blue_tg_1", "blue_dg")
		setFakeObject(client, *redPath, redV1, 1, 0, nil)
		setFakeObject(client, *bluePath, blueV1, 1, 0, nil)

		var fieldsMetric, lastModMetric fakeInt64
		mets := &Metrics{
			Fields:       &fieldsMetric,
			LastModified: &lastModMetric,
		}

		list := MergeList{
			Target: targetPath.String(),
			Path:   targetPath,
			Sources: []Source{
				{Name: "red", Path: redPath},
				{Name: "blue", Path: bluePath},
			},
		}

		ticker := make(chan time.Time)
		snaps, err := mergeUpdateListen(ctx, client, mets, list, false, true, ticker)
		if err != nil {
			t.Fatalf("mergeUpdateListen() unexpected error: %v", err)
		}

		initial := mustReadChannel(t, snaps)
		if _, ok := initial.Dashboards["red_dash_1"]; !ok {
			t.Errorf("initial snapshot missing red_dash_1: %v", initial.Dashboards)
		}
		if _, ok := initial.Dashboards["blue_dash_1"]; !ok {
			t.Errorf("initial snapshot missing blue_dash_1: %v", initial.Dashboards)
		}

		// Update blue to generation 2.
		blueV2 := validConfig("blue_dash_2", "blue_tab", "blue_tg_2", "blue_dg")
		setFakeObject(client, *bluePath, blueV2, 2, 0, nil)
		ticker <- time.Now()

		updated1 := mustReadChannel(t, snaps)
		if _, ok := updated1.Dashboards["red_dash_1"]; !ok {
			t.Errorf("updated1 missing red_dash_1: %v", updated1.Dashboards)
		}
		if _, ok := updated1.Dashboards["blue_dash_2"]; !ok {
			t.Errorf("updated1 missing blue_dash_2: %v", updated1.Dashboards)
		}
		if _, ok := updated1.Dashboards["blue_dash_1"]; ok {
			t.Errorf("updated1 still contains old blue_dash_1")
		}

		// Update red to generation 2.
		redV2 := validConfig("red_dash_2", "red_tab", "red_tg_2", "red_dg")
		setFakeObject(client, *redPath, redV2, 2, 0, nil)
		ticker <- time.Now()

		updated2 := mustReadChannel(t, snaps)
		if _, ok := updated2.Dashboards["red_dash_2"]; !ok {
			t.Errorf("updated2 missing red_dash_2: %v", updated2.Dashboards)
		}
		if _, ok := updated2.Dashboards["blue_dash_2"]; !ok {
			t.Errorf("updated2 missing blue_dash_2: %v", updated2.Dashboards)
		}

		// Verify GCS target has generation 3 (initial + 2 updates) and matches updated2.
		client.Lock.RLock()
		uploaded := client.Uploader[*targetPath]
		client.Lock.RUnlock()
		if uploaded.Generation != 3 {
			t.Errorf("uploaded Generation = %d, want 3", uploaded.Generation)
		}
		var gotUploadedCfg configpb.Configuration
		if err := proto.Unmarshal(uploaded.Buf, &gotUploadedCfg); err != nil {
			t.Fatalf("proto.Unmarshal(uploaded.Buf) error: %v", err)
		}
		if diff := cmp.Diff(updated2, protoToSnapshot(&gotUploadedCfg), protocmp.Transform()); diff != "" {
			t.Errorf("uploaded snapshot diff (-want +got):\n%s", diff)
		}

		fieldsMetric.mu.Lock()
		gotFieldsSet := fieldsMetric.set
		fieldsMetric.mu.Unlock()
		if !gotFieldsSet {
			t.Errorf("expected Fields metric to be set")
		}

		lastModMetric.mu.Lock()
		gotLastModSet := lastModMetric.set
		lastModMetric.mu.Unlock()
		if !gotLastModSet {
			t.Errorf("expected LastModified metric to be set")
		}
	})

	t.Run("Resolves conflicts dynamically without mutating cached shards", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client := newFakeConditionalClient()
		setFakeObject(client, *bluePath, validConfig("shared_dash", "tab_1", "shared_tg", "shared_dg"), 1, 0, nil)
		setFakeObject(client, *redPath, validConfig("shared_dash", "tab_1", "shared_tg", "shared_dg"), 1, 0, nil)

		list := MergeList{
			Target: targetPath.String(),
			Path:   targetPath,
			Sources: []Source{
				{Name: "red", Path: redPath},
				{Name: "blue", Path: bluePath},
			},
		}

		ticker := make(chan time.Time)
		snaps, err := mergeUpdateListen(ctx, client, nil, list, false, false, ticker)
		if err != nil {
			t.Fatalf("mergeUpdateListen() unexpected error: %v", err)
		}

		initial := mustReadChannel(t, snaps)
		if _, ok := initial.Dashboards["shared_dash"]; !ok {
			t.Errorf("initial missing shared_dash")
		}
		if _, ok := initial.Dashboards["red-shared_dash"]; !ok {
			t.Errorf("initial missing red-shared_dash")
		}

		// Update blue so there is no longer a name conflict; red does not change.
		setFakeObject(client, *bluePath, validConfig("blue_unique_dash", "tab_1", "blue_unique_tg", "blue_unique_dg"), 2, 0, nil)
		ticker <- time.Now()

		updated := mustReadChannel(t, snaps)
		if _, ok := updated.Dashboards["blue_unique_dash"]; !ok {
			t.Errorf("updated missing blue_unique_dash")
		}
		if _, ok := updated.Dashboards["shared_dash"]; !ok {
			t.Errorf("updated missing unprefixed shared_dash from red: %v", updated.Dashboards)
		}
		if _, ok := updated.Dashboards["red-shared_dash"]; ok {
			t.Errorf("updated unexpectedly kept prefixed red-shared_dash after conflict was resolved")
		}
	})

	t.Run("Initially invalid source is still observed and merged once fixed", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client := newFakeConditionalClient()
		invalidCfg := &configpb.Configuration{
			Dashboards: []*configpb.Dashboard{{Name: "broken_dash"}},
		}
		setFakeObject(client, *redPath, validConfig("red_dash", "red_tab", "red_tg", "red_dg"), 1, 0, nil)
		setFakeObject(client, *bluePath, invalidCfg, 1, 0, nil)

		list := MergeList{
			Target: targetPath.String(),
			Path:   targetPath,
			Sources: []Source{
				{Name: "red", Path: redPath},
				{Name: "blue", Path: bluePath},
			},
		}

		ticker := make(chan time.Time)
		snaps, err := mergeUpdateListen(ctx, client, nil, list, false, false, ticker)
		if err != nil {
			t.Fatalf("mergeUpdateListen() unexpected error: %v", err)
		}

		initial := mustReadChannel(t, snaps)
		if len(initial.Dashboards) != 1 || initial.Dashboards["red_dash"] == nil {
			t.Fatalf("expected initial snapshot to contain only red_dash, got: %v", initial.Dashboards)
		}

		// Fix blue's config at generation 2.
		setFakeObject(client, *bluePath, validConfig("blue_dash", "blue_tab", "blue_tg", "blue_dg"), 2, 0, nil)
		ticker <- time.Now()

		updated := mustReadChannel(t, snaps)
		if updated.Dashboards["red_dash"] == nil || updated.Dashboards["blue_dash"] == nil {
			t.Errorf("expected updated snapshot to contain both red_dash and blue_dash, got: %v", updated.Dashboards)
		}
	})

	t.Run("Source becoming invalid on update is skipped, then merged again when valid", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client := newFakeConditionalClient()
		setFakeObject(client, *redPath, validConfig("red_dash", "red_tab", "red_tg", "red_dg"), 1, 0, nil)
		setFakeObject(client, *bluePath, validConfig("blue_dash_1", "blue_tab", "blue_tg_1", "blue_dg"), 1, 0, nil)

		list := MergeList{
			Target: targetPath.String(),
			Path:   targetPath,
			Sources: []Source{
				{Name: "red", Path: redPath},
				{Name: "blue", Path: bluePath},
			},
		}

		ticker := make(chan time.Time)
		snaps, err := mergeUpdateListen(ctx, client, nil, list, false, true, ticker)
		if err != nil {
			t.Fatalf("mergeUpdateListen() unexpected error: %v", err)
		}

		initial := mustReadChannel(t, snaps)
		if initial.Dashboards["red_dash"] == nil || initial.Dashboards["blue_dash_1"] == nil {
			t.Fatalf("unexpected initial dashboards: %v", initial.Dashboards)
		}

		// Push an invalid config to blue (generation 2): should be skipped without emitting.
		invalidCfg := &configpb.Configuration{
			Dashboards: []*configpb.Dashboard{{Name: "broken_dash"}},
		}
		setFakeObject(client, *bluePath, invalidCfg, 2, 0, nil)
		ticker <- time.Now()
		assertChannelEmpty(t, snaps, 100*time.Millisecond)

		client.Lock.RLock()
		genAfterInvalid := client.Uploader[*targetPath].Generation
		client.Lock.RUnlock()
		if genAfterInvalid != 1 {
			t.Errorf("expected target upload Generation to remain 1 after invalid update, got %d", genAfterInvalid)
		}

		// Push a valid config to blue (generation 3): should merge and emit.
		setFakeObject(client, *bluePath, validConfig("blue_dash_3", "blue_tab", "blue_tg_3", "blue_dg"), 3, 0, nil)
		ticker <- time.Now()

		updated := mustReadChannel(t, snaps)
		if updated.Dashboards["red_dash"] == nil || updated.Dashboards["blue_dash_3"] == nil {
			t.Errorf("expected red_dash and blue_dash_3, got: %v", updated.Dashboards)
		}
	})

	t.Run("Does not emit snapshot when generations have not changed", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client := newFakeConditionalClient()
		setFakeObject(client, *redPath, validConfig("red_dash", "red_tab", "red_tg", "red_dg"), 1, 0, nil)
		setFakeObject(client, *bluePath, validConfig("blue_dash", "blue_tab", "blue_tg", "blue_dg"), 1, 0, nil)

		list := MergeList{
			Target: targetPath.String(),
			Path:   targetPath,
			Sources: []Source{
				{Name: "red", Path: redPath},
				{Name: "blue", Path: bluePath},
			},
		}

		ticker := make(chan time.Time)
		snaps, err := mergeUpdateListen(ctx, client, nil, list, false, false, ticker)
		if err != nil {
			t.Fatalf("mergeUpdateListen() unexpected error: %v", err)
		}

		mustReadChannel(t, snaps)
		ticker <- time.Now()
		assertChannelEmpty(t, snaps, 100*time.Millisecond)
	})
}

func validTestGroup(name string) *configpb.TestGroup {
	return &configpb.TestGroup{
		Name:             name,
		GcsPrefix:        "tests_live_here",
		DaysOfResults:    1,
		NumColumnsRecent: 1,
	}
}

func validConfig(dashName, tabName, tgName, dgName string) *configpb.Configuration {
	return &configpb.Configuration{
		Dashboards: []*configpb.Dashboard{
			{
				Name: dashName,
				DashboardTab: []*configpb.DashboardTab{
					{
						Name:          tabName,
						TestGroupName: tgName,
					},
				},
			},
		},
		TestGroups: []*configpb.TestGroup{
			validTestGroup(tgName),
		},
		DashboardGroups: []*configpb.DashboardGroup{
			{
				Name:           dgName,
				DashboardNames: []string{dashName},
			},
		},
	}
}

func newFakeConditionalClient() *fake.ConditionalClient {
	return &fake.ConditionalClient{
		UploadClient: fake.UploadClient{
			Uploader: fake.Uploader{},
			Client: fake.Client{
				Lister: fake.Lister{},
				Opener: fake.Opener{
					Paths: map[gcs.Path]fake.Object{},
					Lock:  &sync.RWMutex{},
				},
			},
			Stater: fake.Stater{},
		},
		Lock: &sync.RWMutex{},
	}
}

func setFakeObject(c *fake.ConditionalClient, path gcs.Path, cfg *configpb.Configuration, gen, size int64, err error) {
	var data string
	if cfg != nil {
		b, mErr := proto.Marshal(cfg)
		if mErr != nil {
			panic(mErr)
		}
		data = string(b)
		if size == 0 {
			size = int64(len(b))
		}
	}
	c.Lock.Lock()
	defer c.Lock.Unlock()
	c.Opener.Paths[path] = fake.Object{
		Data: data,
		Attrs: &storage.ReaderObjectAttrs{
			Generation:   gen,
			Size:         size,
			LastModified: time.Now().Add(-time.Second),
		},
		ReadErr: err,
	}
	c.Stater[path] = fake.Stat{
		Attrs: storage.ObjectAttrs{
			Generation: gen,
			Size:       size,
		},
		Err: err,
	}
}

func mustReadChannel(t *testing.T, snaps <-chan *snapshot.Config) *snapshot.Config {
	t.Helper()
	select {
	case s, ok := <-snaps:
		if !ok {
			t.Fatalf("snaps channel closed unexpectedly")
		}
		return s
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for snapshot")
		return nil
	}
}

func assertChannelEmpty(t *testing.T, snaps <-chan *snapshot.Config, wait time.Duration) {
	t.Helper()
	select {
	case s, ok := <-snaps:
		if !ok {
			t.Fatalf("snaps channel closed unexpectedly")
		}
		t.Fatalf("did not expect snapshot, but got: %v", s)
	case <-time.After(wait):
	}
}

type fakeOpener map[string]fakeObject

func (fo fakeOpener) Open(_ context.Context, path gcs.Path) (io.ReadCloser, *storage.ReaderObjectAttrs, error) {
	o, ok := fo[path.String()]
	if !ok {
		return nil, nil, fmt.Errorf("wrap not exist: %w", storage.ErrObjectNotExist)
	}
	if o.err != nil {
		return nil, nil, fmt.Errorf("injected open error: %w", o.err)
	}
	return io.NopCloser(bytes.NewReader(o.buf)), &storage.ReaderObjectAttrs{}, nil
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
