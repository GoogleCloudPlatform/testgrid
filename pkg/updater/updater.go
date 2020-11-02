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

// Package updater reads the latest test results and saves updated state.
package updater

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"path"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/fvbommel/sortorder"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/sirupsen/logrus"

	"github.com/GoogleCloudPlatform/testgrid/config"
	"github.com/GoogleCloudPlatform/testgrid/internal/result"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	statuspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
)

func Update(parent context.Context, client gcs.Client, configPath gcs.Path, gridPrefix string, groupConcurrency int, buildConcurrency int, confirm bool, groupTimeout time.Duration, buildTimeout time.Duration, group string) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	log := logrus.WithField("config", configPath)
	cfg, err := config.ReadGCS(ctx, client, configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	log.WithField("groups", len(cfg.TestGroups)).Info("Updating test groups")

	groups := make(chan configpb.TestGroup)
	var wg sync.WaitGroup

	for i := 0; i < groupConcurrency; i++ {
		wg.Add(1)
		go func() {
			for tg := range groups {
				log := log.WithField("group", tg.Name)
				if !tg.UseKubernetesClient {
					log.Debug("Skipping non kubernetes client group")
					continue
				}
				location := path.Join(gridPrefix, tg.Name)
				tgp, err := testGroupPath(configPath, location)
				if err == nil {
					err = updateGroup(ctx, client, tg, *tgp, buildConcurrency, confirm, groupTimeout, buildTimeout)
				}
				if err != nil {
					log.WithError(err).Error("Error updating group")
				}
				// run the garbage collector after each group to minimize
				// extraneous memory usage.
				runtime.GC()
			}
			wg.Done()
		}()
	}

	if group != "" { // Just a specific group
		tg := config.FindTestGroup(group, cfg)
		if tg == nil {
			return errors.New("group not found")
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
	return nil
}

// testGroupPath() returns the path to a test_group proto given this proto
func testGroupPath(g gcs.Path, name string) (*gcs.Path, error) {
	u, err := url.Parse(name)
	if err != nil {
		return nil, fmt.Errorf("invalid url %s: %w", name, err)
	}
	np, err := g.ResolveReference(u)
	if err != nil {
		return nil, fmt.Errorf("resolve reference: %w", err)
	}
	if err == nil && np.Bucket() != g.Bucket() {
		return nil, fmt.Errorf("testGroup %s should not change bucket", name)
	}
	return np, nil
}

// logUpdate posts Update progress every minute, including an ETA for completion.
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

func groupPath(tg configpb.TestGroup) (*gcs.Path, error) {
	u, err := url.Parse("gs://" + tg.GcsPrefix)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if u.Path != "" && u.Path[len(u.Path)-1] != '/' {
		u.Path += "/"
	}

	var p gcs.Path
	if err := p.SetURL(u); err != nil {
		return nil, err
	}
	return &p, nil
}

// truncateRunning filters out all columns until the oldest still running column.
//
// If there are 20 columns where all are complete except the 3rd and 7th, this will
// return the 8th and later columns.
func truncateRunning(cols []inflatedColumn) []inflatedColumn {
	if len(cols) == 0 {
		return cols
	}
	var stillRunning int
	for i, c := range cols {
		if c.cells["Overall"].result == statuspb.TestStatus_RUNNING {
			stillRunning = i + 1
		}
	}
	return cols[stillRunning:]
}

func updateGroup(parent context.Context, client gcs.Client, tg configpb.TestGroup, gridPath gcs.Path, concurrency int, write bool, groupTimeout, buildTimeout time.Duration) error {
	ctx, cancel := context.WithTimeout(parent, groupTimeout)
	defer cancel()
	log := logrus.WithField("group", tg.Name)

	tgPath, err := groupPath(tg)
	if err != nil {
		return fmt.Errorf("group path: %w", err)
	}

	var dur time.Duration
	if tg.DaysOfResults > 0 {
		dur = days(float64(tg.DaysOfResults))
	} else {
		dur = days(7)
	}
	const maxCols = 50

	stop := time.Now().Add(-dur)

	var oldCols []inflatedColumn

	old, err := downloadGrid(ctx, client, gridPath)
	if err != nil {
		log.WithField("path", gridPath).WithError(err).Error("Failed to download existing grid")
	}
	if old != nil {
		oldCols = truncateRunning(inflateGrid(old, stop, time.Now().Add(-4*time.Hour)))
	}

	var since *gcs.Path
	if len(oldCols) > 0 {
		since, err = tgPath.ResolveReference(&url.URL{Path: oldCols[0].column.Build})
		if err != nil {
			log.WithError(err).Warning("Failed to resolve offset")
		}
		newStop := time.Unix(int64(oldCols[0].column.Started/1000), 0)
		if newStop.After(stop) {
			log.WithFields(logrus.Fields{
				"old columns": len(oldCols),
				"previously":  stop,
				"stop":        newStop,
			}).Debug("Advanced stop")
			stop = newStop
		}
	}

	builds, err := gcs.ListBuilds(ctx, client, *tgPath, since)
	if err != nil {
		return fmt.Errorf("list builds: %w", err)
	}
	log.WithField("total", len(builds)).Debug("Listed builds")

	newCols, err := readColumns(ctx, client, tg, builds, stop, maxCols, buildTimeout, concurrency)
	if err != nil {
		return fmt.Errorf("read columns: %w", err)
	}

	cols := mergeColumns(newCols, oldCols)

	grid := constructGrid(tg, cols)
	buf, err := marshalGrid(grid)
	if err != nil {
		return fmt.Errorf("marshal grid: %w", err)
	}
	log = log.WithField("url", gridPath).WithField("bytes", len(buf))
	if !write {
		log.Debug("Skipping write")
	} else {
		log.Debug("Writing")
		// TODO(fejta): configurable cache value
		if err := client.Upload(ctx, gridPath, buf, gcs.DefaultAcl, "no-cache"); err != nil {
			return fmt.Errorf("upload: %w", err)
		}
	}
	log.WithFields(logrus.Fields{
		"cols": len(grid.Columns),
		"rows": len(grid.Rows),
	}).Info("Wrote grid")
	return nil
}

// mergeColumns combines newCols and oldCols.
//
// When old and new both contain a column, chooses the new column.
func mergeColumns(newCols, oldCols []inflatedColumn) []inflatedColumn {
	// accept all the new columns
	out := append([]inflatedColumn{}, newCols...)
	if len(out) == 0 {
		return oldCols
	}

	// accept all the old columns which are older than the accepted columns.
	oldestCol := out[len(out)-1].column
	for i := 0; i < len(oldCols); i++ {
		if oldCols[i].column.Started > oldestCol.Started || oldCols[i].column.Build == oldestCol.Build {
			continue
		}
		return append(out, oldCols[i:]...)
	}
	return out
}

// days converts days float into a time.Duration, assuming a 24 hour day.
//
// A day is not always 24 hours due to things like leap-seconds.
// We do not need this level of precision though, so ignore the complexity.
func days(d float64) time.Duration {
	return time.Duration(24*d) * time.Hour // Close enough
}

// constructGrid will append all the inflatedColumns into the returned Grid.
//
// The returned Grid has correctly compressed row values.
func constructGrid(group configpb.TestGroup, cols []inflatedColumn) statepb.Grid {
	// Add the columns into a grid message
	var grid statepb.Grid
	rows := map[string]*statepb.Row{} // For fast target => row lookup
	failsOpen := int(group.NumFailuresToAlert)
	passesClose := int(group.NumPassesToDisableAlert)
	if failsOpen > 0 && passesClose == 0 {
		passesClose = 1
	}

	for _, col := range cols {
		appendColumn(&grid, rows, col)
		alertRows(grid.Columns, grid.Rows, failsOpen, passesClose)
	}
	sort.SliceStable(grid.Rows, func(i, j int) bool {
		return sortorder.NaturalLess(grid.Rows[i].Name, grid.Rows[j].Name)
	})

	for _, row := range grid.Rows {
		sort.SliceStable(row.Metric, func(i, j int) bool {
			return sortorder.NaturalLess(row.Metric[i], row.Metric[j])
		})
		sort.SliceStable(row.Metrics, func(i, j int) bool {
			return sortorder.NaturalLess(row.Metrics[i].Name, row.Metrics[j].Name)
		})
	}
	return grid
}

// marhshalGrid serializes a state proto into zlib-compressed bytes.
func marshalGrid(grid statepb.Grid) ([]byte, error) {
	buf, err := proto.Marshal(&grid)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	var zbuf bytes.Buffer
	zw := zlib.NewWriter(&zbuf)
	if _, err = zw.Write(buf); err != nil {
		return nil, fmt.Errorf("compress: %w", err)
	}
	if err = zw.Close(); err != nil {
		return nil, fmt.Errorf("close: %w", err)
	}
	return zbuf.Bytes(), nil
}

// appendMetric adds the value at index to metric.
//
// Handles the details of sparse-encoding the results.
// Indices must be monotonically increasing for the same metric.
func appendMetric(metric *statepb.Metric, idx int32, value float64) {
	if l := int32(len(metric.Indices)); l == 0 || metric.Indices[l-2]+metric.Indices[l-1] != idx {
		// If we append V to idx 9 and metric.Indices = [3, 4] then the last filled index is 3+4-1=7
		// So that means we have holes in idx 7 and 8, so start a new group.
		metric.Indices = append(metric.Indices, idx, 1)
	} else {
		metric.Indices[l-1]++ // Expand the length of the current filled list
	}
	metric.Values = append(metric.Values, value)
}

var emptyCell = cell{result: statuspb.TestStatus_NO_RESULT}

// appendCell adds the rowResult column to the row.
//
// Handles the details like missing fields and run-length-encoding the result.
func appendCell(row *statepb.Row, cell cell, count int) {
	latest := int32(cell.result)
	n := len(row.Results)
	switch {
	case n == 0, row.Results[n-2] != latest:
		row.Results = append(row.Results, latest, int32(count))
	default:
		row.Results[n-1] += int32(count)
	}

	for i := 0; i < count; i++ {
		row.CellIds = append(row.CellIds, cell.cellID)
		if cell.result == statuspb.TestStatus_NO_RESULT {
			continue
		}
		for metricName, measurement := range cell.metrics {
			var metric *statepb.Metric
			var ok bool
			for _, name := range row.Metric {
				if name == metricName {
					ok = true
					break
				}
			}
			if !ok {
				row.Metric = append(row.Metric, metricName)
			}
			for _, metric = range row.Metrics {
				if metric.Name == metricName {
					break
				}
				metric = nil
			}
			if metric == nil {
				metric = &statepb.Metric{Name: metricName}
				row.Metrics = append(row.Metrics, metric)
			}
			// len()-1 because we already appended the cell id
			appendMetric(metric, int32(len(row.CellIds)-1), measurement)
		}
		// Javascript client expects no result cells to skip icons/messages
		row.Messages = append(row.Messages, cell.message)
		row.Icons = append(row.Icons, cell.icon)
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

// appendColumn adds the build column to the grid.
//
// This handles details like:
// * rows appearing/disappearing in the middle of the run.
// * adding auto metadata like duration, commit as well as any user-added metadata
// * extracting build metadata into the appropriate column header
// * Ensuring row names are unique and formatted with metadata
func appendColumn(grid *statepb.Grid, rows map[string]*statepb.Row, inflated inflatedColumn) {
	grid.Columns = append(grid.Columns, inflated.column)

	missing := map[string]*statepb.Row{}
	for name, row := range rows {
		missing[name] = row
	}

	for name, cell := range inflated.cells {
		delete(missing, name)

		row, ok := rows[name]
		if !ok {
			row = &statepb.Row{
				Name: name,
				Id:   name,
			}
			rows[name] = row
			grid.Rows = append(grid.Rows, row)
			if n := len(grid.Columns); n > 1 {
				appendCell(row, emptyCell, n-1)
			}
		}
		appendCell(row, cell, 1)
	}

	for _, row := range missing {
		appendCell(row, emptyCell, 1)
	}
}

// alertRows configures the alert for every row that has one.
func alertRows(cols []*statepb.Column, rows []*statepb.Row, openFailures, closePasses int) {
	for _, r := range rows {
		r.AlertInfo = alertRow(cols, r, openFailures, closePasses)
	}
}

// alertRow returns an AlertInfo proto if there have been failuresToOpen consecutive failures more recently than passesToClose.
func alertRow(cols []*statepb.Column, row *statepb.Row, failuresToOpen, passesToClose int) *statepb.AlertInfo {
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
	var lastFail *statepb.Column
	var latestPass *statepb.Column
	var failIdx int
	// find the first number of consecutive passesToClose (no alert)
	// or else failuresToOpen (alert).
	for _, col := range cols {
		// TODO(fejta): ignore old running
		rawRes := <-ch
		res := result.Coalesce(rawRes, result.IgnoreRunning)
		if res == statuspb.TestStatus_NO_RESULT {
			if rawRes == statuspb.TestStatus_RUNNING {
				compressedIdx++
			}
			continue
		}
		if res == statuspb.TestStatus_PASS {
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
		if res == statuspb.TestStatus_FAIL {
			passes = 0
			failures++
			totalFailures++
			if failures == 1 { // note most recent failure for this outage
				failIdx = compressedIdx
			}
			lastFail = col
		}
		if res == statuspb.TestStatus_FLAKY {
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
func alertInfo(failures int32, msg, cellID string, fail, pass *statepb.Column) *statepb.AlertInfo {
	return &statepb.AlertInfo{
		FailCount:      failures,
		FailBuildId:    buildID(fail),
		FailTime:       stamp(fail),
		FailTestId:     cellID,
		FailureMessage: msg,
		PassTime:       stamp(pass),
		PassBuildId:    buildID(pass),
	}
}

// buildID extracts the ID from the first extra row or else the Build field.
func buildID(col *statepb.Column) string {
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
func stamp(col *statepb.Column) *timestamp.Timestamp {
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
