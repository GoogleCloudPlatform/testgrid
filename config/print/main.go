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
	"bytes"
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
	configPath    string
	creds         string
	printFieldUse bool
	compressed    bool
}

func gatherOptions() options {
	var o options
	if len(os.Args) > 1 {
		o.configPath = os.Args[1]
	}
	flag.StringVar(&o.configPath, "path", o.configPath, "Local or cloud config to read")
	flag.StringVar(&o.creds, "gcp-service-account", "", "/path/to/gcp/creds (use local creds if empty)")
	flag.BoolVar(&o.printFieldUse, "print-field-use", false, "If True, print all config fields and # of uses of each")
	flag.BoolVar(&o.compressed, "compressed", false, "Disables pretty-printing, removing all whitespace between fields")
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

	if opt.compressed {
		fmt.Printf("%s\n", b)
	} else {
		var out bytes.Buffer
		json.Indent(&out, b, "", "  ")
		out.WriteTo(os.Stdout)
	}

	if opt.printFieldUse {
		err = printFieldNames(b)
		if err != nil {
			logrus.WithError(err).Fatal("Error printing fields")
		}
	}
}

func printFieldNames(b []byte) error {
	type Dashboard struct {
		DashboardTabs []map[string]interface{} `json:"dashboard_tab"`
	}
	type Config struct {
		TestGroups      []map[string]interface{} `json:"test_groups"`
		Dashboards      []Dashboard              `json:"dashboards"`
		DashboardGroups []map[string]interface{} `json:"dashboard_groups"`
	}
	var output Config
	err := json.Unmarshal(b, &output)
	if err != nil {
		return err
	}

	fmt.Printf("%d test groups, %d dashboards, %d dashboard groups\n", len(output.TestGroups), len(output.Dashboards), len(output.DashboardGroups))

	testGroupFields := map[string]int64{}
	for _, value := range output.TestGroups {
		for k := range value {
			testGroupFields[k]++
		}
	}
	fmt.Printf("Test Groups (%d fields):\n", len(testGroupFields))
	for k, v := range testGroupFields {
		fmt.Printf("%s,%d\n", k, v)
	}

	tabFields := map[string]int64{}
	for _, dashboard := range output.Dashboards {
		for _, val := range dashboard.DashboardTabs {
			for k := range val {
				tabFields[k]++
			}
		}
	}
	fmt.Printf("Dashboard Tabs (%d fields):\n", len(tabFields))
	for k, v := range tabFields {
		fmt.Printf("%s,%d\n", k, v)
	}
	fmt.Printf("Test Groups (%d fields):\n", len(testGroupFields))
	for k, v := range testGroupFields {
		fmt.Printf("%s,%d\n", k, v)
	}
	return nil
}
