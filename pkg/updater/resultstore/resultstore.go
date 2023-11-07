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
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/pkg/updater"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/sirupsen/logrus"

	"github.com/GoogleCloudPlatform/testgrid/pb/config"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	cepb "github.com/GoogleCloudPlatform/testgrid/pb/custom_evaluator"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	statuspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	timestamppb "github.com/golang/protobuf/ptypes/timestamp"
	resultstorepb "google.golang.org/genproto/googleapis/devtools/resultstore/v2"
)

// check if interface is implemented correctly
var _ updater.TargetResult = &singleActionResult{}

type TestResultStatus int64

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

// make singleActionResult satisfy TargetResult interface
func (sar *singleActionResult) TargetStatus() statuspb.TestStatus {
	status := convertStatus[sar.TargetProto.GetStatusAttributes().GetStatus()]
	return status
}

func (sar *singleActionResult) extractHeaders(headerConf *configpb.TestGroup_ColumnHeader) []string {
	if sar == nil {
		return nil
	}

	var headers []string

	if key := headerConf.GetProperty(); key != "" {
		tr := &testResult{sar.ActionProto.GetTestAction().GetTestSuite(), nil}
		for _, p := range tr.properties() {
			if p.GetKey() == key {
				headers = append(headers, p.GetValue())
			}
		}
	}

	return headers
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

func (inv *invocation) extractHeaders(headerConf *configpb.TestGroup_ColumnHeader) []string {
	if inv == nil {
		return nil
	}

	var headers []string

	if key := headerConf.GetConfigurationValue(); key != "" {
		for _, prop := range inv.InvocationProto.GetProperties() {
			if prop.GetKey() == key {
				headers = append(headers, prop.GetValue())
			}
		}
	} else if prefix := headerConf.GetLabel(); prefix != "" {
		for _, label := range inv.InvocationProto.GetInvocationAttributes().GetLabels() {
			if strings.HasPrefix(label, prefix) {
				headers = append(headers, label[len(prefix):])
			}
		}
	}
	return headers
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

		groups := groupInvocations(log, tg, invocations)
		for _, group := range groups {
			inflatedCol := processGroup(tg, group)
			receivers <- *inflatedCol
		}
		return nil
	}
}

// cellMessageIcon attempts to find an interesting message and icon by looking at the associated properties and tags.
func cellMessageIcon(annotations []*configpb.TestGroup_TestAnnotation, properties map[string][]string, tags []string) (string, string) {
	check := make(map[string]string, len(properties))
	for k, v := range properties {
		check[k] = v[0]
	}

	tagMap := make(map[string]string, len(annotations))

	for _, a := range annotations {
		n := a.GetPropertyName()
		check[n] = a.ShortText
		tagMap[n] = a.ShortText
	}

	for _, a := range annotations {
		n := a.GetPropertyName()
		icon, ok := check[n]
		if !ok {
			continue
		}
		values, ok := properties[n]
		if !ok || len(values) == 0 {
			continue
		}
		return values[0], icon
	}

	for _, tag := range tags {
		if icon, ok := tagMap[tag]; ok {
			return tag, icon
		}
	}
	return "", ""
}

func numericIcon(current *string, properties map[string][]string, key string) {
	if properties == nil || key == "" {
		return
	}
	vals, ok := properties[key]
	if !ok {
		return
	}
	mean := updater.Means(map[string][]string{key: vals})[key]
	*current = fmt.Sprintf("%f", mean)
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

// customTargetStatus will determine the overridden status based on custom evaluator rule set
func customTargetStatus(ruleSet *cepb.RuleSet, sar *singleActionResult) *statuspb.TestStatus {
	return updater.CustomTargetStatus(ruleSet.GetRules(), sar)
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

// testResult is a convenient representation of resultstore Test proto
// only one of those fields are set at any time for a testResult instance
type testResult struct {
	suiteProto *resultstorepb.TestSuite
	caseProto  *resultstorepb.TestCase
}

// properties return the recursive list of properties for a particular testResult
func (t *testResult) properties() []*resultstorepb.Property {
	var properties []*resultstorepb.Property
	for _, p := range t.suiteProto.GetProperties() {
		properties = append(properties, p)
	}
	for _, p := range t.caseProto.GetProperties() {
		properties = append(properties, p)
	}

	for _, t := range t.suiteProto.GetTests() {
		newTestResult := &testResult{t.GetTestSuite(), t.GetTestCase()}
		properties = append(properties, newTestResult.properties()...)
	}
	return properties
}

// processGroup will convert grouped invocations into columns
func processGroup(tg *configpb.TestGroup, group *invocationGroup) *updater.InflatedColumn {
	if group == nil || group.Invocations == nil {
		return nil
	}
	methodLimit := testMethodLimit(tg)
	matchMethods, unmatchMethods, matchMethodsErr, unmatchMethodsErr := testMethodRegex(tg)

	col := &updater.InflatedColumn{
		Column: &statepb.Column{
			Name: group.GroupId,
		},
		Cells: map[string]updater.Cell{},
	}

	groupedCells := make(map[string][]updater.Cell)

	hintTime := time.Unix(0, 0)
	headers := make([][]string, len(tg.GetColumnHeader()))

	// extract info from underlying invocations and target results
	for _, invocation := range group.Invocations {

		if build := identifyBuild(tg, invocation); build != "" {
			col.Column.Build = build
		} else {
			col.Column.Build = group.GroupId
		}

		started := invocation.InvocationProto.GetTiming().GetStartTime()
		resultStartTime := timestampMilliseconds(started)
		if col.Column.Started == 0 || resultStartTime < col.Column.Started {
			col.Column.Started = resultStartTime
		}

		if started.AsTime().After(hintTime) {
			hintTime = started.AsTime()
		}

		for i, headerConf := range tg.GetColumnHeader() {
			if invHeaders := invocation.extractHeaders(headerConf); invHeaders != nil {
				headers[i] = append(headers[i], invHeaders...)
			}
		}

		if err := matchMethodsErr; err != nil {
			groupedCells["test_method_match_regex"] = append(groupedCells["test_method_match_regex"],
				updater.Cell{
					Result:  statuspb.TestStatus_TOOL_FAIL,
					Message: err.Error(),
				})
		}

		if err := unmatchMethodsErr; err != nil {
			groupedCells["test_method_unmatch_regex"] = append(groupedCells["test_method_unmatch_regex"],
				updater.Cell{
					Result:  statuspb.TestStatus_TOOL_FAIL,
					Message: err.Error(),
				})
		}

		for targetID, singleActionResults := range invocation.TargetResults {
			for _, sar := range singleActionResults {
				if !includeStatus(tg, sar) {
					continue
				}

				// assign status
				status, ok := convertStatus[sar.TargetProto.GetStatusAttributes().GetStatus()]
				if !ok {
					status = statuspb.TestStatus_UNKNOWN
				}
				// TODO(sultan-duisenbay): sanitize build target and apply naming config
				var cell updater.Cell
				cell.CellID = invocation.InvocationProto.GetId().GetInvocationId()
				cell.ID = targetID
				cell.Result = status
				if cr := customTargetStatus(tg.GetCustomEvaluatorRuleSet(), sar); cr != nil {
					cell.Result = *cr
				}
				groupedCells[targetID] = append(groupedCells[targetID], cell)
				testResults := getTestResults(sar.ActionProto.GetTestAction().GetTestSuite())
				testResults, filtered := filterResults(testResults, tg.GetTestMethodProperties(), matchMethods, unmatchMethods)
				processTestResults(tg, groupedCells, testResults, sar, cell, targetID, methodLimit)
				if filtered && len(testResults) == 0 {
					continue
				}

				for i, headerConf := range tg.GetColumnHeader() {
					if targetHeaders := sar.extractHeaders(headerConf); targetHeaders != nil {
						headers[i] = append(headers[i], targetHeaders...)
					}
				}

				// TODO (@bryanlou) check if we need to include properties from the target in addition to test cases
				properties := map[string][]string{}
				testSuite := sar.ActionProto.GetTestAction().GetTestSuite()
				for _, t := range testSuite.GetTests() {
					appendProperties(properties, t, nil)
				}
			}
		}

		for name, cells := range groupedCells {
			split := updater.SplitCells(name, cells...)
			for outName, outCell := range split {
				col.Cells[outName] = outCell
			}
		}
	}

	hint, err := hintTime.MarshalText()
	if err != nil {
		hint = []byte{}
	}

	col.Column.Hint = string(hint)
	col.Column.Extra = compileHeaders(tg.GetColumnHeader(), headers)

	return col
}

// filterProperties returns the subset of results containing all the specified properties.
func filterProperties(results []*resultstorepb.Test, properties []*configpb.TestGroup_KeyValue) []*resultstorepb.Test {
	if len(properties) == 0 {
		return results
	}

	var out []*resultstorepb.Test

	match := make(map[string]bool, len(properties))

	for _, p := range properties {
		match[p.Key] = true
	}

	for _, r := range results {
		found := map[string]string{}
		fillProperties(found, r, match)
		var miss bool
		for _, p := range properties {
			if found[p.Key] != p.Value {
				miss = true
				break
			}
		}
		if miss {
			continue
		}
		out = append(out, r)
	}
	return out
}

// fillProperties reduces the appendProperties result to the single value for each key or "*" if a key has multiple values.
func fillProperties(properties map[string]string, result *resultstorepb.Test, match map[string]bool) {
	if result == nil {
		return
	}
	multiProps := map[string][]string{}

	appendProperties(multiProps, result, match)

	for key, values := range multiProps {
		if len(values) > 1 {
			var diff bool
			for _, v := range values {
				if v != values[0] {
					properties[key] = "*"
					diff = true
					break
				}
			}
			if diff {
				continue
			}
		}
		properties[key] = values[0]
	}
}

// appendProperties from result and its children into a map, optionally filtering to specific matching keys.
func appendProperties(properties map[string][]string, result *resultstorepb.Test, match map[string]bool) {
	if result == nil {
		return
	}

	for _, p := range result.GetTestCase().GetProperties() {
		key := p.Key
		if match != nil && !match[key] {
			continue
		}
		properties[key] = append(properties[key], p.Value)
	}
	testResults := getTestResults(result.GetTestSuite())
	for _, r := range testResults {
		appendProperties(properties, r, match)
	}
}

// matchResults returns the subset of results with matching / without unmatching names.
func matchResults(results []*resultstorepb.Test, match, unmatch *regexp.Regexp) []*resultstorepb.Test {
	if match == nil && unmatch == nil {
		return results
	}
	var out []*resultstorepb.Test
	for _, r := range results {
		if match != nil && !match.MatchString(r.GetTestCase().CaseName) {
			continue
		}
		if unmatch != nil && unmatch.MatchString(r.GetTestCase().CaseName) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// filterResults returns the subset of results and whether or not filtering was applied.
func filterResults(results []*resultstorepb.Test, properties []*config.TestGroup_KeyValue, match, unmatch *regexp.Regexp) ([]*resultstorepb.Test, bool) {
	results = filterProperties(results, properties)
	results = matchResults(results, match, unmatch)
	filtered := len(properties) > 0 || match != nil || unmatch != nil
	return results, filtered
}

// getTestResults traverses through a test suite and returns a list of all tests
// that exists inside of it.
func getTestResults(testSuite *resultstorepb.TestSuite) []*resultstorepb.Test {
	if testSuite == nil {
		return nil
	}

	if len(testSuite.GetTests()) == 0 {
		return []*resultstorepb.Test{
			{
				TestType: &resultstorepb.Test_TestSuite{TestSuite: testSuite},
			},
		}
	}

	var tests []*resultstorepb.Test
	for _, test := range testSuite.GetTests() {
		if test.GetTestCase() != nil {
			tests = append(tests, test)
		} else {
			tests = append(tests, getTestResults(test.GetTestSuite())...)
		}
	}
	return tests
}

func testMethodLimit(tg *configpb.TestGroup) int {
	var testMethodLimit int
	const defaultTestMethodLimit = 20
	if tg == nil {
		return 0
	}
	if tg.EnableTestMethods {
		testMethodLimit = int(tg.MaxTestMethodsPerTest)
		if testMethodLimit == 0 {
			testMethodLimit = defaultTestMethodLimit
		}
	}
	return testMethodLimit
}

func testMethodRegex(tg *configpb.TestGroup) (matchMethods *regexp.Regexp, unmatchMethods *regexp.Regexp, matchMethodsErr error, unmatchMethodsErr error) {
	if tg == nil {
		return
	}
	if m := tg.GetTestMethodMatchRegex(); m != "" {
		matchMethods, matchMethodsErr = regexp.Compile(m)
	}
	if um := tg.GetTestMethodUnmatchRegex(); um != "" {
		unmatchMethods, unmatchMethodsErr = regexp.Compile(um)
	}
	return
}

func mapStatusToCellResult(testCase *resultstorepb.TestCase) statuspb.TestStatus {
	res := testCase.GetResult()
	switch {
	case strings.HasPrefix(testCase.CaseName, "DISABLED_"):
		return statuspb.TestStatus_PASS_WITH_SKIPS
	case res == resultstorepb.TestCase_SKIPPED:
		return statuspb.TestStatus_PASS_WITH_SKIPS
	case res == resultstorepb.TestCase_SUPPRESSED:
		return statuspb.TestStatus_PASS_WITH_SKIPS
	case res == resultstorepb.TestCase_CANCELLED:
		return statuspb.TestStatus_CANCEL
	case res == resultstorepb.TestCase_INTERRUPTED:
		return statuspb.TestStatus_CANCEL
	case len(testCase.Failures) > 0 || len(testCase.Errors) > 0:
		return statuspb.TestStatus_FAIL
	default:
		return statuspb.TestStatus_PASS
	}
}

// processTestResults iterates through a list of test results and adds them to
// a map of groupedcells based on the method name produced
func processTestResults(tg *config.TestGroup, groupedCells map[string][]updater.Cell, testResults []*resultstorepb.Test, sar *singleActionResult, cell updater.Cell, targetID string, testMethodLimit int) {
	tags := sar.TargetProto.TargetAttributes.GetTags()
	testSuite := sar.ActionProto.GetTestAction().GetTestSuite()
	shortTextMetric := tg.GetShortTextMetric()
	for _, testResult := range testResults {
		var methodName string
		properties := map[string][]string{}
		for _, t := range testSuite.GetTests() {
			appendProperties(properties, t, nil)
		}
		if testResult.GetTestCase() != nil {
			methodName = testResult.GetTestCase().GetCaseName()
		} else {
			methodName = testResult.GetTestSuite().GetSuiteName()
		}

		if tg.UseFullMethodNames {
			parts := strings.Split(testResult.GetTestCase().GetClassName(), ".")
			className := parts[len(parts)-1]
			methodName = className + "." + methodName
		}

		methodName = targetID + "@TESTGRID@" + methodName

		trCell := updater.Cell{
			ID:     targetID,    // same targetID as the parent TargetResult
			CellID: cell.CellID, // same cellID
			Result: mapStatusToCellResult(testResult.GetTestCase()),
		}

		trCell.Message, trCell.Icon = cellMessageIcon(tg.TestAnnotations, properties, tags)
		numericIcon(&trCell.Icon, properties, shortTextMetric)

		if trCell.Result == statuspb.TestStatus_PASS_WITH_SKIPS && tg.IgnoreSkip {
			continue
		}
		if len(testResults) <= testMethodLimit {
			groupedCells[methodName] = append(groupedCells[methodName], trCell)
		}
	}
}

// compileHeaders reduces all seen header values down to the final string value.
// Separates multiple values with || when configured, otherwise the value becomes *
func compileHeaders(columnHeader []*configpb.TestGroup_ColumnHeader, headers [][]string) []string {
	if len(columnHeader) == 0 {
		return nil
	}

	var compiledHeaders []string
	for i, headerList := range headers {
		switch {
		case len(headerList) == 0:
			compiledHeaders = append(compiledHeaders, "")
		case len(headerList) == 1:
			compiledHeaders = append(compiledHeaders, headerList[0])
		case columnHeader[i].GetListAllValues():
			var values []string
			for _, value := range headerList {
				values = append(values, value)
			}
			sort.Strings(values)
			compiledHeaders = append(compiledHeaders, strings.Join(values, "||"))
		default:
			compiledHeaders = append(compiledHeaders, "*")
		}
	}
	return compiledHeaders
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
