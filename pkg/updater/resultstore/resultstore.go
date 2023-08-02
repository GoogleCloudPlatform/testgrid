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
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/sirupsen/logrus"

	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	statuspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	timestamppb "github.com/golang/protobuf/ptypes/timestamp"
	resultstorepb "google.golang.org/genproto/googleapis/devtools/resultstore/v2"
)

// Updater returns a ResultStore-based GroupUpdater, which knows how to process result data stored in ResultStore.
func Updater(resultStoreClient *DownloadClient, gcsClient gcs.Client, groupTimeout time.Duration, write bool) updater.GroupUpdater {
	return func(parent context.Context, log logrus.FieldLogger, client gcs.Client, tg *configpb.TestGroup, gridPath gcs.Path) (bool, error) {
		if !tg.UseKubernetesClient && (tg.ResultSource == nil || tg.ResultSource.GetGcsConfig() == nil) {
			log.Debug("Skipping non-kubernetes client group")
			return false, nil
		}
		if resultStoreClient == nil {
			log.WithField("name", tg.GetName()).Warn("ResultStore update requested, but no client found.")
			return false, nil
		}
		ctx, cancel := context.WithTimeout(parent, groupTimeout)
		defer cancel()
		rsColumnReader := ResultStoreColumnReader(resultStoreClient, 0)
		reprocess := 20 * time.Minute // allow 20m for prow to finish uploading artifacts
		return updater.InflateDropAppend(ctx, log, gcsClient, tg, gridPath, write, rsColumnReader, reprocess)
	}
}

type singleActionResult struct {
	TargetProto           *resultstorepb.Target
	ConfiguredTargetProto *resultstorepb.ConfiguredTarget
	ActionProto           *resultstorepb.Action
}

type multiActionResult struct {
	TargetProto           *resultstorepb.Target
	ConfiguredTargetProto *resultstorepb.ConfiguredTarget
	ActionProtos          []*resultstorepb.Action
}

// invocation is an internal invocation representation which contains
// actual invocation data and results for each target
type invocation struct {
	InvocationProto *resultstorepb.Invocation
	TargetResults   map[string][]*singleActionResult
}

// extractGroupID extracts grouping ID for a results based on the testgroup grouping configuration
// Returns an empty string for no config or incorrect config
func extractGroupID(tg *configpb.TestGroup, inv *invocation) string {
	switch {
	// P - build info
	case inv == nil:
		return ""
	case tg.GetPrimaryGrouping() == configpb.TestGroup_PRIMARY_GROUPING_BUILD:
		return identifyBuild(tg, inv)
	default:
		return inv.InvocationProto.GetId().GetInvocationId()
	}
}

// ResultStoreColumnReader fetches results since last update from ResultStore and translates them into columns.
func ResultStoreColumnReader(client *DownloadClient, reprocess time.Duration) updater.ColumnReader {
	return func(ctx context.Context, log logrus.FieldLogger, tg *configpb.TestGroup, oldCols []updater.InflatedColumn, defaultStop time.Time, receivers chan<- updater.InflatedColumn) error {
		stop := updateStop(log, tg, time.Now(), oldCols, defaultStop, reprocess)
		ids, err := search(ctx, log, client, tg.GetResultSource().GetResultstoreConfig().GetProject(), stop)
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

		invocations := processRawResults(log, results)

		// Reverse-sort invocations by start time.
		sort.SliceStable(invocations, func(i, j int) bool {
			return invocations[i].InvocationProto.GetTiming().GetStartTime().GetSeconds() > invocations[j].InvocationProto.GetTiming().GetStartTime().GetSeconds()
		})

		for _, inv := range invocations {
			inflatedCol := processGroup(tg, inv)
			receivers <- *inflatedCol
		}
		return nil
	}
}

// invocationGroup will contain info on the groupId and all invocations for that group
// a group will correspond to a column after transformation
type invocationGroup struct {
	GroupId     string
	Invocations []*invocation
}

// groupInvocations will group the invocations according to the grouping strategy in the config.
// groups will be reverse sorted by their latest invocation start time
// [inv1,inv2,inv3,inv4] -> [[inv1,inv2,inv3], [inv4]]
func groupInvocations(log logrus.FieldLogger, tg *configpb.TestGroup, invocations []*invocation) []*invocationGroup {
	groupedInvocations := make(map[string]*invocationGroup)

	var sortedGroups []*invocationGroup

	for _, invocation := range invocations {
		groupIdentifier := extractGroupID(tg, invocation)
		group, ok := groupedInvocations[groupIdentifier]
		if !ok {
			group = &invocationGroup{
				GroupId: groupIdentifier,
			}
			groupedInvocations[groupIdentifier] = group
		}
		group.Invocations = append(group.Invocations, invocation)
	}

	for _, group := range groupedInvocations {
		sortedGroups = append(sortedGroups, group)
	}

	// reverse sort groups by invocation time
	sort.SliceStable(sortedGroups, func(i, j int) bool {
		return sortedGroups[i].Invocations[0].InvocationProto.GetTiming().GetStartTime().GetSeconds() > sortedGroups[j].Invocations[0].InvocationProto.GetTiming().GetStartTime().GetSeconds()
	})

	return sortedGroups
}

func processRawResults(log logrus.FieldLogger, results []*fetchResult) []*invocation {
	var invs []*invocation
	for _, result := range results {
		inv := processRawResult(log, result)
		invs = append(invs, inv)
	}
	return invs
}

// processRawResult converts raw fetchResult to invocation with single action/target result/configured target result per targetID
// Will skip processing any entries without Target or ConfiguredTarget
func processRawResult(log logrus.FieldLogger, result *fetchResult) *invocation {

	multiActionResults := collateRawResults(log, result)
	singleActionResults := isolateActions(log, multiActionResults)

	return &invocation{result.Invocation, singleActionResults}
}

// collateRawResults collates targets, configured targets and multiple actions into a single structure using targetID as a key
func collateRawResults(log logrus.FieldLogger, result *fetchResult) map[string]*multiActionResult {
	multiActionResults := make(map[string]*multiActionResult)
	for _, target := range result.Targets {
		trID := target.GetId().GetTargetId()
		tr, ok := multiActionResults[trID]
		if !ok {
			tr = &multiActionResult{}
			multiActionResults[trID] = tr
		} else if tr.TargetProto != nil {
			logrus.WithField("id", trID).Debug("Found duplicate target where not expected.")
		}
		tr.TargetProto = target
	}
	for _, configuredTarget := range result.ConfiguredTargets {
		trID := configuredTarget.GetId().GetTargetId()
		tr, ok := multiActionResults[trID]
		if !ok {
			tr = &multiActionResult{}
			multiActionResults[trID] = tr
			logrus.WithField("id", trID).Debug("Configured target doesn't have corresponding target?")
		} else if tr.ConfiguredTargetProto != nil {
			logrus.WithField("id", trID).Debug("Found duplicate configured target where not expected.")
		}
		tr.ConfiguredTargetProto = configuredTarget
	}
	for _, action := range result.Actions {
		trID := action.GetId().GetTargetId()
		tr, ok := multiActionResults[trID]
		if !ok {
			tr = &multiActionResult{}
			multiActionResults[trID] = tr
			logrus.WithField("id", trID).Debug("Action doesn't have corresponding target or configured target?")
		}
		tr.ActionProtos = append(tr.ActionProtos, action)
	}
	return multiActionResults
}

// isolateActions splits multiActionResults into one per action
// Any entries without Target or ConfiguredTarget will be skipped
func isolateActions(log logrus.FieldLogger, multiActionResults map[string]*multiActionResult) map[string][]*singleActionResult {
	singleActionResults := make(map[string][]*singleActionResult)
	for trID, multitr := range multiActionResults {
		if multitr == nil || multitr.TargetProto == nil || multitr.ConfiguredTargetProto == nil {
			logrus.WithField("id", trID).WithField("rawTargetResult", multitr).Debug("Missing something from rawTargetResult entry.")
			continue
		}
		// no actions for some reason
		if multitr.ActionProtos == nil {
			tr := &singleActionResult{multitr.TargetProto, multitr.ConfiguredTargetProto, nil}
			singleActionResults[trID] = append(singleActionResults[trID], tr)
		}
		for _, action := range multitr.ActionProtos {
			tr := &singleActionResult{multitr.TargetProto, multitr.ConfiguredTargetProto, action}
			singleActionResults[trID] = append(singleActionResults[trID], tr)
		}
	}
	return singleActionResults
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

// includeStatus determines if the single action result should be included based on config
func includeStatus(tg *configpb.TestGroup, sar *singleActionResult) bool {
	status := convertStatus[sar.TargetProto.GetStatusAttributes().GetStatus()]
	if status == statuspb.TestStatus_NO_RESULT {
		return false
	}
	if status == statuspb.TestStatus_BUILD_PASSED && tg.IgnoreBuilt {
		return false
	}
	if status == statuspb.TestStatus_RUNNING && tg.IgnorePending {
		return false
	}
	if status == statuspb.TestStatus_PASS_WITH_SKIPS && tg.IgnoreSkip {
		return false
	}
	return true
}

// processGroup will convert grouped invocations into columns
func processGroup(tg *configpb.TestGroup, inv *invocation) *updater.InflatedColumn {
	if inv == nil || inv.InvocationProto == nil {
		return nil
	}

	started := inv.InvocationProto.GetTiming().GetStartTime()
	groupID := inv.InvocationProto.GetId().GetInvocationId()
	build := identifyBuild(tg, inv)
	// override build by invocation ID
	if build == "" {
		build = groupID
	}
	hint, err := started.AsTime().MarshalText()
	if err != nil {
		hint = []byte{}
	}
	col := &statepb.Column{
		Build:   build,
		Name:    groupID,
		Started: timestampMilliseconds(started),
		Hint:    string(hint),
	}
	cells := make(map[string]updater.Cell)

	for _, targetResults := range inv.TargetResults {
		for _, satr := range targetResults {
			status, ok := convertStatus[satr.TargetProto.GetStatusAttributes().GetStatus()]
			if !ok {
				status = statuspb.TestStatus_UNKNOWN
			}
			cells[satr.TargetProto.GetId().GetTargetId()] = updater.Cell{
				Result: status,
				ID:     satr.TargetProto.GetId().GetTargetId(),
				CellID: inv.InvocationProto.GetId().GetInvocationId(),
			}
		}
	}

	invStatus, ok := convertStatus[inv.InvocationProto.GetStatusAttributes().GetStatus()]
	if !ok {
		invStatus = statuspb.TestStatus_UNKNOWN
	}
	cells["Overall"] = updater.Cell{
		Result: invStatus,
		ID:     "Overall",
		CellID: inv.InvocationProto.GetId().GetInvocationId(),
	}
	return &updater.InflatedColumn{
		Column: col,
		Cells:  cells,
	}
}

// identifyBuild applies build override configurations and assigns a build
// Returns an empty string if no configurations are present or no configs are correctly set.
// i.e. no key is found in properties.
func identifyBuild(tg *configpb.TestGroup, inv *invocation) string {
	switch {
	case tg.GetBuildOverrideConfigurationValue() != "":
		key := tg.GetBuildOverrideConfigurationValue()
		for _, property := range inv.InvocationProto.GetProperties() {
			if property.GetKey() == key {
				return property.GetValue()
			}
		}
		return ""
	case tg.GetBuildOverrideStrftime() != "":
		layout := updater.FormatStrftime(tg.BuildOverrideStrftime)
		timing := inv.InvocationProto.GetTiming().GetStartTime()
		startTime := time.Unix(timing.Seconds, int64(timing.Nanos)).UTC()
		return startTime.Format(layout)
	default:
		return ""
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
	queryProw = "invocation_attributes.labels:\"prow\""
)

func search(ctx context.Context, log logrus.FieldLogger, client *DownloadClient, projectID string, stop time.Time) ([]string, error) {
	if client == nil {
		return nil, fmt.Errorf("no ResultStore client provided")
	}
	query := queryAfter(queryProw, stop)
	log.WithField("query", query).Debug("Searching ResultStore.")
	// Quit if search goes over 5 minutes.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	ids, err := client.Search(ctx, log, query, projectID)
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
