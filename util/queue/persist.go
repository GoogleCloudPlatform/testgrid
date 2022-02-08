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

package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/sirupsen/logrus"
)

// PersistClient contains interfaces for reading from and writing to a Path.
type PersistClient interface {
	gcs.Uploader
	gcs.Opener
}

// Fixer will adjust the queue until the context expires.
type Fixer func(context.Context, *Queue) error

// FixPersistent persists the queue to the remote path every tick.
//
// The first time it will load the state. Thereafter it will save the state.
// This includes restarts due to expiring contexts -- it will just load once.
func FixPersistent(logr logrus.FieldLogger, client PersistClient, path gcs.Path, tick <-chan time.Time) Fixer {
	var shouldSave bool
	log := logr.WithField("path", path)
	return func(ctx context.Context, q *Queue) error {
		log.Debug("Using persistent state")

		tryLoad := func() error {
			reader, attrs, err := client.Open(ctx, path)
			if errors.Is(err, storage.ErrObjectNotExist) {
				log.Info("Previous persistent queue state does not exist.")
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

			log.WithField("from", attrs.LastModified).Info("Loaded previous state, syncing queue.")
			if err := q.FixAll(whens, false); err != nil {
				return fmt.Errorf("fix all: %v", err)
			}
			return nil
		}

		logSave := true

		trySave := func() error {
			currently := q.Current()
			buf, err := json.MarshalIndent(currently, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal: %v", err)
			}
			attrs, err := client.Upload(ctx, path, buf, false, "")
			if logSave {
				logSave = false
				log.WithField("updated", attrs.Updated).Info("Wrote persistent state")
			}
			return err
		}

		for {
			select {
			case <-ctx.Done():
				log.Debug("Stopped syncing persistent state")
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
