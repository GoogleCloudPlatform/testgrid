/*
Copyright 2019 The Kubernetes Authors.

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

package summarizer

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"reflect"
	"sort"
	"testing"
	"time"

	"bitbucket.org/creachadair/stringset"
	"cloud.google.com/go/storage"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/GoogleCloudPlatform/testgrid/internal/result"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	summarypb "github.com/GoogleCloudPlatform/testgrid/pb/summary"
	statuspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs/fake"
)

type fakeGroup struct {
	group *configpb.TestGroup
	grid  *statepb.Grid
	mod   time.Time
	gen   int64
	err   error
}

func TestUpdate(t *testing.T) {
	cases := []struct {
		name string
	}{
		{},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// TODO(fejta): implement
		})
	}
}

func TestUpdateDashboard(t *testing.T) {
	cases := []struct {
		name     string
		dash     *configpb.Dashboard
		groups   map[string]fakeGroup
		tabMode  bool
		expected *summarypb.DashboardSummary
		err      bool
	}{
		{
			name: "basically works",
			dash: &configpb.Dashboard{
				Name: "stale-dashboard",
				DashboardTab: []*configpb.DashboardTab{
					{
						Name:          "stale-tab",
						TestGroupName: "foo-group",
						AlertOptions: &configpb.DashboardTabAlertOptions{
							AlertStaleResultsHours: 1,
						},
					},
				},
			},
			groups: map[string]fakeGroup{
				"foo-group": {
					group: &configpb.TestGroup{},
					grid:  &statepb.Grid{},
					mod:   time.Unix(1000, 0),
				},
			},
			expected: &summarypb.DashboardSummary{
				TabSummaries: []*summarypb.DashboardTabSummary{
					{
						DashboardName:       "stale-dashboard",
						DashboardTabName:    "stale-tab",
						LastUpdateTimestamp: 1000,
						Alert:               noRuns,
						OverallStatus:       summarypb.DashboardTabSummary_STALE,
						Status:              noRuns,
						LatestGreen:         noGreens,
					},
				},
			},
		},
		{
			name: "still update working tabs when some tabs fail",
			dash: &configpb.Dashboard{
				Name: "a-dashboard",
				DashboardTab: []*configpb.DashboardTab{
					{
						Name:          "working",
						TestGroupName: "working-group",
						AlertOptions: &configpb.DashboardTabAlertOptions{
							AlertStaleResultsHours: 1,
						},
					},
					{
						Name:          "missing-tab",
						TestGroupName: "group-not-present",
						AlertOptions: &configpb.DashboardTabAlertOptions{
							AlertStaleResultsHours: 1,
						},
					},
					{
						Name:          "error-tab",
						TestGroupName: "has-errors",
						AlertOptions: &configpb.DashboardTabAlertOptions{
							AlertStaleResultsHours: 1,
						},
					},
					{
						Name:          "still-working",
						TestGroupName: "working-group",
						AlertOptions: &configpb.DashboardTabAlertOptions{
							AlertStaleResultsHours: 1,
						},
					},
				},
			},
			groups: map[string]fakeGroup{
				"working-group": {
					mod:   time.Unix(1000, 0),
					group: &configpb.TestGroup{},
					grid:  &statepb.Grid{},
				},
				"has-errors": {
					err:   errors.New("tragedy"),
					group: &configpb.TestGroup{},
					grid:  &statepb.Grid{},
				},
			},
			expected: &summarypb.DashboardSummary{
				TabSummaries: []*summarypb.DashboardTabSummary{
					{
						DashboardName:       "a-dashboard",
						DashboardTabName:    "working",
						LastUpdateTimestamp: 1000,
						Alert:               noRuns,
						Status:              noRuns,
						OverallStatus:       summarypb.DashboardTabSummary_STALE,
						LatestGreen:         noGreens,
					},
					tabStatus("a-dashboard", "missing-tab", `Test group does not exist: "group-not-present"`),
					tabStatus("a-dashboard", "error-tab", fmt.Sprintf("Error attempting to summarize tab: load has-errors: open: tragedy")),
					{
						DashboardName:       "a-dashboard",
						DashboardTabName:    "still-working",
						LastUpdateTimestamp: 1000,
						Alert:               noRuns,
						Status:              noRuns,
						OverallStatus:       summarypb.DashboardTabSummary_STALE,
						LatestGreen:         noGreens,
					},
				},
			},
			err: true,
		},
		{
			name: "bug url",
			dash: &configpb.Dashboard{
				Name: "a-dashboard",
				DashboardTab: []*configpb.DashboardTab{
					{
						Name:          "none",
						TestGroupName: "a-group",
					},
					{
						Name:            "empty",
						TestGroupName:   "a-group",
						OpenBugTemplate: &configpb.LinkTemplate{},
					},
					{
						Name:          "url",
						TestGroupName: "a-group",
						OpenBugTemplate: &configpb.LinkTemplate{
							Url: "http://some-bugs/",
						},
					},
					{
						Name:          "url-options-empty",
						TestGroupName: "a-group",
						OpenBugTemplate: &configpb.LinkTemplate{
							Url:     "http://more-bugs/",
							Options: []*configpb.LinkOptionsTemplate{},
						},
					},
					{
						Name:          "url-options",
						TestGroupName: "a-group",
						OpenBugTemplate: &configpb.LinkTemplate{
							Url: "http://ooh-bugs/",
							Options: []*configpb.LinkOptionsTemplate{
								{
									Key:   "id",
									Value: "warble",
								},
								{
									Key:   "name",
									Value: "garble",
								},
							},
						},
					},
				},
			},
			groups: map[string]fakeGroup{
				"a-group": {
					mod:   time.Unix(1000, 0),
					group: &configpb.TestGroup{},
					grid:  &statepb.Grid{},
				},
			},
			expected: &summarypb.DashboardSummary{
				TabSummaries: []*summarypb.DashboardTabSummary{
					{
						DashboardName:       "a-dashboard",
						DashboardTabName:    "none",
						LastUpdateTimestamp: 1000,
						Status:              noRuns,
						OverallStatus:       summarypb.DashboardTabSummary_UNKNOWN,
						LatestGreen:         noGreens,
						BugUrl:              "",
					},
					{
						DashboardName:       "a-dashboard",
						DashboardTabName:    "empty",
						LastUpdateTimestamp: 1000,
						Status:              noRuns,
						OverallStatus:       summarypb.DashboardTabSummary_UNKNOWN,
						LatestGreen:         noGreens,
						BugUrl:              "",
					},
					{
						DashboardName:       "a-dashboard",
						DashboardTabName:    "url",
						LastUpdateTimestamp: 1000,
						Status:              noRuns,
						OverallStatus:       summarypb.DashboardTabSummary_UNKNOWN,
						LatestGreen:         noGreens,
						BugUrl:              "http://some-bugs/",
					},
					{
						DashboardName:       "a-dashboard",
						DashboardTabName:    "url-options-empty",
						LastUpdateTimestamp: 1000,
						Status:              noRuns,
						OverallStatus:       summarypb.DashboardTabSummary_UNKNOWN,
						LatestGreen:         noGreens,
						BugUrl:              "http://more-bugs/",
					},
					{
						DashboardName:       "a-dashboard",
						DashboardTabName:    "url-options",
						LastUpdateTimestamp: 1000,
						Status:              noRuns,
						OverallStatus:       summarypb.DashboardTabSummary_UNKNOWN,
						LatestGreen:         noGreens,
						BugUrl:              "http://ooh-bugs/",
					},
				},
			},
		},
	}

	for _, tc := range cases {
		tabUpdater := tabUpdatePool(context.Background(), logrus.WithField("name", "pool"), 5)
		t.Run(tc.name, func(t *testing.T) {
			finder := func(dash string, tab *configpb.DashboardTab) (*gcs.Path, *configpb.TestGroup, gridReader, error) {
				name := tab.TestGroupName
				if name == "inject-error" {
					return nil, nil, nil, errors.New("injected find group error")
				}
				fake, ok := tc.groups[name]
				if !ok {
					return nil, nil, nil, nil
				}
				var path *gcs.Path
				var err error

				path, err = gcs.NewPath(fmt.Sprintf("gs://bucket/grid/%s/%s", dash, name))
				if err != nil {
					t.Helper()
					t.Fatalf("Failed to create path: %v", err)
				}
				reader := func(_ context.Context) (io.ReadCloser, time.Time, int64, error) {
					return ioutil.NopCloser(bytes.NewBuffer(compress(gridBuf(fake.grid)))), fake.mod, fake.gen, fake.err
				}
				return path, fake.group, reader, nil
			}
			var actual summarypb.DashboardSummary
			client := fake.Stater{}
			for name, group := range tc.groups {
				path, err := gcs.NewPath(fmt.Sprintf("gs://bucket/grid/%s/%s", tc.dash.Name, name))
				if err != nil {
					t.Errorf("Failed to create Path: %v", err)
				}
				client[*path] = fake.Stat{
					Attrs: storage.ObjectAttrs{
						Generation: group.gen,
						Updated:    group.mod,
					},
				}
			}
			updateDashboard(context.Background(), client, tc.dash, &actual, finder, tabUpdater)
			if diff := cmp.Diff(tc.expected, &actual, protocmp.Transform()); diff != "" {
				t.Errorf("updateDashboard() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFilterDashboards(t *testing.T) {
	cases := []struct {
		name       string
		dashboards map[string]*configpb.Dashboard
		allowed    []string
		want       map[string]*configpb.Dashboard
	}{
		{
			name: "empty",
		},
		{
			name: "basic",
			dashboards: map[string]*configpb.Dashboard{
				"hello": {Name: "hi"},
			},
			want: map[string]*configpb.Dashboard{
				"hello": {Name: "hi"},
			},
		},
		{
			name: "zero",
			dashboards: map[string]*configpb.Dashboard{
				"hello": {Name: "hi"},
			},
			allowed: []string{"nothing"},
			want:    map[string]*configpb.Dashboard{},
		},
		{
			name: "both",
			dashboards: map[string]*configpb.Dashboard{
				"hello": {Name: "hi"},
				"world": {Name: "there"},
			},
			allowed: []string{"hi", "there"},
			want: map[string]*configpb.Dashboard{
				"hello": {Name: "hi"},
				"world": {Name: "there"},
			},
		},
		{
			name: "one",
			dashboards: map[string]*configpb.Dashboard{
				"hello": {Name: "hi"},
				"drop":  {Name: "cuss-word"},
				"world": {Name: "there"},
			},
			allowed: []string{"hi", "there", "drop"}, // target name, not key
			want: map[string]*configpb.Dashboard{
				"hello": {Name: "hi"},
				"world": {Name: "there"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var allowed stringset.Set
			if tc.allowed != nil {
				allowed = stringset.New(tc.allowed...)
			}

			got := filterDashboards(tc.dashboards, allowed)
			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("filterDashboards() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestStaleHours(t *testing.T) {
	cases := []struct {
		name     string
		tab      *configpb.DashboardTab
		expected time.Duration
	}{
		{
			name:     "zero without an alert",
			expected: 0,
		},
		{
			name: "use defined hours when set",
			tab: &configpb.DashboardTab{
				AlertOptions: &configpb.DashboardTabAlertOptions{
					AlertStaleResultsHours: 4,
				},
			},
			expected: 4 * time.Hour,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.tab == nil {
				tc.tab = &configpb.DashboardTab{}
			}
			if actual := staleHours(tc.tab); actual != tc.expected {
				t.Errorf("actual %v != expected %v", actual, tc.expected)
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

func TestUpdateTab(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name      string
		tab       *configpb.DashboardTab
		group     *configpb.TestGroup
		grid      *statepb.Grid
		mod       time.Time
		gen       int64
		gridError error
		expected  *summarypb.DashboardTabSummary
		err       bool
	}{
		{
			name: "read grid error returns error",
			tab: &configpb.DashboardTab{
				TestGroupName: "foo",
			},
			group:     &configpb.TestGroup{},
			mod:       now,
			gen:       42,
			gridError: errors.New("burninated"),
			err:       true,
		},
		{
			name: "basically works", // TODO(fejta): more better
			tab: &configpb.DashboardTab{
				Name:          "foo-tab",
				TestGroupName: "foo-group",
				AlertOptions: &configpb.DashboardTabAlertOptions{
					AlertStaleResultsHours: 1,
				},
			},
			group: &configpb.TestGroup{},
			grid:  &statepb.Grid{},
			mod:   now,
			gen:   43,
			expected: &summarypb.DashboardTabSummary{
				DashboardTabName:    "foo-tab",
				LastUpdateTimestamp: float64(now.Unix()),
				Alert:               noRuns,
				LatestGreen:         noGreens,
				OverallStatus:       summarypb.DashboardTabSummary_STALE,
				Status:              noRuns,
			},
		},
		{
			name: "missing grid returns a blank summary",
			tab: &configpb.DashboardTab{
				Name: "you know",
			},
			group:     &configpb.TestGroup{},
			gridError: fmt.Errorf("oh yeah: %w", storage.ErrObjectNotExist),
			err:       true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.tab == nil {
				tc.tab = &configpb.DashboardTab{}
			}
			reader := func(_ context.Context) (io.ReadCloser, time.Time, int64, error) {
				if tc.gridError != nil {
					return nil, time.Time{}, 0, tc.gridError
				}
				return ioutil.NopCloser(bytes.NewBuffer(compress(gridBuf(tc.grid)))), tc.mod, tc.gen, nil
			}
			actual, err := updateTab(context.Background(), tc.tab, tc.group, reader)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Errorf("failed to receive expected error")
			case !proto.Equal(actual, tc.expected):
				t.Errorf("actual summary: %s != expected %s", actual, tc.expected)
			}
		})
	}
}

func TestReadGrid(t *testing.T) {
	cases := []struct {
		name         string
		reader       io.Reader
		err          error
		expectedGrid *statepb.Grid
		expectErr    bool
	}{
		{
			name:      "error opening returns error",
			err:       errors.New("open failed"),
			expectErr: true,
		},
		{
			name: "return error when state is not compressed",
			reader: bytes.NewBuffer(gridBuf(&statepb.Grid{
				LastTimeUpdated: 444,
			})),
			expectErr: true,
		},
		{
			name:      "return error when compressed object is not a grid proto",
			reader:    bytes.NewBuffer(compress([]byte("hello"))),
			expectErr: true,
		},
		{
			name: "return error when compressed proto is truncated",
			reader: bytes.NewBuffer(compress(gridBuf(&statepb.Grid{
				Columns: []*statepb.Column{
					{
						Build:      "really long info",
						Name:       "weeee",
						HotlistIds: "super exciting",
					},
				},
				LastTimeUpdated: 555,
			}))[:10]),
			expectErr: true,
		},
		{
			name: "successfully parse compressed grid",
			reader: bytes.NewBuffer(compress(gridBuf(&statepb.Grid{
				LastTimeUpdated: 555,
			}))),
			expectedGrid: &statepb.Grid{
				LastTimeUpdated: 555,
			},
		},
	}

	for _, tc := range cases {
		now := time.Now()
		t.Run(tc.name, func(t *testing.T) {
			const gen = 42
			reader := func(_ context.Context) (io.ReadCloser, time.Time, int64, error) {
				if tc.err != nil {
					return nil, time.Time{}, 0, tc.err
				}
				return ioutil.NopCloser(tc.reader), now, gen, nil
			}

			actualGrid, aT, aGen, err := readGrid(context.Background(), reader)

			switch {
			case err != nil:
				if !tc.expectErr {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.expectErr:
				t.Error("failed to receive expected error")
			case !proto.Equal(actualGrid, tc.expectedGrid):
				t.Errorf("actual state: %#v != expected %#v", actualGrid, tc.expectedGrid)
			case !now.Equal(aT):
				t.Errorf("actual modified: %v != expected %v", aT, now)
			case aGen != gen:
				t.Errorf("actual generation: %d != expected %d", aGen, gen)
			}
		})
	}
}

func TestRecentColumns(t *testing.T) {
	cases := []struct {
		name     string
		tab      int32
		group    int32
		expected int
	}{
		{
			name:     "prefer tab over group",
			tab:      1,
			group:    2,
			expected: 1,
		},
		{
			name:     "use group if tab is empty",
			group:    9,
			expected: 9,
		},
		{
			name:     "use default when both are empty",
			expected: 5,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tabCfg := &configpb.DashboardTab{
				NumColumnsRecent: tc.tab,
			}
			groupCfg := &configpb.TestGroup{
				NumColumnsRecent: tc.group,
			}
			if actual := recentColumns(tabCfg, groupCfg); actual != tc.expected {
				t.Errorf("actual %d != expected %d", actual, tc.expected)
			}
		})
	}
}

func TestAllLinkedIssues(t *testing.T) {
	cases := []struct {
		name string
		rows []*statepb.Row
		want []string
	}{
		{
			name: "no rows",
			rows: []*statepb.Row{},
			want: []string{},
		},
		{
			name: "rows with no linked issues",
			rows: []*statepb.Row{
				{
					Name:    "test-1",
					Results: []int32{int32(statuspb.TestStatus_PASS), 10},
				},
				{
					Name:    "test-2",
					Results: []int32{int32(statuspb.TestStatus_PASS), 10},
				},
				{
					Name:    "test-3",
					Results: []int32{int32(statuspb.TestStatus_PASS), 10},
				},
			},
			want: []string{},
		},
		{
			name: "multiple linked issues",
			rows: []*statepb.Row{
				{
					Name:    "test-1",
					Results: []int32{int32(statuspb.TestStatus_PASS), 10},
					Issues:  []string{"1", "2"},
				},
				{
					Name:    "test-2",
					Results: []int32{int32(statuspb.TestStatus_PASS), 10},
					Issues:  []string{"5"},
				},
				{
					Name:    "test-3",
					Results: []int32{int32(statuspb.TestStatus_PASS), 10},
					Issues:  []string{"10", "7"},
				},
			},
			want: []string{"1", "2", "5", "7", "10"},
		},
		{
			name: "multiple linked issues with duplicates",
			rows: []*statepb.Row{
				{
					Name:    "test-1",
					Results: []int32{int32(statuspb.TestStatus_PASS), 10},
					Issues:  []string{"1", "2"},
				},
				{
					Name:    "test-2",
					Results: []int32{int32(statuspb.TestStatus_PASS), 10},
					Issues:  []string{"2", "3"},
				},
			},
			want: []string{"1", "2", "3"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := allLinkedIssues(tc.rows)
			sort.Strings(got)
			strSort := cmpopts.SortSlices(func(a, b string) bool { return a < b })
			if diff := cmp.Diff(tc.want, got, strSort); diff != "" {
				t.Errorf("allLinkedIssues() unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

func TestFirstFilled(t *testing.T) {
	cases := []struct {
		name     string
		values   []int32
		expected int
	}{
		{
			name: "zero by default",
		},
		{
			name:     "first non-zero value",
			values:   []int32{0, 1, 2},
			expected: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual := firstFilled(tc.values...); actual != tc.expected {
				t.Errorf("actual %d != expected %d", actual, tc.expected)
			}
		})
	}
}

func TestFilterMethods(t *testing.T) {
	cases := []struct {
		name     string
		rows     []*statepb.Row
		recent   int
		expected []*statepb.Row
		err      bool
	}{
		{
			name: "tolerates nil inputs",
		},
		{
			name: "basically works",
			rows: []*statepb.Row{
				{
					Name: "okay",
					Id:   "cool",
				},
			},
			expected: []*statepb.Row{
				{
					Name: "okay",
					Id:   "cool",
				},
			},
		},
		{
			name: "exclude all test methods",
			rows: []*statepb.Row{
				{
					Name: "test-1",
					Id:   "test-1",
				},
				{
					Name: "method-1",
					Id:   "test-1@TESTGRID@method-1",
				},
				{
					Name: "method-2",
					Id:   "test-1@TESTGRID@method-2",
				},
				{
					Name: "test-2",
					Id:   "test-2",
				},
				{
					Name: "test-2@TESTGRID@method-1",
					Id:   "method-1",
				},
			},
			expected: []*statepb.Row{
				{
					Name: "test-1",
					Id:   "test-1",
				},
				{
					Name: "test-2",
					Id:   "test-2",
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, r := range tc.rows {
				if r.Results == nil {
					r.Results = []int32{int32(statuspb.TestStatus_PASS), 100}
				}
			}
			for _, r := range tc.expected {
				if r.Results == nil {
					r.Results = []int32{int32(statuspb.TestStatus_PASS), 100}
				}
			}
			actual := filterMethods(tc.rows)

			if !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("%s != expected %s", actual, tc.expected)
			}
		})
	}
}

func TestRecentRows(t *testing.T) {
	const recent = 10
	cases := []struct {
		name     string
		rows     []*statepb.Row
		expected []string
	}{
		{
			name: "basically works",
		},
		{
			name: "skip row with nil results",
			rows: []*statepb.Row{
				{
					Name:    "include",
					Results: []int32{int32(statuspb.TestStatus_PASS), recent},
				},
				{
					Name: "skip-nil-results",
				},
			},
			expected: []string{"include"},
		},
		{
			name: "skip row with no recent results",
			rows: []*statepb.Row{
				{
					Name:    "include",
					Results: []int32{int32(statuspb.TestStatus_PASS), recent},
				},
				{
					Name:    "skip-this-one-with-no-recent-results",
					Results: []int32{int32(statuspb.TestStatus_NO_RESULT), recent},
				},
			},
			expected: []string{"include"},
		},
		{
			name: "include rows missing some recent results",
			rows: []*statepb.Row{
				{
					Name: "head skips",
					Results: []int32{
						int32(statuspb.TestStatus_NO_RESULT), recent - 1,
						int32(statuspb.TestStatus_PASS_WITH_SKIPS), recent,
					},
				},
				{
					Name: "tail skips",
					Results: []int32{
						int32(statuspb.TestStatus_FLAKY), recent - 1,
						int32(statuspb.TestStatus_NO_RESULT), recent,
					},
				},
				{
					Name: "middle skips",
					Results: []int32{
						int32(statuspb.TestStatus_FAIL), 1,
						int32(statuspb.TestStatus_NO_RESULT), recent - 2,
						int32(statuspb.TestStatus_PASS), 1,
					},
				},
			},
			expected: []string{
				"head skips",
				"tail skips",
				"middle skips",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actualRows := recentRows(tc.rows, recent)

			var actual []string
			for _, r := range actualRows {
				actual = append(actual, r.Name)
			}
			if !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("%s != expected %s", actual, tc.expected)
			}
		})
	}
}

func TestLatestRun(t *testing.T) {
	cases := []struct {
		name         string
		cols         []*statepb.Column
		expectedTime time.Time
		expectedSecs int64
	}{
		{
			name: "basically works",
		},
		{
			name: "zero started returns zero time",
			cols: []*statepb.Column{
				{},
			},
		},
		{
			name: "return first time in unix",
			cols: []*statepb.Column{
				{
					Started: 333333,
				},
				{
					Started: 222222,
				},
			},
			expectedTime: time.Unix(333, 333000000),
			expectedSecs: 333,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			when, s := latestRun(tc.cols)
			if !when.Equal(tc.expectedTime) {
				t.Errorf("time %v != expected %v", when, tc.expectedTime)
			}
			if s != tc.expectedSecs {
				t.Errorf("seconds %d != expected %d", s, tc.expectedSecs)
			}
		})
	}
}

func TestStaleAlert(t *testing.T) {
	cases := []struct {
		name  string
		mod   time.Time
		ran   time.Time
		dur   time.Duration
		alert bool
	}{
		{
			name: "basically works",
			mod:  time.Now().Add(-5 * time.Minute),
			ran:  time.Now().Add(-10 * time.Minute),
			dur:  time.Hour,
		},
		{
			name:  "unmodified alerts",
			mod:   time.Now().Add(-5 * time.Hour),
			ran:   time.Now(),
			dur:   time.Hour,
			alert: true,
		},
		{
			name:  "no recent runs alerts",
			mod:   time.Now(),
			ran:   time.Now().Add(-5 * time.Hour),
			dur:   time.Hour,
			alert: true,
		},
		{
			name:  "no runs alerts",
			mod:   time.Now(),
			dur:   time.Hour,
			alert: true,
		},
		{
			name: "no runs w/ stale hours not configured does not alert",
			mod:  time.Now(),
		},
		{
			name:  "no state w/ stale hours not configured alerts",
			alert: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := staleAlert(tc.mod, tc.ran, tc.dur)
			if actual != "" && !tc.alert {
				t.Errorf("unexpected stale alert: %s", actual)
			}
			if actual == "" && tc.alert {
				t.Errorf("failed to create a stale alert")
			}
		})
	}
}

func TestFailingTestSummaries(t *testing.T) {
	cases := []struct {
		name     string
		rows     []*statepb.Row
		expected []*summarypb.FailingTestSummary
	}{
		{
			name: "do not alert by default",
			rows: []*statepb.Row{
				{},
				{},
			},
		},
		{
			name: "alert when rows have alerts",
			rows: []*statepb.Row{
				{},
				{
					Name:   "foo-name",
					Id:     "foo-target",
					Issues: []string{"1234", "5678"},
					AlertInfo: &statepb.AlertInfo{
						FailBuildId:       "bad",
						LatestFailBuildId: "still-bad",
						PassBuildId:       "good",
						FailCount:         6,
						BuildLink:         "to the past",
						BuildLinkText:     "hyrule",
						BuildUrlText:      "of sandwich",
						FailureMessage:    "pop tart",
						Properties: map[string]string{
							"ham": "eggs",
						},
						HotlistIds: []string{},
					},
				},
				{},
				{
					Name:   "bar-name",
					Id:     "bar-target",
					Issues: []string{"1234"},
					AlertInfo: &statepb.AlertInfo{
						FailBuildId:       "fbi",
						LatestFailBuildId: "lfbi",
						PassBuildId:       "pbi",
						FailTestId:        "819283y823-1232813",
						LatestFailTestId:  "920394z934-2343924",
						FailCount:         1,
						BuildLink:         "bl",
						BuildLinkText:     "blt",
						BuildUrlText:      "but",
						FailureMessage:    "fm",
						Properties: map[string]string{
							"foo":   "bar",
							"hello": "lots",
						},
						HotlistIds: []string{"111", "222"},
					},
				},
				{},
			},
			expected: []*summarypb.FailingTestSummary{
				{
					DisplayName:        "foo-name",
					TestName:           "foo-target",
					FailBuildId:        "bad",
					LatestFailBuildId:  "still-bad",
					PassBuildId:        "good",
					FailCount:          6,
					BuildLink:          "to the past",
					BuildLinkText:      "hyrule",
					BuildUrlText:       "of sandwich",
					FailureMessage:     "pop tart",
					FailTestLink:       " foo-target",
					LatestFailTestLink: " foo-target",
					LinkedBugs:         []string{"1234", "5678"},
					Properties: map[string]string{
						"ham": "eggs",
					},
					HotlistIds: []string{},
				},
				{
					DisplayName:        "bar-name",
					TestName:           "bar-target",
					FailBuildId:        "fbi",
					LatestFailBuildId:  "lfbi",
					PassBuildId:        "pbi",
					FailCount:          1,
					BuildLink:          "bl",
					BuildLinkText:      "blt",
					BuildUrlText:       "but",
					FailureMessage:     "fm",
					FailTestLink:       "819283y823-1232813 bar-target",
					LatestFailTestLink: "920394z934-2343924 bar-target",
					LinkedBugs:         []string{"1234"},
					Properties: map[string]string{
						"foo":   "bar",
						"hello": "lots",
					},
					HotlistIds: []string{"111", "222"},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := failingTestSummaries(tc.rows)
			if diff := cmp.Diff(tc.expected, actual, protocmp.Transform()); diff != "" {
				t.Errorf("failingTestSummaries() (-want, +got): %s", diff)
			}
		})
	}
}

func TestOverallStatus(t *testing.T) {
	cases := []struct {
		name     string
		rows     []*statepb.Row
		recent   int
		stale    string
		broken   bool
		alerts   bool
		expected summarypb.DashboardTabSummary_TabStatus
	}{
		{
			name:     "unknown by default",
			expected: summarypb.DashboardTabSummary_UNKNOWN,
		},
		{
			name:     "stale joke results in stale summary",
			stale:    "joke",
			expected: summarypb.DashboardTabSummary_STALE,
		},
		{
			name:     "alerts result in failure",
			alerts:   true,
			expected: summarypb.DashboardTabSummary_FAIL,
		},
		{
			name:     "prefer stale over failure",
			stale:    "potato chip",
			alerts:   true,
			expected: summarypb.DashboardTabSummary_STALE,
		},
		{
			name:   "completed results result in pass",
			recent: 1,
			rows: []*statepb.Row{
				{
					Results: []int32{int32(statuspb.TestStatus_PASS), 1},
				},
			},
			expected: summarypb.DashboardTabSummary_PASS,
		},
		{
			name:   "non-passing results without an alert results in flaky",
			recent: 1,
			rows: []*statepb.Row{
				{
					Results: []int32{int32(statuspb.TestStatus_FAIL), 1},
				},
			},
			expected: summarypb.DashboardTabSummary_FLAKY,
		},
		{
			name:   "incomplete passing results", // ignore them
			recent: 5,
			rows: []*statepb.Row{
				{
					Results: []int32{int32(statuspb.TestStatus_NO_RESULT), 1},
				},
				{
					Results: []int32{int32(statuspb.TestStatus_PASS), 3},
				},
				{
					Results: []int32{int32(statuspb.TestStatus_RUNNING), 2},
				},
				{
					Results: []int32{int32(statuspb.TestStatus_PASS), 2},
				},
			},
			expected: summarypb.DashboardTabSummary_PASS,
		},
		{
			name:   "incomplete flaky results", // ignore them
			recent: 5,
			rows: []*statepb.Row{
				{
					Results: []int32{int32(statuspb.TestStatus_NO_RESULT), 1},
				},
				{
					Results: []int32{int32(statuspb.TestStatus_PASS), 3},
				},
				{
					Results: []int32{int32(statuspb.TestStatus_RUNNING), 2},
				},
				{
					Results: []int32{int32(statuspb.TestStatus_FAIL), 2},
				},
			},
			expected: summarypb.DashboardTabSummary_FLAKY,
		},
		{
			name:   "ignore old failures",
			recent: 1,
			rows: []*statepb.Row{
				{
					Results: []int32{
						int32(statuspb.TestStatus_PASS), 3,
						int32(statuspb.TestStatus_FAIL), 5,
					},
				},
			},
			expected: summarypb.DashboardTabSummary_PASS,
		},
		{
			name:   "dropped columns", // should not impact status
			recent: 1,
			rows: []*statepb.Row{
				{
					Name: "current",
					Results: []int32{
						int32(statuspb.TestStatus_PASS), 2,
					},
				},
				{
					Name: "ignore dropped",
					Results: []int32{
						int32(statuspb.TestStatus_NO_RESULT), 1,
						int32(statuspb.TestStatus_FAIL), 1,
					},
				},
			},
			expected: summarypb.DashboardTabSummary_PASS,
		},
		{
			name:   "dropped columns", // should not impact status
			recent: 1,
			rows: []*statepb.Row{
				{
					Name: "current",
					Results: []int32{
						int32(statuspb.TestStatus_PASS), 2,
					},
				},
				{
					Name: "ignore dropped",
					Results: []int32{
						int32(statuspb.TestStatus_NO_RESULT), 1,
						int32(statuspb.TestStatus_FAIL), 1,
					},
				},
			},
			expected: summarypb.DashboardTabSummary_PASS,
		},
		{
			name:   "running", // do not count as recent
			recent: 1,
			rows: []*statepb.Row{
				{
					Name: "pass",
					Results: []int32{
						int32(statuspb.TestStatus_PASS), 2,
					},
				},
				{
					Name: "running",
					Results: []int32{
						int32(statuspb.TestStatus_RUNNING), 1,
						int32(statuspb.TestStatus_PASS), 1,
					},
				},
				{
					Name: "flake",
					Results: []int32{
						int32(statuspb.TestStatus_PASS), 1,
						int32(statuspb.TestStatus_FAIL), 1,
					},
				},
			},
			expected: summarypb.DashboardTabSummary_FLAKY,
		},
		{
			name:   "partial results work",
			recent: 50,
			rows: []*statepb.Row{
				{
					Results: []int32{int32(statuspb.TestStatus_PASS), 1},
				},
			},
			expected: summarypb.DashboardTabSummary_PASS,
		},
		{
			name:   "coalesce passes",
			recent: 1,
			rows: []*statepb.Row{
				{
					Results: []int32{int32(statuspb.TestStatus_PASS_WITH_SKIPS), 1},
				},
			},
			expected: summarypb.DashboardTabSummary_PASS,
		},
		{
			name:   "broken cycle",
			recent: 1,
			rows: []*statepb.Row{
				{
					Results: []int32{int32(statuspb.TestStatus_PASS_WITH_SKIPS), 1},
				},
			},
			broken:   true,
			expected: summarypb.DashboardTabSummary_BROKEN,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var alerts []*summarypb.FailingTestSummary
			if tc.alerts {
				alerts = append(alerts, &summarypb.FailingTestSummary{})
			}

			if actual := overallStatus(&statepb.Grid{Rows: tc.rows}, tc.recent, tc.stale, tc.broken, alerts); actual != tc.expected {
				t.Errorf("%s != expected %s", actual, tc.expected)
			}
		})
	}
}

func makeShim(v ...interface{}) []interface{} {
	return v
}

func TestGridMetrics(t *testing.T) {
	cases := []struct {
		name            string
		cols            int
		rows            []*statepb.Row
		recent          int
		passingCols     int
		filledCols      int
		passingCells    int
		filledCells     int
		brokenThreshold float32
		expectedBroken  bool
	}{
		{
			name: "no runs",
		},
		{
			name: "what people want (greens)",
			cols: 2,
			rows: []*statepb.Row{
				{
					Name:    "green eggs",
					Results: []int32{int32(statuspb.TestStatus_PASS), 2},
				},
				{
					Name:    "and ham",
					Results: []int32{int32(statuspb.TestStatus_PASS), 2},
				},
			},
			recent:       2,
			passingCols:  2,
			filledCols:   2,
			passingCells: 4,
			filledCells:  4,
		},
		{
			name: "red: i do not like them sam I am",
			cols: 2,
			rows: []*statepb.Row{
				{
					Name:    "not with a fox",
					Results: []int32{int32(statuspb.TestStatus_FAIL), 2},
				},
				{
					Name:    "not in a box",
					Results: []int32{int32(statuspb.TestStatus_FLAKY), 2},
				},
			},
			recent:       2,
			passingCols:  0,
			filledCols:   2,
			passingCells: 0,
			filledCells:  4,
		},
		{
			name: "passing cells but no green columns",
			cols: 2,
			rows: []*statepb.Row{
				{
					Name: "first doughnut is best",
					Results: []int32{
						int32(statuspb.TestStatus_PASS), 1,
						int32(statuspb.TestStatus_FAIL), 1,
					},
				},
				{
					Name: "fine wine gets better",
					Results: []int32{
						int32(statuspb.TestStatus_FAIL), 1,
						int32(statuspb.TestStatus_PASS), 1,
					},
				},
			},
			recent:       2,
			passingCols:  0,
			filledCols:   2,
			passingCells: 2,
			filledCells:  4,
		},
		{
			name:   "ignore overflow of claimed columns",
			cols:   100,
			recent: 50,
			rows: []*statepb.Row{
				{
					Name:    "a",
					Results: []int32{int32(statuspb.TestStatus_PASS), 3},
				},
				{
					Name:    "b",
					Results: []int32{int32(statuspb.TestStatus_PASS), 3},
				},
			},
			passingCols:  3,
			filledCols:   3,
			passingCells: 6,
			filledCells:  6,
		},
		{
			name:   "ignore bad row data",
			cols:   2,
			recent: 2,
			rows: []*statepb.Row{
				{
					Name: "empty",
				},
				{
					Name:    "filled",
					Results: []int32{int32(statuspb.TestStatus_PASS), 2},
				},
			},
			passingCols:  2,
			filledCols:   2,
			passingCells: 2,
			filledCells:  2,
		},
		{
			name:   "ignore non recent data",
			cols:   100,
			recent: 2,
			rows: []*statepb.Row{
				{
					Name:    "data",
					Results: []int32{int32(statuspb.TestStatus_PASS), 100},
				},
			},
			passingCols:  2,
			filledCols:   2,
			passingCells: 2,
			filledCells:  2,
		},
		{
			name:   "no result cells do not alter column",
			cols:   3,
			recent: 3,
			rows: []*statepb.Row{
				{
					Name:    "always empty",
					Results: []int32{int32(statuspb.TestStatus_NO_RESULT), 3},
				},
				{
					Name: "first empty",
					Results: []int32{
						int32(statuspb.TestStatus_NO_RESULT), 1,
						int32(statuspb.TestStatus_PASS), 2,
					},
				},
				{
					Name: "always pass",
					Results: []int32{
						int32(statuspb.TestStatus_PASS), 3,
					},
				},
				{
					Name: "empty, fail, pass",
					Results: []int32{
						int32(statuspb.TestStatus_NO_RESULT), 1,
						int32(statuspb.TestStatus_FAIL), 1,
						int32(statuspb.TestStatus_PASS), 1,
					},
				},
			},
			passingCols:  2, // pass, fail, pass
			filledCols:   3,
			passingCells: 6,
			filledCells:  7,
		},
		{
			name:   "not enough columns yet works just fine",
			cols:   4,
			recent: 50,
			rows: []*statepb.Row{
				{
					Name: "four passes",
					Results: []int32{
						int32(statuspb.TestStatus_PASS), 4,
					},
				},
			},
			passingCols:  4,
			filledCols:   4,
			passingCells: 4,
			filledCells:  4,
		},
		{
			name:   "half passes and half fails",
			cols:   4,
			recent: 4,
			rows: []*statepb.Row{
				{
					Name: "four passes",
					Results: []int32{
						int32(statuspb.TestStatus_PASS), 4,
					},
				},
				{
					Name: "four fails",
					Results: []int32{
						int32(statuspb.TestStatus_FAIL), 4,
					},
				},
			},
			passingCols:  0,
			filledCols:   4,
			passingCells: 4,
			filledCells:  8,
		},
		{
			name:   "no result in every column",
			cols:   3,
			recent: 3,
			rows: []*statepb.Row{
				{
					Name:    "always empty",
					Results: []int32{int32(statuspb.TestStatus_NO_RESULT), 3},
				},
				{
					Name: "first empty",
					Results: []int32{
						int32(statuspb.TestStatus_NO_RESULT), 1,
						int32(statuspb.TestStatus_PASS), 2,
					},
				},
				{
					Name:    "always empty",
					Results: []int32{int32(statuspb.TestStatus_NO_RESULT), 3},
				},
			},
			passingCols:  2,
			filledCols:   2,
			passingCells: 2,
			filledCells:  2,
		},
		{
			name:   "only no result",
			cols:   3,
			recent: 3,
			rows: []*statepb.Row{
				{
					Name:    "always empty",
					Results: []int32{int32(statuspb.TestStatus_NO_RESULT), 3},
				},
			},
			passingCols:  0,
			filledCols:   0,
			passingCells: 0,
			filledCells:  0,
		},
		{
			name:   "Pass with skips",
			cols:   3,
			recent: 3,
			rows: []*statepb.Row{
				{
					Name:    "always empty",
					Results: []int32{int32(statuspb.TestStatus_PASS_WITH_SKIPS), 3},
				},
				{
					Name:    "all pass",
					Results: []int32{int32(statuspb.TestStatus_PASS), 3},
				},
			},
			passingCols:  3,
			filledCols:   3,
			passingCells: 6,
			filledCells:  6,
		},
		{
			name:   "Pass with errors",
			cols:   3,
			recent: 3,
			rows: []*statepb.Row{
				{
					Name:    "always empty",
					Results: []int32{int32(statuspb.TestStatus_PASS_WITH_ERRORS), 3},
				},
				{
					Name:    "all pass",
					Results: []int32{int32(statuspb.TestStatus_PASS), 3},
				},
			},
			passingCols:  3,
			filledCols:   3,
			passingCells: 6,
			filledCells:  6,
		},
		{
			name:   "All columns past threshold",
			cols:   4,
			recent: 4,
			rows: []*statepb.Row{
				{
					Name: "four passes",
					Results: []int32{
						int32(statuspb.TestStatus_PASS), 4,
					},
				},
				{
					Name: "four fails",
					Results: []int32{
						int32(statuspb.TestStatus_FAIL), 4,
					},
				},
			},
			passingCols:     0,
			filledCols:      4,
			passingCells:    4,
			filledCells:     8,
			brokenThreshold: .4,
			expectedBroken:  true,
		},
		{
			name:   "All columns under threshold",
			cols:   4,
			recent: 4,
			rows: []*statepb.Row{
				{
					Name: "four passes",
					Results: []int32{
						int32(statuspb.TestStatus_PASS), 4,
					},
				},
				{
					Name: "four fails",
					Results: []int32{
						int32(statuspb.TestStatus_FAIL), 4,
					},
				},
			},
			passingCols:     0,
			filledCols:      4,
			passingCells:    4,
			filledCells:     8,
			brokenThreshold: .6,
			expectedBroken:  false,
		},
		{
			name:   "One column past threshold",
			cols:   4,
			recent: 4,
			rows: []*statepb.Row{
				{
					Name: "four passes",
					Results: []int32{
						int32(statuspb.TestStatus_PASS), 4,
					},
				},
				{
					Name: "one pass three fails",
					Results: []int32{
						int32(statuspb.TestStatus_FAIL), 1,
						int32(statuspb.TestStatus_PASS), 3,
					},
				},
			},
			passingCols:     3,
			filledCols:      4,
			passingCells:    7,
			filledCells:     8,
			brokenThreshold: .4,
			expectedBroken:  true,
		},
		{
			name:   "One column under threshold",
			cols:   4,
			recent: 4,
			rows: []*statepb.Row{
				{
					Name: "four passes",
					Results: []int32{
						int32(statuspb.TestStatus_PASS), 4,
					},
				},
				{
					Name: "one pass three fails",
					Results: []int32{
						int32(statuspb.TestStatus_FAIL), 1,
						int32(statuspb.TestStatus_PASS), 3,
					},
				},
			},
			passingCols:     3,
			filledCols:      4,
			passingCells:    7,
			filledCells:     8,
			brokenThreshold: .6,
			expectedBroken:  false,
		},
		{
			name:   "many non-passing/non-failing statuses is not broken",
			cols:   4,
			recent: 4,
			rows: []*statepb.Row{
				{
					Name: "four aborts (foo)",
					Results: []int32{
						int32(statuspb.TestStatus_CATEGORIZED_ABORT), 4,
					},
				},
				{
					Name: "four aborts (bar)",
					Results: []int32{
						int32(statuspb.TestStatus_CATEGORIZED_ABORT), 4,
					},
				},
				{
					Name: "four aborts (baz)",
					Results: []int32{
						int32(statuspb.TestStatus_CATEGORIZED_ABORT), 4,
					},
				},
			},
			passingCols:     0,
			filledCols:      4,
			passingCells:    0,
			filledCells:     12,
			brokenThreshold: .6,
			expectedBroken:  false,
		},
		{
			name:   "many non-passing/non-failing statuses + failing statuses, not broken",
			cols:   4,
			recent: 4,
			rows: []*statepb.Row{
				{
					Name: "four aborts (foo)",
					Results: []int32{
						int32(statuspb.TestStatus_CATEGORIZED_ABORT), 4,
					},
				},
				{
					Name: "four aborts (bar)",
					Results: []int32{
						int32(statuspb.TestStatus_CATEGORIZED_ABORT), 4,
					},
				},
				{
					Name: "four fails",
					Results: []int32{
						int32(statuspb.TestStatus_FAIL), 4,
					},
				},
			},
			passingCols:     0,
			filledCols:      4,
			passingCells:    0,
			filledCells:     12,
			brokenThreshold: .6,
			expectedBroken:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expected := makeShim(tc.passingCols, tc.filledCols, tc.passingCells, tc.filledCells, tc.expectedBroken)
			actual := makeShim(gridMetrics(tc.cols, tc.rows, tc.recent, tc.brokenThreshold))
			assert.Equal(t, expected, actual, fmt.Sprintf("%s != expected %s", actual, expected))
		})
	}
}

func TestStatusMessage(t *testing.T) {
	cases := []struct {
		name             string
		passingCols      int
		completedCols    int
		passingCells     int
		filledCells      int
		expectedOverride string
	}{
		{
			name:             "no filledCells",
			expectedOverride: noRuns,
		},
		{
			name:          "green path",
			passingCols:   2,
			completedCols: 2,
			passingCells:  4,
			filledCells:   4,
		},
		{
			name:          "all red path",
			passingCols:   0,
			completedCols: 2,
			passingCells:  0,
			filledCells:   4,
		},
		{
			name:          "all values the same",
			passingCols:   2,
			completedCols: 2,
			passingCells:  2,
			filledCells:   2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expected := tc.expectedOverride
			if expected == "" {
				expected = fmtStatus(tc.passingCols, tc.completedCols, tc.passingCells, tc.filledCells)
			}
			if actual := statusMessage(tc.passingCols, tc.completedCols, tc.passingCells, tc.filledCells); actual != expected {
				t.Errorf("%s != expected %s", actual, expected)
			}
		})
	}
}

func TestLatestGreen(t *testing.T) {
	cases := []struct {
		name     string
		rows     []*statepb.Row
		cols     []*statepb.Column
		expected string
		first    bool
	}{
		{
			name:     "no recent greens by default",
			expected: noGreens,
		},
		{
			name:     "no recent greens by default, first green",
			first:    true,
			expected: noGreens,
		},
		{
			name: "use build id by default",
			rows: []*statepb.Row{
				{
					Name:    "so pass",
					Results: []int32{int32(statuspb.TestStatus_PASS), 4},
				},
			},
			cols: []*statepb.Column{
				{
					Build: "correct",
					Extra: []string{"wrong"},
				},
			},
			expected: "correct",
		},
		{
			name: "fall back to build id when headers are missing",
			rows: []*statepb.Row{
				{
					Name:    "so pass",
					Results: []int32{int32(statuspb.TestStatus_PASS), 4},
				},
			},
			first: true,
			cols: []*statepb.Column{
				{
					Build: "fallback",
					Extra: []string{},
				},
			},
			expected: "fallback",
		},
		{
			name: "favor first green",
			rows: []*statepb.Row{
				{
					Name:    "so pass",
					Results: []int32{int32(statuspb.TestStatus_PASS), 4},
				},
			},
			cols: []*statepb.Column{
				{
					Extra: []string{"hello", "there"},
				},
				{
					Extra: []string{"bad", "wrong"},
				},
			},
			first:    true,
			expected: "hello",
		},
		{
			name: "accept any kind of pass",
			rows: []*statepb.Row{
				{
					Name: "pass w/ errors",
					Results: []int32{
						int32(statuspb.TestStatus_PASS_WITH_ERRORS), 1,
						int32(statuspb.TestStatus_PASS), 1,
					},
				},
				{
					Name:    "pass pass",
					Results: []int32{int32(statuspb.TestStatus_PASS), 2},
				},
				{
					Name: "pass and skip",
					Results: []int32{
						int32(statuspb.TestStatus_PASS_WITH_SKIPS), 1,
						int32(statuspb.TestStatus_PASS), 1,
					},
				},
			},
			cols: []*statepb.Column{
				{
					Extra: []string{"good"},
				},
				{
					Extra: []string{"bad"},
				},
			},
			first:    true,
			expected: "good",
		},
		{
			name: "avoid columns with running rows",
			rows: []*statepb.Row{
				{
					Name: "running",
					Results: []int32{
						int32(statuspb.TestStatus_RUNNING), 1,
						int32(statuspb.TestStatus_PASS), 1,
					},
				},
				{
					Name: "pass",
					Results: []int32{
						int32(statuspb.TestStatus_PASS), 2,
					},
				},
			},
			cols: []*statepb.Column{
				{
					Extra: []string{"skip-first-col-still-running"},
				},
				{
					Extra: []string{"accept second-all-finished"},
				},
			},
			first:    true,
			expected: "accept second-all-finished",
		},
		{
			name: "avoid columns with flakes",
			rows: []*statepb.Row{
				{
					Name: "flaking",
					Results: []int32{
						int32(statuspb.TestStatus_FLAKY), 1,
						int32(statuspb.TestStatus_PASS), 1,
					},
				},
				{
					Name: "passing",
					Results: []int32{
						int32(statuspb.TestStatus_PASS), 2,
					},
				},
			},
			cols: []*statepb.Column{
				{
					Extra: []string{"skip-first-col-with-flake"},
				},
				{
					Extra: []string{"accept second-no-flake"},
				},
			},
			first:    true,
			expected: "accept second-no-flake",
		},
		{
			name: "avoid columns with failures",
			rows: []*statepb.Row{
				{
					Name: "failing",
					Results: []int32{
						int32(statuspb.TestStatus_FAIL), 1,
						int32(statuspb.TestStatus_PASS), 1,
					},
				},
				{
					Name: "passing",
					Results: []int32{
						int32(statuspb.TestStatus_PASS), 2,
					},
				},
			},
			cols: []*statepb.Column{
				{
					Extra: []string{"skip-first-col-with-fail"},
				},
				{
					Extra: []string{"accept second-after-failure"},
				},
			},
			first:    true,
			expected: "accept second-after-failure",
		},
		{
			name: "multiple failing columns fixed",
			rows: []*statepb.Row{
				{
					Name: "fail then pass",
					Results: []int32{
						int32(statuspb.TestStatus_FAIL), 1,
						int32(statuspb.TestStatus_PASS), 1,
					},
				},
				{
					Name: "also fail then pass",
					Results: []int32{
						int32(statuspb.TestStatus_FAIL), 1,
						int32(statuspb.TestStatus_PASS), 2,
					},
				},
			},
			cols: []*statepb.Column{
				{
					Extra: []string{"skip-first-col-with-fail"},
				},
				{
					Extra: []string{"accept second-after-failure"},
				},
			},
			first:    true,
			expected: "accept second-after-failure",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			grid := statepb.Grid{
				Columns: tc.cols,
				Rows:    tc.rows,
			}
			if actual := latestGreen(&grid, tc.first); actual != tc.expected {
				t.Errorf("%s != expected %s", actual, tc.expected)
			}
		})
	}
}

func TestGetHealthinessForInterval(t *testing.T) {
	now := int64(1000000) // arbitrary time
	secondsInDay := int64(86400)
	// These values are *1000 because Column.Started is in milliseconds
	withinCurrentInterval := (float64(now) - 0.5*float64(secondsInDay)) * 1000.0
	withinPreviousInterval := (float64(now) - 1.5*float64(secondsInDay)) * 1000.0
	notWithinAnyInterval := (float64(now) - 3.0*float64(secondsInDay)) * 1000.0
	cases := []struct {
		name     string
		grid     *statepb.Grid
		tabName  string
		interval int
		expected *summarypb.HealthinessInfo
	}{
		{
			name: "typical inputs returns correct HealthinessInfo",
			grid: &statepb.Grid{
				Columns: []*statepb.Column{
					{Started: withinCurrentInterval},
					{Started: withinCurrentInterval},
					{Started: withinPreviousInterval},
					{Started: withinPreviousInterval},
					{Started: notWithinAnyInterval},
				},
				Rows: []*statepb.Row{
					{
						Name: "test_1",
						Results: []int32{
							statuspb.TestStatus_value["PASS"], 1,
							statuspb.TestStatus_value["FAIL"], 1,
							statuspb.TestStatus_value["FAIL"], 1,
							statuspb.TestStatus_value["FAIL"], 2,
						},
						Messages: []string{
							"",
							"",
							"",
							"infra_fail_1",
							"",
						},
					},
				},
			},
			tabName:  "tab1",
			interval: 1, // enforce that this equals what secondsInDay is multiplied by below in the Timestamps
			expected: &summarypb.HealthinessInfo{
				Start: &timestamp.Timestamp{Seconds: now - secondsInDay},
				End:   &timestamp.Timestamp{Seconds: now},
				Tests: []*summarypb.TestInfo{
					{
						DisplayName:            "test_1",
						TotalNonInfraRuns:      2,
						PassedNonInfraRuns:     1,
						FailedNonInfraRuns:     1,
						TotalRunsWithInfra:     2,
						Flakiness:              50.0,
						PreviousFlakiness:      []float32{100.0},
						ChangeFromLastInterval: summarypb.TestInfo_DOWN,
					},
				},
				AverageFlakiness:  50.0,
				PreviousFlakiness: []float32{100.0},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual := getHealthinessForInterval(tc.grid, tc.tabName, time.Unix(now, 0), tc.interval); !proto.Equal(actual, tc.expected) {
				t.Errorf("actual: %+v != expected: %+v", actual, tc.expected)
			}
		})
	}
}

func TestGoBackDays(t *testing.T) {
	cases := []struct {
		name        string
		days        int
		currentTime time.Time
		expected    int
	}{
		{
			name:        "0 days returns same Time as input",
			days:        0,
			currentTime: time.Unix(0, 0).UTC(),
			expected:    0,
		},
		{
			name:        "positive days input returns that many days in the past",
			days:        7,
			currentTime: time.Unix(0, 0).UTC().AddDate(0, 0, 7), // Gives a date 7 days after Unix 0 time
			expected:    0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual := goBackDays(tc.days, tc.currentTime); actual != tc.expected {
				t.Errorf("goBackDays gave actual: %d != expected: %d for days: %d and currentTime: %+v", actual, tc.expected, tc.days, tc.currentTime)
			}
		})
	}
}

func TestShouldRunHealthiness(t *testing.T) {
	cases := []struct {
		name     string
		tab      *configpb.DashboardTab
		expected bool
	}{
		{
			name: "tab with false Enable returns false",
			tab: &configpb.DashboardTab{
				HealthAnalysisOptions: &configpb.HealthAnalysisOptions{
					Enable: false,
				},
			},
			expected: false,
		},
		{
			name: "tab with true Enable returns true",
			tab: &configpb.DashboardTab{
				HealthAnalysisOptions: &configpb.HealthAnalysisOptions{
					Enable: true,
				},
			},
			expected: true,
		},
		{
			name:     "tab with nil HealthAnalysisOptions returns false",
			tab:      &configpb.DashboardTab{},
			expected: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual := shouldRunHealthiness(tc.tab); actual != tc.expected {
				t.Errorf("actual: %t != expected: %t", actual, tc.expected)
			}
		})
	}
}

func TestCoalesceResult(t *testing.T) {
	cases := []struct {
		name     string
		result   statuspb.TestStatus
		running  bool
		expected statuspb.TestStatus
	}{
		{
			name:     "no result by default",
			expected: statuspb.TestStatus_NO_RESULT,
		},
		{
			name:     "running is no result when ignored",
			result:   statuspb.TestStatus_RUNNING,
			expected: statuspb.TestStatus_NO_RESULT,
			running:  result.IgnoreRunning,
		},
		{
			name:     "running is neutral when shown",
			result:   statuspb.TestStatus_RUNNING,
			expected: statuspb.TestStatus_UNKNOWN,
			running:  result.ShowRunning,
		},
		{
			name:     "fail is fail",
			result:   statuspb.TestStatus_FAIL,
			expected: statuspb.TestStatus_FAIL,
		},
		{
			name:     "flaky is flaky",
			result:   statuspb.TestStatus_FLAKY,
			expected: statuspb.TestStatus_FLAKY,
		},
		{
			name:     "simplify pass",
			result:   statuspb.TestStatus_PASS_WITH_ERRORS,
			expected: statuspb.TestStatus_PASS,
		},
		{
			name:     "categorized abort is neutral",
			result:   statuspb.TestStatus_CATEGORIZED_ABORT,
			expected: statuspb.TestStatus_UNKNOWN,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual := coalesceResult(tc.result, tc.running); actual != tc.expected {
				t.Errorf("actual %s != expected %s", actual, tc.expected)
			}
		})
	}
}

func TestResultIter(t *testing.T) {
	cases := []struct {
		name     string
		cancel   int
		in       []int32
		expected []statuspb.TestStatus
	}{
		{
			name: "basically works",
			in: []int32{
				int32(statuspb.TestStatus_PASS), 3,
				int32(statuspb.TestStatus_FAIL), 2,
			},
			expected: []statuspb.TestStatus{
				statuspb.TestStatus_PASS,
				statuspb.TestStatus_PASS,
				statuspb.TestStatus_PASS,
				statuspb.TestStatus_FAIL,
				statuspb.TestStatus_FAIL,
			},
		},
		{
			name: "ignore last unbalanced input",
			in: []int32{
				int32(statuspb.TestStatus_PASS), 3,
				int32(statuspb.TestStatus_FAIL),
			},
			expected: []statuspb.TestStatus{
				statuspb.TestStatus_PASS,
				statuspb.TestStatus_PASS,
				statuspb.TestStatus_PASS,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			iter := result.Iter(tc.in)
			var actual []statuspb.TestStatus
			var idx int
			for {
				val, ok := iter()
				if !ok {
					return
				}
				idx++
				actual = append(actual, val)
			}
			if !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("%s != expected %s", actual, tc.expected)
			}
		})
	}
}

func TestSummaryPath(t *testing.T) {
	mustPath := func(s string) *gcs.Path {
		p, err := gcs.NewPath(s)
		if err != nil {
			t.Fatalf("gcs.NewPath(%q) got err: %v", s, err)
		}
		return p
	}
	cases := []struct {
		name   string
		path   gcs.Path
		prefix string
		dash   string
		want   *gcs.Path
		err    bool
	}{
		{
			name: "normal",
			path: *mustPath("gs://bucket/config"),
			dash: "hello",
			want: mustPath("gs://bucket/summary-hello"),
		},
		{
			name:   "prefix", // construct path with a prefix correctly
			path:   *mustPath("gs://bucket/config"),
			prefix: "summary",
			dash:   "hello",
			want:   mustPath("gs://bucket/summary/summary-hello"),
		},
		{
			name:   "normalize", // normalize dashboard name correctly
			path:   *mustPath("gs://bucket/config"),
			prefix: "UpperCase",       // do not normalize
			dash:   "Hello --- World", // normalize
			want:   mustPath("gs://bucket/UpperCase/summary-helloworld"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := summaryPath(tc.path, tc.prefix, tc.dash)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("summaryPath(%q, %q, %q) got unexpected error: %v", tc.path, tc.prefix, tc.dash, err)
				}
			case tc.err:
				t.Errorf("summaryPath(%q, %q, %q) failed to get an error", tc.path, tc.prefix, tc.name)
			default:
				if diff := cmp.Diff(tc.want, got, cmp.AllowUnexported(gcs.Path{})); diff != "" {
					t.Errorf("summaryPath(%q, %q, %q) got unexpected diff (-want +got):\n%s", tc.path, tc.prefix, tc.dash, diff)
				}
			}
		})
	}
}
