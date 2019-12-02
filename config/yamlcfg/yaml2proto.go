/*
Copyright 2016 The Kubernetes Authors.

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

package yamlcfg

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	cfgutil "github.com/GoogleCloudPlatform/testgrid/config"
	"github.com/GoogleCloudPlatform/testgrid/pb/config"
	"sigs.k8s.io/yaml"
)

// Takes multiple source paths of the following form:
//   If path is a local file, then the file will be parsed as YAML
//   If path is a directory, then all files and directories within it will be parsed.
// Optionally, defaultPath points to default setting YAML
// Returns a configuration proto containing the data from all of those sources
func ReadConfig(paths []string, defaultpath string) (config.Configuration, error) {

	var result config.Configuration

	var defaults DefaultConfiguration
	if defaultpath != "" {
		b, err := ioutil.ReadFile(defaultpath)
		if err != nil {
			return result, fmt.Errorf("failed to read default at %s: %v", defaultpath, err)
		}
		defaults, err = LoadDefaults(b)
		if err != nil {
			return result, fmt.Errorf("failed to deserialize default at %s: %v", defaultpath, err)
		}
	}

	err := SeekYAMLFiles(paths, func(path string, info os.FileInfo) error {
		// Read YAML file and Update config
		b, err := ioutil.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %v", path, err)
		}
		if err = Update(&result, b, &defaults); err != nil {
			return fmt.Errorf("failed to merge %s into config: %v", path, err)
		}

		return nil
	})

	return result, err
}

// Update reads the config in yamlData and updates the config in c.
// If reconcile is non-nil, it will pad out new entries with those default settings
func Update(cfg *config.Configuration, yamlData []byte, reconcile *DefaultConfiguration) error {

	newConfig := &config.Configuration{}
	if err := yaml.Unmarshal(yamlData, newConfig); err != nil {
		return err
	}

	if cfg == nil {
		cfg = &config.Configuration{}
	}

	for _, testgroup := range newConfig.TestGroups {
		if reconcile != nil {
			ReconcileTestGroup(testgroup, reconcile.DefaultTestGroup)
		}
		cfg.TestGroups = append(cfg.TestGroups, testgroup)
	}

	for _, dashboard := range newConfig.Dashboards {
		if reconcile != nil {
			for _, dashboardtab := range dashboard.DashboardTab {
				ReconcileDashboardTab(dashboardtab, reconcile.DefaultDashboardTab)
			}
		}
		cfg.Dashboards = append(cfg.Dashboards, dashboard)
	}

	for _, dashboardGroup := range newConfig.DashboardGroups {
		cfg.DashboardGroups = append(cfg.DashboardGroups, dashboardGroup)
	}

	return nil
}

// MarshalYAML returns a YAML file representing the parsed configuration.
// Returns an error if config is invalid or encoding failed.
func MarshalYAML(c config.Configuration) ([]byte, error) {
	if err := cfgutil.Validate(c); err != nil {
		return nil, err
	}
	bytes, err := yaml.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("could not write config to yaml: %v", err)
	}
	return bytes, nil
}

type DefaultConfiguration struct {
	// A default testgroup with default initialization data
	DefaultTestGroup *config.TestGroup `json:"default_test_group,omitempty"`
	// A default dashboard tab with default initialization data
	DefaultDashboardTab *config.DashboardTab `json:"default_dashboard_tab,omitempty"`
}

// MissingFieldError is an error that includes the missing field.
type MissingFieldError struct {
	Field string
}

func (e MissingFieldError) Error() string {
	return fmt.Sprintf("field missing or unset: %s", e.Field)
}

// ReconcileTestGroup sets unfilled currentTestGroup fields to the corresponding defaultTestGroup value, if present
func ReconcileTestGroup(currentTestGroup *config.TestGroup, defaultTestGroup *config.TestGroup) {
	if currentTestGroup.DaysOfResults == 0 {
		currentTestGroup.DaysOfResults = defaultTestGroup.DaysOfResults
	}

	if currentTestGroup.TestsNamePolicy == config.TestGroup_TESTS_NAME_UNSPECIFIED {
		currentTestGroup.TestsNamePolicy = defaultTestGroup.TestsNamePolicy
	}

	if currentTestGroup.IgnorePending == false {
		currentTestGroup.IgnorePending = defaultTestGroup.IgnorePending
	}

	if currentTestGroup.IgnoreSkip == false {
		currentTestGroup.IgnoreSkip = defaultTestGroup.IgnoreSkip
	}

	if currentTestGroup.ColumnHeader == nil {
		currentTestGroup.ColumnHeader = defaultTestGroup.ColumnHeader
	}

	if currentTestGroup.NumColumnsRecent == 0 {
		currentTestGroup.NumColumnsRecent = defaultTestGroup.NumColumnsRecent
	}

	if currentTestGroup.AlertStaleResultsHours == 0 {
		currentTestGroup.AlertStaleResultsHours = defaultTestGroup.AlertStaleResultsHours
	}

	if currentTestGroup.NumFailuresToAlert == 0 {
		currentTestGroup.NumFailuresToAlert = defaultTestGroup.NumFailuresToAlert
	}
	if currentTestGroup.CodeSearchPath == "" {
		currentTestGroup.CodeSearchPath = defaultTestGroup.CodeSearchPath
	}
	if currentTestGroup.NumPassesToDisableAlert == 0 {
		currentTestGroup.NumPassesToDisableAlert = defaultTestGroup.NumPassesToDisableAlert
	}
	// is_external and user_kubernetes_client should always be true
	currentTestGroup.IsExternal = true
	currentTestGroup.UseKubernetesClient = true
}

// ReconcileDashboardTab sets unfilled currentTab fields to the corresponding defaultTab value, if present
func ReconcileDashboardTab(currentTab *config.DashboardTab, defaultTab *config.DashboardTab) {
	if currentTab.BugComponent == 0 {
		currentTab.BugComponent = defaultTab.BugComponent
	}

	if currentTab.CodeSearchPath == "" {
		currentTab.CodeSearchPath = defaultTab.CodeSearchPath
	}

	if currentTab.NumColumnsRecent == 0 {
		currentTab.NumColumnsRecent = defaultTab.NumColumnsRecent
	}

	if currentTab.OpenTestTemplate == nil {
		currentTab.OpenTestTemplate = defaultTab.OpenTestTemplate
	}

	if currentTab.FileBugTemplate == nil {
		currentTab.FileBugTemplate = defaultTab.FileBugTemplate
	}

	if currentTab.AttachBugTemplate == nil {
		currentTab.AttachBugTemplate = defaultTab.AttachBugTemplate
	}

	if currentTab.ResultsText == "" {
		currentTab.ResultsText = defaultTab.ResultsText
	}

	if currentTab.ResultsUrlTemplate == nil {
		currentTab.ResultsUrlTemplate = defaultTab.ResultsUrlTemplate
	}

	if currentTab.CodeSearchUrlTemplate == nil {
		currentTab.CodeSearchUrlTemplate = defaultTab.CodeSearchUrlTemplate
	}

	if currentTab.AlertOptions == nil {
		currentTab.AlertOptions = defaultTab.AlertOptions
	}

	if currentTab.OpenBugTemplate == nil {
		currentTab.OpenBugTemplate = defaultTab.OpenBugTemplate
	}
}

// UpdateDefaults reads and validates default settings from YAML
// Returns an error if the defaultConfig is partially or completely missing.
func LoadDefaults(yamlData []byte) (DefaultConfiguration, error) {

	var result DefaultConfiguration
	err := yaml.Unmarshal(yamlData, &result)
	if err != nil {
		return result, err
	}

	if result.DefaultTestGroup == nil {
		return result, MissingFieldError{"DefaultTestGroup"}
	}
	if result.DefaultDashboardTab == nil {
		return result, MissingFieldError{"DefaultDashboardTab"}
	}
	return result, nil
}

// walks through paths and directories, calling the passed function on each YAML file
// future modifications to what Configurator sees as a "config file" can be made here
//TODO(chases2) Rewrite so that it walks recursively, not lexically.
func SeekYAMLFiles(paths []string, callFunc func(path string, info os.FileInfo) error) error {
	for _, path := range paths {
		_, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("Failed status call on %s: %v", path, err)
		}

		err = filepath.Walk(path, func(path string, info os.FileInfo, err error) error {

			// A bad file should not stop us from parsing the directory
			if err != nil {
				return nil
			}

			// Only YAML files will be
			if filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
				return nil
			}

			if info.IsDir() {
				return nil
			}

			return callFunc(path, info)
		})

		if err != nil {
			return fmt.Errorf("Failed to walk through %s: %v", path, err)
		}
	}
	return nil
}
