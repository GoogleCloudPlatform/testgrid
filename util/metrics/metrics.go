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

import "time"

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

// Duration is a metric describing a length of time
type Duration interface {
	Metric
	Set(time.Duration, ...string)
}

// Factory is a collection of functions that create metrics
type Factory struct {
	NewInt64    func(name, desc string, fields ...string) Int64
	NewCounter  func(name, desc string, fields ...string) Counter
	NewDuration func(name, desc string, fields ...string) Duration
}

// NewCyclic derives a cycle metric from the given metrics
func (f Factory) NewCyclic(componentName string) Cyclic {
	fields := []string{"component"}
	return Cyclic{
		errors:       f.NewCounter("errors", "Number of failed updates", fields...),
		skips:        f.NewCounter("skips", "Number of skipped updates", fields...),
		successes:    f.NewCounter("successes", "Number of successful updates", fields...),
		cycleSeconds: f.NewDuration("cycle_duration", "Seconds required to complete an update", fields...),
		fields:       []string{componentName},
	}
}

// Cyclic is a collection of metrics that measures how long a task takes to complete
type Cyclic struct {
	fields       []string
	errors       Counter
	skips        Counter
	successes    Counter
	cycleSeconds Duration
}

// Start returns a PeriodicReporter that logs metrics when one of its methods are called.
func (p *Cyclic) Start() *CycleReporter {
	if p == nil {
		return nil
	}
	return &CycleReporter{metric: p, when: time.Now()}
}

// CycleReporter reports the status of the task that spawned it and how long it took.
type CycleReporter struct {
	metric *Cyclic
	when   time.Time
}

func (pr *CycleReporter) done() {
	if pr == nil || pr.metric.cycleSeconds == nil {
		return
	}
	pr.metric.cycleSeconds.Set(time.Since(pr.when), pr.metric.fields...)
}

// Success reports success
func (pr *CycleReporter) Success() {
	pr.done()
	if pr == nil || pr.metric.successes == nil {
		return
	}
	pr.metric.successes.Add(1, pr.metric.fields...)
}

// Fail reports a failure or unexpected error
func (pr *CycleReporter) Fail() {
	pr.done()
	if pr == nil || pr.metric.errors == nil {
		return
	}
	pr.metric.errors.Add(1, pr.metric.fields...)
}

// Skip reports a cycle that was skipped due to an expected condition, flag, or configuration
func (pr *CycleReporter) Skip() {
	pr.done()
	if pr == nil || pr.metric.skips == nil {
		return
	}
	pr.metric.skips.Add(1, pr.metric.fields...)
}
