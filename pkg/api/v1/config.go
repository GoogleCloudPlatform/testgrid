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
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/GoogleCloudPlatform/testgrid/config"
	"github.com/GoogleCloudPlatform/testgrid/config/snapshot"
	apipb "github.com/GoogleCloudPlatform/testgrid/pb/api/v1"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
)

const scopeParam = "scope"

func (s *Server) configPath(scope string) (path *gcs.Path, isDefault bool, err error) {
	if scope != "" {
		path, err = gcs.NewPath(fmt.Sprintf("%s/%s", scope, "config"))
		return path, false, err
	}
	if s.DefaultBucket != "" {
		path, err = gcs.NewPath(fmt.Sprintf("%s/%s", s.DefaultBucket, "config"))
		return path, true, err
	}
	return nil, false, errors.New("no testgrid scope")
}

// queryParams returns only the query parameters in the request that need to be passed through to links
func queryParams(scope string) string {
	if scope != "" {
		return fmt.Sprintf("?scope=%s", scope)
	}
	return ""
}

// getConfig will return a config or an error.
// Does not expose wrapped errors to the user, instead logging them to the console.
func (s *Server) getConfig(ctx context.Context, log *logrus.Entry, scope string) (*snapshot.Config, error) {
	configPath, isDefault, err := s.configPath(scope)
	if err != nil || configPath == nil {
		return nil, errors.New("Scope not specified")
	}

	// TODO(chases2): If is default, keep a cached snapshot that we are regularly observing.
	// In addition, keep a map of normalized inputs -> real name to santize user input
	cfgChan, err := snapshot.Observe(ctx, log, s.Client, *configPath, nil)
	if err != nil {
		// Only log an error if we set and use a default scope, but can't access it.
		// Otherwise, invalid requests will write useless logs.
		if isDefault {
			log.WithError(err).Errorf("Can't read default config at %q; check permissions", configPath.String())
		}
		return nil, fmt.Errorf("Could not read config at %q", configPath.String())
	}
	return <-cfgChan, nil
}

// ListDashboardGroup returns every dashboard group in TestGrid
func (s *Server) ListDashboardGroup(ctx context.Context, req *apipb.ListDashboardGroupRequest) (*apipb.ListDashboardGroupResponse, error) {
	cfg, err := s.getConfig(ctx, logrus.WithContext(ctx), req.GetScope())
	if err != nil {
		return nil, err
	}

	var resp apipb.ListDashboardGroupResponse
	for name := range cfg.DashboardGroups {
		rsc := apipb.Resource{
			Name: name,
			Link: fmt.Sprintf("/dashboard-groups/%s%s", config.Normalize(name), queryParams(req.GetScope())),
		}
		resp.DashboardGroups = append(resp.DashboardGroups, &rsc)
	}

	sort.SliceStable(resp.DashboardGroups, func(i, j int) bool {
		return resp.DashboardGroups[i].Name < resp.DashboardGroups[j].Name
	})

	return &resp, nil
}

// ListDashboardGroupHTTP returns every dashboard group in TestGrid
// Response json: ListDashboardGroupResponse
func (s Server) ListDashboardGroupHTTP(w http.ResponseWriter, r *http.Request) {
	req := apipb.ListDashboardGroupRequest{
		Scope: r.URL.Query().Get(scopeParam),
	}

	groups, err := s.ListDashboardGroup(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	s.writeJSON(w, &groups)
}

// GetDashboardGroup returns a given dashboard group
func (s *Server) GetDashboardGroup(ctx context.Context, req *apipb.GetDashboardGroupRequest) (*apipb.GetDashboardGroupResponse, error) {
	cfg, err := s.getConfig(ctx, logrus.WithContext(ctx), req.GetScope())
	if err != nil {
		return nil, err
	}

	dashboardGroupKey := config.Normalize(req.GetDashboardGroup())
	for _, group := range cfg.DashboardGroups {
		if config.Normalize(group.Name) == dashboardGroupKey {
			result := apipb.GetDashboardGroupResponse{}
			for _, dash := range group.DashboardNames {
				rsc := apipb.Resource{
					Name: dash,
					Link: fmt.Sprintf("/dashboards/%s%s", config.Normalize(dash), queryParams(req.GetScope())),
				}
				result.Dashboards = append(result.Dashboards, &rsc)
			}
			sort.SliceStable(result.Dashboards, func(i, j int) bool {
				return result.Dashboards[i].Name < result.Dashboards[j].Name
			})
			return &result, nil
		}
	}
	return nil, fmt.Errorf("Dashboard group %q not found", req.GetDashboardGroup())
}

// GetDashboardGroupHTTP returns a given dashboard group
// Response json: GetDashboardGroupResponse
func (s Server) GetDashboardGroupHTTP(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	req := apipb.GetDashboardGroupRequest{
		Scope:          r.URL.Query().Get(scopeParam),
		DashboardGroup: vars["dashboard-group"],
	}
	resp, err := s.GetDashboardGroup(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	s.writeJSON(w, resp)
}

// ListDashboard returns every dashboard in TestGrid
func (s *Server) ListDashboard(ctx context.Context, req *apipb.ListDashboardRequest) (*apipb.ListDashboardResponse, error) {
	cfg, err := s.getConfig(ctx, logrus.WithContext(ctx), req.GetScope())
	if err != nil {
		return nil, err
	}

	var resp apipb.ListDashboardResponse
	for name := range cfg.Dashboards {
		rsc := apipb.Resource{
			Name: name,
			Link: fmt.Sprintf("/dashboards/%s%s", config.Normalize(name), queryParams(req.GetScope())),
		}
		resp.Dashboards = append(resp.Dashboards, &rsc)
	}
	sort.SliceStable(resp.Dashboards, func(i, j int) bool {
		return resp.Dashboards[i].Name < resp.Dashboards[j].Name
	})

	return &resp, nil
}

// ListDashboardsHTTP returns every dashboard in TestGrid
// Response json: ListDashboardResponse
func (s Server) ListDashboardsHTTP(w http.ResponseWriter, r *http.Request) {
	req := apipb.ListDashboardRequest{
		Scope: r.URL.Query().Get(scopeParam),
	}

	dashboards, err := s.ListDashboard(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	s.writeJSON(w, &dashboards)
}

// GetDashboard returns a given dashboard
func (s *Server) GetDashboard(ctx context.Context, req *apipb.GetDashboardRequest) (*apipb.GetDashboardResponse, error) {
	cfg, err := s.getConfig(ctx, logrus.WithContext(ctx), req.GetScope())
	if err != nil {
		return nil, err
	}

	dashboardKey := config.Normalize(req.GetDashboard())
	for _, dashboard := range cfg.Dashboards {
		if config.Normalize(dashboard.Name) == dashboardKey {
			result := apipb.GetDashboardResponse{
				DefaultTab:          dashboard.DefaultTab,
				HighlightToday:      dashboard.HighlightToday,
				SuppressFailingTabs: dashboard.DownplayFailingTabs,
				Notifications:       dashboard.Notifications,
			}
			return &result, nil
		}
	}
	return nil, fmt.Errorf("Dashboard %q not found", req.GetDashboard())
}

// GetDashboardHTTP returns a given dashboard
// Response json: GetDashboardResponse
func (s Server) GetDashboardHTTP(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	req := apipb.GetDashboardRequest{
		Scope:     r.URL.Query().Get(scopeParam),
		Dashboard: vars["dashboard"],
	}
	resp, err := s.GetDashboard(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	s.writeJSON(w, resp)
}

// ListDashboardTabs returns a given dashboard tabs
func (s *Server) ListDashboardTabs(ctx context.Context, req *apipb.ListDashboardTabsRequest) (*apipb.ListDashboardTabsResponse, error) {
	cfg, err := s.getConfig(ctx, logrus.WithContext(ctx), req.GetScope())
	if err != nil {
		return nil, err
	}

	dashboardKey := config.Normalize((req.GetDashboard()))
	var dashboardTabsResponse apipb.ListDashboardTabsResponse
	for _, dashboard := range cfg.Dashboards {
		if config.Normalize(dashboard.Name) == dashboardKey {
			for _, tab := range dashboard.DashboardTab {
				rsc := apipb.Resource{
					Name: tab.Name,
					Link: fmt.Sprintf("/dashboards/%s/tabs/%s%s", config.Normalize(dashboard.Name), config.Normalize(tab.Name), queryParams(req.GetScope())),
				}
				dashboardTabsResponse.DashboardTabs = append(dashboardTabsResponse.DashboardTabs, &rsc)
			}
			sort.SliceStable(dashboardTabsResponse.DashboardTabs, func(i, j int) bool {
				return dashboardTabsResponse.DashboardTabs[i].Name < dashboardTabsResponse.DashboardTabs[j].Name
			})
			return &dashboardTabsResponse, nil
		}
	}
	return nil, fmt.Errorf("Dashboard %q not found", req.GetDashboard())
}

// ListDashboardTabsHTTP returns a given dashboard tabs
// Response json: ListDashboardTabsResponse
func (s Server) ListDashboardTabsHTTP(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	req := apipb.ListDashboardTabsRequest{
		Scope:     r.URL.Query().Get(scopeParam),
		Dashboard: vars["dashboard"],
	}

	resp, err := s.ListDashboardTabs(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	s.writeJSON(w, resp)
}
