/*
Copyright 2022 The TestGrid Authors.

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

package tabulator

import (
	"bytes"
	"compress/zlib"
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	"github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	"github.com/GoogleCloudPlatform/testgrid/pkg/updater"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs/fake"
)

func TestTabStatePath(t *testing.T) {
	path := newPathOrDie("gs://bucket/config")
	cases := []struct {
		name           string
		dashboardName  string
		tabName        string
		tabStatePrefix string
		expected       *gcs.Path
	}{
		{
			name:     "basically works",
			expected: path,
		},
		{
			name:          "invalid dashboard name errors",
			dashboardName: "---://foo",
			tabName:       "ok",
		},
		{
			name:          "invalid tab name errors",
			dashboardName: "cool",
			tabName:       "--??!f///",
		},
		{
			name:          "bucket change errors",
			dashboardName: "gs://honey-bucket/config",
			tabName:       "tab",
		},
		{
			name:          "normal behavior works",
			dashboardName: "dashboard",
			tabName:       "some-tab",
			expected:      newPathOrDie("gs://bucket/dashboard/some-tab"),
		},
		{
			name:           "target a subfolder works",
			tabStatePrefix: "tab-state",
			dashboardName:  "dashboard",
			tabName:        "some-tab",
			expected:       newPathOrDie("gs://bucket/tab-state/dashboard/some-tab"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := TabStatePath(*path, tc.tabStatePrefix, tc.dashboardName, tc.tabName)
			switch {
			case err != nil:
				if tc.expected != nil {
					t.Errorf("tabStatePath(%v, %v) got unexpected error: %v", tc.dashboardName, tc.tabName, err)
				}
			case tc.expected == nil:
				t.Errorf("tabStatePath(%v, %v) failed to receive an error", tc.dashboardName, tc.tabName)
			default:
				if diff := cmp.Diff(actual, tc.expected, cmp.AllowUnexported(gcs.Path{})); diff != "" {
					t.Errorf("tabStatePath(%v, %v) got unexpected diff (-have, +want):\n%s", tc.dashboardName, tc.tabName, diff)
				}
			}
		})
	}
}

func newPathOrDie(s string) *gcs.Path {
	p, err := gcs.NewPath(s)
	if err != nil {
		panic(err)
	}
	return p
}

func Test_DropEmptyColumns(t *testing.T) {
	testcases := []struct {
		name     string
		grid     []updater.InflatedColumn
		expected []updater.InflatedColumn
	}{
		{
			name:     "empty",
			grid:     []updater.InflatedColumn{},
			expected: []updater.InflatedColumn{},
		},
		{
			name:     "nil",
			grid:     nil,
			expected: []updater.InflatedColumn{},
		},
		{
			name: "drops empty column",
			grid: []updater.InflatedColumn{
				{
					Column: &statepb.Column{Name: "full"},
					Cells: map[string]updater.Cell{
						"first":  {Result: test_status.TestStatus_BUILD_PASSED},
						"second": {Result: test_status.TestStatus_PASS_WITH_SKIPS},
						"third":  {Result: test_status.TestStatus_FAIL},
					},
				},
				{
					Column: &statepb.Column{Name: "empty"},
					Cells: map[string]updater.Cell{
						"first":  {Result: test_status.TestStatus_NO_RESULT},
						"second": {Result: test_status.TestStatus_NO_RESULT},
						"third":  {Result: test_status.TestStatus_NO_RESULT},
					},
				},
				{
					Column: &statepb.Column{Name: "sparse"},
					Cells: map[string]updater.Cell{
						"first":  {Result: test_status.TestStatus_NO_RESULT},
						"second": {Result: test_status.TestStatus_TIMED_OUT},
						"third":  {Result: test_status.TestStatus_NO_RESULT},
					},
				},
			},
			expected: []updater.InflatedColumn{
				{
					Column: &statepb.Column{Name: "full"},
					Cells: map[string]updater.Cell{
						"first":  {Result: test_status.TestStatus_BUILD_PASSED},
						"second": {Result: test_status.TestStatus_PASS_WITH_SKIPS},
						"third":  {Result: test_status.TestStatus_FAIL},
					},
				},
				{
					Column: &statepb.Column{Name: "sparse"},
					Cells: map[string]updater.Cell{
						"first":  {Result: test_status.TestStatus_NO_RESULT},
						"second": {Result: test_status.TestStatus_TIMED_OUT},
						"third":  {Result: test_status.TestStatus_NO_RESULT},
					},
				},
			},
		},
		{
			name: "drop multiple columns in a row",
			grid: []updater.InflatedColumn{
				{
					Column: &statepb.Column{Name: "empty"},
					Cells: map[string]updater.Cell{
						"first":  {Result: test_status.TestStatus_NO_RESULT},
						"second": {Result: test_status.TestStatus_NO_RESULT},
						"third":  {Result: test_status.TestStatus_NO_RESULT},
					},
				},
				{
					Column: &statepb.Column{Name: "nothing"},
					Cells: map[string]updater.Cell{
						"first":  {Result: test_status.TestStatus_NO_RESULT},
						"second": {Result: test_status.TestStatus_NO_RESULT},
						"third":  {Result: test_status.TestStatus_NO_RESULT},
					},
				},
				{
					Column: &statepb.Column{Name: "full"},
					Cells: map[string]updater.Cell{
						"first":  {Result: test_status.TestStatus_BUILD_PASSED},
						"second": {Result: test_status.TestStatus_PASS_WITH_SKIPS},
						"third":  {Result: test_status.TestStatus_FAIL},
					},
				},
				{
					Column: &statepb.Column{Name: "zero"},
					Cells: map[string]updater.Cell{
						"first":  {Result: test_status.TestStatus_NO_RESULT},
						"second": {Result: test_status.TestStatus_NO_RESULT},
						"third":  {Result: test_status.TestStatus_NO_RESULT},
					},
				},
				{
					Column: &statepb.Column{Name: "nada"},
					Cells: map[string]updater.Cell{
						"first":  {Result: test_status.TestStatus_NO_RESULT},
						"second": {Result: test_status.TestStatus_NO_RESULT},
						"third":  {Result: test_status.TestStatus_NO_RESULT},
					},
				},
			},
			expected: []updater.InflatedColumn{
				{
					Column: &statepb.Column{Name: "full"},
					Cells: map[string]updater.Cell{
						"first":  {Result: test_status.TestStatus_BUILD_PASSED},
						"second": {Result: test_status.TestStatus_PASS_WITH_SKIPS},
						"third":  {Result: test_status.TestStatus_FAIL},
					},
				},
			},
		},
		{
			name: "don't drop everything",
			grid: []updater.InflatedColumn{
				{
					Column: &statepb.Column{Name: "first"},
					Cells: map[string]updater.Cell{
						"first":  {Result: test_status.TestStatus_NO_RESULT},
						"second": {Result: test_status.TestStatus_NO_RESULT},
						"third":  {Result: test_status.TestStatus_NO_RESULT},
					},
				},
				{
					Column: &statepb.Column{Name: "empty"},
					Cells: map[string]updater.Cell{
						"first":  {Result: test_status.TestStatus_NO_RESULT},
						"second": {Result: test_status.TestStatus_NO_RESULT},
						"third":  {Result: test_status.TestStatus_NO_RESULT},
					},
				},
				{
					Column: &statepb.Column{Name: "nada"},
					Cells: map[string]updater.Cell{
						"first":  {Result: test_status.TestStatus_NO_RESULT},
						"second": {Result: test_status.TestStatus_NO_RESULT},
						"third":  {Result: test_status.TestStatus_NO_RESULT},
					},
				},
			},
			expected: []updater.InflatedColumn{
				{
					Column: &statepb.Column{Name: "first"},
					Cells: map[string]updater.Cell{
						"first":  {Result: test_status.TestStatus_NO_RESULT},
						"second": {Result: test_status.TestStatus_NO_RESULT},
						"third":  {Result: test_status.TestStatus_NO_RESULT},
					},
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			actual := dropEmptyColumns(tc.grid)
			if diff := cmp.Diff(actual, tc.expected, protocmp.Transform()); diff != "" {
				t.Errorf("(-got, +want): %s", diff)
			}
		})
	}
}

func Test_Tabulate(t *testing.T) {
	var exampleGrid statepb.Grid
	updater.AppendColumn(&exampleGrid, map[string]*statepb.Row{}, updater.InflatedColumn{
		Column: &statepb.Column{Name: "full"},
		Cells: map[string]updater.Cell{
			"first":  {Result: test_status.TestStatus_BUILD_PASSED},
			"second": {Result: test_status.TestStatus_PASS_WITH_SKIPS},
			"third":  {Result: test_status.TestStatus_FAIL},
		},
	})

	tabConfig := configpb.DashboardTab{
		Name: "no filters",
	}

	testcases := []struct {
		name          string
		existingState fake.Object
		confirm       bool
		expectError   bool
		expectUpload  bool
	}{
		{
			name:        "Fails if data is missing",
			expectError: true,
		},
		{
			name: "Does not write without confirm",
			existingState: func() fake.Object {
				return fake.Object{
					Data: string(compress(gridBuf(&exampleGrid))),
				}
			}(),
			confirm:     false,
			expectError: false,
		},
		{
			name: "Fails with uncompressed grid",
			existingState: func() fake.Object {
				return fake.Object{
					Data: string(gridBuf(&exampleGrid)),
				}
			}(),
			expectError: true,
		},
		{
			name: "Writes data when upload is expected",
			existingState: func() fake.Object {
				return fake.Object{
					Data: string(compress(gridBuf(&exampleGrid))),
				}
			}(),
			confirm:      true,
			expectUpload: true,
		},
	}

	fromPath := newPathOrDie("gs://example/from")
	toPath := newPathOrDie("gs://example/to")

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := fake.UploadClient{
				Client: fake.Client{
					Opener: fake.Opener{
						*fromPath: tc.existingState,
					},
				},
				Uploader: fake.Uploader{},
			}

			err := tabulate(ctx, client, &tabConfig, *fromPath, *toPath, tc.confirm, true)
			if tc.expectError == (err == nil) {
				t.Errorf("Wrong error: want %t, got %v", tc.expectError, err)
			}
			res, ok := client.Uploader[*toPath]
			uploaded := ok && (len(res.Buf) != 0)
			if uploaded != tc.expectUpload {
				t.Errorf("Wrong upload: want %t, got %v", tc.expectUpload, ok)
			}
		})
	}
}

func gridBuf(grid *statepb.Grid) []byte {
	buf, err := proto.Marshal(grid)
	if err != nil {
		panic(err)
	}
	return buf
}

func compress(buf []byte) []byte {
	var zbuf bytes.Buffer
	zw := zlib.NewWriter(&zbuf)
	if _, err := zw.Write(buf); err != nil {
		panic(err)
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	return zbuf.Bytes()
}
