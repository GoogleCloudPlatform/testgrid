/*
Copyright 2021 The Kubernetes Authors.

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

package merger

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	yaml "sigs.k8s.io/yaml/goyaml.v2"

	"github.com/GoogleCloudPlatform/testgrid/config"
	"github.com/GoogleCloudPlatform/testgrid/config/snapshot"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/GoogleCloudPlatform/testgrid/util/metrics"
)

const componentName = "config-merger"

// MergeList is a list of config sources to merge together
// ParseAndCheck will construct this from a YAML document
type MergeList struct {
	Target  string    `json:"Target"`
	Path    *gcs.Path `json:"-"`
	Sources []Source  `json:"Sources"`
}

// Source represents a configuration source in cloud storage
type Source struct {
	Name     string    `json:"Name"`
	Location string    `json:"Location"`
	Path     *gcs.Path `json:"-"`
	Contact  string    `json:"Contact,omitempty"`
}

// ParseAndCheck parses and checks the configuration file for common errors
func ParseAndCheck(data []byte) (list MergeList, err error) {
	err = yaml.UnmarshalStrict(data, &list)
	if err != nil {
		return
	}

	list.Path, err = gcs.NewPath(list.Target)
	if err != nil {
		return
	}

	if len(list.Sources) == 0 {
		return list, errors.New("no shards to converge")
	}

	names := map[string]bool{}
	for i, source := range list.Sources {
		if _, exists := names[source.Name]; exists {
			return list, fmt.Errorf("duplicated name %s", source.Name)
		}
		path, err := gcs.NewPath(source.Location)
		if err != nil {
			return list, err
		}
		list.Sources[i].Path = path
		source.Path = path
		names[source.Name] = true
	}

	return
}

// Metrics holds metrics relevant to the config merger.
type Metrics struct {
	Update       metrics.Cyclic
	Fields       metrics.Int64
	LastModified metrics.Int64
}

// CreateMetrics creates metrics for the Config Merger
func CreateMetrics(factory metrics.Factory) *Metrics {
	return &Metrics{
		Update:       factory.NewCyclic(componentName),
		Fields:       factory.NewInt64("config_fields", "Config field usage by name", "component", "field"),
		LastModified: factory.NewInt64("last_modified", "Seconds since shard last modified ", "shard"),
	}
}

type mergeClient interface {
	gcs.ConditionalClient
}

const recheckDuration = 30 * time.Second

type observedSource struct {
	source Source
	ch     <-chan *snapshot.Config
}

type sourceUpdate struct {
	source Source
	snap   *snapshot.Config
}

// MergeUpdateListen gathers configurations from each path and continuously listens to them for changes.
// Returns an initial merged config, and a new config whenever any of the configs in the MergeList change.
// Will not merge an input config if it is invalid and skipValidate is false, but will continue to listen to it.
// Other problems will lead to configs jumping in and out of merging.
func MergeUpdateListen(ctx context.Context, client mergeClient, mets *Metrics, list MergeList, skipValidate, confirm bool) (<-chan *snapshot.Config, error) {
	ticker := time.NewTicker(recheckDuration)
	ch, err := mergeUpdateListen(ctx, client, mets, list, skipValidate, confirm, ticker.C)
	if err != nil {
		ticker.Stop()
		return nil, err
	}
	go func() {
		<-ctx.Done()
		ticker.Stop()
	}()
	return ch, nil
}

func mergeUpdateListen(ctx context.Context, client mergeClient, mets *Metrics, list MergeList, skipValidate, confirm bool, ticker <-chan time.Time) (<-chan *snapshot.Config, error) {
	var finish *metrics.CycleReporter
	if mets != nil {
		finish = mets.Update.Start()
	}

	if len(list.Sources) == 0 {
		finish.Skip()
		return nil, errors.New("no configs to merge")
	}
	for _, source := range list.Sources {
		if source.Path == nil {
			finish.Skip()
			return nil, fmt.Errorf("path at %q is nil", source.Name)
		}
	}
	if confirm && list.Path == nil {
		finish.Skip()
		return nil, errors.New("target path is nil")
	}

	// Cancel context only if we don't initialize properly.
	ctx, cancel := context.WithCancel(ctx)
	started := false
	defer func() {
		if !started {
			cancel()
		}
	}()

	var subTickers []chan time.Time
	if ticker != nil {
		subTickers = make([]chan time.Time, len(list.Sources))
		for i := range subTickers {
			subTickers[i] = make(chan time.Time, 1)
		}
	}

	shards := map[string]*configpb.Configuration{}
	observers := make([]observedSource, 0, len(list.Sources))
	var lock sync.Mutex
	fatal := make(chan error, 1)

	baseLog := logrus.WithField("component", componentName)

	var wg sync.WaitGroup
	for i, source := range list.Sources {
		wg.Add(1)
		i, source := i, source
		var subTicker <-chan time.Time
		if ticker != nil {
			subTicker = subTickers[i]
		}
		go func() {
			defer wg.Done()
			log := baseLog.WithFields(logrus.Fields{
				"config-path": source.Location,
				"contact":     source.Contact,
			})
			ch, err := snapshot.Observe(ctx, log, client, *source.Path, subTicker)
			if err != nil {
				log.WithError(err).Errorf("can't read config %q", source.Name)
				select {
				case fatal <- fmt.Errorf("can't read config %q at %s: %w", source.Name, source.Path, err):
				default:
				}
				return
			}

			snap, ok := <-ch
			if !ok {
				select {
				case fatal <- fmt.Errorf("config channel for %q closed before initial snapshot", source.Name):
				default:
				}
				return
			}

			recordLastModified(&snap.Attrs, mets, source.Name)

			lock.Lock()
			observers = append(observers, observedSource{source: source, ch: ch})
			lock.Unlock()

			cfg := snapshotToProto(snap)
			if !skipValidate {
				if err := config.Validate(cfg); err != nil {
					log.WithError(err).Errorf("config %q is invalid; skipping config", source.Name)
					return
				}
			}

			lock.Lock()
			shards[source.Name] = cfg
			lock.Unlock()
		}()
	}

	wg.Wait()
	close(fatal)

	if err := <-fatal; err != nil {
		finish.Fail()
		return nil, err
	}
	if len(shards) == 0 {
		finish.Skip()
		return nil, errors.New("no configs to merge")
	}

	initialSnap, err := convergeAndUpload(ctx, client, mets, list.Path, shards, confirm)
	if err != nil {
		finish.Fail()
		return nil, err
	}
	finish.Success()

	out := make(chan *snapshot.Config)
	updates := make(chan sourceUpdate)

	if ticker != nil {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case t, ok := <-ticker:
					if !ok {
						return
					}
					for _, st := range subTickers {
						select {
						case st <- t:
						default:
						}
					}
				}
			}
		}()
	}

	var obsWg sync.WaitGroup
	for _, obs := range observers {
		obsWg.Add(1)
		obs := obs
		go func() {
			defer obsWg.Done()
			for snap := range obs.ch {
				select {
				case <-ctx.Done():
					return
				case updates <- sourceUpdate{source: obs.source, snap: snap}:
				}
			}
		}()
	}
	go func() {
		obsWg.Wait()
		close(updates)
	}()

	started = true
	go func() {
		defer close(out)
		defer cancel()

		select {
		case <-ctx.Done():
			return
		case out <- initialSnap:
		}

		for u := range updates {
			var cycleFinish *metrics.CycleReporter
			if mets != nil {
				cycleFinish = mets.Update.Start()
			}

			recordLastModified(&u.snap.Attrs, mets, u.source.Name)

			cfg := snapshotToProto(u.snap)
			if !skipValidate {
				if err := config.Validate(cfg); err != nil {
					baseLog.WithError(err).WithFields(logrus.Fields{
						"config-path": u.source.Location,
						"contact":     u.source.Contact,
					}).Errorf("config %q is invalid; skipping config", u.source.Name)
					cycleFinish.Skip()
					continue
				}
			}

			shards[u.source.Name] = cfg

			mergedSnap, err := convergeAndUpload(ctx, client, mets, list.Path, shards, confirm)
			if err != nil {
				baseLog.WithError(err).Error("can't merge and update configurations")
				cycleFinish.Fail()
				continue
			}
			cycleFinish.Success()

			select {
			case <-ctx.Done():
				return
			case out <- mergedSnap:
			}
		}
	}()

	return out, nil
}

func convergeAndUpload(ctx context.Context, client gcs.Uploader, mets *Metrics, targetPath *gcs.Path, shards map[string]*configpb.Configuration, confirm bool) (*snapshot.Config, error) {
	cloned := make(map[string]*configpb.Configuration, len(shards))
	for k, v := range shards {
		cloned[k] = proto.Clone(v).(*configpb.Configuration)
	}

	result, err := config.Converge(cloned)
	if err != nil {
		return nil, fmt.Errorf("can't merge configurations: %w", err)
	}

	if mets != nil && mets.Fields != nil {
		f := config.Fields(result)
		for name, qty := range f {
			mets.Fields.Set(qty, componentName, name)
		}
	}

	if !confirm {
		return protoToSnapshot(result), nil
	}

	if targetPath == nil {
		return nil, errors.New("target path is nil")
	}

	buf, err := proto.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("can't marshal merged proto: %w", err)
	}

	if _, err := client.Upload(ctx, *targetPath, buf, gcs.DefaultACL, gcs.NoCache); err != nil {
		return nil, fmt.Errorf("can't upload merged proto to %s: %w", targetPath, err)
	}

	return protoToSnapshot(result), nil
}

func snapshotToProto(snap *snapshot.Config) *configpb.Configuration {
	if snap == nil {
		return nil
	}
	cfg := &configpb.Configuration{
		DashboardGroups: make([]*configpb.DashboardGroup, 0, len(snap.DashboardGroups)),
		Dashboards:      make([]*configpb.Dashboard, 0, len(snap.Dashboards)),
		TestGroups:      make([]*configpb.TestGroup, 0, len(snap.Groups)),
	}

	dgNames := make([]string, 0, len(snap.DashboardGroups))
	for name := range snap.DashboardGroups {
		dgNames = append(dgNames, name)
	}
	sort.Strings(dgNames)
	for _, name := range dgNames {
		cfg.DashboardGroups = append(cfg.DashboardGroups, snap.DashboardGroups[name])
	}

	dNames := make([]string, 0, len(snap.Dashboards))
	for name := range snap.Dashboards {
		dNames = append(dNames, name)
	}
	sort.Strings(dNames)
	for _, name := range dNames {
		cfg.Dashboards = append(cfg.Dashboards, snap.Dashboards[name])
	}

	tgNames := make([]string, 0, len(snap.Groups))
	for name := range snap.Groups {
		tgNames = append(tgNames, name)
	}
	sort.Strings(tgNames)
	for _, name := range tgNames {
		cfg.TestGroups = append(cfg.TestGroups, snap.Groups[name])
	}

	return cfg
}

func protoToSnapshot(cfg *configpb.Configuration) *snapshot.Config {
	if cfg == nil {
		return nil
	}
	namedDashboardGroups := make(map[string]*configpb.DashboardGroup, len(cfg.GetDashboardGroups()))
	for _, dg := range cfg.GetDashboardGroups() {
		namedDashboardGroups[dg.GetName()] = dg
	}
	namedDashboards := make(map[string]*configpb.Dashboard, len(cfg.GetDashboards()))
	for _, d := range cfg.GetDashboards() {
		namedDashboards[d.GetName()] = d
	}
	namedGroups := make(map[string]*configpb.TestGroup, len(cfg.GetTestGroups()))
	for _, tg := range cfg.GetTestGroups() {
		namedGroups[tg.GetName()] = tg
	}
	return &snapshot.Config{
		DashboardGroups: namedDashboardGroups,
		Dashboards:      namedDashboards,
		Groups:          namedGroups,
	}
}

// DEPRECATED: Prefer MergeUpdateListen (with no ticker) to read once.
// MergeAndUpdate gathers configurations from each path and merges them.
// Puts the result at targetPath if confirm is true
// Will skip an input config if it is invalid and skipValidate is false
// Other problems are considered fatal and will return an error
func MergeAndUpdate(ctx context.Context, client mergeClient, mets *Metrics, list MergeList, skipValidate, confirm bool) (*configpb.Configuration, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var finish *metrics.CycleReporter
	if mets != nil {
		finish = mets.Update.Start()
	}

	shards := map[string]*configpb.Configuration{}
	var shardsLock sync.Mutex
	var fatal error

	var wg sync.WaitGroup

	for _, source := range list.Sources {
		if source.Path == nil {
			finish.Skip()
			return nil, fmt.Errorf("path at %q is nil", source.Name)
		}

		wg.Add(1)
		source := source
		go func() {
			defer wg.Done()
			cfg, attrs, err := config.ReadGCS(ctx, client, *source.Path)
			recordLastModified(attrs, mets, source.Name)
			if err != nil {
				// Log each fatal error, but it's okay to return any fatal error
				logrus.WithError(err).WithFields(logrus.Fields{
					"component":   "config-merger",
					"config-path": source.Location,
					"contact":     source.Contact,
				}).Errorf("can't read config %q", source.Name)
				fatal = fmt.Errorf("can't read config %q at %s: %w", source.Name, source.Path, err)
				return
			}
			if !skipValidate {
				if err := config.Validate(cfg); err != nil {
					logrus.WithError(err).WithFields(logrus.Fields{
						"component":   "config-merger",
						"config-path": source.Location,
						"contact":     source.Contact,
					}).Errorf("config %q is invalid; skipping config", source.Name)
					return
				}
			}

			shardsLock.Lock()
			defer shardsLock.Unlock()
			shards[source.Name] = cfg
		}()
	}

	wg.Wait()

	if fatal != nil {
		finish.Fail()
		return nil, fatal
	}
	if len(shards) == 0 {
		finish.Skip()
		return nil, errors.New("no configs to merge")
	}

	// Merge and output the result
	result, err := config.Converge(shards)
	if err != nil {
		finish.Fail()
		return result, fmt.Errorf("can't merge configurations: %w", err)
	}

	if !confirm {
		fmt.Println(result)
		finish.Success()
		return result, nil
	}

	// Log each field as a metric
	if mets != nil {
		f := config.Fields(result)
		for name, qty := range f {
			mets.Fields.Set(qty, componentName, name)
		}
	}

	buf, err := proto.Marshal(result)
	if err != nil {
		finish.Fail()
		return result, fmt.Errorf("can't marshal merged proto: %w", err)
	}

	if _, err := client.Upload(ctx, *list.Path, buf, gcs.DefaultACL, gcs.NoCache); err != nil {
		finish.Fail()
		return result, fmt.Errorf("can't upload merged proto to %s: %w", list.Path, err)
	}

	finish.Success()
	return result, nil
}

func recordLastModified(attrs *storage.ReaderObjectAttrs, mets *Metrics, source string) {
	if attrs != nil {
		lastModified := attrs.LastModified
		diff := time.Since(lastModified)
		if mets != nil && mets.LastModified != nil {
			mets.LastModified.Set(int64(diff.Seconds()), source)
		}
		logrus.WithFields(logrus.Fields{
			"diff":  diff,
			"shard": source,
		}).Info("Time since last updated.")
	}
}
