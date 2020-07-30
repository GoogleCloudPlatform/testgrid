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
	"testing"

	"github.com/GoogleCloudPlatform/testgrid/metadata/junit"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

func TestDownloadGrid(t *testing.T) {
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

func TestReadColumns(t *testing.T) {
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

func TestReadResult(t *testing.T) {
	path := newPathOrDie("gs://bucket/path/to/some/build/")
	cases := []struct {
		name     string
		ctx      context.Context
		data     map[string]fakeObject
		expected *gcsResult
	}{
		{
			name: "basically works",
			expected: &gcsResult{
				started: gcs.Started{
					Pending: true,
				},
				finished: gcs.Finished{
					Running: true,
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ctx == nil {
				tc.ctx = context.Background()
			}
			ctx, cancel := context.WithCancel(tc.ctx)
			defer cancel()
			client := fakeClient{
				fakeLister: fakeLister{},
				fakeOpener: fakeOpener{},
			}

			fi := fakeIterator{}
			for name, fo := range tc.data {
				p, err := path.ResolveReference(&url.URL{Path: name})
				if err != nil {
					t.Fatal("path.ResolveReference(%q): %w", name, err)
				}
				fi.objects = append(fi.objects, storage.ObjectAttrs{
					Name: p.Object(),
				})
				client.fakeOpener[*p] = fo
			}
			client.fakeLister[path] = fi

			build := gcs.Build{
				Path: path,
			}
			actual, err := readResult(ctx, client, build)
			switch {
			case err != nil:
				if tc.expected != nil {
					t.Errorf("readResult(): unexpected error: %v", err)
				}
			case tc.expected == nil:
				t.Error("readResult(): failed to receive expected error")
			case !reflect.DeepEqual(actual, tc.expected):
				t.Errorf("readResult():\ngot %+v,\nwant %+v", actual, tc.expected)
			}
		})
	}
}

func newPathOrDie(s string) gcs.Path {
	p, err := gcs.NewPath(s)
	if err != nil {
		panic(err)
	}
	return *p
}

func TestReadSuites(t *testing.T) {
	path := newPathOrDie("gs://bucket/path/to/build/")
	cases := []struct {
		name       string
		data       map[string]fakeObject
		listIdxErr int
		expected   []gcs.SuitesMeta
		err        bool
		ctx        context.Context
	}{
		{
			name: "basically works",
		},
		{
			name: "multiple suites from multiple artifacts work",
			data: map[string]fakeObject{
				"ignore-this": {data: "<invalid></xml>"},
				"junit.xml":   {data: `<testsuite><testcase name="hi"/></testsuite>`},
				"ignore-that": {data: "<invalid></xml>"},
				"nested/junit_context_20201122-1234_88.xml": {
					data: `
                        <testsuites>
                            <testsuite name="fun">
                                <testsuite name="knee">
                                    <testcase name="bone" time="6" />
                                </testsuite>
                                <testcase name="word" time="7" />
                            </testsuite>
                        </testsuites>
                    `,
				},
			},
			expected: []gcs.SuitesMeta{
				{
					Suites: junit.Suites{
						Suites: []junit.Suite{
							{
								XMLName: xml.Name{Local: "testsuite"},
								Results: []junit.Result{
									{Name: "hi"},
								},
							},
						},
					},
					Metadata: map[string]string{
						"Context":   "",
						"Thread":    "",
						"Timestamp": "",
					},
					Path: "gs://bucket/path/to/build/junit.xml",
				},
				{
					Suites: junit.Suites{
						XMLName: xml.Name{Local: "testsuites"},
						Suites: []junit.Suite{
							{
								XMLName: xml.Name{Local: "testsuite"},
								Name:    "fun",
								Suites: []junit.Suite{
									{
										XMLName: xml.Name{Local: "testsuite"},
										Name:    "knee",
										Results: []junit.Result{
											{
												Name: "bone",
												Time: 6,
											},
										},
									},
								},
								Results: []junit.Result{
									{
										Name: "word",
										Time: 7,
									},
								},
							},
						},
					},
					Metadata: map[string]string{
						"Context":   "context",
						"Thread":    "88",
						"Timestamp": "20201122-1234",
					},
					Path: "gs://bucket/path/to/build/nested/junit_context_20201122-1234_88.xml",
				},
			},
		},
		{
			name: "list error returns error",
			data: map[string]fakeObject{
				"ignore-this": {data: "<invalid></xml>"},
				"junit.xml":   {data: `<testsuite><testcase name="hi"/></testsuite>`},
				"ignore-that": {data: "<invalid></xml>"},
			},
			listIdxErr: 1,
			err:        true,
		},
		{
			name: "cancelled context returns err",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			err: true,
		},
		{
			name: "suites error returns error",
			data: map[string]fakeObject{
				"junit.xml": {data: "<invalid></xml>"},
			},
			err: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ctx == nil {
				tc.ctx = context.Background()
			}
			ctx, cancel := context.WithCancel(tc.ctx)
			defer cancel()
			client := fakeClient{
				fakeLister: fakeLister{},
				fakeOpener: fakeOpener{},
			}

			fi := fakeIterator{
				err: tc.listIdxErr,
			}
			for name, fo := range tc.data {
				p, err := path.ResolveReference(&url.URL{Path: name})
				if err != nil {
					t.Fatal("path.ResolveReference(%q): %w", name, err)
				}
				fi.objects = append(fi.objects, storage.ObjectAttrs{
					Name: p.Object(),
				})
				client.fakeOpener[*p] = fo
			}
			client.fakeLister[path] = fi

			build := gcs.Build{
				Path: path,
			}
			actual, err := readSuites(ctx, &client, build)
			sort.SliceStable(actual, func(i, j int) bool {
				return actual[i].Path < actual[j].Path
			})
			sort.SliceStable(tc.expected, func(i, j int) bool {
				return tc.expected[i].Path < tc.expected[j].Path
			})
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("readSuites(): unexpected error: %v", err)
				}
			case tc.err:
				t.Error("readSuites(): failed to receive an error")
			default:
				if !reflect.DeepEqual(actual, tc.expected) {
					t.Errorf("readSuites():\nhave %+v,\nwant %+v", actual, tc.expected)
				}
			}
		})
	}
}

type fakeOpener map[gcs.Path]fakeObject

func (fo fakeOpener) Open(ctx context.Context, path gcs.Path) (io.ReadCloser, error) {
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

type fakeLister map[gcs.Path]fakeIterator

func (fl fakeLister) Objects(ctx context.Context, path gcs.Path, _ string) gcs.Iterator {
	f := fl[path]
	f.ctx = ctx
	return &f
}

type fakeIterator struct {
	objects []storage.ObjectAttrs
	idx     int
	err     int // must be > 0
	ctx     context.Context
}

type fakeClient struct {
	fakeLister
	fakeOpener
}

func (fi *fakeIterator) Next() (*storage.ObjectAttrs, error) {
	if fi.ctx.Err() != nil {
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
