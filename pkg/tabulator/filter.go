/*
Copyright 2022 The TestGrid Authors.

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

// Package tabulator processes test group state into tab state.
package tabulator

import (
	"fmt"
	"net/url"
	"regexp"

	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
)

const (
	includeFilter = "include-filter-by-regex"
	excludeFilter = "exclude-filter-by-regex"
)

// filterGrid filters the grid to rows matching the include/exclude list in the base options
func filterGrid(baseOptions string, rows []*statepb.Row) ([]*statepb.Row, error) {

	vals, err := url.ParseQuery(baseOptions)
	if err != nil {
		return nil, fmt.Errorf("parse %q: %v", baseOptions, err)
	}

	for _, include := range vals[includeFilter] {
		if rows, err = includeRows(rows, include); err != nil {
			return nil, fmt.Errorf("bad %s=%s: %v", includeFilter, include, err)
		}
	}

	for _, exclude := range vals[excludeFilter] {
		if rows, err = excludeRows(rows, exclude); err != nil {
			return nil, fmt.Errorf("bad %s=%s: %v", excludeFilter, exclude, err)
		}
	}

	// TODO(chases2): drop columns that are now empty due to filters
	// TODO(fejta): grouping, which is not used by testgrid.k8s.io
	// TODO(fejta): sorting, unused by testgrid.k8s.io
	// TODO(fejta): graph, unused by testgrid.k8s.io
	// TODO(fejta): tabuluar, unused by testgrid.k8s.io
	return rows, nil
}

// includeRows returns the subset of rows that match the regex
func includeRows(in []*statepb.Row, include string) ([]*statepb.Row, error) {
	re, err := regexp.Compile(include)
	if err != nil {
		return nil, err
	}
	var rows []*statepb.Row
	for _, r := range in {
		if !re.MatchString(r.Name) {
			continue
		}
		rows = append(rows, r)
	}
	return rows, nil
}

// excludeRows returns the subset of rows that do not match the regex
func excludeRows(in []*statepb.Row, exclude string) ([]*statepb.Row, error) {
	re, err := regexp.Compile(exclude)
	if err != nil {
		return nil, err
	}
	var rows []*statepb.Row
	for _, r := range in {
		if re.MatchString(r.Name) {
			continue
		}
		rows = append(rows, r)
	}
	return rows, nil
}
