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

package prometheus

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/util/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
)

// NewFactory constructs a new metrics factory
func NewFactory() metrics.Factory {
	return metrics.Factory{
		NewInt64:    NewInt64,
		NewCounter:  NewCounter,
		NewDuration: NewDuration,
	}
}

// init sets up the Prometheus endpoint at this port and URL when the package is imported
func init() {
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		http.ListenAndServe(":2112", nil)
	}()
}

// Valuer extends a metric to include a report on its values.
type Valuer interface {
	metrics.Metric
	Values() map[string]float64
}

type gaugeMetric struct {
	name   string
	fields map[string]bool
	met    *prometheus.GaugeVec
	lock   sync.RWMutex
}

func gaugeValue(metric *prometheus.GaugeVec, labels ...string) float64 {
	var m = &dto.Metric{}
	if err := metric.WithLabelValues(labels...).Write(m); err != nil {
		return 0
	}
	return m.Gauge.GetValue()
}

type int64Metric gaugeMetric

// NewInt64 creates and registers an Int64 metric with Prometheus.
func NewInt64(name, desc string, fields ...string) metrics.Int64 {
	m := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: name,
		Help: desc,
	}, fields)
	prometheus.MustRegister(m)
	return &int64Metric{
		name:   name,
		fields: map[string]bool{},
		met:    m,
	}
}

// Name returns the metric's name.
func (m *int64Metric) Name() string {
	return m.name
}

// Set the metric's value to the given number.
func (m *int64Metric) Set(n int64, fields ...string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.met.WithLabelValues(fields...).Set(float64(n))
	m.fields[strings.Join(fields, "|")] = true
}

// Values returns each field and its current value.
func (m *int64Metric) Values() map[string]float64 {
	values := map[string]float64{}
	m.lock.RLock()
	defer m.lock.RUnlock()
	for fieldStr := range m.fields {
		fields := strings.Split(fieldStr, "|")
		values[fieldStr] = gaugeValue(m.met, fields...)
	}
	return values
}

type durationMetric gaugeMetric

// NewDuration returns a prometheus-implemented duration metric
// A GagueVec is used instead of a SummaryVec since it shows changes in duration over time more clearly
func NewDuration(name, desc string, fields ...string) metrics.Duration {
	m := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: name,
		Help: desc,
	}, fields)
	prometheus.MustRegister(m)
	return &durationMetric{
		name:   name,
		fields: map[string]bool{},
		met:    m,
	}
}

func (m *durationMetric) Name() string {
	return m.name
}

// Set sets the metric's value to the given duration in seconds
func (m *durationMetric) Set(t time.Duration, fields ...string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.met.WithLabelValues(fields...).Set(t.Seconds())
	m.fields[strings.Join(fields, "|")] = true
}

// Values returns each field and its current value.
func (m *durationMetric) Values() map[string]float64 {
	values := map[string]float64{}
	m.lock.RLock()
	defer m.lock.RUnlock()
	for fieldStr := range m.fields {
		fields := strings.Split(fieldStr, "|")
		values[fieldStr] = gaugeValue(m.met, fields...)
	}
	return values
}

type counterMetric struct {
	name   string
	fields map[string]bool
	met    *prometheus.CounterVec
	lock   sync.RWMutex
}

// NewCounter creates and registers a strictly-increasing counter metric with Prometheus.
func NewCounter(name, desc string, fields ...string) metrics.Counter {
	m := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: name,
		Help: desc,
	}, fields)
	prometheus.MustRegister(m)
	return &counterMetric{
		name:   name,
		fields: map[string]bool{},
		met:    m,
	}
}

// Name returns the metric's name.
func (m *counterMetric) Name() string {
	return m.name
}

// Add the given number to the Counter.
func (m *counterMetric) Add(n int64, fields ...string) {
	m.met.WithLabelValues(fields...).Add(float64(n))
	m.lock.Lock()
	defer m.lock.Unlock()
	m.fields[strings.Join(fields, "|")] = true
}

func counterValue(metric *prometheus.CounterVec, labels ...string) float64 {
	var m = &dto.Metric{}
	if err := metric.WithLabelValues(labels...).Write(m); err != nil {
		return 0
	}
	return m.Counter.GetValue()
}

// Values returns each field and its current value.
func (m *counterMetric) Values() map[string]float64 {
	values := map[string]float64{}
	m.lock.RLock()
	defer m.lock.RUnlock()
	for fieldStr := range m.fields {
		fields := strings.Split(fieldStr, "|")
		values[fieldStr] = counterValue(m.met, fields...)
	}
	return values
}
