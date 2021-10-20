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

package summarizer

import (
	"testing"

	"github.com/GoogleCloudPlatform/testgrid/pkg/pubsub"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/google/go-cmp/cmp"
)

func TestProcessNotification(t *testing.T) {
	mustPath := func(s string) gcs.Path {
		p, err := gcs.NewPath(s)
		if err != nil {
			t.Fatalf("gcs.NewPath(%q): %v", s, err)
		}
		return *p
	}
	pstr := func(s string) *string { return &s }
	cases := []struct {
		name   string
		prefix gcs.Path
		notice *pubsub.Notification
		want   *string
	}{
		{
			name:   "basically works",
			prefix: mustPath("gs://bucket/prefix/"),
			notice: &pubsub.Notification{},
		},
		{
			name:   "wrong bucket",
			prefix: mustPath("gs://bucket/prefix/"),
			notice: &pubsub.Notification{
				Path: mustPath("gs://elsewhere/prefix/foo"),
			},
		},
		{
			name:   "wrong prefix",
			prefix: mustPath("gs://bucket/prefix/"),
			notice: &pubsub.Notification{
				Path: mustPath("gs://bucket/foo"),
			},
		},
		{
			name:   "require trailing slash",
			prefix: mustPath("gs://bucket/prefix"), // missing /
			notice: &pubsub.Notification{
				Path: mustPath("gs://bucket/prefix/foo"),
			},
		},
		{
			name:   "match",
			prefix: mustPath("gs://bucket/prefix/"),
			notice: &pubsub.Notification{
				Path: mustPath("gs://bucket/prefix/foo"),
			},
			want: pstr("foo"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := processNotification(tc.prefix, tc.notice)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("processNotification() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}
