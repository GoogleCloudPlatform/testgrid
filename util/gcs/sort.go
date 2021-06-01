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

package gcs

import (
	"context"
	"sort"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"github.com/fvbommel/sortorder"
	"github.com/sirupsen/logrus"
)

// LeastRecentlyUpdated sorts paths by their update timestamp, noting generations and any errors.
func LeastRecentlyUpdated(ctx context.Context, log logrus.FieldLogger, client Stater, paths []Path) map[Path]int64 {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	log.Debug("Sorting groups")
	updated := make(map[Path]time.Time, len(paths))
	generations := make(map[Path]int64, len(paths))
	var wg sync.WaitGroup
	var lock sync.Mutex
	for _, apath := range paths {
		wg.Add(1)
		path := apath
		go func() {
			defer wg.Done()
			attrs, err := client.Stat(ctx, path)
			lock.Lock()
			defer lock.Unlock()
			switch {
			case err == storage.ErrObjectNotExist:
				generations[path] = 0
			case err != nil:
				log.WithError(err).WithField("path", path).Warning("Stat failed")
				generations[path] = -1
			default:
				updated[path] = attrs.Updated
				generations[path] = attrs.Generation
			}
		}()
	}
	wg.Wait()

	sort.SliceStable(paths, func(i, j int) bool {
		return !updated[paths[i]].After(updated[paths[j]])
	})

	if n := len(paths) - 1; n > 0 {
		p0 := paths[0]
		pn := paths[n]
		log.WithFields(logrus.Fields{
			"newest-path": pn,
			"newest":      updated[pn],
			"oldest-path": p0,
			"oldest":      updated[p0],
		}).Info("Sorted")
	}

	return generations
}

// Touch attempts to win an update of the object.
//
// Cloud copies the current object to itself when the object already exists.
// Otherwise uploads genZero bytes.
func Touch(ctx context.Context, client ConditionalClient, path Path, generation int64, genZero []byte) error {
	var cond storage.Conditions
	if generation != 0 {
		// Attempt to cloud-copy the object to its current location
		// - only 1 will win in a concurrent situation
		// - Increases the last update time.
		cond.GenerationMatch = generation
		return client.If(&cond, &cond).Copy(ctx, path, path)
	}

	// New group, upload the bytes for this situation.
	cond.DoesNotExist = true
	return client.If(&cond, &cond).Upload(ctx, path, genZero, DefaultACL, "no-cache")
}

// Sort the builds by monotonically decreasing original prefix base name.
//
// In other words,
//   gs://b/1
//   gs://a/5
//   gs://c/10
// becomes:
//   gs://c/10
//   gs://a/5
//   gs://b/1
func Sort(builds []Build) {
	sort.SliceStable(builds, func(i, j int) bool { // greater
		return !sortorder.NaturalLess(builds[i].baseName, builds[j].baseName) && builds[i].baseName != builds[j].baseName
	})
}
