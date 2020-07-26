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

	"github.com/GoogleCloudPlatform/testgrid/metadata/junit"
)

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
}

func (fi *fakeIterator) Next() (*storage.ObjectAttrs, error) {
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
	objects func() Iterator
	open    error
	read    error
}

func subdir(prefix string) storage.ObjectAttrs {
	return storage.ObjectAttrs{Prefix: prefix}
}

func link(name, other string) storage.ObjectAttrs {
	return storage.ObjectAttrs{
		Metadata: map[string]string{"x-goog-meta-link": other},
		Name:     name,
	}
}

type fakeInterrogator map[Path]fakeObject

func (fi fakeInterrogator) Objects(_ context.Context, path Path, _ string) Iterator {
	f := fi[path].objects
	if f == nil {
		return &fakeIterator{}
	}
	return f()
}

func (fi fakeInterrogator) Open(ctx context.Context, path Path) (io.ReadCloser, error) {
	o, ok := fi[path]
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

// TODO(fejta): TestStarted
func TestStarted(t *testing.T) {
	cases := []struct {
		name string
	}{
		{},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

		})
	}
}

// TODO(fejta): TestFinished
func TestFinished(t *testing.T) {
	cases := []struct {
		name string
	}{
		{},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

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
		name         string
		ctx          context.Context
		interrogator fakeInterrogator
		expected     *junit.Suites
		checkErr     error
	}{
		{
			name: "basically works",
			interrogator: fakeInterrogator{
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
			interrogator: fakeInterrogator{
				path: {data: `<wrong><type></type></wrong>`},
			},
		},
		{
			name: "read error returns error",
			interrogator: fakeInterrogator{
				path: {
					read: errors.New("injected read error"),
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := readSuites(tc.ctx, tc.interrogator, path)
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
		iterator func() Iterator
		expected []string
		err      bool
	}{
		{
			name: "basically works",
		},
		{
			name: "cancelled context returns error",
			iterator: func() Iterator {
				return &fakeIterator{
					objects: []storage.ObjectAttrs{
						{Name: "whatever"},
						{Name: "stuff"},
					},
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
			iterator: func() Iterator {
				return &fakeIterator{
					objects: []storage.ObjectAttrs{
						{Name: "hello"},
						{Name: "boom"},
						{Name: "world"},
					},
					err: 1,
				}
			},
			err: true,
		},
		{
			name: "multiple objects work",
			iterator: func() Iterator {
				return &fakeIterator{
					objects: []storage.ObjectAttrs{
						{Name: "hello"},
						{Name: "world"},
					},
				}
			},
			expected: []string{"hello", "world"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := Build{
				Path:         path,
				interrogator: fakeInterrogator{path: {objects: tc.iterator}},
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
			fi := fakeInterrogator{}
			b := Build{
				Path:         tc.path,
				interrogator: fi,
			}
			for s, data := range tc.artifacts {
				fi[resolveOrDie(b.Path, s)] = fakeObject{data: data}
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
