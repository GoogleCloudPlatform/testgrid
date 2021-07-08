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

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/pkg/updater"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/GoogleCloudPlatform/testgrid/util/metrics"
	"github.com/sirupsen/logrus"
)

// options configures the updater
type options struct {
	config           gcs.Path // gs://path/to/config/proto
	creds            string
	confirm          bool
	group            string
	groupConcurrency int
	buildConcurrency int
	wait             time.Duration
	groupTimeout     time.Duration
	buildTimeout     time.Duration
	gridPrefix       string

	debug    bool
	trace    bool
	jsonLogs bool
}

// validate ensures sane options
func (o *options) validate() error {
	if o.config.String() == "" {
		return errors.New("empty --config")
	}
	if o.config.Bucket() == "k8s-testgrid" && o.gridPrefix == "" && o.confirm {
		return fmt.Errorf("--config=%s: cannot write grid state to gs://k8s-testgrid", o.config)
	}
	if o.groupConcurrency == 0 {
		o.groupConcurrency = runtime.NumCPU()
	}
	if o.buildConcurrency == 0 {
		o.buildConcurrency = runtime.NumCPU()
		if o.buildConcurrency > 4 {
			o.buildConcurrency = 4
		}
	}

	return nil
}

// gatherOptions reads options from flags
func gatherFlagOptions(fs *flag.FlagSet, args ...string) options {
	var o options
	fs.Var(&o.config, "config", "gs://path/to/config.pb")
	fs.StringVar(&o.creds, "gcp-service-account", "", "/path/to/gcp/creds (use local creds if empty)")
	fs.BoolVar(&o.confirm, "confirm", false, "Upload data if set")
	fs.StringVar(&o.group, "test-group", "", "Only update named group if set")
	fs.IntVar(&o.groupConcurrency, "group-concurrency", 0, "Manually define the number of groups to concurrently update if non-zero")
	fs.IntVar(&o.buildConcurrency, "build-concurrency", 0, "Manually define the number of builds to concurrently read if non-zero")
	fs.DurationVar(&o.wait, "wait", 0, "Ensure at least this much time has passed since the last loop (exit if zero).")
	fs.DurationVar(&o.groupTimeout, "group-timeout", 10*time.Minute, "Maximum time to wait for each group to update")
	fs.DurationVar(&o.buildTimeout, "build-timeout", 3*time.Minute, "Maximum time to wait to read each build")
	fs.StringVar(&o.gridPrefix, "grid-prefix", "grid", "Join this with the grid name to create the GCS suffix")

	fs.BoolVar(&o.debug, "debug", false, "Log debug lines if set")
	fs.BoolVar(&o.trace, "trace", false, "Log trace and debug lines if set")
	fs.BoolVar(&o.jsonLogs, "json-logs", false, "Uses a json logrus formatter when set")

	fs.Parse(args)
	return o
}

// gatherOptions reads options from flags
func gatherOptions() options {
	return gatherFlagOptions(flag.CommandLine, os.Args[1:]...)
}

func main() {
	opt := gatherOptions()
	if err := opt.validate(); err != nil {
		logrus.Fatalf("Invalid flags: %v", err)
	}
	if !opt.confirm {
		logrus.Warning("--confirm=false (DRY-RUN): will not write to gcs")
	}
	switch {
	case opt.trace:
		logrus.SetLevel(logrus.TraceLevel)
	case opt.debug:
		logrus.SetLevel(logrus.DebugLevel)
	}

	if opt.jsonLogs {
		logrus.SetFormatter(&logrus.JSONFormatter{})
	}
	logrus.SetReportCaller(true)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	storageClient, err := gcs.ClientWithCreds(ctx, opt.creds)
	if err != nil {
		logrus.Fatalf("Failed to create storage client: %v", err)
	}
	defer storageClient.Close()

	client := gcs.NewClient(storageClient)

	logrus.WithFields(logrus.Fields{
		"group": opt.groupConcurrency,
		"build": opt.buildConcurrency,
	}).Info("Configured concurrency")

	groupUpdater := updater.GCS(opt.groupTimeout, opt.buildTimeout, opt.buildConcurrency, opt.confirm, updater.SortStarted)
	mets := &updater.Metrics{
		Successes:    metrics.NewLogCounter("successes", "Number of successful updates", logrus.New(), "component"),
		Errors:       metrics.NewLogCounter("errors", "Number of failed updates", logrus.New(), "component"),
		Skips:        metrics.NewLogCounter("skips", "Number of skipped updated", logrus.New(), "component"),
		DelaySeconds: metrics.NewLogInt64("delay", "Seconds updater is behind schedule", logrus.New(), "component"),
		CycleSeconds: metrics.NewLogInt64("cycle", "Seconds updater takes to update a group", logrus.New(), "component"),
	}

	if err := updater.Update(ctx, client, mets, opt.config, opt.gridPrefix, opt.groupConcurrency, opt.group, groupUpdater, opt.confirm, opt.wait); err != nil {
		logrus.WithError(err).Error("Could not update")
	}
}
