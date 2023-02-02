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

// Package v1 (api/v1) is the first versioned implementation of the API
package v1

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/GoogleCloudPlatform/testgrid/config"
	"github.com/GoogleCloudPlatform/testgrid/config/snapshot"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
)

const (
	reobservationTime = 2 * time.Minute
	configFileName    = "config"
)

var (
	ErrScopeNotProvided = errors.New("scope is not provided")
)

// Cached contains a config and a mapping of "normalized" names to actual resources.
// Avoid using normalized names: normalization is costly and the raw name itself works better as a key.
// Used within the API to sanitize inputs: ie. "dashboards/mytab" matches "My Tab"
type cachedConfig struct {
	Config               *snapshot.Config
	NormalDashboardGroup map[string]string
	NormalDashboard      map[string]string
	NormalDashboardTab   map[string]map[string]string // Normal dashboard name AND normal tab name
	NormalTestGroup      map[string]string
	Mutex                sync.RWMutex
}

func (c *cachedConfig) generateNormalCache() {
	if c == nil || c.Config == nil {
		return
	}

	c.NormalDashboardGroup = make(map[string]string, len(c.Config.DashboardGroups))
	c.NormalDashboard = make(map[string]string, len(c.Config.Dashboards))
	c.NormalTestGroup = make(map[string]string, len(c.Config.Groups))
	c.NormalDashboardTab = make(map[string]map[string]string, len(c.Config.Dashboards))

	for name := range c.Config.DashboardGroups {
		c.NormalDashboardGroup[config.Normalize(name)] = name
	}

	for name := range c.Config.Dashboards {
		normalName := config.Normalize((name))
		c.NormalDashboard[normalName] = name
		c.NormalDashboardTab[normalName] = make(map[string]string, len(c.Config.Dashboards[name].DashboardTab))
		for _, tab := range c.Config.Dashboards[name].DashboardTab {
			normalTabName := config.Normalize((tab.Name))
			c.NormalDashboardTab[normalName][normalTabName] = tab.Name
		}
	}

	for name := range c.Config.Groups {
		c.NormalTestGroup[config.Normalize(name)] = name
	}
}

func (s *Server) configPath(scope string) (path *gcs.Path, isDefault bool, err error) {
	if scope != "" {
		path, err = gcs.NewPath(fmt.Sprintf("%s/%s", scope, configFileName))
		return path, false, err
	}
	if s.DefaultBucket != "" {
		path, err = gcs.NewPath(fmt.Sprintf("%s/%s", s.DefaultBucket, configFileName))
		return path, true, err
	}
	return nil, false, ErrScopeNotProvided
}

// getConfig will return a config or an error. The config contains a mutex that you should RLock before reading.
// Does not expose wrapped errors to the user, instead logging them to the console.
func (s *Server) getConfig(ctx context.Context, log *logrus.Entry, scope string) (*cachedConfig, error) {
	configPath, isDefault, err := s.configPath(scope)
	if err != nil || configPath == nil {
		return nil, err
	}

	if isDefault {
		if s.defaultCache == nil || s.defaultCache.Config == nil {
			log = log.WithField("config-path", configPath.String())
			configChanged, err := snapshot.Observe(ctx, log, s.Client, *configPath, time.NewTicker(reobservationTime).C)
			if err != nil {
				log.WithError(err).Errorf("Can't read default config; check permissions")
				return nil, fmt.Errorf("Could not read config at %q", configPath.String())
			}

			s.defaultCache = &cachedConfig{
				Config: <-configChanged,
			}
			s.defaultCache.generateNormalCache()

			log.Info("Observing default config")
			go func(ctx context.Context) {
				for {
					select {
					case newCfg := <-configChanged:
						s.defaultCache.Mutex.Lock()
						s.defaultCache.Config = newCfg
						s.defaultCache.generateNormalCache()
						log.Info("Observed config updated")
						s.defaultCache.Mutex.Unlock()
					case <-ctx.Done():
						return
					}
				}
			}(ctx)

		}
		return s.defaultCache, nil
	}

	cfgChan, err := snapshot.Observe(ctx, log, s.Client, *configPath, nil)
	if err != nil {
		// Do not log; invalid requests will write useless logs.
		return nil, fmt.Errorf("Could not read config at %q", configPath.String())
	}

	result := cachedConfig{
		Config: <-cfgChan,
	}
	result.generateNormalCache()
	return &result, nil
}
