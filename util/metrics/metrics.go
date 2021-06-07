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

// Package metrics provides metric reporting for TestGrid components.
package metrics

// Metric contains common metric functions.
type Metric interface {
	Name() string
}

// Int64 is an int64 metric.
type Int64 interface {
	Metric

	Set(int64, ...string)
}

// Counter is a strictly-increasing metric.
type Counter interface {
	Metric

	Add(int64, ...string)
}
