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
	"reflect"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"

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
						OverallStatus:       summary.DashboardTabSummary_STALE,
						LatestGreen:         noGreens,
					},
					problemTab("missing-tab"),
					problemTab("error-tab"),
					{
						DashboardTabName:    "still-working",
						LastUpdateTimestamp: 1000,
						Alert:               noRuns,
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
