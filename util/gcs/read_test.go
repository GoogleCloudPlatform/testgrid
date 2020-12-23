/*
Copyright 2018 The Kubernetes Authors.

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
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"reflect"
	"sort"
	"sync"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/iterator"

	"github.com/GoogleCloudPlatform/testgrid/metadata"
	"github.com/GoogleCloudPlatform/testgrid/metadata/junit"
)

func subdir(prefix string) storage.ObjectAttrs {
	return storage.ObjectAttrs{Prefix: prefix}
}

func link(path Path, name, other string) storage.ObjectAttrs {
	return storage.ObjectAttrs{
		Metadata: map[string]string{"x-goog-meta-link": other},
		Name:     resolveOrDie(path, name).Object(),
	}
}

func TestListBuilds(t *testing.T) {
	path := newPathOrDie("gs://bucket/path/to/build/")
	cases := []struct {
		name     string
		ctx      context.Context
		iterator fakeIterator
		offset   *Path

		expected []Build
		err      bool
	}{
		{
			name: "basically works",
		},
		{
			name: "multiple paths",
			iterator: fakeIterator{
				objects: []storage.ObjectAttrs{
					subdir(resolveOrDie(path, "hello").Object()),
					subdir(resolveOrDie(path, "world").Object()),
				},
			},
			expected: []Build{
				{
					Path:           resolveOrDie(path, "world"),
					originalPrefix: "path/to/build/world",
				},
				{
					Path:           newPathOrDie("gs://bucket/path/to/build/hello"),
					originalPrefix: resolveOrDie(path, "hello").Object(),
				},
			},
		},
		{
			name: "presubmit symlinks work correctly",
			iterator: fakeIterator{
				objects: []storage.ObjectAttrs{
					link(path, "first", "gs://another-bucket/path/inside"),
					link(path, "second", "gs://second-bucket/somewhere"),
				},
			},
			expected: []Build{
				{
					Path:           newPathOrDie("gs://second-bucket/somewhere/"),
					originalPrefix: resolveOrDie(path, "second").Object(),
				},
				{
					Path:           newPathOrDie("gs://another-bucket/path/inside/"),
					originalPrefix: resolveOrDie(path, "first").Object(),
				},
			},
		},
		{
			name: "cancelled context returns error",
			iterator: fakeIterator{
				objects: []storage.ObjectAttrs{
					subdir(resolveOrDie(path, "hello").Object()),
					subdir(resolveOrDie(path, "world").Object()),
				},
			},
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			err: true,
		},
		{
			name: "iteration error returns error",
			iterator: fakeIterator{
				objects: []storage.ObjectAttrs{
					subdir(resolveOrDie(path, "hello").Object()),
					subdir(resolveOrDie(path, "world").Object()),
					subdir(resolveOrDie(path, "more").Object()),
				},
				err: 1,
			},
			err: true,
		},
		{
			name: "listing latest builds works correctly",
			iterator: fakeIterator{
				objects: []storage.ObjectAttrs{
					subdir(resolveOrDie(path, "hello").Object()),
					subdir(resolveOrDie(path, "more").Object()),
					subdir(resolveOrDie(path, "world").Object()),
				},
			},
			offset: pResolveOrDie(path, "more"),
			expected: []Build{
				{
					Path:           resolveOrDie(path, "world"),
					originalPrefix: resolveOrDie(path, "world").Object(),
				},
				{
					Path:           resolveOrDie(path, "more"),
					originalPrefix: resolveOrDie(path, "more").Object(),
				},
			},
		},
		{
			name: "listing latest builds handles numbers correctly",
			iterator: fakeIterator{
				objects: []storage.ObjectAttrs{
					subdir(resolveOrDie(path, "hello100").Object()),
					subdir(resolveOrDie(path, "hello101").Object()),
					subdir(resolveOrDie(path, "hello2000").Object()),
					subdir(resolveOrDie(path, "hello30").Object()),
					subdir(resolveOrDie(path, "hello31").Object()),
					subdir(resolveOrDie(path, "hello300").Object()),
				},
			},
			offset: pResolveOrDie(path, "hello100"),
			expected: []Build{
				{
					Path:           resolveOrDie(path, "hello2000"),
					originalPrefix: resolveOrDie(path, "hello2000").Object(),
				},
				{
					Path:           resolveOrDie(path, "hello300"),
					originalPrefix: resolveOrDie(path, "hello300").Object(),
				},
				{
					Path:           resolveOrDie(path, "hello101"),
					originalPrefix: resolveOrDie(path, "hello101").Object(),
				},
				{
					Path:           resolveOrDie(path, "hello100"),
					originalPrefix: resolveOrDie(path, "hello100").Object(),
				},
			},
		},
		{
			name: "listing latest presubmit symlinks handles numbers",
			iterator: fakeIterator{
				objects: []storage.ObjectAttrs{
					link(path, "100", "gs://another-bucket/path/inside/100"),
					link(path, "101", "gs://second-bucket/somewhere/101"),
					link(path, "202", "gs://third-bucket/else/202"),
					link(path, "2004", "gs://third-bucket/else/2004"),
					link(path, "30", "gs://third-bucket/else/30"),
					link(path, "303", "gs://third-bucket/else/303"),
				},
			},
			offset: pResolveOrDie(path, "100"),
			expected: []Build{
				{
					Path:           newPathOrDie("gs://third-bucket/else/2004/"),
					originalPrefix: pResolveOrDie(path, "2004").Object(),
				},
				{
					Path:           newPathOrDie("gs://third-bucket/else/303/"),
					originalPrefix: pResolveOrDie(path, "303").Object(),
				},
				{
					Path:           newPathOrDie("gs://third-bucket/else/202/"),
					originalPrefix: pResolveOrDie(path, "202").Object(),
				},
				{
					Path:           newPathOrDie("gs://second-bucket/somewhere/101/"),
					originalPrefix: pResolveOrDie(path, "101").Object(),
				},
				{
					Path:           newPathOrDie("gs://another-bucket/path/inside/100/"),
					originalPrefix: pResolveOrDie(path, "100").Object(),
				},
			},
		},
		{
			name: "listing latest presubmit symlinks work correctly",
			iterator: fakeIterator{
				objects: []storage.ObjectAttrs{
					link(path, "first", "gs://another-bucket/path/inside"),
					link(path, "second", "gs://second-bucket/somewhere"),
					link(path, "third", "gs://third-bucket/else"),
				},
			},
			offset: pResolveOrDie(path, "second"),
			expected: []Build{
				{
					Path:           newPathOrDie("gs://third-bucket/else/"),
					originalPrefix: pResolveOrDie(path, "third").Object(),
				},
				{
					Path:           newPathOrDie("gs://second-bucket/somewhere/"),
					originalPrefix: pResolveOrDie(path, "second").Object(),
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fl := fakeLister{path: tc.iterator}
			ctx := tc.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			actual, err := ListBuilds(ctx, fl, path, tc.offset)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("ListBuilds(): unexpected error: %v", err)
				}
			case tc.err:
				t.Errorf("ListBuilds(): failed to receive an error")
			default:
				if diff := cmp.Diff(actual, tc.expected, cmp.AllowUnexported(Build{}, Path{})); diff != "" {
					t.Errorf("ListBuilds(): got unexpected diff (-have, +want):\n%s", diff)
				}
			}
		})
	}
}

func TestReadLink(t *testing.T) {
	cases := []struct {
		name     string
		meta     map[string]string
		expected string
	}{
		{
			name: "basically works",
			meta: map[string]string{},
		},
		{
			name: "find link",
			meta: map[string]string{
				"link": "foo",
			},
			expected: "foo",
		},
		{
			name: "find x-goog-meta-link",
			meta: map[string]string{
				"x-goog-meta-link": "foo",
			},
			expected: "foo",
		},
		{
			name: "ignore random",
			meta: map[string]string{
				"x-random-link": "foo",
			},
		},
		{
			name: "prefer x-goog-meta-link",
			meta: map[string]string{
				"x-goog-meta-link": "yes",
				"link":             "no",
			},
			expected: "yes",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var oa storage.ObjectAttrs
			oa.Metadata = tc.meta
			if actual := readLink(&oa); actual != tc.expected {
				t.Errorf("readLink(%v) got %q want %q", oa, actual, tc.expected)
			}
		})
	}
}

func TestParseSuitesMeta(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		context   string
		timestamp string
		thread    string
		empty     bool
	}{

		{
			name:  "not junit",
			input: "./started.json",
			empty: true,
		},
		{
			name:  "forgot suffix",
			input: "./junit",
			empty: true,
		},
		{
			name:  "basic",
			input: "./junit.xml",
		},
		{
			name:    "context",
			input:   "./junit_hello world isn't-this exciting!.xml",
			context: "hello world isn't-this exciting!",
		},
		{
			name:    "numeric context",
			input:   "./junit_12345.xml",
			context: "12345",
		},
		{
			name:    "context and thread",
			input:   "./junit_context_12345.xml",
			context: "context",
			thread:  "12345",
		},
		{
			name:      "context and timestamp",
			input:     "./junit_context_20180102-1234.xml",
			context:   "context",
			timestamp: "20180102-1234",
		},
		{
			name:      "context thread timestamp",
			input:     "./junit_context_20180102-1234_5555.xml",
			context:   "context",
			timestamp: "20180102-1234",
			thread:    "5555",
		},
		{
			name:    "accept weird junit name",
			input:   "./junit.e2e_suite.3.xml",
			context: ".e2e_suite.3",
		},
	}

	for _, tc := range cases {
		actual := parseSuitesMeta(tc.input)
		switch {
		case actual == nil && !tc.empty:
			t.Errorf("%s: unexpected nil map", tc.name)
		case actual != nil && tc.empty:
			t.Errorf("%s: should not have returned a map: %v", tc.name, actual)
		case actual != nil:
			for k, expected := range map[string]string{
				"Context":   tc.context,
				"Thread":    tc.thread,
				"Timestamp": tc.timestamp,
			} {
				if a, ok := actual[k]; !ok {
					t.Errorf("%s: missing key %s", tc.name, k)
				} else if a != expected {
					t.Errorf("%s: %s actual %s != expected %s", tc.name, k, a, expected)
				}
			}
		}
	}

}

func TestReadJSON(t *testing.T) {
	cases := []struct {
		name     string
		obj      *fakeObject
		actual   interface{}
		expected interface{}
		is       error
	}{
		{
			name:     "basically works",
			obj:      &fakeObject{data: "{}"},
			actual:   &Started{},
			expected: &Started{},
		},
		{
			name: "read a json object",
			obj:  &fakeObject{data: "{\"hello\": 5}"},
			actual: &struct {
				Hello int `json:"hello"`
			}{},
			expected: &struct {
				Hello int `json:"hello"`
			}{5},
		},
		{
			name: "ErrObjectNotExist on open returns an ErrObjectNotExist error",
			is:   storage.ErrObjectNotExist,
		},
		{
			name: "other open errors also error",
			obj:  &fakeObject{openErr: errors.New("injected open error")},
		},
		{
			name: "read error errors",
			obj: &fakeObject{
				data:    "{}",
				readErr: errors.New("injected read error"),
			},
		},
		{
			name: "close error errors",
			obj: &fakeObject{
				data:     "{}",
				closeErr: errors.New("injected close error"),
			},
		},
		{
			name: "invalid json errors",
			obj: &fakeObject{
				data:     "{\"json\": \"hates trailing commas\",}",
				closeErr: errors.New("injected close error"),
			},
		},
	}

	path := newPathOrDie("gs://bucket/path/to/something")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fo := fakeOpener{}
			if tc.obj != nil {
				fo[path] = *tc.obj
			}
			err := readJSON(context.Background(), fo, path, tc.actual)
			switch {
			case err != nil:
				if tc.expected != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if tc.is != nil && !errors.Is(err, tc.is) {
					t.Errorf("bad error: %v, wanted %v", err, tc.is)
				}
			case tc.expected == nil:
				t.Error("failed to receive expected error")
			default:
				if !reflect.DeepEqual(tc.actual, tc.expected) {
					t.Errorf("got %v, want %v", tc.actual, tc.expected)
				}
			}
		})
	}
}

type fakeOpener map[Path]fakeObject

func (fo fakeOpener) Open(ctx context.Context, path Path) (io.ReadCloser, error) {
	o, ok := fo[path]
	if !ok {
		return nil, fmt.Errorf("wrap not exist: %w", storage.ErrObjectNotExist)
	}
	if o.openErr != nil {
		return nil, o.openErr
	}
	return ioutil.NopCloser(&fakeReader{
		buf:      bytes.NewBufferString(o.data),
		readErr:  o.readErr,
		closeErr: o.closeErr,
	}), nil
}

type fakeObject struct {
	data     string
	openErr  error
	readErr  error
	closeErr error
}

type fakeReader struct {
	buf      *bytes.Buffer
	readErr  error
	closeErr error
}

func (fr *fakeReader) Read(p []byte) (int, error) {
	if fr.readErr != nil {
		return 0, fr.readErr
	}
	return fr.buf.Read(p)
}

func (fr *fakeReader) Close() error {
	if fr.closeErr != nil {
		return fr.closeErr
	}
	fr.readErr = errors.New("already closed")
	fr.closeErr = fr.readErr
	return nil
}

type fakeLister map[Path]fakeIterator

func (fl fakeLister) Objects(ctx context.Context, path Path, _, offset string) Iterator {
	f := fl[path]
	f.ctx = ctx
	f.offset = offset
	return &f
}

type fakeIterator struct {
	objects []storage.ObjectAttrs
	idx     int
	err     int // must be > 0
	ctx     context.Context
	offset  string
}

func (fi *fakeIterator) Next() (*storage.ObjectAttrs, error) {
	if fi.ctx.Err() != nil {
		return nil, fi.ctx.Err()
	}
	for fi.idx < len(fi.objects) {
		if fi.offset == "" {
			break
		}
		name, prefix := fi.objects[fi.idx].Name, fi.objects[fi.idx].Prefix
		if (name == "" || name >= fi.offset) && (prefix == "" || prefix >= fi.offset) {
			break
		}
		fi.idx++
	}
	if fi.idx >= len(fi.objects) {
		return nil, iterator.Done
	}
	if fi.idx > 0 && fi.idx == fi.err {
		return nil, errors.New("injected fakeIterator error")
	}

	o := fi.objects[fi.idx]
	fi.idx++
	return &o, nil
}

func TestStarted(t *testing.T) {
	path := newPathOrDie("gs://bucket/path/")
	started := resolveOrDie(path, "started.json")
	cases := []struct {
		name     string
		ctx      context.Context
		object   *fakeObject
		expected *Started
		checkErr error
	}{
		{
			name:     "basically works",
			object:   &fakeObject{data: "{}"},
			expected: &Started{},
		},
		{
			name:   "canceled context returns error",
			object: &fakeObject{},
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
		},
		{
			name: "all fields parsed",
			object: &fakeObject{
				data: `{
                    "timestamp": 1234,
                    "node": "machine",
                    "pull": "your leg",
                    "repos": {
                        "main": "deadbeef"
                    },
                    "repo-commit": "11111",
                    "metadata": {
                        "version": "fun",
                        "float": 1.2,
                        "object": {"yes": true}
                    }
                }`,
			},
			expected: &Started{
				Started: metadata.Started{
					Timestamp: 1234,
					Node:      "machine",
					Pull:      "your leg",
					Repos: map[string]string{
						"main": "deadbeef",
					},
					RepoCommit: "11111",
					Metadata: metadata.Metadata{
						"version": "fun",
						"float":   1.2,
						"object": map[string]interface{}{
							"yes": true,
						},
					},
				},
			},
		},
		{
			name:     "missing object means pending",
			expected: &Started{Pending: true},
		},
		{
			name:   "read error returns an error",
			object: &fakeObject{readErr: errors.New("injected read error")},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fo := fakeOpener{}
			if tc.object != nil {
				fo[started] = *tc.object
			}
			b := Build{Path: path}
			if tc.ctx == nil {
				tc.ctx = context.Background()
			}
			ctx, cancel := context.WithCancel(tc.ctx)
			defer cancel()
			actual, err := b.Started(ctx, fo)
			switch {
			case err != nil:
				if tc.expected != nil {
					t.Errorf("Started(): unexpected error: %v", err)
				}
			default:
				if !reflect.DeepEqual(actual, tc.expected) {
					t.Errorf("Started(): got %v, want %v", actual, tc.expected)
				}
			}

		})
	}
}

func TestFinished(t *testing.T) {
	yes := true
	path := newPathOrDie("gs://bucket/path/")
	finished := resolveOrDie(path, "finished.json")
	cases := []struct {
		name     string
		ctx      context.Context
		object   *fakeObject
		expected *Finished
		checkErr error
	}{
		{
			name:     "basically works",
			object:   &fakeObject{data: "{}"},
			expected: &Finished{},
		},
		{
			name:   "canceled context returns error",
			object: &fakeObject{},
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
		},
		{
			name: "all fields parsed",
			object: &fakeObject{
				data: `{
                    "timestamp": 1234,
                    "passed": true,
                    "metadata": {
                        "version": "fun",
                        "float": 1.2,
                        "object": {"yes": true}
                    }
                }`,
			},
			expected: &Finished{
				Finished: metadata.Finished{
					Timestamp: func() *int64 {
						var out int64 = 1234
						return &out
					}(),
					Passed: &yes,
					Metadata: metadata.Metadata{
						"version": "fun",
						"float":   1.2,
						"object": map[string]interface{}{
							"yes": true,
						},
					},
				},
			},
		},
		{
			name:     "missing object means running",
			expected: &Finished{Running: true},
		},
		{
			name:   "read error returns an error",
			object: &fakeObject{readErr: errors.New("injected read error")},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fo := fakeOpener{}
			if tc.object != nil {
				fo[finished] = *tc.object
			}
			b := Build{Path: path}
			if tc.ctx == nil {
				tc.ctx = context.Background()
			}
			ctx, cancel := context.WithCancel(tc.ctx)
			defer cancel()
			actual, err := b.Finished(ctx, fo)
			switch {
			case err != nil:
				if tc.expected != nil {
					t.Errorf("Finished(): unexpected error: %v", err)
				}
			default:
				if !reflect.DeepEqual(actual, tc.expected) {
					t.Errorf("Finished(): got %v, want %v", actual, tc.expected)
				}
			}

		})
	}
}

func resolveOrDie(p Path, s string) Path {
	out, err := p.ResolveReference(&url.URL{Path: s})
	if err != nil {
		panic(fmt.Sprintf("%s - %s", p, err))
	}
	return *out
}

func pResolveOrDie(p Path, s string) *Path {
	out := resolveOrDie(p, s)
	return &out
}

func newPathOrDie(s string) Path {
	p, err := NewPath(s)
	if err != nil {
		panic(err)
	}
	return *p
}

func TestReadSuites(t *testing.T) {
	path := newPathOrDie("gs://bucket/object")
	cases := []struct {
		name     string
		ctx      context.Context
		opener   fakeOpener
		expected *junit.Suites
		checkErr error
	}{
		{
			name: "basically works",
			opener: fakeOpener{
				path: {
					data: `<testsuites><testsuite><testcase name="foo"/></testsuite></testsuites>`,
				},
			},
			expected: &junit.Suites{
				XMLName: xml.Name{Local: "testsuites"},
				Suites: []junit.Suite{
					{
						XMLName: xml.Name{Local: "testsuite"},
						Results: []junit.Result{
							{
								Name: "foo",
							},
						},
					},
				},
			},
		},
		{
			name:     "not found returns not found error",
			checkErr: storage.ErrObjectNotExist,
		},
		{
			name: "invalid junit returns error",
			opener: fakeOpener{
				path: {data: `<wrong><type></type></wrong>`},
			},
		},
		{
			name: "read error returns error",
			opener: fakeOpener{
				path: {
					readErr: errors.New("injected read error"),
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := readSuites(tc.ctx, tc.opener, path)
			switch {
			case err != nil:
				if tc.expected != nil {
					t.Errorf("readSuites(): unexpected error: %v", err)
				} else if tc.checkErr != nil && !errors.Is(err, tc.checkErr) {
					t.Errorf("readSuites(): bad error %v, wanted %v", err, tc.checkErr)
				}
			case tc.expected == nil:
				t.Error("readSuites(): failed to receive an error")
			default:
				if !reflect.DeepEqual(actual, tc.expected) {
					t.Errorf("readSuites(): got %v, want %v", actual, tc.expected)
				}
			}
		})
	}
}

func TestArtifacts(t *testing.T) {
	path := newPathOrDie("gs://bucket/path/")
	cases := []struct {
		name     string
		ctx      context.Context
		iterator fakeIterator
		expected []string
		err      bool
	}{
		{
			name: "basically works",
		},
		{
			name: "cancelled context returns error",
			iterator: fakeIterator{
				objects: []storage.ObjectAttrs{
					{Name: "whatever"},
					{Name: "stuff"},
				},
			},
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			err: true,
		},
		{
			name: "iteration error returns error",
			iterator: fakeIterator{
				objects: []storage.ObjectAttrs{
					{Name: "hello"},
					{Name: "boom"},
					{Name: "world"},
				},
				err: 1,
			},
			err: true,
		},
		{
			name: "multiple objects work",
			iterator: fakeIterator{
				objects: []storage.ObjectAttrs{
					{Name: "hello"},
					{Name: "world"},
				},
			},
			expected: []string{"hello", "world"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := Build{
				Path: path,
			}
			var actual []string
			ch := make(chan string)
			var lock sync.Mutex
			lock.Lock()
			go func() {
				defer lock.Unlock()
				for a := range ch {
					actual = append(actual, a)
				}
			}()
			if tc.ctx == nil {
				tc.ctx = context.Background()
			}
			fl := fakeLister{path: tc.iterator}
			err := b.Artifacts(tc.ctx, fl, ch)
			close(ch)
			lock.Lock()
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("Artifacts(): unexpected error: %v", err)
				}
			case tc.err:
				t.Errorf("Artifacts(): failed to receive an error")
			default:
				if !reflect.DeepEqual(actual, tc.expected) {
					t.Errorf("Artifacts(): got %v, want %v", actual, tc.expected)
				}
			}
		})
	}
}

func TestSuites(t *testing.T) {
	cases := []struct {
		name        string
		ctx         context.Context
		path        Path
		artifacts   map[string]string
		concurrency int

		expected []SuitesMeta
		err      bool
	}{
		{
			name: "basically works",
		},
		{
			name: "ignore random file",
			path: newPathOrDie("gs://where/whatever"),
			artifacts: map[string]string{
				"/something/ignore.txt":  "hello",
				"/something/ignore.json": "{}",
			},
		},
		{
			name: "support testsuite",
			path: newPathOrDie("gs://where/whatever"),
			artifacts: map[string]string{
				"/something/junit.xml": `<testsuites><testsuite><testcase name="foo"/></testsuite></testsuites>`,
			},
			expected: []SuitesMeta{
				{
					Suites: junit.Suites{
						XMLName: xml.Name{Local: "testsuites"},
						Suites: []junit.Suite{
							{
								XMLName: xml.Name{Local: "testsuite"},
								Results: []junit.Result{
									{
										Name: "foo",
									},
								},
							},
						},
					},
					Metadata: parseSuitesMeta("/something/junit.xml"),
					Path:     "gs://where/something/junit.xml",
				},
			},
		},
		{
			name:        "support testsuite with minimal concurrency",
			concurrency: 1,
			path:        newPathOrDie("gs://where/whatever"),
			artifacts: func() map[string]string {
				out := map[string]string{}
				for i := 0; i < 3; i++ {
					out[fmt.Sprintf("/something/junit_%d.xml", i)] = `<testsuites><testsuite><testcase name="foo"/></testsuite></testsuites>`
				}
				return out
			}(),
			expected: func() []SuitesMeta {
				templ := SuitesMeta{
					Suites: junit.Suites{
						XMLName: xml.Name{Local: "testsuites"},
						Suites: []junit.Suite{
							{
								XMLName: xml.Name{Local: "testsuite"},
								Results: []junit.Result{
									{
										Name: "foo",
									},
								},
							},
						},
					},
					Metadata: parseSuitesMeta("/something/junit.xml"),
					Path:     "gs://where/something/junit.xml",
				}
				var out []SuitesMeta
				for i := 0; i < 3; i++ {
					name := fmt.Sprintf("/something/junit_%d.xml", i)
					templ.Metadata = parseSuitesMeta(name)
					templ.Path = "gs://where" + name
					out = append(out, templ)
				}
				return out
			}(),
		},
		{
			name: "support testsuites",
			path: newPathOrDie("gs://where/whatever"),
			artifacts: map[string]string{
				"/something/junit.xml": `<testsuite><testcase name="foo"/></testsuite>`,
			},
			expected: []SuitesMeta{
				{
					Suites: junit.Suites{
						Suites: []junit.Suite{
							{
								XMLName: xml.Name{Local: "testsuite"},
								Results: []junit.Result{
									{
										Name: "foo",
									},
								},
							},
						},
					},
					Metadata: parseSuitesMeta("/something/junit.xml"),
					Path:     "gs://where/something/junit.xml",
				},
			},
		},
		{
			name: "capture metadata",
			path: newPathOrDie("gs://where/whatever"),
			artifacts: map[string]string{
				"/something/junit_foo-context_20200708-1234_88.xml": `<testsuite><testcase name="foo"/></testsuite>`,
				"/something/junit_bar-context_20211234-0808_33.xml": `<testsuite><testcase name="bar"/></testsuite>`,
			},
			expected: []SuitesMeta{
				{
					Suites: junit.Suites{
						Suites: []junit.Suite{
							{
								XMLName: xml.Name{Local: "testsuite"},
								Results: []junit.Result{
									{
										Name: "foo",
									},
								},
							},
						},
					},
					Metadata: parseSuitesMeta("/something/junit_foo-context_20200708-1234_88.xml"),
					Path:     "gs://where/something/junit_foo-context_20200708-1234_88.xml",
				},
				{
					Suites: junit.Suites{
						Suites: []junit.Suite{
							{
								XMLName: xml.Name{Local: "testsuite"},
								Results: []junit.Result{
									{
										Name: "bar",
									},
								},
							},
						},
					},
					Metadata: parseSuitesMeta("/something/junit_bar-context_20211234-0808_33.xml"),
					Path:     "gs://where/something/junit_bar-context_20211234-0808_33.xml",
				},
			},
		},
		{
			name: "read suites error returns errors",
			path: newPathOrDie("gs://where/whatever"),
			artifacts: map[string]string{
				"/something/junit.xml": `<this is invalid json`,
			},
			err: true,
		},
		{
			name: "interrupted context returns error",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			path: newPathOrDie("gs://where/whatever"),
			artifacts: map[string]string{
				"/something/junit_foo-context_20200708-1234_88.xml": `<testsuite><testcase name="foo"/></testsuite>`,
			},
			err: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fo := fakeOpener{}
			b := Build{
				Path:              tc.path,
				suitesConcurrency: tc.concurrency,
			}
			for s, data := range tc.artifacts {
				fo[resolveOrDie(b.Path, s)] = fakeObject{data: data}
			}

			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.ctx == nil {
				tc.ctx = parent
			}
			arts := make(chan string)
			go func() {
				defer close(arts)
				for a := range tc.artifacts {
					select {
					case arts <- a:
					case <-parent.Done():
						return
					}
				}
			}()

			var actual []SuitesMeta
			suites := make(chan SuitesMeta)
			var lock sync.Mutex
			lock.Lock()
			go func() {
				defer lock.Unlock()
				for sm := range suites {
					actual = append(actual, sm)
				}
			}()

			err := b.Suites(tc.ctx, fo, arts, suites)
			close(suites)
			lock.Lock() // ensure actual is up to date
			defer lock.Unlock()
			// actual items appended in random order, so sort for consistency.
			sort.SliceStable(actual, func(i, j int) bool {
				return actual[i].Path < actual[j].Path
			})
			sort.SliceStable(tc.expected, func(i, j int) bool {
				return tc.expected[i].Path < tc.expected[j].Path
			})
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("Suites() unexpected error: %v", err)
				}
			case tc.err:
				t.Errorf("Suites() failed to receive expected error")
			default:
				if !reflect.DeepEqual(actual, tc.expected) {
					t.Errorf("Suites() got %#v, want %#v", actual, tc.expected)
				}
			}
		})
	}
}
