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

// Package v1 (api/v1) is the first versioned implementation of the API
package v1

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/GoogleCloudPlatform/testgrid/config"
	apipb "github.com/GoogleCloudPlatform/testgrid/pb/api/v1"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
)

func (s Server) configPath(r *http.Request) (path *gcs.Path, isDefault bool, err error) {
	scope := r.URL.Query().Get("scope")
	if scope != "" {
		path, err = gcs.NewPath(fmt.Sprintf("%s/%s", scope, "config"))
		isDefault = false
		return
	}
	if s.DefaultBucket != "" {
		path, err = gcs.NewPath(fmt.Sprintf("%s/%s", s.DefaultBucket, "config"))
		isDefault = true
		return
	}
	return nil, false, errors.New("no testgrid scope")
}

// passQueryParameters returns only the query parameters in the request that need to be passed through to links
func passQueryParameters(r *http.Request) string {
	if scope := r.URL.Query().Get("scope"); scope != "" {
		return fmt.Sprintf("?scope=%s", scope)
	}
	return ""
}

// getConfig will return a config file or will send an error to the http writer
// If this function returns nil, no further writes should be made to 'w'
func (s Server) getConfig(w http.ResponseWriter, r *http.Request) *configpb.Configuration {
	configPath, isDefault, err := s.configPath(r)
	if err != nil || configPath == nil {
		http.Error(w, "Scope not specified", http.StatusBadRequest)
		return nil
	}

	cfg, err := config.ReadGCS(r.Context(), s.Client, *configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Could not read config at %q", configPath.String()), http.StatusInternalServerError)
		// Only log an error if we set and use a default scope, but can't access it.
		// Otherwise, invalid requests will write useless logs.
		if isDefault {
			// TODO(chases2): Pass a logrus logger through the server object
			logrus.WithError(err).Errorf("Can't read default config at %q; check permissions", configPath.String())
		}
		return nil
	}
	return cfg
}

// ListDashboardGroups returns every dashboard group in TestGrid
// Response Proto: ListDashboardGroupResponse
func (s Server) ListDashboardGroups(w http.ResponseWriter, r *http.Request) {
	cfg := s.getConfig(w, r)
	if cfg == nil {
		return
	}

	var groups apipb.ListDashboardGroupResponse
	for _, group := range cfg.DashboardGroups {
		rsc := apipb.Resource{
			Name: group.Name,
			Link: fmt.Sprintf("%s/dashboard-groups/%s%s", s.Host, config.Normalize(group.Name), passQueryParameters(r)),
		}
		groups.DashboardGroups = append(groups.DashboardGroups, &rsc)
	}

	writeJSON(w, &groups)
}

// GetDashboardGroup returns a given dashboard group
// Response Proto: GetDashboardGroupResponse
func (s Server) GetDashboardGroup(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cfg := s.getConfig(w, r)
	if cfg == nil {
		return
	}

	for _, group := range cfg.DashboardGroups {
		if group.Name == vars["dashboard-group"] {
			result := apipb.GetDashboardGroupResponse{}
			for _, dash := range group.DashboardNames {
				rsc := apipb.Resource{
					Name: dash,
					Link: fmt.Sprintf("%s/dashboards/%s%s", s.Host, config.Normalize(dash), passQueryParameters(r)),
				}
				result.Dashboards = append(result.Dashboards, &rsc)
			}
			writeJSON(w, &result)
			return
		}
	}
	http.Error(w, fmt.Sprintf("Dashboard group %q not found", vars["dashboard-group"]), http.StatusNotFound)
}
