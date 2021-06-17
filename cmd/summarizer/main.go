/*
Copyright 2020 The Kubernetes Authors.

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
	"runtime"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/GoogleCloudPlatform/testgrid/util/metrics"

	"github.com/GoogleCloudPlatform/testgrid/pkg/summarizer"
)

type options struct {
	config            gcs.Path // gcs://path/to/config/proto
	creds             string
	confirm           bool
	dashboard         string
	concurrency       int
	wait              time.Duration
	gridPathPrefix    string
	summaryPathPrefix string

	debug    bool
	trace    bool
	jsonLogs bool
}

func (o *options) validate() error {
	if o.config.String() == "" {
		return errors.New("empty --config")
	}
	if o.concurrency == 0 {
		o.concurrency = 4 * runtime.NumCPU()
	}
	return nil
}

func gatherOptions() options {
	var o options
	flag.Var(&o.config, "config", "gs://path/to/config.pb")
	flag.StringVar(&o.creds, "gcp-service-account", "", "/path/to/gcp/creds (use local creds if empty)")
	flag.BoolVar(&o.confirm, "confirm", false, "Upload data if set")
	flag.StringVar(&o.dashboard, "dashboard", "", "Only update named dashboard if set")
	flag.IntVar(&o.concurrency, "concurrency", 0, "Manually define the number of dashboards to concurrently update if non-zero")
	flag.DurationVar(&o.wait, "wait", 0, "Ensure at least this much time has passed since the last loop (exit if zero).")
	flag.StringVar(&o.gridPathPrefix, "grid-path", "", "Read grid states under this GCS path.")
	flag.StringVar(&o.summaryPathPrefix, "summary-path", "", "Write summaries under this GCS path.")

	flag.BoolVar(&o.debug, "debug", false, "Log debug lines if set")
	flag.BoolVar(&o.trace, "trace", false, "Log trace and debug lines if set")
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
		logrus.Info("--confirm=false (DRY-RUN): will not write to gcs")
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
		logrus.Fatalf("Failed to read storage client: %v", err)
	}

	client := gcs.NewClient(storageClient)
	mets := &summarizer.Metrics{
		Successes: metrics.NewLogCounter("successes", "Number of successful updates", logrus.New(), "component"),
		Errors:    metrics.NewLogCounter("errors", "Number of failed updates", logrus.New(), "component"),
	}
	updateOnce := func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		return summarizer.Update(ctx, client, mets, opt.config, opt.concurrency, opt.dashboard, opt.gridPathPrefix, opt.summaryPathPrefix, opt.confirm)
	}

	if err := updateOnce(ctx); err != nil {
		logrus.WithError(err).Error("Failed update")
	}
	if opt.wait == 0 {
		return
	}
	timer := time.NewTimer(opt.wait)
	defer timer.Stop()
	for range timer.C {
		timer.Reset(opt.wait)
		if err := updateOnce(ctx); err != nil {
			logrus.WithError(err).Error("Failed update")
		}
		logrus.WithField("wait", opt.wait).Info("Sleeping")
	}
}
