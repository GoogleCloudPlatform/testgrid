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

package v1

import (
	"net/url"
	"time"

	apipb "github.com/GoogleCloudPlatform/testgrid/pb/api/v1"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/gorilla/mux"
)

// Server contains the necessary settings and i/o objects needed to serve this api
type Server struct {
	Client         gcs.Client
	Host           *url.URL
	DefaultBucket  string
	GridPathPrefix string
	Timeout        time.Duration
}

// Ensure the server implementation conforms to the API
var _ apipb.TestGridDataServer = (*Server)(nil)

// Route applies all the v1 API functions provided by the Server to the Router given.
// If the router is nil, a new one is instantiated.
func Route(r *mux.Router, s Server) *mux.Router {
	if r == nil {
		r = mux.NewRouter()
	}
	r.HandleFunc("/dashboard-groups", s.ListDashboardGroupHTTP).Methods("GET")
	r.HandleFunc("/dashboard-groups/{dashboard-group}", s.GetDashboardGroupHTTP).Methods("GET")
	r.HandleFunc("/dashboards", s.ListDashboardsHTTP).Methods("GET")
	r.HandleFunc("/dashboards/{dashboard}/tabs", s.ListDashboardTabsHTTP).Methods("GET")
	r.HandleFunc("/dashboards/{dashboard}", s.GetDashboardHTTP).Methods("GET")

	r.HandleFunc("/dashboards/{dashboard}/tabs/{tab}/headers", s.ListHeadersHTTP).Methods("GET")
	r.HandleFunc("/dashboards/{dashboard}/tabs/{tab}/rows", s.ListRowsHTTP).Methods("GET")
	return r
}
