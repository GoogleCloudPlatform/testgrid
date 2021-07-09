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

package main

import (
	"flag"
	"fmt"
	"net/http"

	"github.com/GoogleCloudPlatform/testgrid/pkg/api"
	"github.com/sirupsen/logrus"
)

type options struct {
	port   string
	router api.RouterOptions
}

func gatherOptions() options {
	var o options
	flag.StringVar(&o.router.HomeBucket, "scope", "", "Local or cloud TestGrid context to read from")
	flag.StringVar(&o.router.GcsCredentials, "gcp-service-account", "", "/path/to/gcp/creds (use local creds if empty)")
	flag.StringVar(&o.port, "port", "8080", "Port to deploy to")
	flag.StringVar(&o.router.Hostname, "host", "", "Friendly hostname used to serve links")
	flag.Parse()

	if o.router.Hostname == "" {
		o.router.Hostname = fmt.Sprintf("localhost:%s", o.port)
	}
	return o
}

func main() {
	log := logrus.WithField("component", "api")
	opt := gatherOptions()

	log.WithField("port", opt.port).Info("Listening...")
	router, err := api.GetRouter(opt.router)
	if err != nil {
		log.WithError(err).WithField("router-options", opt.router).Fatal("Can't create router")
	}

	if err := http.ListenAndServe(fmt.Sprintf(":%s", opt.port), router); err != nil {
		log.WithError(err).Fatal("HTTP Server Error")
	} else {
		log.Info("HTTP listener stopped listening (with no error)")
	}
}
