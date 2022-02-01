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

	"cloud.google.com/go/storage"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs/fake"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

func TestObserve(t *testing.T) {
	tests := []struct {
		name             string
		config           *configpb.Configuration
		configGeneration int64
		readErr          error
		expectDashboard  *configpb.Dashboard
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
			expectDashboard: &configpb.Dashboard{
				Name: "dashboard",
			},
		},
		{
			name:        "Returns error if config isn't present at startup",
			readErr:     errors.New("can't read in this test"),
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

			client := &fake.ConditionalClient{
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

			client.Opener[*path] = fake.Object{
				Data: string(mustMarshalConfig(test.config)),
				Attrs: &storage.ReaderObjectAttrs{
					Generation: test.configGeneration,
				},
				ReadErr: test.readErr,
			}
			client.Stater[*path] = fake.Stat{
				Attrs: storage.ObjectAttrs{
					Generation: test.configGeneration,
				},
			}

			onUpdate := func(*Config) error {
				return nil
			}

			cs, err := Observe(ctx, logrus.StandardLogger(), client, *path, onUpdate)

			if (err != nil) != test.expectError {
				t.Errorf("Error: got %v, expected? %t", err, test.expectError)
			}

			if result := cs.Dashboard("dashboard"); !proto.Equal(result, test.expectDashboard) {
				t.Errorf("got dashboard %v, expected %v", result, test.expectDashboard)
			}
		})
	}
}

func TestTick(t *testing.T) {
	tests := []struct {
		name             string
		cs               *Config
		config           *configpb.Configuration
		configGeneration int64
		readErr          error
		expectDashboard  *configpb.Dashboard
		expectUpdate     bool
		expectError      bool
	}{
		{
			name: "Reads configs",
			cs:   &Config{},
			config: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "dashboard",
					},
				},
			},
			configGeneration: 1,
			expectDashboard: &configpb.Dashboard{
				Name: "dashboard",
			},
			expectUpdate: true,
		},
		{
			name: "Updates configs",
			cs: &Config{
				dashboards: map[string]*configpb.Dashboard{
					"stale": {
						Name: "stale",
					},
				},
				attrs: storage.ReaderObjectAttrs{
					Generation: 1,
				},
			},
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
			expectUpdate: true,
		},
		{
			name: "Does not update same config",
			cs: &Config{
				dashboards: map[string]*configpb.Dashboard{
					"dashboard": {
						Name: "dashboard",
					},
				},
				attrs: storage.ReaderObjectAttrs{
					Generation: 1,
				},
			},
			config: &configpb.Configuration{
				Dashboards: []*configpb.Dashboard{
					{
						Name: "the-same",
					},
				},
			},
			configGeneration: 1,
			expectDashboard: &configpb.Dashboard{
				Name: "dashboard",
			},
		},
		{
			name: "Doesn't update on read error",
			cs: &Config{
				dashboards: map[string]*configpb.Dashboard{
					"dashboard": {
						Name: "dashboard",
					},
				},
				attrs: storage.ReaderObjectAttrs{
					Generation: 1,
				},
			},
			readErr: errors.New("can't read in this test"),
			expectDashboard: &configpb.Dashboard{
				Name: "dashboard",
			},
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

			client := &fake.ConditionalClient{
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

			client.Opener[*path] = fake.Object{
				Data: string(mustMarshalConfig(test.config)),
				Attrs: &storage.ReaderObjectAttrs{
					Generation: test.configGeneration,
				},
				ReadErr: test.readErr,
			}
			client.Stater[*path] = fake.Stat{
				Attrs: storage.ObjectAttrs{
					Generation: test.configGeneration,
				},
			}

			var updated bool
			onUpdate := func(*Config) error {
				updated = true
				return nil
			}

			reportsUpdate, err := test.cs.tick(ctx, client, *path, onUpdate)
			if (err != nil) != test.expectError {
				t.Errorf("Error: got %v, expected? %t", err, test.expectError)
			}

			if reportsUpdate != test.expectUpdate {
				t.Errorf("ReportsUpdate: got %t, expected %t", updated, test.expectUpdate)
			}

			if updated != test.expectUpdate {
				t.Errorf("OnUpdate: got %t, expected %t", updated, test.expectUpdate)
			}

			if result := test.cs.Dashboard("dashboard"); !proto.Equal(result, test.expectDashboard) {
				t.Errorf("got dashboard %v, expected %v", result, test.expectDashboard)
			}
		})
	}
}

func TestGetters(t *testing.T) {
	cs := Config{
		dashboards: map[string]*configpb.Dashboard{
			"corkboard": {
				Name: "corkboard",
				DashboardTab: []*configpb.DashboardTab{
					{
						Name:          "pushpins",
						TestGroupName: "pegs",
					},
					{
						Name:          "thumbtacks",
						TestGroupName: "corks",
					},
				},
			},
			"chalkboard": {
				Name: "chalkboard",
			},
		},
		groups: map[string]*configpb.TestGroup{
			"pegs": {
				Name: "pegs",
			},
			"corks": {
				Name: "corks",
			},
		},
	}

	tests := map[string]func(*testing.T){
		"TestDashboard": func(t *testing.T) {
			result := cs.Dashboard("corkboard")
			if result.Name != "corkboard" {
				t.Errorf("got %v, expected corkboard", result)
			}
		},
		"TestGroup": func(t *testing.T) {
			result := cs.Group("pegs")
			if result.Name != "pegs" {
				t.Errorf("got %v, expected pegs", result)
			}
		},
		"TestDashboardTestGroups": func(t *testing.T) {
			dash, groups := cs.DashboardTestGroups("corkboard")
			if dash.Name != "corkboard" {
				t.Errorf("got %v, expected corkboard", dash)
			}
			if g := groups["pushpins"]; g.Name != "pegs" {
				t.Errorf("got %v, expected pegs", g)
			}
			if g := groups["thumbtacks"]; g.Name != "corks" {
				t.Errorf("got %v, expected corks", g)
			}
		},
		"TestAllDashboards": func(t *testing.T) {
			dashes := cs.AllDashboards()
			if len(dashes) != 2 {
				t.Errorf("got %v, expected %d dashes", dashes, 2)
			}
		},
	}

	// Test for race conditions
	var wg sync.WaitGroup
	for _, test := range tests {
		wg.Add(3)
		for i := 0; i < 3; i++ {
			test := test
			go func() {
				test(t)
				wg.Done()
			}()
		}
	}
	wg.Wait()
}

func mustMarshalConfig(c *configpb.Configuration) []byte {
	b, err := proto.Marshal(c)
	if err != nil {
		panic(err)
	}
	return b
}
