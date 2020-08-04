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
	"runtime"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/pkg/updater"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"

	"github.com/sirupsen/logrus"
)

// options configures the updater
type options struct {
	config           gcs.Path // gs://path/to/config/proto
	creds            string
	confirm          bool
	debug            bool
	group            string
	groupConcurrency int
	buildConcurrency int
	wait             time.Duration
	groupTimeout     time.Duration
	buildTimeout     time.Duration
	gridPrefix       string
	jsonLogs         bool
}

// validate ensures sane options
func (o *options) validate() error {
	if o.config.String() == "" {
		return errors.New("empty --config")
	}
	if o.config.Bucket() == "k8s-testgrid" && o.gridPrefix != "" && o.confirm {
		return fmt.Errorf("--config=%s: cannot write grid state to gs://k8s-testgrid", o.config)
	}
	if o.groupConcurrency == 0 {
		o.groupConcurrency = 4 * runtime.NumCPU()
	}
	if o.buildConcurrency == 0 {
		o.buildConcurrency = 4 * runtime.NumCPU()
	}

	return nil
}

// gatherOptions reads options from flags
func gatherOptions() options {
	var o options
	flag.Var(&o.config, "config", "gs://path/to/config.pb")
	flag.StringVar(&o.creds, "gcp-service-account", "", "/path/to/gcp/creds (use local creds if empty)")
	flag.BoolVar(&o.confirm, "confirm", false, "Upload data if set")
	flag.BoolVar(&o.debug, "debug", false, "Log debug lines if set")
	flag.StringVar(&o.group, "test-group", "", "Only update named group if set")
	flag.IntVar(&o.groupConcurrency, "group-concurrency", 0, "Manually define the number of groups to concurrently update if non-zero")
	flag.IntVar(&o.buildConcurrency, "build-concurrency", 0, "Manually define the number of builds to concurrently read if non-zero")
	flag.DurationVar(&o.wait, "wait", 0, "Ensure at least this much time has passed since the last loop (exit if zero).")
	flag.DurationVar(&o.groupTimeout, "group-timeout", 10*time.Minute, "Maximum time to wait for each group to update")
	flag.DurationVar(&o.buildTimeout, "build-timeout", 3*time.Minute, "Maximum time to wait to read each build")
	flag.StringVar(&o.gridPrefix, "grid-prefix", "grid", "Join this with the grid name to create the GCS suffix")
	flag.BoolVar(&o.jsonLogs, "json-logs", false, "Uses a json logrus formatter when set")
	flag.Parse()
	return o
}

func main() {
	opt := gatherOptions()
	if err := opt.validate(); err != nil {
		logrus.Fatalf("Invalid flags: %v", err)
	}
	if !opt.confirm {
		logrus.Warning("--confirm=false (DRY-RUN): will not write to gcs")
	}
	if opt.debug {
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

	updateOnce := func() {
		start := time.Now()
		if err := updater.Update(client, ctx, opt.config, opt.gridPrefix, opt.groupConcurrency, opt.buildConcurrency, opt.confirm, opt.groupTimeout, opt.buildTimeout, opt.group); err != nil {
			logrus.WithError(err).Error("Could not update")
		}
		logrus.Infof("Update completed in %s", time.Since(start))
	}

	updateOnce()
	if opt.wait == 0 {
		return
	}
	timer := time.NewTimer(opt.wait)
	defer timer.Stop()
	for range timer.C {
		timer.Reset(opt.wait)
		updateOnce()
		logrus.WithField("wait", opt.wait).Info("Sleeping...")
	}
}
