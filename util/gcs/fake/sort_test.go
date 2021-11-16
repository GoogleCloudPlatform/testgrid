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

package fake // Needs to be in fake package to access the fakes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/googleapi"
)

func mustPath(s string) *gcs.Path {
	p, err := gcs.NewPath(s)
	if err != nil {
		panic(err)
	}
	return p
}

func TestStatExisting(t *testing.T) {
	cases := []struct {
		name  string
		stats Stater
		paths []gcs.Path

		want []*storage.ObjectAttrs
	}{
		{
			name: "basic",
			want: []*storage.ObjectAttrs{},
		},
		{
			name: "err",
			stats: Stater{
				*mustPath("gs://bucket/path/to/boom"): Stat{
					Err: errors.New("boom"),
				},
			},
			paths: []gcs.Path{
				*mustPath("gs://bucket/path/to/boom"),
			},
			want: []*storage.ObjectAttrs{nil},
		},
		{
			name: "not found",
			stats: Stater{
				*mustPath("gs://bucket/path/to/boom"): Stat{
					Err: storage.ErrObjectNotExist,
				},
				*mustPath("gs://bucket/path/to/wrapped"): Stat{
					Err: fmt.Errorf("wrap: %w", storage.ErrObjectNotExist),
				},
			},
			paths: []gcs.Path{
				*mustPath("gs://bucket/path/to/boom"),
				*mustPath("gs://bucket/path/to/wrapped"),
			},
			want: []*storage.ObjectAttrs{{}, {}},
		},
		{
			name: "found",
			stats: Stater{
				*mustPath("gs://bucket/path/to/boom"): Stat{
					Attrs: storage.ObjectAttrs{
						Name: "yo",
					},
				},
			},
			paths: []gcs.Path{
				*mustPath("gs://bucket/path/to/boom"),
			},
			want: []*storage.ObjectAttrs{
				{
					Name: "yo",
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &ConditionalClient{
				UploadClient: UploadClient{
					Stater: tc.stats,
				},
			}
			got := gcs.StatExisting(context.Background(), logrus.WithField("name", tc.name), client, tc.paths...)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("gridAttrs() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLeastRecentlyUpdated(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name      string
		ctx       context.Context
		client    Stater
		paths     []gcs.Path
		wantPaths []gcs.Path
		wantGens  map[gcs.Path]int64
	}{
		{
			name: "already sorted",
			client: Stater{
				*mustPath("gs://bucket/first"): {
					Attrs: storage.ObjectAttrs{
						Generation: 101,
					},
				},
				*mustPath("gs://bucket/second"): {
					Attrs: storage.ObjectAttrs{
						Generation: 102,
						Updated:    now.Add(time.Minute),
					},
				},
				*mustPath("gs://bucket/third"): {
					Attrs: storage.ObjectAttrs{
						Generation: 103,
						Updated:    now.Add(2 * time.Minute),
					},
				},
			},
			paths: []gcs.Path{
				*mustPath("gs://bucket/first"),
				*mustPath("gs://bucket/second"),
				*mustPath("gs://bucket/third"),
			},
			wantPaths: []gcs.Path{
				*mustPath("gs://bucket/first"),
				*mustPath("gs://bucket/second"),
				*mustPath("gs://bucket/third"),
			},
			wantGens: map[gcs.Path]int64{
				*mustPath("gs://bucket/first"):  101,
				*mustPath("gs://bucket/second"): 102,
				*mustPath("gs://bucket/third"):  103,
			},
		},
		{
			name: "unsorted",
			client: Stater{
				*mustPath("gs://bucket/first"): {
					Attrs: storage.ObjectAttrs{
						Generation: 101,
					},
				},
				*mustPath("gs://bucket/second"): {
					Attrs: storage.ObjectAttrs{
						Generation: 102,
						Updated:    now.Add(time.Minute),
					},
				},
				*mustPath("gs://bucket/third"): {
					Attrs: storage.ObjectAttrs{
						Generation: 103,
						Updated:    now.Add(2 * time.Minute),
					},
				},
			},
			paths: []gcs.Path{
				*mustPath("gs://bucket/third"),
				*mustPath("gs://bucket/first"),
				*mustPath("gs://bucket/second"),
			},
			wantPaths: []gcs.Path{
				*mustPath("gs://bucket/first"),
				*mustPath("gs://bucket/second"),
				*mustPath("gs://bucket/third"),
			},
			wantGens: map[gcs.Path]int64{
				*mustPath("gs://bucket/first"):  101,
				*mustPath("gs://bucket/second"): 102,
				*mustPath("gs://bucket/third"):  103,
			},
		},
		{
			name: "missing",
			client: Stater{
				*mustPath("gs://bucket/first"): {
					Attrs: storage.ObjectAttrs{
						Generation: 101,
					},
				},
				*mustPath("gs://bucket/third"): {
					Attrs: storage.ObjectAttrs{
						Generation: 103,
						Updated:    now.Add(2 * time.Minute),
					},
				},
			},
			paths: []gcs.Path{
				*mustPath("gs://bucket/third"),
				*mustPath("gs://bucket/first"),
				*mustPath("gs://bucket/missing"),
			},
			wantPaths: []gcs.Path{
				*mustPath("gs://bucket/missing"),
				*mustPath("gs://bucket/first"),
				*mustPath("gs://bucket/third"),
			},
			wantGens: map[gcs.Path]int64{
				*mustPath("gs://bucket/first"):   101,
				*mustPath("gs://bucket/third"):   103,
				*mustPath("gs://bucket/missing"): 0,
			},
		},
		{
			name: "error",
			client: Stater{
				*mustPath("gs://bucket/first"): {
					Attrs: storage.ObjectAttrs{
						Generation: 101,
					},
				},
				*mustPath("gs://bucket/third"): {
					Attrs: storage.ObjectAttrs{
						Generation: 103,
						Updated:    now.Add(2 * time.Minute),
					},
				},
				*mustPath("gs://bucket/error"): {
					Err: errors.New("injected error"),
				},
			},
			paths: []gcs.Path{
				*mustPath("gs://bucket/third"),
				*mustPath("gs://bucket/first"),
				*mustPath("gs://bucket/error"),
			},
			wantPaths: []gcs.Path{
				*mustPath("gs://bucket/error"),
				*mustPath("gs://bucket/first"),
				*mustPath("gs://bucket/third"),
			},
			wantGens: map[gcs.Path]int64{
				*mustPath("gs://bucket/first"): 101,
				*mustPath("gs://bucket/third"): 103,
				*mustPath("gs://bucket/error"): -1,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tc.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			got := gcs.LeastRecentlyUpdated(ctx, logrus.WithField("name", tc.name), tc.client, tc.paths)
			if diff := cmp.Diff(tc.wantGens, got, cmp.AllowUnexported(gcs.Path{})); diff != "" {
				t.Errorf("unexpected generation diff (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantPaths, tc.paths, cmp.AllowUnexported(gcs.Path{})); diff != "" {
				t.Errorf("unexpected path diffs (-want +got):\n%s", diff)
			}
		})
	}
}

func TestTouch(t *testing.T) {
	path := mustPath("gs://bucket/obj")

	cases := []struct {
		name       string
		ctx        context.Context
		client     *ConditionalClient
		generation int64
		buf        []byte
		expected   Uploader
		badCond    bool
		err        bool
	}{
		{
			name: "canceled context",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			generation: 123,
			client: &ConditionalClient{
				UploadClient: UploadClient{
					Uploader: Uploader{
						*path: {
							Buf:        []byte("hi"),
							Generation: 123,
						},
					},
					Stater: Stater{
						*path: Stat{
							Attrs: storage.ObjectAttrs{
								Generation: 123,
							},
						},
					},
				},
			},
			err: true,
		},
		{
			name:       "same gen",
			generation: 123,
			client: &ConditionalClient{
				UploadClient: UploadClient{
					Uploader: Uploader{
						*path: {
							Buf:        []byte("hi"),
							Generation: 123,
						},
					},
					Stater: Stater{
						*path: Stat{
							Attrs: storage.ObjectAttrs{
								Generation: 123,
							},
						},
					},
				},
			},
			expected: Uploader{
				*path: {
					Buf:        []byte("hi"),
					Generation: 124,
				},
			},
		},
		{
			name:       "wrong read gen",
			generation: 123,
			client: &ConditionalClient{
				UploadClient: UploadClient{
					Uploader: Uploader{
						*path: {
							Err: errors.New("should not get here"),
						},
					},
					Stater: Stater{
						*path: Stat{
							Attrs: storage.ObjectAttrs{
								Generation: 777,
							},
						},
					},
				},
			},
			badCond: true,
			err:     true,
		},
		{
			name:       "fail copy",
			generation: 123,
			client: &ConditionalClient{
				UploadClient: UploadClient{
					Uploader: Uploader{
						*path: {
							Err: errors.New("upload should fail"),
						},
					},
					Stater: Stater{
						*path: Stat{
							Attrs: storage.ObjectAttrs{
								Generation: 123,
							},
						},
					},
				},
			},
			err: true,
		},
		{
			name: "upload",
			client: &ConditionalClient{
				UploadClient: UploadClient{
					Uploader: Uploader{},
					Stater:   Stater{},
				},
			},
			buf: []byte("hello"),
			expected: Uploader{
				*path: {
					Buf:          []byte("hello"),
					Generation:   1,
					CacheControl: "no-cache",
				},
			},
		},
		{
			name: "fail upload",
			client: &ConditionalClient{
				UploadClient: UploadClient{
					Uploader: Uploader{
						*path: {
							Err: errors.New("upload should fail"),
						},
					},
					Stater: Stater{},
				},
			},
			err: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tc.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			_, err := gcs.Touch(ctx, tc.client, *path, tc.generation, tc.buf)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("got unexpected error: %v", err)
				}
				if tc.badCond {
					var ee *googleapi.Error
					if !errors.As(err, &ee) {
						t.Errorf("wanted googleapi.Error, got %v", err)
					}
					if got, want := ee.Code, http.StatusPreconditionFailed; want != got {
						t.Errorf("wanted %v got %v", want, got)
					}
				}
			case tc.err:
				t.Error("failed to receive an error")
			default:
				if diff := cmp.Diff(tc.expected, tc.client.Uploader); diff != "" {
					t.Errorf("got unexpected diff (-want +got):\n%s", diff)
				}
			}
		})
	}
}
