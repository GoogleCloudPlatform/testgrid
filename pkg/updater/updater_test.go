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

	"github.com/GoogleCloudPlatform/testgrid/config"
	"github.com/GoogleCloudPlatform/testgrid/metadata"
	_ "github.com/GoogleCloudPlatform/testgrid/metadata/junit"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	statuspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
)

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
					buf:          mustGrid(&statepb.Grid{}),
					cacheControl: "no-cache",
					worldRead:    gcs.DefaultAcl,
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
				fakeUploader: fakeUploader{},
				fakeClient: fakeClient{
					fakeLister: fakeLister{},
					fakeOpener: fakeOpener{},
				},
			}

			client.fakeOpener[configPath] = fakeObject{
				data: func() string {
					b, err := config.MarshalBytes(&tc.config)
					if err != nil {
						t.Fatalf("config.MarshalBytes() errored: %v", err)
					}
					return string(b)
				}(),
				readErr: tc.configErr,
			}

			for _, group := range tc.config.TestGroups {
				builds, ok := tc.builds[group.Name]
				if !ok {
					continue
				}
				buildsPath := newPathOrDie("gs://" + group.GcsPrefix)
				fi := client.fakeLister[buildsPath]
				for _, build := range client.addBuilds(buildsPath, builds...) {
					fi.objects = append(fi.objects, storage.ObjectAttrs{
						Prefix: build.Path.Object(),
					})
				}
				client.fakeLister[buildsPath] = fi
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
			)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("Update() got unexpected error: %v", err)
				}
			case tc.err:
				t.Error("Update() failed to receive an errro")
			default:
				actual := client.fakeUploader
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

type fakeUploadClient struct {
	fakeClient
	fakeUploader
	fakeStater
}

type fakeStat struct {
	err   error
	attrs storage.ObjectAttrs
}

type fakeStater map[gcs.Path]fakeStat

func (fs fakeStater) Stat(ctx context.Context, path gcs.Path) (*storage.ObjectAttrs, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("injected interrupt: %w", err)
	}

	ret, ok := fs[path]
	if !ok {
		return nil, storage.ErrObjectNotExist
	}
	if ret.err != nil {
		return nil, fmt.Errorf("injected upload error: %w", ret.err)
	}
	return &ret.attrs, nil
}

type fakeUploader map[gcs.Path]fakeUpload

func (fuc fakeUploader) Upload(ctx context.Context, path gcs.Path, buf []byte, worldRead bool, cacheControl string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("injected interrupt: %w", err)
	}
	if err := fuc[path].err; err != nil {
		return fmt.Errorf("injected upload error: %w", err)
	}

	fuc[path] = fakeUpload{
		buf:          buf,
		cacheControl: cacheControl,
		worldRead:    worldRead,
	}
	return nil
}

type fakeUpload struct {
	buf          []byte
	cacheControl string
	worldRead    bool
	err          error
}

func jsonStarted(stamp int64) *fakeObject {
	return &fakeObject{
		data: jsonData(metadata.Started{Timestamp: stamp}),
	}
}

func jsonFinished(stamp int64, passed bool, meta metadata.Metadata) *fakeObject {
	return &fakeObject{
		data: jsonData(metadata.Finished{
			Timestamp: &stamp,
			Passed:    &passed,
			Metadata:  meta,
		}),
	}
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
		t.Fatal("bad path: %v", err)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			client := fakeStater{}
			for name, updated := range tc.updated {
				var stat fakeStat
				if updated == nil {
					stat.err = errors.New("error")
				} else {
					stat.attrs.Updated = *updated
				}
				path, err := testGroupPath(*configPath, gridPrefix, name)
				if err != nil {
					t.Fatal("bad group path: %v", err)
				}
				client[*path] = stat
			}
			defer cancel()
			sortGroups(ctx, logrus.WithField("case", tc.name), client, *configPath, gridPrefix, tc.groups)
		})
	}
}

func TestTruncateRunning(t *testing.T) {
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
				{column: &statepb.Column{Build: "this"}},
				{column: &statepb.Column{Build: "that"}},
				{column: &statepb.Column{Build: "another"}},
			},
		},
		{
			name: "keep everything completed",
			cols: []inflatedColumn{
				{
					column: &statepb.Column{Build: "passed"},
					cells:  map[string]cell{"Overall": {result: statuspb.TestStatus_PASS}},
				},
				{
					column: &statepb.Column{Build: "failed"},
					cells:  map[string]cell{"Overall": {result: statuspb.TestStatus_FAIL}},
				},
			},
		},
		{
			name: "drop everything before oldest running",
			cols: []inflatedColumn{
				{column: &statepb.Column{Build: "this1"}},
				{column: &statepb.Column{Build: "this2"}},
				{
					column: &statepb.Column{Build: "running1"},
					cells:  map[string]cell{"Overall": {result: statuspb.TestStatus_RUNNING}},
				},
				{column: &statepb.Column{Build: "this3"}},
				{
					column: &statepb.Column{Build: "running2"},
					cells:  map[string]cell{"Overall": {result: statuspb.TestStatus_RUNNING}},
				},
				{column: &statepb.Column{Build: "this4"}},
				{column: &statepb.Column{Build: "this5"}},
				{column: &statepb.Column{Build: "this6"}},
				{column: &statepb.Column{Build: "this7"}},
			},
			expected: func(cols []inflatedColumn) []inflatedColumn {
				return cols[5:] // this4 and earlier
			},
		},
		{
			name: "drop all as all are running",
			cols: []inflatedColumn{
				{
					column: &statepb.Column{Build: "running1"},
					cells:  map[string]cell{"Overall": {result: statuspb.TestStatus_RUNNING}},
				},
				{
					column: &statepb.Column{Build: "running2"},
					cells:  map[string]cell{"Overall": {result: statuspb.TestStatus_RUNNING}},
				},
			},
			expected: func(cols []inflatedColumn) []inflatedColumn {
				return cols[2:]
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
			if diff := cmp.Diff(actual, expected, cmp.AllowUnexported(inflatedColumn{}, cell{}), protocmp.Transform()); diff != "" {
				t.Errorf("truncateRunning() got unexpected diff:\n%s", diff)
			}
		})
	}
}

func TestUpdateGCSGroup(t *testing.T) {
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
			expected: &fakeUpload{
				buf: mustGrid(&statepb.Grid{
					Columns: []*statepb.Column{
						{
							Build:   "99",
							Started: float64(now+99) * 1000,
							Extra:   []string{""},
						},
						{
							Build:   "80",
							Started: float64(now+80) * 1000,
							Extra:   []string{"build80"},
						},
						{
							Build:   "50",
							Started: float64(now+50) * 1000,
							Extra:   []string{"build50"},
						},
						{
							Build:   "10",
							Started: float64(now+10) * 1000,
							Extra:   []string{"build10"},
						},
					},
					Rows: []*statepb.Row{
						setupRow(
							&statepb.Row{
								Name: "Overall",
								Id:   "Overall",
							},
							cell{
								result:  statuspb.TestStatus_RUNNING,
								message: "Build still running...",
								icon:    "R",
							},
							cell{
								result:  statuspb.TestStatus_PASS,
								metrics: setElapsed(nil, 1),
							},
							cell{
								result:  statuspb.TestStatus_FAIL,
								metrics: setElapsed(nil, 1),
							},
							cell{
								result:  statuspb.TestStatus_PASS,
								metrics: setElapsed(nil, 1),
							},
						),
						setupRow(
							&statepb.Row{
								Name: "flaky",
								Id:   "flaky",
							},
							cell{result: statuspb.TestStatus_NO_RESULT},
							cell{result: statuspb.TestStatus_PASS},
							cell{
								result:  statuspb.TestStatus_FAIL,
								message: "flaky",
								icon:    "F",
							},
							cell{result: statuspb.TestStatus_PASS},
						),
						setupRow(
							&statepb.Row{
								Name: "good1",
								Id:   "good1",
							},
							cell{result: statuspb.TestStatus_NO_RESULT},
							cell{result: statuspb.TestStatus_PASS},
							cell{result: statuspb.TestStatus_PASS},
							cell{result: statuspb.TestStatus_PASS},
						),
						setupRow(
							&statepb.Row{
								Name: "good2",
								Id:   "good2",
							},
							cell{result: statuspb.TestStatus_NO_RESULT},
							cell{result: statuspb.TestStatus_PASS},
							cell{result: statuspb.TestStatus_PASS},
							cell{result: statuspb.TestStatus_PASS},
						),
					},
				}),
				cacheControl: "no-cache",
				worldRead:    gcs.DefaultAcl,
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
				fakeUploader: fakeUploader{},
				fakeClient: fakeClient{
					fakeLister: fakeLister{},
					fakeOpener: fakeOpener{},
				},
			}

			buildsPath := newPathOrDie("gs://" + tc.group.GcsPrefix)
			fi := client.fakeLister[buildsPath]
			for _, build := range client.addBuilds(buildsPath, tc.builds...) {
				fi.objects = append(fi.objects, storage.ObjectAttrs{
					Prefix: build.Path.Object(),
				})
			}
			client.fakeLister[buildsPath] = fi

			err := updateGCSGroup(
				ctx,
				logrus.WithField("test", tc.name),
				client,
				&tc.group,
				uploadPath,
				tc.concurrency,
				!tc.skipWrite,
				*tc.buildTimeout,
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
				actual := client.fakeUploader
				diff := cmp.Diff(actual, expected, cmp.AllowUnexported(gcs.Path{}, fakeUpload{}), protocmp.Transform())
				if diff == "" {
					return
				}
				t.Errorf("updateGroup() got unexpected diff (-have, +want):\n%s", diff)
				fakeDownloader := fakeOpener{
					uploadPath: {data: string(actual[uploadPath].buf)},
				}
				actualGrid, err := downloadGrid(ctx, fakeDownloader, uploadPath)
				if err != nil {
					t.Errorf("actual downloadGrid() got unexpected error: %v", err)
				}
				fakeDownloader[uploadPath] = fakeObject{data: string(tc.expected.buf)}
				expectedGrid, err := downloadGrid(ctx, fakeDownloader, uploadPath)
				if err != nil {
					t.Errorf("expected downloadGrid() got unexpected error: %v", err)
				}
				diff = cmp.Diff(actualGrid, expectedGrid, protocmp.Transform())
				if diff == "" {
					return
				}
				t.Errorf("downloadGrid() got unexpected diff (-have, +want):\n%s", diff)
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
					column: &statepb.Column{
						Build: "hello",
					},
					cells: map[string]cell{
						"this": {result: statuspb.TestStatus_PASS},
					},
				},
				{
					column: &statepb.Column{
						Build: "world",
					},
					cells: map[string]cell{
						"that": {result: statuspb.TestStatus_FAIL},
					},
				},
			},
			expected: []inflatedColumn{
				{
					column: &statepb.Column{
						Build: "hello",
					},
					cells: map[string]cell{
						"this": {result: statuspb.TestStatus_PASS},
					},
				},
				{
					column: &statepb.Column{
						Build: "world",
					},
					cells: map[string]cell{
						"that": {result: statuspb.TestStatus_FAIL},
					},
				},
			},
		},
		{
			name: "only old cols",
			oldCols: []inflatedColumn{
				{
					column: &statepb.Column{
						Build: "ancient",
					},
					cells: map[string]cell{
						"this": {result: statuspb.TestStatus_PASS},
					},
				},
				{
					column: &statepb.Column{
						Build: "graveyard",
					},
					cells: map[string]cell{
						"that": {result: statuspb.TestStatus_FAIL},
					},
				},
			},
			expected: []inflatedColumn{
				{
					column: &statepb.Column{
						Build: "ancient",
					},
					cells: map[string]cell{
						"this": {result: statuspb.TestStatus_PASS},
					},
				},
				{
					column: &statepb.Column{
						Build: "graveyard",
					},
					cells: map[string]cell{
						"that": {result: statuspb.TestStatus_FAIL},
					},
				},
			},
		},
		{
			name: "accept all when old are all older than new",
			newCols: []inflatedColumn{
				{
					column: &statepb.Column{
						Build:   "new-1000",
						Started: 1000,
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_RUNNING},
					},
				},
				{
					column: &statepb.Column{
						Build:   "new-900",
						Started: 900,
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_PASS},
					},
				},
			},
			oldCols: []inflatedColumn{
				{
					column: &statepb.Column{
						Build:   "old-50",
						Started: 50,
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_FAIL},
					},
				},
				{
					column: &statepb.Column{
						Build:   "old-40",
						Started: 40,
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_FLAKY},
					},
				},
			},
			expected: []inflatedColumn{
				{
					column: &statepb.Column{
						Build:   "new-1000",
						Started: 1000,
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_RUNNING},
					},
				},
				{
					column: &statepb.Column{
						Build:   "new-900",
						Started: 900,
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_PASS},
					},
				},
				{
					column: &statepb.Column{
						Build:   "old-50",
						Started: 50,
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_FAIL},
					},
				},
				{
					column: &statepb.Column{
						Build:   "old-40",
						Started: 40,
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_FLAKY},
					},
				},
			},
		},
		{
			name: "accept all new and oldest old, reject olds which are >= new",
			newCols: []inflatedColumn{
				{
					column: &statepb.Column{
						Build:   "new-1000",
						Started: 1000,
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_RUNNING},
					},
				},
				{
					column: &statepb.Column{
						Build:   "new-900",
						Started: 900,
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_PASS},
					},
				},
				{
					column: &statepb.Column{
						Build:   "new-200",
						Started: 200,
					},
					cells: map[string]cell{
						"test": {message: "accept new 200"},
					},
				},
				{
					column: &statepb.Column{
						Build:   "new-100",
						Started: 100,
					},
					cells: map[string]cell{
						"test": {message: "accept new 100"},
					},
				},
			},
			oldCols: []inflatedColumn{
				{
					column: &statepb.Column{
						Build:   "old-500",
						Started: 500,
					},
					cells: map[string]cell{
						"test": {message: "reject old"},
					},
				},
				{
					column: &statepb.Column{
						Build:   "old-150",
						Started: 150,
					},
					cells: map[string]cell{
						"test": {message: "reject old"},
					},
				},
				{
					column: &statepb.Column{
						Build:   "old-50",
						Started: 50,
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_FAIL},
					},
				},
				{
					column: &statepb.Column{
						Build:   "old-40",
						Started: 40,
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_FLAKY},
					},
				},
			},
			expected: []inflatedColumn{
				{
					column: &statepb.Column{
						Build:   "new-1000",
						Started: 1000,
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_RUNNING},
					},
				},
				{
					column: &statepb.Column{
						Build:   "new-900",
						Started: 900,
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_PASS},
					},
				},
				{
					column: &statepb.Column{
						Build:   "new-200",
						Started: 200,
					},
					cells: map[string]cell{
						"test": {message: "accept new 200"},
					},
				},
				{
					column: &statepb.Column{
						Build:   "new-100",
						Started: 100,
					},
					cells: map[string]cell{
						"test": {message: "accept new 100"},
					},
				},
				{
					column: &statepb.Column{
						Build:   "old-50",
						Started: 50,
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_FAIL},
					},
				},
				{
					column: &statepb.Column{
						Build:   "old-40",
						Started: 40,
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_FLAKY},
					},
				},
			},
		},
		{
			name: "accept all new and oldest old, reject old duplicates",
			newCols: []inflatedColumn{
				{
					column: &statepb.Column{
						Build:   "new-1000",
						Started: 1000,
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_RUNNING},
					},
				},
				{
					column: &statepb.Column{
						Build:   "new-900",
						Started: 900,
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_PASS},
					},
				},
				{
					column: &statepb.Column{
						Build:   "shared-110",
						Started: 110,
					},
					cells: map[string]cell{
						"test": {message: "accept new 110"},
					},
				},
				{
					column: &statepb.Column{
						Build:   "shared-100",
						Started: 100,
					},
					cells: map[string]cell{
						"test": {message: "accept new 100"},
					},
				},
				{
					column: &statepb.Column{
						Build:   "shared-90",
						Started: 90,
					},
					cells: map[string]cell{
						"test": {message: "accept new 90"},
					},
				},
			},
			oldCols: []inflatedColumn{
				{
					column: &statepb.Column{
						Build:   "shared-110",
						Started: 110,
						Extra:   []string{"reject old"},
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_FAIL},
					},
				},
				{
					column: &statepb.Column{
						Build:   "shared-100",
						Started: 100,
						Extra:   []string{"reject old"},
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_FAIL},
					},
				},
				{
					column: &statepb.Column{
						Build:   "shared-90",
						Started: 90,
						Extra:   []string{"reject old"},
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_FAIL},
					},
				},
				{
					column: &statepb.Column{
						Build:   "old-50",
						Started: 50,
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_FAIL},
					},
				},
				{
					column: &statepb.Column{
						Build:   "old-40",
						Started: 40,
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_FLAKY},
					},
				},
			},
			expected: []inflatedColumn{
				{
					column: &statepb.Column{
						Build:   "new-1000",
						Started: 1000,
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_RUNNING},
					},
				},
				{
					column: &statepb.Column{
						Build:   "new-900",
						Started: 900,
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_PASS},
					},
				},
				{
					column: &statepb.Column{
						Build:   "shared-110",
						Started: 110,
					},
					cells: map[string]cell{
						"test": {message: "accept new 110"},
					},
				},
				{
					column: &statepb.Column{
						Build:   "shared-100",
						Started: 100,
					},
					cells: map[string]cell{
						"test": {message: "accept new 100"},
					},
				},
				{
					column: &statepb.Column{
						Build:   "shared-90",
						Started: 90,
					},
					cells: map[string]cell{
						"test": {message: "accept new 90"},
					},
				},
				{
					column: &statepb.Column{
						Build:   "old-50",
						Started: 50,
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_FAIL},
					},
				},
				{
					column: &statepb.Column{
						Build:   "old-40",
						Started: 40,
					},
					cells: map[string]cell{
						"test": {result: statuspb.TestStatus_FLAKY},
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
					column: &statepb.Column{Build: "15"},
					cells: map[string]cell{
						"green": {
							result: statuspb.TestStatus_PASS,
						},
						"red": {
							result: statuspb.TestStatus_FAIL,
						},
						"only-15": {
							result: statuspb.TestStatus_FLAKY,
						},
					},
				},
				{
					column: &statepb.Column{Build: "10"},
					cells: map[string]cell{
						"full": {
							result:  statuspb.TestStatus_PASS,
							cellID:  "cell",
							icon:    "icon",
							message: "message",
							metrics: map[string]float64{
								"elapsed": 1,
								"keys":    2,
							},
						},
						"green": {
							result: statuspb.TestStatus_PASS,
						},
						"red": {
							result: statuspb.TestStatus_FAIL,
						},
						"only-10": {
							result: statuspb.TestStatus_FLAKY,
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
							result:  statuspb.TestStatus_PASS,
							cellID:  "cell",
							icon:    "icon",
							message: "message",
							metrics: map[string]float64{
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
						cell{result: statuspb.TestStatus_PASS},
						cell{result: statuspb.TestStatus_PASS},
					),
					setupRow(
						&statepb.Row{
							Name: "only-10",
							Id:   "only-10",
						},
						emptyCell,
						cell{result: statuspb.TestStatus_FLAKY},
					),
					setupRow(
						&statepb.Row{
							Name: "only-15",
							Id:   "only-15",
						},
						cell{result: statuspb.TestStatus_FLAKY},
						emptyCell,
					),
					setupRow(
						&statepb.Row{
							Name: "red",
							Id:   "red",
						},
						cell{result: statuspb.TestStatus_FAIL},
						cell{result: statuspb.TestStatus_FAIL},
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
					column: &statepb.Column{Build: "4"},
					cells: map[string]cell{
						"just-flaky": {
							result: statuspb.TestStatus_FAIL,
						},
						"broken": {
							result: statuspb.TestStatus_FAIL,
						},
					},
				},
				{
					column: &statepb.Column{Build: "3"},
					cells: map[string]cell{
						"just-flaky": {
							result: statuspb.TestStatus_PASS,
						},
						"broken": {
							result: statuspb.TestStatus_FAIL,
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
						cell{result: statuspb.TestStatus_FAIL},
						cell{result: statuspb.TestStatus_FAIL},
					),
					setupRow(
						&statepb.Row{
							Name: "just-flaky",
							Id:   "just-flaky",
						},
						cell{result: statuspb.TestStatus_FAIL},
						cell{result: statuspb.TestStatus_PASS},
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
					column: &statepb.Column{Build: "4"},
					cells: map[string]cell{
						"still-broken": {
							result: statuspb.TestStatus_PASS,
						},
						"fixed": {
							result: statuspb.TestStatus_PASS,
						},
					},
				},
				{
					column: &statepb.Column{Build: "3"},
					cells: map[string]cell{
						"still-broken": {
							result: statuspb.TestStatus_FAIL,
						},
						"fixed": {
							result: statuspb.TestStatus_PASS,
						},
					},
				},
				{
					column: &statepb.Column{Build: "2"},
					cells: map[string]cell{
						"still-broken": {
							result: statuspb.TestStatus_FAIL,
						},
						"fixed": {
							result: statuspb.TestStatus_FAIL,
						},
					},
				},
				{
					column: &statepb.Column{Build: "1"},
					cells: map[string]cell{
						"still-broken": {
							result: statuspb.TestStatus_FAIL,
						},
						"fixed": {
							result: statuspb.TestStatus_FAIL,
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
						cell{result: statuspb.TestStatus_PASS},
						cell{result: statuspb.TestStatus_PASS},
						cell{result: statuspb.TestStatus_FAIL},
						cell{result: statuspb.TestStatus_FAIL},
					),
					setupRow(
						&statepb.Row{
							Name: "still-broken",
							Id:   "still-broken",
						},
						cell{result: statuspb.TestStatus_PASS},
						cell{result: statuspb.TestStatus_FAIL},
						cell{result: statuspb.TestStatus_FAIL},
						cell{result: statuspb.TestStatus_FAIL},
					),
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := constructGrid(&tc.group, tc.cols)
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
				result: statuspb.TestStatus_PASS,
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
				result:  statuspb.TestStatus_PASS,
				cellID:  "cell-id",
				message: "hi",
				icon:    "there",
				metrics: map[string]float64{
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
				result:  statuspb.TestStatus_FLAKY,
				message: "echo",
				cellID:  "again and",
				icon:    "keeps going",
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
				result: statuspb.TestStatus_PASS,
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
			name: "append no result (results, cellIDs, no messages or icons)",
			row: statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FLAKY), 3,
				},
				CellIds:  []string{"", "", ""},
				Messages: []string{"", "", ""},
				Icons:    []string{"", "", ""},
			},
			cell: cell{
				result: statuspb.TestStatus_NO_RESULT,
			},
			count: 2,
			expected: statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FLAKY), 3,
					int32(statuspb.TestStatus_NO_RESULT), 2,
				},
				CellIds:  []string{"", "", "", "", ""},
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
				result: statuspb.TestStatus_PASS,
				metrics: map[string]float64{
					"continued-series": 5.1,
					"new-series":       5.2,
				},
			},
			count: 1,
			expected: statepb.Row{
				Results:  []int32{int32(statuspb.TestStatus_PASS), 6},
				CellIds:  []string{"", "", "", "", "c", ""},
				Messages: []string{"", "", "", "", "m", ""},
				Icons:    []string{"", "", "", "", "i", ""},
				Metric: []string{
					"continued-series",
					"new-series",
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
				},
			},
		},
		{
			name:  "add a bunch of initial blank columns (eg a deleted row)",
			cell:  emptyCell,
			count: 7,
			expected: statepb.Row{
				Results: []int32{int32(statuspb.TestStatus_NO_RESULT), 7},
				CellIds: []string{"", "", "", "", "", "", ""},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			appendCell(&tc.row, tc.cell, tc.count)
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
	for _, c := range cells {
		appendCell(row, c, 1)
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
			col:  inflatedColumn{column: &statepb.Column{Build: "10"}},
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
			col: inflatedColumn{column: &statepb.Column{Build: "20"}},
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
				column: &statepb.Column{Build: "10"},
				cells: map[string]cell{
					"hello": {
						result: statuspb.TestStatus_PASS,
						cellID: "yes",
						metrics: map[string]float64{
							"answer": 42,
						},
					},
					"world": {
						result:  statuspb.TestStatus_FAIL,
						message: "boom",
						icon:    "X",
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
							result:  statuspb.TestStatus_PASS,
							cellID:  "yes",
							metrics: map[string]float64{"answer": 42},
						}),
					setupRow(&statepb.Row{
						Name: "world",
						Id:   "world",
					}, cell{
						result:  statuspb.TestStatus_FAIL,
						message: "boom",
						icon:    "X",
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
						cell{result: statuspb.TestStatus_PASS},
						cell{result: statuspb.TestStatus_PASS},
						cell{result: statuspb.TestStatus_PASS},
					),
					setupRow(
						&statepb.Row{Name: "always"},
						cell{result: statuspb.TestStatus_PASS},
						cell{result: statuspb.TestStatus_PASS},
						cell{result: statuspb.TestStatus_PASS},
					),
				},
			},
			col: inflatedColumn{
				column: &statepb.Column{Build: "20"},
				cells: map[string]cell{
					"always": {result: statuspb.TestStatus_PASS},
					"new":    {result: statuspb.TestStatus_PASS},
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
						cell{result: statuspb.TestStatus_PASS},
						cell{result: statuspb.TestStatus_PASS},
						cell{result: statuspb.TestStatus_PASS},
						emptyCell,
					),
					setupRow(
						&statepb.Row{Name: "always"},
						cell{result: statuspb.TestStatus_PASS},
						cell{result: statuspb.TestStatus_PASS},
						cell{result: statuspb.TestStatus_PASS},
						cell{result: statuspb.TestStatus_PASS},
					),
					setupRow(
						&statepb.Row{
							Name: "new",
							Id:   "new",
						},
						emptyCell,
						emptyCell,
						emptyCell,
						cell{result: statuspb.TestStatus_PASS},
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
				Messages: []string{"hello", "no", "no again", "very wrong"},
				CellIds:  []string{"yes", "no", "no again", "very wrong"},
			},
			failOpen: 3,
			expected: alertInfo(3, "hello", "yes", columns[2], columns[3]),
		},
		{
			name: "too few passes do not close",
			row: statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_PASS), 2,
					int32(statuspb.TestStatus_FAIL), 4,
				},
				Messages: []string{"nope", "no", "yay", "very wrong"},
				CellIds:  []string{"wrong", "no", "yep", "very wrong"},
			},
			failOpen:  1,
			passClose: 3,
			expected:  alertInfo(4, "yay", "yep", columns[5], nil),
		},
		{
			name: "flakes do not close",
			row: statepb.Row{
				Results: []int32{
					int32(statuspb.TestStatus_FLAKY), 2,
					int32(statuspb.TestStatus_FAIL), 4,
				},
				Messages: []string{"nope", "no", "yay", "very wrong"},
				CellIds:  []string{"wrong", "no", "yep", "very wrong"},
			},
			failOpen: 1,
			expected: alertInfo(4, "yay", "yep", columns[5], nil),
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
				Messages: []string{"nope", "no", "buu", "wrong", "this one"},
				CellIds:  []string{"wrong", "no", "buzz", "wrong2", "good job"},
			},
			failOpen:  2,
			passClose: 2,
			expected:  alertInfo(4, "this one", "good job", columns[5], nil),
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
				Messages: []string{"yay", "no", "buu", "wrong", "nono"},
				CellIds:  []string{"yay-cell", "no", "buzz", "wrong2", "nada"},
			},
			failOpen:  5,
			passClose: 2,
			expected:  alertInfo(5, "yay", "yay-cell", columns[5], nil),
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
			expected: alertInfo(5, "fail1-expected", "yep", columns[5], nil),
		},
	}

	for _, tc := range cases {
		if actual := alertRow(columns, &tc.row, tc.failOpen, tc.passClose); !reflect.DeepEqual(actual, tc.expected) {
			t.Errorf("%s alert %s != expected %s", tc.name, actual, tc.expected)
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
