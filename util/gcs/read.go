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
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/fvbommel/sortorder"
	"google.golang.org/api/iterator"
	core "k8s.io/api/core/v1"

	"github.com/GoogleCloudPlatform/testgrid/metadata"
	"github.com/GoogleCloudPlatform/testgrid/metadata/junit"
)

// PodInfo holds podinfo.json (data about the pod).
type PodInfo struct {
	Pod *core.Pod `json:"pod,omitempty"`
	// ignore unused events
}

const (
	// MissingPodInfo appears when builds complete without a podinfo.json report.
	MissingPodInfo = "podinfo.json not found, please install prow's GCS reporter"
	// NoPodUtils appears when builds run without decoration.
	NoPodUtils = "not using decoration, please set decorate: true on prowjob"
)

func truncate(s string, max int) string {
	if max <= 0 {
		return s
	}
	l := len(s)
	if l < max {
		return s
	}
	h := max / 2
	return s[:h] + "..." + s[l-h:]
}

func checkContainerStatus(status core.ContainerStatus) (bool, string) {
	name := status.Name
	if status.State.Waiting != nil {
		return false, fmt.Sprintf("%s still waiting: %s", name, status.State.Waiting.Message)
	}
	if status.State.Running != nil {
		return false, fmt.Sprintf("%s still running", name)
	}
	if status.State.Terminated != nil && status.State.Terminated.ExitCode != 0 {
		return false, fmt.Sprintf("%s exited %d: %s", name, status.State.Terminated.ExitCode, truncate(status.State.Terminated.Message, 140))
	}
	return true, ""
}

// Summarize returns if the pod completed successfully and a diagnostic message.
func (pi PodInfo) Summarize() (bool, string) {
	if pi.Pod == nil {
		return false, MissingPodInfo
	}

	if pi.Pod.Status.Phase == core.PodSucceeded {
		return true, ""
	}

	conditions := make(map[core.PodConditionType]core.PodCondition, len(pi.Pod.Status.Conditions))

	for _, cond := range pi.Pod.Status.Conditions {
		conditions[cond.Type] = cond
	}

	if cond, ok := conditions[core.PodScheduled]; ok && cond.Status != core.ConditionTrue {
		return false, fmt.Sprintf("pod did not schedule: %s", cond.Message)
	}

	if cond, ok := conditions[core.PodInitialized]; ok && cond.Status != core.ConditionTrue {
		return false, fmt.Sprintf("pod could not initialize: %s", cond.Message)
	}

	for _, status := range pi.Pod.Status.InitContainerStatuses {
		if pass, msg := checkContainerStatus(status); !pass {
			return pass, fmt.Sprintf("init container %s", msg)
		}
	}

	var foundSidecar bool
	for _, status := range pi.Pod.Status.ContainerStatuses {
		if status.Name == "sidecar" {
			foundSidecar = true
		}
		pass, msg := checkContainerStatus(status)
		if pass {
			continue
		}
		if status.Name == "sidecar" {
			return pass, msg
		}
		if status.State.Terminated == nil {
			return pass, msg
		}
	}

	if !foundSidecar {
		return true, NoPodUtils
	}
	return true, ""
}

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
	Path     Path
	baseName string
}

func (build Build) object() string {
	o := build.Path.Object()
	if strings.HasSuffix(o, "/") {
		return o[0 : len(o)-1]
	}
	return o
}

// Build is the unique invocation id of the job.
func (build Build) Build() string {
	return path.Base(build.object())
}

// Job is the name of the job for this build
func (build Build) Job() string {
	return path.Base(path.Dir(build.object()))
}

func (build Build) String() string {
	return build.Path.String()
}

func readLink(objAttrs *storage.ObjectAttrs) string {
	if link, ok := objAttrs.Metadata["x-goog-meta-link"]; ok {
		return link
	}
	if link, ok := objAttrs.Metadata["link"]; ok {
		return link
	}
	return ""
}

// hackOffset handles tot's sequential names, which GCS handles poorly
// AKA asking GCS to return results after 6 will never find 10
// So we always have to list everything for these types of numbers.
func hackOffset(offset *string) string {
	if *offset == "" {
		return ""
	}
	if strings.HasSuffix(*offset, "/") {
		*offset = (*offset)[:len(*offset)-1]
	}
	dir, offsetBaseName := path.Split(*offset)
	const first = 1000000000000000000
	if n, err := strconv.Atoi(offsetBaseName); err == nil && n < first {
		*offset = path.Join(dir, "0")
	}
	return offsetBaseName
}

// ListBuilds returns the array of builds under path, sorted in monotonically decreasing order.
func ListBuilds(parent context.Context, lister Lister, gcsPath Path, after *Path) ([]Build, error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	var offset string
	if after != nil {
		offset = after.Object()
	}
	offsetBaseName := hackOffset(&offset)
	if !strings.HasSuffix(offset, "/") {
		offset += "/"
	}
	it := lister.Objects(ctx, gcsPath, "/", offset)
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
		if link := readLink(objAttrs); link != "" {
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
				Path:     linkPath,
				baseName: path.Base(objAttrs.Name),
			})
			continue
		}

		if objAttrs.Prefix == "" {
			continue // not a symlink to a directory
		}

		loc := "gs://" + gcsPath.Bucket() + "/" + objAttrs.Prefix
		gcsPath, err := NewPath(loc)
		if err != nil {
			return nil, fmt.Errorf("bad path %q: %w", loc, err)
		}

		all = append(all, Build{
			Path:     *gcsPath,
			baseName: path.Base(objAttrs.Prefix),
		})
	}

	Sort(all)

	if offsetBaseName != "" {
		// GCS will return 200 2000 30 for a prefix of 100
		// testgrid expects this as 2000 200 (dropping 30)
		for i, b := range all {
			if sortorder.NaturalLess(b.baseName, offsetBaseName) || b.baseName == offsetBaseName {
				return all[:i], nil // b <= offsetBaseName, so skip this one
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
	reader, _, err := opener.Open(ctx, p)
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

// PodInfo parses the build's pod state.
func (build Build) PodInfo(ctx context.Context, opener Opener) (*PodInfo, error) {
	path, err := build.Path.ResolveReference(&url.URL{Path: "podinfo.json"})
	if err != nil {
		return nil, fmt.Errorf("resolve: %w", err)
	}
	var podInfo PodInfo
	err = readJSON(ctx, opener, *path, &podInfo)
	if errors.Is(err, storage.ErrObjectNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	return &podInfo, nil
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
	Suites   *junit.Suites     // suites data extracted from file contents
	Metadata map[string]string // metadata extracted from path name
	Path     string
	Err      error
}

const (
	maxSize int64 = 100e6 // 100 million, coarce to int not float
)

func readSuites(ctx context.Context, opener Opener, p Path) (*junit.Suites, error) {
	r, attrs, err := opener.Open(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer r.Close()
	if attrs != nil && attrs.Size > maxSize {
		return nil, fmt.Errorf("too large: %d bytes > %d bytes max", attrs.Size, maxSize)
	}
	suitesMeta, err := junit.ParseStream(r)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return suitesMeta, nil
}

// Suites takes a channel of artifact names, parses those representing junit suites, sending the result to the suites channel.
//
// Truncates xml results when set to a positive number of max bytes.
func (build Build) Suites(ctx context.Context, opener Opener, artifacts <-chan string, suites chan<- SuitesMeta, max int) error {
	for {
		var art string
		var more bool
		select {
		case <-ctx.Done():
			return ctx.Err()
		case art, more = <-artifacts:
			if !more {
				return nil
			}
		}
		meta := parseSuitesMeta(art)
		if meta == nil {
			continue // not a junit file ignore it, ignore it
		}
		if art != "" && art[0] != '/' {
			art = "/" + art
		}
		path, err := build.Path.ResolveReference(&url.URL{Path: art})
		if err != nil {
			return fmt.Errorf("resolve %q: %v", art, err)
		}
		out := SuitesMeta{
			Metadata: meta,
			Path:     path.String(),
		}
		out.Suites, err = readSuites(ctx, opener, *path)
		if err != nil {
			out.Err = err
		} else {
			out.Suites.Truncate(max)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case suites <- out:
		}
	}
}
