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

// The api utility hosts a web server that serves TestGrid data
package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/GoogleCloudPlatform/testgrid/pkg/api"
)

type options struct {
	httpPort string
	grpcPort string
	router   api.RouterOptions
}

func gatherOptions() (options, error) {
	var o options
	flag.StringVar(&o.router.HomeBucket, "scope", "", "Local or cloud TestGrid context to read from")
	flag.StringVar(&o.router.GcsCredentials, "gcp-service-account", "", "/path/to/gcp/creds (use local creds if empty)")
	flag.StringVar(&o.httpPort, "port", "8080", "Alias for http-port")
	flag.StringVar(&o.httpPort, "http-port", "8080", "Port to deploy REST server to")
	flag.StringVar(&o.grpcPort, "grpc-port", "50051", "Port to deploy gRPC server to")
	flag.StringVar(&o.router.AccessControlAllowOrigin, "allowed-origin", "", "Allowed 'Access-Control-Allow-Origin' for HTTP calls, if any")
	flag.StringVar(&o.router.GridPathPrefix, "grid", "grid", "Read grid states under this GCS path.")
	flag.StringVar(&o.router.TabPathPrefix, "tab", "tabs", "Read tab path states under this path")
	flag.DurationVar(&o.router.Timeout, "timeout", 10*time.Minute, "Maximum time allocated to complete one request")
	flag.Parse()

	return o, nil
}

func main() {
	log := logrus.WithField("component", "api")
	opt, err := gatherOptions()
	if err != nil {
		log.WithError(err).Fatal("Can't parse options")
	}

	httpMux, grpcMux, err := api.GetRouters(opt.router, nil)
	if err != nil {
		log.WithError(err).WithField("router-options", opt.router).Fatal("Can't create router")
	}

	terminate := make(chan interface{})
	go func() {
		lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%s", opt.grpcPort))
		if err != nil {
			log.Fatalf("failed to listen: %v", err)
		}

		log.WithField("port", opt.grpcPort).Info("Listening via gRPC...")
		if err := grpcMux.Serve(lis); err != nil {
			log.WithError(err).Error("gRPC Server Error")
		} else {
			log.Info("gRPC listener stopped listening (with no error)")
		}
		terminate <- 0
	}()

	log.WithField("port", opt.httpPort).Info("Listening via http...")
	if err := http.ListenAndServe(fmt.Sprintf(":%s", opt.httpPort), httpMux); err != nil {
		log.WithError(err).Error("HTTP Server Error")
	} else {
		log.Info("HTTP listener stopped listening (with no error)")
	}
	<-terminate
}
