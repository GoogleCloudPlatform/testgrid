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
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"bitbucket.org/creachadair/stringset"
	"github.com/sirupsen/logrus"
)

// Valuer extends a metric to include a report on its values.
type Valuer interface {
	Metric
	Values() map[string]map[string]interface{}
}

// Reporter is a collection of metric values to report.
type Reporter []Valuer

// Report the status of its metrics every freq until the context expires.
func (r *Reporter) Report(ctx context.Context, log logrus.FieldLogger, freq time.Duration) error {
	if log == nil {
		log = logrus.New()
	}
	ticker := time.NewTicker(freq)
	defer ticker.Stop()
	names := stringset.NewSize(len(*r))
	metricMap := make(map[string]int, len(*r))
	for i, m := range *r {
		name := m.Name()
		names.Add(name)
		metricMap[name] = i
	}
	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return ctx.Err()
		}

		for _, name := range names.Elements() {
			i := metricMap[name]
			metric := (*r)[i]
			log := log.WithField("metric", metric.Name())
			for field, values := range metric.Values() {
				log := log.WithField("field", field)
				for value, measurement := range values {
					log = log.WithField(value, measurement)
				}
				log.Info("Current status")
			}
		}
	}
}

// Int64 configures a new Int64 metric to report.
func (r *Reporter) Int64(name, desc string, log logrus.FieldLogger, fields ...string) Int64 {
	current := make([]map[string][]int64, len(fields))
	for i := range fields {
		current[i] = make(map[string][]int64, 1)
	}

	out := &logInt64{
		name:    name,
		desc:    desc,
		log:     log,
		fields:  fields,
		current: current,
	}
	*r = append(*r, out)
	return out
}

// Counter configures a new Counter metric to report
func (r *Reporter) Counter(name, desc string, log logrus.FieldLogger, fields ...string) Counter {
	current := make([]map[string]int64, len(fields))
	previous := make([]map[string]int64, len(fields))
	for i := range fields {
		current[i] = make(map[string]int64, 1)
		previous[i] = make(map[string]int64, 1)
	}

	out := &logCounter{
		name:     name,
		desc:     desc,
		log:      log,
		fields:   fields,
		current:  current,
		previous: previous,
		last:     time.Now(),
	}
	*r = append(*r, out)
	return out
}

type logInt64 struct {
	name    string
	desc    string
	fields  []string
	log     logrus.FieldLogger
	current []map[string][]int64
	lock    sync.Mutex
}

// Name returns the metric's name.
func (m *logInt64) Name() string {
	return m.name
}

// Set the metric's value to the given number.
func (m *logInt64) Set(n int64, fields ...string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	if len(fields) != len(m.fields) {
		m.log.WithFields(logrus.Fields{
			"want": m.fields,
			"got":  fields,
		}).Fatal("Wrong number of fields")
	}
	for i, fieldValue := range fields {
		m.current[i][fieldValue] = append(m.current[i][fieldValue], n)
	}
}

func (m *logInt64) Values() map[string]map[string]interface{} {
	m.lock.Lock()
	defer m.lock.Unlock()

	trend := make(map[string]map[string]interface{}, len(m.current))
	for i, field := range m.fields {
		current := m.current[i]
		if len(current) == 0 {
			continue
		}
		trend[field] = make(map[string]interface{}, len(current))
		for fieldValue, measurements := range current {

			trend[field][fieldValue] = mean{measurements}
			delete(current, fieldValue)
		}

	}
	return trend
}

type mean struct {
	values []int64
}

func (m mean) String() string {
	tot := len(m.values)
	switch tot {
	case 0:
		return "0 values"
	case 1:
		return strconv.FormatInt(m.values[0], 10)
	}
	var val float64
	n := float64(tot)
	for _, v := range m.values {
		val += (float64(v) / n)
	}
	return fmt.Sprintf("%s average (%d values)", strconv.FormatFloat(val, 'g', 3, 64), tot)
}

type logCounter struct {
	name     string
	desc     string
	fields   []string
	log      logrus.FieldLogger
	current  []map[string]int64
	previous []map[string]int64
	last     time.Time
	lock     sync.Mutex
}

// Name returns the metric's name.
func (m *logCounter) Name() string {
	return m.name
}

// Add the given number to the counter.
func (m *logCounter) Add(n int64, fields ...string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	if len(fields) != len(m.fields) {
		m.log.WithFields(logrus.Fields{
			"want": m.fields,
			"got":  fields,
		}).Fatal("Wrong number of fields")
	}
	if n < 0 {
		m.log.WithField("value", n).Fatal("Negative value are invalid")
	}
	for i, fieldValue := range fields {
		m.current[i][fieldValue] += n
	}
}

func (m *logCounter) Values() map[string]map[string]interface{} {
	m.lock.Lock()
	defer m.lock.Unlock()

	dur := time.Since(m.last)
	m.last = time.Now()
	rate := make(map[string]map[string]interface{}, len(m.current))
	for i, field := range m.fields {
		current := m.current[i]
		previous := m.previous[i]
		rate[field] = make(map[string]interface{}, len(current))
		for fieldValue, now := range current {
			delta := now - previous[fieldValue]
			rate[field][fieldValue] = gauge{now, delta, dur}
			previous[fieldValue] = now
		}
	}
	return rate
}

type gauge struct {
	total int64
	delta int64
	dur   time.Duration
}

func (g gauge) qps() string {
	s := g.dur.Seconds()
	if s == 0 {
		return "0 per second"
	}
	qps := float64(g.delta) / s
	if qps == 0 {
		return "0 per second"
	}
	if qps > 0.5 {
		return fmt.Sprintf("%.2f per second", qps)
	}
	seconds := time.Duration(1 / qps * float64(time.Second))
	seconds = seconds.Round(time.Millisecond)
	return fmt.Sprintf("once per %s", seconds)
}

func (g gauge) String() string {
	return fmt.Sprintf("%s (%d total)", g.qps(), g.total)
}
