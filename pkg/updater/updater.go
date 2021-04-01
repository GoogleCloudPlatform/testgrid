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
	"net/http"
	"net/url"
	"path"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"github.com/fvbommel/sortorder"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/googleapi"

	"github.com/GoogleCloudPlatform/testgrid/config"
	"github.com/GoogleCloudPlatform/testgrid/internal/result"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	statuspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
)

// GroupUpdater will compile the grid state proto for the specified group and upload it.
//
// This typically involves downloading the existing state, dropping old columns,
// compiling any new columns and inserting them into the front and then uploading
// the proto to GCS.
type GroupUpdater func(parent context.Context, log logrus.FieldLogger, client gcs.Client, tg *configpb.TestGroup, gridPath gcs.Path) error

// GCS returns a GCS-based GroupUpdater, which knows how to process result data stored in GCS.
func GCS(groupTimeout, buildTimeout time.Duration, concurrency int, write bool) GroupUpdater {
	return func(parent context.Context, log logrus.FieldLogger, client gcs.Client, tg *configpb.TestGroup, gridPath gcs.Path) error {
		if !tg.UseKubernetesClient {
			log.Debug("Skipping non-kubernetes client group")
			return nil
		}
		ctx, cancel := context.WithTimeout(parent, groupTimeout)
		defer cancel()
		gcsColReader := gcsColumnReader(client, buildTimeout, concurrency)
		return InflateDropAppend(ctx, log, client, tg, gridPath, write, gcsColReader)
	}
}

// sortGroups sorts test groups by last update time, returning the current generation ID for each group.
func sortGroups(ctx context.Context, log logrus.FieldLogger, client gcs.Stater, configPath gcs.Path, gridPrefix string, groups []*configpb.TestGroup) (map[string]int64, error) {
	log.Info("Sorting groups")
	updated := make(map[string]time.Time, len(groups))
	generations := make(map[string]int64, len(groups))
	var wg sync.WaitGroup
	var lock sync.Mutex
	for _, tg := range groups {
		tgp, err := testGroupPath(configPath, gridPrefix, tg.Name)
		if err != nil {
			return nil, fmt.Errorf("%s bad group path: %w", tg.Name, err)
		}
		wg.Add(1)
		log := log.WithField("group", tg.Name)
		name := tg.Name
		go func() {
			defer wg.Done()
			attrs, err := client.Stat(ctx, *tgp)
			lock.Lock()
			defer lock.Unlock()
			switch {
			case err == storage.ErrObjectNotExist:
				log.Info("No existing grid state")
				generations[name] = 0
			case err != nil:
				log.WithError(err).Warning("Cannot determine group update time")
				generations[name] = -1 // Should always fail
			default:
				updated[name] = attrs.Updated
				generations[name] = attrs.Generation
				log.WithField("updated", attrs.Updated).Debug("Found updated time")
			}
		}()
	}
	wg.Wait()

	sort.SliceStable(groups, func(i, j int) bool {
		return !updated[groups[i].Name].After(updated[groups[j].Name])
	})
	n := len(groups) - 1
	if n > 0 {
		log.WithFields(logrus.Fields{
			"newest-name": groups[n].Name,
			"newest":      updated[groups[n].Name],
			"oldest-name": groups[0].Name,
			"oldest":      updated[groups[0].Name],
		}).Info("Sorted")
	}
	return generations, nil
}

// lockGroup makes a conditional GCS write operation to ensure it has authority to update this object.
//
// This allows multiple decentralized updaters to collaborate on updating groups:
// Regardless of how many updaters are trying to concurrently update an object foo at generation X, GCS
// will only allow one of them to "win". The others receive a PreconditionFailed error and can
// move onto the next group.
func lockGroup(ctx context.Context, client gcs.ConditionalClient, path gcs.Path, generation int64) error {
	var cond storage.Conditions
	if generation != 0 {
		// Attempt to cloud-copy the object to its current location
		// - only 1 will win in a concurrent situation
		// - Increases the last update time.
		cond.GenerationMatch = generation
		return client.If(&cond, &cond).Copy(ctx, path, path)
	}

	// New group, create an empty grid for it.
	cond.DoesNotExist = true
	var grid statepb.Grid
	buf, err := marshalGrid(&grid)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return client.If(&cond, &cond).Upload(ctx, path, buf, gcs.DefaultACL, "no-cache")
}

// Update performs a single update pass of all all test groups specified by the config.
func Update(parent context.Context, client gcs.ConditionalClient, configPath gcs.Path, gridPrefix string, groupConcurrency int, group string, updateGroup GroupUpdater, write bool) error {
	defer growMaxUpdateArea()
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
	defer wg.Wait()
	defer close(groups)

	var generations map[string]int64

	for i := 0; i < groupConcurrency; i++ {
		wg.Add(1)
		go func() {
			for tg := range groups {
				log := log.WithField("group", tg.Name)
				log.Debug("Starting update")
				tgp, err := testGroupPath(configPath, gridPrefix, tg.Name)
				if err != nil {
					log.WithError(err).Error("Bad path")
					continue
				}
				if write && generations != nil {
					if err := lockGroup(ctx, client, *tgp, generations[tg.Name]); err != nil {
						switch ee := err.(type) {
						case *googleapi.Error:
							if ee.Code == http.StatusPreconditionFailed {
								log.Debug("Lost the lock race")
								continue
							}
						}
						log.WithError(err).Warning("Failed to acquire lock")
						continue
					} else {
						log.Debug("Acquired update lock")
					}
				}
				if err := updateGroup(ctx, log, client, &tg, *tgp); err != nil {
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
		log.Info("Sorting groups")
		generations, err = sortGroups(ctx, log, client, configPath, gridPrefix, cfg.TestGroups)
		if err != nil {
			log.WithError(err).Warning("Failed to sort groups")
		}
		log.Info("Sorted")
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
	return nil
}

// testGroupPath() returns the path to a test_group proto given this proto
func testGroupPath(g gcs.Path, gridPrefix, groupName string) (*gcs.Path, error) {
	name := path.Join(gridPrefix, groupName)
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

// AllowMultiplePaths enables combining multiple jobs together using a comma.
//
// This feature is undocumented and poorly tested, so restrict to specific groups.
// TODO(fejta): redesign this feature (using symlinks?), ensure it works correctly.
var AllowMultiplePaths = map[string]bool{}

func groupPaths(tg *configpb.TestGroup) ([]gcs.Path, error) {
	var out []gcs.Path
	prefixes := strings.Split(tg.GcsPrefix, ",")
	if len(prefixes) > 1 && !AllowMultiplePaths[tg.Name] {
		return nil, fmt.Errorf("Maximum of one GCS path allowed")
	}
	for idx, prefix := range prefixes {
		prefix := strings.TrimSpace(prefix)
		if prefix == "" {
			continue
		}
		u, err := url.Parse("gs://" + prefix)
		if err != nil {
			return nil, fmt.Errorf("parse: %w", err)
		}
		if u.Path != "" && u.Path[len(u.Path)-1] != '/' {
			u.Path += "/"
		}

		var p gcs.Path
		if err := p.SetURL(u); err != nil {
			if idx > 0 {
				return nil, fmt.Errorf("%d: %s: %w", idx, prefix, err)
			}
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// truncateRunning filters out all columns until the oldest still running column.
//
// If there are 20 columns where all are complete except the 3rd and 7th, this will
// return the 8th and later columns.
func truncateRunning(cols []InflatedColumn) []InflatedColumn {
	if len(cols) == 0 {
		return cols
	}
	var stillRunning int
	for i, c := range cols {
		if c.Cells[overallRow].Result == statuspb.TestStatus_RUNNING {
			stillRunning = i + 1
		}
	}
	return cols[stillRunning:]
}

var (
	maxUpdateArea  int = 20000
	updateAreaLock sync.RWMutex
)

const maxMaxUpdateArea = 1000000
const initialCols = 5

// growMaxUpdateArea allows testgrid to increase the update area size over time.
//
// This allows the potential for larger, faster updates when the system is stable
// While also falling back to a slower, more stable mode after a crash (potentially
// caused by too large an update)
func growMaxUpdateArea() {
	updateAreaLock.RLock()
	cur := maxUpdateArea
	updateAreaLock.RUnlock()
	if cur >= maxMaxUpdateArea {
		return
	}
	updateAreaLock.Lock()
	maxUpdateArea *= 2
	updateAreaLock.Unlock()
}

func truncateBuilds(log logrus.FieldLogger, builds []gcs.Build, cols []inflatedColumn) []gcs.Build {
	// determine the average number of rows per column
	var rows int
	for _, c := range cols {
		rows += len(c.Cells)
	}

	nc := len(cols)
	if nc == 0 {
		nc = 1
	}
	rows /= nc
	updateAreaLock.RLock()
	updateArea := maxUpdateArea
	updateAreaLock.RUnlock()
	if rows == 0 {
		rows = updateArea / initialCols
	}

	nCols := updateArea / rows
	if nCols == 0 {
		nCols = 1 // At least one column
	}
	if n := len(builds); n > nCols {
		log.WithFields(logrus.Fields{
			"from":    n,
			"to":      nCols,
			"delayed": n - nCols,
			"old":     len(cols),
		}).Info("Trucated update")
		return builds[n-nCols : n]
	}
	return builds
}

func listBuilds(ctx context.Context, client gcs.Lister, since string, paths ...gcs.Path) ([]gcs.Build, error) {
	var out []gcs.Build

	for idx, tgPath := range paths {
		var offset *gcs.Path
		var err error
		if since != "" {
			if offset, err = tgPath.ResolveReference(&url.URL{Path: since}); err != nil {
				return nil, fmt.Errorf("resolve since: %w", err)
			}
		}
		builds, err := gcs.ListBuilds(ctx, client, tgPath, offset)
		if err != nil {
			return nil, fmt.Errorf("%d: %s: %w", idx, tgPath, err)
		}
		out = append(out, builds...)
	}

	if len(paths) > 0 {
		gcs.Sort(out)
	}

	return out, nil
}

// A ColumnReader will find, process and return new columns to insert into the front of grid state.
type ColumnReader func(ctx context.Context, log logrus.FieldLogger, tg *configpb.TestGroup, oldCols []inflatedColumn, stop time.Time) ([]inflatedColumn, error)

// InflateDropAppend updates groups by downloading the existing grid, dropping old rows and appending new ones.
func InflateDropAppend(ctx context.Context, log logrus.FieldLogger, client gcs.Client, tg *configpb.TestGroup, gridPath gcs.Path, write bool, readCols ColumnReader) error {
	var dur time.Duration
	if tg.DaysOfResults > 0 {
		dur = days(float64(tg.DaysOfResults))
	} else {
		dur = days(7)
	}

	stop := time.Now().Add(-dur)

	var oldCols []inflatedColumn

	old, err := gcs.DownloadGrid(ctx, client, gridPath)
	if err != nil {
		log.WithField("path", gridPath).WithError(err).Error("Failed to download existing grid")
	}
	if old != nil {
		oldCols = truncateRunning(inflateGrid(old, stop, time.Now().Add(-12*time.Hour)))
	}

	newCols, err := readCols(ctx, log, tg, oldCols, stop)
	if err != nil {
		return fmt.Errorf("read columns: %w", err)
	}

	cols := mergeColumns(newCols, oldCols)

	grid := constructGrid(log, tg, cols)
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
		if err := client.Upload(ctx, gridPath, buf, gcs.DefaultACL, "no-cache"); err != nil {
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
func mergeColumns(newCols, oldCols []InflatedColumn) []InflatedColumn {
	// accept all the new columns
	out := append([]InflatedColumn{}, newCols...)
	if len(out) == 0 {
		return oldCols
	}

	// accept all the old columns which are older than the accepted columns.
	oldestCol := out[len(out)-1].Column
	for i := 0; i < len(oldCols); i++ {
		if oldCols[i].Column.Started > oldestCol.Started || oldCols[i].Column.Build == oldestCol.Build {
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
func constructGrid(log logrus.FieldLogger, group *configpb.TestGroup, cols []inflatedColumn) *statepb.Grid {
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
	}

	dropEmptyRows(log, &grid, rows)

	alertRows(grid.Columns, grid.Rows, failsOpen, passesClose)
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
	return &grid
}

func dropEmptyRows(log logrus.FieldLogger, grid *statepb.Grid, rows map[string]*statepb.Row) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	filled := make([]*statepb.Row, 0, len(rows))
	var dropped int
	for _, r := range grid.Rows {
		var found bool
		for res := range result.Iter(ctx, r.Results) {
			if res == statuspb.TestStatus_NO_RESULT {
				continue
			}
			found = true
			break
		}
		if !found {
			dropped++
			delete(rows, r.Name)
			continue
		}
		filled = append(filled, r)
	}

	if dropped == 0 {
		return
	}

	grid.Rows = filled
	log.WithField("dropped", dropped).Info("Dropped old rows")
}

// marhshalGrid serializes a state proto into zlib-compressed bytes.
func marshalGrid(grid *statepb.Grid) ([]byte, error) {
	buf, err := proto.Marshal(grid)
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

var emptyCell = Cell{Result: statuspb.TestStatus_NO_RESULT}

func hasCellID(name string) bool {
	return !strings.Contains(name, "@TESTGRID@")
}

// appendCell adds the rowResult column to the row.
//
// Handles the details like missing fields and run-length-encoding the result.
func appendCell(row *statepb.Row, cell cell, start, count int) {
	latest := int32(cell.Result)
	n := len(row.Results)
	switch {
	case n == 0, row.Results[n-2] != latest:
		row.Results = append(row.Results, latest, int32(count))
	default:
		row.Results[n-1] += int32(count)
	}

	addCellID := hasCellID(row.Name)

	for i := 0; i < count; i++ {
		columnIdx := int32(start + i)
		for metricName, measurement := range cell.Metrics {
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
			appendMetric(metric, columnIdx, measurement)
		}
		if cell.Result == statuspb.TestStatus_NO_RESULT {
			continue
		}
		if addCellID {
			row.CellIds = append(row.CellIds, cell.CellID)
		}
		// Javascript client expects no result cells to skip icons/messages
		row.Messages = append(row.Messages, cell.Message)
		row.Icons = append(row.Icons, cell.Icon)
	}
}

// appendColumn adds the build column to the grid.
//
// This handles details like:
// * rows appearing/disappearing in the middle of the run.
// * adding auto metadata like duration, commit as well as any user-added metadata
// * extracting build metadata into the appropriate column header
// * Ensuring row names are unique and formatted with metadata
func appendColumn(grid *statepb.Grid, rows map[string]*statepb.Row, inflated InflatedColumn) {
	grid.Columns = append(grid.Columns, inflated.Column)
	colIdx := len(grid.Columns) - 1

	missing := map[string]*statepb.Row{}
	for name, row := range rows {
		missing[name] = row
	}

	for name, cell := range inflated.Cells {
		delete(missing, name)

		row, ok := rows[name]
		if !ok {
			id := cell.ID
			if id == "" {
				id = name
			}
			row = &statepb.Row{
				Name:    name,
				Id:      id,
				CellIds: []string{}, // TODO(fejta): try and leave this nil
			}
			rows[name] = row
			grid.Rows = append(grid.Rows, row)
			if colIdx > 0 {
				appendCell(row, emptyCell, 0, colIdx)
			}
		}
		appendCell(row, cell, colIdx, 1)
	}

	for _, row := range missing {
		appendCell(row, emptyCell, colIdx, 1)
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
	var firstFail *statepb.Column
	var latestFail *statepb.Column
	var latestPass *statepb.Column
	var failIdx int
	var latestFailIdx int
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
				latestFailIdx = compressedIdx
				latestFail = col
			}
			failIdx = compressedIdx
			firstFail = col
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
	var id string
	var latestID string
	if len(row.CellIds) > 0 { // not all rows have cell ids
		id = row.CellIds[failIdx]
		latestID = row.CellIds[latestFailIdx]
	}
	msg := row.Messages[latestFailIdx]
	return alertInfo(totalFailures, msg, id, latestID, firstFail, latestFail, latestPass)
}

// alertInfo returns an alert proto with the configured fields
func alertInfo(failures int32, msg, cellID, latestCellID string, fail, latestFail, pass *statepb.Column) *statepb.AlertInfo {
	return &statepb.AlertInfo{
		FailCount:         failures,
		FailBuildId:       buildID(fail),
		LatestFailBuildId: buildID(latestFail),
		FailTime:          stamp(fail),
		FailTestId:        cellID,
		LatestFailTestId:  latestCellID,
		FailureMessage:    msg,
		PassTime:          stamp(pass),
		PassBuildId:       buildID(pass),
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
