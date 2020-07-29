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
	"google.golang.org/api/iterator"

	"github.com/GoogleCloudPlatform/testgrid/metadata"
	"github.com/GoogleCloudPlatform/testgrid/metadata/junit"
)

func subdir(prefix string) storage.ObjectAttrs {
	return storage.ObjectAttrs{Prefix: prefix}
}

func link(name, other string) storage.ObjectAttrs {
	return storage.ObjectAttrs{
		Metadata: map[string]string{"x-goog-meta-link": other},
		Name:     name,
	}
}

func TestListBuilds(t *testing.T) {
	path := newPathOrDie("gs://bucket/path/to/build/")
	cases := []struct {
		name     string
		iterator fakeIterator
		expected Builds
		err      bool
		ctx      context.Context
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
			expected: Builds{
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
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fc := fakeClient{
				path: {
					objects: func(ctx context.Context) Iterator {
						tc.iterator.ctx = ctx
						return &tc.iterator
					},
				},
			}
			for i := range tc.expected {
				tc.expected[i].client = fc
			}
			ctx := tc.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			actual, err := ListBuilds(ctx, fc, path)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("ListBuilds(): unexpected error: %v", err)
				}
			case tc.err:
				t.Errorf("ListBuilds(): failed to receive an error")
			default:
				if !reflect.DeepEqual(actual, tc.expected) {
					t.Errorf("ListBuilds(): got %v, want %v", actual, tc.expected)
				}
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

func TestReadJSON(t *testing.T) {
	cases := []struct {
		name     string
		reader   fakeReader
		openErr  error
		actual   interface{}
		expected interface{}
		is       error
	}{
		{
			name:     "basically works",
			reader:   fakeReader{buf: bytes.NewBufferString("{}")},
			actual:   &Started{},
			expected: &Started{},
		},
		{
			name:   "read a json object",
			reader: fakeReader{buf: bytes.NewBufferString("{\"hello\": 5}")},
			actual: &struct {
				Hello int `json:"hello"`
			}{},
			expected: &struct {
				Hello int `json:"hello"`
			}{5},
		},
		{
			name:    "ErrObjectNotExist on open returns an ErrObjectNotExist error",
			openErr: storage.ErrObjectNotExist,
			is:      storage.ErrObjectNotExist,
		},
		{
			name:    "other open errors also error",
			openErr: errors.New("injected open error"),
		},
		{
			name: "read error errors",
			reader: fakeReader{
				buf:     bytes.NewBufferString("{}"),
				readErr: errors.New("injected read error"),
			},
		},
		{
			name: "close error errors",
			reader: fakeReader{
				buf:      bytes.NewBufferString("{}"),
				closeErr: errors.New("injected close error"),
			},
		},
		{
			name: "invalid json errors",
			reader: fakeReader{
				buf:      bytes.NewBufferString("{\"json\": \"hates trailing commas\",}"),
				closeErr: errors.New("injected close error"),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakeOpen := func() (io.ReadCloser, error) {
				if tc.openErr != nil {
					return nil, tc.openErr
				}
				return &tc.reader, nil
			}
			err := readJSON(fakeOpen, tc.actual)
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

type fakeIterator struct {
	objects []storage.ObjectAttrs
	idx     int
	err     int // must be > 0
	ctx     context.Context
}

func (fi *fakeIterator) Next() (*storage.ObjectAttrs, error) {
	if fi.ctx != nil && fi.ctx.Err() != nil {
		return nil, fi.ctx.Err()
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

type fakeObject struct {
	data    string
	objects func(context.Context) Iterator
	open    error
	read    error
}

type fakeClient map[Path]fakeObject

func (fc fakeClient) Objects(ctx context.Context, path Path, _ string) Iterator {
	f := fc[path].objects
	if f == nil {
		return &fakeIterator{}
	}
	return f(ctx)
}

func (fc fakeClient) Open(ctx context.Context, path Path) (io.ReadCloser, error) {
	o, ok := fc[path]
	if !ok {
		return nil, fmt.Errorf("wrap not exist: %w", storage.ErrObjectNotExist)
	}
	if o.open != nil {
		return nil, o.open
	}
	return ioutil.NopCloser(&fakeReader{
		buf:     bytes.NewBufferString(o.data),
		readErr: o.read,
	}), nil
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
			object: &fakeObject{read: errors.New("injected read error")},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fc := fakeClient{}
			if tc.object != nil {
				fc[started] = *tc.object
			}
			b := Build{
				Path:   path,
				client: fc,
			}
			if tc.ctx == nil {
				tc.ctx = context.Background()
			}
			ctx, cancel := context.WithCancel(tc.ctx)
			defer cancel()
			actual, err := b.Started(ctx)
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
			object: &fakeObject{read: errors.New("injected read error")},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fc := fakeClient{}
			if tc.object != nil {
				fc[finished] = *tc.object
			}
			b := Build{
				Path:   path,
				client: fc,
			}
			if tc.ctx == nil {
				tc.ctx = context.Background()
			}
			ctx, cancel := context.WithCancel(tc.ctx)
			defer cancel()
			actual, err := b.Finished(ctx)
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
		client   fakeClient
		expected *junit.Suites
		checkErr error
	}{
		{
			name: "basically works",
			client: fakeClient{
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
			client: fakeClient{
				path: {data: `<wrong><type></type></wrong>`},
			},
		},
		{
			name: "read error returns error",
			client: fakeClient{
				path: {
					read: errors.New("injected read error"),
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := readSuites(tc.ctx, tc.client, path)
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
		iterator func(context.Context) Iterator
		expected []string
		err      bool
	}{
		{
			name: "basically works",
		},
		{
			name: "cancelled context returns error",
			iterator: func(ctx context.Context) Iterator {
				return &fakeIterator{
					objects: []storage.ObjectAttrs{
						{Name: "whatever"},
						{Name: "stuff"},
					},
					ctx: ctx,
				}
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
			iterator: func(ctx context.Context) Iterator {
				return &fakeIterator{
					objects: []storage.ObjectAttrs{
						{Name: "hello"},
						{Name: "boom"},
						{Name: "world"},
					},
					err: 1,
					ctx: ctx,
				}
			},
			err: true,
		},
		{
			name: "multiple objects work",
			iterator: func(ctx context.Context) Iterator {
				return &fakeIterator{
					objects: []storage.ObjectAttrs{
						{Name: "hello"},
						{Name: "world"},
					},
					ctx: ctx,
				}
			},
			expected: []string{"hello", "world"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := Build{
				Path:   path,
				client: fakeClient{path: {objects: tc.iterator}},
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
			err := b.Artifacts(tc.ctx, ch)
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
		name      string
		ctx       context.Context
		path      Path
		artifacts map[string]string
		expected  []SuitesMeta
		err       bool
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
				"/something/junit.xml": `this is invalid json`,
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
			fc := fakeClient{}
			b := Build{
				Path:   tc.path,
				client: fc,
			}
			for s, data := range tc.artifacts {
				fc[resolveOrDie(b.Path, s)] = fakeObject{data: data}
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

			err := b.Suites(tc.ctx, arts, suites)
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
