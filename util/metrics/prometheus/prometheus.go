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

	"github.com/GoogleCloudPlatform/testgrid/util/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
)

// Valuer extends a metric to include a report on its values.
type Valuer interface {
	metrics.Metric
	Values() map[string]float64
}

type int64Metric struct {
	name   string
	fields map[string]bool
	met    *prometheus.GaugeVec
}

// init sets up the Prometheus endpoint at this port and URL when the package is imported
func init() {
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		http.ListenAndServe(":2112", nil)
	}()
}

// NewInt64 creates and registers an Int64 metric with Prometheus.
func NewInt64(name, desc string, fields ...string) metrics.Int64 {
	m := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: name,
		Help: desc,
	}, fields)
	prometheus.MustRegister(m)
	return &int64Metric{
		name:   name,
		fields: make(map[string]bool),
		met:    m,
	}
}

// Name returns the metric's name.
func (m *int64Metric) Name() string {
	return m.name
}

// Set the metric's value to the given number.
func (m *int64Metric) Set(n int64, fields ...string) {
	m.met.WithLabelValues(fields...).Set(float64(n))
	m.fields[strings.Join(fields, "|")] = true
}

func gaugeValue(metric *prometheus.GaugeVec, labels ...string) float64 {
	var m = &dto.Metric{}
	if err := metric.WithLabelValues(labels...).Write(m); err != nil {
		return 0
	}
	return m.Gauge.GetValue()
}

// Values returns each field and its current value.
func (m *int64Metric) Values() map[string]float64 {
	values := make(map[string]float64)
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
		fields: make(map[string]bool),
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
	values := make(map[string]float64)
	for fieldStr := range m.fields {
		fields := strings.Split(fieldStr, "|")
		values[fieldStr] = counterValue(m.met, fields...)
	}
	return values
}
