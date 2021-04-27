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
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/fvbommel/sortorder"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/testing/protocmp"
	core "k8s.io/api/core/v1"

	"github.com/GoogleCloudPlatform/testgrid/config"
	"github.com/GoogleCloudPlatform/testgrid/metadata"
	_ "github.com/GoogleCloudPlatform/testgrid/metadata/junit"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	statuspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs/fake"
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
		group configpb.TestGroup
		fail  bool
	}{
		{
			name: "ignore non-kubernetes clients",
		},
		{
			name: "fail kubernetes clients",
			group: configpb.TestGroup{
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
			updater := GCS(0, 0, 0, false)
			defer func() {
				if r := recover(); r != nil {
					if !tc.fail {
						t.Errorf("updater() got an unexpected panic: %#v", r)
					}
				}
			}()
			err := updater(ctx, logrus.WithField("case", tc.name), nil, &tc.group, gcs.Path{})
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
		config           configpb.Configuration
		configErr        error
		builds           map[string][]fakeBuild
		gridPrefix       string
		groupConcurrency int
		buildConcurrency int
		skipConfirm      bool
		groupTimeout     *time.Duration
		buildTimeout     *time.Duration
		group            string

		expected fakeUploader
		err      bool
	}{
		{
			name: "basically works",
			config: configpb.Configuration{
				TestGroups: []*configpb.TestGroup{
					{
						Name:                "hello",
						GcsPrefix:           "kubernetes-jenkins/path/to/job",
						DaysOfResults:       7,
						UseKubernetesClient: true,
						NumColumnsRecent:    6,
					},
					{
						Name:                "skip-non-k8s",
						GcsPrefix:           "kubernetes-jenkins/path/to/job",
						DaysOfResults:       7,
						UseKubernetesClient: false,
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
				},
				*resolveOrDie(&configPath, "skip-non-k8s"): {
					Buf:          mustGrid(&statepb.Grid{}),
					CacheControl: "no-cache",
					WorldRead:    gcs.DefaultACL,
				},
			},
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

			client := fakeUploadClient{
				Uploader: fakeUploader{},
				Client: fakeClient{
					Lister: fakeLister{},
					Opener: fakeOpener{},
				},
			}

			client.Opener[configPath] = fakeObject{
				Data: func() string {
					b, err := config.MarshalBytes(&tc.config)
					if err != nil {
						t.Fatalf("config.MarshalBytes() errored: %v", err)
					}
					return string(b)
				}(),
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

			groupUpdater := GCS(*tc.groupTimeout, *tc.buildTimeout, tc.buildConcurrency, !tc.skipConfirm)

			err := Update(
				ctx,
				client,
				configPath,
				tc.gridPrefix,
				tc.groupConcurrency,
				tc.group,
				groupUpdater,
				!tc.skipConfirm,
			)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("Update() got unexpected error: %v", err)
				}
			case tc.err:
				t.Error("Update() failed to receive an errro")
			default:
				actual := client.Uploader
				diff := cmp.Diff(actual, tc.expected, cmp.AllowUnexported(fakeUpload{}))
				if diff == "" {
					return
				}
				t.Errorf("Update() uploaded files got unexpected diff (-have, +want):\n%s", diff)
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
			actual, err := testGroupPath(path, tc.gridPrefix, tc.groupName)
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
		Result:  statuspb.TestStatus_FAIL,
		Icon:    "!",
		Message: gcs.MissingPodInfo,
	}
)

func jsonPodInfo(podInfo gcs.PodInfo) *fakeObject {
	return &fakeObject{Data: jsonData(podInfo)}
}

func mustGrid(grid *statepb.Grid) []byte {
	buf, err := marshalGrid(grid)
	if err != nil {
		panic(err)
	}
	return buf
}

func TestSortGroups(t *testing.T) {
	now := time.Now()
	times := []time.Time{
		now.Add(1 * time.Hour),
		now.Add(2 * time.Hour),
		now.Add(3 * time.Hour),
		now.Add(4 * time.Hour),
	}

	cases := []struct {
		name     string
		groups   []*configpb.TestGroup
		updated  map[string]*time.Time
		expected []*configpb.TestGroup
	}{
		{
			name: "basically works",
		},
		{
			name: "sorts multiple groups by least recently updated",
			groups: []*configpb.TestGroup{
				{Name: "middle"},
				{Name: "new"},
				{Name: "old"},
			},
			updated: map[string]*time.Time{
				"old":    &times[0],
				"middle": &times[1],
				"new":    &times[2],
			},
			expected: []*configpb.TestGroup{
				{Name: "old"},
				{Name: "middle"},
				{Name: "new"},
			},
		},
		{
			name: "groups with no existing state come first",
			groups: []*configpb.TestGroup{
				{Name: "new"},
				{Name: "never"},
				{Name: "old"},
			},
			updated: map[string]*time.Time{
				"old": &times[0],
				"new": &times[2],
			},
			expected: []*configpb.TestGroup{
				{Name: "never"},
				{Name: "old"},
				{Name: "new"},
			},
		},
		{
			name: "groups with an error state also go first",
			groups: []*configpb.TestGroup{
				{Name: "new"},
				{Name: "boom"},
				{Name: "old"},
			},
			updated: map[string]*time.Time{
				"boom": nil,
				"old":  &times[0],
				"new":  &times[2],
			},
			expected: []*configpb.TestGroup{
				{Name: "boom"},
				{Name: "old"},
				{Name: "new"},
			},
		},
	}

	const gridPrefix = "grid"
	configPath, err := gcs.NewPath("gs://k8s-testgrid-canary/config")
	if err != nil {
		t.Fatalf("bad path: %v", err)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			client := fakeStater{}
			for name, updated := range tc.updated {
				var stat fakeStat
				if updated == nil {
					stat.Err = errors.New("error")
				} else {
					stat.Attrs.Updated = *updated
				}
				path, err := testGroupPath(*configPath, gridPrefix, name)
				if err != nil {
					t.Fatalf("bad group path: %v", err)
				}
				client[*path] = stat
			}
			defer cancel()
			sortGroups(ctx, logrus.WithField("case", tc.name), client, *configPath, gridPrefix, tc.groups)
		})
	}
}

func TestTruncateRunning(t *testing.T) {
	now := float64(time.Now().UTC().Unix() * 1000)
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
			actual := truncateRunning(tc.cols)
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

func TestTruncateBuilds(t *testing.T) {
	cases := []struct {
		name   string
		builds int
		rows   []int
		start  int
		end    int
	}{
		{
			name: "basically works",
		},
		{
			name:   "usually include everything",
			builds: 5,
			rows:   []int{100},
			start:  0,
			end:    5,
		},
		{
			name:   "many rows truncates columns",
			builds: 10,
			rows:   []int{maxUpdateArea / 4, maxUpdateArea / 4, maxUpdateArea / 4, maxUpdateArea / 4},
			start:  6,
			end:    10,
		},
		{
			name:   "many new columns truncates",
			builds: maxUpdateArea,
			rows:   []int{1000},
			start:  maxUpdateArea - (maxUpdateArea / 1000),
			end:    maxUpdateArea,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var builds []gcs.Build
			var cols []inflatedColumn
			var expected []gcs.Build

			for i := 0; i < tc.builds; i++ {
				p, err := gcs.NewPath(fmt.Sprintf("gs://fake/job/%d", i))
				if err != nil {
					t.Fatalf("bad path: %v", err)
				}
				b := gcs.Build{Path: *p}
				builds = append(builds, b)
				if i >= tc.start && i < tc.end {
					expected = append(expected, b)
				}
			}

			for _, r := range tc.rows {
				col := inflatedColumn{
					Cells: map[string]cell{},
				}
				for i := 0; i < r; i++ {
					id := fmt.Sprintf("cell %d", i)
					c := cell{CellID: id}
					col.Cells[id] = c
				}
				cols = append(cols, col)
			}

			actual := truncateBuilds(logrus.WithField("name", tc.name), builds, cols)
			diff := cmp.Diff(actual, expected, cmp.AllowUnexported(gcs.Build{}, gcs.Path{}, cell{}, inflatedColumn{}), protocmp.Transform())
			if diff == "" {
				return
			}
			if have, want := len(actual), len(expected); have != want {
				t.Errorf("truncateRunning() got %d columns, want %d", have, want)
			} else {
				t.Errorf("truncateRunning() got unexpected diff (-have, +want):\n%s", diff)
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
					Idx:    0,
					Err:    0,
					Offset: "",
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
					},
					Idx:    0,
					Err:    0,
					Offset: "",
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
					Path: newPathOrDie("gs://prefix/job/3/"),
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
	now := time.Now().Unix()
	uploadPath := newPathOrDie("gs://fake/upload/location")
	defaultTimeout := 5 * time.Minute
	cases := []struct {
		name         string
		ctx          context.Context
		builds       []fakeBuild
		group        configpb.TestGroup
		concurrency  int
		skipWrite    bool
		groupTimeout *time.Duration
		buildTimeout *time.Duration
		expected     *fakeUpload
		err          bool
	}{
		{
			name: "basically works",
			group: configpb.TestGroup{
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
								Name: overallRow,
								Id:   overallRow,
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
								Name: podInfoRow,
								Id:   podInfoRow,
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
			},
		},
		{
			name:      "do not write when requested",
			skipWrite: true,
			group: configpb.TestGroup{
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
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ctx == nil {
				tc.ctx = context.Background()
			}
			ctx, cancel := context.WithCancel(tc.ctx)
			defer cancel()

			if tc.concurrency == 0 {
				tc.concurrency = 1
			}
			if tc.groupTimeout == nil {
				tc.groupTimeout = &defaultTimeout
			}
			if tc.buildTimeout == nil {
				tc.buildTimeout = &defaultTimeout
			}

			client := fakeUploadClient{
				Uploader: fakeUploader{},
				Client: fakeClient{
					Lister: fakeLister{},
					Opener: fakeOpener{},
				},
			}

			buildsPath := newPathOrDie("gs://" + tc.group.GcsPrefix)
			fi := client.Lister[buildsPath]
			for _, build := range addBuilds(&client.Client, buildsPath, tc.builds...) {
				fi.Objects = append(fi.Objects, storage.ObjectAttrs{
					Prefix: build.Path.Object(),
				})
			}
			client.Lister[buildsPath] = fi

			colReader := gcsColumnReader(client, *tc.buildTimeout, tc.concurrency)
			err := InflateDropAppend(
				ctx,
				logrus.WithField("test", tc.name),
				client,
				&tc.group,
				uploadPath,
				!tc.skipWrite,
				colReader,
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
				diff := cmp.Diff(actual, expected, cmp.AllowUnexported(gcs.Path{}, fakeUpload{}), protocmp.Transform())
				if diff == "" {
					return
				}
				t.Errorf("updateGroup() got unexpected diff (-have, +want):\n%s", diff)
				fakeDownloader := fakeOpener{
					uploadPath: {Data: string(actual[uploadPath].Buf)},
				}
				actualGrid, err := gcs.DownloadGrid(ctx, fakeDownloader, uploadPath)
				if err != nil {
					t.Errorf("actual gcs.DownloadGrid() got unexpected error: %v", err)
				}
				fakeDownloader[uploadPath] = fakeObject{Data: string(tc.expected.Buf)}
				expectedGrid, err := gcs.DownloadGrid(ctx, fakeDownloader, uploadPath)
				if err != nil {
					t.Errorf("expected gcs.DownloadGrid() got unexpected error: %v", err)
				}
				diff = cmp.Diff(actualGrid, expectedGrid, protocmp.Transform())
				if diff == "" {
					return
				}
				t.Errorf("gcs.DownloadGrid() got unexpected diff (-have, +want):\n%s", diff)
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
				CommitOverrideStrftime: "%y-%m-%d (%Y) %H:%M:%S %p",
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
				CommitOverrideStrftime: "hello 2006 PM",
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

func TestMergeColumns(t *testing.T) {
	cases := []struct {
		name     string
		newCols  []inflatedColumn
		oldCols  []inflatedColumn
		expected []inflatedColumn
	}{
		{
			name: "basically works",
		},
		{
			name: "only new cols",
			newCols: []inflatedColumn{
				{
					Column: &statepb.Column{
						Build: "hello",
					},
					Cells: map[string]cell{
						"this": {Result: statuspb.TestStatus_PASS},
					},
				},
				{
					Column: &statepb.Column{
						Build: "world",
					},
					Cells: map[string]cell{
						"that": {Result: statuspb.TestStatus_FAIL},
					},
				},
			},
			expected: []inflatedColumn{
				{
					Column: &statepb.Column{
						Build: "hello",
					},
					Cells: map[string]cell{
						"this": {Result: statuspb.TestStatus_PASS},
					},
				},
				{
					Column: &statepb.Column{
						Build: "world",
					},
					Cells: map[string]cell{
						"that": {Result: statuspb.TestStatus_FAIL},
					},
				},
			},
		},
		{
			name: "only old cols",
			oldCols: []inflatedColumn{
				{
					Column: &statepb.Column{
						Build: "ancient",
					},
					Cells: map[string]cell{
						"this": {Result: statuspb.TestStatus_PASS},
					},
				},
				{
					Column: &statepb.Column{
						Build: "graveyard",
					},
					Cells: map[string]cell{
						"that": {Result: statuspb.TestStatus_FAIL},
					},
				},
			},
			expected: []inflatedColumn{
				{
					Column: &statepb.Column{
						Build: "ancient",
					},
					Cells: map[string]cell{
						"this": {Result: statuspb.TestStatus_PASS},
					},
				},
				{
					Column: &statepb.Column{
						Build: "graveyard",
					},
					Cells: map[string]cell{
						"that": {Result: statuspb.TestStatus_FAIL},
					},
				},
			},
		},
		{
			name: "accept all when old are all older than new",
			newCols: []inflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "new-1000",
						Started: 1000,
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_RUNNING},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "new-900",
						Started: 900,
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_PASS},
					},
				},
			},
			oldCols: []inflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "old-50",
						Started: 50,
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_FAIL},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "old-40",
						Started: 40,
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_FLAKY},
					},
				},
			},
			expected: []inflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "new-1000",
						Started: 1000,
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_RUNNING},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "new-900",
						Started: 900,
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_PASS},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "old-50",
						Started: 50,
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_FAIL},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "old-40",
						Started: 40,
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_FLAKY},
					},
				},
			},
		},
		{
			name: "accept all new and oldest old, reject olds which are >= new",
			newCols: []inflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "new-1000",
						Started: 1000,
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_RUNNING},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "new-900",
						Started: 900,
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_PASS},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "new-200",
						Started: 200,
					},
					Cells: map[string]cell{
						"test": {Message: "accept new 200"},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "new-100",
						Started: 100,
					},
					Cells: map[string]cell{
						"test": {Message: "accept new 100"},
					},
				},
			},
			oldCols: []inflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "old-500",
						Started: 500,
					},
					Cells: map[string]cell{
						"test": {Message: "reject old"},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "old-150",
						Started: 150,
					},
					Cells: map[string]cell{
						"test": {Message: "reject old"},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "old-50",
						Started: 50,
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_FAIL},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "old-40",
						Started: 40,
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_FLAKY},
					},
				},
			},
			expected: []inflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "new-1000",
						Started: 1000,
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_RUNNING},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "new-900",
						Started: 900,
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_PASS},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "new-200",
						Started: 200,
					},
					Cells: map[string]cell{
						"test": {Message: "accept new 200"},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "new-100",
						Started: 100,
					},
					Cells: map[string]cell{
						"test": {Message: "accept new 100"},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "old-50",
						Started: 50,
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_FAIL},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "old-40",
						Started: 40,
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_FLAKY},
					},
				},
			},
		},
		{
			name: "accept all new and oldest old, reject old duplicates",
			newCols: []inflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "new-1000",
						Started: 1000,
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_RUNNING},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "new-900",
						Started: 900,
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_PASS},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "shared-110",
						Started: 110,
					},
					Cells: map[string]cell{
						"test": {Message: "accept new 110"},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "shared-100",
						Started: 100,
					},
					Cells: map[string]cell{
						"test": {Message: "accept new 100"},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "shared-90",
						Started: 90,
					},
					Cells: map[string]cell{
						"test": {Message: "accept new 90"},
					},
				},
			},
			oldCols: []inflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "shared-110",
						Started: 110,
						Extra:   []string{"reject old"},
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_FAIL},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "shared-100",
						Started: 100,
						Extra:   []string{"reject old"},
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_FAIL},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "shared-90",
						Started: 90,
						Extra:   []string{"reject old"},
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_FAIL},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "old-50",
						Started: 50,
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_FAIL},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "old-40",
						Started: 40,
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_FLAKY},
					},
				},
			},
			expected: []inflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "new-1000",
						Started: 1000,
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_RUNNING},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "new-900",
						Started: 900,
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_PASS},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "shared-110",
						Started: 110,
					},
					Cells: map[string]cell{
						"test": {Message: "accept new 110"},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "shared-100",
						Started: 100,
					},
					Cells: map[string]cell{
						"test": {Message: "accept new 100"},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "shared-90",
						Started: 90,
					},
					Cells: map[string]cell{
						"test": {Message: "accept new 90"},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "old-50",
						Started: 50,
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_FAIL},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "old-40",
						Started: 40,
					},
					Cells: map[string]cell{
						"test": {Result: statuspb.TestStatus_FLAKY},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := mergeColumns(tc.newCols, tc.oldCols)
			internals := cmp.AllowUnexported(inflatedColumn{}, cell{})
			if diff := cmp.Diff(actual, tc.expected, internals, protocmp.Transform()); diff != "" {
				t.Errorf("mergeColumns() got unexpected diff (-have, +want):\n%s", diff)
			}
		})
	}
}

func TestConstructGrid(t *testing.T) {
	cases := []struct {
		name     string
		group    configpb.TestGroup
		cols     []inflatedColumn
		expected statepb.Grid
	}{
		{
			name: "basically works",
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
			expected: statepb.Grid{
				Columns: []*statepb.Column{
					{Build: "15"},
					{Build: "10"},
				},
				Rows: []*statepb.Row{
					setupRow(
						&statepb.Row{
							Name: "full",
							Id:   "full",
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
			name: "open alert",
			group: configpb.TestGroup{
				NumFailuresToAlert: 2,
			},
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
			expected: statepb.Grid{
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
			name: "close alert",
			group: configpb.TestGroup{
				NumPassesToDisableAlert: 2,
				NumFailuresToAlert:      1,
			},
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
			expected: statepb.Grid{
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
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := constructGrid(logrus.WithField("name", tc.name), &tc.group, tc.cols)
			failuresOpen := int(tc.group.NumFailuresToAlert)
			passesClose := int(tc.group.NumPassesToDisableAlert)
			if failuresOpen > 0 && passesClose == 0 {
				passesClose = 1
			}
			alertRows(tc.expected.Columns, tc.expected.Rows, failuresOpen, passesClose)
			for _, row := range tc.expected.Rows {
				sort.SliceStable(row.Metric, func(i, j int) bool {
					return sortorder.NaturalLess(row.Metric[i], row.Metric[j])
				})
				sort.SliceStable(row.Metrics, func(i, j int) bool {
					return sortorder.NaturalLess(row.Metrics[i].Name, row.Metrics[j].Name)
				})
			}
			if diff := cmp.Diff(actual, tc.expected, protocmp.Transform()); diff != "" {
				t.Errorf("constructGrid() got unexpected diff (-have, +want):\n%s", diff)
			}
		})
	}
}

func TestMarshalGrid(t *testing.T) {
	g1 := statepb.Grid{
		Columns: []*statepb.Column{
			{Build: "alpha"},
			{Build: "second"},
		},
	}
	g2 := statepb.Grid{
		Columns: []*statepb.Column{
			{Build: "first"},
			{Build: "second"},
		},
	}

	b1, e1 := marshalGrid(&g1)
	b2, e2 := marshalGrid(&g2)
	uncompressed, e1a := proto.Marshal(&g1)

	switch {
	case e1 != nil, e2 != nil:
		t.Errorf("unexpected error %v %v %v", e1, e2, e1a)
	}

	if reflect.DeepEqual(b1, b2) {
		t.Errorf("unexpected equality %v == %v", b1, b2)
	}

	if reflect.DeepEqual(b1, uncompressed) {
		t.Errorf("should be compressed but is not: %v", b1)
	}
}

func TestAppendMetric(t *testing.T) {
	cases := []struct {
		name     string
		metric   statepb.Metric
		idx      int32
		value    float64
		expected statepb.Metric
	}{
		{
			name: "basically works",
			expected: statepb.Metric{
				Indices: []int32{0, 1},
				Values:  []float64{0},
			},
		},
		{
			name:  "start metric at random column",
			idx:   7,
			value: 11,
			expected: statepb.Metric{
				Indices: []int32{7, 1},
				Values:  []float64{11},
			},
		},
		{
			name: "continue existing series",
			metric: statepb.Metric{
				Indices: []int32{6, 2},
				Values:  []float64{6.1, 6.2},
			},
			idx:   8,
			value: 88,
			expected: statepb.Metric{
				Indices: []int32{6, 3},
				Values:  []float64{6.1, 6.2, 88},
			},
		},
		{
			name: "start new series",
			metric: statepb.Metric{
				Indices: []int32{3, 2},
				Values:  []float64{6.1, 6.2},
			},
			idx:   8,
			value: 88,
			expected: statepb.Metric{
				Indices: []int32{3, 2, 8, 1},
				Values:  []float64{6.1, 6.2, 88},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			appendMetric(&tc.metric, tc.idx, tc.value)
			if diff := cmp.Diff(tc.metric, tc.expected, protocmp.Transform()); diff != "" {
				t.Errorf("appendMetric() got unexpected diff (-got +want):\n%s", diff)
			}
		})
	}
}

func TestAppendCell(t *testing.T) {
	cases := []struct {
		name  string
		row   statepb.Row
		cell  cell
		start int
		count int

		expected statepb.Row
	}{
		{
			name: "basically works",
			expected: statepb.Row{
				Results: []int32{0, 0},
			},
		},
		{
			name: "first result",
			cell: cell{
				Result: statuspb.TestStatus_PASS,
			},
			count: 1,
			expected: statepb.Row{
				Results:  []int32{int32(statuspb.TestStatus_PASS), 1},
				CellIds:  []string{""},
				Messages: []string{""},
				Icons:    []string{""},
			},
		},
		{
			name: "all fields filled",
			cell: cell{
				Result:  statuspb.TestStatus_PASS,
				CellID:  "cell-id",
				Message: "hi",
				Icon:    "there",
				Metrics: map[string]float64{
					"pi":     3.14,
					"golden": 1.618,
				},
			},
			count: 1,
			expected: statepb.Row{
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
			},
		},
		{
			name: "append same result",
			row: statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FLAKY), 3,
				},
				CellIds:  []string{"", "", ""},
				Messages: []string{"", "", ""},
				Icons:    []string{"", "", ""},
			},
			cell: cell{
				Result:  statuspb.TestStatus_FLAKY,
				Message: "echo",
				CellID:  "again and",
				Icon:    "keeps going",
			},
			count: 2,
			expected: statepb.Row{
				Results:  []int32{int32(statuspb.TestStatus_FLAKY), 5},
				CellIds:  []string{"", "", "", "again and", "again and"},
				Messages: []string{"", "", "", "echo", "echo"},
				Icons:    []string{"", "", "", "keeps going", "keeps going"},
			},
		},
		{
			name: "append different result",
			row: statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FLAKY), 3,
				},
				CellIds:  []string{"", "", ""},
				Messages: []string{"", "", ""},
				Icons:    []string{"", "", ""},
			},
			cell: cell{
				Result: statuspb.TestStatus_PASS,
			},
			count: 2,
			expected: statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FLAKY), 3,
					int32(statuspb.TestStatus_PASS), 2,
				},
				CellIds:  []string{"", "", "", "", ""},
				Messages: []string{"", "", "", "", ""},
				Icons:    []string{"", "", "", "", ""},
			},
		},
		{
			name: "append no Result (results, no cellIDs, messages or icons)",
			row: statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FLAKY), 3,
				},
				CellIds:  []string{"", "", ""},
				Messages: []string{"", "", ""},
				Icons:    []string{"", "", ""},
			},
			cell: cell{
				Result: statuspb.TestStatus_NO_RESULT,
			},
			count: 2,
			expected: statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FLAKY), 3,
					int32(statuspb.TestStatus_NO_RESULT), 2,
				},
				CellIds:  []string{"", "", ""},
				Messages: []string{"", "", ""},
				Icons:    []string{"", "", ""},
			},
		},
		{
			name: "add metric to series",
			row: statepb.Row{
				Results:  []int32{int32(statuspb.TestStatus_PASS), 5},
				CellIds:  []string{"", "", "", "", "c"},
				Messages: []string{"", "", "", "", "m"},
				Icons:    []string{"", "", "", "", "i"},
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
			expected: statepb.Row{
				Results:  []int32{int32(statuspb.TestStatus_PASS), 6},
				CellIds:  []string{"", "", "", "", "c", ""},
				Messages: []string{"", "", "", "", "m", ""},
				Icons:    []string{"", "", "", "", "i", ""},
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
			cell:  emptyCell,
			count: 7,
			expected: statepb.Row{
				Results: []int32{int32(statuspb.TestStatus_NO_RESULT), 7},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			appendCell(&tc.row, tc.cell, tc.start, tc.count)
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

func setupRow(row *statepb.Row, cells ...cell) *statepb.Row {
	for idx, c := range cells {
		appendCell(row, c, idx, 1)
	}
	return row
}

func TestAppendColumn(t *testing.T) {
	cases := []struct {
		name     string
		grid     statepb.Grid
		col      inflatedColumn
		expected statepb.Grid
	}{
		{
			name: "append first column",
			col:  inflatedColumn{Column: &statepb.Column{Build: "10"}},
			expected: statepb.Grid{
				Columns: []*statepb.Column{
					{Build: "10"},
				},
			},
		},
		{
			name: "append additional column",
			grid: statepb.Grid{
				Columns: []*statepb.Column{
					{Build: "10"},
					{Build: "11"},
				},
			},
			col: inflatedColumn{Column: &statepb.Column{Build: "20"}},
			expected: statepb.Grid{
				Columns: []*statepb.Column{
					{Build: "10"},
					{Build: "11"},
					{Build: "20"},
				},
			},
		},
		{
			name: "add rows to first column",
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
						Result:  statuspb.TestStatus_FAIL,
						Message: "boom",
						Icon:    "X",
					},
				},
			},
			expected: statepb.Grid{
				Columns: []*statepb.Column{
					{Build: "10"},
				},
				Rows: []*statepb.Row{
					setupRow(
						&statepb.Row{
							Name: "hello",
							Id:   "hello",
						},
						cell{
							Result:  statuspb.TestStatus_PASS,
							CellID:  "yes",
							Metrics: map[string]float64{"answer": 42},
						}),
					setupRow(&statepb.Row{
						Name: "world",
						Id:   "world",
					}, cell{
						Result:  statuspb.TestStatus_FAIL,
						Message: "boom",
						Icon:    "X",
					}),
				},
			},
		},
		{
			name: "add empty cells",
			grid: statepb.Grid{
				Columns: []*statepb.Column{
					{Build: "10"},
					{Build: "11"},
					{Build: "12"},
				},
				Rows: []*statepb.Row{
					setupRow(
						&statepb.Row{Name: "deleted"},
						cell{Result: statuspb.TestStatus_PASS},
						cell{Result: statuspb.TestStatus_PASS},
						cell{Result: statuspb.TestStatus_PASS},
					),
					setupRow(
						&statepb.Row{Name: "always"},
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
			expected: statepb.Grid{
				Columns: []*statepb.Column{
					{Build: "10"},
					{Build: "11"},
					{Build: "12"},
					{Build: "20"},
				},
				Rows: []*statepb.Row{
					setupRow(
						&statepb.Row{Name: "deleted"},
						cell{Result: statuspb.TestStatus_PASS},
						cell{Result: statuspb.TestStatus_PASS},
						cell{Result: statuspb.TestStatus_PASS},
						emptyCell,
					),
					setupRow(
						&statepb.Row{Name: "always"},
						cell{Result: statuspb.TestStatus_PASS},
						cell{Result: statuspb.TestStatus_PASS},
						cell{Result: statuspb.TestStatus_PASS},
						cell{Result: statuspb.TestStatus_PASS},
					),
					setupRow(
						&statepb.Row{
							Name: "new",
							Id:   "new",
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
			appendColumn(&tc.grid, rows, tc.col)
			sort.SliceStable(tc.grid.Rows, func(i, j int) bool {
				return tc.grid.Rows[i].Name < tc.grid.Rows[j].Name
			})
			sort.SliceStable(tc.expected.Rows, func(i, j int) bool {
				return tc.expected.Rows[i].Name < tc.expected.Rows[j].Name
			})
			if diff := cmp.Diff(tc.grid, tc.expected, protocmp.Transform()); diff != "" {
				t.Errorf("appendColumn() got unexpected diff (-got +want):\n%s", diff)
			}
		})
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
		row       statepb.Row
		failOpen  int
		passClose int
		expected  *statepb.AlertInfo
	}{
		{
			name: "never alert by default",
			row: statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FAIL), 6,
				},
			},
		},
		{
			name: "passes do not alert",
			row: statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_PASS), 6,
				},
			},
			failOpen:  1,
			passClose: 3,
		},
		{
			name: "flakes do not alert",
			row: statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FLAKY), 6,
				},
			},
			failOpen: 1,
		},
		{
			name: "intermittent failures do not alert",
			row: statepb.Row{
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
			row: statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FAIL), 3,
					int32(statuspb.TestStatus_PASS), 3,
				},
				Messages: []string{"no", "no again", "very wrong", "yes", "hi", "hello"},
				CellIds:  []string{"no", "no again", "very wrong", "yes", "hi", "hello"},
			},
			failOpen: 3,
			expected: alertInfo(3, "no", "very wrong", "no", columns[2], columns[0], columns[3]),
		},
		{
			name: "rows without cell IDs can alert",
			row: statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FAIL), 3,
					int32(statuspb.TestStatus_PASS), 3,
				},
				Messages: []string{"no", "no again", "very wrong", "yes", "hi", "hello"},
			},
			failOpen: 3,
			expected: alertInfo(3, "no", "", "", columns[2], columns[0], columns[3]),
		},
		{
			name: "too few passes do not close",
			row: statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_PASS), 2,
					int32(statuspb.TestStatus_FAIL), 4,
				},
				Messages: []string{"nope", "no", "yay", "very wrong", "hi", "hello"},
				CellIds:  []string{"wrong", "no", "yep", "very wrong", "hi", "hello"},
			},
			failOpen:  1,
			passClose: 3,
			expected:  alertInfo(4, "yay", "hello", "yep", columns[5], columns[2], nil),
		},
		{
			name: "flakes do not close",
			row: statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FLAKY), 2,
					int32(statuspb.TestStatus_FAIL), 4,
				},
				Messages: []string{"nope", "no", "yay", "very wrong", "hi", "hello"},
				CellIds:  []string{"wrong", "no", "yep", "very wrong", "hi", "hello"},
			},
			failOpen: 1,
			expected: alertInfo(4, "yay", "hello", "yep", columns[5], columns[2], nil),
		},
		{
			name: "count failures after flaky passes",
			row: statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FAIL), 1,
					int32(statuspb.TestStatus_FLAKY), 1,
					int32(statuspb.TestStatus_FAIL), 1,
					int32(statuspb.TestStatus_PASS), 1,
					int32(statuspb.TestStatus_FAIL), 2,
				},
				Messages: []string{"nope", "no", "buu", "wrong", "this one", "hi"},
				CellIds:  []string{"wrong", "no", "buzz", "wrong2", "good job", "hi"},
			},
			failOpen:  2,
			passClose: 2,
			expected:  alertInfo(4, "this one", "hi", "good job", columns[5], columns[4], nil),
		},
		{
			name: "close alert",
			row: statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_PASS), 1,
					int32(statuspb.TestStatus_FAIL), 5,
				},
			},
			failOpen: 1,
		},
		{
			name: "track through empty results",
			row: statepb.Row{
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
			expected:  alertInfo(5, "yay", "nada", "yay-cell", columns[5], columns[0], nil),
		},
		{
			name: "track passes through empty results",
			row: statepb.Row{
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
			row: statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_RUNNING), 1,
					int32(statuspb.TestStatus_FAIL), 5,
				},
				Messages: []string{"running0", "fail1-expected", "fail2", "fail3", "fail4", "fail5"},
				CellIds:  []string{"wrong", "yep", "no2", "no3", "no4", "no5"},
			},
			failOpen: 1,
			expected: alertInfo(5, "fail1-expected", "no5", "yep", columns[5], columns[1], nil),
		},
	}

	for _, tc := range cases {
		actual := alertRow(columns, &tc.row, tc.failOpen, tc.passClose)
		if diff := cmp.Diff(tc.expected, actual, protocmp.Transform()); diff != "" {
			t.Errorf("alertRow() not as expected (-want, +got): %s", diff)
		}
	}
}

func TestBuildID(t *testing.T) {
	cases := []struct {
		name     string
		build    string
		extra    string
		expected string
	}{
		{
			name: "return empty by default",
		},
		{
			name:     "favor extra if it exists",
			build:    "wrong",
			extra:    "right",
			expected: "right",
		},
		{
			name:     "build if no extra",
			build:    "yes",
			expected: "yes",
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
			if actual := buildID(&col); actual != tc.expected {
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
				Started: 2,
			},
			expected: &timestamp.Timestamp{
				Seconds: 2,
				Nanos:   0,
			},
		},
		{
			name: "has nanos",
			col: &statepb.Column{
				Started: 1.1,
			},
			expected: &timestamp.Timestamp{
				Seconds: 1,
				Nanos:   1e8,
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

func TestBumpMaxUpdateArea(t *testing.T) {
	updateAreaLock.RLock()
	orig := maxUpdateArea
	updateAreaLock.RUnlock()
	defer func(orig int) {
		updateAreaLock.Lock()
		maxUpdateArea = orig
		updateAreaLock.Unlock()
	}(orig)

	cases := []struct {
		name    string
		start   int
		floor   int
		ceiling int
	}{
		{
			name:    "bump grows update area",
			start:   1,
			floor:   2,
			ceiling: maxMaxUpdateArea,
		},
		{
			name:    "original update area smaller than final",
			start:   orig,
			floor:   orig + 1,
			ceiling: maxMaxUpdateArea - 1,
		},
		{
			name:  "grow to limit",
			start: maxMaxUpdateArea - 1,
			floor: maxMaxUpdateArea,
		},
		{
			name:    "do not grow past limt",
			start:   maxMaxUpdateArea + 1,
			floor:   maxMaxUpdateArea + 1,
			ceiling: maxMaxUpdateArea + 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			updateAreaLock.Lock()
			maxUpdateArea = tc.start
			updateAreaLock.Unlock()
			growMaxUpdateArea()
			updateAreaLock.RLock()
			actual := maxUpdateArea
			updateAreaLock.RUnlock()
			if tc.floor != 0 && actual < tc.floor {
				t.Errorf("maxUpdateArea=%d growMaxUpdateArea() got %d < %d", tc.start, actual, tc.floor)
			}
			if tc.ceiling != 0 && actual > tc.ceiling {
				t.Errorf("maxUpdateArea=%d growMaxUpdateArea() got %d > %d", tc.start, actual, tc.ceiling)
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
