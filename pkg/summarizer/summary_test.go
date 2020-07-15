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
	"net/url"
	"reflect"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/stretchr/testify/assert"

	"github.com/GoogleCloudPlatform/testgrid/internal/result"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/pb/state"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	summarypb "github.com/GoogleCloudPlatform/testgrid/pb/summary"
)

type fakeGroup struct {
	group configpb.TestGroup
	grid  statepb.Grid
	mod   time.Time
	gen   int64
	err   error
}

func TestUpdateDashboard(t *testing.T) {
	cases := []struct {
		name     string
		dash     *configpb.Dashboard
		groups   map[string]fakeGroup
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
					},
				},
			},
			groups: map[string]fakeGroup{
				"foo-group": {
					mod: time.Unix(1000, 0),
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
					},
					{
						Name:          "missing-tab",
						TestGroupName: "group-not-present",
					},
					{
						Name:          "error-tab",
						TestGroupName: "has-errors",
					},
					{
						Name:          "still-working",
						TestGroupName: "working-group",
					},
				},
			},
			groups: map[string]fakeGroup{
				"working-group": {
					mod: time.Unix(1000, 0),
				},
				"has-errors": {
					err: errors.New("tragedy"),
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
					problemTab("a-dashboard", "missing-tab"),
					problemTab("a-dashboard", "error-tab"),
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
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			finder := func(name string) (*configpb.TestGroup, gridReader, error) {
				if name == "inject-error" {
					return nil, nil, errors.New("injected find group error")
				}
				fake, ok := tc.groups[name]
				if !ok {
					return nil, nil, nil
				}
				reader := func(_ context.Context) (io.ReadCloser, time.Time, int64, error) {
					return ioutil.NopCloser(bytes.NewBuffer(compress(gridBuf(&fake.grid)))), fake.mod, fake.gen, fake.err
				}
				return &fake.group, reader, nil
			}
			actual, err := updateDashboard(context.Background(), tc.dash, finder)
			if err != nil && !tc.err {
				t.Errorf("unexpected error: %v", err)
			}
			if tc.err && err == nil {
				t.Error("failed to receive expected error")
			}
			if !proto.Equal(actual, tc.expected) {
				t.Errorf("actual dashboard summary %s != expected %s", actual, tc.expected)
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
		findError error
		grid      statepb.Grid
		mod       time.Time
		gen       int64
		gridError error
		expected  *summarypb.DashboardTabSummary
		err       bool
	}{
		{
			name:      "find grid error returns error",
			findError: errors.New("failed to resolve url reference"),
			err:       true,
		},
		{
			name: "grid does not exist returns error",
			err:  true,
		},
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
			},
			group: &configpb.TestGroup{},
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
			expected: &summarypb.DashboardTabSummary{
				DashboardTabName: "you know",
				Alert:            noRuns,
				OverallStatus:    summarypb.DashboardTabSummary_STALE,
				Status:           noRuns,
				LatestGreen:      noGreens,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			finder := func(name string) (*configpb.TestGroup, gridReader, error) {
				if name != tc.tab.TestGroupName {
					t.Fatalf("wrong name: %q != expected %q", name, tc.tab.TestGroupName)
				}
				if tc.findError != nil {
					return nil, nil, tc.findError
				}
				reader := func(_ context.Context) (io.ReadCloser, time.Time, int64, error) {
					if tc.gridError != nil {
						return nil, time.Time{}, 0, tc.gridError
					}
					return ioutil.NopCloser(bytes.NewBuffer(compress(gridBuf(&tc.grid)))), tc.mod, tc.gen, nil
				}
				return tc.group, reader, nil
			}

			if tc.tab == nil {
				tc.tab = &configpb.DashboardTab{}
			}
			actual, err := updateTab(context.Background(), tc.tab, finder)
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

func TestFilterGrid(t *testing.T) {
	cases := []struct {
		name        string
		baseOptions string
		rows        []*statepb.Row
		recent      int
		expected    []*statepb.Row
		err         bool
	}{
		{
			name: "basically works",
		},
		{
			name:        "bad options returns error",
			baseOptions: "%z",
			err:         true,
		},
		{
			name: "everything works",
			baseOptions: url.Values{
				includeFilter: []string{"foo"},
				excludeFilter: []string{"bar"},
			}.Encode(),
			rows: []*statepb.Row{
				{
					Name:    "include-food",
					Results: []int32{int32(statepb.Row_PASS), 10},
				},
				{
					Name:    "exclude-included-bart",
					Results: []int32{int32(statepb.Row_PASS), 10},
				},
				{
					Name: "ignore-included-stale",
					Results: []int32{
						int32(statepb.Row_NO_RESULT), 5,
						int32(statepb.Row_PASS_WITH_SKIPS), 10,
					},
				},
			},
			recent: 5,
			expected: []*statepb.Row{
				{
					Name:    "include-food",
					Results: []int32{int32(statepb.Row_PASS), 10},
				},
			},
		},
		{
			name: "must match all includes",
			baseOptions: url.Values{
				includeFilter: []string{"foo", "spam"},
			}.Encode(),
			rows: []*statepb.Row{
				{
					Name: "spam-is-food",
				},
				{
					Name: "spam-musubi",
				},
				{
					Name: "unagi-food",
				},
				{
					Name: "email-spam-is-not-food",
				},
			},
			expected: []*statepb.Row{
				{
					Name: "spam-is-food",
				},
				{
					Name: "email-spam-is-not-food",
				},
			},
		},
		{
			name: "exclude any exclusions",
			baseOptions: url.Values{
				excludeFilter: []string{"not", "nope"},
			}.Encode(),
			rows: []*statepb.Row{
				{
					Name: "yes please",
				},
				{
					Name: "leslie knope",
				},
				{
					Name: "not eating",
				},
				{
					Name: "fluffy waffles",
				},
			},
			expected: []*statepb.Row{
				{
					Name: "yes please",
				},
				{
					Name: "fluffy waffles",
				},
			},
		},
		{
			name: "bad inclusion regexp errors",
			baseOptions: url.Values{
				includeFilter: []string{"this.("},
			}.Encode(),
			err: true,
		},
		{
			name: "bad exclude regexp errors",
			baseOptions: url.Values{
				excludeFilter: []string{"this.("},
			}.Encode(),
			err: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, r := range tc.rows {
				if r.Results == nil {
					r.Results = []int32{int32(statepb.Row_PASS), 100}
				}
			}
			for _, r := range tc.expected {
				if r.Results == nil {
					r.Results = []int32{int32(statepb.Row_PASS), 100}
				}
			}
			actual, err := filterGrid(tc.baseOptions, tc.rows, tc.recent)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Error("failed to return an error")
			case !reflect.DeepEqual(actual, tc.expected):
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
					Results: []int32{int32(statepb.Row_PASS), recent},
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
					Results: []int32{int32(statepb.Row_PASS), recent},
				},
				{
					Name:    "skip-this-one-with-no-recent-results",
					Results: []int32{int32(statepb.Row_NO_RESULT), recent},
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
						int32(statepb.Row_NO_RESULT), recent - 1,
						int32(statepb.Row_PASS_WITH_SKIPS), recent,
					},
				},
				{
					Name: "tail skips",
					Results: []int32{
						int32(statepb.Row_FLAKY), recent - 1,
						int32(statepb.Row_NO_RESULT), recent,
					},
				},
				{
					Name: "middle skips",
					Results: []int32{
						int32(statepb.Row_FAIL), 1,
						int32(statepb.Row_NO_RESULT), recent - 2,
						int32(statepb.Row_PASS), 1,
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

func TestIncludeRows(t *testing.T) {
	cases := []struct {
		name     string
		names    []string
		include  string
		expected []string
		err      bool
	}{
		{
			name: "basically works",
		},
		{
			name:    "bad regex errors",
			include: "^[a-z",
			err:     true,
		},
		{
			name:    "return nothing rows when nothing matches",
			names:   []string{"hello", "world"},
			include: "dog",
		},
		{
			name:     "include only matching rows",
			include:  "fun",
			names:    []string{"apply", "function", "to", "funny", "bone"},
			expected: []string{"function", "funny"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rows []*statepb.Row
			for _, n := range tc.names {
				rows = append(rows, &statepb.Row{Name: n})
			}
			actualRows, err := includeRows(rows, tc.include)
			var actual []string
			for _, r := range actualRows {
				actual = append(actual, r.Name)
			}
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Error("failed to return expected error")
			case !reflect.DeepEqual(actual, tc.expected):
				t.Errorf("actual %s != expected %s", actual, tc.expected)
			}
		})
	}
}

func TestExcludeRows(t *testing.T) {
	cases := []struct {
		name     string
		names    []string
		exclude  string
		expected []string
		err      bool
	}{
		{
			name: "basically works",
		},
		{
			name:    "bad regex errors",
			exclude: "^[a-z",
			err:     true,
		},
		{
			name:     "return all rows when nothing matches",
			names:    []string{"hello", "world"},
			exclude:  "dog",
			expected: []string{"hello", "world"},
		},
		{
			name:     "drop matching rows",
			exclude:  "fun",
			names:    []string{"apply", "function", "to", "funny", "bone"},
			expected: []string{"apply", "to", "bone"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rows []*statepb.Row
			for _, n := range tc.names {
				rows = append(rows, &statepb.Row{Name: n})
			}
			actualRows, err := excludeRows(rows, tc.exclude)
			var actual []string
			for _, r := range actualRows {
				actual = append(actual, r.Name)
			}
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Error("failed to return expected error")
			case !reflect.DeepEqual(actual, tc.expected):
				t.Errorf("actual %s != expected %s", actual, tc.expected)
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
					Started: 333.333,
				},
				{
					Started: 222,
				},
			},
			expectedTime: time.Unix(333, 0),
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
					Name: "foo-name",
					Id:   "foo-target",
					AlertInfo: &statepb.AlertInfo{
						FailBuildId:    "bad",
						PassBuildId:    "good",
						FailCount:      6,
						BuildLink:      "to the past",
						BuildLinkText:  "hyrule",
						BuildUrlText:   "of sandwich",
						FailureMessage: "pop tart",
					},
				},
				{},
				{
					Name: "bar-name",
					Id:   "bar-target",
					AlertInfo: &statepb.AlertInfo{
						FailBuildId:    "fbi",
						PassBuildId:    "pbi",
						FailTestId:     "819283y823-1232813",
						FailCount:      1,
						BuildLink:      "bl",
						BuildLinkText:  "blt",
						BuildUrlText:   "but",
						FailureMessage: "fm",
					},
				},
				{},
			},
			expected: []*summarypb.FailingTestSummary{
				{
					DisplayName:    "foo-name",
					TestName:       "foo-target",
					FailBuildId:    "bad",
					PassBuildId:    "good",
					FailCount:      6,
					BuildLink:      "to the past",
					BuildLinkText:  "hyrule",
					BuildUrlText:   "of sandwich",
					FailureMessage: "pop tart",
					FailTestLink:   " foo-target",
				},
				{
					DisplayName:    "bar-name",
					TestName:       "bar-target",
					FailBuildId:    "fbi",
					PassBuildId:    "pbi",
					FailCount:      1,
					BuildLink:      "bl",
					BuildLinkText:  "blt",
					BuildUrlText:   "but",
					FailureMessage: "fm",
					FailTestLink:   "819283y823-1232813 bar-target",
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual := failingTestSummaries(tc.rows); !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("%v != expected %v", actual, tc.expected)
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
					Results: []int32{int32(statepb.Row_PASS), 1},
				},
			},
			expected: summarypb.DashboardTabSummary_PASS,
		},
		{
			name:   "non-passing results without an alert results in flaky",
			recent: 1,
			rows: []*statepb.Row{
				{
					Results: []int32{int32(statepb.Row_FAIL), 1},
				},
			},
			expected: summarypb.DashboardTabSummary_FLAKY,
		},
		{
			name:   "do not consider still-running results as flaky",
			recent: 5,
			rows: []*statepb.Row{
				{
					Results: []int32{int32(statepb.Row_NO_RESULT), 1},
				},
				{
					Results: []int32{int32(statepb.Row_PASS), 3},
				},
				{
					Results: []int32{int32(statepb.Row_NO_RESULT), 2},
				},
				{
					Results: []int32{int32(statepb.Row_PASS), 2},
				},
			},
			expected: summarypb.DashboardTabSummary_PASS,
		},
		{
			name:   "ignore old failures",
			recent: 1,
			rows: []*statepb.Row{
				{
					Results: []int32{
						int32(statepb.Row_PASS), 3,
						int32(statepb.Row_FAIL), 5,
					},
				},
			},
			expected: summarypb.DashboardTabSummary_PASS,
		},
		{
			name:   "partial results work",
			recent: 50,
			rows: []*statepb.Row{
				{
					Results: []int32{int32(statepb.Row_PASS), 1},
				},
			},
			expected: summarypb.DashboardTabSummary_PASS,
		},
		{
			name:   "coalesce passes",
			recent: 1,
			rows: []*statepb.Row{
				{
					Results: []int32{int32(statepb.Row_PASS_WITH_SKIPS), 1},
				},
			},
			expected: summarypb.DashboardTabSummary_PASS,
		},
		{
			name:   "broken cycle",
			recent: 1,
			rows: []*statepb.Row{
				{
					Results: []int32{int32(statepb.Row_PASS_WITH_SKIPS), 1},
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
					Results: []int32{int32(statepb.Row_PASS), 2},
				},
				{
					Name:    "and ham",
					Results: []int32{int32(statepb.Row_PASS), 2},
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
					Results: []int32{int32(statepb.Row_FAIL), 2},
				},
				{
					Name:    "not in a box",
					Results: []int32{int32(statepb.Row_FLAKY), 2},
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
						int32(statepb.Row_PASS), 1,
						int32(statepb.Row_FAIL), 1,
					},
				},
				{
					Name: "fine wine gets better",
					Results: []int32{
						int32(statepb.Row_FAIL), 1,
						int32(statepb.Row_PASS), 1,
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
					Results: []int32{int32(statepb.Row_PASS), 3},
				},
				{
					Name:    "b",
					Results: []int32{int32(statepb.Row_PASS), 3},
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
					Results: []int32{int32(statepb.Row_PASS), 2},
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
					Results: []int32{int32(statepb.Row_PASS), 100},
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
					Results: []int32{int32(statepb.Row_NO_RESULT), 3},
				},
				{
					Name: "first empty",
					Results: []int32{
						int32(statepb.Row_NO_RESULT), 1,
						int32(statepb.Row_PASS), 2,
					},
				},
				{
					Name: "always pass",
					Results: []int32{
						int32(statepb.Row_PASS), 3,
					},
				},
				{
					Name: "empty, fail, pass",
					Results: []int32{
						int32(statepb.Row_NO_RESULT), 1,
						int32(statepb.Row_FAIL), 1,
						int32(statepb.Row_PASS), 1,
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
						int32(statepb.Row_PASS), 4,
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
						int32(statepb.Row_PASS), 4,
					},
				},
				{
					Name: "four fails",
					Results: []int32{
						int32(statepb.Row_FAIL), 4,
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
					Results: []int32{int32(statepb.Row_NO_RESULT), 3},
				},
				{
					Name: "first empty",
					Results: []int32{
						int32(statepb.Row_NO_RESULT), 1,
						int32(statepb.Row_PASS), 2,
					},
				},
				{
					Name:    "always empty",
					Results: []int32{int32(statepb.Row_NO_RESULT), 3},
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
					Results: []int32{int32(statepb.Row_NO_RESULT), 3},
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
					Results: []int32{int32(statepb.Row_PASS_WITH_SKIPS), 3},
				},
				{
					Name:    "all pass",
					Results: []int32{int32(statepb.Row_PASS), 3},
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
					Results: []int32{int32(statepb.Row_PASS_WITH_ERRORS), 3},
				},
				{
					Name:    "all pass",
					Results: []int32{int32(statepb.Row_PASS), 3},
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
						int32(statepb.Row_PASS), 4,
					},
				},
				{
					Name: "four fails",
					Results: []int32{
						int32(statepb.Row_FAIL), 4,
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
						int32(statepb.Row_PASS), 4,
					},
				},
				{
					Name: "four fails",
					Results: []int32{
						int32(statepb.Row_FAIL), 4,
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
						int32(statepb.Row_PASS), 4,
					},
				},
				{
					Name: "one pass three fails",
					Results: []int32{
						int32(statepb.Row_FAIL), 1,
						int32(statepb.Row_PASS), 3,
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
						int32(statepb.Row_PASS), 4,
					},
				},
				{
					Name: "one pass three fails",
					Results: []int32{
						int32(statepb.Row_FAIL), 1,
						int32(statepb.Row_PASS), 3,
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
					Results: []int32{int32(statepb.Row_PASS), 4},
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
					Results: []int32{int32(statepb.Row_PASS), 4},
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
					Results: []int32{int32(statepb.Row_PASS), 4},
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
						int32(statepb.Row_PASS_WITH_ERRORS), 1,
						int32(statepb.Row_PASS), 1,
					},
				},
				{
					Name:    "pass pass",
					Results: []int32{int32(statepb.Row_PASS), 2},
				},
				{
					Name: "pass and skip",
					Results: []int32{
						int32(statepb.Row_PASS_WITH_SKIPS), 1,
						int32(statepb.Row_PASS), 1,
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
						int32(statepb.Row_RUNNING), 1,
						int32(statepb.Row_PASS), 1,
					},
				},
				{
					Name: "pass",
					Results: []int32{
						int32(statepb.Row_PASS), 2,
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
						int32(statepb.Row_FLAKY), 1,
						int32(statepb.Row_PASS), 1,
					},
				},
				{
					Name: "passing",
					Results: []int32{
						int32(statepb.Row_PASS), 2,
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
						int32(statepb.Row_FAIL), 1,
						int32(statepb.Row_PASS), 1,
					},
				},
				{
					Name: "passing",
					Results: []int32{
						int32(statepb.Row_PASS), 2,
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
			grid: &state.Grid{
				Columns: []*state.Column{
					{Started: withinCurrentInterval},
					{Started: withinCurrentInterval},
					{Started: withinPreviousInterval},
					{Started: withinPreviousInterval},
					{Started: notWithinAnyInterval},
				},
				Rows: []*state.Row{
					{
						Name: "test_1",
						Results: []int32{
							state.Row_Result_value["PASS"], 1,
							state.Row_Result_value["FAIL"], 1,
							state.Row_Result_value["FAIL"], 1,
							state.Row_Result_value["FAIL"], 2,
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
						ChangeFromLastInterval: summarypb.TestInfo_DOWN,
					},
				},
				AverageFlakiness: 50.0,
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
		result   statepb.Row_Result
		running  bool
		expected statepb.Row_Result
	}{
		{
			name:     "no result by default",
			expected: statepb.Row_NO_RESULT,
		},
		{
			name:     "running is no result when ignored",
			result:   statepb.Row_RUNNING,
			expected: statepb.Row_NO_RESULT,
			running:  result.IgnoreRunning,
		},
		{
			name:     "running is no result when ignored",
			result:   statepb.Row_RUNNING,
			expected: statepb.Row_FAIL,
			running:  result.FailRunning,
		},
		{
			name:     "fail is fail",
			result:   statepb.Row_FAIL,
			expected: statepb.Row_FAIL,
		},
		{
			name:     "flaky is flaky",
			result:   statepb.Row_FLAKY,
			expected: statepb.Row_FLAKY,
		},
		{
			name:     "simplify pass",
			result:   statepb.Row_PASS_WITH_ERRORS,
			expected: statepb.Row_PASS,
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
		expected []statepb.Row_Result
	}{
		{
			name: "basically works",
			in: []int32{
				int32(statepb.Row_PASS), 3,
				int32(statepb.Row_FAIL), 2,
			},
			expected: []statepb.Row_Result{
				statepb.Row_PASS,
				statepb.Row_PASS,
				statepb.Row_PASS,
				statepb.Row_FAIL,
				statepb.Row_FAIL,
			},
		},
		{
			name: "ignore last unbalanced input",
			in: []int32{
				int32(statepb.Row_PASS), 3,
				int32(statepb.Row_FAIL),
			},
			expected: []statepb.Row_Result{
				statepb.Row_PASS,
				statepb.Row_PASS,
				statepb.Row_PASS,
			},
		},
		{
			name: "cancel aborts early",
			in: []int32{
				int32(statepb.Row_PASS), 50,
			},
			cancel: 2,
			expected: []statepb.Row_Result{
				statepb.Row_PASS,
				statepb.Row_PASS,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			out := resultIter(ctx, tc.in)
			var actual []statepb.Row_Result
			var idx int
			for val := range out {
				idx++
				if tc.cancel > 0 && idx == tc.cancel {
					cancel()
				}
				actual = append(actual, val)
			}
			if !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("%s != expected %s", actual, tc.expected)
			}
		})
	}
}
