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

// inflatedColumn holds all the entries for a given column.
//
// This includes both:
// * Column state metadata and
// * cell values for every row in this column
type inflatedColumn struct {
	column *statepb.Column
	cells  map[string]cell
}

// cell holds a row's values for a given column
type cell struct {
	result statuspb.TestStatus

	cellID string

	icon    string
	message string

	metrics map[string]float64
}

// inflateGrid inflates the grid's rows into an inflatedColumn channel.
func inflateGrid(grid *statepb.Grid, earliest, latest time.Time) []inflatedColumn {
	var cols []inflatedColumn

	// nothing is blocking, so no need for a parent context.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rows := make(map[string]<-chan cell, len(grid.Rows))
	for _, row := range grid.Rows {
		rows[row.Name] = inflateRow(ctx, row)
	}

	for _, col := range grid.Columns {
		// Even if we wind up skipping the column
		// we still need to inflate the cells.
		item := inflatedColumn{
			column: col,
			cells:  make(map[string]cell, len(rows)),
		}
		for rowName, rowCells := range rows {
			item.cells[rowName] = <-rowCells
		}
		when := int64(col.Started / 1000)
		if when > latest.Unix() {
			continue
		}
		if when < earliest.Unix() {
			break
		}
		cols = append(cols, item)

	}
	return cols
}

// inflateRow inflates the values for each column into a cell channel.
func inflateRow(parent context.Context, row *statepb.Row) <-chan cell {
	out := make(chan cell)

	go func() {
		ctx, cancel := context.WithCancel(parent)
		defer close(out)
		defer cancel()
		var filledIdx int
		var cellIdx int
		metrics := map[string]<-chan *float64{}
		for i, m := range row.Metrics {
			if m.Name == "" && len(row.Metrics) > i {
				m.Name = row.Metric[i]
			}
			metrics[m.Name] = inflateMetric(ctx, m)
		}
		var val *float64
		for result := range inflateResults(ctx, row.Results) {
			c := cell{
				cellID: row.CellIds[cellIdx],
				result: result,
			}
			cellIdx++
			for name, ch := range metrics {
				select {
				case <-ctx.Done():
					return
				case val = <-ch:
				}
				if val == nil {
					continue
				}
				if c.metrics == nil {
					c.metrics = map[string]float64{}
				}
				c.metrics[name] = *val
			}
			if result != statuspb.TestStatus_NO_RESULT {
				c.icon = row.Icons[filledIdx]
				c.message = row.Messages[filledIdx]
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
		// TODO(fejta): ugh? this might be wrong
		// I believe we may need to ignore NO_RESULT columns.
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
