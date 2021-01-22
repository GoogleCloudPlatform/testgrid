/*
Copyright 2021 The Kubernetes Authors.

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

// Package config merges together multiple configurations.
package config

import (
	"fmt"
	"sort"

	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/golang/protobuf/proto"
)

// Zero-memory member for hash sets
type void struct{}

var insert void

// Converge merges together the Configurations given in the map set.
// If there are duplicate entries, the string key may be added as a prefix to
// maintain uniqueness for Dashboards, DashboardGroups, and TestGroups.
// The config at key "" will not be modified.
//
// The output protobuf will pass config.Validate if all its inputs pass config.Validate
func Converge(shards map[string]*configpb.Configuration) (*configpb.Configuration, error) {
	if len(shards) == 0 {
		return nil, fmt.Errorf("No configurations to converge")
	}

	result := configpb.Configuration{}

	// Go through shards in key string order, so the prefix results are predictable and alphabetical.
	var keys []string
	for key := range shards {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Dashboards and Dashboard Groups can't share names with each other
	DashboardsAndGroups := make(map[string]void)
	TestGroups := make(map[string]void)

	for _, key := range keys {
		cfg := shards[key]
		NewDashboardsAndGroups := make(map[string]void)
		NewTestGroups := make(map[string]void)
		for _, testgroup := range cfg.TestGroups {
			NewTestGroups[testgroup.Name] = insert
		}
		for _, dashboard := range cfg.Dashboards {
			NewDashboardsAndGroups[dashboard.Name] = insert
		}
		for _, dashboardGroup := range cfg.DashboardGroups {
			NewDashboardsAndGroups[dashboardGroup.Name] = insert
		}

		dashboardRenames := negotiateConversions(key, DashboardsAndGroups, NewDashboardsAndGroups)
		testGroupRenames := negotiateConversions(key, TestGroups, NewTestGroups)

		for olddash, newdash := range dashboardRenames {
			RenameDashboardOrGroup(olddash, newdash, cfg)
		}

		for oldtest, newtest := range testGroupRenames {
			RenameTestGroup(oldtest, newtest, cfg)
		}

		// Merge protos and cached sets
		proto.Merge(&result, cfg)

		for _, dash := range cfg.Dashboards {
			DashboardsAndGroups[dash.Name] = insert
		}
		for _, group := range cfg.DashboardGroups {
			DashboardsAndGroups[group.Name] = insert
		}
		for _, test := range cfg.TestGroups {
			TestGroups[test.Name] = insert
		}
	}

	return &result, nil
}

// Given two sets of strings, returns a "conversions" for each duplicate.
// If there are no duplicates, returns a zero-length map.
// If there are duplicates, returns a map of the string in "new" -> the string it should become.
// Will try "prefix-string" first, then "prefix-2-string", etc.
func negotiateConversions(prefix string, original, new map[string]void) map[string]string {
	if len(original) == 0 || len(new) == 0 {
		return nil
	}

	result := make(map[string]string)

	for key := range new {
		if _, exists := original[key]; exists {
			candidate := fmt.Sprintf("%s-%s", prefix, key)
			attempt := 1
			for true {
				_, existInOld := original[candidate]
				_, existInNew := new[candidate]
				if !existInOld && !existInNew {
					result[key] = candidate
					break
				}
				attempt++
				candidate = fmt.Sprintf("%s-%d-%s", prefix, attempt, key)
			}
		}
	}

	return result
}

// RenameTestGroup renames all references to TestGroup 'original' to 'new'.
// Does not verify if the new name is already taken.
func RenameTestGroup(original, new string, cfg *configpb.Configuration) *configpb.Configuration {
	// If a TestGroup is changed, it needs to be changed for tabs that reference it also
	for _, testGroup := range cfg.TestGroups {
		if testGroup.Name == original {
			testGroup.Name = new
		}
	}
	for _, dashboard := range cfg.Dashboards {
		for _, dashTab := range dashboard.DashboardTab {
			if dashTab.TestGroupName == original {
				dashTab.TestGroupName = new
			}
		}
	}
	return cfg
}

// RenameDashboardOrGroup renames all Dashboards and DashboardGroups named 'original' to 'new'.
// Since Dashboards and Dashboard Groups can't share names with each other, a valid Configuration will rename at most one item.
func RenameDashboardOrGroup(original, new string, cfg *configpb.Configuration) *configpb.Configuration {
	return RenameDashboard(original, new, RenameDashboardGroup(original, new, cfg))
}

// RenameDashboard renames all references to Dashboard 'original' to 'new'.
// Does not verify if the new name is already taken.
func RenameDashboard(original, new string, cfg *configpb.Configuration) *configpb.Configuration {
	// If a dashboard is renamed, it needs to be changed for DashboardGroups that reference it also
	for _, dashboard := range cfg.Dashboards {
		if dashboard.Name == original {
			dashboard.Name = new
		}
	}

	for _, dashgroup := range cfg.DashboardGroups {
		for i, dashboard := range dashgroup.DashboardNames {
			if dashboard == original {
				dashgroup.DashboardNames[i] = new
			}
		}
	}

	return cfg
}

// RenameDashboardGroup renames all references to DashboardGroup 'original' to 'new'.
// Does not verify if the new name is already taken.
func RenameDashboardGroup(original, new string, cfg *configpb.Configuration) *configpb.Configuration {
	for _, dashgroup := range cfg.DashboardGroups {
		if dashgroup.Name == original {
			dashgroup.Name = new
		}
	}

	return cfg
}
