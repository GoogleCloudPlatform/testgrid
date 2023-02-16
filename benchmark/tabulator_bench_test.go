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

package benchmark

import (
	"context"
	"flag"
	"runtime"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/pkg/tabulator"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/GoogleCloudPlatform/testgrid/util/metrics/prometheus"
)

var config = flag.String("config", "", "Config file of working testgrid directory. The directory may be modified.")

func BenchmarkTabulator(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if config == nil || *config == "" {
		b.Fatalf("needs --config flag")
	}

	cfgPath, err := gcs.NewPath(*config)
	if err != nil {
		b.Fatalf("bad config path %q: %v", *config, err)
	}

	storageClient, err := gcs.ClientWithCreds(ctx, "")
	if err != nil {
		b.Fatalf("create client: %v", err)
	}
	defer storageClient.Close()

	client := gcs.NewClient(storageClient)

	mets := tabulator.CreateMetrics(prometheus.NewFactory()) // TODO(chases2): replace with logging metrics factory or teach tabulator to tolerate missing metrics

	opts := &tabulator.UpdateOptions{
		ConfigPath:          *cfgPath,
		ReadConcurrency:     2 * runtime.NumCPU(),
		WriteConcurrency:    4 * runtime.NumCPU(),
		GridPathPrefix:      "grid",
		TabsPathPrefix:      "tabs",
		AllowedGroups:       nil,
		Confirm:             true,
		CalculateStats:      true,
		UseTabAlertSettings: true,
		ExtendState:         false,
		Freq:                time.Duration(0),
	}
	if err = tabulator.Update(ctx, client, mets, opts); err != nil {
		b.Fatalf("update error: %v", err)
	}
}
