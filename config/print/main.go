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

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/GoogleCloudPlatform/testgrid/config"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"

	"github.com/sirupsen/logrus"
)

type options struct {
	configPath string
	creds      string
}

func gatherOptions() options {
	var o options
	if len(os.Args) > 1 {
		o.configPath = os.Args[1]
	}
	flag.StringVar(&o.configPath, "path", o.configPath, "Local or cloud config to read")
	flag.StringVar(&o.creds, "gcp-service-account", "", "/path/to/gcp/creds (use local creds if empty)")
	flag.Parse()
	return o
}

func main() {
	log := logrus.WithField("component", "config-printer")
	opt := gatherOptions()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	storageClient, err := gcs.ClientWithCreds(ctx, opt.creds)
	if err != nil {
		log.WithError(err).Info("Can't make cloud storage client; proceeding")
	}

	cfg, err := config.Read(ctx, opt.configPath, storageClient)
	if err != nil {
		logrus.WithError(err).WithField("path", opt.configPath).Fatal("Can't read from path")
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		logrus.WithError(err).Fatal("Can't marshal JSON")
	}

	fmt.Printf("%s\n", b)
}
