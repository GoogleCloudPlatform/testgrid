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

	// Properties maps key:value pairs for cell IDs.
	Properties map[string]string

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

// InflateGrid inflates the grid's rows into an InflatedColumn channel.
//
// Drops columns before earliest or more recent than latest.
// Also returns a map of issues associated with each row name.
func InflateGrid(ctx context.Context, grid *statepb.Grid, earliest, latest time.Time) ([]InflatedColumn, map[string][]string, error) {
	var cols []InflatedColumn
	if n := len(grid.Columns); n > 0 {
		cols = make([]InflatedColumn, 0, n)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	rows := make(map[string]func() *Cell, len(grid.Rows))
	issues := make(map[string][]string, len(grid.Rows))
	for _, row := range grid.Rows {
		rows[row.Name] = inflateRow(row)
		if len(row.Issues) > 0 {
			issues[row.Name] = row.Issues
		}
	}

	for _, col := range grid.Columns {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		// Even if we wind up skipping the column
		// we still need to inflate the cells.
		item := InflatedColumn{
			Column: col,
			Cells:  make(map[string]Cell, len(rows)),
		}
		if col.Hint == "" { // TODO(fejta): drop after everything sets its hint.
			col.Hint = col.Build
		}
		for rowName, nextCell := range rows {
			cell := nextCell()
			if cell != nil {
				item.Cells[rowName] = *cell
			}
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
	return cols, issues, nil
}

// inflateRow inflates the values for each column into a Cell channel.
func inflateRow(row *statepb.Row) func() *Cell {
	if row == nil {
		return func() *Cell { return nil }
	}
	addCellID := hasCellID(row.Name)

	var filledIdx int
	var mets map[string]func() (*float64, bool)
	if len(row.Metrics) > 0 {
		mets = make(map[string]func() (*float64, bool), len(row.Metrics))
	}
	for i, m := range row.Metrics {
		if m.Name == "" && len(row.Metrics) > i {
			m.Name = row.Metric[i]
		}
		mets[m.Name] = inflateMetric(m)
	}
	var val *float64
	nextResult := inflateResults(row.Results)
	return func() *Cell {
		for cur := nextResult(); cur != nil; cur = nextResult() {
			result := *cur
			c := Cell{
				Result: result,
				ID:     row.Id,
			}
			for name, nextValue := range mets {
				val, _ = nextValue()
				if val == nil {
					continue
				}
				if c.Metrics == nil {
					c.Metrics = make(map[string]float64, 2)
				}
				c.Metrics[name] = *val
			}
			// TODO(fejta): consider returning (nil, true) instead here
			if result != statuspb.TestStatus_NO_RESULT {
				c.Icon = row.Icons[filledIdx]
				c.Message = row.Messages[filledIdx]
				if addCellID {
					c.CellID = row.CellIds[filledIdx]
				}
				if len(row.Properties) != 0 && c.Properties == nil {
					c.Properties = make(map[string]string)
				}
				if filledIdx < len(row.Properties) {
					for k, v := range row.GetProperties()[filledIdx].GetProperty() {
						c.Properties[k] = v
					}
				}
				if n := len(row.UserProperty); n > filledIdx {
					c.UserProperty = row.UserProperty[filledIdx]
				}
				filledIdx++
			}
			return &c
		}
		return nil
	}
}

// inflateMetric inflates the sparse-encoded metric values into a channel
//
// {Indices: [0,2,6,4], Values: {0.1, 0.2, 6.1, 6.2, 6.3, 6.4}} encodes:
// {0.1, 0.2, nil, nil, nil, nil, 6.1, 6.2, 6.3, 6.4}
func inflateMetric(metric *statepb.Metric) func() (*float64, bool) {
	idx := -1
	var remain int32
	valueIdx := -1
	current := int32(-1)
	var start int32
	more := true
	return func() (*float64, bool) {
		if !more {
			return nil, false
		}
		for {
			if remain > 0 {
				current++
				if current < start {
					return nil, true
				}
				remain--
				valueIdx++
				if valueIdx == len(metric.Values) {
					break
				}
				v := metric.Values[valueIdx]
				return &v, true
			}
			idx++
			if idx >= len(metric.Indices)-1 {
				break
			}
			start = metric.Indices[idx]
			idx++
			remain = metric.Indices[idx]
		}
		more = false
		return nil, false
	}
}

// inflateResults inflates the run-length encoded row results into a channel.
//
// [PASS, 2, NO_RESULT, 1, FAIL, 3] is equivalent to:
// [PASS, PASS, NO_RESULT, FAIL, FAIL, FAIL]
func inflateResults(results []int32) func() *statuspb.TestStatus {
	idx := -1
	var current statuspb.TestStatus
	var remain int32
	more := true
	return func() *statuspb.TestStatus {
		if !more {
			return nil
		}
		for {
			if remain > 0 {
				remain--
				return &current
			}
			idx++
			if idx == len(results) {
				break
			}
			current = statuspb.TestStatus(results[idx])
			idx++
			remain = results[idx]
		}
		more = false
		return nil
	}
}
