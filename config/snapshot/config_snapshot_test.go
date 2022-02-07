/*
Copyright 2022 The TestGrid Authors.

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

package snapshot

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs/fake"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestObserve_OnInit(t *testing.T) {
	tests := []struct {
		name             string
		config           *configpb.Configuration
		configGeneration int64
		gcsErr           error
		expectInitial    *configpb.Dashboard
		expectError      bool
	}{
		{
			name: "Reads configs",
			config: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dashboard",
					},
				},
			},
			configGeneration: 1,
			expectInitial: &configpb.Dashboard{
				Name: "dashboard",
			},
		},
		{
			name:        "Returns error if config isn't present at startup",
			gcsErr:      errors.New("file missing"),
			expectError: true,
		},
	}

	path, err := gcs.NewPath("gs://config/example")
	if err != nil {
		t.Fatal("could not path")
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			client := fakeClient()
			client.Opener[*path] = fake.Object{
				Data: string(mustMarshalConfig(test.config)),
				Attrs: &storage.ReaderObjectAttrs{
					Generation: test.configGeneration,
				},
				ReadErr: test.gcsErr,
			}
			client.Stater[*path] = fake.Stat{
				Attrs: storage.ObjectAttrs{
					Generation: test.configGeneration,
				},
				Err: test.gcsErr,
			}

			snaps, err := Observe(ctx, nil, client, *path, nil)

			if err != nil {
				if !test.expectError {
					t.Errorf("got unexpected error: %v", err)
				}
				return
			}

			select {
			case cs := <-snaps:
				if result := cs.Dashboards["dashboard"]; !proto.Equal(result, test.expectInitial) {
					t.Errorf("got dashboard %v, expected %v", result, test.expectInitial)
				}
			case <-time.After(5 * time.Second):
				t.Error("expected an initial snapshot, but got none")
			}
		})
	}
}

func TestObserve_OnTick(t *testing.T) {
	tests := []struct {
		name             string
		config           *configpb.Configuration
		configGeneration int64
		gcsErr           error
		expectDashboard  *configpb.Dashboard
	}{
		{
			name: "Reads new configs",
			config: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dashboard",
					},
				},
			},
			configGeneration: 2,
			expectDashboard: &configpb.Dashboard{
				Name: "dashboard",
			},
		},
		{
			name: "Does not snapshot if generation match",
			config: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dashboard",
					},
				},
			},
			configGeneration: 1,
		},
		{
			name:             "Handles read error",
			configGeneration: 2,
			gcsErr:           errors.New("reading fails after init"),
		},
	}

	path, err := gcs.NewPath("gs://config/example")
	if err != nil {
		t.Fatal("could not path")
	}

	initialConfig := &configpb.Configuration{
		Dashboards: []*configpb.Dashboard{
			{
				Name: "old-dashboard",
			},
		},
	}

	now := time.Now()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			client := fakeClient()
			client.Opener[*path] = fake.Object{
				Data: string(mustMarshalConfig(initialConfig)),
				Attrs: &storage.ReaderObjectAttrs{
					Generation: 1,
				},
			}
			client.Stater[*path] = fake.Stat{
				Attrs: storage.ObjectAttrs{
					Generation: 1,
				},
			}

			ticker := make(chan time.Time)
			defer close(ticker)
			snaps, err := Observe(ctx, nil, client, *path, ticker)
			if err != nil {
				t.Fatalf("error in initial observe: %v", err)
			}
			<-snaps

			// Change the config
			client.Opener[*path] = fake.Object{
				Data: string(mustMarshalConfig(test.config)),
				Attrs: &storage.ReaderObjectAttrs{
					Generation: test.configGeneration,
				},
				ReadErr: test.gcsErr,
			}
			client.Stater[*path] = fake.Stat{
				Attrs: storage.ObjectAttrs{
					Generation: test.configGeneration,
				},
				Err: test.gcsErr,
			}

			ticker <- now

			if test.expectDashboard != nil {
				select {
				case cs := <-snaps:
					if result := cs.Dashboards["dashboard"]; !proto.Equal(result, test.expectDashboard) {
						t.Errorf("got dashboard %v, expected %v", result, test.expectDashboard)
					}
				case <-time.After(5 * time.Hour):
					t.Error("expected a snapshot after tick, but got none")
				}
			} else {
				select {
				case cs := <-snaps:
					t.Errorf("did not expect a snapshot, but got %v", cs)
				default:
				}
			}
		})
	}
}

func TestObserve_Data(t *testing.T) {
	tests := []struct {
		name     string
		config   *configpb.Configuration
		expected *Config
	}{
		{
			name: "Empty config",
			expected: &Config{
				Dashboards: map[string]*configpb.Dashboard{},
				Groups:     map[string]*configpb.TestGroup{},
				Attrs: storage.ReaderObjectAttrs{
					Generation: 1,
				},
			},
		},
		{
			name: "Dashboards and TestGroups",
			config: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name:       "chess",
						DefaultTab: "Ke5",
					},
					{
						Name:       "checkers",
						DefaultTab: "10-15",
					},
				},
				TestGroups: []*configpb.TestGroup{
					{
						Name:          "king",
						DaysOfResults: 17,
					},
					{
						Name:          "pawn",
						DaysOfResults: 1,
					},
				},
			},
			expected: &Config{
				Dashboards: map[string]*configpb.Dashboard{
					"chess": {
						Name:       "chess",
						DefaultTab: "Ke5",
					},
					"checkers": {
						Name:       "checkers",
						DefaultTab: "10-15",
					},
				},
				Groups: map[string]*configpb.TestGroup{
					"king": {
						Name:          "king",
						DaysOfResults: 17,
					},
					"pawn": {
						Name:          "pawn",
						DaysOfResults: 1,
					},
				},
				Attrs: storage.ReaderObjectAttrs{
					Generation: 1,
				},
			},
		},
	}

	path, err := gcs.NewPath("gs://config/example")
	if err != nil {
		t.Fatal("could not path")
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			client := fakeClient()
			client.Opener[*path] = fake.Object{
				Data: string(mustMarshalConfig(test.config)),
				Attrs: &storage.ReaderObjectAttrs{
					Generation: 1,
				},
			}
			client.Stater[*path] = fake.Stat{
				Attrs: storage.ObjectAttrs{
					Generation: 1,
				},
			}

			snaps, err := Observe(ctx, nil, client, *path, nil)
			if err != nil {
				t.Fatalf("error in initial observe: %v", err)
			}

			select {
			case cs := <-snaps:
				if diff := cmp.Diff(test.expected, cs, protocmp.Transform()); diff != "" {
					t.Errorf("(-want +got): %v", diff)
				}
			case <-time.After(5 * time.Second):
				t.Error("expected an initial snapshot, but got none")
			}

		})
	}
}

func mustMarshalConfig(c *configpb.Configuration) []byte {
	b, err := proto.Marshal(c)
	if err != nil {
		panic(err)
	}
	return b
}

func fakeClient() *fake.ConditionalClient {
	return &fake.ConditionalClient{
		UploadClient: fake.UploadClient{
			Uploader: fake.Uploader{},
			Client: fake.Client{
				Lister: fake.Lister{},
				Opener: fake.Opener{},
			},
			Stater: fake.Stater{},
		},
		Lock: &sync.RWMutex{},
	}
}
