/*
Copyright 2018 The Kubernetes Authors.

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

package updater

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/testgrid/config"
	"github.com/GoogleCloudPlatform/testgrid/metadata"
	_ "github.com/GoogleCloudPlatform/testgrid/metadata/junit"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	statuspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs/fake"
	"github.com/fvbommel/sortorder"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/googleapi"
	"google.golang.org/protobuf/testing/protocmp"
	core "k8s.io/api/core/v1"
)

type fakeUpload = fake.Upload
type fakeStater = fake.Stater
type fakeStat = fake.Stat
type fakeUploader = fake.Uploader
type fakeUploadClient = fake.UploadClient
type fakeLister = fake.Lister
type fakeOpener = fake.Opener

func TestGCS(t *testing.T) {
	cases := []struct {
		name  string
		ctx   context.Context
		group *configpb.TestGroup
		fail  bool
	}{
		{
			name:  "contextless",
			group: &configpb.TestGroup{},
			fail:  true,
		},
		{
			name:  "basic",
			ctx:   context.Background(),
			group: &configpb.TestGroup{},
		},
		{
			name: "kubernetes", // should fail
			ctx:  context.Background(),
			group: &configpb.TestGroup{
				UseKubernetesClient: true,
			},
			fail: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Goal here is to ignore for non-k8s client otherwise if we get past this check
			// send updater() arguments that should fail if it tries to do anything,
			// either because the context is canceled or things like client are unset)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			defer func() {
				if r := recover(); r != nil {
					if !tc.fail {
						t.Errorf("updater() got an unexpected panic: %#v", r)
					}
				}
			}()
			updater := GCS(tc.ctx, nil, 0, 0, 0, false, SortStarted)
			_, err := updater(ctx, logrus.WithField("case", tc.name), nil, tc.group, gcs.Path{})
			switch {
			case err != nil:
				if !tc.fail {
					t.Errorf("updater() got unexpected error: %v", err)
				}
			case tc.fail:
				t.Error("updater() failed to return an error")
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	defaultTimeout := 5 * time.Minute
	configPath := newPathOrDie("gs://bucket/path/to/config")
	cases := []struct {
		name             string
		ctx              context.Context
		config           *configpb.Configuration
		configErr        error
		builds           map[string][]fakeBuild
		gridPrefix       string
		groupConcurrency int
		buildConcurrency int
		skipConfirm      bool
		groupUpdater     GroupUpdater
		groupTimeout     *time.Duration
		buildTimeout     *time.Duration
		groupNames       []string
		freq             time.Duration

		expected  fakeUploader
		err       bool
		successes int
		errors    int
		skips     int
	}{
		{
			name: "basically works",
			config: &configpb.Configuration{
				TestGroups: []*configpb.TestGroup{
					{
						Name:                "hello",
						GcsPrefix:           "kubernetes-jenkins/path/to/job",
						DaysOfResults:       7,
						UseKubernetesClient: true,
						NumColumnsRecent:    6,
					},
					{
						Name:             "modern",
						DaysOfResults:    7,
						NumColumnsRecent: 6,
						ResultSource: &configpb.TestGroup_ResultSource{
							ResultSourceConfig: &configpb.TestGroup_ResultSource_GcsConfig{
								GcsConfig: &configpb.GCSConfig{
									GcsPrefix: "kubernetes-jenkins/path/to/another-job",
								},
							},
						},
					},
					{
						Name:             "skip-non-k8s",
						GcsPrefix:        "kubernetes-jenkins/path/to/job",
						DaysOfResults:    7,
						NumColumnsRecent: 6,
					},
				},
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dash",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name:          "hello-tab",
								TestGroupName: "hello",
							},
							{
								Name:          "modern-tab",
								TestGroupName: "modern",
							},
							{
								Name:          "skip-tab",
								TestGroupName: "skip-non-k8s",
							},
						},
					},
				},
			},
			expected: fakeUploader{
				*resolveOrDie(&configPath, "hello"): {
					Buf:          mustGrid(&statepb.Grid{}),
					CacheControl: "no-cache",
					WorldRead:    gcs.DefaultACL,
					Generation:   2,
				},
				*resolveOrDie(&configPath, "modern"): {
					Buf:          mustGrid(&statepb.Grid{}),
					CacheControl: "no-cache",
					Generation:   2,
					WorldRead:    gcs.DefaultACL,
				},
				*resolveOrDie(&configPath, "skip-non-k8s"): {
					Buf:          mustGrid(&statepb.Grid{}),
					CacheControl: "no-cache",
					WorldRead:    gcs.DefaultACL,
					Generation:   1,
				},
			},
			successes: 3,
		},
		{
			name:       "bad grid prefix",
			gridPrefix: "!@#$%^&*()",
			config: &configpb.Configuration{
				TestGroups: []*configpb.TestGroup{
					{
						Name:                "hello",
						GcsPrefix:           "kubernetes-jenkins/path/to/job",
						DaysOfResults:       7,
						UseKubernetesClient: true,
						NumColumnsRecent:    6,
					},
				},
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dash",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name:          "hello-tab",
								TestGroupName: "hello",
							},
						},
					},
				},
			},
			expected: fakeUploader{},
			err:      true,
		},
		{
			name: "update specified",
			config: &configpb.Configuration{
				TestGroups: []*configpb.TestGroup{
					{
						Name:                "hello",
						GcsPrefix:           "kubernetes-jenkins/path/to/job",
						DaysOfResults:       7,
						UseKubernetesClient: true,
						NumColumnsRecent:    6,
					},
					{
						Name:                "hiya",
						GcsPrefix:           "kubernetes-jenkins/path/to/job",
						DaysOfResults:       7,
						UseKubernetesClient: true,
						NumColumnsRecent:    6,
					},
					{
						Name:                "goodbye",
						GcsPrefix:           "kubernetes-jenkins/path/to/job",
						DaysOfResults:       7,
						UseKubernetesClient: true,
						NumColumnsRecent:    6,
					},
				},
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dash",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name:          "hello-tab",
								TestGroupName: "hello",
							},
							{
								Name:          "hiya-tab",
								TestGroupName: "hiya",
							},
							{
								Name:          "goodbye-tab",
								TestGroupName: "goodbye",
							},
						},
					},
				},
			},
			groupNames: []string{"hello", "hiya"},
			expected: fakeUploader{
				*resolveOrDie(&configPath, "hello"): {
					Buf:          mustGrid(&statepb.Grid{}),
					CacheControl: "no-cache",
					WorldRead:    gcs.DefaultACL,
					Generation:   2,
				},
				*resolveOrDie(&configPath, "hiya"): {
					Buf:          mustGrid(&statepb.Grid{}),
					CacheControl: "no-cache",
					Generation:   2,
					WorldRead:    gcs.DefaultACL,
				},
			},
			successes: 2,
		},
		{
			name: "update error with freq = 0",
			config: &configpb.Configuration{
				TestGroups: []*configpb.TestGroup{
					{
						Name:                "hello",
						GcsPrefix:           "kubernetes-jenkins/path/to/job",
						DaysOfResults:       7,
						UseKubernetesClient: true,
						NumColumnsRecent:    6,
					},
					{
						Name:                "world",
						GcsPrefix:           "kubernetes-jenkins/path/to/job",
						DaysOfResults:       7,
						UseKubernetesClient: true,
						NumColumnsRecent:    6,
					},
				},
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dash",
						DashboardTab: []*configpb.DashboardTab{
							{
								Name:          "hello-tab",
								TestGroupName: "hello",
							},
							{
								Name:          "world-tab",
								TestGroupName: "world",
							},
						},
					},
				},
			},
			groupUpdater: func(_ context.Context, _ logrus.FieldLogger, _ gcs.Client, tg *configpb.TestGroup, _ gcs.Path) (bool, error) {
				if tg.Name == "world" {
					return false, &googleapi.Error{
						Code: http.StatusPreconditionFailed,
					}
				}
				return false, errors.New("bad update")

			},
			builds: make(map[string][]fakeBuild),
			freq:   time.Duration(0),
			expected: fakeUploader{
				*resolveOrDie(&configPath, "hello"): {
					Buf:          mustGrid(&statepb.Grid{}),
					CacheControl: "no-cache",
					WorldRead:    gcs.DefaultACL,
					Generation:   1,
				},
				*resolveOrDie(&configPath, "world"): {
					Buf:          mustGrid(&statepb.Grid{}),
					CacheControl: "no-cache",
					WorldRead:    gcs.DefaultACL,
					Generation:   1,
				},
			},
			errors: 1,
			skips:  1,
		},
		// TODO(fejta): more cases
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ctx == nil {
				tc.ctx = context.Background()
			}
			ctx, cancel := context.WithCancel(tc.ctx)
			defer cancel()

			if tc.groupConcurrency == 0 {
				tc.groupConcurrency = 1
			}
			if tc.buildConcurrency == 0 {
				tc.buildConcurrency = 1
			}
			if tc.groupTimeout == nil {
				tc.groupTimeout = &defaultTimeout
			}
			if tc.buildTimeout == nil {
				tc.buildTimeout = &defaultTimeout
			}

			client := &fake.ConditionalClient{
				UploadClient: fake.UploadClient{
					Uploader: fakeUploader{},
					Client: fakeClient{
						Lister: fakeLister{},
						Opener: fakeOpener{},
					},
				},
				Lock: &sync.RWMutex{},
			}

			client.Opener[configPath] = fakeObject{
				Data: func() string {
					b, err := config.MarshalBytes(tc.config)
					if err != nil {
						t.Fatalf("config.MarshalBytes() errored: %v", err)
					}
					return string(b)
				}(),
				Attrs:   &storage.ReaderObjectAttrs{},
				ReadErr: tc.configErr,
			}

			for _, group := range tc.config.TestGroups {
				builds, ok := tc.builds[group.Name]
				if !ok {
					continue
				}
				buildsPath := newPathOrDie("gs://" + group.GcsPrefix)
				fi := client.Lister[buildsPath]
				for _, build := range addBuilds(&client.Client, buildsPath, builds...) {
					fi.Objects = append(fi.Objects, storage.ObjectAttrs{
						Prefix: build.Path.Object(),
					})
				}
				client.Lister[buildsPath] = fi
			}

			if tc.groupUpdater == nil {
				poolCtx, poolCancel := context.WithCancel(context.Background())
				defer poolCancel()
				tc.groupUpdater = GCS(poolCtx, client, *tc.groupTimeout, *tc.buildTimeout, tc.buildConcurrency, !tc.skipConfirm, SortStarted)
			}
			err := Update(
				ctx,
				client,
				nil, // metric
				configPath,
				tc.gridPrefix,
				tc.groupConcurrency,
				tc.groupNames,
				tc.groupUpdater,
				!tc.skipConfirm,
				tc.freq,
			)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("Update() got unexpected error: %v", err)
				}
			case tc.err:
				t.Error("Update() failed to receive an error")
			default:
				actual := client.Uploader
				if diff := cmp.Diff(tc.expected, actual, cmp.AllowUnexported(fakeUpload{})); diff != "" {
					t.Errorf("Update() uploaded files got unexpected diff (-want, +got):\n%s", diff)
				}
			}
		})
	}
}

func TestTestGroupPath(t *testing.T) {
	path := newPathOrDie("gs://bucket/config")
	pNewPathOrDie := func(s string) *gcs.Path {
		p := newPathOrDie(s)
		return &p
	}
	cases := []struct {
		name       string
		groupName  string
		gridPrefix string
		expected   *gcs.Path
	}{
		{
			name:     "basically works",
			expected: &path,
		},
		{
			name:      "invalid group name errors",
			groupName: "---://foo",
		},
		{
			name:      "bucket change errors",
			groupName: "gs://honey-bucket/config",
		},
		{
			name:      "normal behavior works",
			groupName: "random-group",
			expected:  pNewPathOrDie("gs://bucket/random-group"),
		},
		{
			name:      "target a subfolder works",
			groupName: "beta/random-group",
			expected:  pNewPathOrDie("gs://bucket/beta/random-group"),
		},
		{
			name:      "resolve reference fails",
			groupName: "http://bucket/config",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := TestGroupPath(path, tc.gridPrefix, tc.groupName)
			switch {
			case err != nil:
				if tc.expected != nil {
					t.Errorf("testGroupPath(%v, %v) got unexpected error: %v", path, tc.groupName, err)
				}
			case tc.expected == nil:
				t.Errorf("testGroupPath(%v, %v) failed to receive an error", path, tc.groupName)
			default:
				if diff := cmp.Diff(actual, tc.expected, cmp.AllowUnexported(gcs.Path{})); diff != "" {
					t.Errorf("testGroupPath(%v, %v) got unexpected diff (-have, +want):\n%s", path, tc.groupName, diff)
				}
			}
		})
	}
}

func jsonStarted(stamp int64) *fakeObject {
	return &fakeObject{
		Data: jsonData(metadata.Started{Timestamp: stamp}),
	}
}

func jsonFinished(stamp int64, passed bool, meta metadata.Metadata) *fakeObject {
	return &fakeObject{
		Data: jsonData(metadata.Finished{
			Timestamp: &stamp,
			Passed:    &passed,
			Metadata:  meta,
		}),
	}
}

var (
	podInfoSuccessPodInfo = gcs.PodInfo{
		Pod: &core.Pod{
			Status: core.PodStatus{
				Phase: core.PodSucceeded,
			},
		},
	}
	podInfoSuccess     = jsonPodInfo(podInfoSuccessPodInfo)
	podInfoPassCell    = cell{Result: statuspb.TestStatus_PASS}
	podInfoMissingCell = cell{
		Result:  statuspb.TestStatus_RUNNING,
		Icon:    "!",
		Message: gcs.MissingPodInfo,
	}
)

func jsonPodInfo(podInfo gcs.PodInfo) *fakeObject {
	return &fakeObject{Data: jsonData(podInfo)}
}

func mustGrid(grid *statepb.Grid) []byte {
	buf, err := gcs.MarshalGrid(grid)
	if err != nil {
		panic(err)
	}
	return buf
}

func TestTruncateRunning(t *testing.T) {
	now := float64(time.Now().UTC().Unix() * 1000)
	floor := time.Now().Add(-72 * time.Hour)
	ancient := float64(time.Now().Add(-74*time.Hour).UTC().Unix() * 1000)
	cases := []struct {
		name     string
		cols     []inflatedColumn
		expected func([]inflatedColumn) []inflatedColumn
	}{
		{
			name: "basically works",
		},
		{
			name: "keep everything (no Overall)",
			cols: []inflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "this",
						Started: now,
					},
				},
				{
					Column: &statepb.Column{
						Build:   "that",
						Started: now,
					},
				},
				{
					Column: &statepb.Column{
						Build:   "another",
						Started: now,
					},
				},
			},
		},
		{
			name: "keep everything completed",
			cols: []inflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "passed",
						Started: now,
					},
					Cells: map[string]cell{overallRow: {Result: statuspb.TestStatus_PASS}},
				},
				{
					Column: &statepb.Column{
						Build:   "failed",
						Started: now,
					},
					Cells: map[string]cell{overallRow: {Result: statuspb.TestStatus_FAIL}},
				},
			},
		},
		{
			name: "drop everything before oldest running",
			cols: []inflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "this1",
						Started: now,
					},
				},
				{
					Column: &statepb.Column{
						Build:   "this2",
						Started: now,
					},
				},
				{
					Column: &statepb.Column{
						Build:   "running1",
						Started: now,
					},
					Cells: map[string]cell{overallRow: {Result: statuspb.TestStatus_RUNNING}},
				},
				{
					Column: &statepb.Column{
						Build:   "this3",
						Started: now,
					},
				},
				{
					Column: &statepb.Column{
						Build:   "running2",
						Started: now,
					},
					Cells: map[string]cell{overallRow: {Result: statuspb.TestStatus_RUNNING}},
				},
				{
					Column: &statepb.Column{
						Build:   "this4",
						Started: now,
					},
				},
				{
					Column: &statepb.Column{
						Build:   "this5",
						Started: now,
					},
				},
				{
					Column: &statepb.Column{
						Build:   "this6",
						Started: now,
					},
				},
				{
					Column: &statepb.Column{
						Build:   "this7",
						Started: now,
					},
				},
			},
			expected: func(cols []inflatedColumn) []inflatedColumn {
				return cols[5:] // this4 and earlier
			},
		},
		{
			name: "drop all as all are running",
			cols: []inflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "running1",
						Started: now,
					},
					Cells: map[string]cell{overallRow: {Result: statuspb.TestStatus_RUNNING}},
				},
				{
					Column: &statepb.Column{
						Build:   "running2",
						Started: now,
					},
					Cells: map[string]cell{overallRow: {Result: statuspb.TestStatus_RUNNING}},
				},
			},
			expected: func(cols []inflatedColumn) []inflatedColumn {
				return cols[2:]
			},
		},
		{
			name: "drop running columns if any process is running",
			cols: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "running",
						Started: now,
					},
					Cells: map[string]cell{
						"process1": {Result: statuspb.TestStatus_RUNNING},
						"process2": {Result: statuspb.TestStatus_RUNNING},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "running-partially",
						Started: now,
					},
					Cells: map[string]cell{
						"process1": {Result: statuspb.TestStatus_RUNNING},
						"process2": {Result: statuspb.TestStatus_PASS},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "ok",
						Started: now,
					},
					Cells: map[string]cell{
						"process1": {Result: statuspb.TestStatus_PASS},
						"process2": {Result: statuspb.TestStatus_PASS},
					},
				},
			},
			expected: func(cols []inflatedColumn) []inflatedColumn {
				return cols[2:]
			},
		},
		{
			name: "ignore ancient running columns",
			cols: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "recent-running",
						Started: now,
					},
					Cells: map[string]cell{"drop": {Result: statuspb.TestStatus_RUNNING}},
				},
				{
					Column: &statepb.Column{
						Build:   "recent-done",
						Started: now - 1,
					},
					Cells: map[string]cell{"keep": {Result: statuspb.TestStatus_PASS}},
				},

				{
					Column: &statepb.Column{
						Build:   "running-ancient",
						Started: ancient,
					},
					Cells: map[string]cell{"too-old-to-drop": {Result: statuspb.TestStatus_RUNNING}},
				},
				{
					Column: &statepb.Column{
						Build:   "ok",
						Started: ancient - 1,
					},
					Cells: map[string]cell{"also keep": {Result: statuspb.TestStatus_PASS}},
				},
			},
			expected: func(cols []inflatedColumn) []inflatedColumn {
				return cols[1:]
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := truncateRunning(tc.cols, floor)
			expected := tc.cols
			if tc.expected != nil {
				expected = tc.expected(expected)
			}
			if diff := cmp.Diff(actual, expected, protocmp.Transform()); diff != "" {
				t.Errorf("truncateRunning() got unexpected diff:\n%s", diff)
			}
		})
	}
}

func TestListBuilds(t *testing.T) {
	cases := []struct {
		name     string
		since    string
		client   fakeLister
		paths    []gcs.Path
		expected []gcs.Build
		err      bool
	}{
		{
			name: "basically works",
		},
		{
			name: "list err",
			client: fakeLister{
				newPathOrDie("gs://prefix/job/"): fakeIterator{
					Objects: []storage.ObjectAttrs{
						{
							Prefix: "job/1/",
						},
						{
							Prefix: "job/10/",
						},
						{
							Prefix: "job/2/",
						},
					},
					Err: 1,
				},
			},
			paths: []gcs.Path{
				newPathOrDie("gs://prefix/job/"),
			},
			err: true,
		},
		{
			name: "bucket err",
			client: fakeLister{
				newPathOrDie("gs://prefix/job/"): fakeIterator{
					Objects: []storage.ObjectAttrs{
						{
							Prefix: "job/1/",
						},
						{
							Prefix: "job/10/",
						},
						{
							Prefix: "job/2/",
						},
					},
					ErrOpen: storage.ErrBucketNotExist,
				},
			},
			paths: []gcs.Path{
				newPathOrDie("gs://prefix/job/"),
			},
			err: true,
		},
		{
			name: "list stuff correctly",
			client: fakeLister{
				newPathOrDie("gs://prefix/job/"): fakeIterator{
					Objects: []storage.ObjectAttrs{
						{
							Prefix: "job/1/",
						},
						{
							Prefix: "job/10/",
						},
						{
							Prefix: "job/2/",
						},
					},
				},
			},
			paths: []gcs.Path{
				newPathOrDie("gs://prefix/job/"),
			},
			expected: []gcs.Build{
				{
					Path: newPathOrDie("gs://prefix/job/10/"),
				},
				{
					Path: newPathOrDie("gs://prefix/job/2/"),
				},
				{
					Path: newPathOrDie("gs://prefix/job/1/"),
				},
			},
		},
		{
			name:  "list offsets correctly",
			since: "3",
			client: fakeLister{
				newPathOrDie("gs://prefix/job/"): fakeIterator{
					Objects: []storage.ObjectAttrs{
						{
							Prefix: "job/1/",
						},
						{
							Prefix: "job/10/",
						},
						{
							Prefix: "job/2/",
						},
						{
							Prefix: "job/3/",
						},
						{
							Prefix: "job/4/",
						},
					},
				},
			},
			paths: []gcs.Path{
				newPathOrDie("gs://prefix/job/"),
			},
			expected: []gcs.Build{
				{
					Path: newPathOrDie("gs://prefix/job/10/"),
				},
				{
					Path: newPathOrDie("gs://prefix/job/4/"),
				},
			},
		},
		{
			name: "collate stuff correctly",
			client: fakeLister{
				newPathOrDie("gs://prefix/job/"): fakeIterator{
					Objects: []storage.ObjectAttrs{
						{
							Prefix: "job/1/",
						},
						{
							Prefix: "job/10/",
						},
						{
							Prefix: "job/3/",
						},
					},
				},
				newPathOrDie("gs://other-prefix/presubmit-job/"): fakeIterator{
					Objects: []storage.ObjectAttrs{
						{
							Name: "job/2",
							Metadata: map[string]string{
								"link": "gs://foo/bar333", // intentionally larger than job 20 and 4
							},
						},
						{
							Name: "job/20",
							Metadata: map[string]string{
								"link": "gs://foo/bar222",
							},
						},
						{

							Name: "job/4",
							Metadata: map[string]string{
								"link": "gs://foo/bar111",
							},
						},
					},
				},
			},
			paths: []gcs.Path{
				newPathOrDie("gs://prefix/job/"),
				newPathOrDie("gs://other-prefix/presubmit-job/"),
			},
			expected: []gcs.Build{
				{
					Path: newPathOrDie("gs://foo/bar222/"),
					// baseName: 20
				},
				{
					Path: newPathOrDie("gs://prefix/job/10/"),
				},
				{
					Path: newPathOrDie("gs://foo/bar111/"),
					// baseName: 4
				},
				{
					Path: newPathOrDie("gs://prefix/job/3/"),
				},
				{
					Path: newPathOrDie("gs://foo/bar333/"),
					// baseName: 2
				},
				{
					Path: newPathOrDie("gs://prefix/job/1/"),
				},
			},
		},
		{
			name:  "collated offsets work correctly",
			since: "5", // drop 4 3 2 1, keep 20, 10
			client: fakeLister{
				newPathOrDie("gs://prefix/job/"): fakeIterator{
					Objects: []storage.ObjectAttrs{
						{
							Prefix: "job/1/",
						},
						{
							Prefix: "job/10/",
						},
						{
							Prefix: "job/3/",
						},
					},
				},
				newPathOrDie("gs://other-prefix/presubmit-job/"): fakeIterator{
					Objects: []storage.ObjectAttrs{
						{
							Name: "job/2",
							Metadata: map[string]string{
								"link": "gs://foo/bar333", // intentionally larger than job 20 and 4
							},
						},
						{
							Name: "job/20",
							Metadata: map[string]string{
								"link": "gs://foo/bar222",
							},
						},
						{

							Name: "job/4",
							Metadata: map[string]string{
								"link": "gs://foo/bar111",
							},
						},
					},
				},
			},
			paths: []gcs.Path{
				newPathOrDie("gs://prefix/job/"),
				newPathOrDie("gs://other-prefix/presubmit-job/"),
			},
			expected: []gcs.Build{
				{
					Path: newPathOrDie("gs://foo/bar222/"),
					// baseName: 20
				},
				{
					Path: newPathOrDie("gs://prefix/job/10/"),
				},
			},
		},
	}

	compareBuilds := cmp.Comparer(func(x, y gcs.Build) bool {
		return x.String() == y.String()
	})
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := listBuilds(ctx, tc.client, tc.since, tc.paths...)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("listBuilds() got unexpected error: %v", err)
				}
			case tc.err:
				t.Errorf("listBuilds() failed to return an error")
			default:
				if diff := cmp.Diff(actual, tc.expected, cmp.AllowUnexported(gcs.Path{}), compareBuilds); diff != "" {
					t.Errorf("listBuilds() got unexpected diff (-have, +want):\n%s", diff)
				}
			}
		})
	}
}

func TestInflateDropAppend(t *testing.T) {
	const dayAgo = 60 * 60 * 24
	now := time.Now().Unix()
	uploadPath := newPathOrDie("gs://fake/upload/location")
	defaultTimeout := 5 * time.Minute
	// a simple ColumnReader that parses fakeBuilds
	fakeColReader := func(builds []fakeBuild) ColumnReader {
		return func(ctx context.Context, _ logrus.FieldLogger, _ *configpb.TestGroup, _ []InflatedColumn, _ time.Time, receivers chan<- InflatedColumn) error {
			ctx, cancel := context.WithCancel(ctx)
			defer cancel() // do not leak go routines
			for i := len(builds) - 1; i >= 0; i-- {
				b := builds[i]
				started := metadata.Started{}
				if err := json.Unmarshal([]byte(b.started.Data), &started); err != nil {
					return err
				}
				col := InflatedColumn{
					Column: &statepb.Column{
						Build:   b.id,
						Started: float64(started.Timestamp * 1000),
						Hint:    b.id,
					},
					Cells: map[string]Cell{},
				}
				for _, cell := range b.passed {
					col.Cells[cell] = Cell{Result: statuspb.TestStatus_PASS}
				}
				for _, cell := range b.failed {
					col.Cells[cell] = Cell{Result: statuspb.TestStatus_FAIL}
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case receivers <- col:
				}
			}
			return nil
		}
	}
	cases := []struct {
		name         string
		ctx          context.Context
		builds       []fakeBuild
		group        *configpb.TestGroup
		concurrency  int
		skipWrite    bool
		colReader    func(builds []fakeBuild) ColumnReader
		colSorter    ColumnSorter
		reprocess    time.Duration
		groupTimeout *time.Duration
		buildTimeout *time.Duration
		current      *fake.Object
		byteCeiling  int
		expected     *fakeUpload
		err          bool
	}{
		{
			name: "basically works",
			group: &configpb.TestGroup{
				GcsPrefix: "bucket/path/to/build/",
				ColumnHeader: []*configpb.TestGroup_ColumnHeader{
					{
						ConfigurationValue: "Commit",
					},
				},
			},
			builds: []fakeBuild{
				{
					id:      "99",
					started: jsonStarted(now + 99),
				},
				{
					id:      "80",
					started: jsonStarted(now + 80),
					podInfo: podInfoSuccess,
					finished: jsonFinished(now+81, true, metadata.Metadata{
						metadata.JobVersion: "build80",
					}),
					passed: []string{"good1", "good2", "flaky"},
				},
				{
					id:      "50",
					started: jsonStarted(now + 50),
					podInfo: podInfoSuccess,
					finished: jsonFinished(now+51, false, metadata.Metadata{
						metadata.JobVersion: "build50",
					}),
					passed: []string{"good1", "good2"},
					failed: []string{"flaky"},
				},
				{
					id:      "10",
					started: jsonStarted(now + 10),
					podInfo: podInfoSuccess,
					finished: jsonFinished(now+11, true, metadata.Metadata{
						metadata.JobVersion: "build10",
					}),
					passed: []string{"good1", "good2", "flaky"},
				},
			},
			expected: &fakeUpload{
				Buf: mustGrid(&statepb.Grid{
					Columns: []*statepb.Column{
						{
							Build:   "99",
							Hint:    "99",
							Started: float64(now+99) * 1000,
							Extra:   []string{""},
						},
						{
							Build:   "80",
							Hint:    "80",
							Started: float64(now+80) * 1000,
							Extra:   []string{"build80"},
						},
						{
							Build:   "50",
							Hint:    "50",
							Started: float64(now+50) * 1000,
							Extra:   []string{"build50"},
						},
						{
							Build:   "10",
							Hint:    "10",
							Started: float64(now+10) * 1000,
							Extra:   []string{"build10"},
						},
					},
					Rows: []*statepb.Row{
						setupRow(
							&statepb.Row{
								Name: "build." + overallRow,
								Id:   "build." + overallRow,
							},
							cell{
								Result:  statuspb.TestStatus_RUNNING,
								Message: "Build still running...",
								Icon:    "R",
							},
							cell{
								Result:  statuspb.TestStatus_PASS,
								Metrics: setElapsed(nil, 1),
							},
							cell{
								Result:  statuspb.TestStatus_FAIL,
								Metrics: setElapsed(nil, 1),
							},
							cell{
								Result:  statuspb.TestStatus_PASS,
								Metrics: setElapsed(nil, 1),
							},
						),
						setupRow(
							&statepb.Row{
								Name: "build." + podInfoRow,
								Id:   "build." + podInfoRow,
							},
							cell{Result: statuspb.TestStatus_NO_RESULT},
							podInfoPassCell,
							podInfoPassCell,
							podInfoPassCell,
						),
						setupRow(
							&statepb.Row{
								Name: "flaky",
								Id:   "flaky",
							},
							cell{Result: statuspb.TestStatus_NO_RESULT},
							cell{Result: statuspb.TestStatus_PASS},
							cell{
								Result:  statuspb.TestStatus_FAIL,
								Message: "flaky",
								Icon:    "F",
							},
							cell{Result: statuspb.TestStatus_PASS},
						),
						setupRow(
							&statepb.Row{
								Name: "good1",
								Id:   "good1",
							},
							cell{Result: statuspb.TestStatus_NO_RESULT},
							cell{Result: statuspb.TestStatus_PASS},
							cell{Result: statuspb.TestStatus_PASS},
							cell{Result: statuspb.TestStatus_PASS},
						),
						setupRow(
							&statepb.Row{
								Name: "good2",
								Id:   "good2",
							},
							cell{Result: statuspb.TestStatus_NO_RESULT},
							cell{Result: statuspb.TestStatus_PASS},
							cell{Result: statuspb.TestStatus_PASS},
							cell{Result: statuspb.TestStatus_PASS},
						),
					},
				}),
				CacheControl: "no-cache",
				WorldRead:    gcs.DefaultACL,
				Generation:   1,
			},
		},
		{
			name: "sort ascending",
			group: &configpb.TestGroup{
				GcsPrefix: "bucket/path/to/build/",
				ColumnHeader: []*configpb.TestGroup_ColumnHeader{
					{
						ConfigurationValue: "Commit",
					},
				},
			},
			colSorter: func(tg *configpb.TestGroup, cols []InflatedColumn) {
				want := configpb.TestGroup{ // same as tc.group
					GcsPrefix: "bucket/path/to/build/",
					ColumnHeader: []*configpb.TestGroup_ColumnHeader{
						{
							ConfigurationValue: "Commit",
						},
					},
				}
				if diff := cmp.Diff(&want, tg, protocmp.Transform()); diff != "" {
					panic(fmt.Sprintf("bad TestGroup:\n%s", diff))
				}
				sort.SliceStable(cols, func(i, j int) bool {
					return cols[i].Column.Started < cols[j].Column.Started
				})
			},
			builds: []fakeBuild{
				{
					id:      "99",
					started: jsonStarted(now + 99),
				},
				{
					id:      "80",
					started: jsonStarted(now + 80),
					podInfo: podInfoSuccess,
					finished: jsonFinished(now+81, true, metadata.Metadata{
						metadata.JobVersion: "build80",
					}),
					passed: []string{"good1", "good2", "flaky"},
				},
				{
					id:      "50",
					started: jsonStarted(now + 50),
					podInfo: podInfoSuccess,
					finished: jsonFinished(now+51, false, metadata.Metadata{
						metadata.JobVersion: "build50",
					}),
					passed: []string{"good1", "good2"},
					failed: []string{"flaky"},
				},
				{
					id:      "10",
					started: jsonStarted(now + 10),
					podInfo: podInfoSuccess,
					finished: jsonFinished(now+11, true, metadata.Metadata{
						metadata.JobVersion: "build10",
					}),
					passed: []string{"good1", "good2", "flaky"},
				},
			},
			expected: &fakeUpload{
				Buf: mustGrid(&statepb.Grid{
					Columns: []*statepb.Column{
						{
							Build:   "10",
							Hint:    "10",
							Started: float64(now+10) * 1000,
							Extra:   []string{"build10"},
						},
						{
							Build:   "50",
							Hint:    "50",
							Started: float64(now+50) * 1000,
							Extra:   []string{"build50"},
						},
						{
							Build:   "80",
							Hint:    "80",
							Started: float64(now+80) * 1000,
							Extra:   []string{"build80"},
						},
						{
							Build:   "99",
							Hint:    "99",
							Started: float64(now+99) * 1000,
							Extra:   []string{""},
						},
					},
					Rows: []*statepb.Row{
						setupRow(
							&statepb.Row{
								Name: "build." + overallRow,
								Id:   "build." + overallRow,
							},
							cell{
								Result:  statuspb.TestStatus_PASS,
								Metrics: setElapsed(nil, 1),
							},
							cell{
								Result:  statuspb.TestStatus_FAIL,
								Metrics: setElapsed(nil, 1),
							},
							cell{
								Result:  statuspb.TestStatus_PASS,
								Metrics: setElapsed(nil, 1),
							},
							cell{
								Result:  statuspb.TestStatus_RUNNING,
								Message: "Build still running...",
								Icon:    "R",
							},
						),
						setupRow(
							&statepb.Row{
								Name: "build." + podInfoRow,
								Id:   "build." + podInfoRow,
							},
							podInfoPassCell,
							podInfoPassCell,
							podInfoPassCell,
							cell{Result: statuspb.TestStatus_NO_RESULT},
						),
						setupRow(
							&statepb.Row{
								Name: "flaky",
								Id:   "flaky",
							},
							cell{Result: statuspb.TestStatus_PASS},
							cell{
								Result:  statuspb.TestStatus_FAIL,
								Message: "flaky",
								Icon:    "F",
							},
							cell{Result: statuspb.TestStatus_PASS},
							cell{Result: statuspb.TestStatus_NO_RESULT},
						),
						setupRow(
							&statepb.Row{
								Name: "good1",
								Id:   "good1",
							},
							cell{Result: statuspb.TestStatus_PASS},
							cell{Result: statuspb.TestStatus_PASS},
							cell{Result: statuspb.TestStatus_PASS},
							cell{Result: statuspb.TestStatus_NO_RESULT},
						),
						setupRow(
							&statepb.Row{
								Name: "good2",
								Id:   "good2",
							},
							cell{Result: statuspb.TestStatus_PASS},
							cell{Result: statuspb.TestStatus_PASS},
							cell{Result: statuspb.TestStatus_PASS},
							cell{Result: statuspb.TestStatus_NO_RESULT},
						),
					},
				}),
				CacheControl: "no-cache",
				WorldRead:    gcs.DefaultACL,
				Generation:   1,
			},
		},
		{
			name:      "do not write when requested",
			skipWrite: true,
			group: &configpb.TestGroup{
				GcsPrefix: "bucket/path/to/build/",
				ColumnHeader: []*configpb.TestGroup_ColumnHeader{
					{
						ConfigurationValue: "Commit",
					},
				},
			},
			builds: []fakeBuild{
				{
					id:      "99",
					started: jsonStarted(now + 99),
				},
				{
					id:      "80",
					started: jsonStarted(now + 80),
					finished: jsonFinished(now+81, true, metadata.Metadata{
						metadata.JobVersion: "build80",
					}),
					passed: []string{"good1", "good2", "flaky"},
				},
				{
					id:      "50",
					started: jsonStarted(now + 50),
					finished: jsonFinished(now+51, false, metadata.Metadata{
						metadata.JobVersion: "build50",
					}),
					passed: []string{"good1", "good2"},
					failed: []string{"flaky"},
				},
				{
					id:      "10",
					started: jsonStarted(now + 10),
					finished: jsonFinished(now+11, true, metadata.Metadata{
						metadata.JobVersion: "build10",
					}),
					passed: []string{"good1", "good2", "flaky"},
				},
			},
		},
		{
			name: "recent", // keep columns past the reprocess boundary
			group: &configpb.TestGroup{
				GcsPrefix: "bucket/path/to/build/",
				ColumnHeader: []*configpb.TestGroup_ColumnHeader{
					{
						ConfigurationValue: "Commit",
					},
				},
			},
			reprocess: 10 * time.Second,
			builds: []fakeBuild{
				{
					id:      "current",
					started: jsonStarted(now),
				},
			},
			current: &fake.Object{
				Data: string(mustGrid(&statepb.Grid{
					Columns: []*statepb.Column{
						{
							Build:   "current",
							Hint:    "should reprocess",
							Started: float64(now * 1000),
							Extra:   []string{""},
						},
						{
							Build:   "near boundary",
							Hint:    "1 should disappear",
							Started: float64(now-7) * 1000, // allow for 2s of clock drift
							Extra:   []string{""},
						},
						{
							Build:   "past boundary",
							Hint:    "boundary+999",
							Started: float64(now-9)*1000 - 1,
							Extra:   []string{"keep"},
						},
					},
					Rows: []*statepb.Row{
						setupRow(
							&statepb.Row{
								Name: "build." + overallRow,
								Id:   "build." + overallRow,
							},
							cell{
								Result:  statuspb.TestStatus_PASS,
								Message: "old data",
								Icon:    "should reprocess",
							},
							cell{
								Result:  statuspb.TestStatus_FAIL,
								Message: "delete me",
								Icon:    "me too",
							},
							cell{
								Result:  statuspb.TestStatus_FLAKY,
								Message: "keep me",
								Icon:    "yes stay",
							},
						),
					},
				})),
			},
			expected: &fakeUpload{
				Buf: mustGrid(&statepb.Grid{
					Columns: []*statepb.Column{
						{
							Build:   "current",
							Hint:    "current",
							Started: float64(now) * 1000,
							Extra:   []string{""},
						},
						{
							Build:   "past boundary",
							Hint:    "boundary+999",
							Started: float64(now-9)*1000 - 1,
							Extra:   []string{"keep"},
						},
					},
					Rows: []*statepb.Row{
						setupRow(
							&statepb.Row{
								Name: "build." + overallRow,
								Id:   "build." + overallRow,
							},
							cell{
								Result:  statuspb.TestStatus_RUNNING,
								Message: "Build still running...",
								Icon:    "R",
							},
							cell{
								Result:  statuspb.TestStatus_FLAKY,
								Message: "keep me",
								Icon:    "yes stay",
							},
						),
					},
				}),
				CacheControl: "no-cache",
				WorldRead:    gcs.DefaultACL,
				Generation:   1,
			},
		},
		{
			name: "only shrink",
			group: &configpb.TestGroup{
				GcsPrefix: "bucket/path/to/build/",
				ColumnHeader: []*configpb.TestGroup_ColumnHeader{
					{
						ConfigurationValue: "Commit",
					},
				},
			},
			reprocess:   time.Nanosecond,
			byteCeiling: 1,
			builds: []fakeBuild{
				{
					id:      "bad",
					started: jsonStarted(now),
				},
			},
			current: &fake.Object{
				Attrs: &storage.ReaderObjectAttrs{
					Size: 100000000,
				},
				Data: string(mustGrid(&statepb.Grid{
					Columns: []*statepb.Column{
						{
							Build:   "a",
							Hint:    "10",
							Started: float64(now * 1000),
							Extra:   []string{""},
						},
						{
							Build:   "b",
							Hint:    "5",
							Started: float64(now-5) * 1000,
							Extra:   []string{""},
						},
						{
							Build:   "c",
							Hint:    "3",
							Started: float64(now-7) * 1000,
							Extra:   []string{""},
						},
						{
							Build:   "d",
							Hint:    "2",
							Started: float64(now-8) * 1000,
							Extra:   []string{""},
						},
					},
					Rows: []*statepb.Row{
						setupRow(
							&statepb.Row{
								Name: "build." + overallRow,
								Id:   "build." + overallRow,
							},
							cell{
								Result:  statuspb.TestStatus_PASS,
								Message: "a",
								Icon:    "a",
							},
							cell{
								Result:  statuspb.TestStatus_FAIL,
								Message: "b",
								Icon:    "b",
							},
							cell{
								Result:  statuspb.TestStatus_FAIL,
								Message: "c",
								Icon:    "c",
							},
							cell{
								Result:  statuspb.TestStatus_FAIL,
								Message: "d",
								Icon:    "d",
							},
						),
					},
				})),
			},
			expected: &fakeUpload{
				Buf: mustGrid(&statepb.Grid{
					Columns: []*statepb.Column{
						{
							Hint:    "10",
							Started: float64(now-8) * 1000,
							Extra:   []string{""},
						},
					},
					Rows: []*statepb.Row{
						setupRow(
							&statepb.Row{
								Name: "Truncated",
								Id:   "Truncated",
							},
							cell{
								Result:  statuspb.TestStatus_UNKNOWN,
								Message: "4 cell grid exceeds maximum size of 1 cells, removed 2 rows",
							},
						),
						setupRow(
							&statepb.Row{
								Name: "build." + overallRow,
								Id:   "build." + overallRow,
							},
							cell{
								Result:  statuspb.TestStatus_PASS,
								Message: "a",
								Icon:    "a",
							},
						),
						setupRow(
							&statepb.Row{
								Name: "build." + overallRow + " [1]",
								Id:   "build." + overallRow,
							},
							cell{
								Result:  statuspb.TestStatus_FAIL,
								Message: "b",
								Icon:    "b",
							},
						),
					},
				}),
				CacheControl: "no-cache",
				WorldRead:    gcs.DefaultACL,
				Generation:   1,
			},
		},
		{
			name: "skip reprocess",
			group: &configpb.TestGroup{
				GcsPrefix: "bucket/path/to/build/",
				ColumnHeader: []*configpb.TestGroup_ColumnHeader{
					{
						ConfigurationValue: "Commit",
					},
				},
			},
			reprocess: 10 * time.Second,
			builds: []fakeBuild{
				{
					id:      "current",
					started: jsonStarted(now),
				},
			},
			current: &fake.Object{
				Data: string(mustGrid(&statepb.Grid{
					Columns: []*statepb.Column{
						{
							Build:   "current",
							Hint:    "should reprocess",
							Started: float64(now * 1000),
							Extra:   []string{""},
						},
						{
							Build:   "at boundary",
							Hint:    "1 should disappear",
							Started: float64(now-9) * 1000,
							Extra:   []string{""},
						},
						{
							Build:   "past boundary",
							Hint:    "1 running",
							Started: float64(now-40) * 1000,
							Extra:   []string{""},
						},
						{
							Build:   "paster boundary",
							Hint:    "1 maybe fix",
							Started: float64(now-50) * 1000,
							Extra:   []string{""},
						},
						{
							Build:   "pastest boundary",
							Hint:    "1 oldest",
							Started: float64(now-60) * 1000,
							Extra:   []string{"keep"},
						},
					},
					Rows: []*statepb.Row{
						setupRow(
							&statepb.Row{
								Name: "build." + overallRow,
								Id:   "build." + overallRow,
							},
							cell{
								Result:  statuspb.TestStatus_PASS,
								Message: "old data",
								Icon:    "should reprocess",
							},
							cell{
								Result:  statuspb.TestStatus_FAIL,
								Message: "delete me",
								Icon:    "me too",
							},
							cell{
								Result:  statuspb.TestStatus_RUNNING,
								Message: "delete me",
								Icon:    "me too",
							},
							cell{
								Result:  statuspb.TestStatus_FLAKY,
								Message: "maybe reprocess",
								Icon:    "?",
							},
							cell{
								Result:  statuspb.TestStatus_PASS,
								Message: "keep me",
								Icon:    "yes stay",
							},
						),
					},
				})),
			},
			expected: &fakeUpload{
				Buf: mustGrid(&statepb.Grid{
					Columns: []*statepb.Column{
						{
							Build:   "current",
							Hint:    "current",
							Started: float64(now) * 1000,
							Extra:   []string{""},
						},
						{
							Build:   "paster boundary",
							Hint:    "1 maybe fix",
							Started: float64(now-50) * 1000,
							Extra:   []string{""},
						},
						{
							Build:   "pastest boundary",
							Hint:    "1 oldest",
							Started: float64(now-60) * 1000,
							Extra:   []string{"keep"},
						},
					},
					Rows: []*statepb.Row{
						setupRow(
							&statepb.Row{
								Name: "build." + overallRow,
								Id:   "build." + overallRow,
							},
							cell{
								Result:  statuspb.TestStatus_RUNNING,
								Message: "Build still running...",
								Icon:    "R",
							},
							cell{
								Result:  statuspb.TestStatus_FLAKY,
								Message: "maybe reprocess",
								Icon:    "?",
							},
							cell{
								Result:  statuspb.TestStatus_PASS,
								Message: "keep me",
								Icon:    "yes stay",
							},
						),
					},
				}),
				CacheControl: "no-cache",
				WorldRead:    gcs.DefaultACL,
				Generation:   1,
			},
		},
		{
			name: "reprocess",
			group: &configpb.TestGroup{
				GcsPrefix: "bucket/path/to/build/",
				ColumnHeader: []*configpb.TestGroup_ColumnHeader{
					{
						ConfigurationValue: "Commit",
					},
				},
				DaysOfResults: 30,
			},
			reprocess: 10 * time.Second,
			builds: []fakeBuild{
				{
					id:      "current",
					started: jsonStarted(now),
				},
			},
			current: &fake.Object{
				Data: string(mustGrid(&statepb.Grid{
					Config: &configpb.TestGroup{
						GcsPrefix: "bucket/path/to/build/",
						ColumnHeader: []*configpb.TestGroup_ColumnHeader{
							{
								ConfigurationValue: "Commit",
							},
						},
						DaysOfResults: 20,
					},
					Columns: []*statepb.Column{
						{
							Build:   "current",
							Hint:    "should reprocess",
							Started: float64(now * 1000),
							Extra:   []string{""},
						},
						{
							Build:   "at boundary",
							Hint:    "1 should disappear",
							Started: float64(now-9) * 1000,
							Extra:   []string{""},
						},
						{
							Build:   "past boundary",
							Hint:    "1 running",
							Started: float64(now-40) * 1000,
							Extra:   []string{""},
						},
						{
							Build:   "paster boundary",
							Hint:    "1 maybe fix",
							Started: float64(now-50-dayAgo) * 1000,
							Extra:   []string{""},
						},
						{
							Build:   "pastest boundary",
							Hint:    "1 oldest",
							Started: float64(now-60-10*dayAgo) * 1000,
							Extra:   []string{"keep"},
						},
					},
					Rows: []*statepb.Row{
						setupRow(
							&statepb.Row{
								Name: "build." + overallRow,
								Id:   "build." + overallRow,
							},
							cell{
								Result:  statuspb.TestStatus_PASS,
								Message: "old data",
								Icon:    "should reprocess",
							},
							cell{
								Result:  statuspb.TestStatus_FAIL,
								Message: "delete me",
								Icon:    "me too",
							},
							cell{
								Result:  statuspb.TestStatus_RUNNING,
								Message: "delete me",
								Icon:    "me too",
							},
							cell{
								Result:  statuspb.TestStatus_FLAKY,
								Message: "maybe reprocess",
								Icon:    "?",
							},
							cell{
								Result:  statuspb.TestStatus_PASS,
								Message: "keep me",
								Icon:    "yes stay",
							},
						),
					},
				})),
			},
			expected: &fakeUpload{
				Buf: mustGrid(&statepb.Grid{
					Columns: []*statepb.Column{
						{
							Build:   "current",
							Hint:    "current",
							Started: float64(now) * 1000,
							Extra:   []string{""},
						},
						{
							Build:   "pastest boundary",
							Hint:    "1 oldest",
							Started: float64(now-60-10*dayAgo) * 1000,
							Extra:   []string{"keep"},
						},
					},
					Rows: []*statepb.Row{
						setupRow(
							&statepb.Row{
								Name: "build." + overallRow,
								Id:   "build." + overallRow,
							},
							cell{
								Result:  statuspb.TestStatus_RUNNING,
								Message: "Build still running...",
								Icon:    "R",
							},
							cell{
								Result:  statuspb.TestStatus_PASS,
								Message: "keep me",
								Icon:    "yes stay",
							},
						),
					},
				}),
				CacheControl: "no-cache",
				WorldRead:    gcs.DefaultACL,
				Generation:   1,
			},
		},
		{
			// short reprocessing time depends on our reprocessing running columns outside this timeframe.
			name: "running", // reprocess everything at least as new as the running column
			group: &configpb.TestGroup{
				GcsPrefix: "bucket/path/to/build/",
				ColumnHeader: []*configpb.TestGroup_ColumnHeader{
					{
						ConfigurationValue: "Commit",
					},
				},
			},
			reprocess: 10 * time.Second,
			builds: []fakeBuild{
				{
					id:      "current",
					started: jsonStarted(now),
				},
			},
			current: &fake.Object{
				Data: string(mustGrid(&statepb.Grid{
					Columns: []*statepb.Column{
						{
							Build:   "current",
							Hint:    "should reprocess",
							Started: float64(now * 1000),
							Extra:   []string{""},
						},
						{
							Build:   "at boundary",
							Hint:    "1 should disappear",
							Started: float64(now-9) * 1000,
							Extra:   []string{""},
						},
						{
							Build:   "past boundary",
							Hint:    "1 done but still reprocess",
							Started: float64(now-40) * 1000,
							Extra:   []string{""},
						},
						{
							Build:   "paster boundary",
							Hint:    "1 running",
							Started: float64(now-50) * 1000,
							Extra:   []string{""},
						},
						{
							Build:   "pastest boundary",
							Hint:    "1 oldest",
							Started: float64(now-60) * 1000,
							Extra:   []string{"keep"},
						},
					},
					Rows: []*statepb.Row{
						setupRow(
							&statepb.Row{
								Name: "build." + overallRow,
								Id:   "build." + overallRow,
							},
							cell{
								Result:  statuspb.TestStatus_PASS,
								Message: "old data",
								Icon:    "should reprocess",
							},
							cell{
								Result:  statuspb.TestStatus_FAIL,
								Message: "delete me",
								Icon:    "me too",
							},
							cell{
								Result:  statuspb.TestStatus_FAIL,
								Message: "delete me",
								Icon:    "me too",
							},
							cell{
								Result:  statuspb.TestStatus_RUNNING,
								Message: "delete me",
								Icon:    "me too",
							},
							cell{
								Result:  statuspb.TestStatus_FLAKY,
								Message: "keep me",
								Icon:    "yes stay",
							},
						),
					},
				})),
			},
			expected: &fakeUpload{
				Buf: mustGrid(&statepb.Grid{
					Columns: []*statepb.Column{
						{
							Build:   "current",
							Hint:    "current",
							Started: float64(now) * 1000,
							Extra:   []string{""},
						},
						{
							Build:   "pastest boundary",
							Hint:    "1 oldest",
							Started: float64(now-60) * 1000,
							Extra:   []string{"keep"},
						},
					},
					Rows: []*statepb.Row{
						setupRow(
							&statepb.Row{
								Name: "build." + overallRow,
								Id:   "build." + overallRow,
							},
							cell{
								Result:  statuspb.TestStatus_RUNNING,
								Message: "Build still running...",
								Icon:    "R",
							},
							cell{
								Result:  statuspb.TestStatus_FLAKY,
								Message: "keep me",
								Icon:    "yes stay",
							},
						),
					},
				}),
				CacheControl: "no-cache",
				WorldRead:    gcs.DefaultACL,
				Generation:   1,
			},
		},
		{
			name: "ignore empty", // ignore builds with no results.
			group: &configpb.TestGroup{
				GcsPrefix:              "bucket/path/to/build/",
				DisableProwjobAnalysis: true,
			},
			reprocess: 10 * time.Second,
			colReader: fakeColReader,
			builds: []fakeBuild{
				{
					id:       "cool5",
					started:  jsonStarted(now - 10),
					finished: jsonFinished(now-9, true, metadata.Metadata{}),
					passed:   []string{"a-test"},
				},
				{
					id:       "empty4",
					started:  jsonStarted(now - 20),
					finished: jsonFinished(now-19, true, metadata.Metadata{}),
				},
				{
					id:       "empty3",
					started:  jsonStarted(now - 30),
					finished: jsonFinished(now-29, true, metadata.Metadata{}),
				},
				{
					id:       "rad2",
					started:  jsonStarted(now - 40),
					finished: jsonFinished(now-39, true, metadata.Metadata{}),
					passed:   []string{"a-test"},
				},
				{
					id:       "empty1",
					started:  jsonStarted(now - 50),
					finished: jsonFinished(now-49, true, metadata.Metadata{}),
				},
			},
			current: &fake.Object{
				Data: string(mustGrid(&statepb.Grid{
					Columns: []*statepb.Column{},
					Rows:    []*statepb.Row{},
				})),
			},
			expected: &fakeUpload{
				Buf: mustGrid(&statepb.Grid{
					Columns: []*statepb.Column{
						{
							Build:   "cool5",
							Hint:    "cool5",
							Started: float64((now - 10) * 1000),
						},
						{
							Build:   "rad2",
							Hint:    "rad2",
							Started: float64((now - 40) * 1000),
						},
						{
							Build:   "",
							Hint:    "empty4",
							Started: float64((now - 50) * 1000),
						},
					},
					Rows: []*statepb.Row{
						setupRow(
							&statepb.Row{
								Name: "a-test",
								Id:   "a-test",
							},
							cell{Result: statuspb.TestStatus_PASS},
							cell{Result: statuspb.TestStatus_PASS},
							cell{Result: statuspb.TestStatus_NO_RESULT},
						),
					},
				}),
				CacheControl: "no-cache",
				WorldRead:    gcs.DefaultACL,
				Generation:   1,
			},
		},
		{
			name: "ignore empty with old columns", // correctly group old and new empty columns
			group: &configpb.TestGroup{
				GcsPrefix:              "bucket/path/to/build/",
				DisableProwjobAnalysis: true,
			},
			reprocess: 10 * time.Second,
			colReader: fakeColReader,
			builds: []fakeBuild{
				{
					id:       "empty9",
					started:  jsonStarted(now - 10),
					finished: jsonFinished(now-9, true, metadata.Metadata{}),
				},
				{
					id:       "empty8",
					started:  jsonStarted(now - 20),
					finished: jsonFinished(now-19, true, metadata.Metadata{}),
				},
				{
					id:       "wicked7",
					started:  jsonStarted(now - 30),
					finished: jsonFinished(now-29, true, metadata.Metadata{}),
					passed:   []string{"a-test"},
				},
				{
					id:       "empty6",
					started:  jsonStarted(now - 40),
					finished: jsonFinished(now-39, true, metadata.Metadata{}),
				},
			},
			current: &fake.Object{
				Data: string(mustGrid(&statepb.Grid{
					Columns: []*statepb.Column{
						{
							Build:   "cool5",
							Hint:    "cool5",
							Started: float64((now - 50) * 1000),
						},
						{
							Build:   "rad2",
							Hint:    "rad2",
							Started: float64((now - 80) * 1000),
						},
						{
							Build:   "",
							Name:    "",
							Hint:    "empty4",
							Started: float64((now - 90) * 1000),
						},
					},
					Rows: []*statepb.Row{
						setupRow(
							&statepb.Row{
								Name: "a-test",
								Id:   "a-test",
							},
							cell{Result: statuspb.TestStatus_PASS},
							cell{Result: statuspb.TestStatus_PASS},
							cell{Result: statuspb.TestStatus_NO_RESULT},
						),
					},
				})),
			},
			expected: &fakeUpload{
				Buf: mustGrid(&statepb.Grid{
					Columns: []*statepb.Column{
						{
							Build:   "wicked7",
							Hint:    "wicked7",
							Started: float64((now - 30) * 1000),
						},
						{
							Build:   "cool5",
							Hint:    "cool5",
							Started: float64((now - 50) * 1000),
						},
						{
							Build:   "rad2",
							Hint:    "rad2",
							Started: float64((now - 80) * 1000),
						},
						{
							Build:   "",
							Name:    "",
							Hint:    "empty9",
							Started: float64((now - 90) * 1000),
						},
					},
					Rows: []*statepb.Row{
						setupRow(
							&statepb.Row{
								Name: "a-test",
								Id:   "a-test",
							},
							cell{Result: statuspb.TestStatus_PASS},
							cell{Result: statuspb.TestStatus_PASS},
							cell{Result: statuspb.TestStatus_PASS},
							cell{Result: statuspb.TestStatus_NO_RESULT},
						),
					},
				}),
				CacheControl: "no-cache",
				WorldRead:    gcs.DefaultACL,
				Generation:   1,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ctx == nil {
				tc.ctx = context.Background()
			}
			ctx, cancel := context.WithCancel(tc.ctx)
			defer cancel()

			if tc.byteCeiling != 0 {
				defer func(orig int) {
					byteCeiling = orig
				}(byteCeiling)
				defer func(orig int) {
					cellCeiling = orig
				}(cellCeiling)
				byteCeiling = tc.byteCeiling
				cellCeiling = tc.byteCeiling

			}

			if tc.concurrency == 0 {
				tc.concurrency = 1
			}
			readResult := resultReaderPool(ctx, logrus.WithField("name", tc.name), tc.concurrency)
			if tc.groupTimeout == nil {
				tc.groupTimeout = &defaultTimeout
			}
			if tc.buildTimeout == nil {
				tc.buildTimeout = &defaultTimeout
			}

			client := &fake.ConditionalClient{
				UploadClient: fake.UploadClient{
					Uploader: fakeUploader{},
					Client: fakeClient{
						Lister: fakeLister{},
						Opener: fakeOpener{},
					},
				},
			}

			if tc.current != nil {
				client.Opener[uploadPath] = *tc.current
			}

			buildsPath := newPathOrDie("gs://" + tc.group.GcsPrefix)
			fi := client.Lister[buildsPath]
			for _, build := range addBuilds(&client.Client, buildsPath, tc.builds...) {
				fi.Objects = append(fi.Objects, storage.ObjectAttrs{
					Prefix: build.Path.Object(),
				})
			}
			client.Lister[buildsPath] = fi

			colReader := gcsColumnReader(client, *tc.buildTimeout, readResult)
			if tc.colReader != nil {
				colReader = tc.colReader(tc.builds)
			}
			if tc.colSorter == nil {
				tc.colSorter = SortStarted
			}
			_, err := InflateDropAppend(
				ctx,
				logrus.WithField("test", tc.name),
				client,
				tc.group,
				uploadPath,
				!tc.skipWrite,
				colReader,
				tc.colSorter,
				tc.reprocess,
			)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("updateGroup() got unexpected error: %v", err)
				}
			case tc.err:
				t.Error("updateGroup() failed to receive an exception")
			default:
				expected := fakeUploader{}
				if tc.expected != nil {
					expected[uploadPath] = *tc.expected
				}
				actual := client.Uploader
				diff := cmp.Diff(expected, actual, cmp.AllowUnexported(gcs.Path{}, fakeUpload{}), protocmp.Transform())
				if diff == "" {
					return
				}
				t.Errorf("updateGroup() got unexpected diff (-want +got):\n%s", diff)
				fakeDownloader := fakeOpener{
					uploadPath: {Data: string(actual[uploadPath].Buf)},
				}
				actualGrid, _, err := gcs.DownloadGrid(ctx, fakeDownloader, uploadPath)
				if err != nil {
					t.Errorf("actual gcs.DownloadGrid() got unexpected error: %v", err)
				}
				fakeDownloader[uploadPath] = fakeObject{Data: string(tc.expected.Buf)}
				expectedGrid, _, err := gcs.DownloadGrid(ctx, fakeDownloader, uploadPath)
				if err != nil {
					t.Errorf("expected gcs.DownloadGrid() got unexpected error: %v", err)
				}
				diff = cmp.Diff(expectedGrid, actualGrid, protocmp.Transform())
				if diff == "" {
					return
				}
				t.Errorf("gcs.DownloadGrid() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFormatStrftime(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{
			name: "basically works",
			want: "basically works",
		},
		{
			name: "Mon Jan 2 15:04:05",
			want: "Mon Jan 2 15:04:05",
		},
		{
			name: "python am/pm: %p",
			want: "python am/pm: PM",
		},
		{
			name: "python year: %Y",
			want: "python year: 2006",
		},
		{
			name: "python short year: %y",
			want: "python short year: 06",
		},
		{
			name: "python month: %m",
			want: "python month: 01",
		},
		{
			name: "python date: %d",
			want: "python date: 02",
		},
		{
			name: "python 24hr: %H",
			want: "python 24hr: 15",
		},
		{
			name: "python minutes: %M",
			want: "python minutes: 04",
		},
		{
			name: "python seconds: %S",
			want: "python seconds: 05",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatStrftime(tc.name); got != tc.want {
				t.Errorf("formatStrftime(%q) got %q want %q", tc.name, got, tc.want)
			}
		})
	}

}

func TestTruncateGrid(t *testing.T) {
	addRows := func(n int, skel *Cell) map[string]Cell {
		if skel == nil {
			skel = &Cell{}
		}
		out := make(map[string]Cell, n)
		for i := 0; i < n; i++ {
			skel.ID = fmt.Sprintf("row-%d", i)
			out[skel.ID] = *skel
		}
		return out
	}

	addCols := func(rows ...map[string]Cell) []InflatedColumn {
		out := make([]InflatedColumn, 0, len(rows))
		for i, cells := range rows {
			id := fmt.Sprintf("col-%d", i)
			out = append(out, InflatedColumn{
				Column: &statepb.Column{
					Name:  id,
					Build: id,
				},
				Cells: cells,
			})
		}
		return out
	}

	cases := []struct {
		name    string
		cols    []InflatedColumn
		ceiling int
		want    []InflatedColumn
	}{
		{
			name: "basically works",
		},
		{
			name: "keep first two cols",
			cols: addCols(
				addRows(1000, nil),
				addRows(1000, nil),
			),
			want: addCols(
				addRows(1000, nil),
				addRows(1000, nil),
			),
		},
		{
			name: "shrink after second column",
			cols: addCols(
				addRows(1000, nil),
				addRows(1000, nil),
				addRows(1000, nil),
				addRows(1000, nil),
				addRows(1000, nil),
			),
			want: append(
				addCols(
					addRows(1000, nil),
					addRows(1000, nil),
					truncatedCells(5000, 0, 3000, "cell"),
				),
				InflatedColumn{
					Column: &statepb.Column{},
				},
				InflatedColumn{
					Column: &statepb.Column{},
				},
			),
		},
		{
			name: "honor ceiling",
			cols: addCols(
				addRows(1000, nil),
				addRows(1000, nil),
				addRows(1000, nil),
				addRows(1, nil),
				addRows(1000, nil),
			),
			ceiling: 3001,
			want: addCols(
				addRows(1000, nil),
				addRows(1000, nil),
				addRows(1000, nil),
				addRows(1, nil),
				truncatedCells(4001, 3001, 1000, "cell"),
			),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			truncateGrid(tc.cols, tc.ceiling)
			if diff := cmp.Diff(tc.want, tc.cols, protocmp.Transform()); diff != "" {
				t.Errorf("truncateGrid() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestReprocessColumn(t *testing.T) {

	now := time.Now()

	cases := []struct {
		name string
		old  *statepb.Grid
		cfg  *configpb.TestGroup
		when time.Time
		want *InflatedColumn
	}{
		{
			name: "empty",
			old:  &statepb.Grid{},
		},
		{
			name: "same",
			old: &statepb.Grid{
				Config: &configpb.TestGroup{Name: "same"},
			},
			cfg: &configpb.TestGroup{Name: "same"},
		},
		{
			name: "changed",
			old: &statepb.Grid{
				Config: &configpb.TestGroup{Name: "old"},
			},
			cfg:  &configpb.TestGroup{Name: "new"},
			when: now,
			want: &InflatedColumn{
				Column: &statepb.Column{
					Started: float64(now.Unix() * 1000),
				},
				Cells: map[string]Cell{
					"reprocess": {
						Result: statuspb.TestStatus_RUNNING,
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := reprocessColumn(logrus.WithField("name", tc.name), tc.old, tc.cfg, tc.when)
			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("reprocessColumn() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestShrinkGrid(t *testing.T) {
	random := func(i int) string {
		b := make([]byte, i)
		if _, err := rand.Read(b); err != nil {
			t.Fatalf("rand.Read(%d): %v", i, err)
		}
		return base64.StdEncoding.EncodeToString(b)
	}
	cases := []struct {
		name    string
		ctx     context.Context
		tg      *configpb.TestGroup
		cols    []InflatedColumn
		issues  map[string][]string
		ceiling int

		want func(*configpb.TestGroup, []InflatedColumn, map[string][]string) *statepb.Grid
		err  bool
	}{
		{
			name: "basically works",
			tg:   &configpb.TestGroup{},
			want: func(*configpb.TestGroup, []InflatedColumn, map[string][]string) *statepb.Grid {
				return &statepb.Grid{}
			},
		},
		{
			name: "unchanged",
			tg:   &configpb.TestGroup{},
			cols: []InflatedColumn{
				{
					Column: &statepb.Column{
						Name:  "hi",
						Build: "there",
					},
					Cells: map[string]Cell{
						"cell": {
							Result:  statuspb.TestStatus_FAIL,
							Message: "yo",
						},
					},
				},
				{
					Column: &statepb.Column{
						Name:  "two-name",
						Build: "two-build",
					},
					Cells: map[string]Cell{
						"cell": {
							Result:  statuspb.TestStatus_FAIL,
							Message: "yo",
						},
						"two": {
							Result: statuspb.TestStatus_PASS,
							Icon:   "S",
						},
					},
				},
			},
			want: func(tg *configpb.TestGroup, cols []InflatedColumn, issues map[string][]string) *statepb.Grid {
				return constructGridFromGroupConfig(logrus.New(), tg, cols, issues)
			},
		},
		{
			name: "truncate only metrics, properties, messages",
			tg:   &configpb.TestGroup{},
			cols: []InflatedColumn{
				{
					Column: &statepb.Column{
						Name:  "hi",
						Build: "there",
					},
					Cells: map[string]Cell{
						"cell": {
							Result:  statuspb.TestStatus_FAIL,
							Message: "yo",
						},
					},
				},
				{
					Column: &statepb.Column{
						Name:  "two-name",
						Build: "two-build",
					},
					Cells: func() map[string]Cell {
						cells := map[string]Cell{}

						for i := 0; i < 10; i++ {
							cells[fmt.Sprintf("cell-%d", i)] = Cell{
								Result:  statuspb.TestStatus_FAIL,
								Message: random(10000),
								Metrics: map[string]float64{
									random(4096): 7,
								},
								Properties: map[string]string{
									"junk": random(100000),
								},
							}
						}
						return cells
					}(),
				},
			},
			ceiling: 200000,
			want: func(tg *configpb.TestGroup, origCols []InflatedColumn, issues map[string][]string) *statepb.Grid {
				logger := logrus.New()
				grid := constructGridFromGroupConfig(logger, tg, origCols, issues)
				buf, _ := gcs.MarshalGrid(grid)
				orig := len(buf)
				cols := []InflatedColumn{
					{
						Column: &statepb.Column{
							Name:  "hi",
							Build: "there",
						},
						Cells: map[string]Cell{
							"cell": {
								Result:  statuspb.TestStatus_FAIL,
								Message: "yo",
							},
						},
					},
					{
						Column: &statepb.Column{
							Name:  "two-name",
							Build: "two-build",
						},
						Cells: func() map[string]Cell {
							cells := map[string]Cell{}

							for i := 0; i < 10; i++ {
								cells[fmt.Sprintf("cell-%d", i)] = Cell{
									Result:  statuspb.TestStatus_FAIL,
									Message: "foo",
								}
							}
							stripCells(cells, orig, 100000)
							return cells
						}(),
					},
				}
				return constructGridFromGroupConfig(logger, tg, cols, issues)
			},
		},
		{
			name: "truncate row data",
			tg:   &configpb.TestGroup{},
			cols: []InflatedColumn{
				{
					Column: &statepb.Column{
						Name:  "hi",
						Build: "there",
					},
					Cells: map[string]Cell{
						"cell": {
							Result:  statuspb.TestStatus_FAIL,
							Message: "yo",
						},
					},
				},
				{
					Column: &statepb.Column{
						Name:  "two-name",
						Build: "two-build",
					},
					Cells: func() map[string]Cell {
						cells := map[string]Cell{}

						for i := 0; i < 1000; i++ {
							cells[fmt.Sprintf("cell-%d", i)] = Cell{
								Result:  statuspb.TestStatus_FAIL,
								Message: "yo",
							}
						}
						return cells
					}(),
				},
			},
			ceiling: 2000,
			want: func(tg *configpb.TestGroup, _ []InflatedColumn, issues map[string][]string) *statepb.Grid {
				cols := []InflatedColumn{
					{
						Column: &statepb.Column{
							Name:  "hi",
							Build: "there",
						},
						Cells: map[string]Cell{
							"cell": {
								Result:  statuspb.TestStatus_FAIL,
								Message: "yo",
							},
						},
					},
					{
						Column: &statepb.Column{
							Name:  "two-name",
							Build: "two-build",
						},
						Cells: func() map[string]Cell {
							cells := map[string]Cell{}

							for i := 0; i < 1000; i++ {
								cells[fmt.Sprintf("cell-%d", i)] = Cell{
									Result:  statuspb.TestStatus_FAIL,
									Message: "yo",
								}
							}
							return cells
						}(),
					},
				}
				logger := logrus.New()
				grid := constructGridFromGroupConfig(logger, tg, cols, issues)
				buf, _ := gcs.MarshalGrid(grid)
				orig := len(buf)
				cols[1].Cells = truncatedCells(orig, 1000, len(cols[1].Cells), "byte")

				return constructGridFromGroupConfig(logger, tg, cols, issues)
			},
		},
		{
			name: "truncate col and row data",
			tg:   &configpb.TestGroup{},
			cols: []InflatedColumn{
				{
					Column: &statepb.Column{
						Name:  "hi",
						Build: "there",
					},
					Cells: map[string]Cell{
						"cell": {
							Result:  statuspb.TestStatus_FAIL,
							Message: "yo",
						},
					},
				},
				{
					Column: &statepb.Column{
						Name:  "two-name",
						Build: "two-build",
					},
					Cells: func() map[string]Cell {
						cells := map[string]Cell{}

						for i := 0; i < 1000; i++ {
							cells[fmt.Sprintf("cell-%d", i)] = Cell{
								Result:  statuspb.TestStatus_FAIL,
								Message: "yo",
							}
						}
						return cells
					}(),
				},
				{
					Column: &statepb.Column{
						Name:  "three-name",
						Build: "three-build",
					},
					Cells: map[string]Cell{
						"cell": {
							Result:  statuspb.TestStatus_FAIL,
							Message: "yo",
						},
					},
				},
			},
			ceiling: 200,
			want: func(tg *configpb.TestGroup, _ []InflatedColumn, issues map[string][]string) *statepb.Grid {
				cols := []InflatedColumn{
					{
						Column: &statepb.Column{
							Name:  "hi",
							Build: "there",
						},
						Cells: map[string]Cell{
							"cell": {
								Result:  statuspb.TestStatus_FAIL,
								Message: "yo",
							},
						},
					},
					{
						Column: &statepb.Column{
							Name:  "two-name",
							Build: "two-build",
						},
						Cells: func() map[string]Cell {
							cells := map[string]Cell{}

							for i := 0; i < 1000; i++ {
								cells[fmt.Sprintf("cell-%d", i)] = Cell{
									Result:  statuspb.TestStatus_FAIL,
									Message: "yo",
								}
							}
							return cells
						}(),
					},
					{
						Column: &statepb.Column{
							Name:  "three-name",
							Build: "three-build",
						},
						Cells: map[string]Cell{
							"cell": {
								Result:  statuspb.TestStatus_FAIL,
								Message: "yo",
							},
						},
					},
				}
				logger := logrus.New()
				grid := constructGridFromGroupConfig(logger, tg, cols, issues)
				buf, _ := gcs.MarshalGrid(grid)
				orig := len(buf)
				// Shrink row data for second column
				cols[1].Cells = truncatedCells(orig, 100, len(cols[1].Cells), "byte")

				// Merge column 3 into column 2
				cols[1].Column = &statepb.Column{}
				cols[1].Cells["cell"] = cols[2].Cells["cell"]
				cols = cols[:2]

				return constructGridFromGroupConfig(logger, tg, cols, issues)
			},
		},
		{
			name: "cancelled context",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Err()
				return ctx
			}(),
			tg: &configpb.TestGroup{},
			cols: []InflatedColumn{
				{
					Column: &statepb.Column{
						Name:  "hi",
						Build: "there",
					},
					Cells: map[string]Cell{
						"cell": {
							Result:  statuspb.TestStatus_FAIL,
							Message: "yo",
						},
					},
				},
				{
					Column: &statepb.Column{
						Name:  "two-name",
						Build: "two-build",
					},
					Cells: func() map[string]Cell {
						cells := map[string]Cell{}

						for i := 0; i < 1000; i++ {
							cells[fmt.Sprintf("cell-%d", i)] = Cell{
								Result:  statuspb.TestStatus_FAIL,
								Message: "yo",
							}
						}
						return cells
					}(),
				},
				{
					Column: &statepb.Column{
						Name:  "three-name",
						Build: "three-build",
					},
					Cells: map[string]Cell{
						"cell": {
							Result:  statuspb.TestStatus_FAIL,
							Message: "yo",
						},
					},
				},
			},
			ceiling: 100,
			want: func(tg *configpb.TestGroup, cols []InflatedColumn, issues map[string][]string) *statepb.Grid {
				logger := logrus.New()
				return constructGridFromGroupConfig(logger, tg, cols, issues)
			},
		},
		{
			name: "no ceiling",
			tg:   &configpb.TestGroup{},
			cols: []InflatedColumn{
				{
					Column: &statepb.Column{
						Name:  "hi",
						Build: "there",
					},
					Cells: map[string]Cell{
						"cell": {
							Result:  statuspb.TestStatus_FAIL,
							Message: "yo",
						},
					},
				},
				{
					Column: &statepb.Column{
						Name:  "two-name",
						Build: "two-build",
					},
					Cells: func() map[string]Cell {
						cells := map[string]Cell{}

						for i := 0; i < 1000; i++ {
							cells[fmt.Sprintf("cell-%d", i)] = Cell{
								Result:  statuspb.TestStatus_FAIL,
								Message: "yo",
							}
						}
						return cells
					}(),
				},
				{
					Column: &statepb.Column{
						Name:  "three-name",
						Build: "three-build",
					},
					Cells: map[string]Cell{
						"cell": {
							Result:  statuspb.TestStatus_FAIL,
							Message: "yo",
						},
					},
				},
			},
			want: func(tg *configpb.TestGroup, cols []InflatedColumn, issues map[string][]string) *statepb.Grid {
				logger := logrus.New()
				return constructGridFromGroupConfig(logger, tg, cols, issues)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ctx == nil {
				tc.ctx = context.Background()
			}
			var want *statepb.Grid
			if tc.want != nil {
				want = tc.want(tc.tg, tc.cols, tc.issues)
			}
			got, buf, err := shrinkGrid(tc.ctx, logrus.WithField("name", tc.name), tc.tg, tc.cols, tc.issues, tc.ceiling)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("shrinkGrid() got unexpected error: %v", err)
				}
			case tc.err:
				t.Errorf("shrinkGrid() failed to get an error, got %v", got)
			default:
				if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
					t.Errorf("shrinkGrid() got unexpected grid diff (-want +got):\n%s", diff)
					return
				}
				wantBuf, err := gcs.MarshalGrid(want)
				if err != nil {
					t.Fatalf("Failed to marshal grid: %v", err)
				}
				if diff := cmp.Diff(wantBuf, buf); diff != "" {
					t.Errorf("shrinkGrid() got unexpected buf diff (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestOverrideBuild(t *testing.T) {
	cases := []struct {
		name string
		tg   *configpb.TestGroup
		cols []InflatedColumn
		want []InflatedColumn
	}{
		{
			name: "basically works",
			tg:   &configpb.TestGroup{},
		},
		{
			name: "empty override does not override",
			tg:   &configpb.TestGroup{},
			cols: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "hello",
						Started: 7,
					},
					Cells: map[string]Cell{
						"keep": {ID: "me"},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "world",
						Started: 6,
					},
				},
			},
			want: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "hello",
						Started: 7,
					},
					Cells: map[string]Cell{
						"keep": {ID: "me"},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "world",
						Started: 6,
					},
				},
			},
		},
		{
			name: "override with python style",
			tg: &configpb.TestGroup{
				BuildOverrideStrftime: "%y-%m-%d (%Y) %H:%M:%S %p",
			},
			cols: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "drop",
						Hint:    "of fruit",
						Name:    "keep",
						Started: float64(time.Date(2021, 04, 22, 13, 14, 15, 0, time.Local).Unix() * 1000),
					},
					Cells: map[string]Cell{
						"keep": {ID: "me too"},
					},
				},
			},
			want: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "21-04-22 (2021) 13:14:15 PM",
						Hint:    "of fruit",
						Name:    "keep",
						Started: float64(time.Date(2021, 04, 22, 13, 14, 15, 0, time.Local).Unix() * 1000),
					},
					Cells: map[string]Cell{
						"keep": {ID: "me too"},
					},
				},
			},
		},
		{
			name: "override with golang format",
			tg: &configpb.TestGroup{
				BuildOverrideStrftime: "hello 2006 PM",
			},
			cols: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "drop",
						Hint:    "of fruit",
						Name:    "keep",
						Started: float64(time.Date(2021, 04, 22, 13, 14, 15, 0, time.Local).Unix() * 1000),
					},
					Cells: map[string]Cell{
						"keep": {ID: "me too"},
					},
				},
			},
			want: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "hello 2021 PM",
						Hint:    "of fruit",
						Name:    "keep",
						Started: float64(time.Date(2021, 04, 22, 13, 14, 15, 0, time.Local).Unix() * 1000),
					},
					Cells: map[string]Cell{
						"keep": {ID: "me too"},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			overrideBuild(tc.tg, tc.cols)
			if diff := cmp.Diff(tc.want, tc.cols, protocmp.Transform()); diff != "" {
				t.Errorf("overrideBuild() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGroupColumns(t *testing.T) {
	cases := []struct {
		name string
		tg   *configpb.TestGroup
		cols []InflatedColumn
		want []InflatedColumn
	}{
		{
			name: "basically works",
		},
		{
			name: "single column groups do not change",
			cols: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "hello",
						Name:    "world",
						Started: 7,
					},
					Cells: map[string]Cell{
						"keep": {ID: "me"},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "another",
						Name:    "column",
						Started: 9,
					},
					Cells: map[string]Cell{
						"also": {ID: "remains"},
					},
				},
			},
			want: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "hello",
						Name:    "world",
						Started: 7,
					},
					Cells: map[string]Cell{
						"keep": {ID: "me"},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "another",
						Name:    "column",
						Started: 9,
					},
					Cells: map[string]Cell{
						"also": {ID: "remains"},
					},
				},
			},
		},
		{
			name: "group columns with the same build and name",
			cols: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "same",
						Name:    "lemming",
						Hint:    "99",
						Started: 7,
						Extra: []string{
							"first",
							"",
							"same",
							"different",
						},
					},
					Cells: map[string]Cell{
						"keep": {ID: "me"},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "same",
						Name:    "lemming",
						Hint:    "100",
						Started: 9,
						Extra: []string{
							"",
							"second",
							"same",
							"changed",
						},
					},
					Cells: map[string]Cell{
						"also": {ID: "remains"},
					},
				},
			},
			want: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "same",
						Name:    "lemming",
						Started: 7,
						Hint:    "100",
						Extra: []string{
							"first",
							"second",
							"same",
							"*",
						},
					},
					Cells: map[string]Cell{
						"keep": {ID: "me"},
						"also": {ID: "remains"},
					},
				},
			},
		},
		{
			name: "group columns with the same build and name, listing all values",
			tg: &configpb.TestGroup{
				ColumnHeader: []*configpb.TestGroup_ColumnHeader{
					{
						Property:      "",
						ListAllValues: true,
					},
					{
						Property:      "",
						ListAllValues: true,
					},
					{
						Property:      "",
						ListAllValues: true,
					},
					{
						Property:      "",
						ListAllValues: true,
					},
					{
						Property:      "",
						ListAllValues: true,
					},
				},
			},
			cols: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "same",
						Name:    "lemming",
						Hint:    "99",
						Started: 7,
						Extra: []string{
							"first",
							"",
							"same",
							"different",
							"overlap||some",
						},
					},
					Cells: map[string]Cell{
						"keep": {ID: "me"},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "same",
						Name:    "lemming",
						Hint:    "100",
						Started: 9,
						Extra: []string{
							"",
							"second",
							"same",
							"changed",
							"other||overlap",
						},
					},
					Cells: map[string]Cell{
						"also": {ID: "remains"},
					},
				},
			},
			want: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "same",
						Name:    "lemming",
						Started: 7,
						Hint:    "100",
						Extra: []string{
							"first",
							"second",
							"same",
							"changed||different",
							"other||overlap||some",
						},
					},
					Cells: map[string]Cell{
						"keep": {ID: "me"},
						"also": {ID: "remains"},
					},
				},
			},
		},
		{
			name: "columns add more headers",
			tg: &configpb.TestGroup{
				ColumnHeader: []*configpb.TestGroup_ColumnHeader{
					{
						Property: "",
					},
					{
						Property: "",
					},
					{
						Property: "",
					},
				},
			},
			cols: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "same",
						Name:    "lemming",
						Hint:    "99",
						Started: 7,
						Extra: []string{
							"one",
						},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "same",
						Name:    "lemming",
						Hint:    "100",
						Started: 9,
						Extra: []string{
							"one",
							"two",
						},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "same",
						Name:    "lemming",
						Hint:    "100",
						Started: 9,
						Extra: []string{
							"one",
							"two",
							"three",
						},
					},
				},
			},
			want: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "same",
						Name:    "lemming",
						Started: 7,
						Hint:    "100",
						Extra: []string{
							"one",
							"two",
							"three",
						},
					},
					Cells: map[string]Cell{},
				},
			},
		},
		{
			name: "columns remove headers",
			tg: &configpb.TestGroup{
				ColumnHeader: []*configpb.TestGroup_ColumnHeader{
					{
						Property: "",
					},
					{
						Property: "",
					},
					{
						Property: "",
					},
				},
			},
			cols: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "same",
						Name:    "lemming",
						Hint:    "99",
						Started: 7,
						Extra: []string{
							"one",
							"two",
							"three",
						},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "same",
						Name:    "lemming",
						Hint:    "100",
						Started: 9,
						Extra: []string{
							"one",
							"two",
						},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "same",
						Name:    "lemming",
						Hint:    "100",
						Started: 9,
						Extra: []string{
							"one",
						},
					},
				},
			},
			want: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "same",
						Name:    "lemming",
						Started: 7,
						Hint:    "100",
						Extra: []string{
							"one",
							"two",
							"three",
						},
					},
					Cells: map[string]Cell{},
				},
			},
		},
		{
			name: "do not group different builds",
			cols: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "this",
						Name:    "same",
						Started: 7,
					},
					Cells: map[string]Cell{
						"keep": {ID: "me"},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "that",
						Name:    "same",
						Started: 9,
					},
					Cells: map[string]Cell{
						"also": {ID: "remains"},
					},
				},
			},
			want: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "this",
						Name:    "same",
						Started: 7,
					},
					Cells: map[string]Cell{
						"keep": {ID: "me"},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "that",
						Name:    "same",
						Started: 9,
					},
					Cells: map[string]Cell{
						"also": {ID: "remains"},
					},
				},
			},
		},
		{
			name: "do not group different names",
			cols: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "same",
						Name:    "different",
						Started: 7,
					},
					Cells: map[string]Cell{
						"keep": {ID: "me"},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "same",
						Name:    "changed",
						Started: 9,
					},
					Cells: map[string]Cell{
						"also": {ID: "remains"},
					},
				},
			},
			want: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "same",
						Name:    "different",
						Started: 7,
					},
					Cells: map[string]Cell{
						"keep": {ID: "me"},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "same",
						Name:    "changed",
						Started: 9,
					},
					Cells: map[string]Cell{
						"also": {ID: "remains"},
					},
				},
			},
		},
		{
			name: "split merged rows with the same name",
			cols: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "same",
						Name:    "same",
						Started: 7,
					},
					Cells: map[string]Cell{
						"first": {ID: "first"},
						"same":  {ID: "first-different"},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "same",
						Name:    "same",
						Started: 9,
					},
					Cells: map[string]Cell{
						"same":   {ID: "second-changed"},
						"second": {ID: "second"},
					},
				},
			},
			want: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "same",
						Name:    "same",
						Started: 7,
					},
					Cells: map[string]Cell{
						"first":    {ID: "first"},
						"same":     {ID: "first-different"},
						"same [1]": {ID: "second-changed"},
						"second":   {ID: "second"},
					},
				},
			},
		},
		{
			name: "ignore_old_results only takes newest",
			tg: &configpb.TestGroup{
				IgnoreOldResults: true,
			},
			cols: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "same",
						Name:    "same",
						Started: 9,
					},
					Cells: map[string]Cell{
						"first": {ID: "first"},
						"same":  {ID: "first-different"},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "same",
						Name:    "same",
						Started: 7,
					},
					Cells: map[string]Cell{
						"same":   {ID: "second-changed"},
						"second": {ID: "second"},
					},
				},
			},
			want: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "same",
						Name:    "same",
						Started: 7,
					},
					Cells: map[string]Cell{
						"first":  {ID: "first"},
						"same":   {ID: "first-different"},
						"second": {ID: "second"},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tg := tc.tg
			if tg == nil {
				tg = &configpb.TestGroup{}
			}
			got := groupColumns(tg, tc.cols)
			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("groupColumns() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

// TODO(amerai): This is kind of, but not quite, redundant with summary.gridMetrics().
func TestColumnStats(t *testing.T) {
	passCell := Cell{Result: statuspb.TestStatus_PASS}
	failCell := Cell{Result: statuspb.TestStatus_FAIL}

	cases := []struct {
		name            string
		cells           map[string]Cell
		brokenThreshold float32
		want            *statepb.Stats
	}{
		{
			name:            "nil",
			brokenThreshold: 0.5,
			want:            nil,
		},
		{
			name:            "empty",
			brokenThreshold: 0.5,
			cells:           map[string]Cell{},
			want:            &statepb.Stats{},
		},
		{
			name: "nil, no threshold",
			want: nil,
		},
		{
			name:  "empty, no threshold",
			cells: map[string]Cell{},
			want:  nil,
		},
		{
			name: "no threshold",
			cells: map[string]Cell{
				"a": passCell,
				"b": failCell,
			},
			want: nil,
		},
		{
			name:            "blank",
			brokenThreshold: 0.5,
			cells: map[string]Cell{
				"a": emptyCell,
				"b": emptyCell,
				"c": emptyCell,
				"d": emptyCell,
			},
			want: &statepb.Stats{
				PassCount:  0,
				FailCount:  0,
				TotalCount: 0,
			},
		},
		{
			name:            "passing",
			brokenThreshold: 0.5,
			cells: map[string]Cell{
				"a": passCell,
				"b": passCell,
				"c": passCell,
				"d": passCell,
			},
			want: &statepb.Stats{
				PassCount:  4,
				FailCount:  0,
				TotalCount: 4,
			},
		},
		{
			name:            "failing",
			brokenThreshold: 0.5,
			cells: map[string]Cell{
				"a": failCell,
				"b": failCell,
				"c": failCell,
				"d": failCell,
			},
			want: &statepb.Stats{
				PassCount:  0,
				FailCount:  4,
				TotalCount: 4,
				Broken:     true,
			},
		},
		{
			name:            "mix, not broken",
			brokenThreshold: 0.5,
			cells: map[string]Cell{
				"a": passCell,
				"b": passCell,
				"c": failCell,
				"d": passCell,
			},
			want: &statepb.Stats{
				PassCount:  3,
				FailCount:  1,
				TotalCount: 4,
			},
		},
		{
			name:            "mix, broken",
			brokenThreshold: 0.5,
			cells: map[string]Cell{
				"a": failCell,
				"b": passCell,
				"c": failCell,
				"d": failCell,
			},
			want: &statepb.Stats{
				PassCount:  1,
				FailCount:  3,
				TotalCount: 4,
				Broken:     true,
			},
		},
		{
			name:            "mix, blank cells",
			brokenThreshold: 0.5,
			cells: map[string]Cell{
				"a": failCell,
				"b": passCell,
				"c": emptyCell,
				"d": emptyCell,
				"e": failCell,
				"f": passCell,
				"g": failCell,
				"h": emptyCell,
			},
			want: &statepb.Stats{
				PassCount:  2,
				FailCount:  3,
				TotalCount: 5,
				Broken:     true,
			},
		},
		{
			name:            "pending",
			brokenThreshold: 0.5,
			cells: map[string]Cell{
				"a": failCell,
				"b": passCell,
				"c": passCell,
				"d": {Result: statuspb.TestStatus_RUNNING},
			},
			want: &statepb.Stats{
				PassCount:  2,
				FailCount:  1,
				TotalCount: 4,
				Pending:    true,
			},
		},
		{
			name:            "advanced",
			brokenThreshold: 0.5,
			cells: map[string]Cell{
				"a": failCell,
				"b": passCell,
				"c": emptyCell,
				"d": {Result: statuspb.TestStatus_BLOCKED},
				"e": {Result: statuspb.TestStatus_BUILD_FAIL},
				"f": {Result: statuspb.TestStatus_BUILD_PASSED},
				"g": {Result: statuspb.TestStatus_CANCEL},
				"h": {Result: statuspb.TestStatus_CATEGORIZED_ABORT},
				"i": {Result: statuspb.TestStatus_CATEGORIZED_FAIL},
				"j": {Result: statuspb.TestStatus_FLAKY},
				"k": {Result: statuspb.TestStatus_PASS_WITH_ERRORS},
				"l": {Result: statuspb.TestStatus_PASS_WITH_SKIPS},
				"m": {Result: statuspb.TestStatus_TIMED_OUT},
				"n": {Result: statuspb.TestStatus_TOOL_FAIL},
				"o": {Result: statuspb.TestStatus_UNKNOWN},
				"p": {Result: statuspb.TestStatus_RUNNING},
			},
			want: &statepb.Stats{
				PassCount:  4,
				FailCount:  5,
				TotalCount: 15,
				Pending:    true,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := columnStats(tc.cells, tc.brokenThreshold)
			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("columnStats(%v, %f) got unexpected diff (-want +got):\n%s", tc.cells, tc.brokenThreshold, diff)
			}
		})
	}
}

func TestConstructGrid(t *testing.T) {
	cases := []struct {
		name                    string
		cols                    []inflatedColumn
		numFailuresToAlert      int
		numPassesToDisableAlert int
		issues                  map[string][]string
		brokenThreshold         float32
		expected                *statepb.Grid
	}{
		{
			name:     "basically works",
			expected: &statepb.Grid{},
		},
		{
			name: "multiple columns",
			cols: []inflatedColumn{
				{
					Column: &statepb.Column{Build: "15"},
					Cells: map[string]cell{
						"green": {
							Result: statuspb.TestStatus_PASS,
						},
						"red": {
							Result: statuspb.TestStatus_FAIL,
						},
						"only-15": {
							Result: statuspb.TestStatus_FLAKY,
						},
					},
				},
				{
					Column: &statepb.Column{Build: "10"},
					Cells: map[string]cell{
						"full": {
							Result:  statuspb.TestStatus_PASS,
							CellID:  "cell",
							Icon:    "icon",
							Message: "message",
							Metrics: map[string]float64{
								"elapsed": 1,
								"keys":    2,
							},
							UserProperty: "food",
						},
						"green": {
							Result: statuspb.TestStatus_PASS,
						},
						"red": {
							Result: statuspb.TestStatus_FAIL,
						},
						"only-10": {
							Result: statuspb.TestStatus_FLAKY,
						},
					},
				},
			},
			expected: &statepb.Grid{
				Columns: []*statepb.Column{
					{Build: "15"},
					{Build: "10"},
				},
				Rows: []*statepb.Row{
					setupRow(
						&statepb.Row{
							Name:         "full",
							Id:           "full",
							UserProperty: []string{},
						},
						emptyCell,
						cell{
							Result:  statuspb.TestStatus_PASS,
							CellID:  "cell",
							Icon:    "icon",
							Message: "message",
							Metrics: map[string]float64{
								"elapsed": 1,
								"keys":    2,
							},
							UserProperty: "food",
						},
					),
					setupRow(
						&statepb.Row{
							Name: "green",
							Id:   "green",
						},
						cell{Result: statuspb.TestStatus_PASS},
						cell{Result: statuspb.TestStatus_PASS},
					),
					setupRow(
						&statepb.Row{
							Name: "only-10",
							Id:   "only-10",
						},
						emptyCell,
						cell{Result: statuspb.TestStatus_FLAKY},
					),
					setupRow(
						&statepb.Row{
							Name: "only-15",
							Id:   "only-15",
						},
						cell{Result: statuspb.TestStatus_FLAKY},
						emptyCell,
					),
					setupRow(
						&statepb.Row{
							Name: "red",
							Id:   "red",
						},
						cell{Result: statuspb.TestStatus_FAIL},
						cell{Result: statuspb.TestStatus_FAIL},
					),
				},
			},
		},
		{
			name:            "multiple columns with threshold",
			brokenThreshold: 0.3,
			cols: []inflatedColumn{
				{
					Column: &statepb.Column{Build: "15"},
					Cells: map[string]cell{
						"green": {
							Result: statuspb.TestStatus_PASS,
						},
						"red": {
							Result: statuspb.TestStatus_FAIL,
						},
						"only-15": {
							Result: statuspb.TestStatus_FLAKY,
						},
					},
				},
				{
					Column: &statepb.Column{Build: "10"},
					Cells: map[string]cell{
						"full": {
							Result:  statuspb.TestStatus_PASS,
							CellID:  "cell",
							Icon:    "icon",
							Message: "message",
							Metrics: map[string]float64{
								"elapsed": 1,
								"keys":    2,
							},
							UserProperty: "food",
						},
						"green": {
							Result: statuspb.TestStatus_PASS,
						},
						"red": {
							Result: statuspb.TestStatus_FAIL,
						},
						"only-10": {
							Result: statuspb.TestStatus_FLAKY,
						},
					},
				},
			},
			expected: &statepb.Grid{
				Columns: []*statepb.Column{
					{
						Build: "15",
						Stats: &statepb.Stats{
							FailCount:  1,
							PassCount:  1,
							TotalCount: 3,
							Broken:     true,
						},
					},
					{
						Build: "10",
						Stats: &statepb.Stats{
							FailCount:  1,
							PassCount:  2,
							TotalCount: 4,
						},
					},
				},
				Rows: []*statepb.Row{
					setupRow(
						&statepb.Row{
							Name:         "full",
							Id:           "full",
							UserProperty: []string{},
						},
						emptyCell,
						cell{
							Result:  statuspb.TestStatus_PASS,
							CellID:  "cell",
							Icon:    "icon",
							Message: "message",
							Metrics: map[string]float64{
								"elapsed": 1,
								"keys":    2,
							},
							UserProperty: "food",
						},
					),
					setupRow(
						&statepb.Row{
							Name: "green",
							Id:   "green",
						},
						cell{Result: statuspb.TestStatus_PASS},
						cell{Result: statuspb.TestStatus_PASS},
					),
					setupRow(
						&statepb.Row{
							Name: "only-10",
							Id:   "only-10",
						},
						emptyCell,
						cell{Result: statuspb.TestStatus_FLAKY},
					),
					setupRow(
						&statepb.Row{
							Name: "only-15",
							Id:   "only-15",
						},
						cell{Result: statuspb.TestStatus_FLAKY},
						emptyCell,
					),
					setupRow(
						&statepb.Row{
							Name: "red",
							Id:   "red",
						},
						cell{Result: statuspb.TestStatus_FAIL},
						cell{Result: statuspb.TestStatus_FAIL},
					),
				},
			},
		},
		{
			name:                    "open alert",
			numFailuresToAlert:      2,
			numPassesToDisableAlert: 2,
			cols: []inflatedColumn{
				{
					Column: &statepb.Column{Build: "4"},
					Cells: map[string]cell{
						"just-flaky": {
							Result: statuspb.TestStatus_FAIL,
						},
						"broken": {
							Result: statuspb.TestStatus_FAIL,
						},
					},
				},
				{
					Column: &statepb.Column{Build: "3"},
					Cells: map[string]cell{
						"just-flaky": {
							Result: statuspb.TestStatus_PASS,
						},
						"broken": {
							Result: statuspb.TestStatus_FAIL,
						},
					},
				},
			},
			expected: &statepb.Grid{
				Columns: []*statepb.Column{
					{Build: "4"},
					{Build: "3"},
				},
				Rows: []*statepb.Row{
					setupRow(
						&statepb.Row{
							Name: "broken",
							Id:   "broken",
						},
						cell{Result: statuspb.TestStatus_FAIL},
						cell{Result: statuspb.TestStatus_FAIL},
					),
					setupRow(
						&statepb.Row{
							Name: "just-flaky",
							Id:   "just-flaky",
						},
						cell{Result: statuspb.TestStatus_FAIL},
						cell{Result: statuspb.TestStatus_PASS},
					),
				},
			},
		},
		{
			name:                    "close alert",
			numFailuresToAlert:      1,
			numPassesToDisableAlert: 2,
			cols: []inflatedColumn{
				{
					Column: &statepb.Column{Build: "4"},
					Cells: map[string]cell{
						"still-broken": {
							Result: statuspb.TestStatus_PASS,
						},
						"fixed": {
							Result: statuspb.TestStatus_PASS,
						},
					},
				},
				{
					Column: &statepb.Column{Build: "3"},
					Cells: map[string]cell{
						"still-broken": {
							Result: statuspb.TestStatus_FAIL,
						},
						"fixed": {
							Result: statuspb.TestStatus_PASS,
						},
					},
				},
				{
					Column: &statepb.Column{Build: "2"},
					Cells: map[string]cell{
						"still-broken": {
							Result: statuspb.TestStatus_FAIL,
						},
						"fixed": {
							Result: statuspb.TestStatus_FAIL,
						},
					},
				},
				{
					Column: &statepb.Column{Build: "1"},
					Cells: map[string]cell{
						"still-broken": {
							Result: statuspb.TestStatus_FAIL,
						},
						"fixed": {
							Result: statuspb.TestStatus_FAIL,
						},
					},
				},
			},
			expected: &statepb.Grid{
				Columns: []*statepb.Column{
					{Build: "4"},
					{Build: "3"},
					{Build: "2"},
					{Build: "1"},
				},
				Rows: []*statepb.Row{
					setupRow(
						&statepb.Row{
							Name: "fixed",
							Id:   "fixed",
						},
						cell{Result: statuspb.TestStatus_PASS},
						cell{Result: statuspb.TestStatus_PASS},
						cell{Result: statuspb.TestStatus_FAIL},
						cell{Result: statuspb.TestStatus_FAIL},
					),
					setupRow(
						&statepb.Row{
							Name: "still-broken",
							Id:   "still-broken",
						},
						cell{Result: statuspb.TestStatus_PASS},
						cell{Result: statuspb.TestStatus_FAIL},
						cell{Result: statuspb.TestStatus_FAIL},
						cell{Result: statuspb.TestStatus_FAIL},
					),
				},
			},
		},
		{
			name: "issues",
			cols: []inflatedColumn{
				{
					Column: &statepb.Column{Build: "15"},
					Cells: map[string]cell{
						"row": {
							Result: statuspb.TestStatus_PASS,
							Issues: []string{
								"from-cell-15",
								"should-deduplicate-from-both",
								"should-deduplicate-from-row",
								"should-deduplicate-from-cell",
								"should-deduplicate-from-cell",
							},
						},
					},
				},
				{
					Column: &statepb.Column{Build: "10"},
					Cells: map[string]cell{
						"row": {
							Result: statuspb.TestStatus_PASS,
							Issues: []string{
								"from-cell-10",
								"should-deduplicate-from-row",
							},
						},
						"other": {
							Result: statuspb.TestStatus_PASS,
							Issues: []string{"fun"},
						},
						"sort": {
							Result: statuspb.TestStatus_PASS,
							Issues: []string{
								"3-is-second",
								"100-is-last",
								"2-is-first",
							},
						},
					},
				},
			},
			issues: map[string][]string{
				"row": {
					"from-argument",
					"should-deduplicate-from-arg",
					"should-deduplicate-from-arg",
					"should-deduplicate-from-both",
				},
			},
			expected: &statepb.Grid{
				Columns: []*statepb.Column{
					{Build: "15"},
					{Build: "10"},
				},
				Rows: []*statepb.Row{
					setupRow(
						&statepb.Row{
							Name:   "other",
							Id:     "other",
							Issues: []string{"fun"},
						},
						cell{Result: statuspb.TestStatus_NO_RESULT},
						cell{Result: statuspb.TestStatus_PASS},
					),
					setupRow(
						&statepb.Row{
							Name: "row",
							Id:   "row",
							Issues: []string{
								"should-deduplicate-from-row",
								"should-deduplicate-from-cell",
								"should-deduplicate-from-both",
								"should-deduplicate-from-arg",
								"from-cell-15",
								"from-cell-10",
								"from-argument",
							},
						},
						cell{Result: statuspb.TestStatus_PASS},
						cell{Result: statuspb.TestStatus_PASS},
					),
					setupRow(
						&statepb.Row{
							Name: "sort",
							Id:   "sort",
							Issues: []string{
								"100-is-last",
								"3-is-second",
								"2-is-first",
							},
						},
						cell{Result: statuspb.TestStatus_NO_RESULT},
						cell{Result: statuspb.TestStatus_PASS},
					),
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := ConstructGrid(logrus.WithField("name", tc.name), tc.cols, tc.issues, tc.numFailuresToAlert, tc.numPassesToDisableAlert, true, "userProperty", tc.brokenThreshold)
			alertRows(tc.expected.Columns, tc.expected.Rows, tc.numFailuresToAlert, tc.numPassesToDisableAlert, true, "userProperty")
			for _, row := range tc.expected.Rows {
				sort.SliceStable(row.Metric, func(i, j int) bool {
					return sortorder.NaturalLess(row.Metric[i], row.Metric[j])
				})
				sort.SliceStable(row.Metrics, func(i, j int) bool {
					return sortorder.NaturalLess(row.Metrics[i].Name, row.Metrics[j].Name)
				})
			}
			if diff := cmp.Diff(tc.expected, actual, protocmp.Transform()); diff != "" {
				t.Errorf("ConstructGrid() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestAppendMetric(t *testing.T) {
	cases := []struct {
		name     string
		metric   *statepb.Metric
		idx      int32
		value    float64
		expected *statepb.Metric
	}{
		{
			name:   "basically works",
			metric: &statepb.Metric{},
			expected: &statepb.Metric{
				Indices: []int32{0, 1},
				Values:  []float64{0},
			},
		},
		{
			name:   "start metric at random column",
			metric: &statepb.Metric{},
			idx:    7,
			value:  11,
			expected: &statepb.Metric{
				Indices: []int32{7, 1},
				Values:  []float64{11},
			},
		},
		{
			name: "continue existing series",
			metric: &statepb.Metric{
				Indices: []int32{6, 2},
				Values:  []float64{6.1, 6.2},
			},
			idx:   8,
			value: 88,
			expected: &statepb.Metric{
				Indices: []int32{6, 3},
				Values:  []float64{6.1, 6.2, 88},
			},
		},
		{
			name: "start new series",
			metric: &statepb.Metric{
				Indices: []int32{3, 2},
				Values:  []float64{6.1, 6.2},
			},
			idx:   8,
			value: 88,
			expected: &statepb.Metric{
				Indices: []int32{3, 2, 8, 1},
				Values:  []float64{6.1, 6.2, 88},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			appendMetric(tc.metric, tc.idx, tc.value)
			if diff := cmp.Diff(tc.metric, tc.expected, protocmp.Transform()); diff != "" {
				t.Errorf("appendMetric() got unexpected diff (-got +want):\n%s", diff)
			}
		})
	}
}

func TestAppendCell(t *testing.T) {
	cases := []struct {
		name  string
		row   *statepb.Row
		cell  cell
		start int
		count int

		expected *statepb.Row
	}{
		{
			name: "basically works",
			row:  &statepb.Row{},
			expected: &statepb.Row{
				Results: []int32{0, 0},
			},
		},
		{
			name: "first result",
			row:  &statepb.Row{},
			cell: cell{
				Result: statuspb.TestStatus_PASS,
			},
			count: 1,
			expected: &statepb.Row{
				Results:      []int32{int32(statuspb.TestStatus_PASS), 1},
				CellIds:      []string{""},
				Messages:     []string{""},
				Icons:        []string{""},
				UserProperty: []string{""},
				Properties:   []*statepb.Property{{}},
			},
		},
		{
			name: "all fields filled",
			row:  &statepb.Row{},
			cell: cell{
				Result:  statuspb.TestStatus_PASS,
				CellID:  "cell-id",
				Message: "hi",
				Icon:    "there",
				Metrics: map[string]float64{
					"pi":     3.14,
					"golden": 1.618,
				},
				UserProperty: "hello",
				Properties: map[string]string{
					"workflow-id":   "run-1",
					"workflow-name": "//workflow-a",
				},
			},
			count: 1,
			expected: &statepb.Row{
				Results:  []int32{int32(statuspb.TestStatus_PASS), 1},
				CellIds:  []string{"cell-id"},
				Messages: []string{"hi"},
				Icons:    []string{"there"},
				Metric: []string{
					"golden",
					"pi",
				},
				Metrics: []*statepb.Metric{
					{
						Name:    "pi",
						Indices: []int32{0, 1},
						Values:  []float64{3.14},
					},
					{
						Name:    "golden",
						Indices: []int32{0, 1},
						Values:  []float64{1.618},
					},
				},
				UserProperty: []string{"hello"},
				Properties: []*statepb.Property{{
					Property: map[string]string{
						"workflow-id":   "run-1",
						"workflow-name": "//workflow-a",
					},
				}},
			},
		},
		{
			name: "append same result",
			row: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FLAKY), 3,
				},
				CellIds:      []string{"", "", ""},
				Messages:     []string{"", "", ""},
				Icons:        []string{"", "", ""},
				UserProperty: []string{"", "", ""},
				Properties:   []*statepb.Property{{}, {}, {}},
			},
			cell: cell{
				Result:       statuspb.TestStatus_FLAKY,
				Message:      "echo",
				CellID:       "again and",
				Icon:         "keeps going",
				UserProperty: "more more",
				Properties: map[string]string{
					"workflow-id":   "run-1",
					"workflow-name": "//workflow-a",
				},
			},
			count: 2,
			expected: &statepb.Row{
				Results:      []int32{int32(statuspb.TestStatus_FLAKY), 5},
				CellIds:      []string{"", "", "", "again and", "again and"},
				Messages:     []string{"", "", "", "echo", "echo"},
				Icons:        []string{"", "", "", "keeps going", "keeps going"},
				UserProperty: []string{"", "", "", "more more", "more more"},
				Properties: []*statepb.Property{
					{},
					{},
					{},
					{
						Property: map[string]string{
							"workflow-id":   "run-1",
							"workflow-name": "//workflow-a",
						},
					},
					{
						Property: map[string]string{
							"workflow-id":   "run-1",
							"workflow-name": "//workflow-a",
						},
					},
				},
			},
		},
		{
			name: "append different result",
			row: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FLAKY), 3,
				},
				CellIds:      []string{"", "", ""},
				Messages:     []string{"", "", ""},
				Icons:        []string{"", "", ""},
				UserProperty: []string{"", "", ""},
				Properties:   []*statepb.Property{{}, {}, {}},
			},
			cell: cell{
				Result: statuspb.TestStatus_PASS,
			},
			count: 2,
			expected: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FLAKY), 3,
					int32(statuspb.TestStatus_PASS), 2,
				},
				CellIds:      []string{"", "", "", "", ""},
				Messages:     []string{"", "", "", "", ""},
				Icons:        []string{"", "", "", "", ""},
				UserProperty: []string{"", "", "", "", ""},
				Properties:   []*statepb.Property{{}, {}, {}, {}, {}},
			},
		},
		{
			name: "append no Result (results, no cellIDs, messages or icons)",
			row: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FLAKY), 3,
				},
				CellIds:      []string{"", "", ""},
				Messages:     []string{"", "", ""},
				Icons:        []string{"", "", ""},
				UserProperty: []string{"", "", ""},
				Properties:   []*statepb.Property{{}, {}, {}},
			},
			cell: cell{
				Result: statuspb.TestStatus_NO_RESULT,
			},
			count: 2,
			expected: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FLAKY), 3,
					int32(statuspb.TestStatus_NO_RESULT), 2,
				},
				CellIds:      []string{"", "", ""},
				Messages:     []string{"", "", ""},
				Icons:        []string{"", "", ""},
				UserProperty: []string{"", "", ""},
				Properties:   []*statepb.Property{{}, {}, {}},
			},
		},
		{
			name: "add metric to series",
			row: &statepb.Row{
				Results:      []int32{int32(statuspb.TestStatus_PASS), 5},
				CellIds:      []string{"", "", "", "", "c"},
				Messages:     []string{"", "", "", "", "m"},
				Icons:        []string{"", "", "", "", "i"},
				UserProperty: []string{"", "", "", "", "up"},
				Properties:   []*statepb.Property{{}, {}, {}, {}, {}},
				Metric: []string{
					"continued-series",
					"new-series",
				},
				Metrics: []*statepb.Metric{
					{
						Name:    "continued-series",
						Indices: []int32{0, 5},
						Values:  []float64{0, 1, 2, 3, 4},
					},
					{
						Name:    "new-series",
						Indices: []int32{2, 2},
						Values:  []float64{2, 3},
					},
				},
			},
			cell: cell{
				Result: statuspb.TestStatus_PASS,
				Metrics: map[string]float64{
					"continued-series":  5.1,
					"new-series":        5.2,
					"additional-metric": 5.3,
				},
			},
			start: 5,
			count: 1,
			expected: &statepb.Row{
				Results:      []int32{int32(statuspb.TestStatus_PASS), 6},
				CellIds:      []string{"", "", "", "", "c", ""},
				Messages:     []string{"", "", "", "", "m", ""},
				Icons:        []string{"", "", "", "", "i", ""},
				UserProperty: []string{"", "", "", "", "up", ""},
				Properties:   []*statepb.Property{{}, {}, {}, {}, {}, {}},
				Metric: []string{
					"continued-series",
					"new-series",
					"additional-metric",
				},
				Metrics: []*statepb.Metric{
					{
						Name:    "continued-series",
						Indices: []int32{0, 6},
						Values:  []float64{0, 1, 2, 3, 4, 5.1},
					},
					{
						Name:    "new-series",
						Indices: []int32{2, 2, 5, 1},
						Values:  []float64{2, 3, 5.2},
					},
					{
						Name:    "additional-metric",
						Indices: []int32{5, 1},
						Values:  []float64{5.3},
					},
				},
			},
		},
		{
			name:  "add a bunch of initial blank columns (eg a deleted row)",
			row:   &statepb.Row{},
			cell:  emptyCell,
			count: 7,
			expected: &statepb.Row{
				Results: []int32{int32(statuspb.TestStatus_NO_RESULT), 7},
			},
		},
		{
			name:  "issues",
			row:   &statepb.Row{},
			count: 395,
			cell: Cell{
				Issues: []string{"problematic", "state"},
			},
			expected: &statepb.Row{
				Results: []int32{int32(statuspb.TestStatus_NO_RESULT), 395},
				Issues:  []string{"problematic", "state"},
			},
		},
		{
			name: "append to group delimiter",
			row: &statepb.Row{
				Name: "test1@TESTGRID@something",
				Results: []int32{
					int32(statuspb.TestStatus_PASS), 1,
				},
				Messages:     []string{""},
				Icons:        []string{""},
				UserProperty: []string{""},
			},
			cell: cell{
				Result: statuspb.TestStatus_PASS,
				CellID: "cell-id-1",
				Properties: map[string]string{
					"workflow-id":   "run-1",
					"workflow-name": "//workflow-a",
				},
			},
			count: 1,
			expected: &statepb.Row{
				Name: "test1@TESTGRID@something",
				Results: []int32{
					int32(statuspb.TestStatus_PASS), 2,
				},
				Messages:     []string{"", ""},
				Icons:        []string{"", ""},
				UserProperty: []string{"", ""},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			appendCell(tc.row, tc.cell, tc.start, tc.count)
			sort.SliceStable(tc.row.Metric, func(i, j int) bool {
				return tc.row.Metric[i] < tc.row.Metric[j]
			})
			sort.SliceStable(tc.row.Metrics, func(i, j int) bool {
				return tc.row.Metrics[i].Name < tc.row.Metrics[j].Name
			})
			sort.SliceStable(tc.expected.Metric, func(i, j int) bool {
				return tc.expected.Metric[i] < tc.expected.Metric[j]
			})
			sort.SliceStable(tc.expected.Metrics, func(i, j int) bool {
				return tc.expected.Metrics[i].Name < tc.expected.Metrics[j].Name
			})
			if diff := cmp.Diff(tc.row, tc.expected, protocmp.Transform()); diff != "" {
				t.Errorf("appendCell() got unexpected diff (-got +want):\n%s", diff)
			}
		})
	}
}

// setupRow appends cells to the row.
//
// Auto-drops UserProperty if row.UserProperty == nil (set to empty to preserve).
func setupRow(row *statepb.Row, cells ...cell) *statepb.Row {
	dropUserPropety := row.UserProperty == nil
	for idx, c := range cells {
		appendCell(row, c, idx, 1)
	}
	if dropUserPropety {
		row.UserProperty = nil
	}

	return row
}

func TestAppendColumn(t *testing.T) {
	cases := []struct {
		name     string
		grid     *statepb.Grid
		col      inflatedColumn
		expected *statepb.Grid
	}{
		{
			name: "append first column",
			grid: &statepb.Grid{},
			col:  inflatedColumn{Column: &statepb.Column{Build: "10"}},
			expected: &statepb.Grid{
				Columns: []*statepb.Column{
					{Build: "10"},
				},
			},
		},
		{
			name: "append additional column",
			grid: &statepb.Grid{
				Columns: []*statepb.Column{
					{Build: "10"},
					{Build: "11"},
				},
			},
			col: inflatedColumn{Column: &statepb.Column{Build: "20"}},
			expected: &statepb.Grid{
				Columns: []*statepb.Column{
					{Build: "10"},
					{Build: "11"},
					{Build: "20"},
				},
			},
		},
		{
			name: "add rows to first column",
			grid: &statepb.Grid{},
			col: inflatedColumn{
				Column: &statepb.Column{Build: "10"},
				Cells: map[string]cell{
					"hello": {
						Result: statuspb.TestStatus_PASS,
						CellID: "yes",
						Metrics: map[string]float64{
							"answer": 42,
						},
					},
					"world": {
						Result:       statuspb.TestStatus_FAIL,
						Message:      "boom",
						Icon:         "X",
						UserProperty: "prop",
					},
				},
			},
			expected: &statepb.Grid{
				Columns: []*statepb.Column{
					{Build: "10"},
				},
				Rows: []*statepb.Row{
					setupRow(
						&statepb.Row{
							Name:         "hello",
							Id:           "hello",
							UserProperty: []string{},
						},
						cell{
							Result:  statuspb.TestStatus_PASS,
							CellID:  "yes",
							Metrics: map[string]float64{"answer": 42},
						}),
					setupRow(
						&statepb.Row{
							Name:         "world",
							Id:           "world",
							UserProperty: []string{},
						},
						cell{
							Result:       statuspb.TestStatus_FAIL,
							Message:      "boom",
							Icon:         "X",
							UserProperty: "prop",
						},
					),
				},
			},
		},
		{
			name: "add empty cells",
			grid: &statepb.Grid{
				Columns: []*statepb.Column{
					{Build: "10"},
					{Build: "11"},
					{Build: "12"},
				},
				Rows: []*statepb.Row{
					setupRow(
						&statepb.Row{
							Name:         "deleted",
							UserProperty: []string{},
						},
						cell{Result: statuspb.TestStatus_PASS},
						cell{Result: statuspb.TestStatus_PASS},
						cell{Result: statuspb.TestStatus_PASS},
					),
					setupRow(
						&statepb.Row{
							Name:         "always",
							UserProperty: []string{},
						},
						cell{Result: statuspb.TestStatus_PASS},
						cell{Result: statuspb.TestStatus_PASS},
						cell{Result: statuspb.TestStatus_PASS},
					),
				},
			},
			col: inflatedColumn{
				Column: &statepb.Column{Build: "20"},
				Cells: map[string]cell{
					"always": {Result: statuspb.TestStatus_PASS},
					"new":    {Result: statuspb.TestStatus_PASS},
				},
			},
			expected: &statepb.Grid{
				Columns: []*statepb.Column{
					{Build: "10"},
					{Build: "11"},
					{Build: "12"},
					{Build: "20"},
				},
				Rows: []*statepb.Row{
					setupRow(
						&statepb.Row{
							Name:         "deleted",
							UserProperty: []string{},
						},
						cell{Result: statuspb.TestStatus_PASS},
						cell{Result: statuspb.TestStatus_PASS},
						cell{Result: statuspb.TestStatus_PASS},
						emptyCell,
					),
					setupRow(
						&statepb.Row{
							Name:         "always",
							UserProperty: []string{},
						},
						cell{Result: statuspb.TestStatus_PASS},
						cell{Result: statuspb.TestStatus_PASS},
						cell{Result: statuspb.TestStatus_PASS},
						cell{Result: statuspb.TestStatus_PASS},
					),
					setupRow(
						&statepb.Row{
							Name:         "new",
							Id:           "new",
							UserProperty: []string{},
						},
						emptyCell,
						emptyCell,
						emptyCell,
						cell{Result: statuspb.TestStatus_PASS},
					),
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := map[string]*statepb.Row{}
			for _, r := range tc.grid.Rows {
				rows[r.Name] = r
			}
			AppendColumn(tc.grid, rows, tc.col)
			sort.SliceStable(tc.grid.Rows, func(i, j int) bool {
				return tc.grid.Rows[i].Name < tc.grid.Rows[j].Name
			})
			sort.SliceStable(tc.expected.Rows, func(i, j int) bool {
				return tc.expected.Rows[i].Name < tc.expected.Rows[j].Name
			})
			if diff := cmp.Diff(&tc.expected, &tc.grid, protocmp.Transform()); diff != "" {
				t.Errorf("appendColumn() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDynamicEmails(t *testing.T) {
	columnWithEmails := statepb.Column{Build: "columnWithEmail", Started: 100 - float64(0), EmailAddresses: []string{"email1@", "email2@"}}
	anotherColumnWithEmails := statepb.Column{Build: "anotherColumnWithEmails", Started: 100 - float64(1), EmailAddresses: []string{"email3@", "email2@"}}
	cases := []struct {
		name     string
		row      *statepb.Row
		columns  []*statepb.Column
		expected *statepb.AlertInfo
	}{
		{
			name: "first column with dynamic emails",
			row: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FAIL), 1,
				},
				Messages: []string{""},
				CellIds:  []string{""},
			},
			columns:  []*statepb.Column{&columnWithEmails},
			expected: alertInfo(1, "", "", "", nil, &columnWithEmails, &columnWithEmails, nil, false),
		},
		{
			name: "two column with dynamic emails, we get only the first one",
			row: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FAIL), 2,
				},
				Messages: []string{"", ""},
				CellIds:  []string{"", ""},
			},
			columns:  []*statepb.Column{&anotherColumnWithEmails, &columnWithEmails},
			expected: alertInfo(2, "", "", "", nil, &columnWithEmails, &anotherColumnWithEmails, nil, false),
		},
		{
			name: "first column don't have results, second column emails on the alert",
			row: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_NO_RESULT), 1,
					int32(statuspb.TestStatus_FAIL), 1,
				},
				Messages: []string{"", ""},
				CellIds:  []string{"", ""},
			},
			columns:  []*statepb.Column{&columnWithEmails, &anotherColumnWithEmails},
			expected: alertInfo(1, "", "", "", nil, &anotherColumnWithEmails, &anotherColumnWithEmails, nil, false),
		},
	}
	for _, tc := range cases {
		actual := alertRow(tc.columns, tc.row, 1, 1, false, "")
		if diff := cmp.Diff(tc.expected, actual, protocmp.Transform()); diff != "" {
			t.Errorf("alertRow() not as expected (-want, +got): %s", diff)
		}
	}
}

func TestAlertRow(t *testing.T) {
	var columns []*statepb.Column
	for i, id := range []string{"a", "b", "c", "d", "e", "f"} {
		columns = append(columns, &statepb.Column{
			Build:   id,
			Started: 100 - float64(i),
		})
	}
	cases := []struct {
		name      string
		row       *statepb.Row
		failOpen  int
		passClose int
		property  string
		expected  *statepb.AlertInfo
	}{
		{
			name: "never alert by default",
			row: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FAIL), 6,
				},
			},
		},
		{
			name: "passes do not alert",
			row: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_PASS), 6,
				},
			},
			failOpen:  1,
			passClose: 3,
		},
		{
			name: "flakes do not alert",
			row: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FLAKY), 6,
				},
			},
			failOpen: 1,
		},
		{
			name: "intermittent failures do not alert",
			row: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FAIL), 2,
					int32(statuspb.TestStatus_PASS), 1,
					int32(statuspb.TestStatus_FAIL), 2,
				},
			},
			failOpen: 3,
		},
		{
			name: "new failures alert",
			row: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FAIL), 3,
					int32(statuspb.TestStatus_PASS), 3,
				},
				Messages: []string{"no", "no again", "very wrong", "yes", "hi", "hello"},
				CellIds:  []string{"no", "no again", "very wrong", "yes", "hi", "hello"},
			},
			failOpen: 3,
			expected: alertInfo(3, "no", "very wrong", "no", nil, columns[2], columns[0], columns[3], false),
		},
		{
			name: "rows without cell IDs can alert",
			row: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FAIL), 3,
					int32(statuspb.TestStatus_PASS), 3,
				},
				Messages: []string{"no", "no again", "very wrong", "yes", "hi", "hello"},
			},
			failOpen: 3,
			expected: alertInfo(3, "no", "", "", nil, columns[2], columns[0], columns[3], false),
		},
		{
			name: "too few passes do not close",
			row: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_PASS), 2,
					int32(statuspb.TestStatus_FAIL), 4,
				},
				Messages: []string{"nope", "no", "yay", "very wrong", "hi", "hello"},
				CellIds:  []string{"wrong", "no", "yep", "very wrong", "hi", "hello"},
			},
			failOpen:  1,
			passClose: 3,
			expected:  alertInfo(4, "yay", "hello", "yep", nil, columns[5], columns[2], nil, false),
		},
		{
			name: "flakes do not close",
			row: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FLAKY), 2,
					int32(statuspb.TestStatus_FAIL), 4,
				},
				Messages: []string{"nope", "no", "yay", "very wrong", "hi", "hello"},
				CellIds:  []string{"wrong", "no", "yep", "very wrong", "hi", "hello"},
			},
			failOpen: 1,
			expected: alertInfo(4, "yay", "hello", "yep", nil, columns[5], columns[2], nil, false),
		},
		{
			name: "failures after insufficient passes",
			row: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FAIL), 1,
					int32(statuspb.TestStatus_FLAKY), 1,
					int32(statuspb.TestStatus_FAIL), 1,
					int32(statuspb.TestStatus_PASS), 1,
					int32(statuspb.TestStatus_FAIL), 2,
				},
				Messages: []string{"newest-fail", "what", "carelessness", "okay", "alert-here", "misfortune"},
				CellIds:  []string{"f0", "flake", "f2", "p3", "f4", "f5"},
			},
			failOpen:  2,
			passClose: 2,
			expected:  alertInfo(4, "newest-fail", "f5", "f0", nil, columns[5], columns[0], nil, false),
		},
		{
			name: "close alert",
			row: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_PASS), 1,
					int32(statuspb.TestStatus_FAIL), 5,
				},
			},
			failOpen: 1,
		},
		{
			name: "track through empty results",
			row: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FAIL), 1,
					int32(statuspb.TestStatus_NO_RESULT), 1,
					int32(statuspb.TestStatus_FAIL), 4,
				},
				Messages: []string{"yay" /*no result */, "no", "buu", "wrong", "nono"},
				CellIds:  []string{"yay-cell" /*no result */, "no", "buzz", "wrong2", "nada"},
			},
			failOpen:  5,
			passClose: 2,
			expected:  alertInfo(5, "yay", "nada", "yay-cell", nil, columns[5], columns[0], nil, false),
		},
		{
			name: "track passes through empty results",
			row: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_PASS), 1,
					int32(statuspb.TestStatus_NO_RESULT), 1,
					int32(statuspb.TestStatus_PASS), 1,
					int32(statuspb.TestStatus_FAIL), 3,
				},
			},
			failOpen:  1,
			passClose: 2,
		},
		{
			name: "running cells advance compressed index",
			row: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_RUNNING), 1,
					int32(statuspb.TestStatus_FAIL), 5,
				},
				Messages: []string{"running0", "fail1-expected", "fail2", "fail3", "fail4", "fail5"},
				CellIds:  []string{"wrong", "yep", "no2", "no3", "no4", "no5"},
			},
			failOpen: 1,
			expected: alertInfo(5, "fail1-expected", "no5", "yep", nil, columns[5], columns[1], nil, false),
		},
		{
			name: "complex",
			row: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_PASS), 1,
					int32(statuspb.TestStatus_FAIL), 1,
					int32(statuspb.TestStatus_PASS), 1,
					int32(statuspb.TestStatus_FAIL), 2,
					int32(statuspb.TestStatus_PASS), 1,
				},
				Messages: []string{"latest pass", "latest fail", "pass", "second fail", "first fail", "first pass"},
				CellIds:  []string{"no-p0", "no-f1", "no-p2", "no-f3", "yes-f4", "yes-p5"},
			},
			failOpen:  2,
			passClose: 2,
			expected:  alertInfo(3, "latest fail", "yes-f4", "no-f1", nil, columns[4], columns[1], columns[5], false),
		},
		{
			name: "alert consecutive failures only",
			row: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_PASS), 1,
					int32(statuspb.TestStatus_FAIL), 1,
					int32(statuspb.TestStatus_PASS), 1,
					int32(statuspb.TestStatus_FAIL), 1,
					int32(statuspb.TestStatus_PASS), 3,
				},
				Messages: []string{"latest pass", "latest fail", "pass", "second fail", "pass", "pass", "pass"},
				CellIds:  []string{"p0", "f1", "p2", "f3", "p4", "p5", "p6"},
			},
			failOpen:  2,
			passClose: 3,
		},
		{
			name: "properties",
			row: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FAIL), 3,
					int32(statuspb.TestStatus_PASS), 3,
				},
				Messages:     []string{"no", "no again", "very wrong", "yes", "hi", "hello"},
				CellIds:      []string{"no", "no again", "very wrong", "yes", "hi", "hello"},
				UserProperty: []string{"prop0", "prop1", "prop2", "prop3", "prop4", "prop5"},
			},
			failOpen: 3,
			property: "some-prop",
			expected: alertInfo(3, "no", "very wrong", "no", map[string]string{"some-prop": "prop0"}, columns[2], columns[0], columns[3], false),
		},
		{
			name: "properties after passes",
			row: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_PASS), 2,
					int32(statuspb.TestStatus_FAIL), 3,
					int32(statuspb.TestStatus_PASS), 1,
				},
				Messages:     []string{"no", "no again", "very wrong", "yes", "hi", "hello"},
				CellIds:      []string{"no", "no again", "very wrong", "yes", "hi", "hello"},
				UserProperty: []string{"prop0", "prop1", "prop2", "prop3", "prop4", "prop5"},
			},
			failOpen:  3,
			passClose: 3,
			property:  "some-prop",
			expected:  alertInfo(3, "very wrong", "hi", "very wrong", map[string]string{"some-prop": "prop2"}, columns[4], columns[2], columns[5], false),
		},
		{
			name: "empty properties",
			row: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FAIL), 3,
					int32(statuspb.TestStatus_PASS), 3,
				},
				Messages:     []string{"no", "no again", "very wrong", "yes", "hi", "hello"},
				CellIds:      []string{"no", "no again", "very wrong", "yes", "hi", "hello"},
				UserProperty: []string{},
			},
			failOpen: 3,
			property: "some-prop",
			expected: alertInfo(3, "no", "very wrong", "no", nil, columns[2], columns[0], columns[3], false),
		},
		{
			name: "insufficient properties",
			row: &statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_PASS), 2,
					int32(statuspb.TestStatus_FAIL), 4,
				},
				Messages:     []string{"no", "no again", "very wrong", "yes", "hi", "hello"},
				CellIds:      []string{"no", "no again", "very wrong", "yes", "hi", "hello"},
				UserProperty: []string{"prop0"},
			},
			failOpen:  3,
			passClose: 3,
			property:  "some-prop",
			expected:  alertInfo(4, "very wrong", "hello", "very wrong", nil, columns[5], columns[2], nil, false),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := alertRow(columns, tc.row, tc.failOpen, tc.passClose, false, tc.property)
			if diff := cmp.Diff(tc.expected, actual, protocmp.Transform()); diff != "" {
				t.Errorf("alertRow() not as expected (-want, +got): %s", diff)
			}
		})
	}
}

func TestBuildID(t *testing.T) {
	cases := []struct {
		name      string
		build     string
		extra     string
		useCommit bool
		expected  string
	}{
		{
			name: "return empty by default",
		},
		{
			name:      "use header as commit",
			build:     "wrong",
			extra:     "right",
			useCommit: true,
			expected:  "right",
		},
		{
			name:     "use build otherwise",
			build:    "right",
			extra:    "wrong",
			expected: "right",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			col := statepb.Column{
				Build: tc.build,
			}
			if tc.extra != "" {
				col.Extra = append(col.Extra, tc.extra)
			}
			if actual := buildID(&col, tc.useCommit); actual != tc.expected {
				t.Errorf("%q != expected %q", actual, tc.expected)
			}
		})
	}
}

func TestStamp(t *testing.T) {
	cases := []struct {
		name     string
		col      *statepb.Column
		expected *timestamp.Timestamp
	}{
		{
			name: "0 returns nil",
		},
		{
			name: "no nanos",
			col: &statepb.Column{
				Started: 2000,
			},
			expected: &timestamp.Timestamp{
				Seconds: 2,
				Nanos:   0,
			},
		},
		{
			name: "milli to nano",
			col: &statepb.Column{
				Started: 1234,
			},
			expected: &timestamp.Timestamp{
				Seconds: 1,
				Nanos:   234000000,
			},
		},
		{
			name: "double to nanos",
			col: &statepb.Column{
				Started: 1.1,
			},
			expected: &timestamp.Timestamp{
				Seconds: 0,
				Nanos:   1100000,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual := stamp(tc.col); !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("stamp %s != expected stamp %s", actual, tc.expected)
			}
		})
	}
}

func TestDropEmptyRows(t *testing.T) {
	cases := []struct {
		name     string
		results  map[string][]int32
		expected map[string][]int32
	}{
		{
			name:     "basically works",
			expected: map[string][]int32{},
		},
		{
			name: "keep everything",
			results: map[string][]int32{
				"pass":    {int32(statuspb.TestStatus_PASS), 1},
				"fail":    {int32(statuspb.TestStatus_FAIL), 2},
				"running": {int32(statuspb.TestStatus_RUNNING), 3},
			},
			expected: map[string][]int32{
				"pass":    {int32(statuspb.TestStatus_PASS), 1},
				"fail":    {int32(statuspb.TestStatus_FAIL), 2},
				"running": {int32(statuspb.TestStatus_RUNNING), 3},
			},
		},
		{
			name: "keep mixture",
			results: map[string][]int32{
				"was empty": {
					int32(statuspb.TestStatus_PASS), 1,
					int32(statuspb.TestStatus_NO_RESULT), 1,
				},
				"now empty": {
					int32(statuspb.TestStatus_NO_RESULT), 2,
					int32(statuspb.TestStatus_FAIL), 2,
				},
			},
			expected: map[string][]int32{
				"was empty": {
					int32(statuspb.TestStatus_PASS), 1,
					int32(statuspb.TestStatus_NO_RESULT), 1,
				},
				"now empty": {
					int32(statuspb.TestStatus_NO_RESULT), 2,
					int32(statuspb.TestStatus_FAIL), 2,
				},
			},
		},
		{
			name: "drop everything",
			results: map[string][]int32{
				"drop": {int32(statuspb.TestStatus_NO_RESULT), 1},
				"gone": {int32(statuspb.TestStatus_NO_RESULT), 10},
				"poof": {int32(statuspb.TestStatus_NO_RESULT), 100},
			},
			expected: map[string][]int32{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var grid statepb.Grid
			rows := make(map[string]*statepb.Row, len(tc.results))
			for name, res := range tc.results {
				r := &statepb.Row{Name: name}
				r.Results = res
				grid.Rows = append(grid.Rows, r)
				rows[name] = r
			}
			dropEmptyRows(logrus.WithField("name", tc.name), &grid, rows)
			actualRowMap := make(map[string]*statepb.Row, len(grid.Rows))
			for _, r := range grid.Rows {
				actualRowMap[r.Name] = r
			}

			if diff := cmp.Diff(rows, actualRowMap, protocmp.Transform()); diff != "" {
				t.Fatalf("dropEmptyRows() unmatched row maps (-grid, +map):\n%s", diff)
			}

			actual := make(map[string][]int32, len(rows))
			for name, row := range rows {
				actual[name] = row.Results
			}

			if diff := cmp.Diff(actual, tc.expected, protocmp.Transform()); diff != "" {
				t.Errorf("dropEmptyRows() got unexpected diff (-have, +want):\n%s", diff)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		max  int
		want string
	}{
		{
			name: "empty",
			msg:  "",
			max:  20,
			want: "",
		},
		{
			name: "short",
			msg:  "short message",
			max:  20,
			want: "short message",
		},
		{
			name: "long",
			msg:  "i'm too long of a message, oh no what will i do",
			max:  20,
			want: "i'm too lo... will i do",
		},
		{
			name: "long runes",
			msg:  "庭には二羽鶏がいる。", // In the yard two chickens are there.
			max:  20,
			want: "庭には...いる。",
		},
		{
			name: "short runes",
			msg:  "鶏がいる。", // Two chickens are there.
			max:  20,
			want: "鶏がいる。",
		},
		{
			name: "small max",
			msg:  "short message",
			max:  2,
			want: "s...e",
		},
		{
			name: "odd max",
			msg:  "short message",
			max:  5,
			want: "sh...ge",
		},
		{
			name: "max 1",
			msg:  "short message",
			max:  1,
			want: "...",
		},
		{
			name: "max 0",
			msg:  "short message",
			max:  0,
			want: "short message",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := truncate(tc.msg, tc.max); got != tc.want {
				t.Errorf("truncate(%q, %d) got %q, want %q", tc.msg, tc.max, got, tc.want)
			}
		})
	}
}

func TestHotlistIDs(t *testing.T) {
	cases := []struct {
		name       string
		hotlistIDs string
		want       []string
	}{
		{
			name:       "none",
			hotlistIDs: "",
			want:       nil,
		},
		{
			name:       "empty",
			hotlistIDs: ",,",
			want:       nil,
		},
		{
			name:       "one",
			hotlistIDs: "123",
			want:       []string{"123"},
		},
		{
			name:       "many",
			hotlistIDs: "123,456,789",
			want:       []string{"123", "456", "789"},
		},
		{
			name:       "spaces",
			hotlistIDs: "123 , 456, 789 ",
			want:       []string{"123", "456", "789"},
		},
		{
			name:       "many empty",
			hotlistIDs: "123,,456,",
			want:       []string{"123", "456"},
		},
		{
			name:       "complex",
			hotlistIDs: " 123,456,,, 789 ,",
			want:       []string{"123", "456", "789"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			col := &statepb.Column{
				HotlistIds: tc.hotlistIDs,
			}
			got := hotlistIDs(col)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("hotlistIDs(%v) differed (-want, +got): %s", col, diff)
			}
		})
	}
}
