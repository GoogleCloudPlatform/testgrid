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
	"strings"
	"time"

	gpubsub "cloud.google.com/go/pubsub"
	"github.com/GoogleCloudPlatform/testgrid/pkg/pubsub"
	"github.com/GoogleCloudPlatform/testgrid/pkg/summarizer"
	"github.com/GoogleCloudPlatform/testgrid/util"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/GoogleCloudPlatform/testgrid/util/metrics/prometheus"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"
)

type options struct {
	config            gcs.Path // gcs://path/to/config/proto
	persistQueue      gcs.Path
	creds             string
	confirm           bool
	dashboards        util.Strings
	concurrency       int
	wait              time.Duration
	gridPathPrefix    string
	summaryPathPrefix string
	pubsub            string

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
	flag.Var(&o.persistQueue, "persist-queue", "Load previous queue state from gs://path/to/queue-state.json and regularly save to it thereafter")
	flag.StringVar(&o.creds, "gcp-service-account", "", "/path/to/gcp/creds (use local creds if empty)")
	flag.BoolVar(&o.confirm, "confirm", false, "Upload data if set")
	flag.Var(&o.dashboards, "dashboard", "Only update named dashboards if set (repeateable)")
	flag.IntVar(&o.concurrency, "concurrency", 0, "Manually define the number of dashboards to concurrently update if non-zero")
	flag.DurationVar(&o.wait, "wait", 0, "Ensure at least this much time has passed since the last loop (exit if zero).")
	flag.StringVar(&o.gridPathPrefix, "grid-path", "grid", "Read grid states under this GCS path.")
	flag.StringVar(&o.summaryPathPrefix, "summary-path", "summary", "Write summaries under this GCS path.")
	flag.StringVar(&o.pubsub, "pubsub", "", "listen for test group updates at project/subscription")

	flag.BoolVar(&o.debug, "debug", false, "Log debug lines if set")
	flag.BoolVar(&o.trace, "trace", false, "Log trace and debug lines if set")
	flag.BoolVar(&o.jsonLogs, "json-logs", false, "Uses a json logrus formatter when set")

	flag.Parse()
	return o
}

func gcsFixer(ctx context.Context, projectSub string, configPath gcs.Path, gridPrefix, credPath string) (summarizer.Fixer, error) {
	if projectSub == "" {
		return nil, nil
	}
	parts := strings.SplitN(projectSub, "/", 2)
	if len(parts) != 2 {
		return nil, errors.New("malformed project/subscription")
	}
	projID, subID := parts[0], parts[1]
	pubsubClient, err := gpubsub.NewClient(ctx, "", option.WithCredentialsFile(credPath))
	if err != nil {
		logrus.WithError(err).Fatal("Failed to create pubsub client")
	}
	client := pubsub.NewClient(pubsubClient)
	return summarizer.FixGCS(client, logrus.StandardLogger(), projID, subID, configPath, gridPrefix)
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
		logrus.WithError(err).Fatal("Failed to read storage client")
	}

	client := gcs.NewClient(storageClient)
	metrics := summarizer.CreateMetrics(prometheus.NewFactory())
	fixer, err := gcsFixer(ctx, opt.pubsub, opt.config, opt.gridPathPrefix, opt.creds)
	if err != nil {
		logrus.WithError(err).WithField("subscription", opt.pubsub).Fatal("Failed to configure pubsub")
	}

	fixers := []summarizer.Fixer{
		fixer,
	}

	if path := opt.persistQueue; path.String() != "" {
		const freq = time.Minute
		ticker := time.NewTicker(freq)
		log := logrus.WithField("frequency", freq)
		fixers = append(fixers, summarizer.FixPersistent(log, client, path, ticker.C))
	}

	if err := summarizer.Update(ctx, client, metrics, opt.config, opt.concurrency, opt.gridPathPrefix, opt.summaryPathPrefix, opt.dashboards.Strings(), opt.confirm, opt.wait, fixers...); err != nil {
		logrus.WithError(err).Error("Could not summarize")
	}
}
