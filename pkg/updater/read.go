/*
Copyright 2020 The TestGrid Authors.

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

package updater

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	statuspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/fvbommel/sortorder"
	"github.com/sirupsen/logrus"
)

// hintStarted returns the maximum hint
func hintStarted(cols []InflatedColumn) string {
	var hint string
	for i, col := range cols {
		if newHint := col.Column.Hint; i == 0 || sortorder.NaturalLess(hint, newHint) {
			hint = newHint
		}
	}
	return hint
}

func gcsColumnReader(client gcs.Client, buildTimeout time.Duration, concurrency int) ColumnReader {
	return func(ctx context.Context, parentLog logrus.FieldLogger, tg *configpb.TestGroup, oldCols []InflatedColumn, stop time.Time, receivers chan<- InflatedColumn) error {
		tgPaths, err := groupPaths(tg)
		if err != nil {
			return fmt.Errorf("group path: %w", err)
		}

		since := hintStarted(oldCols)
		log := parentLog.WithField("since", since)

		log.Trace("Listing builds...")
		builds, err := listBuilds(ctx, client, since, tgPaths...)
		if err != nil {
			return fmt.Errorf("list builds: %w", err)
		}
		log.WithField("total", len(builds)).Debug("Listed builds")

		return readColumns(ctx, client, log, tg, builds, stop, buildTimeout, receivers)
	}
}

// readColumns will list, download and process builds into inflatedColumns.
func readColumns(ctx context.Context, client gcs.Downloader, log logrus.FieldLogger, group *configpb.TestGroup, builds []gcs.Build, stop time.Time, buildTimeout time.Duration, receivers chan<- InflatedColumn) error {
	if len(builds) == 0 {
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // do not leak go routines

	nameCfg := makeNameConfig(group)
	var heads []string
	for _, h := range group.ColumnHeader {
		heads = append(heads, h.ConfigurationValue)
	}

	var good bool
	bad := make([]string, 0, 11)

	// TODO(fejta): restore inter-build concurrency
	var failures int // since last good column
	var extra []string
	var started float64
	for i := len(builds) - 1; i >= 0; i-- {
		b := builds[i]
		log := log.WithField("build", b)
		buildCtx, cancelBuild := context.WithTimeout(ctx, buildTimeout)
		log.Trace("Reading result")
		result, err := readResult(buildCtx, client, b, stop)
		cancelBuild()
		id := path.Base(b.Path.Object())
		var col InflatedColumn
		if err != nil {
			switch len(bad) {
			case 10:
				bad[9] = "..."
				bad = append(bad, id)
			case 11:
				bad[10] = id
			default:
				bad = append(bad, id)
			}
			failures++
			log.WithError(err).Trace("Failed to read build")
			if extra == nil {
				extra = make([]string, len(heads))
			}
			when := started + 0.01*float64(failures)
			if err == errAncient {
				col = ancientColumn(id, when, extra)
			} else {
				msg := fmt.Sprintf("Failed to download %s: %s", b, err.Error())
				col = erroredColumn(id, when, extra, msg)
			}
		} else {
			col = convertResult(log, nameCfg, id, heads, *result, makeOptions(group))
			log.WithField("rows", len(col.Cells)).Debug("Read result")
			failures = 0
			good = true
			extra = col.Column.Extra
			started = col.Column.Started
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case receivers <- col:
		}
	}

	switch {
	case !good && len(bad) == 0:
		log.WithField("prefix", "gs://"+group.GcsPrefix).Trace("No recent builds")
	case !good:
		return fmt.Errorf("%d builds failed: %v", len(bad), bad)
	}
	return nil
}

func ancientColumn(id string, when float64, extra []string) InflatedColumn {
	return InflatedColumn{
		Column: &statepb.Column{
			Build:   id,
			Hint:    id,
			Started: when,
			Extra:   extra,
		},
		Cells: map[string]Cell{
			overallRow: {
				Message: "Build is too old to process",
				Result:  statuspb.TestStatus_UNKNOWN,
			},
		},
	}
}

func erroredColumn(id string, when float64, extra []string, msg string) InflatedColumn {
	return InflatedColumn{
		Column: &statepb.Column{
			Build:   id,
			Hint:    id,
			Started: when,
			Extra:   extra,
		},
		Cells: map[string]Cell{
			overallRow: {
				Message: msg,
				Result:  statuspb.TestStatus_TOOL_FAIL,
			},
		},
	}
}

type groupOptions struct {
	merge          bool
	analyzeProwJob bool
	addCellID      bool
	metricKey      string
	userKey        string
}

func makeOptions(group *configpb.TestGroup) groupOptions {
	return groupOptions{
		merge:          !group.DisableMergedStatus,
		analyzeProwJob: !group.DisableProwjobAnalysis,
		addCellID:      group.BuildOverrideStrftime != "",
		metricKey:      group.ShortTextMetric,
		userKey:        group.UserProperty,
	}
}

const (
	testsName = "Tests name"
	jobName   = "Job name"
)

type nameConfig struct {
	format   string
	parts    []string
	multiJob bool
}

// render the metadata into the expect test name format.
//
// Argument order determines precedence.
func (nc nameConfig) render(job, test string, metadatas ...map[string]string) string {
	parsed := make([]interface{}, len(nc.parts))
	for i, p := range nc.parts {
		var s string
		switch p {
		case jobName:
			s = job
		case testsName:
			s = test
		default:
			for _, metadata := range metadatas {
				v, present := metadata[p]
				if present {
					s = v
					break
				}
			}
		}
		parsed[i] = s
	}
	return fmt.Sprintf(nc.format, parsed...)
}

func makeNameConfig(group *configpb.TestGroup) nameConfig {
	nameCfg := convertNameConfig(group.TestNameConfig)
	if strings.Contains(group.GcsPrefix, ",") {
		nameCfg.multiJob = true
		ensureJobName(&nameCfg)
	}
	return nameCfg
}

func firstFilled(strs ...string) string {
	for _, s := range strs {
		if s != "" {
			return s
		}
	}
	return ""
}

func convertNameConfig(tnc *configpb.TestNameConfig) nameConfig {
	if tnc == nil {
		return nameConfig{
			format: "%s",
			parts:  []string{testsName},
		}
	}
	nc := nameConfig{
		format: tnc.NameFormat,
		parts:  make([]string, len(tnc.NameElements)),
	}
	for i, e := range tnc.NameElements {
		// TODO(fejta): build_target = true
		// TODO(fejta): tags = 'SOMETHING'
		nc.parts[i] = firstFilled(e.TargetConfig, e.TestProperty)
	}
	return nc
}

func ensureJobName(nc *nameConfig) {
	for _, p := range nc.parts {
		if p == jobName {
			return
		}
	}
	nc.format = "%s." + nc.format
	nc.parts = append([]string{jobName}, nc.parts...)
}

var (
	errAncient = errors.New("build is too old")
)

// readResult will download all GCS artifacts in parallel.
//
// Specifically download the following files:
// * started.json
// * finished.json
// * any junit.xml files under the artifacts directory.
func readResult(parent context.Context, client gcs.Downloader, build gcs.Build, stop time.Time) (*gcsResult, error) {
	ctx, cancel := context.WithCancel(parent) // Allows aborting after first error
	defer cancel()
	result := gcsResult{
		job:   build.Job(),
		build: build.Build(),
	}
	ec := make(chan error) // Receives errors from anyone

	var lock sync.Mutex
	addMalformed := func(s ...string) {
		lock.Lock()
		defer lock.Unlock()
		result.malformed = append(result.malformed, s...)
	}

	var work int

	// Download podinfo.json
	work++
	go func() {
		pi, err := build.PodInfo(ctx, client)
		switch {
		case errors.Is(err, io.EOF):
			addMalformed("podinfo.json")
			err = nil
		case err != nil:
			err = fmt.Errorf("podinfo: %w", err)
		case pi != nil:
			result.podInfo = *pi
		}
		select {
		case <-ctx.Done():
		case ec <- err:
		}
	}()

	// Download started.json
	work++
	go func() {
		s, err := build.Started(ctx, client)
		switch {
		case errors.Is(err, io.EOF):
			addMalformed("started.json")
			err = nil
		case err != nil:
			err = fmt.Errorf("started: %w", err)
		case time.Unix(s.Timestamp, 0).Before(stop):
			err = errAncient
		default:
			result.started = *s
		}
		select {
		case <-ctx.Done():
		case ec <- err:
		}
	}()

	// Download finished.json
	work++
	go func() {
		f, err := build.Finished(ctx, client)
		switch {
		case errors.Is(err, io.EOF):
			addMalformed("finished.json")
			err = nil
		case err != nil:
			err = fmt.Errorf("finished: %w", err)
		default:
			result.finished = *f
		}
		select {
		case <-ctx.Done():
		case ec <- err:
		}
	}()

	// Download suites
	work++
	go func() {
		suites, err := readSuites(ctx, client, build)
		if err != nil {
			err = fmt.Errorf("suites: %w", err)
		}
		var problems []string
		for _, s := range suites {
			if s.Err != nil {
				p := strings.TrimPrefix(s.Path, build.Path.String())
				problems = append(problems, fmt.Sprintf("%s: %s", p, s.Err))
			} else {
				result.suites = append(result.suites, s)
			}
		}
		if len(problems) > 0 {
			addMalformed(problems...)
		}

		select {
		case <-ctx.Done():
		case ec <- err:
		}
	}()

	for ; work > 0; work-- {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout: %w", ctx.Err())
		case err := <-ec:
			if err != nil {
				return nil, err
			}
		}
	}
	sort.Slice(result.malformed, func(i, j int) bool {
		return result.malformed[i] < result.malformed[j]
	})
	return &result, nil
}

// readSuites asynchrounously lists and downloads junit.xml files
func readSuites(parent context.Context, client gcs.Downloader, build gcs.Build) ([]gcs.SuitesMeta, error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	ec := make(chan error)

	// List
	artifacts := make(chan string, 1)
	go func() {
		defer close(artifacts) // No more artifacts
		if err := build.Artifacts(ctx, client, artifacts); err != nil {
			select {
			case <-ctx.Done():
			case ec <- fmt.Errorf("list: %w", err):
			}
		}
	}()

	// Download
	suitesChan := make(chan gcs.SuitesMeta, 1)
	go func() {
		defer close(suitesChan) // No more rows
		const max = 1000
		if err := build.Suites(ctx, client, artifacts, suitesChan, max); err != nil {
			select {
			case <-ctx.Done():
			case ec <- fmt.Errorf("download: %w", err):
			}
		}
	}()

	// Append
	var suites []gcs.SuitesMeta
	go func() {
		for suite := range suitesChan {
			suites = append(suites, suite)
		}
		select {
		case <-ctx.Done():
		case ec <- nil:
		}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-ec:
		if err != nil {
			return nil, err
		}
	}
	return suites, nil
}
