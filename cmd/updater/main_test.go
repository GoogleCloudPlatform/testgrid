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

package main

import (
	"flag"
	"runtime"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/GoogleCloudPlatform/testgrid/util"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
)

func newPathOrDie(s string) *gcs.Path {
	p, err := gcs.NewPath(s)
	if err != nil {
		panic(err)
	}
	return p
}

func TestGatherFlagOptions(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		expected func(*options)
		err      bool
	}{
		{
			name: "config is required",
			err:  true,
		},
		{
			name: "basically works",
			args: []string{"--config=gs://bucket/whatever"},
			expected: func(o *options) {
				o.config = *newPathOrDie("gs://bucket/whatever")
			},
		},
		{
			name: "allow --config=gs://k8s-testgrid/config with default grid prefix",
			args: []string{
				"--config=gs://k8s-testgrid/config",
				"--confirm",
			},
			expected: func(o *options) {
				o.config = *newPathOrDie("gs://k8s-testgrid/config")
				o.confirm = true
			},
		},
		{
			name: "allow --config=gs://k8s-testgrid/config --grid-prefix= without confirm",
			args: []string{
				"--config=gs://k8s-testgrid/config",
				"--grid-prefix=",
			},
			expected: func(o *options) {
				o.config = *newPathOrDie("gs://k8s-testgrid/config")
				o.gridPrefix = ""
			},
		},
		{
			name: "allow --config=gs://k8s-testgrid/config with random grid prefix",
			args: []string{
				"--config=gs://k8s-testgrid/config",
				"--grid-prefix=random",
				"--confirm",
			},
			expected: func(o *options) {
				o.config = *newPathOrDie("gs://k8s-testgrid/config")
				o.gridPrefix = "random"
				o.confirm = true
			},
		},
		{
			name: "reject --config=gs://k8s-testgrid/config --grid-prefix=",
			args: []string{
				"--config=gs://k8s-testgrid/config",
				"--grid-prefix=",
				"--confirm",
			},
			err: true,
		},
		{
			name: "allow --config=gs://random/location --grid-prefix=",
			args: []string{
				"--config=gs://random/location",
				"--grid-prefix=",
				"--confirm",
			},
			expected: func(o *options) {
				o.config = *newPathOrDie("gs://random/location")
				o.gridPrefix = ""
				o.confirm = true
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cores := runtime.NumCPU()
			expected := options{
				buildTimeout:      3 * time.Minute,
				buildConcurrency:  cores * 4,
				groupConcurrency:  cores,
				groupTimeout:      10 * time.Minute,
				gridPrefix:        "grid",
				reprocessOnChange: true,
				pooled:            true,
			}
			if tc.expected != nil {
				tc.expected(&expected)
			}
			actual := gatherFlagOptions(flag.NewFlagSet(tc.name, flag.ContinueOnError), tc.args...)
			switch err := actual.validate(); {
			case err != nil:
				if !tc.err {
					t.Errorf("validate() got an unexpected error: %v", err)
				}
			case tc.err:
				t.Error("validate() failed to return an error")
			default:
				if diff := cmp.Diff(expected, actual, cmp.AllowUnexported(options{}, gcs.Path{}), cmp.AllowUnexported(options{}, util.Strings{})); diff != "" {
					t.Fatalf("gatherFlagOptions() got unexpected diff (-want +got):\n%s", diff)
				}

			}
		})
	}
}
