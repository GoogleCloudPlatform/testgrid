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
	"fmt"
	"regexp"
	"strings"
)

func translateAtom(simpleAtom string) (string, error) {
	if simpleAtom == "" {
		return "", nil
	}
	// For now, we expect an atom with the exact form `target:"<target>"`
	// Split the `key:value` atom.
	parts := strings.SplitN(simpleAtom, ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("unrecognized atom %q", simpleAtom)
	}
	key := strings.TrimSpace(parts[0])
	val := strings.Trim(strings.TrimSpace(parts[1]), `"`)

	switch {
	case key == "target":
		return fmt.Sprintf(`id.target_id="%s"`, val), nil
	default:
		return "", fmt.Errorf("unrecognized atom key %q", key)
	}
}

var (
	queryRe = regexp.MustCompile(`^target:".*"$`)
)

func TranslateQuery(simpleQuery string) (string, error) {
	if simpleQuery == "" {
		return "", nil
	}
	// For now, we expect a query with a single atom, with the exact form `target:"<target>"`
	if !queryRe.MatchString(simpleQuery) {
		return "", fmt.Errorf("invalid query %q: must match %q", simpleQuery, queryRe.String())
	}
	query, err := translateAtom(simpleQuery)
	if err != nil {
		return "", fmt.Errorf("invalid query %q: %v", simpleQuery, err)
	}
	return query, nil
}
