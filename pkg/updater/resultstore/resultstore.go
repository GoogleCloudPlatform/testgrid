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

// resultstore fetches and process results from ResultStore.
package resultstore

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/pkg/updater"
	"github.com/sirupsen/logrus"

	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	statuspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	timestamppb "github.com/golang/protobuf/ptypes/timestamp"
	resultstorepb "google.golang.org/genproto/googleapis/devtools/resultstore/v2"
)

// ResultStoreColumnReader fetches results since last update from ResultStore and translates them into columns.
func ResultStoreColumnReader(client *downloadClient, reprocess time.Duration) updater.ColumnReader {
	return func(ctx context.Context, log logrus.FieldLogger, tg *configpb.TestGroup, oldCols []updater.InflatedColumn, defaultStop time.Time, receivers chan<- updater.InflatedColumn) error {
		stop := updateStop(log, tg, time.Now(), oldCols, defaultStop, reprocess)
		ids, err := search(ctx, log, client, stop)
		if err != nil {
			return fmt.Errorf("error searching invocations: %v", err)
		}
		invocationErrors := make(map[string]error)
		var results []*fetchResult
		for _, id := range ids {
			result, invErr := client.FetchInvocation(ctx, log, id)
			if invErr != nil {
				invocationErrors[id] = invErr
				continue
			}
			results = append(results, result)
		}

		// Reverse-sort invocations by start time.
		sort.SliceStable(results, func(i, j int) bool {
			return results[i].Invocation.GetTiming().GetStartTime().GetSeconds() > results[j].Invocation.GetTiming().GetStartTime().GetSeconds()
		})
		for _, result := range results {
			// TODO: Make group ID something other than start time.
			inflatedCol := processGroup(result)
			receivers <- inflatedCol
		}
		return nil
	}
}

func timestampMilliseconds(t *timestamppb.Timestamp) float64 {
	return float64(t.GetSeconds())*1000.0 + float64(t.GetNanos())/1000.0
}

var convertStatus = map[resultstorepb.Status]statuspb.TestStatus{
	resultstorepb.Status_STATUS_UNSPECIFIED: statuspb.TestStatus_NO_RESULT,
	resultstorepb.Status_BUILDING:           statuspb.TestStatus_RUNNING,
	resultstorepb.Status_BUILT:              statuspb.TestStatus_BUILD_PASSED,
	resultstorepb.Status_FAILED_TO_BUILD:    statuspb.TestStatus_BUILD_FAIL,
	resultstorepb.Status_TESTING:            statuspb.TestStatus_RUNNING,
	resultstorepb.Status_PASSED:             statuspb.TestStatus_PASS,
	resultstorepb.Status_FAILED:             statuspb.TestStatus_FAIL,
	resultstorepb.Status_TIMED_OUT:          statuspb.TestStatus_TIMED_OUT,
	resultstorepb.Status_CANCELLED:          statuspb.TestStatus_CANCEL,
	resultstorepb.Status_TOOL_FAILED:        statuspb.TestStatus_TOOL_FAIL,
	resultstorepb.Status_INCOMPLETE:         statuspb.TestStatus_UNKNOWN,
	resultstorepb.Status_FLAKY:              statuspb.TestStatus_FLAKY,
	resultstorepb.Status_UNKNOWN:            statuspb.TestStatus_UNKNOWN,
	resultstorepb.Status_SKIPPED:            statuspb.TestStatus_PASS_WITH_SKIPS,
}

func processGroup(result *fetchResult) updater.InflatedColumn {
	if result == nil {
		return updater.InflatedColumn{}
	}
	started := result.Invocation.GetTiming().GetStartTime()
	groupID := fmt.Sprintf("%d", started.GetSeconds())
	hint, err := started.AsTime().MarshalText()
	if err != nil {
		hint = []byte{}
	}
	col := &statepb.Column{
		Build:   groupID,
		Name:    groupID,
		Started: timestampMilliseconds(started),
		Hint:    string(hint),
	}
	cells := make(map[string]updater.Cell)
	for _, target := range result.Targets {
		status, ok := convertStatus[target.GetStatusAttributes().GetStatus()]
		if !ok {
			status = statuspb.TestStatus_UNKNOWN
		}
		cells[target.GetId().GetTargetId()] = updater.Cell{
			Result: status,
			ID:     target.GetId().GetTargetId(),
			CellID: result.Invocation.GetId().GetInvocationId(),
		}
	}
	return updater.InflatedColumn{
		Column: col,
		Cells:  cells,
	}
}

func queryAfter(query string, when time.Time) string {
	if query == "" {
		return ""
	}
	return fmt.Sprintf("%s timing.start_time>=\"%s\"", query, when.UTC().Format(time.RFC3339))
}

// TODO: Replace these hardcoded values with adjustable ones.
const (
	testProjectID = "k8s-testgrid"
	testQuery     = "invocation_attributes.labels:\"prow\""
	testToken     = "fake-token-1234"
)

func search(ctx context.Context, log logrus.FieldLogger, client *downloadClient, stop time.Time) ([]string, error) {
	if client == nil {
		return nil, fmt.Errorf("no ResultStore client provided")
	}
	query := queryAfter(testQuery, stop)
	log.WithField("query", query).Debug("Searching ResultStore.")
	// Quit if search goes over 5 minutes.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	ids, err := client.Search(ctx, log, query, testProjectID)
	log.WithField("ids", len(ids)).WithError(err).Debug("Searched ResultStore.")
	return ids, err
}

func mostRecent(times []time.Time) time.Time {
	var max time.Time
	for _, t := range times {
		if t.After(max) {
			max = t
		}
	}
	return max
}

func stopFromColumns(log logrus.FieldLogger, cols []updater.InflatedColumn) time.Time {
	var stop time.Time
	for _, col := range cols {
		log = log.WithField("start", col.Column.Started).WithField("hint", col.Column.Hint)
		startedMillis := col.Column.Started
		if startedMillis == 0 {
			continue
		}
		started := time.Unix(int64(startedMillis/1000), 0)

		var hint time.Time
		if err := hint.UnmarshalText([]byte(col.Column.Hint)); col.Column.Hint != "" && err != nil {
			log.WithError(err).Warning("Could not parse hint, ignoring.")
		}
		stop = mostRecent([]time.Time{started, hint, stop})
	}
	return stop.Truncate(time.Second) // We don't need sub-second resolution.
}

// updateStop returns the time to stop searching after, given previous columns and a default.
func updateStop(log logrus.FieldLogger, tg *configpb.TestGroup, now time.Time, oldCols []updater.InflatedColumn, defaultStop time.Time, reprocess time.Duration) time.Time {
	hint := stopFromColumns(log, oldCols)
	// Process at most twice days_of_results.
	days := tg.GetDaysOfResults()
	if days == 0 {
		days = 1
	}
	max := now.AddDate(0, 0, -2*int(days))

	stop := mostRecent([]time.Time{hint, defaultStop, max})

	// Process at least the reprocess threshold.
	if reprocessTime := now.Add(-1 * reprocess); stop.After(reprocessTime) {
		stop = reprocessTime
	}

	// Primary grouping can sometimes miss recent results, mitigate by extending the stop.
	if tg.GetPrimaryGrouping() == configpb.TestGroup_PRIMARY_GROUPING_BUILD {
		stop.Add(-30 * time.Minute)
	}

	return stop.Truncate(time.Second) // We don't need sub-second resolution.
}
