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

	"github.com/GoogleCloudPlatform/testgrid/config"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"

	"github.com/golang/protobuf/proto"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

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
	err = yaml.Unmarshal(data, &list)
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

type mergeClient interface {
	gcs.Opener
	gcs.Uploader
}

// MergeAndUpdate gathers configurations from each path and merges them.
// Puts the result at targetPath if confirm is true
// Will skip an input config if it is invalid and skipValidate is false
// Other problems are considered fatal and will return an error
func MergeAndUpdate(ctx context.Context, client mergeClient, list MergeList, skipValidate, confirm bool) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Deserialize each proto
	// TODO: Reading and validating can be done in parallel with a wait group.
	// TODO: Cache the version for each source. Only read if they've changed.
	shards := map[string]*configpb.Configuration{}
	// paths, targetPath

	for _, source := range list.Sources {
		if source.Path == nil {
			return fmt.Errorf("path at %q is nil", source.Name)
		}
		cfg, err := config.ReadGCS(ctx, client, *source.Path)
		if err != nil {
			return fmt.Errorf("can't read config %q at %s: %w", source.Name, source.Path, err)
		}
		if !skipValidate {
			if err := config.Validate(cfg); err != nil {
				logrus.WithError(err).WithFields(logrus.Fields{
					"component":   "config-merger",
					"config-path": source.Location,
					"contact":     source.Contact,
				}).Errorf("config %q is invalid; skipping config", source.Name)
				continue
			}
		}
		shards[source.Name] = cfg
	}

	if len(shards) == 0 {
		return errors.New("no configs to merge")
	}

	// Merge and marshal the result
	result, err := config.Converge(shards)
	if err != nil {
		return fmt.Errorf("can't merge configurations: %w", err)
	}

	if !confirm {
		fmt.Println(result)
		return nil
	}

	buf, err := proto.Marshal(result)
	if err != nil {
		return fmt.Errorf("can't marshal merged proto: %w", err)
	}

	if _, err := client.Upload(ctx, *list.Path, buf, gcs.DefaultACL, "no-cache"); err != nil {
		return fmt.Errorf("can't upload merged proto to %s: %w", list.Path, err)
	}

	return nil
}
