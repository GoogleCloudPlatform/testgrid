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
	"context"
	"fmt"
	"net/http"
	"sort"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/GoogleCloudPlatform/testgrid/config"
	apipb "github.com/GoogleCloudPlatform/testgrid/pb/api/v1"
)

const scopeParam = "scope"

// queryParams returns only the query parameters in the request that need to be passed through to links
func queryParams(scope string) string {
	if scope != "" {
		return fmt.Sprintf("?scope=%s", scope)
	}
	return ""
}

// ListDashboardGroup returns every dashboard group in TestGrid
func (s *Server) ListDashboardGroup(ctx context.Context, req *apipb.ListDashboardGroupRequest) (*apipb.ListDashboardGroupResponse, error) {
	c, err := s.getConfig(ctx, logrus.WithContext(ctx), req.GetScope())
	if err != nil {
		return nil, err
	}
	c.Mutex.RLock()
	defer c.Mutex.RUnlock()

	var resp apipb.ListDashboardGroupResponse
	for name := range c.Config.DashboardGroups {
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
	c, err := s.getConfig(ctx, logrus.WithContext(ctx), req.GetScope())
	if err != nil {
		return nil, err
	}
	c.Mutex.RLock()
	defer c.Mutex.RUnlock()

	dashGroupName := c.NormalDashboardGroup[config.Normalize(req.GetDashboardGroup())]
	group := c.Config.DashboardGroups[dashGroupName]

	if group != nil {
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
	c, err := s.getConfig(ctx, logrus.WithContext(ctx), req.GetScope())
	if err != nil {
		return nil, err
	}
	c.Mutex.RLock()
	defer c.Mutex.RUnlock()

	var resp apipb.ListDashboardResponse
	for name := range c.Config.Dashboards {
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
	c, err := s.getConfig(ctx, logrus.WithContext(ctx), req.GetScope())
	if err != nil {
		return nil, err
	}
	c.Mutex.RLock()
	defer c.Mutex.RUnlock()

	dashboardName := c.NormalDashboard[config.Normalize(req.GetDashboard())]
	dashboard := c.Config.Dashboards[dashboardName]

	if dashboard != nil {
		result := apipb.GetDashboardResponse{
			DefaultTab:          dashboard.DefaultTab,
			HighlightToday:      dashboard.HighlightToday,
			SuppressFailingTabs: dashboard.DownplayFailingTabs,
			Notifications:       dashboard.Notifications,
		}
		return &result, nil
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
	c, err := s.getConfig(ctx, logrus.WithContext(ctx), req.GetScope())
	if err != nil {
		return nil, err
	}
	c.Mutex.RLock()
	defer c.Mutex.RUnlock()

	dashboardName := c.NormalDashboard[config.Normalize((req.GetDashboard()))]
	dashboard := c.Config.Dashboards[dashboardName]

	if dashboard != nil {
		var dashboardTabsResponse apipb.ListDashboardTabsResponse

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
