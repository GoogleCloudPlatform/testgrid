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
	"fmt"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/metadata"
	"github.com/GoogleCloudPlatform/testgrid/metadata/junit"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	statuspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
)

// gcsResult holds all the downloaded information for a build of a job.
//
// The suite results become rows and the job metadata is added to the column.
type gcsResult struct {
	started  gcs.Started
	finished gcs.Finished
	suites   []gcs.SuitesMeta
}

// convertResult returns an inflatedColumn representation of the GCS result.
func convertResult(nameCfg nameConfig, id string, headers []string, result gcsResult) inflatedColumn {
	overall := overallCell(result)
	out := inflatedColumn{
		column: &statepb.Column{
			Build:   id,
			Started: float64(result.started.Timestamp * 1000),
		},
		cells: map[string]cell{
			"Overall": overall,
		},
	}

	meta := result.finished.Metadata.Strings()
	version := metadata.Version(result.started.Started, result.finished.Finished)

	for _, h := range headers {
		val, ok := meta[h]
		if !ok && h == "Commit" && version != metadata.Missing {
			val = version
		} else if !ok && overall.result != statuspb.TestStatus_RUNNING {
			val = "missing"
		}
		out.column.Extra = append(out.column.Extra, val)
	}

	// Append each result into the column
	for _, suite := range result.suites {
		for _, r := range flattenResults(suite.Suites.Suites...) {
			if r.Skipped != nil && *r.Skipped == "" {
				continue
			}
			var c cell
			// TODO(fejta): process properties?
			if elapsed := r.Time; elapsed > 0 {
				c.metrics = setElapsed(c.metrics, elapsed)
			}

			const max = 140
			if msg := r.Message(max); msg != "" {
				c.message = msg
			}

			switch {
			case r.Failure != nil:
				c.result = statuspb.TestStatus_FAIL
				if c.message != "" {
					c.icon = "F"
				}
			case r.Skipped != nil:
				c.result = statuspb.TestStatus_PASS_WITH_SKIPS
				c.icon = "S"
			default:
				c.result = statuspb.TestStatus_PASS
			}

			parsed := make([]interface{}, len(nameCfg.parts))
			for i, p := range nameCfg.parts {
				if p == "Tests name" {
					parsed[i] = r.Name
					continue
				}
				v, present := suite.Metadata[p]
				if present {
					parsed[i] = v
					continue
				}
				parsed[i] = meta[p]
			}
			name := fmt.Sprintf(nameCfg.format, parsed...)

			// Ensure each name is unique
			// If we have multiple results with the same name foo
			// then append " [n]" to the name so we wind up with:
			//   foo
			//   foo [1]
			//   foo [2]
			//   etc
			if _, present := out.cells[name]; present {
				for idx := 1; true; idx++ {
					attempt := fmt.Sprintf("%s [%d]", name, idx)
					if _, present := out.cells[attempt]; present {
						continue
					}
					name = attempt
					break
				}
			}
			out.cells[name] = c
		}
	}

	if overall.result == statuspb.TestStatus_FAIL && overall.message == "" { // Ensure failing build has a failing cell and/or overall message
		var found bool
		for n, c := range out.cells {
			if n == "Overall" {
				continue
			}
			if c.result == statuspb.TestStatus_FAIL {
				found = true // Failing test, huzzah!
				break
			}
		}
		if !found { // Nope, add the F icon and an explanatory message
			overall := out.cells["Overall"]
			overall.icon = "F"
			overall.message = "Build failed outside of test results"
			out.cells["Overall"] = overall
		}
	}

	return out
}

// overallCell generates the overall cell for this GCS result.
func overallCell(result gcsResult) cell {
	var c cell
	var finished int64
	if result.finished.Timestamp != nil {
		finished = *result.finished.Timestamp
	}
	switch {
	case finished > 0: // completed result
		if result.finished.Passed != nil && *result.finished.Passed {
			c.result = statuspb.TestStatus_PASS
		} else {
			c.result = statuspb.TestStatus_FAIL
		}
		c.metrics = setElapsed(nil, float64(finished-result.started.Timestamp))
	case time.Now().Add(-24*time.Hour).Unix() > result.started.Timestamp:
		c.result = statuspb.TestStatus_FAIL
		c.message = "Build did not complete within 24 hours"
		c.icon = "T"
	default:
		c.result = statuspb.TestStatus_RUNNING
		c.message = "Build still running..."
		c.icon = "R"
	}
	return c
}

const elapsedKey = "seconds-elapsed"

// setElapsed inserts the seconds-elapsed metric.
func setElapsed(metrics map[string]float64, seconds float64) map[string]float64 {
	if metrics == nil {
		metrics = map[string]float64{}
	}
	metrics[elapsedKey] = seconds
	return metrics
}

// flattenResults returns the DFS of all junit results in all suites.
func flattenResults(suites ...junit.Suite) []junit.Result {
	var results []junit.Result
	for _, suite := range suites {
		for _, innerSuite := range suite.Suites {
			innerSuite.Name = dotName(suite.Name, innerSuite.Name)
			results = append(results, flattenResults(innerSuite)...)
		}
		for _, r := range suite.Results {
			r.Name = dotName(suite.Name, r.Name)
			results = append(results, r)
		}
	}
	return results
}

// dotName returns left.right or left or right
func dotName(left, right string) string {
	if left != "" && right != "" {
		return left + "." + right
	}
	if right == "" {
		return left
	}
	return right
}
