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

package metrics

import (
	"github.com/sirupsen/logrus"
)

type logInt64 struct {
	name   string
	desc   string
	fields []string
	log    logrus.FieldLogger
	// TODO: Record different values for different specified fields.
	val int64
}

// NewLogInt64 creates a new Int64 metric that logs.
func NewLogInt64(name, desc string, log logrus.FieldLogger, fields ...string) Int64 {
	return &logInt64{
		name:   name,
		desc:   desc,
		log:    log,
		fields: fields,
	}
}

// Name returns the metric's name.
func (m *logInt64) Name() string {
	return m.name
}

func (m *logInt64) Val() int64 {
	return m.val
}

// Set the metric's value to the given number.
func (m *logInt64) Set(n int64, fields ...string) {
	log := m.log
	if len(fields) != len(m.fields) {
		log.Errorf("wrong number of fields; want %d (%v), got %d (%v)", len(m.fields), m.fields, len(fields), fields)
		return
	}
	for i, field := range fields {
		log.WithField(m.fields[i], field)
	}
	m.val += n
	log.Infof("int64 %q.Set(%d) = %d", m.Name(), n, m.val)
}

type logCounter struct {
	name   string
	desc   string
	fields []string
	log    logrus.FieldLogger
	// TODO: Record different values for different specified fields.
	val int64
}

// NewLogCounter creates a new counter that logs.
func NewLogCounter(name, desc string, log logrus.FieldLogger, fields ...string) Counter {
	return &logCounter{
		name:   name,
		desc:   desc,
		log:    log,
		fields: fields,
	}
}

// Name returns the metric's name.
func (m *logCounter) Name() string {
	return m.name
}

func (m *logCounter) Val() int64 {
	return m.val
}

// Add the given number to the counter.
func (m *logCounter) Add(n int64, fields ...string) {
	log := m.log
	if len(fields) != len(m.fields) {
		log.Errorf("wrong number of fields; want %d (%v), got %d (%v)", len(m.fields), m.fields, len(fields), fields)
		return
	}
	if n < 0 {
		log.Errorf("n must be positive, got %d", n)
		return
	}
	for i, field := range fields {
		log.WithField(m.fields[i], field)
	}
	m.val += n
	log.Infof("counter %q.Add(%d) = %d", m.Name(), n, m.val)
}
