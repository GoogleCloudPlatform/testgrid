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

package snapshot

import (
	"context"
	"fmt"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/testgrid/config"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/sirupsen/logrus"
)

// Config is a mapped representation of the config at a particular point in time
type Config struct {
	dashboards map[string]*configpb.Dashboard
	groups     map[string]*configpb.TestGroup
	attrs      storage.ReaderObjectAttrs
	lock       sync.RWMutex
}

// Dashboard returns a dashboard's configuration
func (cs *Config) Dashboard(name string) *configpb.Dashboard {
	if cs == nil {
		return nil
	}
	cs.lock.RLock()
	defer cs.lock.RUnlock()

	return cs.dashboards[name]
}

// AllDashboards returns all dashboards
func (cs *Config) AllDashboards() []*configpb.Dashboard {
	if cs == nil {
		return nil
	}
	cs.lock.RLock()
	defer cs.lock.RUnlock()

	db := make([]*configpb.Dashboard, len(cs.dashboards))
	var i int
	for _, d := range cs.dashboards {
		db[i] = d
		i++
	}
	return db
}

// Group returns a test group's configuration
func (cs *Config) Group(name string) *configpb.TestGroup {
	if cs == nil {
		return nil
	}
	cs.lock.RLock()
	defer cs.lock.RUnlock()

	return cs.groups[name]
}

// DashboardTestGroups returns a dashboard's configuration and a map containing:
//   Each dashboard tabs name -> the configuration of the test group backing it
func (cs *Config) DashboardTestGroups(dashboardName string) (*configpb.Dashboard, map[string]*configpb.TestGroup) {
	if cs == nil {
		return nil, nil
	}
	cs.lock.RLock()
	defer cs.lock.RUnlock()

	d := cs.dashboards[dashboardName]
	if d == nil {
		return nil, nil
	}
	gs := make(map[string]*configpb.TestGroup, len(d.DashboardTab))
	for _, tab := range d.DashboardTab {
		gs[tab.Name] = cs.groups[tab.TestGroupName]
	}
	return d, gs
}

type OnConfigUpdate func(*Config) error

// Observe will continuously keep the config up-to-date.
// If it the config changes, the callback will be called.
// Runs continuously until the context expires.
func Observe(ctx context.Context, log logrus.FieldLogger, client gcs.ConditionalClient, configPath gcs.Path, update OnConfigUpdate) (*Config, error) {
	var cs Config
	log = log.WithField("observed-path", configPath.String())

	_, err := cs.update(ctx, client, configPath, nil)
	if err != nil {
		return nil, err
	}
	ticker := time.NewTicker(time.Minute) // TODO(fejta): subscribe to notifications

	go func() {
		for {
			changed, err := cs.tick(ctx, client, configPath, update)
			if changed {
				log.Infof("Configuration changed")
			}
			if err != nil {
				log.Errorf("Error while observing config: %v", err)
			}

			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
			}
		}
	}()

	return &cs, nil
}

func (cs *Config) tick(ctx context.Context, client gcs.ConditionalClient, configPath gcs.Path, update OnConfigUpdate) (bool, error) {
	var cond storage.Conditions
	cond.GenerationNotMatch = cs.attrs.Generation
	_, err := cs.update(ctx, client, configPath, &cond)
	switch {
	case err == nil:
		// Configuration changed
		return true, update(cs)
	case !gcs.IsPreconditionFailed(err):
		return false, err
	}
	return false, nil
}

func (cs *Config) update(ctx context.Context, client gcs.ConditionalClient, configPath gcs.Path, cond *storage.Conditions) (*storage.ReaderObjectAttrs, error) {
	cfg, attrs, err := fetchConfig(ctx, client.If(cond, cond), configPath)
	if err != nil {
		return nil, err
	}

	namedDashboards := make(map[string]*configpb.Dashboard, len(cfg.Dashboards))
	for _, d := range cfg.Dashboards {
		namedDashboards[d.Name] = d
	}
	namedGroups := make(map[string]*configpb.TestGroup, len(cfg.TestGroups))
	for _, tg := range cfg.TestGroups {
		namedGroups[tg.Name] = tg
	}

	cs.lock.Lock()
	defer cs.lock.Unlock()
	cs.dashboards = namedDashboards
	cs.groups = namedGroups
	if attrs == nil {
		cs.attrs = storage.ReaderObjectAttrs{}
	} else {
		cs.attrs = *attrs
	}
	return attrs, nil
}

func fetchConfig(ctx context.Context, client gcs.ConditionalClient, configPath gcs.Path) (*configpb.Configuration, *storage.ReaderObjectAttrs, error) {
	r, attrs, err := client.Open(ctx, configPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open: %w", err)
	}

	cfg, err := config.Unmarshal(r)
	if err != nil {
		return nil, nil, fmt.Errorf("unmarshal: %v", err)
	}
	return cfg, attrs, nil
}
