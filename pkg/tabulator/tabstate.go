/*
Copyright 2022 The TestGrid Authors.

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

// Package tabulator processes test group state into tab state.
package tabulator

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"sync"
	"time"

	"bitbucket.org/creachadair/stringset"
	"github.com/sirupsen/logrus"

	"github.com/GoogleCloudPlatform/testgrid/config"
	"github.com/GoogleCloudPlatform/testgrid/config/snapshot"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	tspb "github.com/GoogleCloudPlatform/testgrid/pb/test_status"
	"github.com/GoogleCloudPlatform/testgrid/pkg/updater"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/GoogleCloudPlatform/testgrid/util/metrics"
)

const componentName = "tabulator"

// Metrics holds metrics relevant to this controller.
type Metrics struct {
	UpdateState  metrics.Cyclic
	DelaySeconds metrics.Duration
}

// CreateMetrics creates metrics for this controller
func CreateMetrics(factory metrics.Factory) *Metrics {
	return &Metrics{
		UpdateState:  factory.NewCyclic(componentName),
		DelaySeconds: factory.NewDuration("delay", "Seconds tabulator is behind schedule", "component"),
	}
}

type view struct {
	dashboard *configpb.Dashboard
	tab       *configpb.DashboardTab
}

func getViews(cfg *snapshot.Config) map[string][]view {
	groupToTabs := make(map[string][]view, len(cfg.Groups))

	for _, dashboard := range cfg.Dashboards {
		for _, tab := range dashboard.DashboardTab {
			g := tab.TestGroupName
			groupToTabs[g] = append(groupToTabs[g], view{dashboard, tab})
		}
	}

	return groupToTabs
}

// Fixer should adjust the queue until the context expires.
type Fixer func(context.Context, *config.TestGroupQueue) error

// Update tab state with the given frequency continuously. If freq == 0, runs only once.
//
// Copies the grid into the tab state, removing unneeded data.
// Observes each test group in allowedGroups, or all of them in the config if not specified
func Update(ctx context.Context, client gcs.ConditionalClient, mets *Metrics, configPath gcs.Path, concurrency int, gridPathPrefix, tabsPathPrefix string, allowedGroups []string, confirm, dropEmptyCols, calculateStats, useTabAlertSettings bool, freq time.Duration, fixers ...Fixer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if concurrency < 1 {
		return fmt.Errorf("concurrency must be positive, got: %d", concurrency)
	}
	log := logrus.WithField("config", configPath)

	var q config.TestGroupQueue

	log.Debug("Observing config...")
	cfgChanged, err := snapshot.Observe(ctx, log, client, configPath, time.NewTicker(time.Minute).C)
	if err != nil {
		return fmt.Errorf("error while observing config %q: %w", configPath.String(), err)
	}

	var cfg *snapshot.Config
	var groupToTabs map[string][]view
	fixSnapshot := func(newConfig *snapshot.Config) {
		cfg = newConfig
		groupToTabs = getViews(cfg)

		if len(allowedGroups) != 0 {
			groups := make([]*configpb.TestGroup, 0, len(allowedGroups))
			for _, group := range allowedGroups {
				c, ok := cfg.Groups[group]
				if !ok {
					log.Errorf("Could not find requested group %q in config", c)
					continue
				}
				groups = append(groups, c)
			}

			q.Init(log, groups, time.Now())
			return

		}

		groups := make([]*configpb.TestGroup, 0, len(cfg.Groups))
		for _, group := range cfg.Groups {
			groups = append(groups, group)
		}

		q.Init(log, groups, time.Now())
	}

	fixSnapshot(<-cfgChanged)

	go func(ctx context.Context) {
		fixCtx, fixCancel := context.WithCancel(ctx)
		var fixWg sync.WaitGroup
		fixAll := func() {
			n := len(fixers)
			log.WithField("fixers", n).Debug("Starting fixers on current groups...")
			fixWg.Add(n)
			for i, fix := range fixers {
				go func(i int, fix Fixer) {
					defer fixWg.Done()
					if err := fix(fixCtx, &q); err != nil && !errors.Is(err, context.Canceled) {
						log.WithError(err).WithField("fixer", i).Warning("Fixer failed")
					}
				}(i, fix)
			}
			log.WithField("fixers", n).Info("Started fixers on current groups.")
		}

		ticker := time.NewTicker(time.Minute)
		fixAll()
		defer ticker.Stop()
		for {
			depth, next, when := q.Status()
			log := log.WithField("depth", depth)
			if next != nil {
				log = log.WithField("next", &next)
			}
			delay := time.Since(when)
			if delay < 0 {
				delay = 0
				log = log.WithField("sleep", -delay)
			}
			mets.DelaySeconds.Set(delay, componentName)
			log.Debug("Calculated metrics")

			select {
			case <-ctx.Done():
				ticker.Stop()
				fixCancel()
				fixWg.Wait()
				return
			case newConfig := <-cfgChanged:
				log.Info("Configuration changed")
				fixCancel()
				fixWg.Wait()
				fixCtx, fixCancel = context.WithCancel(ctx)
				fixSnapshot(newConfig)
				fixAll()
			case <-ticker.C:
			}
		}
	}(ctx)

	// Set up threads
	var active stringset.Set
	var waiting stringset.Set
	var lock sync.Mutex

	channel := make(chan *configpb.TestGroup)

	update := func(log *logrus.Entry, group *configpb.TestGroup) error {
		for _, view := range groupToTabs[group.Name] {
			log := log.WithField("dashboard", view.dashboard.Name).WithField("tab", view.tab.Name)
			fromPath, err := updater.TestGroupPath(configPath, gridPathPrefix, view.tab.TestGroupName)
			if err != nil {
				return fmt.Errorf("can't make tg path %q: %w", view.tab.TestGroupName, err)
			}
			toPath, err := TabStatePath(configPath, tabsPathPrefix, view.dashboard.Name, view.tab.Name)
			if err != nil {
				return fmt.Errorf("can't make dashtab path %s/%s: %w", view.dashboard.Name, view.tab.Name, err)
			}
			log.WithFields(logrus.Fields{
				"from": fromPath.String(),
				"to":   toPath.String(),
			}).Info("Calculating state")

			err = createTabState(ctx, log, client, view.tab, group, *fromPath, *toPath, confirm, dropEmptyCols, calculateStats, useTabAlertSettings)
			if err != nil {
				return fmt.Errorf("can't calculate state: %w", err)
			}
		}
		return nil
	}

	// Run threads continuously
	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			for group := range channel {
				lock.Lock()
				start := active.Add(group.Name)
				if !start {
					waiting.Add(group.Name)
				}
				lock.Unlock()
				if !start {
					continue
				}

				log := log.WithField("group", group.Name)
				finish := mets.UpdateState.Start()

				if err := update(log, group); err != nil {
					finish.Fail()
					retry := time.Now().Add(freq / 10)
					q.Fix(group.Name, retry, true)
					log.WithError(err).WithField("retry-at", retry).Error("Failed to generate tab state")
				} else {
					finish.Success()
					log.Info("Built tab state")
				}

				lock.Lock()
				active.Discard(group.Name)
				restart := waiting.Discard(group.Name)
				lock.Unlock()
				if restart {
					q.Fix(group.Name, time.Now(), false)
				}
			}
		}()
	}
	defer wg.Wait()
	defer close(channel)

	return q.Send(ctx, channel, freq)
}

// TabStatePath returns the path for a given tab.
func TabStatePath(configPath gcs.Path, tabPrefix, dashboardName, tabName string) (*gcs.Path, error) {
	name := path.Join(tabPrefix, dashboardName, tabName)
	u, err := url.Parse(name)
	if err != nil {
		return nil, fmt.Errorf("invalid url %s: %w", name, err)
	}
	np, err := configPath.ResolveReference(u)
	if err != nil {
		return nil, fmt.Errorf("resolve reference: %w", err)
	}
	if np.Bucket() != configPath.Bucket() {
		return nil, fmt.Errorf("tabState %s should not change bucket", name)
	}
	return np, nil
}

// tabulate cuts the passed-in grid down to only the part that needs to be displayed by the UI.
func tabulate(ctx context.Context, log logrus.FieldLogger, grid *statepb.Grid, tabCfg *configpb.DashboardTab, groupCfg *configpb.TestGroup, dropEmptyCols, calculateStats, useTabAlertSettings bool) (*statepb.Grid, error) {
	filterRows, err := filterGrid(tabCfg.GetBaseOptions(), grid.GetRows())
	if err != nil {
		return nil, fmt.Errorf("filterGrid: %w", err)
	}
	grid.Rows = filterRows

	if dropEmptyCols {
		// TODO(chases2): Instead of inflate/drop/rewrite, move to inflate/drop/append
		inflatedGrid, issues, err := updater.InflateGrid(ctx, grid, time.Time{}, time.Now())
		if err != nil {
			return nil, fmt.Errorf("inflateGrid: %w", err)
		}

		inflatedGrid = dropEmptyColumns(inflatedGrid)

		usesK8sClient := groupCfg.UseKubernetesClient || (groupCfg.GetResultSource().GetGcsConfig() != nil)
		var brokenThreshold float32
		if calculateStats {
			brokenThreshold = tabCfg.GetBrokenColumnThreshold()
		}
		var alert, unalert int
		if useTabAlertSettings {
			alert = int(tabCfg.GetAlertOptions().GetNumFailuresToAlert())
			unalert = int(tabCfg.GetAlertOptions().GetNumPassesToDisableAlert())
		} else {
			alert = int(groupCfg.GetNumFailuresToAlert())
			unalert = int(groupCfg.GetNumPassesToDisableAlert())
		}
		grid = updater.ConstructGrid(log, inflatedGrid, issues, alert, unalert, usesK8sClient, groupCfg.GetUserProperty(), brokenThreshold)
	}
	return grid, nil
}

// createTabState creates the tab state from the group state
func createTabState(ctx context.Context, log logrus.FieldLogger, client gcs.Client, dashCfg *configpb.DashboardTab, groupCfg *configpb.TestGroup, testGroupPath, tabStatePath gcs.Path, confirm, dropEmptyCols, calculateStats, useTabAlerts bool) error {
	rawGrid, _, err := gcs.DownloadGrid(ctx, client, testGroupPath)
	if err != nil {
		return fmt.Errorf("downloadGrid(%s): %w", testGroupPath, err)
	}

	grid, err := tabulate(ctx, log, rawGrid, dashCfg, groupCfg, dropEmptyCols, calculateStats, useTabAlerts)
	if err != nil {
		return fmt.Errorf("tabulate: %w", err)
	}

	if !confirm {
		logrus.Debug("Successfully created tab state; discarding")
		return nil
	}

	buf, err := gcs.MarshalGrid(grid)
	if err != nil {
		return fmt.Errorf("marshalGrid: %w", err)
	}

	_, err = client.Upload(ctx, tabStatePath, buf, gcs.DefaultACL, gcs.NoCache)
	if err != nil {
		return fmt.Errorf("client.Upload(%s): %w", tabStatePath, err)
	}
	return nil

}

// dropEmptyColumns drops every column in-place that has no results
func dropEmptyColumns(grid []updater.InflatedColumn) []updater.InflatedColumn {
	result := make([]updater.InflatedColumn, 0, len(grid))
	for i, col := range grid {
		for _, cell := range col.Cells {
			if cell.Result != tspb.TestStatus_NO_RESULT {
				result = append(result, grid[i])
				break
			}
		}
	}
	if len(result) == 0 && len(grid) != 0 {
		// If everything would be dropped, keep the first column so there's something left
		result = grid[0:1]
	}
	return result
}
