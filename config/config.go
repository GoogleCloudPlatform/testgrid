/*
Copyright 2019 The Kubernetes Authors.

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

package config

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/golang/protobuf/proto"

	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	multierror "github.com/hashicorp/go-multierror"
)

// MissingFieldError is an error that includes the missing field.
type MissingFieldError struct {
	Field string
}

func (e MissingFieldError) Error() string {
	return fmt.Sprintf("field missing or unset: %s", e.Field)
}

// DuplicateNameError is an error that includes the duplicate name.
type DuplicateNameError struct {
	Name   string
	Entity string
}

func (e DuplicateNameError) Error() string {
	return fmt.Sprintf("found duplicate name after normalizing: (%s) %s", e.Entity, e.Name)
}

type MissingEntityError struct {
	Name   string
	Entity string
}

func (e MissingEntityError) Error() string {
	return fmt.Sprintf("could not find the referenced (%s) %s", e.Entity, e.Name)
}

type ConfigError struct {
	Name    string
	Entity  string
	Message string
}

func (e ConfigError) Error() string {
	return fmt.Sprintf("configuration error for (%s) %s: %s", e.Entity, e.Name, e.Message)
}

// normalize lowercases, and removes all non-alphanumeric characters from a string.
func normalize(s string) string {
	regex := regexp.MustCompile("[^a-zA-Z0-9]+")
	s = regex.ReplaceAllString(s, "")
	s = strings.ToLower(s)
	return s
}

// validateUnique checks that a list has no duplicate normalized entries.
func validateUnique(items []string, entity string) error {
	mErr := &multierror.Error{}
	set := map[string]bool{}
	for _, item := range items {
		s := normalize(item)
		_, ok := set[s]
		if ok {
			mErr = multierror.Append(mErr, DuplicateNameError{s, entity})
		} else {
			set[s] = true
		}
	}
	return mErr.ErrorOrNil()
}

func validateAllUnique(c configpb.Configuration) error {
	mErr := &multierror.Error{}
	var tgNames []string
	for idx, tg := range c.TestGroups {
		if tg.Name == "" {
			mErr = multierror.Append(mErr, fmt.Errorf("TestGroup %d has no name", idx))
			continue
		}
		tgNames = append(tgNames, tg.Name)
	}
	// Test Group names must be unique.
	err := validateUnique(tgNames, "TestGroup")
	if err != nil {
		mErr = multierror.Append(mErr, err)
	}

	var dashNames []string
	for idx, dash := range c.Dashboards {
		if dash.Name == "" {
			mErr = multierror.Append(mErr, fmt.Errorf("Dashboard %d has no name", idx))
			continue
		}
		dashNames = append(dashNames, dash.Name)
		var tabNames []string
		for tIdx, tab := range dash.DashboardTab {
			if tab.Name == "" {
				mErr = multierror.Append(mErr, fmt.Errorf("Tab %d in %s has no name", tIdx, tab.Name))
				continue
			}
			tabNames = append(tabNames, tab.Name)
		}
		// Dashboard Tab names must be unique within a Dashboard.
		err = validateUnique(tabNames, "DashboardTab")
		if err != nil {
			mErr = multierror.Append(mErr, err)
		}
	}
	// Dashboard names must be unique within Dashboards.
	err = validateUnique(dashNames, "Dashboard")
	if err != nil {
		mErr = multierror.Append(mErr, err)
	}

	var dgNames []string
	for idx, dg := range c.DashboardGroups {
		if dg.Name == "" {
			mErr = multierror.Append(mErr, fmt.Errorf("DashboardGroup %d has no name", idx))
			continue
		}
		dgNames = append(dgNames, dg.Name)
	}
	// Dashboard Group names must be unique within Dashboard Groups.
	err = validateUnique(dgNames, "DashboardGroup")
	if err != nil {
		mErr = multierror.Append(mErr, err)
	}

	// Names must also be unique within DashboardGroups AND Dashbaords.
	err = validateUnique(append(dashNames, dgNames...), "Dashboard/DashboardGroup")
	if err != nil {
		mErr = multierror.Append(mErr, err)
	}

	return mErr.ErrorOrNil()
}

func validateReferencesExist(c configpb.Configuration) error {
	mErr := &multierror.Error{}

	tgNames := map[string]bool{}
	for _, tg := range c.TestGroups {
		tgNames[tg.Name] = true

		for idx, header := range tg.ColumnHeader {
			if cv, p, l := header.ConfigurationValue, header.Property, header.Label; cv == "" && p == "" && l == "" {
				mErr = multierror.Append(mErr, fmt.Errorf("Column Header %d in group %s is empty", idx, tg.Name))
			} else if cv != "" && (p != "" || l != "") || p != "" && (cv != "" || l != "") {
				mErr = multierror.Append(mErr, fmt.Errorf("Column Header %d in group %s must only set one value, got configuration_value: %q, property: %q, label: %q", idx, tg.Name, cv, p, l))
			}

		}
		if tg.TestNameConfig != nil {
			if tg.TestNameConfig.NameFormat == "" {
				mErr = multierror.Append(mErr, fmt.Errorf("Group %s has an empty name format", tg.Name))
			}

			if got, want := len(tg.TestNameConfig.NameElements), strings.Count(tg.TestNameConfig.NameFormat, "%"); got != want {
				mErr = multierror.Append(mErr, fmt.Errorf("Group %s TestNameConfig has %d elements, format %s wants %d", tg.Name, got, tg.TestNameConfig.NameFormat, want))
			}
		}
	}
	tgInTabs := map[string]bool{}
	for _, dash := range c.Dashboards {
		for _, tab := range dash.DashboardTab {
			tabTg := tab.TestGroupName
			tgInTabs[tabTg] = true
			// Verify that each Test Group referenced by a Dashboard Tab exists.
			if _, ok := tgNames[tabTg]; !ok {
				mErr = multierror.Append(mErr, MissingEntityError{tabTg, "TestGroup"})
			}
		}
	}
	// Likewise, each Test Group must be referenced by a Dashboard Tab, so each Test Group gets displayed.
	for tgName := range tgNames {
		if _, ok := tgInTabs[tgName]; !ok {
			mErr = multierror.Append(mErr, ConfigError{tgName, "TestGroup", "Each Test Group must be referenced by at least 1 Dashboard Tab."})
		}
	}

	dashNames := map[string]bool{}
	for _, dash := range c.Dashboards {
		dashNames[dash.Name] = true
	}
	dashToDg := map[string]bool{}
	for _, dg := range c.DashboardGroups {
		for _, name := range dg.DashboardNames {
			dgDash := name
			if _, ok := dashNames[dgDash]; !ok {
				// The Dashboards each Dashboard Group references must exist.
				mErr = multierror.Append(mErr, MissingEntityError{dgDash, "Dashboard"})
			} else if _, ok = dashToDg[dgDash]; ok {
				mErr = multierror.Append(mErr, ConfigError{dgDash, "Dashboard", "A Dashboard cannot be in more than 1 Dashboard Group."})
			} else {
				dashToDg[dgDash] = true
			}
		}
	}
	return mErr.ErrorOrNil()
}

// Validate checks that a configuration is well-formed.
func Validate(c configpb.Configuration) error {
	mErr := &multierror.Error{}

	// TestGrid requires at least 1 TestGroup and 1 Dashboard in order to do anything.
	if len(c.TestGroups) == 0 {
		return multierror.Append(mErr, MissingFieldError{"TestGroups"})
	}
	if len(c.Dashboards) == 0 {
		return multierror.Append(mErr, MissingFieldError{"Dashboards"})
	}

	// Names have to be unique (after normalizing) within types of entities, to prevent storing
	// duplicate state on updates and confusion between similar names.
	err := validateAllUnique(c)
	if err != nil {
		mErr = multierror.Append(mErr, err)
	}

	// The entity that an entity references must exist.
	err = validateReferencesExist(c)
	if err != nil {
		mErr = multierror.Append(mErr, err)
	}

	return mErr.ErrorOrNil()
}

// Unmarshal reads a protocol buffer into memory
func Unmarshal(r io.Reader) (*configpb.Configuration, error) {
	buf, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %v", err)
	}
	var cfg configpb.Configuration
	if err = proto.Unmarshal(buf, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse: %v", err)
	}
	return &cfg, nil
}

// MarshalText writes a text version of the parsed configuration to the supplied io.Writer.
// Returns an error if config is invalid or writing failed.
func MarshalText(c configpb.Configuration, w io.Writer) error {
	if err := Validate(c); err != nil {
		return err
	}
	return proto.MarshalText(w, &c)
}

// MarshalBytes returns the wire-encoded protobuf data for the parsed configuration.
// Returns an error if config is invalid or encoding failed.
func MarshalBytes(c configpb.Configuration) ([]byte, error) {
	if err := Validate(c); err != nil {
		return nil, err
	}
	return proto.Marshal(&c)
}

// ReadGCS reads the config from gcs and unmarshals it into a Configuration struct.
func ReadGCS(ctx context.Context, obj *storage.ObjectHandle) (*configpb.Configuration, error) {
	r, err := obj.NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to open config: %v", err)
	}
	return Unmarshal(r)
}

// ReadPath reads the config from the specified local file path.
func ReadPath(path string) (*configpb.Configuration, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %v", err)
	}
	return Unmarshal(f)
}

// Read will read the Configuration proto message from a local or gs:// path.
//
// The ctx and client are only relevant when path refers to GCS.
func Read(path string, ctx context.Context, client *storage.Client) (*configpb.Configuration, error) {
	if strings.HasPrefix(path, "gs://") {
		var gcsPath gcs.Path
		if err := gcsPath.Set(path); err != nil {
			return nil, fmt.Errorf("bad gcs path: %v", err)
		}
		return ReadGCS(ctx, client.Bucket(gcsPath.Bucket()).Object(gcsPath.Object()))
	}
	return ReadPath(path)
}

func FindTestGroup(name string, cfg *configpb.Configuration) *configpb.TestGroup {
	for _, tg := range cfg.TestGroups {
		if tg.Name == name {
			return tg
		}
	}
	return nil
}

func FindDashboard(name string, cfg *configpb.Configuration) *configpb.Dashboard {
	for _, d := range cfg.Dashboards {
		if d.Name == name {
			return d
		}
	}
	return nil
}
