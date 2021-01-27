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
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/metadata"
	"github.com/GoogleCloudPlatform/testgrid/metadata/junit"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	statuspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"

	"cloud.google.com/go/storage"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/testing/protocmp"
)

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
			Failure: &f,
		})
	}
	return xmlData(suite)
}

func pint64(n int64) *int64 {
	return &n
}

func TestReadColumns(t *testing.T) {
	now := time.Now().Unix()
	yes := true
	var no bool
	cases := []struct {
		name        string
		ctx         context.Context
		builds      []fakeBuild
		group       configpb.TestGroup
		max         int
		stop        time.Time
		dur         time.Duration
		concurrency int

		expected []inflatedColumn
		err      bool
	}{
		{
			name:     "basically works",
			expected: []inflatedColumn{},
		},
		{
			name: "convert results correctly",
			builds: []fakeBuild{
				{
					id: "11",
					started: &fakeObject{
						data: jsonData(metadata.Started{Timestamp: now + 11}),
					},
					finished: &fakeObject{
						data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 22),
							Passed:    &no,
						}),
					},
				},
				{
					id: "10",
					started: &fakeObject{
						data: jsonData(metadata.Started{Timestamp: now + 10}),
					},
					finished: &fakeObject{
						data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 20),
							Passed:    &yes,
						}),
					},
				},
			},
			group: configpb.TestGroup{
				GcsPrefix: "bucket/path/to/build/",
			},
			expected: []inflatedColumn{
				{
					column: &statepb.Column{
						Build:   "11",
						Started: float64(now+11) * 1000,
					},
					cells: map[string]cell{
						"Overall": {
							result:  statuspb.TestStatus_FAIL,
							icon:    "F",
							message: "Build failed outside of test results",
							metrics: map[string]float64{
								"test-duration-minutes": 11 / 60.0,
							},
						},
					},
				},
				{
					column: &statepb.Column{
						Build:   "10",
						Started: float64(now+10) * 1000,
					},
					cells: map[string]cell{
						"Overall": {
							result: statuspb.TestStatus_PASS,
							metrics: map[string]float64{
								"test-duration-minutes": 10 / 60.0,
							},
						},
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
						data: jsonData(metadata.Started{Timestamp: now + 11}),
					},
					finished: &fakeObject{
						data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 22),
							Passed:    &no,
							Metadata: metadata.Metadata{
								metadata.JobVersion: "v0.0.0-alpha.0+build11",
								"random":            "new information",
							},
						}),
					},
				},
				{
					id: "10",
					started: &fakeObject{
						data: jsonData(metadata.Started{Timestamp: now + 10}),
					},
					finished: &fakeObject{
						data: jsonData(metadata.Finished{
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
			group: configpb.TestGroup{
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
			expected: []inflatedColumn{
				{
					column: &statepb.Column{
						Build:   "11",
						Started: float64(now+11) * 1000,
						Extra: []string{
							"build11",
							"new information",
						},
					},
					cells: map[string]cell{
						"Overall": {
							result:  statuspb.TestStatus_FAIL,
							icon:    "F",
							message: "Build failed outside of test results",
							metrics: map[string]float64{
								"test-duration-minutes": 11 / 60.0,
							},
						},
					},
				},
				{
					column: &statepb.Column{
						Build:   "10",
						Started: float64(now+10) * 1000,
						Extra: []string{
							"build10",
							"old information",
						},
					},
					cells: map[string]cell{
						"Overall": {
							result: statuspb.TestStatus_PASS,
							metrics: map[string]float64{
								"test-duration-minutes": 10 / 60.0,
							},
						},
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
						data: jsonData(metadata.Started{Timestamp: now + 10}),
					},
					finished: &fakeObject{
						data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 20),
							Passed:    &yes,
						}),
					},
					artifacts: map[string]fakeObject{
						"junit_context-a_33.xml": {
							data: makeJunit([]string{"good"}, []string{"bad"}),
						},
						"junit_context-b_44.xml": {
							data: makeJunit([]string{"good"}, []string{"bad"}),
						},
					},
				},
			},
			group: configpb.TestGroup{
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
			expected: []inflatedColumn{
				{
					column: &statepb.Column{
						Build:   "10",
						Started: float64(now+10) * 1000,
					},
					cells: map[string]cell{
						"Overall": {
							result: statuspb.TestStatus_PASS,
							metrics: map[string]float64{
								"test-duration-minutes": 10 / 60.0,
							},
						},
						"name good - context context-a - thread 33": {
							result: statuspb.TestStatus_PASS,
						},
						"name bad - context context-a - thread 33": {
							result:  statuspb.TestStatus_FAIL,
							icon:    "F",
							message: "bad",
						},
						"name good - context context-b - thread 44": {
							result: statuspb.TestStatus_PASS,
						},
						"name bad - context context-b - thread 44": {
							result:  statuspb.TestStatus_FAIL,
							icon:    "F",
							message: "bad",
						},
					},
				},
			},
		},
		{
			name: "stop columns at max",
			max:  2,
			builds: []fakeBuild{
				{
					id: "12",
					started: &fakeObject{
						data: jsonData(metadata.Started{Timestamp: now + 12}),
					},
					finished: &fakeObject{
						data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 24),
							Passed:    &yes,
						}),
					},
				},
				{
					id: "11",
					started: &fakeObject{
						data: jsonData(metadata.Started{Timestamp: now + 11}),
					},
					finished: &fakeObject{
						data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 22),
							Passed:    &yes,
						}),
					},
				},
				{
					id: "10",
					started: &fakeObject{
						data: jsonData(metadata.Started{Timestamp: now + 10}),
					},
					finished: &fakeObject{
						data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 20),
							Passed:    &yes,
						}),
					},
				},
			},
			group: configpb.TestGroup{
				GcsPrefix: "bucket/path/to/build/",
			},
			expected: []inflatedColumn{
				{
					column: &statepb.Column{
						Build:   "12",
						Started: float64(now+12) * 1000,
					},
					cells: map[string]cell{
						"Overall": {
							result: statuspb.TestStatus_PASS,
							metrics: map[string]float64{
								"test-duration-minutes": 12 / 60.0,
							},
						},
					},
				},
				{
					column: &statepb.Column{
						Build:   "11",
						Started: float64(now+11) * 1000,
					},
					cells: map[string]cell{
						"Overall": {
							result: statuspb.TestStatus_PASS,
							metrics: map[string]float64{
								"test-duration-minutes": 11 / 60.0,
							},
						},
					},
				},
			},
		},
		{
			name: "truncate columns after the newest old result",
			stop: time.Unix(now+13, 0), // should capture 13 and 12
			builds: []fakeBuild{
				{
					id: "13",
					started: &fakeObject{
						data: jsonData(metadata.Started{Timestamp: now + 13}),
					},
					finished: &fakeObject{
						data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 26),
							Passed:    &yes,
						}),
					},
				},
				{
					id: "12",
					started: &fakeObject{
						data: jsonData(metadata.Started{Timestamp: now + 12}),
					},
					finished: &fakeObject{
						data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 24),
							Passed:    &yes,
						}),
					},
				},
				{
					id: "11",
					started: &fakeObject{
						data: jsonData(metadata.Started{Timestamp: now + 11}),
					},
					finished: &fakeObject{
						data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 22),
							Passed:    &yes,
						}),
					},
				},
				{
					id: "10",
					started: &fakeObject{
						data: jsonData(metadata.Started{Timestamp: now + 10}),
					},
					finished: &fakeObject{
						data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 20),
							Passed:    &yes,
						}),
					},
				},
			},
			group: configpb.TestGroup{
				GcsPrefix: "bucket/path/to/build/",
			},
			expected: []inflatedColumn{
				{
					column: &statepb.Column{
						Build:   "13",
						Started: float64(now+13) * 1000,
					},
					cells: map[string]cell{
						"Overall": {
							result: statuspb.TestStatus_PASS,
							metrics: map[string]float64{
								"test-duration-minutes": 13 / 60.0,
							},
						},
					},
				},
				{
					column: &statepb.Column{
						Build:   "12",
						Started: float64(now+12) * 1000,
					},
					cells: map[string]cell{
						"Overall": {
							result: statuspb.TestStatus_PASS,
							metrics: map[string]float64{
								"test-duration-minutes": 12 / 60.0,
							},
						},
					},
				},
				// drop 11 and 10
			},
		},
		{
			name:        "high concurrency works",
			concurrency: 4,
			builds: []fakeBuild{
				{
					id: "13",
					started: &fakeObject{
						data: jsonData(metadata.Started{Timestamp: now + 13}),
					},
					finished: &fakeObject{
						data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 26),
							Passed:    &yes,
						}),
					},
				},
				{
					id: "12",
					started: &fakeObject{
						data: jsonData(metadata.Started{Timestamp: now + 12}),
					},
					finished: &fakeObject{
						data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 24),
							Passed:    &yes,
						}),
					},
				},
				{
					id: "11",
					started: &fakeObject{
						data: jsonData(metadata.Started{Timestamp: now + 11}),
					},
					finished: &fakeObject{
						data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 22),
							Passed:    &yes,
						}),
					},
				},
				{
					id: "10",
					started: &fakeObject{
						data: jsonData(metadata.Started{Timestamp: now + 10}),
					},
					finished: &fakeObject{
						data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 20),
							Passed:    &yes,
						}),
					},
				},
			},
			group: configpb.TestGroup{
				GcsPrefix: "bucket/path/to/build/",
			},
			expected: []inflatedColumn{
				{
					column: &statepb.Column{
						Build:   "13",
						Started: float64(now+13) * 1000,
					},
					cells: map[string]cell{
						"Overall": {
							result: statuspb.TestStatus_PASS,
							metrics: map[string]float64{
								"test-duration-minutes": 13 / 60.0,
							},
						},
					},
				},
				{
					column: &statepb.Column{
						Build:   "12",
						Started: float64(now+12) * 1000,
					},
					cells: map[string]cell{
						"Overall": {
							result: statuspb.TestStatus_PASS,
							metrics: map[string]float64{
								"test-duration-minutes": 12 / 60.0,
							},
						},
					},
				},
				{
					column: &statepb.Column{
						Build:   "11",
						Started: float64(now+11) * 1000,
					},
					cells: map[string]cell{
						"Overall": {
							result: statuspb.TestStatus_PASS,
							metrics: map[string]float64{
								"test-duration-minutes": 11 / 60.0,
							},
						},
					},
				},
				{
					column: &statepb.Column{
						Build:   "10",
						Started: float64(now+10) * 1000,
					},
					cells: map[string]cell{
						"Overall": {
							result: statuspb.TestStatus_PASS,
							metrics: map[string]float64{
								"test-duration-minutes": 10 / 60.0,
							},
						},
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
						data: jsonData(metadata.Started{Timestamp: now + 13}),
					},
					finished: &fakeObject{
						data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 26),
							Passed:    &yes,
						}),
					},
				},
				{
					id: "12",
					started: &fakeObject{
						data: jsonData(metadata.Started{Timestamp: now + 12}),
					},
					finished: &fakeObject{
						data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 24),
							Passed:    &yes,
						}),
					},
				},
				{
					id: "11",
					started: &fakeObject{
						data: jsonData(metadata.Started{Timestamp: now + 11}),
					},
					finished: &fakeObject{
						data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 22),
							Passed:    &yes,
						}),
					},
				},
				{
					id: "10",
					started: &fakeObject{
						data: jsonData(metadata.Started{Timestamp: now + 10}),
					},
					finished: &fakeObject{
						data: jsonData(metadata.Finished{
							Timestamp: pint64(now + 20),
							Passed:    &yes,
						}),
					},
				},
			},
			group: configpb.TestGroup{
				GcsPrefix: "bucket/path/to/build/",
			},
			expected: []inflatedColumn{
				{
					column: &statepb.Column{
						Build:   "13",
						Started: float64(now+13) * 1000,
					},
					cells: map[string]cell{
						"Overall": {
							result: statuspb.TestStatus_PASS,
							metrics: map[string]float64{
								"test-duration-minutes": 13 / 60.0,
							},
						},
					},
				},
				{
					column: &statepb.Column{
						Build:   "12",
						Started: float64(now+12) * 1000,
					},
					cells: map[string]cell{
						"Overall": {
							result: statuspb.TestStatus_PASS,
							metrics: map[string]float64{
								"test-duration-minutes": 12 / 60.0,
							},
						},
					},
				},
				// drop 11 and 10
			},
		},
		{
			name: "cancelled context returns error",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			err: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := newPathOrDie("gs://" + tc.group.GcsPrefix)
			if tc.ctx == nil {
				tc.ctx = context.Background()
			}
			ctx, cancel := context.WithCancel(tc.ctx)
			defer cancel()
			client := fakeClient{
				fakeLister: fakeLister{},
				fakeOpener: fakeOpener{},
			}

			builds := client.addBuilds(path, tc.builds...)

			if tc.concurrency == 0 {
				tc.concurrency = 1
			}

			if tc.max == 0 {
				tc.max = len(builds)
			}

			if tc.dur == 0 {
				tc.dur = 5 * time.Minute
			}

			actual, err := readColumns(ctx, client, &tc.group, builds, tc.stop, tc.max, tc.dur, tc.concurrency)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("readColumns(): unexpected error: %v", err)
				}
			case tc.err:
				t.Error("readColumns(): failed to receive an error")
			default:
				if diff := cmp.Diff(actual, tc.expected, cmp.AllowUnexported(inflatedColumn{}, cell{}), protocmp.Transform()); diff != "" {
					t.Errorf("readColumns() got unexpected diff (-got +want):\n%s", diff)
				}
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
			name: "auto-inject job name into default config",
			group: &configpb.TestGroup{
				GcsPrefix: "this,that",
			},
			expected: nameConfig{
				format: "%s.%s",
				parts:  []string{jobName, testsName},
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
				format: "%s.%s %s",
				parts:  []string{jobName, "hello", "world"},
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
				format: "%s %s (%s)",
				parts:  []string{"hello", "world", jobName},
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
		name     string
		ctx      context.Context
		data     map[string]fakeObject
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
				"started.json":       {data: `{"node": "fun"}`},
				"finished.json":      {data: `{"passed": true}`},
				"junit_super_88.xml": {data: `<testsuite><testcase name="foo"/></testsuite>`},
			},
			expected: &gcsResult{
				started: gcs.Started{
					Started: metadata.Started{Node: "fun"},
				},
				finished: gcs.Finished{
					Finished: metadata.Finished{Passed: &yes},
				},
				suites: []gcs.SuitesMeta{
					{
						Suites: junit.Suites{
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
			name: "missing started.json reports pending",
			data: map[string]fakeObject{
				"finished.json":      {data: `{"passed": true}`},
				"junit_super_88.xml": {data: `<testsuite><testcase name="foo"/></testsuite>`},
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
						Suites: junit.Suites{
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
				"started.json":       {data: `{"node": "fun"}`},
				"junit_super_88.xml": {data: `<testsuite><testcase name="foo"/></testsuite>`},
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
						Suites: junit.Suites{
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
				"started.json":  {data: `{"node": "fun"}`},
				"finished.json": {data: `{"passed": true}`},
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
					data:     "{}",
					closeErr: errors.New("injected closer error"),
				},
				"finished.json":      {data: `{"passed": true}`},
				"junit_super_88.xml": {data: `<testsuite><testcase name="foo"/></testsuite>`},
			},
		},
		{
			name: "finished error returns error",
			data: map[string]fakeObject{
				"started.json":       {data: `{"node": "fun"}`},
				"finished.json":      {readErr: errors.New("injected read error")},
				"junit_super_88.xml": {data: `<testsuite><testcase name="foo"/></testsuite>`},
			},
		},
		{
			name: "artifact error returns error",
			data: map[string]fakeObject{
				"started.json":       {data: `{"node": "fun"}`},
				"finished.json":      {data: `{"passed": true}`},
				"junit_super_88.xml": {openErr: errors.New("injected open error")},
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
				fakeLister: fakeLister{},
				fakeOpener: fakeOpener{},
			}

			fi := fakeIterator{}
			for name, fo := range tc.data {
				p, err := path.ResolveReference(&url.URL{Path: name})
				if err != nil {
					t.Fatalf("path.ResolveReference(%q): %w", name, err)
				}
				fi.objects = append(fi.objects, storage.ObjectAttrs{
					Name: p.Object(),
				})
				client.fakeOpener[*p] = fo
			}
			client.fakeLister[path] = fi

			build := gcs.Build{
				Path: path,
			}
			actual, err := readResult(ctx, client, build)
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
				"ignore-this": {data: "<invalid></xml>"},
				"junit.xml":   {data: `<testsuite><testcase name="hi"/></testsuite>`},
				"ignore-that": {data: "<invalid></xml>"},
				"nested/junit_context_20201122-1234_88.xml": {
					data: `
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
					Suites: junit.Suites{
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
					Suites: junit.Suites{
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
				"ignore-this": {data: "<invalid></xml>"},
				"junit.xml":   {data: `<testsuite><testcase name="hi"/></testsuite>`},
				"ignore-that": {data: "<invalid></xml>"},
			},
			listIdxErr: 1,
			err:        true,
		},
		{
			name: "cancelled context returns err",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			err: true,
		},
		{
			name: "suites error returns error",
			data: map[string]fakeObject{
				"junit.xml": {data: "<invalid></xml>"},
			},
			err: true,
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
				fakeLister: fakeLister{},
				fakeOpener: fakeOpener{},
			}

			fi := fakeIterator{
				err: tc.listIdxErr,
			}
			for name, fo := range tc.data {
				p, err := path.ResolveReference(&url.URL{Path: name})
				if err != nil {
					t.Fatalf("path.ResolveReference(%q): %w", name, err)
				}
				fi.objects = append(fi.objects, storage.ObjectAttrs{
					Name: p.Object(),
				})
				client.fakeOpener[*p] = fo
			}
			client.fakeLister[path] = fi

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
				if !reflect.DeepEqual(actual, tc.expected) {
					t.Errorf("readSuites():\nhave %+v,\nwant %+v", actual, tc.expected)
				}
			}
		})
	}
}

type fakeOpener map[gcs.Path]fakeObject

func (fo fakeOpener) Open(ctx context.Context, path gcs.Path) (io.ReadCloser, error) {
	o, ok := fo[path]
	if !ok {
		return nil, fmt.Errorf("wrap not exist: %w", storage.ErrObjectNotExist)
	}
	if o.openErr != nil {
		return nil, o.openErr
	}
	return &fakeReader{
		buf:      bytes.NewBufferString(o.data),
		readErr:  o.readErr,
		closeErr: o.closeErr,
	}, nil
}

type fakeObject struct {
	data     string
	openErr  error
	readErr  error
	closeErr error
}

type fakeReader struct {
	buf      *bytes.Buffer
	readErr  error
	closeErr error
}

func (fr *fakeReader) Read(p []byte) (int, error) {
	if fr.readErr != nil {
		return 0, fr.readErr
	}
	return fr.buf.Read(p)
}

func (fr *fakeReader) Close() error {
	if fr.closeErr != nil {
		return fr.closeErr
	}
	fr.readErr = errors.New("already closed")
	fr.closeErr = fr.readErr
	return nil
}

type fakeLister map[gcs.Path]fakeIterator

func (fl fakeLister) Objects(ctx context.Context, path gcs.Path, _, offset string) gcs.Iterator {
	f := fl[path]
	f.ctx = ctx
	return &f
}

type fakeIterator struct {
	objects []storage.ObjectAttrs
	idx     int
	err     int // must be > 0
	ctx     context.Context
	offset  string
}

type fakeClient struct {
	fakeLister
	fakeOpener
}

func (fi *fakeIterator) Next() (*storage.ObjectAttrs, error) {
	if fi.ctx.Err() != nil {
		return nil, fi.ctx.Err()
	}
	for fi.idx < len(fi.objects) {
		if fi.offset == "" {
			break
		}
		name, prefix := fi.objects[fi.idx].Name, fi.objects[fi.idx].Prefix
		if name != "" && name < fi.offset {
			continue
		}
		if prefix != "" && prefix < fi.offset {
			continue
		}
		fi.idx++
	}
	if fi.idx >= len(fi.objects) {
		return nil, iterator.Done
	}
	if fi.idx > 0 && fi.idx == fi.err {
		return nil, errors.New("injected fakeIterator error")
	}

	o := fi.objects[fi.idx]
	fi.idx++
	return &o, nil
}

func (fc *fakeClient) addBuilds(path gcs.Path, fakes ...fakeBuild) []gcs.Build {
	var builds []gcs.Build
	for _, build := range fakes {
		buildPath := resolveOrDie(&path, build.id+"/")
		builds = append(builds, gcs.Build{Path: *buildPath})
		fi := fakeIterator{}

		if build.started != nil {
			p := resolveOrDie(buildPath, "started.json")
			fi.objects = append(fi.objects, storage.ObjectAttrs{
				Name: p.Object(),
			})
			fc.fakeOpener[*p] = *build.started
		}
		if build.finished != nil {
			p := resolveOrDie(buildPath, "finished.json")
			fi.objects = append(fi.objects, storage.ObjectAttrs{
				Name: p.Object(),
			})
			fc.fakeOpener[*p] = *build.finished
		}
		if len(build.passed)+len(build.failed) > 0 {
			p := resolveOrDie(buildPath, "junit_automatic.xml")
			fc.fakeOpener[*p] = fakeObject{data: makeJunit(build.passed, build.failed)}
			fi.objects = append(fi.objects, storage.ObjectAttrs{
				Name: p.Object(),
			})
		}
		for n, fo := range build.artifacts {
			p := resolveOrDie(buildPath, n)
			fi.objects = append(fi.objects, storage.ObjectAttrs{
				Name: p.Object(),
			})
			fc.fakeOpener[*p] = fo
		}
		fc.fakeLister[*buildPath] = fi
	}
	return builds

}

type fakeBuild struct {
	id        string
	started   *fakeObject
	finished  *fakeObject
	artifacts map[string]fakeObject
	rawJunit  *fakeObject
	passed    []string
	failed    []string
}
