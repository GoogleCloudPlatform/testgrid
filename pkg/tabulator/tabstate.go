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

	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"

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
const writeTimeout = 10 * time.Minute

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

type writeTask struct {
	dashboard *configpb.Dashboard
	tab       *configpb.DashboardTab
	group     *configpb.TestGroup
	data      *statepb.Grid //TODO(chases2): change to inflatedColumns (and additional data) now that "filter-columns" is used everywhere
}

func mapTasks(cfg *snapshot.Config) map[string][]writeTask {
	groupToTabs := make(map[string][]writeTask, len(cfg.Groups))

	for _, dashboard := range cfg.Dashboards {
		for _, tab := range dashboard.DashboardTab {
			g := tab.TestGroupName
			groupToTabs[g] = append(groupToTabs[g], writeTask{
				dashboard: dashboard,
				tab:       tab,
				group:     cfg.Groups[g],
			})
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
func Update(ctx context.Context, client gcs.ConditionalClient, mets *Metrics, configPath gcs.Path, readConcurrency, writeConcurrency int, gridPathPrefix, tabsPathPrefix string, allowedGroups []string, confirm, calculateStats, useTabAlertSettings, extendState bool, freq time.Duration, fixers ...Fixer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if readConcurrency < 1 || writeConcurrency < 1 {
		return fmt.Errorf("concurrency must be positive, got read %d and write %d", readConcurrency, writeConcurrency)
	}
	log := logrus.WithField("config", configPath)

	var q config.TestGroupQueue

	log.Debug("Observing config...")
	cfgChanged, err := snapshot.Observe(ctx, log, client, configPath, time.NewTicker(time.Minute).C)
	if err != nil {
		return fmt.Errorf("error while observing config %q: %w", configPath.String(), err)
	}

	var cfg *snapshot.Config
	var tasksPerGroup map[string][]writeTask
	fixSnapshot := func(newConfig *snapshot.Config) {
		cfg = newConfig
		tasksPerGroup = mapTasks(cfg)

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
			case newConfig, ok := <-cfgChanged:
				if !ok {
					log.Info("Configuration channel closed")
					cfgChanged = nil
					continue
				}
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

	// Set up worker pools
	groups := make(chan *configpb.TestGroup)
	tasks := make(chan writeTask)
	var tabLock sync.Mutex

	read := func(ctx context.Context, log *logrus.Entry, group *configpb.TestGroup) error {
		if group == nil {
			return errors.New("nil group to read")
		}

		fromPath, err := updater.TestGroupPath(configPath, gridPathPrefix, group.Name)
		if err != nil {
			return fmt.Errorf("can't make tg path %q: %w", group.Name, err)
		}

		log.WithField("from", fromPath.String()).Info("Reading state")

		grid, _, err := gcs.DownloadGrid(ctx, client, *fromPath)
		if err != nil {
			return fmt.Errorf("downloadGrid(%s): %w", fromPath, err)
		}

		tabLock.Lock()
		defer tabLock.Unlock()
		// lock out all other readers so that all these tabs get handled as soon as possible
		for _, task := range tasksPerGroup[group.Name] {
			log := log.WithFields(logrus.Fields{
				"group":     task.group.GetName(),
				"dashboard": task.dashboard.GetName(),
				"tab":       task.tab.GetName(),
			})
			select {
			case <-ctx.Done():
				log.Debug("Skipping irrelevant task")
				continue
			default:
				out := task
				out.data = proto.Clone(grid).(*statepb.Grid)
				log.Debug("Requesting write task")
				tasks <- out
			}
		}
		return nil
	}

	// Run threads continuously
	var readWg, writeWg sync.WaitGroup
	readWg.Add(readConcurrency)
	for i := 0; i < readConcurrency; i++ {
		go func() {
			defer readWg.Done()
			for group := range groups {
				readCtx, cancel := context.WithCancel(ctx)
				log = log.WithField("group", group.Name)
				err := read(readCtx, log, group)
				cancel()
				if err != nil {
					next := time.Now().Add(freq / 10)
					q.Fix(group.Name, next, false)
					log.WithError(err).WithField("retry-at", next).Error("failed to read, retry later")
				}
			}
		}()
	}
	writeWg.Add(writeConcurrency)
	for i := 0; i < writeConcurrency; i++ {
		go func() {
			defer writeWg.Done()
			for task := range tasks {
				writeCtx, cancel := context.WithTimeout(ctx, writeTimeout)
				finish := mets.UpdateState.Start()
				log = log.WithField("dashboard", task.dashboard.Name).WithField("tab", task.tab.Name)
				err := createTabState(writeCtx, log, client, task, configPath, tabsPathPrefix, confirm, calculateStats, useTabAlertSettings, extendState)
				cancel()
				if err != nil {
					finish.Fail()
					log.Errorf("write: %v", err)
					continue
				}
				finish.Success()
			}
		}()
	}

	defer writeWg.Wait()
	defer close(tasks)
	defer readWg.Wait()
	defer close(groups)

	return q.Send(ctx, groups, freq)
}

// createTabState creates the tab state from the group state
func createTabState(ctx context.Context, log logrus.FieldLogger, client gcs.Client, task writeTask, configPath gcs.Path, tabsPathPrefix string, confirm, calculateStats, useTabAlerts, extendState bool) error {
	location, err := TabStatePath(configPath, tabsPathPrefix, task.dashboard.Name, task.tab.Name)
	if err != nil {
		return fmt.Errorf("can't make dashtab path %s/%s: %w", task.dashboard.Name, task.tab.Name, err)
	}

	log.WithFields(logrus.Fields{
		"to": location.String(),
	}).Info("Calculating state")

	var existingGrid *statepb.Grid
	if extendState {
		// TODO(chases2): Download grid only if task.Data was truncated (last column is UNKNOWN)
		existingGrid, _, err = gcs.DownloadGrid(ctx, client, *location)
		if err != nil {
			return fmt.Errorf("downloadGrid: %w", err)
		}
	}

	grid, err := tabulate(ctx, log, task.data, task.tab, task.group, calculateStats, useTabAlerts, existingGrid)
	if err != nil {
		return fmt.Errorf("tabulate: %w", err)
	}

	if !confirm {
		log.Debug("Successfully created tab state; discarding")
		return nil
	}

	buf, err := gcs.MarshalGrid(grid)
	if err != nil {
		return fmt.Errorf("marshalGrid: %w", err)
	}

	_, err = client.Upload(ctx, *location, buf, gcs.DefaultACL, gcs.NoCache)
	if err != nil {
		return fmt.Errorf("client.Upload(%s): %w", location, err)
	}
	return nil
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

// tabulate transforms "grid" to only the part that needs to be displayed by the UI.
// If an existingGrid is passed in, new results from "grid" will be grafted onto it.
func tabulate(ctx context.Context, log logrus.FieldLogger, grid *statepb.Grid, tabCfg *configpb.DashboardTab, groupCfg *configpb.TestGroup, calculateStats, useTabAlertSettings bool, existingGrid *statepb.Grid) (*statepb.Grid, error) {
	if grid == nil {
		return nil, errors.New("no grid")
	}
	if tabCfg == nil || groupCfg == nil {
		return nil, errors.New("no config")
	}
	filterRows, err := filterGrid(tabCfg.BaseOptions, grid.Rows)
	if err != nil {
		return nil, fmt.Errorf("filterGrid: %w", err)
	}
	grid.Rows = filterRows

	inflatedGrid, issues, err := updater.InflateGrid(ctx, grid, time.Time{}, time.Now())
	if err != nil {
		return nil, fmt.Errorf("inflateGrid: %w", err)
	}

	inflatedGrid = dropEmptyColumns(inflatedGrid)

	usesK8sClient := groupCfg.UseKubernetesClient || (groupCfg.GetResultSource().GetGcsConfig() != nil)
	var brokenThreshold float32
	if calculateStats {
		brokenThreshold = tabCfg.BrokenColumnThreshold
	}
	var alert, unalert int
	if useTabAlertSettings {
		alert = int(tabCfg.GetAlertOptions().GetNumFailuresToAlert())
		unalert = int(tabCfg.GetAlertOptions().GetNumPassesToDisableAlert())
	} else {
		alert = int(groupCfg.NumFailuresToAlert)
		unalert = int(groupCfg.NumPassesToDisableAlert)
	}
	if existingGrid != nil {
		existingInflatedGrid, _, err := updater.InflateGrid(ctx, existingGrid, time.Time{}, time.Now())
		if err != nil {
			return nil, fmt.Errorf("inflate existing grid: %w", err)
		}
		inflatedGrid = mergeGrids(existingInflatedGrid, inflatedGrid)
	}
	grid = updater.ConstructGrid(log, inflatedGrid, issues, alert, unalert, usesK8sClient, groupCfg.GetUserProperty(), brokenThreshold)
	return grid, nil
}

// mergeGrids merges two sorted, inflated grids together.
// Precondition: "addition" is an output of an Updater with an "unknown" column last.
//    This final column will be dropped and replaced with existing results.
func mergeGrids(existing, addition []updater.InflatedColumn) []updater.InflatedColumn {
	if len(addition) == 0 {
		return existing
	}
	seam := addition[len(addition)-1].Column.Started
	min := 0
	max := len(existing)
	for min != max {
		check := (min + max) / 2
		if existing[check].Column.Started <= seam {
			max = check
		} else {
			min = check + 1
		}
	}
	if max == len(existing) {
		return addition
	}
	return append(addition[:len(addition)-1], existing[max:]...)
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
