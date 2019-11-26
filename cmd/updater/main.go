/*
Copyright 2018 The Kubernetes Authors.

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
	"flag"
	"fmt"
	"math"
	"net/url"
	"path"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/config"
	"github.com/GoogleCloudPlatform/testgrid/internal/result"
	"github.com/GoogleCloudPlatform/testgrid/metadata/junit"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/pb/state"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"

	"cloud.google.com/go/storage"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/sirupsen/logrus"
	"vbom.ml/util/sortorder"
)

// Build holds data to builds stored in GCS.
type Build = gcs.Build

// Builds holds a slice of builds, which will sort naturally (aka 2 < 10).
type Builds = gcs.Builds

// options configures the updater
type options struct {
	config           gcs.Path // gs://path/to/config/proto
	creds            string
	confirm          bool
	group            string
	groupConcurrency int
	buildConcurrency int
	wait             time.Duration
}

// validate ensures sane options
func (o *options) validate() error {
	if o.config.String() == "" {
		return errors.New("empty --config")
	}
	if o.config.Bucket() == "k8s-testgrid" && o.config.Object() != "beta/config" && o.confirm { // TODO(fejta): remove
		return fmt.Errorf("--config=%s cannot write to gs://k8s-testgrid/config", o.config)
	}
	if o.groupConcurrency == 0 {
		o.groupConcurrency = 4 * runtime.NumCPU()
	}
	if o.buildConcurrency == 0 {
		o.buildConcurrency = 4 * runtime.NumCPU()
	}

	return nil
}

// gatherOptions reads options from flags
func gatherOptions() options {
	o := options{}
	flag.Var(&o.config, "config", "gs://path/to/config.pb")
	flag.StringVar(&o.creds, "gcp-service-account", "", "/path/to/gcp/creds (use local creds if empty)")
	flag.BoolVar(&o.confirm, "confirm", false, "Upload data if set")
	flag.StringVar(&o.group, "test-group", "", "Only update named group if set")
	flag.IntVar(&o.groupConcurrency, "group-concurrency", 0, "Manually define the number of groups to concurrently update if non-zero")
	flag.IntVar(&o.buildConcurrency, "build-concurrency", 0, "Manually define the number of builds to concurrently read if non-zero")
	flag.DurationVar(&o.wait, "wait", 0, "Ensure at least this much time has passed since the last loop (exit if zero).")
	flag.Parse()
	return o
}

// testGroupPath() returns the path to a test_group proto given this proto
func testGroupPath(g gcs.Path, name string) (*gcs.Path, error) {
	u, err := url.Parse(name)
	if err != nil {
		return nil, fmt.Errorf("invalid url %s: %v", name, err)
	}
	np, err := g.ResolveReference(u)
	if err == nil && np.Bucket() != g.Bucket() {
		return nil, fmt.Errorf("testGroup %s should not change bucket", name)
	}
	return np, nil
}

// Row converts the junit result into a Row result, prepending the suite name.
func row(jr junit.Result, suite string) (string, Row) {
	n := jr.Name
	if suite != "" {
		n = suite + "." + n
	}
	r := Row{
		Metrics: map[string]float64{},
		Metadata: map[string]string{
			"Tests name": n,
		},
	}
	if jr.Time > 0 {
		r.Metrics[elapsedKey] = jr.Time
	}
	const max = 140
	if msg := jr.Message(max); msg != "" {
		r.Message = msg
	}
	switch {
	case jr.Failure != nil:
		r.Result = state.Row_FAIL
		if r.Message != "" {
			r.Icon = "F"
		}
	case jr.Skipped != nil:
		r.Result = state.Row_PASS_WITH_SKIPS
		if r.Message != "" {
			r.Icon = "S"
		}
	default:
		r.Result = state.Row_PASS
	}
	return n, r
}

func extractRows(suites junit.Suites, meta map[string]string) map[string][]Row {
	rows := map[string][]Row{}
	for _, suite := range suites.Suites {
		for _, sr := range suite.Results {
			if sr.Skipped != nil && len(*sr.Skipped) == 0 {
				continue
			}

			n, r := row(sr, suite.Name)
			for k, v := range meta {
				r.Metadata[k] = v
			}
			rows[n] = append(rows[n], r)
		}
	}
	return rows
}

// ColumnMetadata holds key => value mapping of metadata info.
type ColumnMetadata map[string]string

// Column represents a build run, which includes one or more row results and metadata.
type Column struct {
	ID       string
	Started  int64
	Finished int64
	Passed   bool
	Rows     map[string][]Row
	Metadata ColumnMetadata
}

// Row holds results for a piece of a build run, such as a test result.
type Row struct {
	Result   state.Row_Result
	Metrics  map[string]float64
	Metadata map[string]string
	Message  string
	Icon     string
}

// Overall calculates the generated-overall row value for the current column
func (br Column) Overall() Row {
	r := Row{
		Metadata: map[string]string{"Tests name": "Overall"},
	}
	switch {
	case br.Finished > 0:
		// Completed, did we pass?
		if br.Passed {
			r.Result = state.Row_PASS // Yep
		} else {
			r.Result = state.Row_FAIL
		}
		r.Metrics = map[string]float64{
			elapsedKey: float64(br.Finished - br.Started),
		}
	case time.Now().Add(-24*time.Hour).Unix() > br.Started:
		// Timed out
		r.Result = state.Row_FAIL
		r.Message = "Testing did not complete within 24 hours"
		r.Icon = "T"
	default:
		r.Result = state.Row_RUNNING
		r.Message = "Still running; has not finished..."
		r.Icon = "R"
	}
	return r
}

// AppendMetric adds the value at index to metric.
//
// Handles the details of sparse-encoding the results.
// Indices must be monotonically increasing for the same metric.
func AppendMetric(metric *state.Metric, idx int32, value float64) {
	if l := int32(len(metric.Indices)); l == 0 || metric.Indices[l-2]+metric.Indices[l-1] != idx {
		// If we append V to idx 9 and metric.Indices = [3, 4] then the last filled index is 3+4-1=7
		// So that means we have holes in idx 7 and 8, so start a new group.
		metric.Indices = append(metric.Indices, idx, 1)
	} else {
		metric.Indices[l-1]++ // Expand the length of the current filled list
	}
	metric.Values = append(metric.Values, value)
}

// FindMetric returns the first metric with the specified name.
func FindMetric(row *state.Row, name string) *state.Metric {
	for _, m := range row.Metrics {
		if m.Name == name {
			return m
		}
	}
	return nil
}

var noResult = Row{Result: state.Row_NO_RESULT}

// AppendResult adds the rowResult column to the row.
//
// Handles the details like missing fields and run-length-encoding the result.
func AppendResult(row *state.Row, rowResult Row, count int) {
	latest := int32(rowResult.Result)
	n := len(row.Results)
	switch {
	case n == 0, row.Results[n-2] != latest:
		row.Results = append(row.Results, latest, int32(count))
	default:
		row.Results[n-1] += int32(count)
	}

	for i := 0; i < count; i++ { // TODO(fejta): update server to allow empty cellids
		row.CellIds = append(row.CellIds, "")
	}

	// Javascript client expects no result cells to skip icons/messages
	// TODO(fejta): reconsider this
	if rowResult.Result != state.Row_NO_RESULT {
		for i := 0; i < count; i++ {
			row.Messages = append(row.Messages, rowResult.Message)
			row.Icons = append(row.Icons, rowResult.Icon)
		}
	}
}

type nameConfig struct {
	format string
	parts  []string
}

func makeNameConfig(tnc *configpb.TestNameConfig) nameConfig {
	if tnc == nil {
		return nameConfig{
			format: "%s",
			parts:  []string{"Tests name"},
		}
	}
	nc := nameConfig{
		format: tnc.NameFormat,
		parts:  make([]string, len(tnc.NameElements)),
	}
	for i, e := range tnc.NameElements {
		nc.parts[i] = e.TargetConfig
	}
	return nc
}

// Format renders any requested metadata into the name
func (r Row) Format(config nameConfig, meta map[string]string) string {
	parsed := make([]interface{}, len(config.parts))
	for i, p := range config.parts {
		if v, ok := r.Metadata[p]; ok {
			parsed[i] = v
			continue
		}
		parsed[i] = meta[p] // "" if missing
	}
	return fmt.Sprintf(config.format, parsed...)
}

// appendColumn adds the build column to the grid.
//
// This handles details like:
// * rows appearing/disappearing in the middle of the run.
// * adding auto metadata like duration, commit as well as any user-added metadata
// * extracting build metadata into the appropriate column header
// * Ensuring row names are unique and formatted with metadata
func appendColumn(grid *state.Grid, headers []string, format nameConfig, rows map[string]*state.Row, build Column) {
	c := state.Column{
		Build:   build.ID,
		Started: float64(build.Started * 1000),
	}
	for _, h := range headers {
		if build.Finished == 0 {
			c.Extra = append(c.Extra, "")
			continue
		}
		trunc := 0
		var ah string
		if h == "Commit" { // TODO(fejta): fix, jobs use explicit key, support truncation
			h = "repo-commit"
			trunc = 9
			ah = "job-version"
		}
		v, ok := build.Metadata[h]
		if !ok {
			// TODO(fejta): fix, make jobs use one or the other
			if ah == "" {
				logrus.WithFields(logrus.Fields{
					"build":  c.Build,
					"header": h,
				}).Warning("build metadata is missing header")
				v = "missing"
			} else {
				if av, ok := build.Metadata[ah]; ok {
					parts := strings.SplitN(av, "+", 2)
					v = parts[len(parts)-1]
				} else {
					logrus.WithFields(logrus.Fields{
						"build":  c.Build,
						"header": h,
						"alt":    ah,
					}).Warning("build metadata missing both key and alternate key")
				}
			}
		}
		if trunc > 0 && trunc < len(v) {
			v = v[0:trunc]
		}
		c.Extra = append(c.Extra, v)
	}
	grid.Columns = append(grid.Columns, &c)

	missing := map[string]*state.Row{}
	for name, row := range rows {
		missing[name] = row
	}

	found := map[string]bool{}

	for target, results := range build.Rows {
		for _, br := range results {
			prefix := br.Format(format, build.Metadata)
			name := prefix
			// Ensure each name is unique
			// If we have multiple results with the same name foo
			// then append " [n]" to the name so we wind up with:
			//   foo
			//   foo [1]
			//   foo [2]
			//   etc
			for idx := 1; found[name]; idx++ {
				// found[name] exists, so try foo [n+1]
				name = fmt.Sprintf("%s [%d]", prefix, idx)
			}
			// hooray, name not in found
			found[name] = true
			delete(missing, name)

			// Does this row already exist?
			r, ok := rows[name]
			if !ok { // New row
				r = &state.Row{
					Name: name,
					Id:   target,
				}
				rows[name] = r
				grid.Rows = append(grid.Rows, r)
				if n := len(grid.Columns); n > 1 {
					// Add missing entries for more recent builds (aka earlier columns)
					AppendResult(r, noResult, n-1)
				}
			}

			AppendResult(r, br, 1)
			for k, v := range br.Metrics {
				m := FindMetric(r, k)
				if m == nil {
					m = &state.Metric{Name: k}
					r.Metrics = append(r.Metrics, m)
				}
				AppendMetric(m, int32(len(r.Messages)), v)
			}
		}
	}

	for _, row := range missing {
		AppendResult(row, noResult, 1)
	}
}

// alertRows configures the alert for every row that has one.
func alertRows(cols []*state.Column, rows []*state.Row, openFailures, closePasses int) {
	for _, r := range rows {
		r.AlertInfo = alertRow(cols, r, openFailures, closePasses)
	}
}

// alertRow returns an AlertInfo proto if there have been failuresToOpen consecutive failures more recently than passesToClose.
func alertRow(cols []*state.Column, row *state.Row, failuresToOpen, passesToClose int) *state.AlertInfo {
	if failuresToOpen == 0 {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var failures int
	var totalFailures int32
	var passes int
	var compressedIdx int
	ch := result.Iter(ctx, row.Results)
	var lastFail *state.Column
	var latestPass *state.Column
	var failIdx int
	// find the first number of consecutive passesToClose (no alert)
	// or else failuresToOpen (alert).
	for _, col := range cols {
		// TODO(fejta): ignore old running
		rawRes := <-ch
		res := result.Coalesce(rawRes, result.IgnoreRunning)
		if res == state.Row_NO_RESULT {
			if rawRes == state.Row_RUNNING {
				compressedIdx++
			}
			continue
		}
		if res == state.Row_PASS {
			passes++
			if failures >= failuresToOpen {
				latestPass = col // most recent pass before outage
				break
			}
			if passes >= passesToClose {
				return nil // there is no outage
			}
			failures = 0
		}
		if res == state.Row_FAIL {
			passes = 0
			failures++
			totalFailures++
			if failures == 1 { // note most recent failure for this outage
				failIdx = compressedIdx
			}
			lastFail = col
		}
		if res == state.Row_FLAKY {
			passes = 0
			if failures >= failuresToOpen {
				break // cannot definitively say which commit is at fault
			}
			failures = 0
		}
		compressedIdx++
	}
	if failures < failuresToOpen {
		return nil
	}
	msg := row.Messages[failIdx]
	id := row.CellIds[failIdx]
	return alertInfo(totalFailures, msg, id, lastFail, latestPass)
}

// alertInfo returns an alert proto with the configured fields
func alertInfo(failures int32, msg, cellId string, fail, pass *state.Column) *state.AlertInfo {
	return &state.AlertInfo{
		FailCount:      failures,
		FailBuildId:    buildID(fail),
		FailTime:       stamp(fail),
		FailTestId:     cellId,
		FailureMessage: msg,
		PassTime:       stamp(pass),
		PassBuildId:    buildID(pass),
	}
}

// buildID extracts the ID from the first extra row or else the Build field.
func buildID(col *state.Column) string {
	if col == nil {
		return ""
	}
	if len(col.Extra) > 0 {
		return col.Extra[0]
	}
	return col.Build
}

const billion = 1e9

// stamp converts seconds into a timestamp proto
func stamp(col *state.Column) *timestamp.Timestamp {
	if col == nil {
		return nil
	}
	seconds := col.Started
	floor := math.Floor(seconds)
	remain := seconds - floor
	return &timestamp.Timestamp{
		Seconds: int64(floor),
		Nanos:   int32(remain * billion),
	}
}

const elapsedKey = "seconds-elapsed"

// readBuild asynchronously downloads the files in build from gcs and converts them into a build.
func readBuild(build Build) (*Column, error) {
	var wg sync.WaitGroup                                             // Each subtask does wg.Add(1), then we wg.Wait() for them to finish
	ctx, cancel := context.WithTimeout(build.Context, 30*time.Second) // Allows aborting after first error
	build.Context = ctx
	ec := make(chan error) // Receives errors from anyone

	// Download started.json, send to sc
	wg.Add(1)
	sc := make(chan gcs.Started) // Receives started.json result
	go func() {
		defer wg.Done()
		started, err := build.Started()
		if err != nil {
			select {
			case <-ctx.Done():
			case ec <- err:
			}
			return
		}
		select {
		case <-ctx.Done():
		case sc <- *started:
		}
	}()

	// Download finished.json, send to fc
	wg.Add(1)
	fc := make(chan gcs.Finished) // Receives finished.json result
	go func() {
		defer wg.Done()
		finished, err := build.Finished()
		if err != nil {
			select {
			case <-ctx.Done():
			case ec <- err:
			}
			return
		}
		select {
		case <-ctx.Done():
		case fc <- *finished:
		}
	}()

	// List artifacts to the artifacts channel
	wg.Add(1)
	artifacts := make(chan string) // Receives names of arifacts
	go func() {
		defer wg.Done()
		defer close(artifacts) // No more artifacts
		if err := build.Artifacts(artifacts); err != nil {
			select {
			case <-ctx.Done():
			case ec <- err:
			}
		}
	}()

	// Download each artifact, send row map to rc
	// With parallelism: 60s without: 220s
	wg.Add(1)
	suitesChan := make(chan gcs.SuitesMeta)
	go func() {
		defer wg.Done()
		defer close(suitesChan) // No more rows
		if err := build.Suites(artifacts, suitesChan); err != nil {
			select {
			case <-ctx.Done():
			case ec <- err:
			}
		}
	}()

	// Append each row into the column
	rows := map[string][]Row{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for suitesMeta := range suitesChan {
			rowsPart := extractRows(suitesMeta.Suites, suitesMeta.Metadata)
			for name, results := range rowsPart {
				rows[name] = append(rows[name], results...)
			}
		}
	}()

	// Wait for everyone to complete their work
	go func() {
		wg.Wait()
		select {
		case <-ctx.Done():
			return
		case ec <- nil:
		}
	}()
	var finished *gcs.Finished
	var started *gcs.Started
	for { // Wait until we receive started and finished and/or an error
		select {
		case err := <-ec:
			if err != nil {
				cancel()
				return nil, fmt.Errorf("failed to read %s: %v", build, err)
			}
			break
		case s := <-sc:
			started = &s
		case f := <-fc:
			finished = &f
		}
		if started != nil && finished != nil {
			break
		}
	}
	br := Column{
		ID:      path.Base(build.Prefix),
		Started: started.Timestamp,
	}
	// Has the build finished?
	if finished.Running { // No
		cancel()
		br.Rows = map[string][]Row{
			"Overall": {br.Overall()},
		}
		return &br, nil
	}
	if finished.Timestamp != nil {
		br.Finished = *finished.Timestamp
	}
	br.Metadata = finished.Metadata.Strings()
	if finished.Passed != nil {
		br.Passed = *finished.Passed
	}
	or := br.Overall()
	br.Rows = map[string][]Row{
		"Overall": {or},
	}
	select {
	case <-ctx.Done():
		cancel()
		return nil, fmt.Errorf("interrupted reading %s", build)
	case err := <-ec:
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to read %s: %v", build, err)
		}
	}

	for t, rs := range rows {
		br.Rows[t] = append(br.Rows[t], rs...)
	}
	if or.Result == state.Row_FAIL { // Ensure failing build has a failing row
		ft := false
		for n, rs := range br.Rows {
			if n == "Overall" {
				continue
			}
			for _, r := range rs {
				if r.Result == state.Row_FAIL {
					ft = true // Failing test, huzzah!
					break
				}
			}
			if ft {
				break
			}
		}
		if !ft { // Nope, add the F icon and an explanatory message
			br.Rows["Overall"][0].Icon = "F"
			br.Rows["Overall"][0].Message = "Build failed outside of test results"
		}
	}

	cancel()
	return &br, nil
}

// Headers returns the list of ColumnHeader ConfigurationValues for this group.
func Headers(group configpb.TestGroup) []string {
	var extra []string
	for _, h := range group.ColumnHeader {
		extra = append(extra, h.ConfigurationValue)
	}
	return extra
}

// Rows is a slice of Row pointers
type Rows []*state.Row

func (r Rows) Len() int      { return len(r) }
func (r Rows) Swap(i, j int) { r[i], r[j] = r[j], r[i] }
func (r Rows) Less(i, j int) bool {
	return sortorder.NaturalLess(r[i].Name, r[j].Name)
}

// readBuilds will asynchronously construct a Grid for the group out of the specified builds.
func readBuilds(parent context.Context, group configpb.TestGroup, builds Builds, max int, dur time.Duration, concurrency int) (*state.Grid, error) {
	// Spawn build readers
	if concurrency == 0 {
		return nil, fmt.Errorf("zero readers for %s", group.Name)
	}
	log := logrus.WithField("group", group.Name).WithField("prefix", "gs://"+group.GcsPrefix)
	ctx, cancel := context.WithCancel(parent)
	var stop time.Time
	if dur != 0 {
		stop = time.Now().Add(-dur)
	}
	lb := len(builds)
	if lb > max {
		log.WithField("total", lb).WithField("max", max).Debug("Truncating")
		lb = max
	}
	cols := make([]*Column, lb)
	log.WithField("duration", dur).Debug("Updating")
	ec := make(chan error)
	old := make(chan int)
	var wg sync.WaitGroup

	// Send build indices to readers
	indices := make(chan int)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(indices)
		for i := range builds[:lb] {
			select {
			case <-ctx.Done():
				return
			case <-old:
				return
			case indices <- i:
			}
		}
	}()

	// Concurrently receive indices and read builds
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case i, open := <-indices:
					if !open {
						return
					}
					b := builds[i]
					c, err := readBuild(b)
					if err != nil {
						ec <- err
						return
					}
					cols[i] = c
					if c.Started < stop.Unix() {
						select {
						case <-ctx.Done():
						case old <- i:
							log.WithFields(logrus.Fields{
								"idx":     i,
								"prefix":  b.Prefix,
								"started": c.Started,
								"stop":    stop.Unix(),
							}).Info("stopping")
						default: // Someone else may have already reported an old result
						}
					}
				}
			}
		}()
	}

	// Wait for everyone to finish
	go func() {
		wg.Wait()
		select {
		case <-ctx.Done():
		case ec <- nil: // No error
		}
	}()

	// Determine if we got an error
	select {
	case <-ctx.Done():
		cancel()
		return nil, fmt.Errorf("interrupted reading %s", group.Name)
	case err := <-ec:
		if err != nil {
			cancel()
			return nil, fmt.Errorf("error reading %s: %v", group.Name, err)
		}
	}

	// Add the columns into a grid message
	grid := &state.Grid{}
	rows := map[string]*state.Row{} // For fast target => row lookup
	heads := Headers(group)
	nameCfg := makeNameConfig(group.TestNameConfig)
	failsOpen := int(group.NumFailuresToAlert)
	passesClose := int(group.NumPassesToDisableAlert)
	if failsOpen > 0 && passesClose == 0 {
		passesClose = 1
	}

	for _, c := range cols {
		select {
		case <-ctx.Done():
			cancel()
			return nil, fmt.Errorf("interrupted appending columns to %s", group.Name)
		default:
		}
		if c == nil {
			continue
		}
		appendColumn(grid, heads, nameCfg, rows, *c)
		alertRows(grid.Columns, grid.Rows, failsOpen, passesClose)
		if c.Started < stop.Unix() { // There may be concurrency results < stop.Unix()
			logrus.WithFields(logrus.Fields{
				"group": group.Name,
				"id":    c.ID,
				"stop":  stop,
			}).Debug("Column started before oldest allowed time, stopping processing earlier columns")
			break // Just process the first result < stop.Unix()
		}
	}
	sort.Stable(Rows(grid.Rows))
	cancel()
	return grid, nil
}

// Days converts days float into a time.Duration, assuming a 24 hour day.
//
// A day is not always 24 hours due to things like leap-seconds.
// We do not need this level of precision though, so ignore the complexity.
func Days(d float64) time.Duration {
	return time.Duration(24*d) * time.Hour // Close enough
}

func main() {
	opt := gatherOptions()
	if err := opt.validate(); err != nil {
		logrus.Fatalf("Invalid flags: %v", err)
	}
	if !opt.confirm {
		logrus.Warning("--confirm=false (DRY-RUN): will not write to gcs")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updateOnce(ctx, opt)
	if opt.wait == 0 {
		return
	}
	timer := time.NewTimer(opt.wait)
	defer timer.Stop()
	for range timer.C {
		timer.Reset(opt.wait)
		updateOnce(ctx, opt)
		logrus.WithField("wait", opt.wait).Info("Sleeping...")
	}
}

func updateOnce(ctx context.Context, opt options) {
	client, err := gcs.ClientWithCreds(ctx, opt.creds)
	if err != nil {
		logrus.Fatalf("Failed to create storage client: %v", err)
	}
	defer client.Close()

	cfg, err := config.ReadGCS(ctx, client.Bucket(opt.config.Bucket()).Object(opt.config.Object()))
	if err != nil {
		logrus.Fatalf("Failed to read %s: %v", opt.config, err)
	}
	logrus.WithField("groups", len(cfg.TestGroups)).Info("Updating test groups")

	groups := make(chan configpb.TestGroup)
	var wg sync.WaitGroup

	for i := 0; i < opt.groupConcurrency; i++ {
		wg.Add(1)
		go func() {
			for tg := range groups {
				tgp, err := testGroupPath(opt.config, tg.Name)
				if err == nil {
					err = updateGroup(ctx, client, tg, *tgp, opt.buildConcurrency, opt.confirm)
				}
				if err != nil {
					logrus.WithField("group", tg.Name).WithError(err).Error("could not update group")
				}
			}
			wg.Done()
		}()
	}

	if opt.group != "" { // Just a specific group
		tg := config.FindTestGroup(opt.group, cfg)
		if tg == nil {
			logrus.WithField("group", opt.group).WithField("config", opt.config).Fatal("group not found")
		}
		groups <- *tg
	} else { // All groups
		idxChan := make(chan int)
		defer close(idxChan)
		go logUpdate(idxChan, len(cfg.TestGroups), "Update in progress")
		for i, tg := range cfg.TestGroups {
			select {
			case idxChan <- i:
			default:
			}
			groups <- *tg
		}
	}
	close(groups)
	wg.Wait()
}

// logUpdate posts updateOnce progress every minute, including an ETA for completion.
func logUpdate(ch <-chan int, total int, msg string) {
	start := time.Now()
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	var current int
	var ok bool
	for {
		select {
		case current, ok = <-ch:
			if !ok { // channel is closed
				return
			}
		case now := <-timer.C:
			elapsed := now.Sub(start)
			rate := elapsed / time.Duration(current)
			eta := time.Duration(total-current) * rate

			logrus.WithFields(logrus.Fields{
				"current": current,
				"total":   total,
				"percent": (100 * current) / total,
				"remain":  eta.Round(time.Minute),
				"eta":     now.Add(eta).Round(time.Minute),
			}).Info(msg)
			timer.Reset(time.Minute)
		}
	}
}

func updateGroup(ctx context.Context, client *storage.Client, tg configpb.TestGroup, gridPath gcs.Path, concurrency int, write bool) error {
	log := logrus.WithField("group", tg.Name)
	o := tg.Name

	var tgPath gcs.Path
	if err := tgPath.Set("gs://" + tg.GcsPrefix); err != nil {
		return fmt.Errorf("group %s has an invalid gcs_prefix %s: %v", o, tg.GcsPrefix, err)
	}

	g := state.Grid{}
	g.Columns = append(g.Columns, &state.Column{Build: "first", Started: 1})
	log.Info("Listing builds")
	builds, err := gcs.ListBuilds(ctx, client, tgPath)
	if err != nil {
		return fmt.Errorf("failed to list %s builds: %v", o, err)
	}
	var dur time.Duration
	if tg.DaysOfResults > 0 {
		dur = Days(float64(tg.DaysOfResults))
	} else {
		dur = Days(7)
	}
	const maxCols = 50
	grid, err := readBuilds(ctx, tg, builds, maxCols, dur, concurrency)
	if err != nil {
		return err
	}
	buf, err := marshalGrid(*grid)
	if err != nil {
		return fmt.Errorf("failed to marshal %s grid: %v", o, err)
	}
	tgp := gridPath
	log = log.WithField("url", tgp).WithField("bytes", len(buf))
	if !write {
		log.Info("Skipping write")
	} else {
		log.Debug("Writing")
		// TODO(fejta): configurable cache value
		if err := gcs.Upload(ctx, client, tgp, buf, gcs.DefaultAcl, "no-cache"); err != nil {
			return fmt.Errorf("upload %s to %s failed: %v", o, tgp, err)
		}
	}
	log.WithFields(logrus.Fields{
		"cols": len(grid.Columns),
		"rows": len(grid.Rows),
	}).Info("Wrote grid")
	return nil
}

// marhshalGrid serializes a state proto into zlib-compressed bytes.
func marshalGrid(grid state.Grid) ([]byte, error) {
	buf, err := proto.Marshal(&grid)
	if err != nil {
		return nil, fmt.Errorf("proto encoding failed: %v", err)
	}
	var zbuf bytes.Buffer
	zw := zlib.NewWriter(&zbuf)
	if _, err = zw.Write(buf); err != nil {
		return nil, fmt.Errorf("zlib compression failed: %v", err)
	}
	if err = zw.Close(); err != nil {
		return nil, fmt.Errorf("zlib closing failed: %v", err)
	}
	return zbuf.Bytes(), nil
}
