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
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	cfgutil "github.com/GoogleCloudPlatform/testgrid/config"
	"github.com/GoogleCloudPlatform/testgrid/pb/config"
	"sigs.k8s.io/yaml"
)

// getDefaults take all paths found through seeking, returns list of dirs with defaults
func getDefaults(allPaths []string) (defaults []string, err error) {
	dirsFound := make(map[string]bool)
	for _, path := range allPaths {
		if filepath.Base(path) == "default.yaml" || filepath.Base(path) == "default.yml" {
			if _, ok := dirsFound[filepath.Dir(path)]; ok {
				return nil, fmt.Errorf("two default files found in dir %q", filepath.Dir(path))
			}
			defaults = append(defaults, path)
			dirsFound[filepath.Dir(path)] = true
		}
	}
	return defaults, nil
}

// seekDefaults finds all default files and returns a map of directory to its default contents.
// TODO: Implement filesystem fake in order to unit test this better.
func seekDefaults(paths []string) (map[string]DefaultConfiguration, error) {
	defaultFiles := make(map[string]DefaultConfiguration)
	var allPaths []string
	err := SeekYAMLFiles(paths, func(path string, info os.FileInfo) error {
		allPaths = append(allPaths, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("unable to walk paths, %v", err)
	}
	defaults, err := getDefaults(allPaths)
	if err != nil {
		return nil, fmt.Errorf("unable to get defaults, %v", err)
	}
	for _, path := range defaults {
		b, err := ioutil.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read default at %s: %v", path, err)
		}
		curDefault, err := LoadDefaults(b)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize default at %s: %v", path, err)
		}
		defaultFiles[filepath.Dir(path)] = curDefault
	}
	return defaultFiles, nil
}

// pathDefault returns the closest DefaultConfiguration for a path (default in path's dir, or overall default).
func pathDefault(path string, defaultFiles map[string]DefaultConfiguration, defaults DefaultConfiguration) DefaultConfiguration {
	if localDefaults, ok := defaultFiles[filepath.Dir(path)]; ok {
		return localDefaults
	}
	return defaults
}

// ReadConfig takes multiple source paths of the following form:
//   If path is a local file, then the file will be parsed as YAML
//   If path is a directory, then all files and directories within it will be parsed.
//     If this directory has a default(s).yaml file, apply it to all configured entities,
// 		 after applying defaults from defaultPath.
// Optionally, defaultPath points to default setting YAML
// Returns a configuration proto containing the data from all of those sources
func ReadConfig(paths []string, defaultpath string, strict bool) (*config.Configuration, error) {
	// Get the overall default file, if specified.
	var defaults DefaultConfiguration
	if defaultpath != "" {
		b, err := ioutil.ReadFile(defaultpath)
		if err != nil {
			return nil, fmt.Errorf("failed to read default at %s: %v", defaultpath, err)
		}
		defaults, err = LoadDefaults(b)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize default at %s: %v", defaultpath, err)
		}
	}

	// Find all default files, map their directory to their contents.
	defaultFiles, err := seekDefaults(paths)
	if err != nil {
		return nil, err
	}

	// Gather configuration from each YAML file, applying the config's default.yaml if
	// one exists in its directory, or the overall default otherwise.
	var result *config.Configuration
	err = SeekYAMLFiles(paths, func(path string, info os.FileInfo) error {
		if filepath.Base(path) == "default.yaml" || filepath.Base(path) == "default.yml" {
			return nil
		}
		// Read YAML file and Update config
		b, err := ioutil.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %v", path, err)
		}
		localDefaults := pathDefault(path, defaultFiles, defaults)
		if result, err = Update(result, b, &localDefaults, strict); err != nil {
			return fmt.Errorf("failed to merge %s into config: %v", path, err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("SeekYAMLFiles(%v), gathering config: %v", paths, err)
	}

	return result, err
}

// Update reads the config in yamlData and updates the config in c.
// If reconcile is non-nil, it will pad out new entries with those default settings
func Update(cfg *config.Configuration, yamlData []byte, reconcile *DefaultConfiguration, strict bool) (*config.Configuration, error) {
	newConfig := &config.Configuration{}
	if strict {
		if err := yaml.UnmarshalStrict(yamlData, newConfig); err != nil {
			return nil, err
		}
	} else {
		if err := yaml.Unmarshal(yamlData, newConfig); err != nil {
			return nil, err
		}
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

	return cfg, nil
}

// MarshalYAML returns a YAML file representing the parsed configuration.
// Returns an error if config is invalid or encoding failed.
func MarshalYAML(c *config.Configuration) ([]byte, error) {
	if c == nil {
		return nil, errors.New("got an empty config.Configuration")
	}
	if err := cfgutil.Validate(c); err != nil {
		return nil, err
	}
	bytes, err := yaml.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("could not write config to yaml: %v", err)
	}
	return bytes, nil
}

// DefaultConfiguration describes a default configuration that should be applied before other configs.
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

// LoadDefaults reads and validates default settings from YAML
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

// SeekYAMLFiles walks through paths and directories, calling the passed function on each YAML file
// future modifications to what Configurator sees as a "config file" can be made here
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

			// Only YAML files will be parsed
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
