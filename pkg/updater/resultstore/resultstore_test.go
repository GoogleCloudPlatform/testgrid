/*
Copyright 2023 The TestGrid Authors.

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

package resultstore

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	"github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	"github.com/GoogleCloudPlatform/testgrid/pkg/updater"
	timestamppb "github.com/golang/protobuf/ptypes/timestamp"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/sirupsen/logrus"
	"google.golang.org/genproto/googleapis/devtools/resultstore/v2"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/testing/protocmp"
)

type fakeClient struct {
	searches    map[string][]string
	invocations map[string]fetchResult
}

func (c *fakeClient) SearchInvocations(ctx context.Context, req *resultstore.SearchInvocationsRequest, opts ...grpc.CallOption) (*resultstore.SearchInvocationsResponse, error) {
	notFound := fmt.Errorf("no results found for %q", req.GetQuery())
	if c.searches == nil {
		return nil, notFound
	}
	invocationIDs, ok := c.searches[req.GetQuery()]
	if !ok {
		return nil, notFound
	}
	var invocations []*resultstore.Invocation
	for _, invocationID := range invocationIDs {
		invoc := &resultstore.Invocation{
			Id: &resultstore.Invocation_Id{InvocationId: invocationID},
		}
		invocations = append(invocations, invoc)
	}
	return &resultstore.SearchInvocationsResponse{Invocations: invocations}, nil
}

func (c *fakeClient) ExportInvocation(ctx context.Context, req *resultstore.ExportInvocationRequest, opts ...grpc.CallOption) (*resultstore.ExportInvocationResponse, error) {
	notFound := fmt.Errorf("no result found for invocation %q", req.GetName())
	if c.invocations == nil {
		return nil, notFound
	}
	result, ok := c.invocations[req.GetName()]
	if !ok {
		return nil, notFound
	}
	return &resultstore.ExportInvocationResponse{
		Invocation:        result.Invocation,
		Actions:           result.Actions,
		ConfiguredTargets: result.ConfiguredTargets,
		Targets:           result.Targets,
	}, nil
}

func invocationName(invocationID string) string {
	return fmt.Sprintf("invocations/%s", invocationID)
}

func targetName(targetID, invocationID string) string {
	return fmt.Sprintf("invocations/%s/targets/%s", invocationID, targetID)
}

func timeMustText(t time.Time) string {
	s, err := t.MarshalText()
	if err != nil {
		panic("timeMustText() panicked")
	}
	return string(s)
}

func TestResultStoreColumnReader(t *testing.T) {
	// We already have functions testing 'stop' logic.
	// Scope this test to whether the column reader fetches and returns ascending results.
	oneMonthConfig := &configpb.TestGroup{
		Name:          "a-test-group",
		DaysOfResults: 30,
	}
	now := time.Now()
	oneDayAgo := now.AddDate(0, 0, -1)
	twoDaysAgo := now.AddDate(0, 0, -2)
	threeDaysAgo := now.AddDate(0, 0, -3)
	oneMonthAgo := now.AddDate(0, 0, -30)
	testQueryAfter := queryAfter(testQuery, oneMonthAgo)
	cases := []struct {
		name    string
		client  *fakeClient
		want    []updater.InflatedColumn
		wantErr bool
	}{
		{
			name:    "empty",
			wantErr: true,
		},
		{
			name: "basic",
			client: &fakeClient{
				searches: map[string][]string{
					testQueryAfter: {"id-1", "id-2", "id-3"},
				},
				invocations: map[string]fetchResult{
					invocationName("id-1"): {
						Invocation: &resultstore.Invocation{
							Id: &resultstore.Invocation_Id{
								InvocationId: "id-1",
							},
							Name: invocationName("id-1"),
							Timing: &resultstore.Timing{
								StartTime: &timestamppb.Timestamp{
									Seconds: oneDayAgo.Unix(),
								},
							},
						},
					},
					invocationName("id-2"): {
						Invocation: &resultstore.Invocation{
							Id: &resultstore.Invocation_Id{
								InvocationId: "id-2",
							},
							Name: invocationName("id-2"),
							Timing: &resultstore.Timing{
								StartTime: &timestamppb.Timestamp{
									Seconds: twoDaysAgo.Unix(),
								},
							},
						},
					},
					invocationName("id-3"): {
						Invocation: &resultstore.Invocation{
							Id: &resultstore.Invocation_Id{
								InvocationId: "id-3",
							},
							Name: invocationName("id-3"),
							Timing: &resultstore.Timing{
								StartTime: &timestamppb.Timestamp{
									Seconds: threeDaysAgo.Unix(),
								},
							},
						},
					},
				},
			},
			want: []updater.InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   fmt.Sprintf("%v", oneDayAgo.Unix()),
						Name:    fmt.Sprintf("%v", oneDayAgo.Unix()),
						Started: float64(oneDayAgo.Unix() * 1000),
						Hint:    timeMustText(oneDayAgo.Truncate(time.Second)),
					},
					Cells: map[string]updater.Cell{},
				},
				{
					Column: &statepb.Column{
						Build:   fmt.Sprintf("%v", twoDaysAgo.Unix()),
						Name:    fmt.Sprintf("%v", twoDaysAgo.Unix()),
						Started: float64(twoDaysAgo.Unix() * 1000),
						Hint:    timeMustText(twoDaysAgo.Truncate(time.Second)),
					},
					Cells: map[string]updater.Cell{},
				},
				{
					Column: &statepb.Column{
						Build:   fmt.Sprintf("%v", threeDaysAgo.Unix()),
						Name:    fmt.Sprintf("%v", threeDaysAgo.Unix()),
						Started: float64(threeDaysAgo.Unix() * 1000),
						Hint:    timeMustText(threeDaysAgo.Truncate(time.Second)),
					},
					Cells: map[string]updater.Cell{},
				},
			},
		},
		{
			name: "no results from query",
			client: &fakeClient{
				searches: map[string][]string{},
				invocations: map[string]fetchResult{
					invocationName("id-1"): {
						Invocation: &resultstore.Invocation{
							Id: &resultstore.Invocation_Id{
								InvocationId: "id-1",
							},
							Name: invocationName("id-1"),
							Timing: &resultstore.Timing{
								StartTime: &timestamppb.Timestamp{
									Seconds: oneDayAgo.Unix(),
								},
							},
						},
					},
					invocationName("id-2"): {
						Invocation: &resultstore.Invocation{
							Id: &resultstore.Invocation_Id{
								InvocationId: "id-2",
							},
							Name: invocationName("id-2"),
							Timing: &resultstore.Timing{
								StartTime: &timestamppb.Timestamp{
									Seconds: twoDaysAgo.Unix(),
								},
							},
						},
					},
					invocationName("id-3"): {
						Invocation: &resultstore.Invocation{
							Id: &resultstore.Invocation_Id{
								InvocationId: "id-3",
							},
							Name: invocationName("id-3"),
							Timing: &resultstore.Timing{
								StartTime: &timestamppb.Timestamp{
									Seconds: threeDaysAgo.Unix(),
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "no invocations found",
			client: &fakeClient{
				searches: map[string][]string{
					testQueryAfter: {"id-2", "id-3", "id-1"},
				},
				invocations: map[string]fetchResult{},
			},
			want: nil,
		},
		{
			name: "ids not in order",
			client: &fakeClient{
				searches: map[string][]string{
					testQueryAfter: {"id-2", "id-3", "id-1"},
				},
				invocations: map[string]fetchResult{
					invocationName("id-1"): {
						Invocation: &resultstore.Invocation{
							Id: &resultstore.Invocation_Id{
								InvocationId: "id-1",
							},
							Name: invocationName("id-1"),
							Timing: &resultstore.Timing{
								StartTime: &timestamppb.Timestamp{
									Seconds: oneDayAgo.Unix(),
								},
							},
						},
					},
					invocationName("id-2"): {
						Invocation: &resultstore.Invocation{
							Id: &resultstore.Invocation_Id{
								InvocationId: "id-2",
							},
							Name: invocationName("id-2"),
							Timing: &resultstore.Timing{
								StartTime: &timestamppb.Timestamp{
									Seconds: twoDaysAgo.Unix(),
								},
							},
						},
					},
					invocationName("id-3"): {
						Invocation: &resultstore.Invocation{
							Id: &resultstore.Invocation_Id{
								InvocationId: "id-3",
							},
							Name: invocationName("id-3"),
							Timing: &resultstore.Timing{
								StartTime: &timestamppb.Timestamp{
									Seconds: threeDaysAgo.Unix(),
								},
							},
						},
					},
				},
			},
			want: []updater.InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   fmt.Sprintf("%v", oneDayAgo.Unix()),
						Name:    fmt.Sprintf("%v", oneDayAgo.Unix()),
						Started: float64(oneDayAgo.Unix() * 1000),
						Hint:    timeMustText(oneDayAgo.Truncate(time.Second)),
					},
					Cells: map[string]updater.Cell{},
				},
				{
					Column: &statepb.Column{
						Build:   fmt.Sprintf("%v", twoDaysAgo.Unix()),
						Name:    fmt.Sprintf("%v", twoDaysAgo.Unix()),
						Started: float64(twoDaysAgo.Unix() * 1000),
						Hint:    timeMustText(twoDaysAgo.Truncate(time.Second)),
					},
					Cells: map[string]updater.Cell{},
				},
				{
					Column: &statepb.Column{
						Build:   fmt.Sprintf("%v", threeDaysAgo.Unix()),
						Name:    fmt.Sprintf("%v", threeDaysAgo.Unix()),
						Started: float64(threeDaysAgo.Unix() * 1000),
						Hint:    timeMustText(threeDaysAgo.Truncate(time.Second)),
					},
					Cells: map[string]updater.Cell{},
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var dlClient *downloadClient
			if tc.client != nil {
				dlClient = &downloadClient{client: tc.client}
			}
			columnReader := ResultStoreColumnReader(dlClient, 0)
			var got []updater.InflatedColumn
			ch := make(chan updater.InflatedColumn)
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				for col := range ch {
					got = append(got, col)
				}
			}()
			err := columnReader(context.Background(), logrus.WithField("case", tc.name), oneMonthConfig, nil, oneMonthAgo, ch)
			close(ch)
			wg.Wait()
			if err != nil && !tc.wantErr {
				t.Errorf("columnReader() errored: %v", err)
			} else if err == nil && tc.wantErr {
				t.Errorf("columnReader() did not error as expected")
			}
			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("columnReader() differed (-want, +got): %s", diff)
			}
		})
	}
}

func TestTimestampMilliseconds(t *testing.T) {
	cases := []struct {
		name      string
		timestamp *timestamppb.Timestamp
		want      float64
	}{
		{
			name:      "nil",
			timestamp: nil,
			want:      0,
		},
		{
			name:      "zero",
			timestamp: &timestamppb.Timestamp{},
			want:      0,
		},
		{
			name: "basic",
			timestamp: &timestamppb.Timestamp{
				Seconds: 1234,
				Nanos:   5678,
			},
			want: 1234005.678,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := timestampMilliseconds(tc.timestamp)
			approx := cmpopts.EquateApprox(.01, 0)
			if diff := cmp.Diff(tc.want, got, approx); diff != "" {
				t.Errorf("timestampMilliseconds(%v) differed (-want, +got): %s", tc.timestamp, diff)
			}
		})
	}
}

func TestProcessGroup(t *testing.T) {
	// Invocation        *resultstore.Invocation
	// Actions           []*resultstore.Action
	// ConfiguredTargets []*resultstore.ConfiguredTarget
	// Targets           []*resultstore.Target
	cases := []struct {
		name   string
		result *fetchResult
		want   updater.InflatedColumn
	}{
		{
			name: "nil",
			want: updater.InflatedColumn{},
		},
		{
			name:   "empty",
			result: &fetchResult{},
			want: updater.InflatedColumn{
				Column: &statepb.Column{
					Name:  "0",
					Build: "0",
					Hint:  "1970-01-01T00:00:00Z",
				},
				Cells: map[string]updater.Cell{},
			},
		},
		{
			name: "basic",
			result: &fetchResult{
				Invocation: &resultstore.Invocation{
					Id: &resultstore.Invocation_Id{
						InvocationId: "id-1",
					},
					Name: invocationName("id-1"),
					Timing: &resultstore.Timing{
						StartTime: &timestamppb.Timestamp{
							Seconds: 1234,
						},
					},
				},
				Targets: []*resultstore.Target{
					{
						Name: targetName("id-1", "target-pass"),
						Id: &resultstore.Target_Id{
							TargetId: "target-pass",
						},
						StatusAttributes: &resultstore.StatusAttributes{
							Status: resultstore.Status_PASSED,
						},
					},
					{
						Name: targetName("id-1", "target-fail"),
						Id: &resultstore.Target_Id{
							TargetId: "target-fail",
						},
						StatusAttributes: &resultstore.StatusAttributes{
							Status: resultstore.Status_FAILED,
						},
					},
				},
			},
			want: updater.InflatedColumn{
				Column: &statepb.Column{
					Name:    "1234",
					Build:   "1234",
					Started: 1234000,
					Hint:    "1970-01-01T00:20:34Z",
				},
				Cells: map[string]updater.Cell{
					"target-pass": {
						ID:     "target-pass",
						CellID: "id-1",
						Result: test_status.TestStatus_PASS,
					},
					"target-fail": {
						ID:     "target-fail",
						CellID: "id-1",
						Result: test_status.TestStatus_FAIL,
					},
				},
			},
		},
		{
			name: "invocation without targets",
			result: &fetchResult{
				Invocation: &resultstore.Invocation{
					Id: &resultstore.Invocation_Id{
						InvocationId: "id-1",
					},
					Name: invocationName("id-1"),
					Timing: &resultstore.Timing{
						StartTime: &timestamppb.Timestamp{
							Seconds: 1234,
						},
					},
				},
			},
			want: updater.InflatedColumn{
				Column: &statepb.Column{
					Name:    "1234",
					Build:   "1234",
					Started: 1234000,
					Hint:    "1970-01-01T00:20:34Z",
				},
				Cells: map[string]updater.Cell{},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := processGroup(tc.result)
			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("processGroup() differed (-want, +got): %s", diff)
			}
		})
	}
}

func TestQueryAfter(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name  string
		query string
		when  time.Time
		want  string
	}{
		{
			name: "empty",
			want: "",
		},
		{
			name:  "zero",
			query: testQuery,
			when:  time.Time{},
			want:  "invocation_attributes.labels:\"prow\" timing.start_time>=\"0001-01-01T00:00:00Z\"",
		},
		{
			name:  "basic",
			query: testQuery,
			when:  now,
			want:  fmt.Sprintf("invocation_attributes.labels:\"prow\" timing.start_time>=\"%s\"", now.UTC().Format(time.RFC3339)),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := queryAfter(tc.query, tc.when)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("queryAfter(%q, %v) differed (-want, +got): %s", tc.query, tc.when, diff)
			}
		})
	}
}

func TestSearch(t *testing.T) {
	twoDaysAgo := time.Now().AddDate(0, 0, -2)
	testQueryAfter := queryAfter(testQuery, twoDaysAgo)
	cases := []struct {
		name    string
		stop    time.Time
		client  *fakeClient
		want    []string
		wantErr bool
	}{
		{
			name:    "nil",
			wantErr: true,
		},
		{
			name:    "empty",
			client:  &fakeClient{},
			wantErr: true,
		},
		{
			name: "basic",
			client: &fakeClient{
				searches: map[string][]string{
					testQueryAfter: {"id-1", "id-2", "id-3"},
				},
			},
			stop: twoDaysAgo,
			want: []string{"id-1", "id-2", "id-3"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var dlClient *downloadClient
			if tc.client != nil {
				dlClient = &downloadClient{client: tc.client}
			}
			got, err := search(context.Background(), logrus.WithField("case", tc.name), dlClient, tc.stop)
			if err != nil && !tc.wantErr {
				t.Errorf("search() errored: %v", err)
			} else if err == nil && tc.wantErr {
				t.Errorf("search() did not error as expected")
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("search() differed (-want, +got): %s", diff)
			}
		})
	}
}

func TestMostRecent(t *testing.T) {
	now := time.Now()
	oneHourAgo := now.Add(-1 * time.Hour)
	sixHoursAgo := now.Add(-6 * time.Hour)
	cases := []struct {
		name  string
		times []time.Time
		want  time.Time
	}{
		{
			name: "empty",
			want: time.Time{},
		},
		{
			name:  "single",
			times: []time.Time{oneHourAgo},
			want:  oneHourAgo,
		},
		{
			name:  "mix",
			times: []time.Time{now, oneHourAgo, sixHoursAgo},
			want:  now,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mostRecent(tc.times)
			if !tc.want.Equal(got) {
				t.Errorf("stopFromColumns() differed; got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStopFromColumns(t *testing.T) {
	now := time.Now()
	oneHourAgo := now.Add(-1 * time.Hour)
	sixHoursAgo := now.Add(-6 * time.Hour)
	b, _ := oneHourAgo.MarshalText()
	oneHourHint := string(b)
	cases := []struct {
		name string
		cols []updater.InflatedColumn
		want time.Time
	}{
		{
			name: "empty",
			want: time.Time{},
		},
		{
			name: "column start",
			cols: []updater.InflatedColumn{
				{
					Column: &statepb.Column{
						Started: float64(oneHourAgo.Unix() * 1000),
					},
				},
			},
			want: oneHourAgo.Truncate(time.Second),
		},
		{
			name: "column hint",
			cols: []updater.InflatedColumn{
				{
					Column: &statepb.Column{
						Started: float64(sixHoursAgo.Unix() * 1000),
						Hint:    oneHourHint,
					},
				},
			},
			want: oneHourAgo.Truncate(time.Second),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stopFromColumns(logrus.WithField("case", tc.name), tc.cols)
			if !tc.want.Equal(got) {
				t.Errorf("stopFromColumns() differed; got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUpdateStop(t *testing.T) {
	now := time.Now()
	oneHourAgo := now.Add(-1 * time.Hour)
	sixHoursAgo := now.Add(-6 * time.Hour)
	twoDaysAgo := now.AddDate(0, 0, -2)
	twoWeeksAgo := now.AddDate(0, 0, -14)
	oneMonthAgo := now.AddDate(0, 0, -30)
	b, _ := oneHourAgo.MarshalText()
	oneHourHint := string(b)
	cases := []struct {
		name        string
		tg          *configpb.TestGroup
		cols        []updater.InflatedColumn
		defaultStop time.Time
		reprocess   time.Duration
		want        time.Time
	}{
		{
			name: "empty",
			want: twoDaysAgo.Truncate(time.Second),
		},
		{
			name:      "reprocess",
			reprocess: 14 * 24 * time.Hour,
			want:      twoWeeksAgo.Truncate(time.Second),
		},
		{
			name: "days of results",
			tg: &configpb.TestGroup{
				DaysOfResults: 7,
			},
			want: twoWeeksAgo.Truncate(time.Second),
		},
		{
			name:        "default stop, no days of results",
			defaultStop: oneMonthAgo,
			want:        twoDaysAgo.Truncate(time.Second),
		},
		{
			name: "default stop earlier than days of results",
			tg: &configpb.TestGroup{
				DaysOfResults: 7,
			},
			defaultStop: oneMonthAgo,
			want:        twoWeeksAgo.Truncate(time.Second),
		},
		{
			name: "default stop later than days of results",
			tg: &configpb.TestGroup{
				DaysOfResults: 30,
			},
			defaultStop: twoWeeksAgo,
			want:        twoWeeksAgo.Truncate(time.Second),
		},
		{
			name: "column start",
			cols: []updater.InflatedColumn{
				{
					Column: &statepb.Column{
						Started: float64(oneHourAgo.Unix() * 1000),
					},
				},
			},
			defaultStop: twoWeeksAgo,
			want:        oneHourAgo.Truncate(time.Second),
		},
		{
			name: "column hint",
			cols: []updater.InflatedColumn{
				{
					Column: &statepb.Column{
						Started: float64(sixHoursAgo.Unix() * 1000),
						Hint:    oneHourHint,
					},
				},
			},
			defaultStop: twoWeeksAgo,
			want:        oneHourAgo.Truncate(time.Second),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := updateStop(logrus.WithField("testcase", tc.name), tc.tg, now, tc.cols, tc.defaultStop, tc.reprocess)
			if !tc.want.Equal(got) {
				t.Errorf("updateStop() differed; got %v, want %v", got, tc.want)
			}
		})
	}
}
