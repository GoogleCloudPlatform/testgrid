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
	"math"
	"net/http"
	"net/url"
	"path"

	"github.com/GoogleCloudPlatform/testgrid/config"
	apipb "github.com/GoogleCloudPlatform/testgrid/pb/api/v1"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/gorilla/mux"
)

// findDashboardTab locates dashboard tab in config
func findDashboardTab(cfg *configpb.Configuration, dashboardKey string, tabKey string) (*configpb.DashboardTab, error) {
	if cfg == nil {
		return nil, errors.New("invalid config")
	}
	for _, dashboard := range cfg.Dashboards {
		if config.Normalize(dashboard.Name) == dashboardKey {
			for _, tab := range dashboard.DashboardTab {
				if config.Normalize(tab.Name) == tabKey {
					return tab, nil
				}
			}
			return nil, fmt.Errorf("Dashboard {%q} not found", dashboardKey)
		}
	}
	return nil, fmt.Errorf("Tab {%q} not found", tabKey)
}

// GroupGrid fetch tab group name grid info (columns, rows, ..etc)
func (s Server) GroupGrid(configPath *gcs.Path, groupName string) (*statepb.Grid, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.Timeout)
	defer cancel()
	groupPath, err := configPath.ResolveReference(&url.URL{Path: path.Join(s.GridPathPrefix, groupName)})
	grid, _, err := gcs.DownloadGrid(ctx, s.Client, *groupPath)
	if err != nil {
		return nil, fmt.Errorf("load %s: %v", groupName, err)
	}
	return grid, err
}

// Grid fetch tab and grid info (columns, rows, ..etc)
func (s Server) Grid(r *http.Request, cfg *configpb.Configuration, dashboardKey string, tabKey string) (*statepb.Grid, error) {
	tab, err := findDashboardTab(cfg, dashboardKey, tabKey)
	if err != nil {
		return nil, err
	}
	configPath, _, err := s.configPath(r)
	if err != nil {
		return nil, err
	}
	grid, err := s.GroupGrid(configPath, tab.TestGroupName)
	if err != nil {
		return nil, err
	}
	return grid, nil
}

// decodeRLE decodes the run length encoded data
//   [0, 3, 5, 4] -> [0, 0, 0, 5, 5, 5, 5]
func decodeRLE(encodedData []int32) []int32 {
	var decodedResult []int32
	encodedDataLength := len(encodedData)
	if encodedDataLength%2 == 0 {
		for encodedDataIdx := 0; encodedDataIdx < encodedDataLength; encodedDataIdx += 2 {
			for cellRepeatCount := encodedData[encodedDataIdx+1]; cellRepeatCount > 0; cellRepeatCount-- {
				decodedResult = append(decodedResult, encodedData[encodedDataIdx])
			}
		}
	}
	return decodedResult
}

// ListHeaders returns dashboard tab headers
// Response json: ListHeadersResponse
func (s Server) ListHeaders(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cfg := s.getConfig(w, r)
	if cfg == nil {
		return
	}

	dashboardKey, tabKey := vars["dashboard"], vars["tab"]
	grid, err := s.Grid(r, cfg, dashboardKey, tabKey)
	if err != nil {
		http.Error(w, fmt.Sprintf("Dashboard {%q} or tab {%q} not found", dashboardKey, tabKey), http.StatusNotFound)
		return
	}
	if grid == nil {
		http.Error(w, "Grid not found.", http.StatusNotFound)
		return
	}

	var dashboardTabResponse apipb.ListHeadersResponse
	for _, gColumn := range grid.Columns {
		// TODO(#683): Remove timestamp conversion math
		millis := gColumn.Started
		sec := millis / 1000
		nanos := math.Mod(millis, 1000) * 1e6
		column := apipb.ListHeadersResponse_Header{
			Name:  gColumn.Name,
			Build: gColumn.Build,
			Started: &timestamp.Timestamp{
				Seconds: int64(sec),
				Nanos:   int32(nanos),
			},
			Extra:      gColumn.Extra,
			HotlistIds: gColumn.HotlistIds,
		}
		dashboardTabResponse.Headers = append(dashboardTabResponse.Headers, &column)
	}
	writeJSON(w, &dashboardTabResponse)
}

// ListRows returns dashboard tab rows
// Response json: ListRowsResponse
func (s Server) ListRows(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cfg := s.getConfig(w, r)
	if cfg == nil {
		return
	}

	dashboardKey, tabKey := vars["dashboard"], vars["tab"]
	grid, err := s.Grid(r, cfg, dashboardKey, tabKey)
	if err != nil {
		http.Error(w, fmt.Sprintf("Dashboard {%q} or tab {%q} not found", dashboardKey, tabKey), http.StatusNotFound)
		return
	}
	if grid == nil {
		http.Error(w, "Grid not found.", http.StatusNotFound)
		return
	}

	var dashboardTabResponse apipb.ListRowsResponse
	for _, gRow := range grid.Rows {
		row := apipb.ListRowsResponse_Row{
			Name:   gRow.Name,
			Issues: gRow.Issues,
			Alert:  gRow.AlertInfo,
		}
		cellsCount := len(gRow.CellIds)
		gRowDecodedResults := decodeRLE(gRow.Results)
		// loop through CellIds, Messages, Icons slices and build cell struct objects
		for cellIdx := 0; cellIdx < cellsCount; cellIdx++ {
			cell := apipb.ListRowsResponse_Cell{
				Result:  gRowDecodedResults[cellIdx],
				CellId:  gRow.CellIds[cellIdx],
				Message: gRow.Messages[cellIdx],
				Icon:    gRow.Icons[cellIdx],
			}
			row.Cells = append(row.Cells, &cell)
		}
		dashboardTabResponse.Rows = append(dashboardTabResponse.Rows, &row)
	}
	writeJSON(w, &dashboardTabResponse)
}
