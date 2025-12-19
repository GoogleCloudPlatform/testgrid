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

	"github.com/go-chi/chi"
	"github.com/sirupsen/logrus"

	"github.com/GoogleCloudPlatform/testgrid/config"
	apipb "github.com/GoogleCloudPlatform/testgrid/pb/api/v1"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
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
func (s *Server) ListDashboardGroups(ctx context.Context, req *apipb.ListDashboardGroupsRequest) (*apipb.ListDashboardGroupsResponse, error) {
	c, err := s.getConfig(ctx, logrus.WithContext(ctx), req.GetScope())
	if err != nil {
		return nil, err
	}
	c.Mutex.RLock()
	defer c.Mutex.RUnlock()

	var resp apipb.ListDashboardGroupsResponse
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
	req := apipb.ListDashboardGroupsRequest{
		Scope: r.URL.Query().Get(scopeParam),
	}

	groups, err := s.ListDashboardGroups(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	s.writeJSON(w, groups)
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

// Translates link template options from the config proto to the api proto.
func apiLinkOptionsTemplates(options []*configpb.LinkOptionsTemplate) []*apipb.LinkOptionsTemplate {
	result := []*apipb.LinkOptionsTemplate{}
	for _, opt := range options {
		result = append(result, &apipb.LinkOptionsTemplate{
			Key:   opt.GetKey(),
			Value: opt.GetValue(),
		})
	}
	return result
}

func apiResultSource(resultSource *configpb.TestGroup_ResultSource, gcsPrefix string) *apipb.ResultSource {
	result := &apipb.ResultSource{}
	switch resultSource.GetResultSourceConfig().(type) {
	case *configpb.TestGroup_ResultSource_GcsConfig:
		result.ResultSourceConfig = &apipb.ResultSource_GcsConfig{
			GcsConfig: &apipb.GCSConfig{
				GcsPrefix:          resultSource.GetGcsConfig().GetGcsPrefix(),
				PubsubProject:      resultSource.GetGcsConfig().GetPubsubProject(),
				PubsubSubscription: resultSource.GetGcsConfig().GetPubsubSubscription(),
			},
		}
	case *configpb.TestGroup_ResultSource_ResultstoreConfig:
		result.ResultSourceConfig = &apipb.ResultSource_ResultstoreConfig{
			ResultstoreConfig: &apipb.ResultStoreConfig{
				Project: resultSource.GetResultstoreConfig().GetProject(),
				Query:   resultSource.GetResultstoreConfig().GetQuery(),
			},
		}
	default:
		// No result source was specified; use the value from gcsPrefix instead.
		result.ResultSourceConfig = &apipb.ResultSource_GcsConfig{
			GcsConfig: &apipb.GCSConfig{
				GcsPrefix: gcsPrefix,
			},
		}
	}
	return result
}

func tabMenuTemplates(tab *configpb.DashboardTab) []*apipb.LinkTemplate {
	var templates []*apipb.LinkTemplate
	if tab.GetResultsUrlTemplate() != nil {
		templates = append(templates, &apipb.LinkTemplate{
			Text:    tab.GetResultsText(),
			Url:     tab.GetResultsUrlTemplate().GetUrl(),
			Options: apiLinkOptionsTemplates(tab.GetResultsUrlTemplate().GetOptions()),
		})
	}
	if tab.GetAboutDashboardUrl() != "" {
		templates = append(templates, &apipb.LinkTemplate{
			Text: "About this tab",
			Url:  tab.GetAboutDashboardUrl(),
		})
	}
	return templates
}

func cellMenuTemplates(tab *configpb.DashboardTab) []*apipb.LinkTemplate {
	var templates []*apipb.LinkTemplate
	if tab.GetOpenTestTemplate() != nil {
		templates = append(templates, &apipb.LinkTemplate{
			Text:    "Open Test",
			Url:     tab.GetOpenTestTemplate().GetUrl(),
			Options: apiLinkOptionsTemplates(tab.GetOpenTestTemplate().GetOptions()),
		})
	}
	if tab.GetFileBugTemplate() != nil {
		templates = append(templates, &apipb.LinkTemplate{
			Text:    "File a Bug",
			Url:     tab.GetFileBugTemplate().GetUrl(),
			Options: apiLinkOptionsTemplates(tab.GetFileBugTemplate().GetOptions()),
		})
	}
	if tab.GetAttachBugTemplate() != nil {
		templates = append(templates, &apipb.LinkTemplate{
			Text:    "Attach to Bug",
			Url:     tab.GetAttachBugTemplate().GetUrl(),
			Options: apiLinkOptionsTemplates(tab.GetAttachBugTemplate().GetOptions()),
		})
	}
	return templates
}

func columnDiffMenuTemplates(tab *configpb.DashboardTab) []*apipb.LinkTemplate {
	var templates []*apipb.LinkTemplate
	if tab.GetCodeSearchUrlTemplate() != nil {
		templates = append(templates, &apipb.LinkTemplate{
			Text:    "Search for Changes",
			Url:     tab.GetCodeSearchUrlTemplate().GetUrl(),
			Options: apiLinkOptionsTemplates(tab.GetCodeSearchUrlTemplate().GetOptions()),
		})
	}
	if len(tab.GetColumnDiffLinkTemplates()) != 0 {
		for _, diffTemplate := range tab.GetColumnDiffLinkTemplates() {
			templates = append(templates, &apipb.LinkTemplate{
				Text:    diffTemplate.GetName(),
				Url:     diffTemplate.GetUrl(),
				Options: apiLinkOptionsTemplates(diffTemplate.GetOptions()),
			})
		}
	}
	return templates
}

// GetDashboardTab returns the configuration for a given dashboard tab.
func (s *Server) GetDashboardTab(ctx context.Context, req *apipb.GetDashboardTabRequest) (*apipb.GetDashboardTabResponse, error) {
	c, err := s.getConfig(ctx, logrus.WithContext(ctx), req.GetScope())
	if err != nil {
		return nil, err
	}
	c.Mutex.RLock()
	defer c.Mutex.RUnlock()

	dashboardName := c.NormalDashboard[config.Normalize(req.GetDashboard())]
	dashboard := c.Config.Dashboards[dashboardName]
	tabName := c.NormalDashboardTab[config.Normalize(req.GetDashboard())][config.Normalize(req.GetTab())]

	for _, tab := range dashboard.GetDashboardTab() {
		if tab.GetName() == tabName {
			testGroupName := c.NormalTestGroup[config.Normalize(tab.GetTestGroupName())]
			testGroup := c.Config.Groups[testGroupName]
			if testGroup == nil {
				return nil, fmt.Errorf("test group %q backing dashboard tab %q/%q not found", tab.GetTestGroupName(), req.GetDashboard(), req.GetTab())
			}

			return &apipb.GetDashboardTabResponse{
				Name:                tab.GetName(),
				TestGroupName:       tab.GetTestGroupName(),
				Description:         tab.GetDescription(),
				ResultSource:        apiResultSource(testGroup.GetResultSource(), testGroup.GetGcsPrefix()),
				TabMenuTemplates:    tabMenuTemplates(tab),
				CellMenuTemplates:   cellMenuTemplates(tab),
				ColumnDiffTemplates: columnDiffMenuTemplates(tab),
				AlertOptions: &apipb.AlertOptions{
					NumColumnsRecent:        tab.GetNumColumnsRecent(),
					AlertStaleResultsHours:  tab.GetAlertOptions().GetAlertStaleResultsHours(),
					NumFailuresToAlert:      tab.GetAlertOptions().GetNumFailuresToAlert(),
					NumPassesToDisableAlert: tab.GetAlertOptions().GetNumPassesToDisableAlert(),
				},
				BaseOptions:       tab.GetBaseOptions(),
				DisplayLocalTime:  tab.GetDisplayLocalTime(),
				TabularNamesRegex: tab.GetTabularNamesRegex(),
			}, nil
		}
	}

	return nil, fmt.Errorf("dashboard tab %q/%q not found", req.GetDashboard(), req.GetTab())
}

// GetDashboardTabHTTP returns a given dashboard tab
// Response json: GetDashboardTabResponse
func (s Server) GetDashboardTabHTTP(w http.ResponseWriter, r *http.Request) {
	req := apipb.GetDashboardTabRequest{
		Scope:     r.URL.Query().Get(scopeParam),
		Dashboard: chi.URLParam(r, "dashboard"),
		Tab:       chi.URLParam(r, "tab"),
	}
	resp, err := s.GetDashboardTab(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	s.writeJSON(w, resp)
}

// GetDashboardGroupHTTP returns a given dashboard group
// Response json: GetDashboardGroupResponse
func (s Server) GetDashboardGroupHTTP(w http.ResponseWriter, r *http.Request) {
	req := apipb.GetDashboardGroupRequest{
		Scope:          r.URL.Query().Get(scopeParam),
		DashboardGroup: chi.URLParam(r, "dashboard-group"),
	}
	resp, err := s.GetDashboardGroup(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	s.writeJSON(w, resp)
}

// ListDashboard returns every dashboard in TestGrid
func (s *Server) ListDashboards(ctx context.Context, req *apipb.ListDashboardsRequest) (*apipb.ListDashboardsResponse, error) {
	c, err := s.getConfig(ctx, logrus.WithContext(ctx), req.GetScope())
	if err != nil {
		return nil, err
	}
	c.Mutex.RLock()
	defer c.Mutex.RUnlock()

	// TODO(sultan-duisenbay): consider moving this to config snapshot or cached config
	dashboardsToGroups := make(map[string]string)
	for _, groupConfig := range c.Config.DashboardGroups {
		for _, dashboardName := range groupConfig.DashboardNames {
			dashboardsToGroups[dashboardName] = groupConfig.Name
		}
	}

	var resp apipb.ListDashboardsResponse
	for name := range c.Config.Dashboards {
		rsc := apipb.DashboardResource{
			Name:               name,
			Link:               fmt.Sprintf("/dashboards/%s%s", config.Normalize(name), queryParams(req.GetScope())),
			DashboardGroupName: dashboardsToGroups[name],
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
	req := apipb.ListDashboardsRequest{
		Scope: r.URL.Query().Get(scopeParam),
	}

	dashboards, err := s.ListDashboards(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	s.writeJSON(w, dashboards)
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
	req := apipb.GetDashboardRequest{
		Scope:     r.URL.Query().Get(scopeParam),
		Dashboard: chi.URLParam(r, "dashboard"),
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
	req := apipb.ListDashboardTabsRequest{
		Scope:     r.URL.Query().Get(scopeParam),
		Dashboard: chi.URLParam(r, "dashboard"),
	}

	resp, err := s.ListDashboardTabs(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	s.writeJSON(w, resp)
}
