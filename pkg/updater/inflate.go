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
	"time"

	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	statuspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
)

// InflatedColumn holds all the entries for a given column.
//
// This includes both:
// * Column state metadata and
// * Cell values for every row in this column
type InflatedColumn struct {
	// Column holds the header data.
	Column *statepb.Column
	// Cells holds each row's uncompressed data for this column.
	Cells map[string]Cell
}

// Cell holds a row's values for a given column
type Cell struct {
	// Result determines the color of the cell, defaulting to NO_RESULT (clear)
	Result statuspb.TestStatus

	// The name of the row before user-customized formatting
	ID string

	// CellID specifies the an identifier to the build, which allows
	// clicking different cells in a column to go to different locations.
	CellID string

	// Icon is a short string that appears on the cell
	Icon string
	// Message is a longer string that appears on mouse-over
	Message string

	// Metrics holds numerical data, such as how long it ran, coverage, etc.
	Metrics map[string]float64

	// UserProperty holds the value of a user-defined property, which allows
	// runtime flexibility in generating links to click on.
	UserProperty string

	// Issues relevant to this cell
	// TODO(fejta): persist cell association, currently gets written out as a row-association.
	// TODO(fejta): support issue association when parsing prow job results.
	Issues []string
}

// inflateGrid inflates the grid's rows into an InflatedColumn channel.
func inflateGrid(grid *statepb.Grid, earliest, latest time.Time) ([]InflatedColumn, map[string][]string) {
	var cols []InflatedColumn

	// nothing is blocking, so no need for a parent context.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rows := make(map[string]<-chan Cell, len(grid.Rows))
	issues := make(map[string][]string, len(grid.Rows))
	for _, row := range grid.Rows {
		rows[row.Name] = inflateRow(ctx, row)
		if len(row.Issues) > 0 {
			issues[row.Name] = row.Issues
		}
	}

	for _, col := range grid.Columns {
		// Even if we wind up skipping the column
		// we still need to inflate the cells.
		item := InflatedColumn{
			Column: col,
			Cells:  make(map[string]Cell, len(rows)),
		}
		if col.Hint == "" { // TODO(fejta): drop after everything sets its hint.
			col.Hint = col.Build
		}
		for rowName, rowCells := range rows {
			item.Cells[rowName] = <-rowCells
		}
		when := int64(col.Started / 1000)
		if when > latest.Unix() {
			continue
		}
		if when < earliest.Unix() && len(cols) > 0 { // Always keep at least one old column
			continue // Do not assume they are sorted by start time.
		}
		cols = append(cols, item)

	}
	return cols, issues
}

// inflateRow inflates the values for each column into a Cell channel.
func inflateRow(parent context.Context, row *statepb.Row) <-chan Cell {
	out := make(chan Cell)
	addCellID := hasCellID(row.Name)

	go func() {
		ctx, cancel := context.WithCancel(parent)
		defer close(out)
		defer cancel()
		var filledIdx int
		Metrics := map[string]<-chan *float64{}
		for i, m := range row.Metrics {
			if m.Name == "" && len(row.Metrics) > i {
				m.Name = row.Metric[i]
			}
			Metrics[m.Name] = inflateMetric(ctx, m)
		}
		var val *float64
		for result := range inflateResults(ctx, row.Results) {
			c := Cell{Result: result}
			for name, ch := range Metrics {
				select {
				case <-ctx.Done():
					return
				case val = <-ch:
				}
				if val == nil {
					continue
				}
				if c.Metrics == nil {
					c.Metrics = map[string]float64{}
				}
				c.Metrics[name] = *val
			}
			if result != statuspb.TestStatus_NO_RESULT {
				c.Icon = row.Icons[filledIdx]
				c.Message = row.Messages[filledIdx]
				if addCellID {
					c.CellID = row.CellIds[filledIdx]
				}
				if n := len(row.UserProperty); n > filledIdx {
					c.UserProperty = row.UserProperty[filledIdx]
				}
				filledIdx++
			}
			select {
			case <-ctx.Done():
				return
			case out <- c:
			}
		}

	}()
	return out
}

// inflateMetric inflates the sparse-encoded metric values into a channel
func inflateMetric(ctx context.Context, metric *statepb.Metric) <-chan *float64 {
	out := make(chan *float64)
	go func() {
		defer close(out)
		var current int32
		var valueIdx int
		for i := 0; i < len(metric.Indices); i++ {
			start := metric.Indices[i]
			i++
			remain := metric.Indices[i]
			for ; remain > 0; current++ {
				if current < start {
					select {
					case <-ctx.Done():
						return
					case out <- nil:
					}
					continue
				}
				remain--
				value := metric.Values[valueIdx]
				valueIdx++
				select {
				case <-ctx.Done():
					return
				case out <- &value:
				}
			}
		}
	}()
	return out
}

// inflateResults inflates the run-length encoded row results into a channel.
func inflateResults(ctx context.Context, results []int32) <-chan statuspb.TestStatus {
	out := make(chan statuspb.TestStatus)
	go func() {
		defer close(out)
		for idx := 0; idx < len(results); idx++ {
			val := results[idx]
			idx++
			for n := results[idx]; n > 0; n-- {
				select {
				case <-ctx.Done():
					return
				case out <- statuspb.TestStatus(val):
				}
			}
		}
	}()
	return out
}
