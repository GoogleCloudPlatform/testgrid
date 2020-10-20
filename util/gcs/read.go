/*
Copyright 2019 The Kubernetes Authors.

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
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"

	"cloud.google.com/go/storage"
	"github.com/fvbommel/sortorder"
	"google.golang.org/api/iterator"

	"github.com/GoogleCloudPlatform/testgrid/metadata"
	"github.com/GoogleCloudPlatform/testgrid/metadata/junit"
)

// Started holds started.json data.
type Started struct {
	metadata.Started
	// Pending when the job has not started yet
	Pending bool
}

// Finished holds finished.json data.
type Finished struct {
	metadata.Finished
	// Running when the job hasn't finished and finished.json doesn't exist
	Running bool
}

// Build points to a build stored under a particular gcs prefix.
type Build struct {
	Path              Path
	originalPrefix    string
	suitesConcurrency int // override the max number of concurrent suite downloads
}

func (build Build) String() string {
	return build.Path.String()
}

// ListBuilds returns the array of builds under path, sorted in monotonically decreasing order.
func ListBuilds(parent context.Context, lister Lister, path Path, after *Path) ([]Build, error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	var offset string
	if after != nil {
		offset = after.Object()
	}
	it := lister.Objects(ctx, path, "/", offset)
	var all []Build
	for {
		objAttrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list objects: %w", err)
		}

		// if this is a link under directory/, resolve the build value
		// This is used for PR type jobs which we store in a PR specific prefix.
		// The directory prefix contains a link header to the result
		// under the PR specific prefix.
		if link := objAttrs.Metadata["x-goog-meta-link"]; len(link) > 0 {
			// links created by bootstrap.py have a space
			link = strings.TrimSpace(link)
			u, err := url.Parse(link)
			if err != nil {
				return nil, fmt.Errorf("parse %s link: %v", objAttrs.Name, err)
			}
			if !strings.HasSuffix(u.Path, "/") {
				u.Path += "/"
			}
			var linkPath Path
			if err := linkPath.SetURL(u); err != nil {
				return nil, fmt.Errorf("bad %s link path %s: %w", objAttrs.Name, u, err)
			}
			all = append(all, Build{
				Path:           linkPath,
				originalPrefix: objAttrs.Name,
			})
			continue
		}

		if objAttrs.Prefix == "" {
			continue // not a symlink to a directory
		}

		loc := "gs://" + path.Bucket() + "/" + objAttrs.Prefix
		path, err := NewPath(loc)
		if err != nil {
			return nil, fmt.Errorf("bad path %q: %w", loc, err)
		}

		all = append(all, Build{
			Path:           *path,
			originalPrefix: objAttrs.Prefix,
		})
	}

	sort.SliceStable(all, func(i, j int) bool {
		// ! because we want the latest (aka largest) items first.
		return !sortorder.NaturalLess(all[i].originalPrefix, all[j].originalPrefix)
	})

	if offset != "" {
		// GCS will return 200 2000 30 for a prefix of 100
		// testgrid expects this as 2000 200 (dropping 30)
		for i, b := range all {
			if sortorder.NaturalLess(b.originalPrefix, offset) {
				return all[:i], nil
			}
		}
	}

	return all, nil
}

// junit_CONTEXT_TIMESTAMP_THREAD.xml
var re = regexp.MustCompile(`.+/junit((_[^_]+)?(_\d+-\d+)?(_\d+)?|.+)?\.xml$`)

// dropPrefix removes the _ in _CONTEXT to help keep the regexp simple
func dropPrefix(name string) string {
	if len(name) == 0 {
		return name
	}
	return name[1:]
}

// parseSuitesMeta returns the metadata for this junit file (nil for a non-junit file).
//
// Expected format: junit_context_20180102-1256_07.xml
// Results in {
//   "Context": "context",
//   "Timestamp": "20180102-1256",
//   "Thread": "07",
// }
func parseSuitesMeta(name string) map[string]string {
	mat := re.FindStringSubmatch(name)
	if mat == nil {
		return nil
	}
	c, ti, th := dropPrefix(mat[2]), dropPrefix(mat[3]), dropPrefix(mat[4])
	if c == "" && ti == "" && th == "" {
		c = mat[1]
	}
	return map[string]string{
		"Context":   c,
		"Timestamp": ti,
		"Thread":    th,
	}

}

// readJSON will decode the json object stored in GCS.
func readJSON(ctx context.Context, opener Opener, p Path, i interface{}) error {
	reader, err := opener.Open(ctx, p)
	if errors.Is(err, storage.ErrObjectNotExist) {
		return err
	}
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer reader.Close()
	if err = json.NewDecoder(reader).Decode(i); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	if err := reader.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	return nil
}

// Started parses the build's started metadata.
func (build Build) Started(ctx context.Context, opener Opener) (*Started, error) {
	path, err := build.Path.ResolveReference(&url.URL{Path: "started.json"})
	if err != nil {
		return nil, fmt.Errorf("resolve: %w", err)
	}
	var started Started
	err = readJSON(ctx, opener, *path, &started)
	if errors.Is(err, storage.ErrObjectNotExist) {
		started.Pending = true
		return &started, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	return &started, nil
}

// Finished parses the build's finished metadata.
func (build Build) Finished(ctx context.Context, opener Opener) (*Finished, error) {
	path, err := build.Path.ResolveReference(&url.URL{Path: "finished.json"})
	if err != nil {
		return nil, fmt.Errorf("resolve: %w", err)
	}
	var finished Finished
	err = readJSON(ctx, opener, *path, &finished)
	if errors.Is(err, storage.ErrObjectNotExist) {
		finished.Running = true
		return &finished, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	return &finished, nil
}

// Artifacts writes the object name of all paths under the build's artifact dir to the output channel.
func (build Build) Artifacts(ctx context.Context, lister Lister, artifacts chan<- string) error {
	objs := lister.Objects(ctx, build.Path, "", "") // no delim or offset so we get all objects.
	for {
		obj, err := objs.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("list %s: %w", build.Path, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case artifacts <- obj.Name:
		}
	}
	return nil
}

// SuitesMeta holds testsuites xml and metadata from the filename
type SuitesMeta struct {
	Suites   junit.Suites      // suites data extracted from file contents
	Metadata map[string]string // metadata extracted from path name
	Path     string
}

func readSuites(ctx context.Context, opener Opener, p Path) (*junit.Suites, error) {
	r, err := opener.Open(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer r.Close()
	buf, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	suitesMeta, err := junit.Parse(buf)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return &suitesMeta, nil
}

// Suites takes a channel of artifact names, parses those representing junit suites, writing the result to the suites channel.
//
// Note that junit suites are parsed in parallel, so there are no guarantees about suites ordering.
func (build Build) Suites(parent context.Context, opener Opener, artifacts <-chan string, suites chan<- SuitesMeta) error {
	var wg sync.WaitGroup
	var work int

	ec := make(chan error)
	ctx, cancel := context.WithCancel(parent)

	// semaphore sets a ceiling of size go-routines slots
	size := build.suitesConcurrency
	if size == 0 {
		size = 5
	}
	semaphore := make(chan int, size)
	defer close(semaphore) // close after all goroutines are done
	defer wg.Wait()        // ensure all goroutines exit before returning
	defer cancel()

	for art := range artifacts {
		meta := parseSuitesMeta(art)
		if meta == nil {
			continue // not a junit file ignore it, ignore it
		}
		// concurrently parse each file because there may be a lot of them, and
		// each takes a non-trivial amount of time waiting for the network.
		work++
		wg.Add(1)

		go func(art string, meta map[string]string) {
			semaphore <- 1 // wait for free slot
			defer wg.Done()
			defer func() { <-semaphore }() // free up slot
			if art != "" && art[0] != '/' {
				art = "/" + art
			}
			path, err := build.Path.ResolveReference(&url.URL{Path: art})
			if err != nil {
				select {
				case <-ctx.Done():
				case ec <- fmt.Errorf("resolve %q: %w", art, err):
				}
				return
			}
			out := SuitesMeta{
				Metadata: meta,
				Path:     path.String(),
			}
			s, err := readSuites(ctx, opener, *path)
			if err != nil {
				select {
				case <-ctx.Done():
				case ec <- fmt.Errorf("read %s suites: %w", *path, err):
				}
				return
			}
			out.Suites = *s
			select {
			case <-ctx.Done():
				return
			case suites <- out:
			}

			select {
			case <-ctx.Done():
			case ec <- nil:
			}
		}(art, meta)
	}

	for ; work > 0; work-- {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout: %w", ctx.Err())
		case err := <-ec:
			if err != nil {
				return err
			}
		}
	}
	return nil
}
