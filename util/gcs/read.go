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
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"vbom.ml/util/sortorder"

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
	Bucket         *storage.BucketHandle
	Prefix         string
	BucketPath     string
	originalPrefix string
}

func (build Build) String() string {
	return "gs://" + build.BucketPath + "/" + build.Prefix
}

// Builds is a slice of builds.
type Builds []Build

func (b Builds) Len() int      { return len(b) }
func (b Builds) Swap(i, j int) { b[i], b[j] = b[j], b[i] }

// Expect builds to be in monotonically increasing order.
// So build8 < build9 < build10 < build888
func (b Builds) Less(i, j int) bool {
	return sortorder.NaturalLess(b[i].originalPrefix, b[j].originalPrefix)
}

// ListBuilds returns the array of builds under path, sorted in monotonically decreasing order.
func ListBuilds(parent context.Context, client *storage.Client, path Path) (Builds, error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	p := path.Object()
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}
	bkt := client.Bucket(path.Bucket())
	it := bkt.Objects(ctx, &storage.Query{
		Delimiter: "/",
		Prefix:    p,
	})
	var all Builds
	for {
		objAttrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %v", err)
		}

		// if this is a link under directory, resolve the build value
		if link := objAttrs.Metadata["x-goog-meta-link"]; len(link) > 0 {
			// links created by bootstrap.py have a space
			link = strings.TrimSpace(link)
			u, err := url.Parse(link)
			if err != nil {
				return nil, fmt.Errorf("could not parse link for key %s: %v", objAttrs.Name, err)
			}
			if !strings.HasSuffix(u.Path, "/") {
				u.Path += "/"
			}
			var linkPath Path
			if err := linkPath.SetURL(u); err != nil {
				return nil, fmt.Errorf("could not make GCS path for key %s: %v", objAttrs.Name, err)
			}
			all = append(all, Build{
				Bucket:         bkt,
				Prefix:         linkPath.Object(),
				BucketPath:     path.Bucket(),
				originalPrefix: objAttrs.Name,
			})
			continue
		}

		if len(objAttrs.Prefix) == 0 {
			continue
		}

		all = append(all, Build{
			Bucket:         bkt,
			Prefix:         objAttrs.Prefix,
			BucketPath:     path.Bucket(),
			originalPrefix: objAttrs.Prefix,
		})
	}
	sort.Sort(sort.Reverse(all))
	return all, nil
}

// junit_CONTEXT_TIMESTAMP_THREAD.xml
var re = regexp.MustCompile(`.+/junit(_[^_]+)?(_\d+-\d+)?(_\d+)?\.xml$`)

// dropPrefix removes the _ in _CONTEXT to help keep the regexp simple
func dropPrefix(name string) string {
	if len(name) == 0 {
		return name
	}
	return name[1:]
}

// parseSuitesMeta returns the metadata for this junit file (nil for a non-junit file).
//
// Expected format: junit_context_20180102-1256-07.xml
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
	return map[string]string{
		"Context":   dropPrefix(mat[1]),
		"Timestamp": dropPrefix(mat[2]),
		"Thread":    dropPrefix(mat[3]),
	}

}

// readOpener returns a reader and a possible error.
type readOpener func() (io.ReadCloser, error)

// gcsOpener adapts o.NewReader()'s return type to io.ReadCloser
func gcsOpener(ctx context.Context, o *storage.ObjectHandle) readOpener {
	return func() (io.ReadCloser, error) {
		return o.NewReader(ctx)
	}
}

// readJSON will decode the json object stored in GCS.
func readJSON(open readOpener, i interface{}) error {
	reader, err := open()
	if err == storage.ErrObjectNotExist {
		return err
	}
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	if err = json.NewDecoder(reader).Decode(i); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	if err := reader.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	return nil
}

// Started parses the build's started metadata.
func (build Build) Started(ctx context.Context) (*Started, error) {
	uri := build.Prefix + "started.json"
	var started Started
	err := readJSON(gcsOpener(ctx, build.Bucket.Object(uri)), &started)
	if err == storage.ErrObjectNotExist {
		started.Pending = true
		return &started, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %v", uri, err)
	}
	return &started, nil
}

// Finished parses the build's finished metadata.
func (build Build) Finished(ctx context.Context) (*Finished, error) {
	uri := build.Prefix + "finished.json"
	var finished Finished
	err := readJSON(gcsOpener(ctx, build.Bucket.Object(uri)), &finished)
	if err == storage.ErrObjectNotExist {
		finished.Running = true
		return &finished, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %v", uri, err)
	}
	return &finished, nil
}

// Artifacts writes the object name of all paths under the build's artifact dir to the output channel.
func (build Build) Artifacts(ctx context.Context, artifacts chan<- string) error {
	pref := build.Prefix
	objs := build.Bucket.Objects(ctx, &storage.Query{Prefix: pref})
	for {
		obj, err := objs.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to list %s: %v", pref, err)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("interrupted listing %s", pref)
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

// Suites takes a channel of artifact names, parses those representing junit suites, writing the result to the suites channel.
//
// Note that junit suites are parsed in parallel, so there are no guarantees about suites ordering.
func (build Build) Suites(parent context.Context, artifacts <-chan string, suites chan<- SuitesMeta) error {

	var work int

	ec := make(chan error)
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	for art := range artifacts {
		meta := parseSuitesMeta(art)
		if meta == nil {
			continue // not a junit file ignore it, ignore it
		}
		// concurrently parse each file because there may be a lot of them, and
		// each takes a non-trivial amount of time waiting for the network.
		work++
		go func(art string, meta map[string]string) {
			out := SuitesMeta{
				Metadata: meta,
				Path:     "gs://" + build.BucketPath + "/" + art,
			}
			if err := readJSON(gcsOpener(ctx, build.Bucket.Object(art)), &out.Suites); err != nil {
				select {
				case <-ctx.Done():
				case ec <- fmt.Errorf("open %q: %w", art, err):
				}
				return
			}
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
			return err
		}
	}
	return nil
}
