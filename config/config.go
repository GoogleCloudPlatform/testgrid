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
	Name string
}

func (e DuplicateNameError) Error() string {
	return fmt.Sprintf("found duplicate name after normalizing: %s", e.Name)
}

// normalize lowercases, and removes all non-alphanumeric characters from a string.
func normalize(s string) string {
	regex := regexp.MustCompile("[^a-zA-Z0-9]+")
	s = regex.ReplaceAllString(s, "")
	s = strings.ToLower(s)
	return s
}

// validateUnique checks that a list has no duplicate entries.
func validateUnique(items []string) error {
	mErr := &multierror.Error{}
	set := map[string]bool{}
	for _, s := range items {
		_, ok := set[s]
		if ok {
			mErr = multierror.Append(mErr, DuplicateNameError{s})
		} else {
			set[s] = true
		}
	}
	return mErr.ErrorOrNil()
}

// Validate checks that a configuration is well-formed.
func Validate(c configpb.Configuration) error {
	mErr := &multierror.Error{}

	// TestGrid requires at least 1 TestGroup and 1 Dashboard in order to do anything.
	if len(c.TestGroups) == 0 {
		return MissingFieldError{"TestGroups"}
	}
	if len(c.Dashboards) == 0 {
		return MissingFieldError{"Dashboards"}
	}

	// Names have to be unique (after normalizing) within types of entities, to prevent storing
	// duplicate state on updates and confusion between similar names.
	tgNames := []string{}
	for _, tg := range c.TestGroups {
		tgNames = append(tgNames, normalize(tg.GetName()))
	}
	// Test Group names must be unique.
	err := validateUnique(tgNames)
	if err != nil {
		mErr = multierror.Append(mErr, err)
	}

	dashNames := []string{}
	for _, dash := range c.Dashboards {
		dashNames = append(dashNames, normalize(dash.GetName()))
		tabNames := []string{}
		for _, tab := range dash.GetDashboardTab() {
			tabNames = append(tabNames, normalize(tab.GetName()))
		}
		// Dashboard Tab names must be unique within a Dashboard.
		err = validateUnique(tabNames)
		if err != nil {
			mErr = multierror.Append(mErr, err)
		}
	}
	// Dashboard names must be unique within Dashboards.
	err = validateUnique(dashNames)
	if err != nil {
		mErr = multierror.Append(mErr, err)
	}

	dgNames := []string{}
	for _, dg := range c.DashboardGroups {
		dgNames = append(dgNames, normalize(dg.GetName()))
	}
	// Dashboard Group names must be unique within Dashboard Groups.
	err = validateUnique(dgNames)
	if err != nil {
		mErr = multierror.Append(mErr, err)
	}

	// Names must also be unique within DashboardGroups AND Dashbaords.
	err = validateUnique(append(dashNames, dgNames...))
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
