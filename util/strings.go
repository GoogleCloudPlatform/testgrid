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

package util

import (
	"strings"
)

// Strings represents the value of a flag that accept multiple strings.
type Strings struct {
	vals []string
}

// Strings returns the slice of strings set for this value instance.
func (s *Strings) Strings() []string {
	return s.vals
}

// String returns a concatenated string of all the values joined by commas.
func (s *Strings) String() string {
	return strings.Join(s.vals, ",")
}

// Set records the value passed
func (s *Strings) Set(value string) error {
	s.vals = append(s.vals, value)
	return nil
}
