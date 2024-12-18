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
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"github.com/golang/protobuf/proto"
	"github.com/sirupsen/logrus"
	yaml "sigs.k8s.io/yaml/goyaml.v2"

	"github.com/GoogleCloudPlatform/testgrid/config"
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
	gcs.Opener
	gcs.Uploader
}

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

	// TODO: Cache the version for each source. Only read if they've changed.
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
		if mets != nil {
			mets.LastModified.Set(int64(diff.Seconds()), source)
		}
		logrus.WithFields(logrus.Fields{
			"diff":  diff,
			"shard": source,
		}).Info("Time since last updated.")
	}
}
