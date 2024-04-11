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
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/testgrid/metadata"
	"github.com/GoogleCloudPlatform/testgrid/metadata/junit"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	statuspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs/fake"
	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/testing/protocmp"
	core "k8s.io/api/core/v1"
)

type fakeObject = fake.Object
type fakeClient = fake.Client
type fakeIterator = fake.Iterator

func TestDownloadGrid(t *testing.T) {
	cases := []struct {
		name string
	}{
		{},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
		})
	}
}

func resolveOrDie(path *gcs.Path, s string) *gcs.Path {
	p, err := path.ResolveReference(&url.URL{Path: s})
	if err != nil {
		panic(err)
	}
	return p
}

func jsonData(i interface{}) string {
	buf, err := json.Marshal(i)
	if err != nil {
		panic(err)
	}
	return string(buf)
}

func xmlData(i interface{}) string {
	buf, err := xml.Marshal(i)
	if err != nil {
		panic(err)
	}
	return string(buf)
}

func makeJunit(passed, failed []string) string {
	var suite junit.Suite
	for _, name := range passed {
		suite.Results = append(suite.Results, junit.Result{Name: name})
	}

	for _, name := range failed {
		f := name
		suite.Results = append(suite.Results, junit.Result{
			Name:    name,
			Failure: &junit.Failure{Value: f},
		})
	}
	return xmlData(suite)
}

func pint64(n int64) *int64 {
	return &n
}

func TestHintStarted(t *testing.T) {
	cases := []struct {
		name string
		cols []InflatedColumn
		want string
	}{
		{
			name: "basic",
		},
		{
			name: "ordered",
			cols: []InflatedColumn{
				{
					Column: &statepb.Column{
						Hint:    "b",
						Started: 1200,
					},
				},
				{
					Column: &statepb.Column{
						Hint:    "a",
						Started: 1100,
					},
				},
			},
			want: "b",
		},
		{
			name: "reversed",
			cols: []InflatedColumn{
				{
					Column: &statepb.Column{
						Hint:    "a",
						Started: 1100,
					},
				},
				{
					Column: &statepb.Column{
						Hint:    "b",
						Started: 1200,
					},
				},
			},
			want: "b",
		},
		{
			name: "different", // hint and started come from diff cols
			cols: []InflatedColumn{
				{
					Column: &statepb.Column{
						Hint:    "a",
						Started: 1100,
					},
				},
				{
					Column: &statepb.Column{
						Hint:    "b",
						Started: 900,
					},
				},
			},
			want: "b",
		},
		{
			name: "numerical", // hint10 > hint2
			cols: []InflatedColumn{
				{
					Column: &statepb.Column{
						Hint: "hint2",
					},
				},
				{
					Column: &statepb.Column{
						Hint: "hint10",
					},
				},
			},
			want: "hint10",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hintStarted(tc.cols)
			if tc.want != got {
				t.Errorf("hintStarted() got hint %q, want %q", got, tc.want)
			}
		})
	}
}

func pstr(s string) *string { return &s }

func TestReadColumns(t *testing.T) {
	now := time.Now().Unix()
	yes := true
	var no bool
	var noStartErr *noStartError
	cases := []struct {
		name               string
		ctx                context.Context
		builds             []fakeBuild
		group              *configpb.TestGroup
		stop               time.Time
		dur                time.Duration
		concurrency        int
		readResultOverride *resultReader
		enableIgnoreSkip   bool

		expected []InflatedColumn
		err      bool
	}{
		{
			name: "basically works",
		},
		{
			name: "convert results correctly",
			builds: []fakeBuild{
				{
					id: "11",
					started: &fakeObject{
						Data: jsonData(metadata.Started{Timestamp: now + 11}),
					},
					finished: &fakeObject{
						Data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 22),
							Passed:    &no,
						}),
					},
					podInfo: podInfoSuccess,
				},
				{
					id: "10",
					started: &fakeObject{
						Data: jsonData(metadata.Started{Timestamp: now + 10}),
					},
					finished: &fakeObject{
						Data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 20),
							Passed:    &yes,
						}),
					},
				},
			},
			group: &configpb.TestGroup{
				GcsPrefix: "bucket/path/to/build/",
			},
			expected: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "10",
						Hint:    "10",
						Started: float64(now+10) * 1000,
					},
					Cells: map[string]cell{
						"build." + overallRow: {
							Result: statuspb.TestStatus_PASS,
							Metrics: map[string]float64{
								"test-duration-minutes": 10 / 60.0,
							},
						},
						"build." + podInfoRow: podInfoMissingCell,
					},
				},
				{
					Column: &statepb.Column{
						Build:   "11",
						Hint:    "11",
						Started: float64(now+11) * 1000,
					},
					Cells: map[string]cell{
						"build." + overallRow: {
							Result:  statuspb.TestStatus_FAIL,
							Icon:    "F",
							Message: "Build failed outside of test results",
							Metrics: map[string]float64{
								"test-duration-minutes": 11 / 60.0,
							},
						},
						"build." + podInfoRow: podInfoPassCell,
					},
				},
			},
		},
		{
			name: "column headers processed correctly",
			builds: []fakeBuild{
				{
					id: "11",
					started: &fakeObject{
						Data: jsonData(metadata.Started{Timestamp: now + 11}),
					},
					finished: &fakeObject{
						Data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 22),
							Passed:    &no,
							Metadata: metadata.Metadata{
								metadata.JobVersion: "v0.0.0-alpha.0+build11",
								"random":            "new information",
							},
						}),
					},
					podInfo: podInfoSuccess,
				},
				{
					id: "10",
					started: &fakeObject{
						Data: jsonData(metadata.Started{Timestamp: now + 10}),
					},
					finished: &fakeObject{
						Data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 20),
							Passed:    &yes,
							Metadata: metadata.Metadata{
								metadata.JobVersion: "v0.0.0-alpha.0+build10",
								"random":            "old information",
							},
						}),
					},
				},
			},
			group: &configpb.TestGroup{
				GcsPrefix: "bucket/path/to/build/",
				ColumnHeader: []*configpb.TestGroup_ColumnHeader{
					{
						ConfigurationValue: "Commit",
					},
					{
						ConfigurationValue: "random",
					},
				},
			},
			expected: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "10",
						Hint:    "10",
						Started: float64(now+10) * 1000,
						Extra: []string{
							"build10",
							"old information",
						},
					},
					Cells: map[string]cell{
						"build." + overallRow: {
							Result: statuspb.TestStatus_PASS,
							Metrics: map[string]float64{
								"test-duration-minutes": 10 / 60.0,
							},
						},
						"build." + podInfoRow: podInfoMissingCell,
					},
				},
				{
					Column: &statepb.Column{
						Build:   "11",
						Hint:    "11",
						Started: float64(now+11) * 1000,
						Extra: []string{
							"build11",
							"new information",
						},
					},
					Cells: map[string]cell{
						"build." + overallRow: {
							Result:  statuspb.TestStatus_FAIL,
							Icon:    "F",
							Message: "Build failed outside of test results",
							Metrics: map[string]float64{
								"test-duration-minutes": 11 / 60.0,
							},
						},
						"build." + podInfoRow: podInfoPassCell,
					},
				},
			},
		},
		{
			name: "name config works correctly",
			builds: []fakeBuild{
				{
					id: "10",
					started: &fakeObject{
						Data: jsonData(metadata.Started{Timestamp: now + 10}),
					},
					podInfo: podInfoSuccess,
					finished: &fakeObject{
						Data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 20),
							Passed:    &yes,
						}),
					},
					artifacts: map[string]fakeObject{
						"junit_context-a_33.xml": {
							Data: makeJunit([]string{"good"}, []string{"bad"}),
						},
						"junit_context-b_44.xml": {
							Data: makeJunit([]string{"good"}, []string{"bad"}),
						},
					},
				},
			},
			group: &configpb.TestGroup{
				GcsPrefix: "bucket/path/to/build/",
				TestNameConfig: &configpb.TestNameConfig{
					NameFormat: "name %s - context %s - thread %s",
					NameElements: []*configpb.TestNameConfig_NameElement{
						{
							TargetConfig: "Tests name",
						},
						{
							TargetConfig: "Context",
						},
						{
							TargetConfig: "Thread",
						},
					},
				},
			},
			expected: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "10",
						Hint:    "10",
						Started: float64(now+10) * 1000,
					},
					Cells: map[string]cell{
						"build." + overallRow: {
							Result: statuspb.TestStatus_PASS,
							Metrics: map[string]float64{
								"test-duration-minutes": 10 / 60.0,
							},
						},
						"build." + podInfoRow: podInfoPassCell,
						"name good - context context-a - thread 33": {
							Result: statuspb.TestStatus_PASS,
						},
						"name bad - context context-a - thread 33": {
							Result:  statuspb.TestStatus_FAIL,
							Icon:    "F",
							Message: "bad",
						},
						"name good - context context-b - thread 44": {
							Result: statuspb.TestStatus_PASS,
						},
						"name bad - context context-b - thread 44": {
							Result:  statuspb.TestStatus_FAIL,
							Icon:    "F",
							Message: "bad",
						},
					},
				},
			},
		},
		{
			name: "truncate columns after the newest old result",
			stop: time.Unix(now+13, 0), // should capture 14 and 13
			builds: []fakeBuild{
				{
					id: "14",
					started: &fakeObject{
						Data: jsonData(metadata.Started{Timestamp: now + 14}),
					},
					finished: &fakeObject{
						Data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 28),
							Passed:    &yes,
						}),
					},
					podInfo: podInfoSuccess,
				},
				{
					id: "13",
					started: &fakeObject{
						Data: jsonData(metadata.Started{Timestamp: now + 13}),
					},
					finished: &fakeObject{
						Data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 26),
							Passed:    &yes,
						}),
					},
					podInfo: podInfoSuccess,
				},
				{
					id: "12",
					started: &fakeObject{
						Data: jsonData(metadata.Started{Timestamp: now + 12}),
					},
					finished: &fakeObject{
						Data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 24),
							Passed:    &yes,
						}),
					},
				},
				{
					id: "11",
					started: &fakeObject{
						Data: jsonData(metadata.Started{Timestamp: now + 11}),
					},
					finished: &fakeObject{
						Data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 22),
							Passed:    &yes,
						}),
					},
				},
				{
					id: "10",
					started: &fakeObject{
						Data: jsonData(metadata.Started{Timestamp: now + 10}),
					},
					finished: &fakeObject{
						Data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 20),
							Passed:    &yes,
						}),
					},
				},
			},
			group: &configpb.TestGroup{
				GcsPrefix: "bucket/path/to/build/",
			},
			expected: []InflatedColumn{
				ancientColumn("10", .01, nil, fmt.Sprintf("build too old; started %v before %v)", now+10, now+13)),
				ancientColumn("11", .02, nil, fmt.Sprintf("build too old; started %v before %v)", now+11, now+13)),
				ancientColumn("12", .03, nil, fmt.Sprintf("build too old; started %v before %v)", now+12, now+13)),
				{
					Column: &statepb.Column{
						Build:   "13",
						Hint:    "13",
						Started: float64(now+13) * 1000,
					},
					Cells: map[string]cell{
						"build." + overallRow: {
							Result: statuspb.TestStatus_PASS,
							Metrics: map[string]float64{
								"test-duration-minutes": 13 / 60.0,
							},
						},
						"build." + podInfoRow: podInfoPassCell,
					},
				},
				{
					Column: &statepb.Column{
						Build:   "14",
						Hint:    "14",
						Started: float64(now+14) * 1000,
					},
					Cells: map[string]cell{
						"build." + overallRow: {
							Result: statuspb.TestStatus_PASS,
							Metrics: map[string]float64{
								"test-duration-minutes": 14 / 60.0,
							},
						},
						"build." + podInfoRow: podInfoPassCell,
					},
				},
			},
		},
		{
			name: "include no-start-time column",
			stop: time.Unix(now+13, 0), // should capture 15, 14, 13
			builds: []fakeBuild{
				{
					id: "15",
					started: &fakeObject{
						Data: jsonData(metadata.Started{Timestamp: now + 15}),
					},
					finished: &fakeObject{
						Data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 30),
							Passed:    &yes,
						}),
					},
					podInfo: podInfoSuccess,
				},
				{
					id: "14",
					started: &fakeObject{
						Data: jsonData(metadata.Started{Timestamp: 0}),
					},
					finished: &fakeObject{
						Data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 28),
							Passed:    &yes,
						}),
					},
					podInfo: podInfoSuccess,
				},
				{
					id: "13",
					started: &fakeObject{
						Data: jsonData(metadata.Started{Timestamp: now + 13}),
					},
					finished: &fakeObject{
						Data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 26),
							Passed:    &yes,
						}),
					},
					podInfo: podInfoSuccess,
				},
			},
			group: &configpb.TestGroup{
				GcsPrefix: "bucket/path/to/build/",
			},
			expected: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "13",
						Hint:    "13",
						Started: float64(now+13) * 1000,
					},
					Cells: map[string]cell{
						"build." + overallRow: {
							Result: statuspb.TestStatus_PASS,
							Metrics: map[string]float64{
								"test-duration-minutes": 13 / 60.0,
							},
						},
						"build." + podInfoRow: podInfoPassCell,
					},
				},
				noStartColumn("14", float64(now+13)*1000+0.01, nil, noStartErr.Error()), // start * 1000 + 0.01 * failures (1)
				{
					Column: &statepb.Column{
						Build:   "15",
						Hint:    "15",
						Started: float64(now+15) * 1000,
					},
					Cells: map[string]cell{
						"build." + overallRow: {
							Result: statuspb.TestStatus_PASS,
							Metrics: map[string]float64{
								"test-duration-minutes": 15 / 60.0,
							},
						},
						"build." + podInfoRow: podInfoPassCell,
					},
				},
			},
		},
		{
			name:        "high concurrency works",
			concurrency: 4,
			builds: []fakeBuild{
				{
					id: "13",
					started: &fakeObject{
						Data: jsonData(metadata.Started{Timestamp: now + 13}),
					},
					finished: &fakeObject{
						Data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 26),
							Passed:    &yes,
						}),
					},
					podInfo: podInfoSuccess,
				},
				{
					id: "12",
					started: &fakeObject{
						Data: jsonData(metadata.Started{Timestamp: now + 12}),
					},
					finished: &fakeObject{
						Data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 24),
							Passed:    &yes,
						}),
					},
				},
				{
					id: "11",
					started: &fakeObject{
						Data: jsonData(metadata.Started{Timestamp: now + 11}),
					},
					finished: &fakeObject{
						Data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 22),
							Passed:    &yes,
						}),
					},
					podInfo: podInfoSuccess,
				},
				{
					id: "10",
					started: &fakeObject{
						Data: jsonData(metadata.Started{Timestamp: now + 10}),
					},
					finished: &fakeObject{
						Data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 20),
							Passed:    &yes,
						}),
					},
				},
			},
			group: &configpb.TestGroup{
				GcsPrefix: "bucket/path/to/build/",
			},
			expected: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "13",
						Hint:    "13",
						Started: float64(now+13) * 1000,
					},
					Cells: map[string]cell{
						"build." + overallRow: {
							Result: statuspb.TestStatus_PASS,
							Metrics: map[string]float64{
								"test-duration-minutes": 13 / 60.0,
							},
						},
						"build." + podInfoRow: podInfoPassCell,
					},
				},
				{
					Column: &statepb.Column{
						Build:   "12",
						Hint:    "12",
						Started: float64(now+12) * 1000,
					},
					Cells: map[string]cell{
						"build." + overallRow: {
							Result: statuspb.TestStatus_PASS,
							Metrics: map[string]float64{
								"test-duration-minutes": 12 / 60.0,
							},
						},
						"build." + podInfoRow: podInfoMissingCell,
					},
				},
				{
					Column: &statepb.Column{
						Build:   "11",
						Hint:    "11",
						Started: float64(now+11) * 1000,
					},
					Cells: map[string]cell{
						"build." + overallRow: {
							Result: statuspb.TestStatus_PASS,
							Metrics: map[string]float64{
								"test-duration-minutes": 11 / 60.0,
							},
						},
						"build." + podInfoRow: podInfoPassCell,
					},
				},
				{
					Column: &statepb.Column{
						Build:   "10",
						Hint:    "10",
						Started: float64(now+10) * 1000,
					},
					Cells: map[string]cell{
						"build." + overallRow: {
							Result: statuspb.TestStatus_PASS,
							Metrics: map[string]float64{
								"test-duration-minutes": 10 / 60.0,
							},
						},
						"build." + podInfoRow: podInfoMissingCell,
					},
				},
			},
		},
		{
			name:        "truncate columns after the newest old result with high concurrency",
			concurrency: 30,
			stop:        time.Unix(now+13, 0), // should capture 13 and 12
			builds: []fakeBuild{
				{
					id: "13",
					started: &fakeObject{
						Data: jsonData(metadata.Started{Timestamp: now + 13}),
					},
					finished: &fakeObject{
						Data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 26),
							Passed:    &yes,
						}),
					},
				},
				{
					id: "12",
					started: &fakeObject{
						Data: jsonData(metadata.Started{Timestamp: now + 12}),
					},
					finished: &fakeObject{
						Data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 24),
							Passed:    &yes,
						}),
					},
					podInfo: podInfoSuccess,
				},
				{
					id: "11",
					started: &fakeObject{
						Data: jsonData(metadata.Started{Timestamp: now + 11}),
					},
					finished: &fakeObject{
						Data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 22),
							Passed:    &yes,
						}),
					},
				},
				{
					id: "10",
					started: &fakeObject{
						Data: jsonData(metadata.Started{Timestamp: now + 10}),
					},
					finished: &fakeObject{
						Data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 20),
							Passed:    &yes,
						}),
					},
				},
			},
			group: &configpb.TestGroup{
				GcsPrefix: "bucket/path/to/build/",
			},
			expected: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "13",
						Hint:    "13",
						Started: float64(now+13) * 1000,
					},
					Cells: map[string]cell{
						"build." + overallRow: {
							Result: statuspb.TestStatus_PASS,
							Metrics: map[string]float64{
								"test-duration-minutes": 13 / 60.0,
							},
						},
						"build." + podInfoRow: podInfoMissingCell,
					},
				},
				{
					Column: &statepb.Column{
						Build:   "12",
						Hint:    "12",
						Started: float64(now+12) * 1000,
					},
					Cells: map[string]cell{
						"build." + overallRow: {
							Result: statuspb.TestStatus_PASS,
							Metrics: map[string]float64{
								"test-duration-minutes": 12 / 60.0,
							},
						},
						"build." + podInfoRow: podInfoPassCell,
					},
				},
				// drop 11 and 10
			},
		},
		{
			name: "cancelled context returns error",
			builds: []fakeBuild{
				{id: "10"},
			},
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Err()
				return ctx
			}(),
		},
		{
			name: "some errors",
			builds: []fakeBuild{
				{
					id: "14-err",
					started: &fakeObject{
						OpenErr: errors.New("fake open 14-err"),
					},
				},
				{
					id: "13",
					started: &fakeObject{
						Data: jsonData(metadata.Started{Timestamp: now + 13}),
					},
					finished: &fakeObject{
						Data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 26),
							Passed:    &no,
						}),
					},
					podInfo: podInfoSuccess,
				},
				{
					id: "10-b-err",
					started: &fakeObject{
						OpenErr: errors.New("fake open 10-b-err"),
					},
				},
				{
					id: "10-a-err",
					started: &fakeObject{
						ReadErr: errors.New("fake read 10-a-err"),
					},
				},
				{
					id: "9",
					started: &fakeObject{
						Data: jsonData(metadata.Started{Timestamp: now + 9}),
					},
					finished: &fakeObject{
						Data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 18),
							Passed:    &yes,
						}),
					},
				},
				{
					id: "8-err",
					started: &fakeObject{
						ReadErr: errors.New("fake read 8-err"),
					},
				},
			},
			group: &configpb.TestGroup{
				GcsPrefix: "bucket/path/to/build/",
			},
			expected: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "8-err",
						Hint:    "8-err",
						Started: .01,
					},
					Cells: map[string]cell{
						overallRow: {
							Result:  statuspb.TestStatus_TOOL_FAIL,
							Message: "Failed to download gs://bucket/path/to/build/8-err/: started: read: decode: fake read 8-err",
						},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "9",
						Hint:    "9",
						Started: float64(now+9) * 1000,
					},
					Cells: map[string]cell{
						"build." + overallRow: {
							Result: statuspb.TestStatus_PASS,
							Metrics: map[string]float64{
								"test-duration-minutes": 9 / 60.0,
							},
						},
						"build." + podInfoRow: podInfoMissingCell,
					},
				},
				{
					Column: &statepb.Column{
						Build:   "10-a-err",
						Hint:    "10-a-err",
						Started: float64(now+9)*1000 + .01,
					},
					Cells: map[string]cell{
						overallRow: {
							Result:  statuspb.TestStatus_TOOL_FAIL,
							Message: "Failed to download gs://bucket/path/to/build/10-a-err/: started: read: decode: fake read 10-a-err",
						},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "10-b-err",
						Hint:    "10-b-err",
						Started: float64(now+9)*1000 + 0.02,
					},
					Cells: map[string]cell{
						overallRow: {
							Result:  statuspb.TestStatus_TOOL_FAIL,
							Message: "Failed to download gs://bucket/path/to/build/10-b-err/: started: read: open: fake open 10-b-err",
						},
					},
				},
				{
					Column: &statepb.Column{
						Build:   "13",
						Hint:    "13",
						Started: float64(now+13) * 1000,
					},
					Cells: map[string]cell{
						"build." + overallRow: {
							Result:  statuspb.TestStatus_FAIL,
							Icon:    "F",
							Message: "Build failed outside of test results",
							Metrics: map[string]float64{
								"test-duration-minutes": 13 / 60.0,
							},
						},
						"build." + podInfoRow: podInfoPassCell,
					},
				},
				{
					Column: &statepb.Column{
						Build:   "14-err",
						Hint:    "14-err",
						Started: float64(now+13)*1000 + 0.01,
					},
					Cells: map[string]cell{
						overallRow: {
							Result:  statuspb.TestStatus_TOOL_FAIL,
							Message: "Failed to download gs://bucket/path/to/build/14-err/: started: read: open: fake open 14-err",
						},
					},
				},
			},
		},
		{
			name: "only errors",
			builds: []fakeBuild{
				{
					id: "10-b-err",
					started: &fakeObject{
						OpenErr: errors.New("fake open 10-b-err"),
					},
				},
				{
					id: "10-a-err",
					started: &fakeObject{
						ReadErr: errors.New("fake read 10-a-err"),
					},
				},
			},
			group: &configpb.TestGroup{
				GcsPrefix: "bucket/path/to/build/",
			},
			expected: []InflatedColumn{
				erroredColumn("10-a-err", 0.01, nil, "Failed to download gs://bucket/path/to/build/10-a-err/: started: read: decode: fake read 10-a-err"),
				erroredColumn("10-b-err", 0.02, nil, "Failed to download gs://bucket/path/to/build/10-b-err/: started: read: open: fake open 10-b-err"),
			},
		},
		{
			name: "ignore_skip works when enabled",
			group: &configpb.TestGroup{
				IgnoreSkip: true,
			},
			enableIgnoreSkip: true,
			builds: []fakeBuild{
				{
					id:      "build-1",
					podInfo: podInfoSuccess,
					started: &fakeObject{
						Data: jsonData(metadata.Started{Timestamp: now}),
					},
					finished: &fakeObject{
						Data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 10),
							Passed:    &yes,
						}),
					},
					artifacts: map[string]fakeObject{
						"junit_context-a_33.xml": {
							Data: xmlData(
								junit.Suite{
									Results: []junit.Result{
										{
											Name:    "visible skip non-default msg",
											Skipped: &junit.Skipped{Message: *pstr("non-default message")},
										},
									},
								}),
						},
					},
				},
			},
			expected: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "build-1",
						Started: float64(now * 1000),
						Hint:    "build-1",
					},
					Cells: map[string]Cell{
						".." + overallRow: {
							Result: statuspb.TestStatus_PASS,
							Metrics: map[string]float64{
								"test-duration-minutes": 10 / 60.0,
							},
						},
						".." + podInfoRow: podInfoPassCell,
					},
				},
			},
		},
		{
			name: "ignore_skip ignored when disabled",
			group: &configpb.TestGroup{
				IgnoreSkip: true,
			},
			builds: []fakeBuild{
				{
					id:      "build-1",
					podInfo: podInfoSuccess,
					started: &fakeObject{
						Data: jsonData(metadata.Started{Timestamp: now}),
					},
					finished: &fakeObject{
						Data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 10),
							Passed:    &yes,
						}),
					},
					artifacts: map[string]fakeObject{
						"junit_context-a_33.xml": {
							Data: xmlData(
								junit.Suite{
									Results: []junit.Result{
										{
											Name:    "visible skip non-default msg",
											Skipped: &junit.Skipped{Message: *pstr("non-default message")},
										},
									},
								}),
						},
					},
				},
			},
			expected: []InflatedColumn{
				{
					Column: &statepb.Column{
						Build:   "build-1",
						Started: float64(now * 1000),
						Hint:    "build-1",
					},
					Cells: map[string]Cell{
						".." + overallRow: {
							Result: statuspb.TestStatus_PASS,
							Metrics: map[string]float64{
								"test-duration-minutes": 10 / 60.0,
							},
						},
						".." + podInfoRow: podInfoPassCell,
						"visible skip non-default msg": {
							Result:  statuspb.TestStatus_PASS_WITH_SKIPS,
							Icon:    "S",
							Message: "non-default message",
						},
					},
				},
			},
		},
	}

	poolCtx, poolCancel := context.WithCancel(context.Background())
	defer poolCancel()
	readResultPool := resultReaderPool(poolCtx, logrus.WithField("pool", "readResult"), 10)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.group == nil {
				tc.group = &configpb.TestGroup{}
			}
			path := newPathOrDie("gs://" + tc.group.GcsPrefix)
			if tc.ctx == nil {
				tc.ctx = context.Background()
			}
			ctx, cancel := context.WithCancel(tc.ctx)
			ctx.Err()
			defer cancel()
			client := fakeClient{
				Lister: fake.Lister{},
				Opener: fake.Opener{
					Paths: map[gcs.Path]fake.Object{},
					Lock:  &sync.RWMutex{},
				},
			}

			builds := addBuilds(&client, path, tc.builds...)

			if tc.concurrency == 0 {
				tc.concurrency = 1
			} else {
				t.Skip("TODO(fejta): re-add concurrent build reading")
			}

			if tc.dur == 0 {
				tc.dur = 5 * time.Minute
			}

			var actual []InflatedColumn

			ch := make(chan InflatedColumn)
			var wg sync.WaitGroup

			wg.Add(1)
			go func() {
				defer wg.Done()
				time.Sleep(10 * time.Millisecond) // Give time for context to expire
				for col := range ch {
					actual = append(actual, col)
				}

			}()

			readResult := tc.readResultOverride
			if readResult == nil {
				readResult = readResultPool
			}

			readColumns(ctx, client, logrus.WithField("name", tc.name), tc.group, builds, tc.stop, tc.dur, ch, readResult, tc.enableIgnoreSkip)
			close(ch)
			wg.Wait()

			if diff := cmp.Diff(tc.expected, actual, protocmp.Transform()); diff != "" {
				t.Errorf("readColumns() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRender(t *testing.T) {
	cases := []struct {
		name      string
		format    string
		parts     []string
		job       string
		test      string
		metadatas []map[string]string
		expected  string
	}{
		{
			name: "basically works",
		},
		{
			name:     "test name works",
			format:   "%s",
			parts:    []string{"Tests name"}, // keep the literal
			test:     "hello",
			expected: "hello",
		},
		{
			name:     "missing fields work",
			format:   "%s -(%s)- %s",
			parts:    []string{testsName, "something", jobName},
			job:      "this",
			test:     "hi",
			expected: "hi -()- this",
		},
		{
			name:   "first and second metadata work",
			format: "first %s, second %s",
			parts:  []string{"first", "second"},
			metadatas: []map[string]string{
				{
					"first": "hi",
				},
				{
					"second": "there",
					"first":  "ignore this",
				},
			},
			expected: "first hi, second there",
		},
		{
			name:   "prefer first metadata value over second",
			format: "test: %s, job: %s, meta: %s",
			parts:  []string{testsName, jobName, "meta"},
			test:   "fancy",
			job:    "work",
			metadatas: []map[string]string{
				{
					"meta":    "yes",
					testsName: "ignore",
				},
				{
					"meta":  "no",
					jobName: "wrong",
				},
			},
			expected: "test: fancy, job: work, meta: yes",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nc := nameConfig{
				format: tc.format,
				parts:  tc.parts,
			}
			actual := nc.render(tc.job, tc.test, tc.metadatas...)
			if actual != tc.expected {
				t.Errorf("render() got %q want %q", actual, tc.expected)
			}
		})
	}
}

func TestMakeNameConfig(t *testing.T) {
	cases := []struct {
		name     string
		group    *configpb.TestGroup
		expected nameConfig
	}{
		{
			name:  "basically works",
			group: &configpb.TestGroup{},
			expected: nameConfig{
				format: "%s",
				parts:  []string{testsName},
			},
		},
		{
			name: "explicit config works",
			group: &configpb.TestGroup{
				TestNameConfig: &configpb.TestNameConfig{
					NameFormat: "%s %s",
					NameElements: []*configpb.TestNameConfig_NameElement{
						{
							TargetConfig: "hello",
						},
						{
							TargetConfig: "world",
						},
					},
				},
			},
			expected: nameConfig{
				format: "%s %s",
				parts:  []string{"hello", "world"},
			},
		},
		{
			name: "test properties work",
			group: &configpb.TestGroup{
				TestNameConfig: &configpb.TestNameConfig{
					NameFormat: "%s %s",
					NameElements: []*configpb.TestNameConfig_NameElement{
						{
							TargetConfig: "hello",
						},
						{
							TestProperty: "world",
						},
					},
				},
			},
			expected: nameConfig{
				format: "%s %s",
				parts:  []string{"hello", "world"},
			},
		},
		{
			name: "target config precedes test property",
			group: &configpb.TestGroup{
				TestNameConfig: &configpb.TestNameConfig{
					NameFormat: "%s works",
					NameElements: []*configpb.TestNameConfig_NameElement{
						{
							TargetConfig: "good-target",
							TestProperty: "nope-property",
						},
					},
				},
			},
			expected: nameConfig{
				format: "%s works",
				parts:  []string{"good-target"},
			},
		},
		{
			name: "auto-inject job name into default config",
			group: &configpb.TestGroup{
				GcsPrefix: "this,that",
			},
			expected: nameConfig{
				format:   "%s.%s",
				parts:    []string{jobName, testsName},
				multiJob: true,
			},
		},
		{
			name: "auto-inject job name into explicit config",
			group: &configpb.TestGroup{
				GcsPrefix: "this,that",
				TestNameConfig: &configpb.TestNameConfig{
					NameFormat: "%s %s",
					NameElements: []*configpb.TestNameConfig_NameElement{
						{
							TargetConfig: "hello",
						},
						{
							TargetConfig: "world",
						},
					},
				},
			},
			expected: nameConfig{
				format:   "%s.%s %s",
				parts:    []string{jobName, "hello", "world"},
				multiJob: true,
			},
		},
		{
			name: "allow explicit job name config",
			group: &configpb.TestGroup{
				GcsPrefix: "this,that",
				TestNameConfig: &configpb.TestNameConfig{
					NameFormat: "%s %s (%s)",
					NameElements: []*configpb.TestNameConfig_NameElement{
						{
							TargetConfig: "hello",
						},
						{
							TargetConfig: "world",
						},
						{
							TargetConfig: jobName,
						},
					},
				},
			},
			expected: nameConfig{
				format:   "%s %s (%s)",
				parts:    []string{"hello", "world", jobName},
				multiJob: true,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := makeNameConfig(tc.group)
			if diff := cmp.Diff(actual, tc.expected, cmp.AllowUnexported(nameConfig{})); diff != "" {
				t.Errorf("makeNameConfig() got unexpected diff (-got +want):\n%s", diff)
			}
		})
	}
}

func TestReadResult(t *testing.T) {
	path := newPathOrDie("gs://bucket/path/to/some/build/")
	yes := true
	cases := []struct {
		name string
		ctx  context.Context
		data map[string]fakeObject
		stop time.Time

		expected *gcsResult
	}{
		{
			name: "basically works",
			expected: &gcsResult{
				started: gcs.Started{
					Pending: true,
				},
				finished: gcs.Finished{
					Running: true,
				},
				job:   "some",
				build: "build",
			},
		},
		{
			name: "cancelled context returns error",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
		},
		{
			name: "all info present",
			data: map[string]fakeObject{
				"podinfo.json":       {Data: `{"pod":{"metadata":{"name":"woot"}}}`},
				"started.json":       {Data: `{"node": "fun"}`},
				"finished.json":      {Data: `{"passed": true}`},
				"junit_super_88.xml": {Data: `<testsuite><testcase name="foo"/></testsuite>`},
			},
			expected: &gcsResult{
				podInfo: func() gcs.PodInfo {
					out := gcs.PodInfo{Pod: &core.Pod{}}
					out.Pod.Name = "woot"
					return out
				}(),
				started: gcs.Started{
					Started: metadata.Started{Node: "fun"},
				},
				finished: gcs.Finished{
					Finished: metadata.Finished{Passed: &yes},
				},
				suites: []gcs.SuitesMeta{
					{
						Suites: &junit.Suites{
							Suites: []junit.Suite{
								{
									XMLName: xml.Name{Local: "testsuite"},
									Results: []junit.Result{
										{Name: "foo"},
									},
								},
							},
						},
						Metadata: map[string]string{
							"Context":   "super",
							"Thread":    "88",
							"Timestamp": "",
						},
						Path: "gs://bucket/path/to/some/build/junit_super_88.xml",
					},
				},
			},
		},
		{
			name: "empty files report missing",
			data: map[string]fakeObject{
				"finished.json": {Data: ""},
				"started.json":  {Data: ""},
				"podinfo.json":  {Data: ""},
			},
			expected: &gcsResult{
				malformed: []string{
					"finished.json",
					"podinfo.json",
					"started.json",
				},
			},
		},
		{
			name: "missing started.json reports pending",
			data: map[string]fakeObject{
				"finished.json":      {Data: `{"passed": true}`},
				"junit_super_88.xml": {Data: `<testsuite><testcase name="foo"/></testsuite>`},
			},
			expected: &gcsResult{
				started: gcs.Started{
					Pending: true,
				},
				finished: gcs.Finished{
					Finished: metadata.Finished{Passed: &yes},
				},
				suites: []gcs.SuitesMeta{
					{
						Suites: &junit.Suites{
							Suites: []junit.Suite{
								{
									XMLName: xml.Name{Local: "testsuite"},
									Results: []junit.Result{
										{Name: "foo"},
									},
								},
							},
						},
						Metadata: map[string]string{
							"Context":   "super",
							"Thread":    "88",
							"Timestamp": "",
						},
						Path: "gs://bucket/path/to/some/build/junit_super_88.xml",
					},
				},
			},
		},
		{
			name: "no finished reports running",
			data: map[string]fakeObject{
				"started.json":       {Data: `{"node": "fun"}`},
				"junit_super_88.xml": {Data: `<testsuite><testcase name="foo"/></testsuite>`},
			},
			expected: &gcsResult{
				started: gcs.Started{
					Started: metadata.Started{Node: "fun"},
				},
				finished: gcs.Finished{
					Running: true,
				},
				suites: []gcs.SuitesMeta{
					{
						Suites: &junit.Suites{
							Suites: []junit.Suite{
								{
									XMLName: xml.Name{Local: "testsuite"},
									Results: []junit.Result{
										{Name: "foo"},
									},
								},
							},
						},
						Metadata: map[string]string{
							"Context":   "super",
							"Thread":    "88",
							"Timestamp": "",
						},
						Path: "gs://bucket/path/to/some/build/junit_super_88.xml",
					},
				},
			},
		},
		{
			name: "no artifacts report no suites",
			data: map[string]fakeObject{
				"started.json":  {Data: `{"node": "fun"}`},
				"finished.json": {Data: `{"passed": true}`},
			},
			expected: &gcsResult{
				started: gcs.Started{
					Started: metadata.Started{Node: "fun"},
				},
				finished: gcs.Finished{
					Finished: metadata.Finished{Passed: &yes},
				},
			},
		},
		{
			name: "started error returns error",
			data: map[string]fakeObject{
				"started.json": {
					Data:     "{}",
					CloseErr: errors.New("injected closer error"),
				},
				"finished.json":      {Data: `{"passed": true}`},
				"junit_super_88.xml": {Data: `<testsuite><testcase name="foo"/></testsuite>`},
			},
		},
		{
			name: "finished error returns error",
			data: map[string]fakeObject{
				"started.json":       {Data: `{"node": "fun"}`},
				"finished.json":      {ReadErr: errors.New("injected read error")},
				"junit_super_88.xml": {Data: `<testsuite><testcase name="foo"/></testsuite>`},
			},
		},
		{
			name: "artifact error added to malformed list",
			data: map[string]fakeObject{
				"started.json":       {Data: `{"node": "fun"}`},
				"finished.json":      {Data: `{"passed": true}`},
				"junit_super_88.xml": {OpenErr: errors.New("injected open error")},
			},
			expected: &gcsResult{
				started: gcs.Started{
					Started: metadata.Started{Node: "fun"},
				},
				finished: gcs.Finished{
					Finished: metadata.Finished{Passed: &yes},
				},
				malformed: []string{"junit_super_88.xml: open: injected open error"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ctx == nil {
				tc.ctx = context.Background()
			}
			if tc.expected != nil {
				tc.expected.job = "some"
				tc.expected.build = "build"
			}
			ctx, cancel := context.WithCancel(tc.ctx)
			defer cancel()
			client := fakeClient{
				Lister: fake.Lister{},
				Opener: fake.Opener{
					Paths: map[gcs.Path]fake.Object{},
					Lock:  &sync.RWMutex{},
				},
			}

			fi := fakeIterator{}
			for name, fo := range tc.data {
				p, err := path.ResolveReference(&url.URL{Path: name})
				if err != nil {
					t.Fatalf("path.ResolveReference(%q): %v", name, err)
				}
				fi.Objects = append(fi.Objects, storage.ObjectAttrs{
					Name: p.Object(),
				})
				client.Opener.Paths[*p] = fo
			}
			client.Lister[path] = fi

			build := gcs.Build{
				Path: path,
			}
			actual, err := readResult(ctx, client, build, tc.stop)
			switch {
			case err != nil:
				if tc.expected != nil {
					t.Errorf("readResult(): unexpected error: %v", err)
				}
			case tc.expected == nil:
				t.Error("readResult(): failed to receive expected error")
			default:
				if diff := cmp.Diff(actual, tc.expected, cmp.AllowUnexported(gcsResult{})); diff != "" {
					t.Errorf("readResult() got unexpected diff (-have, +want):\n%s", diff)
				}
			}
		})
	}
}

func newPathOrDie(s string) gcs.Path {
	p, err := gcs.NewPath(s)
	if err != nil {
		panic(err)
	}
	return *p
}

func TestReadSuites(t *testing.T) {
	path := newPathOrDie("gs://bucket/path/to/build/")
	cases := []struct {
		name       string
		data       map[string]fakeObject
		listIdxErr int
		expected   []gcs.SuitesMeta
		err        bool
		ctx        context.Context
	}{
		{
			name: "basically works",
		},
		{
			name: "multiple suites from multiple artifacts work",
			data: map[string]fakeObject{
				"ignore-this": {Data: "<invalid></xml>"},
				"junit.xml":   {Data: `<testsuite><testcase name="hi"/></testsuite>`},
				"ignore-that": {Data: "<invalid></xml>"},
				"nested/junit_context_20201122-1234_88.xml": {
					Data: `
                        <testsuites>
                            <testsuite name="fun">
                                <testsuite name="knee">
                                    <testcase name="bone" time="6" />
                                </testsuite>
                                <testcase name="word" time="7" />
                            </testsuite>
                        </testsuites>
                    `,
				},
			},
			expected: []gcs.SuitesMeta{
				{
					Suites: &junit.Suites{
						Suites: []junit.Suite{
							{
								XMLName: xml.Name{Local: "testsuite"},
								Results: []junit.Result{
									{Name: "hi"},
								},
							},
						},
					},
					Metadata: map[string]string{
						"Context":   "",
						"Thread":    "",
						"Timestamp": "",
					},
					Path: "gs://bucket/path/to/build/junit.xml",
				},
				{
					Suites: &junit.Suites{
						XMLName: xml.Name{Local: "testsuites"},
						Suites: []junit.Suite{
							{
								XMLName: xml.Name{Local: "testsuite"},
								Name:    "fun",
								Suites: []junit.Suite{
									{
										XMLName: xml.Name{Local: "testsuite"},
										Name:    "knee",
										Results: []junit.Result{
											{
												Name: "bone",
												Time: 6,
											},
										},
									},
								},
								Results: []junit.Result{
									{
										Name: "word",
										Time: 7,
									},
								},
							},
						},
					},
					Metadata: map[string]string{
						"Context":   "context",
						"Thread":    "88",
						"Timestamp": "20201122-1234",
					},
					Path: "gs://bucket/path/to/build/nested/junit_context_20201122-1234_88.xml",
				},
			},
		},
		{
			name: "list error returns error",
			data: map[string]fakeObject{
				"ignore-this": {Data: "<invalid></xml>"},
				"junit.xml":   {Data: `<testsuite><testcase name="hi"/></testsuite>`},
				"ignore-that": {Data: "<invalid></xml>"},
			},
			listIdxErr: 1,
			err:        true,
		},
		{
			name: "cancelled context returns err",
			data: map[string]fakeObject{
				"junit.xml": {Data: `<testsuite><testcase name="hi"/></testsuite>`},
			},
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			err: true,
		},
		{
			name: "suites error contains error",
			data: map[string]fakeObject{
				"junit.xml": {Data: "<invalid></xml>"},
			},
			expected: []gcs.SuitesMeta{
				{
					Metadata: map[string]string{
						"Context":   "",
						"Thread":    "",
						"Timestamp": "",
					},
					Path: "gs://bucket/path/to/build/junit.xml",
					Err:  errors.New("foo"),
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ctx == nil {
				tc.ctx = context.Background()
			}
			ctx, cancel := context.WithCancel(tc.ctx)
			defer cancel()
			client := fakeClient{
				Lister: fake.Lister{},
				Opener: fake.Opener{
					Paths: map[gcs.Path]fake.Object{},
				},
			}

			fi := fakeIterator{
				Err: tc.listIdxErr,
			}
			for name, fo := range tc.data {
				p, err := path.ResolveReference(&url.URL{Path: name})
				if err != nil {
					t.Fatalf("path.ResolveReference(%q): %v", name, err)
				}
				fi.Objects = append(fi.Objects, storage.ObjectAttrs{
					Name: p.Object(),
				})
				client.Opener.Paths[*p] = fo
			}
			client.Lister[path] = fi

			build := gcs.Build{
				Path: path,
			}
			actual, err := readSuites(ctx, &client, build)
			sort.SliceStable(actual, func(i, j int) bool {
				return actual[i].Path < actual[j].Path
			})
			sort.SliceStable(tc.expected, func(i, j int) bool {
				return tc.expected[i].Path < tc.expected[j].Path
			})
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("readSuites(): unexpected error: %v", err)
				}
			case tc.err:
				t.Error("readSuites(): failed to receive an error")
			default:
				cmpErrs := func(x, y error) bool {
					return (x == nil) == (y == nil)
				}
				if diff := cmp.Diff(tc.expected, actual, cmp.Comparer(cmpErrs)); diff != "" {
					t.Errorf("readSuites() got unexpected diff (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func addBuilds(fc *fake.Client, path gcs.Path, s ...fakeBuild) []gcs.Build {
	if fc.Opener.Lock != nil {
		fc.Opener.Lock.Lock()
		defer fc.Opener.Lock.Unlock()
	}
	var builds []gcs.Build
	for _, build := range s {
		buildPath := resolveOrDie(&path, build.id+"/")
		builds = append(builds, gcs.Build{Path: *buildPath})
		fi := fake.Iterator{}

		if build.podInfo != nil {
			p := resolveOrDie(buildPath, "podinfo.json")
			fi.Objects = append(fi.Objects, storage.ObjectAttrs{
				Name: p.Object(),
			})
			fc.Opener.Paths[*p] = *build.podInfo
		}
		if build.started != nil {
			p := resolveOrDie(buildPath, "started.json")
			fi.Objects = append(fi.Objects, storage.ObjectAttrs{
				Name: p.Object(),
			})
			fc.Opener.Paths[*p] = *build.started
		}
		if build.finished != nil {
			p := resolveOrDie(buildPath, "finished.json")
			fi.Objects = append(fi.Objects, storage.ObjectAttrs{
				Name: p.Object(),
			})
			fc.Opener.Paths[*p] = *build.finished
		}
		if len(build.passed)+len(build.failed) > 0 {
			p := resolveOrDie(buildPath, "junit_automatic.xml")
			fc.Opener.Paths[*p] = fake.Object{Data: makeJunit(build.passed, build.failed)}
			fi.Objects = append(fi.Objects, storage.ObjectAttrs{
				Name: p.Object(),
			})
		}
		for n, fo := range build.artifacts {
			p := resolveOrDie(buildPath, n)
			fi.Objects = append(fi.Objects, storage.ObjectAttrs{
				Name: p.Object(),
			})
			fc.Opener.Paths[*p] = fo
		}
		fc.Lister[*buildPath] = fi
	}
	return builds

}

type fakeBuild struct {
	id        string
	started   *fakeObject
	finished  *fakeObject
	podInfo   *fakeObject
	artifacts map[string]fakeObject
	passed    []string
	failed    []string
}
