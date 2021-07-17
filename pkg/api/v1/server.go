package v1

import (
	"github.com/gorilla/mux"

	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
)

// Server contains the necessary settings and i/o objects needed to serve this api
type Server struct {
	Client        gcs.Client
	Host          string
	DefaultBucket string
}

// Route applies all the v1 API functions provided by the Server to the Router given.
// If the router is nil, a new one is instantiated.
func Route(r *mux.Router, s Server) *mux.Router {
	if r == nil {
		r = mux.NewRouter()
	}
	r.HandleFunc("/dashboard-groups", s.ListDashboardGroups).Methods("GET")
	r.HandleFunc("/dashboard-groups/{dashboard-group}", s.GetDashboardGroup).Methods("GET")
	return r
}
