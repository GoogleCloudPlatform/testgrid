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
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/pkg/pubsub"
	"github.com/GoogleCloudPlatform/testgrid/pkg/updater"
	"github.com/GoogleCloudPlatform/testgrid/pkg/updater/resultstore"
	"github.com/GoogleCloudPlatform/testgrid/util"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/GoogleCloudPlatform/testgrid/util/metrics/prometheus"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"

	gpubsub "cloud.google.com/go/pubsub"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
)

// options configures the updater
type options struct {
	config            gcs.Path // gs://path/to/config/proto
	persistQueue      gcs.Path
	creds             string
	confirm           bool
	groups            util.Strings
	groupConcurrency  int
	buildConcurrency  int
	wait              time.Duration
	groupTimeout      time.Duration
	buildTimeout      time.Duration
	gridPrefix        string
	subscriptions     util.Strings
	reprocessList     util.Strings
	enableIgnoreSkip  bool
	enableResultStore bool

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
		o.buildConcurrency = o.groupConcurrency * 4
	}

	return subscribeGCS(o.subscriptions.Strings()...)
}

func subscribeGCS(subs ...string) error {
	for _, sub := range subs {
		parts := strings.SplitN(sub, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("--subscribe format is prefix=proj/sub, got %q", sub)
		}
		prefix := parts[0]
		parts = strings.SplitN(parts[1], "/", 2)
		if len(parts) != 2 {
			return fmt.Errorf("--subscribe format is prefix=proj/sub, got %q", sub)
		}
		proj, sub := parts[0], parts[1]
		updater.AddManualSubscription(proj, sub, prefix)
	}
	return nil
}

// gatherOptions reads options from flags
func gatherFlagOptions(fs *flag.FlagSet, args ...string) options {
	var o options
	fs.Var(&o.config, "config", "gs://path/to/config.pb")
	fs.Var(&o.persistQueue, "persist-queue", "Load previous queue state from gs://path/to/queue-state.json and regularly save to it thereafter")
	fs.StringVar(&o.creds, "gcp-service-account", "", "/path/to/gcp/creds (use local creds if empty)")
	fs.BoolVar(&o.confirm, "confirm", false, "Upload data if set")
	fs.Var(&o.groups, "test-group", "Only update named groups if set (repeatable)")
	fs.IntVar(&o.groupConcurrency, "group-concurrency", 0, "Manually define the number of groups to concurrently update if non-zero")
	fs.IntVar(&o.buildConcurrency, "build-concurrency", 0, "Manually define the number of builds to concurrently read if non-zero")
	fs.DurationVar(&o.wait, "wait", 0, "Ensure at least this much time has passed since the last loop (exit if zero).")
	fs.Var(&o.subscriptions, "subscribe", "gcs-prefix=project-id/sub-id (repeatable)")
	fs.Var(&o.reprocessList, "reprocess-group-on-change", "Limit reprocessing to specific groups if set (repeatable)")

	fs.DurationVar(&o.groupTimeout, "group-timeout", 10*time.Minute, "Maximum time to wait for each group to update")
	fs.DurationVar(&o.buildTimeout, "build-timeout", 3*time.Minute, "Maximum time to wait to read each build")
	fs.StringVar(&o.gridPrefix, "grid-prefix", "grid", "Join this with the grid name to create the GCS suffix")

	fs.BoolVar(&o.enableIgnoreSkip, "enable-ignore-skip", false, "If true, enable ignore_skip behavior.")
	fs.BoolVar(&o.enableResultStore, "enable-resultstore", false, "If true, fetch results from ResultStore.")

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

type resultSource string

const (
	gcsSource         resultSource = "GCS"
	resultStoreSource resultSource = "ResultStore"
	unknownSource     resultSource = "unknown"
)

func source(tg *configpb.TestGroup) resultSource {
	if tg.GetUseKubernetesClient() || tg.GetResultSource().GetGcsConfig() != nil {
		return gcsSource
	}
	if tg.GetResultSource().GetResultstoreConfig() != nil {
		return resultStoreSource
	}
	return unknownSource
}

func updateGroup(updateGCS, updateResultStore updater.GroupUpdater) updater.GroupUpdater {
	return func(parent context.Context, log logrus.FieldLogger, client gcs.Client, tg *configpb.TestGroup, gridPath gcs.Path) (bool, error) {
		source := source(tg)
		switch source {
		case gcsSource:
			return updateGCS(parent, log, client, tg, gridPath)
		case resultStoreSource:
			return updateResultStore(parent, log, client, tg, gridPath)
		default:
			return false, errors.New("invalid result source (must be one of GCS, ResultStore)")
		}
	}
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
		logrus.WithError(err).Fatal("Failed to create storage client")
	}
	defer storageClient.Close()

	client := gcs.NewClient(storageClient)

	log := logrus.WithFields(logrus.Fields{
		"group": opt.groupConcurrency,
		"build": opt.buildConcurrency,
	})
	log.Info("Configured concurrency")

	var rsClient *resultstore.DownloadClient
	if opt.enableResultStore {
		rsConn, err := resultstore.Connect(ctx, "")
		if err != nil {
			logrus.WithError(err).Fatal("Failed to connect to ResultStore.")
		}
		rsClient = resultstore.NewClient(rsConn)
	}

	updateGCS := updater.GCS(ctx, client, opt.groupTimeout, opt.buildTimeout, opt.buildConcurrency, opt.confirm, opt.enableIgnoreSkip)
	updateResultStore := resultstore.Updater(rsClient, client, opt.groupTimeout, opt.confirm)
	updateAll := updateGroup(updateGCS, updateResultStore)

	mets := updater.CreateMetrics(prometheus.NewFactory())

	pubsubClient, err := gpubsub.NewClient(ctx, gpubsub.DetectProjectID, option.WithCredentialsFile(opt.creds))
	if err != nil {
		logrus.WithError(err).Fatal("Failed to create pubsub client")
	}

	fixers := []updater.Fixer{
		updater.FixGCS(pubsub.NewClient(pubsubClient)),
	}

	if path := opt.persistQueue; path.String() != "" {
		const freq = time.Minute
		ticker := time.NewTicker(freq)
		log := logrus.WithField("frequency", freq)
		fixers = append(fixers, updater.FixPersistent(log, client, path, ticker.C))
	}

	opts := &updater.UpdateOptions{
		ConfigPath:       opt.config,
		GridPrefix:       opt.gridPrefix,
		GroupConcurrency: opt.groupConcurrency,
		GroupNames:       opt.groups.Strings(),
		Write:            opt.confirm,
		Freq:             opt.wait,
	}

	if err := updater.Update(ctx, client, mets, updateAll, opts, fixers...); err != nil {
		logrus.WithError(err).Error("Could not update")
	}
}
