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
	"net/url"
	"time"

	"github.com/gorilla/mux"

	"cloud.google.com/go/storage"
	v1 "github.com/GoogleCloudPlatform/testgrid/pkg/api/v1"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
)

// RouterOptions are the options needed to GetRouter
type RouterOptions struct {
	GcsCredentials string
	Hostname       *url.URL
	HomeBucket     string
	GridPathPrefix string
	Timeout        time.Duration
}

const v1InfixRef = "/api/v1"

// GetRouter returns an http router that serves TestGrid's API
// It also instantiates necessary caching and i/o objects
func GetRouter(options RouterOptions, storageClient *storage.Client) (*mux.Router, error) {
	r := mux.NewRouter()
	if storageClient == nil {
		sc, err := gcs.ClientWithCreds(context.Background(), options.GcsCredentials)
		if err != nil {
			return nil, err
		}
		storageClient = sc
	}

	v1InfixURL, err := url.Parse(v1InfixRef)
	if err != nil {
		return nil, err
	}

	sub1 := r.PathPrefix(v1InfixRef).Subrouter()
	s := v1.Server{
		Client:         gcs.NewClient(storageClient),
		Host:           options.Hostname.ResolveReference(v1InfixURL),
		DefaultBucket:  options.HomeBucket,
		GridPathPrefix: options.GridPathPrefix,
		Timeout:        options.Timeout,
	}
	v1.Route(sub1, s)

	return sub1, nil
}
