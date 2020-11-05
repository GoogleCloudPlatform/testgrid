/*
Copyright 2020 The TestGrid Authors.

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

package updater

import (
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"path"
	"sync"
	"time"

	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"

	"cloud.google.com/go/storage"
	"github.com/golang/protobuf/proto"
	"github.com/sirupsen/logrus"
)

func downloadGrid(ctx context.Context, opener gcs.Opener, path gcs.Path) (*statepb.Grid, error) {
	var g statepb.Grid
	r, err := opener.Open(ctx, path)
	if err != nil && err == storage.ErrObjectNotExist {
		return &g, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer r.Close()
	zr, err := zlib.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("open zlib: %w", err)
	}
	pbuf, err := ioutil.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("decompress: %w", err)
	}
	err = proto.Unmarshal(pbuf, &g)
	return &g, err
}

// readColumns will list, download and process builds into inflatedColumns.
func readColumns(parent context.Context, client gcs.Downloader, group *configpb.TestGroup, builds []gcs.Build, stopTime time.Time, max int, buildTimeout time.Duration, concurrency int) ([]inflatedColumn, error) {
	// Spawn build readers
	if concurrency == 0 {
		return nil, errors.New("zero readers")
	}

	var wg sync.WaitGroup
	defer wg.Wait()
	var maxLock sync.Mutex

	log := logrus.WithField("group", group.Name).WithField("prefix", "gs://"+group.GcsPrefix)

	stop := stopTime.Unix() * 1000

	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	if lb := len(builds); lb > max {
		log.WithField("total", lb).WithField("max", max).Debug("Truncating")
		builds = builds[:max]
	}
	maxIdx := len(builds)
	cols := make([]inflatedColumn, maxIdx)
	log.WithField("timeout", buildTimeout).Debug("Updating")
	ec := make(chan error)
	old := make(chan int)

	// Send build indices to readers
	indices := make(chan int)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(indices)
		for i := range builds {
			select {
			case <-ctx.Done():
				return
			case <-old:
				return
			case indices <- i:
			}
		}
	}()

	var heads []string
	for _, h := range group.ColumnHeader {
		heads = append(heads, h.ConfigurationValue)
	}

	// Concurrently receive indices and read builds
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		nameCfg := makeNameConfig(group.TestNameConfig)
		go func() {
			defer wg.Done()
			for {
				var idx int
				var open bool
				select {
				case <-ctx.Done():
					return
				case idx, open = <-indices:
				}

				if !open {
					select {
					case <-ctx.Done():
					case ec <- nil:
					}
					return
				}

				b := builds[idx]

				// use ctx so we finish reading, even if buildCtx is done
				inner, cancel := context.WithTimeout(ctx, buildTimeout)
				defer cancel()
				result, err := readResult(inner, client, b)
				if err != nil {
					cancel()
					select {
					case <-ctx.Done():
					case ec <- fmt.Errorf("read %s: %w", b, err):
					}
					return
				}
				id := path.Base(b.Path.Object())
				col := convertResult(nameCfg, id, heads, *result)
				if int64(col.column.Started) < stop {
					// Multiple go-routines may all read an old result.
					// So we need to use a mutex to read the
					wg.Add(1)
					go func() {
						defer wg.Done()
						maxLock.Lock()
						defer maxLock.Unlock()
						if maxIdx == len(builds) {
							// still vending new indices to download, stop this.
							select {
							case <-ctx.Done():
							case old <- idx:
								log.WithFields(logrus.Fields{
									"idx":     idx,
									"id":      id,
									"path":    b.Path,
									"started": int64(col.column.Started / 1000),
									"stop":    stopTime,
								}).Debug("Stopped")
							}
						}
						if maxIdx > idx+1 {
							maxIdx = idx + 1 // this is the newest old result
						}
					}()
				}
				cols[idx] = col
			}
		}()
	}

	for ; concurrency > 0; concurrency-- {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-ec:
			if err != nil {
				return nil, err
			}
		}
	}

	// wait for the consistent maxIdx value
	cancel()  // no need to notify about an old index
	wg.Wait() // wait for all the old indexes to sync

	return cols[0:maxIdx], nil
}

// readResult will download all GCS artifacts in parallel.
//
// Specifically download the following files:
// * started.json
// * finished.json
// * any junit.xml files under the artifacts directory.
func readResult(parent context.Context, client gcs.Downloader, build gcs.Build) (*gcsResult, error) {
	ctx, cancel := context.WithCancel(parent) // Allows aborting after first error
	defer cancel()
	var result gcsResult
	ec := make(chan error) // Receives errors from anyone

	var work int

	// Download started.json
	work++
	go func() {
		s, err := build.Started(ctx, client)
		if err != nil {
			err = fmt.Errorf("started: %w", err)
		} else {
			result.started = *s
		}
		select {
		case <-ctx.Done():
		case ec <- err:
		}
	}()

	// Download finished.json
	work++
	go func() {
		f, err := build.Finished(ctx, client)
		if err != nil {
			err = fmt.Errorf("finished: %w", err)
		} else {
			result.finished = *f
		}
		select {
		case <-ctx.Done():
		case ec <- err:
		}
	}()

	// Download suites
	work++
	go func() {
		var err error
		result.suites, err = readSuites(ctx, client, build)
		if err != nil {
			err = fmt.Errorf("suites: %w", err)
		}

		select {
		case <-ctx.Done():
		case ec <- err:
		}
	}()

	for ; work > 0; work-- {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout: %w", ctx.Err())
		case err := <-ec:
			if err != nil {
				return nil, err
			}
		}
	}
	return &result, nil
}

// readSuites asynchrounously lists and downloads junit.xml files
func readSuites(parent context.Context, client gcs.Downloader, build gcs.Build) ([]gcs.SuitesMeta, error) {
	var wg sync.WaitGroup
	defer wg.Wait()
	var work int
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	ec := make(chan error)
	// List artifacts to the artifacts channel
	artifacts := make(chan string) // Receives names of arifacts
	work++
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(artifacts) // No more artifacts
		err := build.Artifacts(ctx, client, artifacts)
		if err != nil {
			err = fmt.Errorf("list: %w", err)
		}
		select {
		case ec <- err:
		case <-ctx.Done():
		}
	}()

	// Download each artifact
	// With parallelism: 60s without: 220s
	suitesChan := make(chan gcs.SuitesMeta)
	work++
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(suitesChan) // No more rows
		err := build.Suites(ctx, client, artifacts, suitesChan)
		if err != nil {
			err = fmt.Errorf("download: %w", err)
		}

		select {
		case ec <- err:
		case <-ctx.Done():
		}
	}()

	var suites []gcs.SuitesMeta
	for work > 0 {
		// Add each downloaded artifact to the returned list.

		// Abort if we get an expired context and/or an error.
		// Otherwise keep going until the channel closes
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout: %w", ctx.Err())
		case err := <-ec:
			if err != nil {
				return nil, err // already wrapped.
			}
			work--
		case suite, more := <-suitesChan:
			if !more {
				return suites, nil
			}
			suites = append(suites, suite)
		}
	}
	return suites, nil
}
