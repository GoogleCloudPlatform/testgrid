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

// Package api provides code to host an API displaying TestGrid data
package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"cloud.google.com/go/storage"
	"github.com/gorilla/mux"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	v1pb "github.com/GoogleCloudPlatform/testgrid/pb/api/v1"
	v1 "github.com/GoogleCloudPlatform/testgrid/pkg/api/v1"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
)

// RouterOptions are the options needed to GetRouter
type RouterOptions struct {
	GcsCredentials           string
	HomeBucket               string
	TabPathPrefix            string
	SummaryPathPrefix        string
	AccessControlAllowOrigin string
	Timeout                  time.Duration
}

const v1InfixRef = "/api/v1"

var healthCheckFile = "pkg/api/README.md" // a relative path

// GetRouters returns an http router and gRPC server that both serve TestGrid's API
// It also instantiates necessary caching and i/o objects
func GetRouters(options RouterOptions, storageClient *storage.Client) (*mux.Router, *grpc.Server, error) {
	server, err := GetServer(options, storageClient)
	if err != nil {
		return nil, nil, err
	}

	router := mux.NewRouter()
	router.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		http.ServeFile(w, req, healthCheckFile)
	})
	sub1 := router.PathPrefix(v1InfixRef).Subrouter()
	v1.Route(sub1, *server)

	grpcOptions := []grpc.ServerOption{}
	grpcServer := grpc.NewServer(grpcOptions...)
	v1pb.RegisterTestGridDataServer(grpcServer, server)
	reflection.Register(grpcServer)

	return router, grpcServer, nil
}

// GetServer returns a server that serves TestGrid's API
// It also instantiates necessary caching and i/o objects
func GetServer(options RouterOptions, storageClient *storage.Client) (*v1.Server, error) {
	if storageClient == nil {
		sc, err := gcs.ClientWithCreds(context.Background(), options.GcsCredentials)
		if err != nil {
			return nil, fmt.Errorf("clientWithCreds(): %w", err)
		}
		storageClient = sc
	}

	return &v1.Server{
		Client:                   gcs.NewClient(storageClient),
		DefaultBucket:            options.HomeBucket,
		TabPathPrefix:            options.TabPathPrefix,
		SummaryPathPrefix:        options.SummaryPathPrefix,
		AccessControlAllowOrigin: options.AccessControlAllowOrigin,
		Timeout:                  options.Timeout,
	}, nil
}
