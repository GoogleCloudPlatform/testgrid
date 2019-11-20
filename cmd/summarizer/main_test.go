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

package main

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"io"
	"io/ioutil"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"

	"github.com/GoogleCloudPlatform/testgrid/internal/result"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/pb/state"
	"github.com/GoogleCloudPlatform/testgrid/pb/summary"
)

type fakeGroup struct {
	group configpb.TestGroup
	grid  state.Grid
	mod   time.Time
	gen   int64
	err   error
}

func TestUpdateDashboard(t *testing.T) {
	cases := []struct {
		name     string
		dash     configpb.Dashboard
		groups   map[string]fakeGroup
		expected *summary.DashboardSummary
		err      bool
	}{
		{
			name: "basically works",
			dash: configpb.Dashboard{
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
			expected: &summary.DashboardSummary{
				TabSummaries: []*summary.DashboardTabSummary{
					{
						DashboardTabName:    "stale-tab",
						LastUpdateTimestamp: 1000,
						Alert:               noRuns,
						OverallStatus:       summary.DashboardTabSummary_STALE,
						Status:              noRuns,
						LatestGreen:         noGreens,
					},
				},
			},
		},
		{
			name: "still update working tabs when some tabs fail",
			dash: configpb.Dashboard{
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
			expected: &summary.DashboardSummary{
				TabSummaries: []*summary.DashboardTabSummary{
					{
						DashboardTabName:    "working",
						LastUpdateTimestamp: 1000,
						Alert:               noRuns,
						Status:              noRuns,
						OverallStatus:       summary.DashboardTabSummary_STALE,
						LatestGreen:         noGreens,
					},
					problemTab("missing-tab"),
					problemTab("error-tab"),
					{
						DashboardTabName:    "still-working",
						LastUpdateTimestamp: 1000,
						Alert:               noRuns,
						Status:              noRuns,
						OverallStatus:       summary.DashboardTabSummary_STALE,
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
					return ioutil.NopCloser(bytes.NewBuffer(compress(gridBuf(fake.grid)))), fake.mod, fake.gen, fake.err
				}
				return &fake.group, reader, nil
			}
			actual, err := updateDashboard(context.Background(), &tc.dash, finder)
			if err != nil && !tc.err {
				t.Errorf("unexpected error: %v", err)
			}
			if tc.err && err == nil {
				t.Error("failed to receive expected error")
			}
			if !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("actual dashboard summary %s != expected %s", actual, tc.expected)
			}
		})
	}
}

func TestStaleHours(t *testing.T) {
	cases := []struct {
		name     string
		tab      configpb.DashboardTab
		expected time.Duration
	}{
		{
			name:     "zero without an alert",
			expected: 0,
		},
		{
			name: "use defined hours when set",
			tab: configpb.DashboardTab{
				AlertOptions: &configpb.DashboardTabAlertOptions{
					AlertStaleResultsHours: 4,
				},
			},
			expected: 4 * time.Hour,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual := staleHours(&tc.tab); actual != tc.expected {
				t.Errorf("actual %v != expected %v", actual, tc.expected)
			}
		})
	}
}

func gridBuf(grid state.Grid) []byte {
	buf, err := proto.Marshal(&grid)
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
		tab       configpb.DashboardTab
		group     *configpb.TestGroup
		findError error
		grid      state.Grid
		mod       time.Time
		gen       int64
		gridError error
		expected  *summary.DashboardTabSummary
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
			tab: configpb.DashboardTab{
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
			tab: configpb.DashboardTab{
				Name:          "foo-tab",
				TestGroupName: "foo-group",
			},
			group: &configpb.TestGroup{},
			mod:   now,
			gen:   43,
			expected: &summary.DashboardTabSummary{
				DashboardTabName:    "foo-tab",
				LastUpdateTimestamp: float64(now.Unix()),
				Alert:               noRuns,
				LatestGreen:         noGreens,
				OverallStatus:       summary.DashboardTabSummary_STALE,
				Status:              noRuns,
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
					return ioutil.NopCloser(bytes.NewBuffer(compress(gridBuf(tc.grid)))), tc.mod, tc.gen, nil
				}
				return tc.group, reader, nil
			}

			actual, err := updateTab(context.Background(), &tc.tab, finder)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Errorf("failed to receive expected error")
			case !reflect.DeepEqual(actual, tc.expected):
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
		expectedGrid *state.Grid
		expectErr    bool
	}{
		{
			name:      "error opening returns error",
			err:       errors.New("open failed"),
			expectErr: true,
		},
		{
			name: "return error when state is not compressed",
			reader: bytes.NewBuffer(gridBuf(state.Grid{
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
			reader: bytes.NewBuffer(compress(gridBuf(state.Grid{
				Columns: []*state.Column{
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
			reader: bytes.NewBuffer(compress(gridBuf(state.Grid{
				LastTimeUpdated: 555,
			}))),
			expectedGrid: &state.Grid{
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
			case !reflect.DeepEqual(actualGrid, tc.expectedGrid):
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
		rows        []*state.Row
		recent      int
		expected    []*state.Row
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
			rows: []*state.Row{
				{
					Name:    "include-food",
					Results: []int32{int32(state.Row_PASS), 10},
				},
				{
					Name:    "exclude-included-bart",
					Results: []int32{int32(state.Row_PASS), 10},
				},
				{
					Name: "ignore-included-stale",
					Results: []int32{
						int32(state.Row_NO_RESULT), 5,
						int32(state.Row_PASS_WITH_SKIPS), 10,
					},
				},
			},
			recent: 5,
			expected: []*state.Row{
				{
					Name:    "include-food",
					Results: []int32{int32(state.Row_PASS), 10},
				},
			},
		},
		{
			name: "must match all includes",
			baseOptions: url.Values{
				includeFilter: []string{"foo", "spam"},
			}.Encode(),
			rows: []*state.Row{
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
			expected: []*state.Row{
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
			rows: []*state.Row{
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
			expected: []*state.Row{
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
					r.Results = []int32{int32(state.Row_PASS), 100}
				}
			}
			for _, r := range tc.expected {
				if r.Results == nil {
					r.Results = []int32{int32(state.Row_PASS), 100}
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
		rows     []*state.Row
		expected []string
	}{
		{
			name: "basically works",
		},
		{
			name: "skip row with nil results",
			rows: []*state.Row{
				{
					Name:    "include",
					Results: []int32{int32(state.Row_PASS), recent},
				},
				{
					Name: "skip-nil-results",
				},
			},
			expected: []string{"include"},
		},
		{
			name: "skip row with no recent results",
			rows: []*state.Row{
				{
					Name:    "include",
					Results: []int32{int32(state.Row_PASS), recent},
				},
				{
					Name:    "skip-this-one-with-no-recent-results",
					Results: []int32{int32(state.Row_NO_RESULT), recent},
				},
			},
			expected: []string{"include"},
		},
		{
			name: "include rows missing some recent results",
			rows: []*state.Row{
				{
					Name: "head skips",
					Results: []int32{
						int32(state.Row_NO_RESULT), recent - 1,
						int32(state.Row_PASS_WITH_SKIPS), recent,
					},
				},
				{
					Name: "tail skips",
					Results: []int32{
						int32(state.Row_FLAKY), recent - 1,
						int32(state.Row_NO_RESULT), recent,
					},
				},
				{
					Name: "middle skips",
					Results: []int32{
						int32(state.Row_FAIL), 1,
						int32(state.Row_NO_RESULT), recent - 2,
						int32(state.Row_PASS), 1,
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
			var rows []*state.Row
			for _, n := range tc.names {
				rows = append(rows, &state.Row{Name: n})
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
			var rows []*state.Row
			for _, n := range tc.names {
				rows = append(rows, &state.Row{Name: n})
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
				t.Error("actual %s != expected %s", actual, tc.expected)
			}
		})
	}

}

func TestLatestRun(t *testing.T) {
	cases := []struct {
		name         string
		cols         []*state.Column
		expectedTime time.Time
		expectedSecs int64
	}{
		{
			name: "basically works",
		},
		{
			name: "zero started returns zero time",
			cols: []*state.Column{
				{},
			},
		},
		{
			name: "return first time in unix",
			cols: []*state.Column{
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
		rows     []*state.Row
		expected []*summary.FailingTestSummary
	}{
		{
			name: "do not alert by default",
			rows: []*state.Row{
				{},
				{},
			},
		},
		{
			name: "alert when rows have alerts",
			rows: []*state.Row{
				{},
				{
					Name: "foo-name",
					Id:   "foo-target",
					AlertInfo: &state.AlertInfo{
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
					AlertInfo: &state.AlertInfo{
						FailBuildId:    "fbi",
						PassBuildId:    "pbi",
						FailCount:      1,
						BuildLink:      "bl",
						BuildLinkText:  "blt",
						BuildUrlText:   "but",
						FailureMessage: "fm",
					},
				},
				{},
			},
			expected: []*summary.FailingTestSummary{
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
		rows     []*state.Row
		recent   int
		stale    string
		alerts   bool
		expected summary.DashboardTabSummary_TabStatus
	}{
		{
			name:     "unknown by default",
			expected: summary.DashboardTabSummary_UNKNOWN,
		},
		{
			name:     "stale joke results in stale summary",
			stale:    "joke",
			expected: summary.DashboardTabSummary_STALE,
		},
		{
			name:     "alerts result in failure",
			alerts:   true,
			expected: summary.DashboardTabSummary_FAIL,
		},
		{
			name:     "prefer stale over failure",
			stale:    "potato chip",
			alerts:   true,
			expected: summary.DashboardTabSummary_STALE,
		},
		{
			name:   "completed results result in pass",
			recent: 1,
			rows: []*state.Row{
				{
					Results: []int32{int32(state.Row_PASS), 1},
				},
			},
			expected: summary.DashboardTabSummary_PASS,
		},
		{
			name:   "non-passing results without an alert results in flaky",
			recent: 1,
			rows: []*state.Row{
				{
					Results: []int32{int32(state.Row_FAIL), 1},
				},
			},
			expected: summary.DashboardTabSummary_FLAKY,
		},
		{
			name:   "do not consider still-running results as flaky",
			recent: 5,
			rows: []*state.Row{
				{
					Results: []int32{int32(state.Row_NO_RESULT), 1},
				},
				{
					Results: []int32{int32(state.Row_PASS), 3},
				},
				{
					Results: []int32{int32(state.Row_NO_RESULT), 2},
				},
				{
					Results: []int32{int32(state.Row_PASS), 2},
				},
			},
			expected: summary.DashboardTabSummary_PASS,
		},
		{
			name:   "ignore old failures",
			recent: 1,
			rows: []*state.Row{
				{
					Results: []int32{
						int32(state.Row_PASS), 3,
						int32(state.Row_FAIL), 5,
					},
				},
			},
			expected: summary.DashboardTabSummary_PASS,
		},
		{
			name:   "partial results work",
			recent: 50,
			rows: []*state.Row{
				{
					Results: []int32{int32(state.Row_PASS), 1},
				},
			},
			expected: summary.DashboardTabSummary_PASS,
		},
		{
			name:   "coalesce passes",
			recent: 1,
			rows: []*state.Row{
				{
					Results: []int32{int32(state.Row_PASS_WITH_SKIPS), 1},
				},
			},
			expected: summary.DashboardTabSummary_PASS,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var alerts []*summary.FailingTestSummary
			if tc.alerts {
				alerts = append(alerts, &summary.FailingTestSummary{})
			}

			if actual := overallStatus(&state.Grid{Rows: tc.rows}, tc.recent, tc.stale, alerts); actual != tc.expected {
				t.Errorf("%s != expected %s", actual, tc.expected)
			}
		})
	}
}

func TestStatusMessage(t *testing.T) {
	cases := []struct {
		name             string
		cols             int
		rows             []*state.Row
		recent           int
		passingCols      int
		filledCols       int
		passingCells     int
		filledCells      int
		expectedOverride string
	}{
		{
			name:             "no runs",
			expectedOverride: noRuns,
		},
		{
			name: "what people want (greens)",
			cols: 2,
			rows: []*state.Row{
				{
					Name:    "green eggs",
					Results: []int32{int32(state.Row_PASS), 2},
				},
				{
					Name:    "and ham",
					Results: []int32{int32(state.Row_PASS), 2},
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
			rows: []*state.Row{
				{
					Name:    "not with a fox",
					Results: []int32{int32(state.Row_FAIL), 2},
				},
				{
					Name:    "not in a box",
					Results: []int32{int32(state.Row_FLAKY), 2},
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
			rows: []*state.Row{
				{
					Name: "first doughnut is best",
					Results: []int32{
						int32(state.Row_PASS), 1,
						int32(state.Row_FAIL), 1,
					},
				},
				{
					Name: "fine wine gets better",
					Results: []int32{
						int32(state.Row_FAIL), 1,
						int32(state.Row_PASS), 1,
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
			rows: []*state.Row{
				{
					Name:    "a",
					Results: []int32{int32(state.Row_PASS), 3},
				},
				{
					Name:    "b",
					Results: []int32{int32(state.Row_PASS), 3},
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
			rows: []*state.Row{
				{
					Name: "empty",
				},
				{
					Name:    "filled",
					Results: []int32{int32(state.Row_PASS), 2},
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
			rows: []*state.Row{
				{
					Name:    "data",
					Results: []int32{int32(state.Row_PASS), 100},
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
			rows: []*state.Row{
				{
					Name:    "always empty",
					Results: []int32{int32(state.Row_NO_RESULT), 3},
				},
				{
					Name: "first empty",
					Results: []int32{
						int32(state.Row_NO_RESULT), 1,
						int32(state.Row_PASS), 2,
					},
				},
				{
					Name: "always pass",
					Results: []int32{
						int32(state.Row_PASS), 3,
					},
				},
				{
					Name: "empty, fail, pass",
					Results: []int32{
						int32(state.Row_NO_RESULT), 1,
						int32(state.Row_FAIL), 1,
						int32(state.Row_PASS), 1,
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
			cols:   2,
			recent: 50,
			rows: []*state.Row{
				{
					Name:    "two passes",
					Results: []int32{int32(state.Row_PASS), 2},
				},
			},
			passingCols:  2,
			filledCols:   2,
			passingCells: 2,
			filledCells:  2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expected := tc.expectedOverride
			if expected == "" {
				expected = fmtStatus(tc.passingCols, tc.filledCols, tc.passingCells, tc.filledCells)
			}
			if actual := statusMessage(tc.cols, tc.rows, tc.recent); actual != expected {
				t.Errorf("%s != expected %s", actual, expected)
			}
		})
	}
}

func TestLatestGreen(t *testing.T) {
	cases := []struct {
		name     string
		rows     []*state.Row
		cols     []*state.Column
		expected string
	}{
		{
			name:     "no recent greens by default",
			expected: noGreens,
		},
		{
			name: "favor first green",
			rows: []*state.Row{
				{
					Name:    "so pass",
					Results: []int32{int32(state.Row_PASS), 4},
				},
			},
			cols: []*state.Column{
				{
					Extra: []string{"hello", "there"},
				},
				{
					Extra: []string{"bad", "wrong"},
				},
			},
			expected: "hello",
		},
		{
			name: "accept any kind of pass",
			rows: []*state.Row{
				{
					Name: "pass w/ errors",
					Results: []int32{
						int32(state.Row_PASS_WITH_ERRORS), 1,
						int32(state.Row_PASS), 1,
					},
				},
				{
					Name:    "pass pass",
					Results: []int32{int32(state.Row_PASS), 2},
				},
				{
					Name: "pass and skip",
					Results: []int32{
						int32(state.Row_PASS_WITH_SKIPS), 1,
						int32(state.Row_PASS), 1,
					},
				},
			},
			cols: []*state.Column{
				{
					Extra: []string{"good"},
				},
				{
					Extra: []string{"bad"},
				},
			},
			expected: "good",
		},
		{
			name: "avoid columns with running rows",
			rows: []*state.Row{
				{
					Name: "running",
					Results: []int32{
						int32(state.Row_RUNNING), 1,
						int32(state.Row_PASS), 1,
					},
				},
				{
					Name: "pass",
					Results: []int32{
						int32(state.Row_PASS), 2,
					},
				},
			},
			cols: []*state.Column{
				{
					Extra: []string{"skip-first-col-still-running"},
				},
				{
					Extra: []string{"accept second-all-finished"},
				},
			},
			expected: "accept second-all-finished",
		},
		{
			name: "avoid columns with flakes",
			rows: []*state.Row{
				{
					Name: "flaking",
					Results: []int32{
						int32(state.Row_FLAKY), 1,
						int32(state.Row_PASS), 1,
					},
				},
				{
					Name: "passing",
					Results: []int32{
						int32(state.Row_PASS), 2,
					},
				},
			},
			cols: []*state.Column{
				{
					Extra: []string{"skip-first-col-with-flake"},
				},
				{
					Extra: []string{"accept second-no-flake"},
				},
			},
			expected: "accept second-no-flake",
		},
		{
			name: "avoid columns with failures",
			rows: []*state.Row{
				{
					Name: "failing",
					Results: []int32{
						int32(state.Row_FAIL), 1,
						int32(state.Row_PASS), 1,
					},
				},
				{
					Name: "passing",
					Results: []int32{
						int32(state.Row_PASS), 2,
					},
				},
			},
			cols: []*state.Column{
				{
					Extra: []string{"skip-first-col-with-fail"},
				},
				{
					Extra: []string{"accept second-after-failure"},
				},
			},
			expected: "accept second-after-failure",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			grid := state.Grid{
				Columns: tc.cols,
				Rows:    tc.rows,
			}
			if actual := latestGreen(&grid); actual != tc.expected {
				t.Errorf("%s != expected %s", actual, tc.expected)
			}
		})
	}
}

func TestCoalesceResult(t *testing.T) {
	cases := []struct {
		name     string
		result   state.Row_Result
		running  bool
		expected state.Row_Result
	}{
		{
			name:     "no result by default",
			expected: state.Row_NO_RESULT,
		},
		{
			name:     "running is no result when ignored",
			result:   state.Row_RUNNING,
			expected: state.Row_NO_RESULT,
			running:  result.IgnoreRunning,
		},
		{
			name:     "running is no result when ignored",
			result:   state.Row_RUNNING,
			expected: state.Row_FAIL,
			running:  result.FailRunning,
		},
		{
			name:     "fail is fail",
			result:   state.Row_FAIL,
			expected: state.Row_FAIL,
		},
		{
			name:     "flaky is flaky",
			result:   state.Row_FLAKY,
			expected: state.Row_FLAKY,
		},
		{
			name:     "simplify pass",
			result:   state.Row_PASS_WITH_ERRORS,
			expected: state.Row_PASS,
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
		expected []state.Row_Result
	}{
		{
			name: "basically works",
			in: []int32{
				int32(state.Row_PASS), 3,
				int32(state.Row_FAIL), 2,
			},
			expected: []state.Row_Result{
				state.Row_PASS,
				state.Row_PASS,
				state.Row_PASS,
				state.Row_FAIL,
				state.Row_FAIL,
			},
		},
		{
			name: "ignore last unbalanced input",
			in: []int32{
				int32(state.Row_PASS), 3,
				int32(state.Row_FAIL),
			},
			expected: []state.Row_Result{
				state.Row_PASS,
				state.Row_PASS,
				state.Row_PASS,
			},
		},
		{
			name: "cancel aborts early",
			in: []int32{
				int32(state.Row_PASS), 50,
			},
			cancel: 2,
			expected: []state.Row_Result{
				state.Row_PASS,
				state.Row_PASS,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			out := resultIter(ctx, tc.in)
			var actual []state.Row_Result
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
