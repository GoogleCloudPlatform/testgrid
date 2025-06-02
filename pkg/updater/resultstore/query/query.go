/*
Copyright 2024 The TestGrid Authors.

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

package query

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type keyValue struct {
	key   string
	value string
}

func translateAtom(simpleAtom keyValue, queryTarget bool) (string, error) {
	if simpleAtom.key == "" {
		return "", errors.New("missing key")
	}
	if simpleAtom.value == "" {
		return "", errors.New("missing value")
	}
	switch {
	case simpleAtom.key == "label" && queryTarget:
		return fmt.Sprintf(`invocation.invocation_attributes.labels:"%s"`, simpleAtom.value), nil
	case simpleAtom.key == "label":
		return fmt.Sprintf(`invocation_attributes.labels:"%s"`, simpleAtom.value), nil
	case simpleAtom.key == "target":
		return fmt.Sprintf(`id.target_id="%s"`, simpleAtom.value), nil
	default:
		return "", fmt.Errorf("unknown type of atom %q", simpleAtom.key)
	}
}

var (
	// Captures any atoms of the form `label:"<label>"` or `target:"<target>"`.
	atomReStr = `(?P<atom>(?P<key>label|target):"(?P<value>.+?)")`
	atomRe    = regexp.MustCompile(atomReStr)
	// A query can only have atoms (above), separated by spaces.
	queryRe = regexp.MustCompile(`^(` + atomReStr + ` *)+$`)
)

// TranslateQuery translates a simple query (similar to the syntax of searching invocations in the
// UI) to a query for searching via API.
// More at https://github.com/googleapis/googleapis/blob/master/google/devtools/resultstore/v2/resultstore_download.proto.
//
// This expects a query consisting of any number of space-separated `label:"<label>"` or
// `target:"<target>"` atoms.
func TranslateQuery(simpleQuery string) (string, error) {
	if simpleQuery == "" {
		return "", nil
	}
	if !queryRe.MatchString(simpleQuery) {
		return "", fmt.Errorf("query must consist of only space-separated `label:\"<label>\"` or `target:\"<target>\"` atoms")
	}
	var simpleAtoms []keyValue
	var queryTarget bool
	matches := atomRe.FindAllStringSubmatch(simpleQuery, -1)
	for _, match := range matches {
		if len(match) != 4 {
			return "", fmt.Errorf("atom %v: want 4 submatches (full match, atom, key, value), but got %d", match, len(match))
		}
		simpleAtoms = append(simpleAtoms, keyValue{match[2], match[3]})
		if match[2] == "target" {
			queryTarget = true
		}
	}
	var atoms []string
	for _, simpleAtom := range simpleAtoms {
		atom, err := translateAtom(simpleAtom, queryTarget)
		if err != nil {
			return "", fmt.Errorf("atom %v: %v", simpleAtom, err)
		}
		atoms = append(atoms, atom)
	}
	return strings.Join(atoms, " "), nil
}
