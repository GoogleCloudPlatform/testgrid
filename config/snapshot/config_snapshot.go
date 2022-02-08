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
	"time"

	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/testgrid/config"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/sirupsen/logrus"
)

// Config is a mapped representation of the config at a particular point in time.
// Not concurrency-safe; meant for reading only
type Config struct {
	Dashboards map[string]*configpb.Dashboard
	Groups     map[string]*configpb.TestGroup
	Attrs      storage.ReaderObjectAttrs
}

// Observe reads the config at configPath and return a ConfigSnapshot initially and, if a ticker is supplied, when the config changes
// Returns an error instead if there is no file at configPath
func Observe(ctx context.Context, log logrus.FieldLogger, client gcs.ConditionalClient, configPath gcs.Path, ticker <-chan time.Time) (<-chan *Config, error) {
	ch := make(chan *Config)
	if log == nil {
		log = logrus.New()
	}
	log = log.WithField("observed-path", configPath.String())

	initialSnap, err := updateHash(ctx, client, configPath, nil)
	if err != nil {
		return nil, fmt.Errorf("can't read %q: %w", configPath.String(), err)
	}
	var cond storage.Conditions
	cond.GenerationNotMatch = initialSnap.Attrs.Generation

	go func() {
		defer close(ch)
		ch <- initialSnap
		if ticker == nil {
			return
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker:
				snap, err := updateHash(ctx, client, configPath, &cond)
				switch {
				case err == nil:
					// Configuration changed
					ch <- snap
					cond.GenerationNotMatch = snap.Attrs.Generation
				case !gcs.IsPreconditionFailed(err):
					log.WithError(err).Warning("Error while observing config")
				}
			}

		}
	}()

	return ch, nil
}

func updateHash(ctx context.Context, client gcs.ConditionalClient, configPath gcs.Path, cond *storage.Conditions) (*Config, error) {
	var cs Config
	cfg, attrs, err := fetchConfig(ctx, client, configPath)
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

	cs.Dashboards = namedDashboards
	cs.Groups = namedGroups
	if attrs != nil {
		cs.Attrs = *attrs
	}
	return &cs, nil
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
