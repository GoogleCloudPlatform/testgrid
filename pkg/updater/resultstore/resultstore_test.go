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
	testQueryAfter := queryAfter(queryProw, oneMonthAgo)
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
						Build:   "id-1",
						Name:    "id-1",
						Started: float64(oneDayAgo.Unix() * 1000),
						Hint:    timeMustText(oneDayAgo.Truncate(time.Second)),
					},
					Cells: map[string]updater.Cell{
						"Overall": {ID: "Overall", CellID: "id-1"},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "id-2",
						Name:    "id-2",
						Started: float64(twoDaysAgo.Unix() * 1000),
						Hint:    timeMustText(twoDaysAgo.Truncate(time.Second)),
					},
					Cells: map[string]updater.Cell{
						"Overall": {ID: "Overall", CellID: "id-2"},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "id-3",
						Name:    "id-3",
						Started: float64(threeDaysAgo.Unix() * 1000),
						Hint:    timeMustText(threeDaysAgo.Truncate(time.Second)),
					},
					Cells: map[string]updater.Cell{
						"Overall": {ID: "Overall", CellID: "id-3"},
					},
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
						Build:   "id-1",
						Name:    "id-1",
						Started: float64(oneDayAgo.Unix() * 1000),
						Hint:    timeMustText(oneDayAgo.Truncate(time.Second)),
					},
					Cells: map[string]updater.Cell{
						"Overall": {ID: "Overall", CellID: "id-1"},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "id-2",
						Name:    "id-2",
						Started: float64(twoDaysAgo.Unix() * 1000),
						Hint:    timeMustText(twoDaysAgo.Truncate(time.Second)),
					},
					Cells: map[string]updater.Cell{
						"Overall": {ID: "Overall", CellID: "id-2"},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "id-3",
						Name:    "id-3",
						Started: float64(threeDaysAgo.Unix() * 1000),
						Hint:    timeMustText(threeDaysAgo.Truncate(time.Second)),
					},
					Cells: map[string]updater.Cell{
						"Overall": {ID: "Overall", CellID: "id-3"},
					},
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var dlClient *DownloadClient
			if tc.client != nil {
				dlClient = &DownloadClient{client: tc.client}
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

func TestProcessRawResult(t *testing.T) {
	cases := []struct {
		name   string
		result *fetchResult
		want   *processedResult
	}{
		{
			name: "just invocation",
			result: &fetchResult{
				Invocation: &resultstore.Invocation{
					Name: invocationName("Best invocation"),
					Id: &resultstore.Invocation_Id{
						InvocationId: "uuid-222",
					},
				},
			},
			want: &processedResult{
				InvocationProto: &resultstore.Invocation{
					Name: invocationName("Best invocation"),
					Id: &resultstore.Invocation_Id{
						InvocationId: "uuid-222",
					},
				},
				TargetResults: make(map[string][]*singleActionResult),
			},
		},
		{
			name: "invocation + targets + configured targets",
			result: &fetchResult{
				Invocation: &resultstore.Invocation{
					Name: invocationName("Best invocation"),
					Id: &resultstore.Invocation_Id{
						InvocationId: "uuid-222",
					},
				},
				Targets: []*resultstore.Target{
					{
						Name: targetName("updater", "uuid-222"),
						Id: &resultstore.Target_Id{
							InvocationId: "uuid-222",
							TargetId:     "tgt-uuid-1",
						},
					},
					{
						Name: targetName("tabulator", "uuid-222"),
						Id: &resultstore.Target_Id{
							InvocationId: "uuid-222",
							TargetId:     "tgt-uuid-2",
						},
					},
				},
				ConfiguredTargets: []*resultstore.ConfiguredTarget{
					{
						Name: targetName("updater", "uuid-222"),
						Id: &resultstore.ConfiguredTarget_Id{
							InvocationId: "uuid-222",
							TargetId:     "tgt-uuid-1",
						},
					},
					{
						Name: targetName("tabulator", "uuid-222"),
						Id: &resultstore.ConfiguredTarget_Id{
							InvocationId: "uuid-222",
							TargetId:     "tgt-uuid-2",
						},
					},
				},
			},
			want: &processedResult{
				InvocationProto: &resultstore.Invocation{
					Name: invocationName("Best invocation"),
					Id: &resultstore.Invocation_Id{
						InvocationId: "uuid-222",
					},
				},
				TargetResults: map[string][]*singleActionResult{
					"tgt-uuid-1": {
						{
							TargetProto: &resultstore.Target{
								Name: targetName("updater", "uuid-222"),
								Id: &resultstore.Target_Id{
									InvocationId: "uuid-222",
									TargetId:     "tgt-uuid-1",
								},
							},
							ConfiguredTargetProto: &resultstore.ConfiguredTarget{
								Name: targetName("updater", "uuid-222"),
								Id: &resultstore.ConfiguredTarget_Id{
									InvocationId: "uuid-222",
									TargetId:     "tgt-uuid-1",
								},
							},
						},
					},
					"tgt-uuid-2": {
						{
							TargetProto: &resultstore.Target{
								Name: targetName("tabulator", "uuid-222"),
								Id: &resultstore.Target_Id{
									InvocationId: "uuid-222",
									TargetId:     "tgt-uuid-2",
								},
							},
							ConfiguredTargetProto: &resultstore.ConfiguredTarget{
								Name: targetName("tabulator", "uuid-222"),
								Id: &resultstore.ConfiguredTarget_Id{
									InvocationId: "uuid-222",
									TargetId:     "tgt-uuid-2",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "all together + extra actions",
			result: &fetchResult{
				Invocation: &resultstore.Invocation{
					Name: invocationName("Best invocation"),
					Id: &resultstore.Invocation_Id{
						InvocationId: "uuid-222",
					},
				},
				Targets: []*resultstore.Target{
					{
						Name: "/testgrid/backend:updater",
						Id: &resultstore.Target_Id{
							InvocationId: "uuid-222",
							TargetId:     "tgt-uuid-1",
						},
					},
					{
						Name: "/testgrid/backend:tabulator",
						Id: &resultstore.Target_Id{
							InvocationId: "uuid-222",
							TargetId:     "tgt-uuid-2",
						},
					},
				},
				ConfiguredTargets: []*resultstore.ConfiguredTarget{
					{
						Name: "/testgrid/backend:updater",
						Id: &resultstore.ConfiguredTarget_Id{
							InvocationId: "uuid-222",
							TargetId:     "tgt-uuid-1",
						},
					},
					{
						Name: "/testgrid/backend:tabulator",
						Id: &resultstore.ConfiguredTarget_Id{
							InvocationId: "uuid-222",
							TargetId:     "tgt-uuid-2",
						},
					},
				},
				Actions: []*resultstore.Action{
					{
						Name: "/testgrid/backend:updater",
						Id: &resultstore.Action_Id{
							InvocationId: "uuid-222",
							TargetId:     "tgt-uuid-1",
							ActionId:     "flying",
						},
					},
					{
						Name: "/testgrid/backend:tabulator",
						Id: &resultstore.Action_Id{
							InvocationId: "uuid-222",
							TargetId:     "tgt-uuid-2",
							ActionId:     "walking",
						},
					},
					{
						Name: "/testgrid/backend:tabulator",
						Id: &resultstore.Action_Id{
							InvocationId: "uuid-222",
							TargetId:     "tgt-uuid-2",
							ActionId:     "flying",
						},
					},
				},
			},
			want: &processedResult{
				InvocationProto: &resultstore.Invocation{
					Name: invocationName("Best invocation"),
					Id: &resultstore.Invocation_Id{
						InvocationId: "uuid-222",
					},
				},
				TargetResults: map[string][]*singleActionResult{
					"tgt-uuid-1": {
						{
							TargetProto: &resultstore.Target{
								Name: "/testgrid/backend:updater",
								Id: &resultstore.Target_Id{
									InvocationId: "uuid-222",
									TargetId:     "tgt-uuid-1",
								},
							},
							ConfiguredTargetProto: &resultstore.ConfiguredTarget{
								Name: "/testgrid/backend:updater",
								Id: &resultstore.ConfiguredTarget_Id{
									InvocationId: "uuid-222",
									TargetId:     "tgt-uuid-1",
								},
							},
							ActionProto: &resultstore.Action{
								Name: "/testgrid/backend:updater",
								Id: &resultstore.Action_Id{
									InvocationId: "uuid-222",
									TargetId:     "tgt-uuid-1",
									ActionId:     "flying",
								},
							},
						},
					},
					"tgt-uuid-2": {
						{
							TargetProto: &resultstore.Target{
								Name: "/testgrid/backend:tabulator",
								Id: &resultstore.Target_Id{
									InvocationId: "uuid-222",
									TargetId:     "tgt-uuid-2",
								},
							},
							ConfiguredTargetProto: &resultstore.ConfiguredTarget{
								Name: "/testgrid/backend:tabulator",
								Id: &resultstore.ConfiguredTarget_Id{
									InvocationId: "uuid-222",
									TargetId:     "tgt-uuid-2",
								},
							},
							ActionProto: &resultstore.Action{
								Name: "/testgrid/backend:tabulator",
								Id: &resultstore.Action_Id{
									InvocationId: "uuid-222",
									TargetId:     "tgt-uuid-2",
									ActionId:     "walking",
								},
							},
						}, {
							TargetProto: &resultstore.Target{
								Name: "/testgrid/backend:tabulator",
								Id: &resultstore.Target_Id{
									InvocationId: "uuid-222",
									TargetId:     "tgt-uuid-2",
								},
							},
							ConfiguredTargetProto: &resultstore.ConfiguredTarget{
								Name: "/testgrid/backend:tabulator",
								Id: &resultstore.ConfiguredTarget_Id{
									InvocationId: "uuid-222",
									TargetId:     "tgt-uuid-2",
								},
							},
							ActionProto: &resultstore.Action{
								Name: "/testgrid/backend:tabulator",
								Id: &resultstore.Action_Id{
									InvocationId: "uuid-222",
									TargetId:     "tgt-uuid-2",
									ActionId:     "flying",
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := processRawResult(logrus.WithField("case", tc.name), tc.result)
			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("processRawResult(...) differed (-want, +got): %s", diff)
			}
		})
	}

}
func TestProcessGroup(t *testing.T) {

	cases := []struct {
		name   string
		result *processedResult
		want   *updater.InflatedColumn
	}{
		{
			name: "nil",
			want: nil,
		},
		{
			name:   "empty",
			result: &processedResult{},
			want:   nil,
		},
		{
			name: "basic",
			result: &processedResult{
				InvocationProto: &resultstore.Invocation{
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
				TargetResults: map[string][]*singleActionResult{
					"tgt-id-1": {
						{
							TargetProto: &resultstore.Target{
								Name: targetName("tgt-id-1", "id-1"),
								Id: &resultstore.Target_Id{
									TargetId: "tgt-id-1",
								},
								StatusAttributes: &resultstore.StatusAttributes{
									Status: resultstore.Status_PASSED,
								},
							},
						},
					},
					"tgt-id-2": {
						{
							TargetProto: &resultstore.Target{
								Name: targetName("tgt-id-2", "id-1"),
								Id: &resultstore.Target_Id{
									TargetId: "tgt-id-2",
								},
								StatusAttributes: &resultstore.StatusAttributes{
									Status: resultstore.Status_FAILED,
								},
							},
						},
					},
				},
			},
			want: &updater.InflatedColumn{
				Column: &statepb.Column{
					Name:    "id-1",
					Build:   "id-1",
					Started: 1234000,
					Hint:    "1970-01-01T00:20:34Z",
				},
				Cells: map[string]updater.Cell{
					"Overall": {
						ID:     "Overall",
						CellID: "id-1",
					},
					"tgt-id-1": {
						ID:     "tgt-id-1",
						CellID: "id-1",
						Result: test_status.TestStatus_PASS,
					},
					"tgt-id-2": {
						ID:     "tgt-id-2",
						CellID: "id-1",
						Result: test_status.TestStatus_FAIL,
					},
				},
			},
		},
		{
			name: "invocation without targets",
			result: &processedResult{
				InvocationProto: &resultstore.Invocation{
					Id: &resultstore.Invocation_Id{
						InvocationId: "id-1",
					},
					Name: invocationName("id-1"),
					Timing: &resultstore.Timing{
						StartTime: &timestamppb.Timestamp{
							Seconds: 1234,
						},
					},
					StatusAttributes: &resultstore.StatusAttributes{
						Status: resultstore.Status_PASSED,
					},
				},
			},
			want: &updater.InflatedColumn{
				Column: &statepb.Column{
					Name:    "id-1",
					Build:   "id-1",
					Started: 1234000,
					Hint:    "1970-01-01T00:20:34Z",
				},
				Cells: map[string]updater.Cell{
					"Overall": {
						ID:     "Overall",
						CellID: "id-1",
						Result: test_status.TestStatus_PASS,
					},
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := processGroup(nil, tc.result)
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
			query: queryProw,
			when:  time.Time{},
			want:  "invocation_attributes.labels:\"prow\" timing.start_time>=\"0001-01-01T00:00:00Z\"",
		},
		{
			name:  "basic",
			query: queryProw,
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
	testQueryAfter := queryAfter(queryProw, twoDaysAgo)
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
			var dlClient *DownloadClient
			if tc.client != nil {
				dlClient = &DownloadClient{client: tc.client}
			}
			got, err := search(context.Background(), logrus.WithField("case", tc.name), dlClient, "my-project", tc.stop)
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

func TestIdentifyBuild(t *testing.T) {
	cases := []struct {
		name   string
		result *processedResult
		tg     *configpb.TestGroup
		want   string
	}{
		{
			name: "no override configurations",
			result: &processedResult{
				InvocationProto: &resultstore.Invocation{
					Name: invocationName("id-123"),
					Id: &resultstore.Invocation_Id{
						InvocationId: "id-123",
					},
				},
			},
			want: "",
		},
		{
			name: "override by non-existent property key",
			result: &processedResult{
				InvocationProto: &resultstore.Invocation{
					Name: invocationName("id-1234"),
					Id: &resultstore.Invocation_Id{
						InvocationId: "id-1234",
					},
					Properties: []*resultstore.Property{
						{Key: "Luigi", Value: "Peaches"},
						{Key: "Bowser", Value: "Pingui"},
					},
				},
			},
			tg: &configpb.TestGroup{
				BuildOverrideConfigurationValue: "Mario",
			},
			want: "",
		},
		{
			name: "override by existent property key",
			result: &processedResult{
				InvocationProto: &resultstore.Invocation{
					Name: invocationName("id-1234"),
					Id: &resultstore.Invocation_Id{
						InvocationId: "id-1234",
					},
					Properties: []*resultstore.Property{
						{Key: "Luigi", Value: "Peaches"},
						{Key: "Bowser", Value: "Pingui"},
						{Key: "Waluigi", Value: "Wapeaches"},
					},
				},
			},
			tg: &configpb.TestGroup{
				BuildOverrideConfigurationValue: "Waluigi",
			},
			want: "Wapeaches",
		},
		{
			name: "override by build time strf",
			result: &processedResult{
				InvocationProto: &resultstore.Invocation{
					Name: invocationName("id-1234"),
					Id: &resultstore.Invocation_Id{
						InvocationId: "id-1234",
					},
					Timing: &resultstore.Timing{
						StartTime: &timestamppb.Timestamp{
							Seconds: 1689881216,
							Nanos:   27847,
						},
					},
				},
			},
			tg: &configpb.TestGroup{
				BuildOverrideStrftime: "%Y-%m-%d-%H",
			},
			want: "2023-07-20-19",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := identifyBuild(tc.tg, tc.result)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("queryAfter(...) differed (-want, +got): %s", diff)
			}
		})
	}
}

func TestExtractGroupID(t *testing.T) {
	cases := []struct {
		name string
		tg   *configpb.TestGroup
		pr   *processedResult
		want string
	}{
		{
			name: "primary grouping by build - override configuration value",
			tg: &configpb.TestGroup{
				BuildOverrideConfigurationValue: "test-key-1",
				PrimaryGrouping:                 configpb.TestGroup_PRIMARY_GROUPING_BUILD,
			},
			pr: &processedResult{
				InvocationProto: &resultstore.Invocation{
					Id: &resultstore.Invocation_Id{
						InvocationId: "id-1",
					},
					Properties: []*resultstore.Property{
						{
							Key:   "test-key-1",
							Value: "test-val-1",
						},
					},
					Name: invocationName("id-1"),
					Timing: &resultstore.Timing{
						StartTime: &timestamppb.Timestamp{
							Seconds: 1234,
						},
					},
				},
			},
			want: "test-val-1",
		},
		{
			name: "fallback grouping - defaults to invocationID",
			tg: &configpb.TestGroup{
				FallbackGrouping: configpb.TestGroup_FALLBACK_GROUPING_DATE,
			},
			pr: &processedResult{
				InvocationProto: &resultstore.Invocation{
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
			want: "id-1",
		},
		{
			name: "no grouping specified - defaults to invocationID",
			tg: &configpb.TestGroup{
				FallbackGrouping: configpb.TestGroup_FALLBACK_GROUPING_DATE,
			},
			pr: &processedResult{
				InvocationProto: &resultstore.Invocation{
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
			want: "id-1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractGroupID(tc.tg, tc.pr)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("extractGroupID() differed (-want, +got): %s", diff)
			}
		})
	}
}
