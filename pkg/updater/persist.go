/*
Copyright 2022 The Kubernetes Authors.

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

// Package updater reads the latest test results and saves updated state.
package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/testgrid/config"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/sirupsen/logrus"
)

// PersistClient contains interfaces for reading from and writing to a Path.
type PersistClient interface {
	gcs.Uploader
	gcs.Opener
}

// FixPersistent persists the queue to the remote path every tick.
//
// The first time it will load the state. Thereafter it will save the state.
// This includes restarts due to expiring contexts -- it will just load once.
func FixPersistent(client PersistClient, path gcs.Path, tick <-chan time.Time) Fixer {
	var shouldSave bool
	return func(ctx context.Context, logr logrus.FieldLogger, q *config.TestGroupQueue, _ []*configpb.TestGroup) error {
		log := logr.WithField("path", path)

		log.Debug("Using persistent state")

		tryLoad := func() error {
			reader, attrs, err := client.Open(ctx, path)
			if errors.Is(err, storage.ErrObjectNotExist) {
				return nil
			}
			if err != nil {
				return fmt.Errorf("open: %w", err)
			}

			defer reader.Close()
			dec := json.NewDecoder(reader)
			var whens map[string]time.Time
			if err := dec.Decode(&whens); err != nil {
				return fmt.Errorf("decode: %v", err)
			}

			current := q.Current()

			for name := range whens {
				if _, ok := current[name]; ok {
					continue
				}
				delete(whens, name)
			}

			log.WithField("from", attrs.LastModified).Info("Fixing queue with persistent state.")
			if err := q.FixAll(whens, false); err != nil {
				return fmt.Errorf("fix all: %v", err)
			}
			return nil
		}

		trySave := func() error {
			currently := q.Current()
			buf, err := json.MarshalIndent(currently, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal: %v", err)
			}
			_, err = client.Upload(ctx, path, buf, false, "")
			return err
		}

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-tick:
			}

			if shouldSave {
				log.Trace("Saving persistent state...")
				if err := trySave(); err != nil {
					log.WithError(err).Error("Failed to save persistent state.")
					continue
				}
				log.Debug("Saved persistent state.")
			} else {
				log.Trace("Loading persistent state...")
				if err := tryLoad(); err != nil {
					log.WithError(err).Error("Failed to load persistent state.")
					continue
				}
				shouldSave = true
				log.Debug("Loaded persistent state.")
			}
		}
	}
}
