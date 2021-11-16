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
	"errors"
	"sort"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"github.com/fvbommel/sortorder"
	"github.com/sirupsen/logrus"
)

// StatResult contains the result of calling Stat, including any error
type StatResult struct {
	Attrs *storage.ObjectAttrs
	Err   error
}

// Stat multiple paths using concurrent workers. Result indexes match paths.
func Stat(ctx context.Context, client Stater, workers int, paths ...Path) []StatResult {
	var wg sync.WaitGroup
	wg.Add(workers)
	out := make([]StatResult, len(paths))
	ch := make(chan int)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for idx := range ch {
				out[idx].Attrs, out[idx].Err = client.Stat(ctx, paths[idx])
			}
		}()
	}
	for idx := range paths {
		ch <- idx
	}
	close(ch)
	wg.Wait()
	return out
}

// StatExisting reduces Stat() to an array of ObjectAttrs.
//
// Non-existent objects will return a pointer to a zero storage.ObjectAttrs.
// Objects that fail to stat will be nil (and log).
func StatExisting(ctx context.Context, log logrus.FieldLogger, client Stater, paths ...Path) []*storage.ObjectAttrs {
	out := make([]*storage.ObjectAttrs, len(paths))

	attrs := Stat(ctx, client, 20, paths...)
	for i, attrs := range attrs {
		err := attrs.Err
		switch {
		case attrs.Attrs != nil:
			out[i] = attrs.Attrs
		case errors.Is(err, storage.ErrObjectNotExist):
			out[i] = &storage.ObjectAttrs{}
		default:
			log.WithError(err).WithField("path", paths[i]).Info("Failed to stat")
		}
	}
	return out
}

// LeastRecentlyUpdated sorts paths by their update timestamp, noting generations and any errors.
func LeastRecentlyUpdated(ctx context.Context, log logrus.FieldLogger, client Stater, paths []Path) map[Path]int64 {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	log.Debug("Sorting groups")
	const workers = 20
	attrs := Stat(ctx, client, workers, paths...)
	updated := make(map[Path]time.Time, len(paths))
	generations := make(map[Path]int64, len(paths))

	for i, path := range paths {
		attrs, err := attrs[i].Attrs, attrs[i].Err
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
	}

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
func Touch(ctx context.Context, client ConditionalClient, path Path, generation int64, genZero []byte) (*storage.ObjectAttrs, error) {
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
