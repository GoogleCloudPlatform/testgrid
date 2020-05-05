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

package alerter

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/testgrid/config"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	summarypb "github.com/GoogleCloudPlatform/testgrid/pb/summary"
	"github.com/GoogleCloudPlatform/testgrid/pkg/summarizer"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/golang/protobuf/proto"
	"github.com/sirupsen/logrus"
)

// Read summary protos and calls functions ("Klaxons") on certain conditions
// Maintains a copy of summary protos to recognize last read state after reboot
func Update(ctx context.Context, client *storage.Client, path gcs.Path, concurrency int, dashboard string, ConsistentFailure, Stale Klaxon, confirm bool) error {
	if concurrency < 1 {
		return fmt.Errorf("concurrency must be positive, got: %d", concurrency)
	}

	// get config
	cfg, err := config.ReadGCS(ctx, client.Bucket(path.Bucket()).Object(path.Object()))
	if err != nil {
		return fmt.Errorf("Failed to read config: %w", err)
	}
	logrus.Infof("Found %d dashboards", len(cfg.Dashboards))

	dashboards := make(chan *configpb.Dashboard)
	var wg sync.WaitGroup

	errCh := make(chan error)

	// Launch monitoring goroutines
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for dash := range dashboards {
				// Read the new summary file from the expected location (summary)
				// If it's not there, alerts can't be generated for this dashboard
				newPath, err := path.ResolveReference(&url.URL{Path: summarizer.SummaryPath(dash.Name)})
				if err != nil {
					logrus.Errorf("Could not resolve path to current summaries of %s", dash.Name)
					errCh <- err
					continue
				}

				newSummary, err := readSummary(ctx, client, *newPath)
				if err != nil {
					logrus.Errorf("Could not read summary from %v", newPath)
					errCh <- err
					continue
				}

				// Read the old summary file from the expected location (alerter)
				// It may not be there the first time this is run
				oldPath, err := path.ResolveReference(&url.URL{Path: alerterPath(dash.Name)})
				if err != nil {
					logrus.Errorf("Could not resolve path to previous summaries of %s", dash.Name)
					errCh <- err
					continue
				}

				oldSummary, err := readSummary(ctx, client, *oldPath)
				if err != nil {
					logrus.Warnf("Could not read summary from %v; trying write", oldPath)
				}

				if !confirm {
					logrus.Infof("Summarizers read for dashboard %s (dry-run)", dash.Name)
					continue
				}

				// Detect changes and call the Klaxon function(s) if those changes are pertinent
				if oldSummary != nil {
					callOutDifferences(ctx, newSummary, oldSummary, dash, ConsistentFailure, Stale)
				}

				// Write the new summary to where the old summary was
				err = writeSummary(ctx, client, *oldPath, newSummary)
				if err != nil {
					logrus.Errorf("Could not write summary to %v", oldPath)
					errCh <- err
					continue
				}
				errCh <- nil
			}
		}()
	}

	// Subroutine to concatenate errors together
	resultCh := make(chan error)
	go func() {
		defer close(resultCh)
		var errs []string
		for err := range errCh {
			if err == nil {
				continue
			}
			errs = append(errs, err.Error())
		}
		if n := len(errs); n > 0 {
			resultCh <- fmt.Errorf("failed to update %d dashboards: %v", n, strings.Join(errs, ", "))
		}
		resultCh <- nil
	}()

	// Queue up all the dashboards
	for _, d := range cfg.Dashboards {
		if dashboard != "" && dashboard != d.Name {
			logrus.WithField("dashboard", d.Name).Info("Skipping")
			continue
		}
		dashboards <- d
	}
	close(dashboards)
	wg.Wait() //wait for them to stop reporting
	close(errCh)
	return <-resultCh
}

// Paths to relevant GCS files
var (
	normalizer = regexp.MustCompile(`[^a-z0-9]+`)
)

func alerterPath(name string) string {
	return "alerter-" + normalizer.ReplaceAllString(strings.ToLower(name), "")
}

func writeSummary(ctx context.Context, client *storage.Client, path gcs.Path, sum *summarypb.DashboardSummary) error {
	buf, err := proto.Marshal(sum)
	if err != nil {
		return fmt.Errorf("marshal: %v", err)
	}
	return gcs.Upload(ctx, client, path, buf, gcs.DefaultAcl, "no-cache")
}

func readSummary(ctx context.Context, client *storage.Client, path gcs.Path) (*summarypb.DashboardSummary, error) {
	r, err := client.Bucket(path.Bucket()).Object(path.Object()).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to open config: %v", err)
	}
	buf, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %v", err)
	}
	var summary summarypb.DashboardSummary
	if err = proto.Unmarshal(buf, &summary); err != nil {
		return nil, fmt.Errorf("failed to parse: %v", err)
	}
	return &summary, nil
}

func callOutDifferences(ctx context.Context, new, old *summarypb.DashboardSummary, config *configpb.Dashboard, ConsistentFailure, Stale Klaxon) {
	// Deserialize dashboard summaries into maps for easy access
	oldSummaries := map[string]*summarypb.DashboardTabSummary{}
	newSummaries := map[string]*summarypb.DashboardTabSummary{}

	for _, tabSummary := range old.TabSummaries {
		oldSummaries[tabSummary.DashboardTabName] = tabSummary
	}

	for _, tabSummary := range new.TabSummaries {
		newSummaries[tabSummary.DashboardTabName] = tabSummary
	}

	for _, dashboardTab := range config.DashboardTab {
		newSummary := newSummaries[dashboardTab.Name]
		oldSummary := oldSummaries[dashboardTab.Name]

		if newSummary.OverallStatus == summarypb.DashboardTabSummary_STALE && oldSummary.OverallStatus != summarypb.DashboardTabSummary_STALE {
			err := Stale(ctx, newSummary, oldSummary, dashboardTab)
			if err != nil {
				logrus.Errorf("OnBecomingStale Dashboard %s Error: %v", dashboardTab.Name, err)
			}
		}

		alertThreshold := dashboardTab.AlertOptions.NumFailuresToAlert

		for _, failingTest := range newSummary.FailingTestSummaries {
			if failingTest.FailCount == alertThreshold {
				maySendAlert := true
				for _, oldFailingTest := range oldSummary.FailingTestSummaries {
					if failingTest.TestName == oldFailingTest.TestName && oldFailingTest.FailCount == alertThreshold {
						maySendAlert = false
					}
				}
				if maySendAlert {
					err := ConsistentFailure(ctx, newSummary, oldSummary, dashboardTab)
					if err != nil {
						logrus.Errorf("OnConsistentFailure Dashboard %s Error: %v", dashboardTab.Name, err)
					}
				}
			}
		}
	}
}

// A Klaxon is a function called when something worth alerting happens.
// ex. "Send an email", "Write a message to Slack", etc.
// The new summary is first, then the old summary, then the configuration.
type Klaxon func(context.Context, *summarypb.DashboardTabSummary, *summarypb.DashboardTabSummary, *configpb.DashboardTab) error
