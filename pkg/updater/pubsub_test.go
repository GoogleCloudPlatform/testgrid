/*
Copyright 2021 The TestGrid Authors.

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
	"context"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/config"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/pkg/pubsub"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
)

func TestGCSSubscribedPaths(t *testing.T) {
	origManual := manualSubs
	defer func() {
		manualSubs = origManual
	}()
	mustPath := func(s string) gcs.Path {
		p, err := gcs.NewPath(s)
		if err != nil {
			t.Fatal(err)
		}
		return *p
	}

	cases := []struct {
		name   string
		tgs    []*configpb.TestGroup
		manual map[string]subscription

		want     map[gcs.Path][]string
		wantSubs []subscription
		err      bool
	}{
		{
			name: "empty",
			want: map[gcs.Path][]string{},
		},
		{
			name: "basic",
			tgs: []*configpb.TestGroup{
				{
					Name: "hello",
					ResultSource: &configpb.TestGroup_ResultSource{
						ResultSourceConfig: &configpb.TestGroup_ResultSource_GcsConfig{
							GcsConfig: &configpb.GCSConfig{
								GcsPrefix:          "bucket/path/to/job",
								PubsubProject:      "fancy",
								PubsubSubscription: "cake",
							},
						},
					},
				},
				{
					Name: "multi",
					ResultSource: &configpb.TestGroup_ResultSource{
						ResultSourceConfig: &configpb.TestGroup_ResultSource_GcsConfig{
							GcsConfig: &configpb.GCSConfig{
								GcsPrefix:          "bucket/a,bucket/b",
								PubsubProject:      "super",
								PubsubSubscription: "duper",
							},
						},
					},
				},
				{
					Name: "dup-a",
					ResultSource: &configpb.TestGroup_ResultSource{
						ResultSourceConfig: &configpb.TestGroup_ResultSource_GcsConfig{
							GcsConfig: &configpb.GCSConfig{
								GcsPrefix:          "bucket/dup",
								PubsubProject:      "ha",
								PubsubSubscription: "ha",
							},
						},
					},
				},
				{
					Name: "dup-b",
					ResultSource: &configpb.TestGroup_ResultSource{
						ResultSourceConfig: &configpb.TestGroup_ResultSource_GcsConfig{
							GcsConfig: &configpb.GCSConfig{
								GcsPrefix:          "bucket/dup/",
								PubsubProject:      "ha",
								PubsubSubscription: "ha",
							},
						},
					},
				},
			},
			want: map[gcs.Path][]string{
				mustPath("gs://bucket/path/to/job/"): {"hello"},
				mustPath("gs://bucket/a/"):           {"multi"},
				mustPath("gs://bucket/b/"):           {"multi"},
				mustPath("gs://bucket/dup/"):         {"dup-a", "dup-b"},
			},
			wantSubs: []subscription{
				{"fancy", "cake"},
				{"super", "duper"},
				{"ha", "ha"},
			},
		},
		{
			name: "manually empty",
			manual: map[string]subscription{
				"bucket/foo": {"this", "that"},
			},
			tgs: []*configpb.TestGroup{
				{
					Name:      "hello",
					GcsPrefix: "random/stuff",
				},
			},
			want: map[gcs.Path][]string{},
		},
		{
			name: "manually empty",
			manual: map[string]subscription{
				"bucket/foo": {"this", "that"},
			},
			tgs: []*configpb.TestGroup{
				{
					Name:      "hello",
					GcsPrefix: "bucket/foo/bar",
				},
			},
			want: map[gcs.Path][]string{
				mustPath("gs://bucket/foo/bar/"): {"hello"},
			},
			wantSubs: []subscription{
				{"this", "that"},
			},
		},
		{
			name: "mixed",
			manual: map[string]subscription{
				"bucket/foo": {"this", "that"},
			},
			tgs: []*configpb.TestGroup{
				{
					Name:      "hello",
					GcsPrefix: "bucket/foo/bar",
				},
				{
					Name: "world",
					ResultSource: &configpb.TestGroup_ResultSource{
						ResultSourceConfig: &configpb.TestGroup_ResultSource_GcsConfig{
							GcsConfig: &configpb.GCSConfig{
								GcsPrefix:          "bucket/path/to/job",
								PubsubProject:      "fancy",
								PubsubSubscription: "cake",
							},
						},
					},
				},
			},
			want: map[gcs.Path][]string{
				mustPath("gs://bucket/foo/bar/"):     {"hello"},
				mustPath("gs://bucket/path/to/job/"): {"world"},
			},
			wantSubs: []subscription{
				{"this", "that"},
				{"fancy", "cake"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manualSubs = tc.manual
			got, gotSubs, err := gcsSubscribedPaths(tc.tgs)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("gcsSubscribedPaths() got unexpected error: %v", err)
				}
			case tc.err:
				t.Error("gcsSubscribedPaths() failed to return an error")
			default:
				if diff := cmp.Diff(tc.want, got, cmp.AllowUnexported(gcs.Path{})); diff != "" {
					t.Errorf("gcsSubscribedPaths() got unexpected diff (-want +got):\n%s", diff)
				}
				if diff := cmp.Diff(tc.wantSubs, gotSubs, cmp.AllowUnexported(subscription{})); diff != "" {
					t.Errorf("gcsSubscribedPaths() got unexpected subscription diff (-want +got):\n%s", diff)
				}
			}

		})
	}
}

func TestProcessGCSNotifications(t *testing.T) {
	mustPath := func(s string) gcs.Path {
		p, err := gcs.NewPath(s)
		if err != nil {
			t.Fatal(err)
		}
		return *p
	}
	now := time.Now()
	cases := []struct {
		name     string
		ctx      context.Context
		q        *config.TestGroupQueue
		paths    map[gcs.Path][]string
		notices  []*pubsub.Notification
		err      bool
		want     string
		wantWhen time.Time
	}{
		{
			name: "empty",
			q:    &config.TestGroupQueue{},
		},
		{
			name: "basic",
			q: func() *config.TestGroupQueue {
				var q config.TestGroupQueue
				q.Init([]*configpb.TestGroup{
					{
						Name: "hello",
					},
					{
						Name: "boom",
					},
					{
						Name: "world",
					},
				}, now.Add(time.Hour))
				if err := q.Fix("world", now.Add(30*time.Minute), false); err != nil {
					t.Fatalf("Fixing got unexpected error: %v", err)
				}
				return &q
			}(),
			paths: map[gcs.Path][]string{
				mustPath("gs://foo/boom"): {"boom"},
			},
			notices: []*pubsub.Notification{
				{
					Path: mustPath("gs://foo/boom/build/finished.json"),
					Time: now,
				},
			},
			want:     "boom",
			wantWhen: now.Add(namedDurations["finished.json"]),
		},
		{
			name: "multi",
			q: func() *config.TestGroupQueue {
				var q config.TestGroupQueue
				q.Init([]*configpb.TestGroup{
					{
						Name: "hello",
					},
					{
						Name: "boom",
					},
					{
						Name: "world",
					},
				}, now.Add(time.Hour))
				if err := q.Fix("world", now.Add(30*time.Minute), false); err != nil {
					t.Fatalf("Fixing got unexpected error: %v", err)
				}
				return &q
			}(),
			paths: map[gcs.Path][]string{
				mustPath("gs://foo/multi"): {"world", "boom"},
			},
			notices: []*pubsub.Notification{
				{
					Path: mustPath("gs://foo/multi/build/finished.json"),
					Time: now,
				},
			},
			want:     "world",
			wantWhen: now.Add(namedDurations["finished.json"]),
		},
		{
			name: "unchanged",
			q: func() *config.TestGroupQueue {
				var q config.TestGroupQueue
				q.Init([]*configpb.TestGroup{
					{
						Name: "hello",
					},
					{
						Name: "world",
					},
					{
						Name: "boom",
					},
				}, now.Add(time.Hour))
				if err := q.Fix("world", now.Add(30*time.Minute), false); err != nil {
					t.Fatalf("Fixing got unexpected error: %v", err)
				}
				return &q
			}(),
			paths: map[gcs.Path][]string{
				mustPath("gs://foo/boom"): {"boom"},
			},
			notices: []*pubsub.Notification{
				{
					Path: mustPath("gs://random/stuff"),
					Time: now,
				},
			},
			want:     "world",
			wantWhen: now.Add(30 * time.Minute),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ctx == nil {
				tc.ctx = context.Background()
			}
			ctx, cancel := context.WithCancel(tc.ctx)
			defer cancel()
			ch := make(chan *pubsub.Notification)
			go func() {
				for _, notice := range tc.notices {
					select {
					case <-ctx.Done():
						return
					case ch <- notice:
					}
				}
				cancel()
			}()

			err := processGCSNotifications(ctx, logrus.WithField("name", tc.name), tc.q, tc.paths, ch)
			switch {
			case err != nil && err != context.Canceled:
				if !tc.err {
					t.Errorf("processGCSNotifications() got unexpected err: %v", err)
				}
			case tc.err:
				t.Error("processGCSNotifications() failed to return an error")
			default:
				_, who, when := tc.q.Status()
				var got string
				if who != nil {
					got = who.Name
				}
				if diff := cmp.Diff(tc.want, got); diff != "" {
					t.Errorf("processGCSNotifications got unexpected diff (-want +got):\n%s", diff)
				}
				if diff := cmp.Diff(tc.wantWhen, when); diff != "" {
					t.Errorf("processGCSNotifications got unexpected when diff (-want +got):\n%s", diff)
				}

			}
		})
	}
}

func TestProcessNotification(t *testing.T) {
	mustPath := func(s string) gcs.Path {
		p, err := gcs.NewPath(s)
		if err != nil {
			t.Fatal(err)
		}
		return *p
	}
	type testcase struct {
		name    string
		paths   map[gcs.Path][]string
		n       *pubsub.Notification
		want    []string
		wantDur time.Duration
	}
	cases := []testcase{
		{
			name: "empty",
			n:    &pubsub.Notification{},
		},
		{
			name: "irrelevant path",
			paths: map[gcs.Path][]string{
				mustPath("gs://foo/bar"): {"hello", "world"},
			},
			n: &pubsub.Notification{
				Path: mustPath("gs://random/job/build/finished.json"),
			},
		},
		{
			name: "irrelevant basename",
			paths: map[gcs.Path][]string{
				mustPath("gs://foo/bar"): {"hello", "world"},
			},
			n: &pubsub.Notification{
				Path: mustPath("gs://foo/bar/artifacts/smile.jpeg"),
			},
		},
		{
			name: "simple junit",
			paths: map[gcs.Path][]string{
				mustPath("gs://foo/bar"): {"hello", "world"},
				mustPath("gs://not/me"):  {"nope", "world"},
				mustPath("gs://foo/"):    {"yes", "me"},
			},
			n: &pubsub.Notification{
				Path: mustPath("gs://foo/bar/artifacts/junit.xml"),
			},
			want:    []string{"hello", "me", "world", "yes"},
			wantDur: 5 * time.Minute,
		},
		{
			name: "complex junit",
			paths: map[gcs.Path][]string{
				mustPath("gs://foo/bar"): {"hello", "world"},
				mustPath("gs://not/me"):  {"nope", "world"},
				mustPath("gs://foo/"):    {"yes", "me"},
			},
			n: &pubsub.Notification{
				Path: mustPath("gs://foo/bar/artifacts/junit_debian-23094820.xml"),
			},
			want:    []string{"hello", "me", "world", "yes"},
			wantDur: 5 * time.Minute,
		},
	}

	for name, dur := range namedDurations {
		cases = append(cases, testcase{
			name: name,
			paths: map[gcs.Path][]string{
				mustPath("gs://foo/bar"): {"hello", "world"},
				mustPath("gs://not/me"):  {"nope", "world"},
				mustPath("gs://foo/"):    {"yes", "me"},
			},
			n: &pubsub.Notification{
				Path: mustPath("gs://foo/bar/" + name),
			},
			want:    []string{"hello", "me", "world", "yes"},
			wantDur: dur,
		})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, gotDur := processNotification(tc.paths, tc.n)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("processNotification() got unexpected diff:\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantDur, gotDur); diff != "" {
				t.Errorf("processNotification() got unexpected duration diff:\n%s", diff)
			}
		})
	}
}
