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
	"path"

	v1 "github.com/GoogleCloudPlatform/testgrid/pkg/api/v1"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/gorilla/mux"
)

// RouterOptions are the options needed to GetRouter
type RouterOptions struct {
	GcsCredentials string
	Hostname       string
	HomeBucket     string
}

// GetRouter returns an http router that serves TestGrid's API
// It also instantiates necessary caching and i/o objects
func GetRouter(options RouterOptions) (*mux.Router, error) {
	r := mux.NewRouter()
	storageClient, err := gcs.ClientWithCreds(context.Background(), options.GcsCredentials)
	if err != nil {
		return nil, err
	}

	const v1Infix = "/api/v1"

	sub1 := r.PathPrefix(v1Infix).Subrouter()
	s := v1.Server{
		Client:        gcs.NewClient(storageClient),
		Host:          path.Join(options.Hostname, v1Infix),
		DefaultBucket: options.HomeBucket,
	}
	sub1.HandleFunc("/dashboard-groups", s.ListDashboardGroups).Methods("GET")

	return sub1, nil
}
