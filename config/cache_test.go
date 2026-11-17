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

package config

import (
	"context"
	"errors"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs/fake"
)

func Test_ReadGCS(t *testing.T) {
	expectedConfig := &configpb.Configuration{
		Dashboards: []*configpb.Dashboard{
			{
				Name: "Example",
			},
		},
	}
	expectedBytes, err := proto.Marshal(expectedConfig)
	if err != nil {
		t.Fatalf("Can't marshal expectations: %v", err)
	}

	now := time.Now()

	cases := []struct {
		name         string
		currentCache map[string]Config
		remoteData   []byte
		remoteAttrs  *storage.ReaderObjectAttrs
		wantErr      error
	}{
		{
			name:         "read on fresh cache",
			currentCache: map[string]Config{},
			remoteData:   expectedBytes,
			remoteAttrs: &storage.ReaderObjectAttrs{
				LastModified: now,
				Generation:   1,
			},
		},
		{
			name: "don't read if too recent",
			currentCache: map[string]Config{
				"gs://example": {
					proto: expectedConfig,
					attrs: &storage.ReaderObjectAttrs{
						LastModified: now,
						Generation:   1,
					},
					lastFetch: now,
				},
			},
			remoteData: []byte{1, 2, 3},
			remoteAttrs: &storage.ReaderObjectAttrs{
				LastModified: now,
				Generation:   1,
			},
		},
		{
			name: "read on stale cache",
			currentCache: map[string]Config{
				"gs://example": {
					proto: &configpb.Configuration{
						Dashboards: []*configpb.Dashboard{
							{
								Name: "stale and bad",
							},
						},
					},
					lastFetch: now.Add(-1 * time.Hour),
				},
			},
			remoteData: expectedBytes,
			remoteAttrs: &storage.ReaderObjectAttrs{
				LastModified: now,
				Generation:   1,
			},
		},
		{
			name:         "read with nil attrs",
			currentCache: map[string]Config{},
			remoteData:   expectedBytes,
			remoteAttrs:  nil,
		},
		{
			name:         "read at max config size",
			currentCache: map[string]Config{},
			remoteData:   expectedBytes,
			remoteAttrs: &storage.ReaderObjectAttrs{
				LastModified: now,
				Generation:   1,
				Size:         int64(MaxConfigSize),
			},
		},
		{
			name:         "error when attrs size exceeds max config size",
			currentCache: map[string]Config{},
			remoteData:   expectedBytes,
			remoteAttrs: &storage.ReaderObjectAttrs{
				LastModified: now,
				Generation:   1,
				Size:         int64(MaxConfigSize) + 1,
			},
			wantErr: ConfigTooLargeError{},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			cache = test.currentCache
			client := fake.Client{
				Opener: fake.Opener{
					Paths: map[gcs.Path]fake.Object{},
				},
			}

			client.Opener.Paths[mustPath("gs://example")] = fake.Object{
				Data:  string(test.remoteData),
				Attrs: test.remoteAttrs,
			}
			result, attrs, err := ReadGCS(context.Background(), &client, mustPath("gs://example"))
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Errorf("ReadGCS() error = %v, want %v", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Unexpected error %v", err)
			}
			if diff := cmp.Diff(test.remoteAttrs, attrs, protocmp.Transform()); diff != "" {
				t.Errorf("Attributes should be returned unchanged (-want, +got): %s", diff)
			}
			if diff := cmp.Diff(expectedConfig, result, protocmp.Transform()); diff != "" {
				t.Errorf("Config differed (-want, +got): %s ", diff)
			}
		})
	}
}

func mustPath(s string) gcs.Path {
	p, err := gcs.NewPath(s)
	if err != nil {
		panic(err)
	}
	return *p
}
