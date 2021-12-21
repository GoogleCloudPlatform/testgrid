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
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/golang/protobuf/proto"

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
		name               string
		currentCache       map[string]Config
		remoteData         []byte
		remoteLastModified time.Time
		remoteGeneration   int64
	}{
		{
			name:               "read on fresh cache",
			currentCache:       map[string]Config{},
			remoteData:         expectedBytes,
			remoteLastModified: now,
			remoteGeneration:   1,
		},
		{
			name: "don't read if too recent",
			currentCache: map[string]Config{
				"gs://example": {
					proto:     expectedConfig,
					lastFetch: now,
				},
			},
			remoteData:         []byte{1, 2, 3},
			remoteLastModified: now,
			remoteGeneration:   1,
		},
		{
			name: "read on stale cache",
			currentCache: map[string]Config{
				"gs://example": {
					proto: &configpb.Configuration{
						Dashboards: []*configpb.Dashboard{
							{
								Name: "stale",
							},
						},
					},
					lastFetch: now.Add(-1 * time.Hour),
				},
			},
			remoteData:         expectedBytes,
			remoteLastModified: now,
			remoteGeneration:   1,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			cache = test.currentCache
			client := fake.Client{
				Opener: fake.Opener{},
			}

			client.Opener[mustPath("gs://example")] = fake.Object{
				Data: string(test.remoteData),
				Attrs: &storage.ReaderObjectAttrs{
					LastModified: test.remoteLastModified,
					Generation:   test.remoteGeneration,
				},
			}
			result, err := ReadGCS(context.Background(), &client, mustPath("gs://example"))
			if err != nil {
				t.Errorf("Unexpected error %v", err)
			}
			if !proto.Equal(expectedConfig, result) {
				t.Errorf("Expected %v, got %v", expectedConfig, result)
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
