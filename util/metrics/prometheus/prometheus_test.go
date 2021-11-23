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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestInt64Set(t *testing.T) {
	cases := []struct {
		name   string
		fields []string
		sets   []map[int64][]string
		want   map[string]float64
	}{
		{
			name: "zero",
			want: map[string]float64{},
		},
		{
			name:   "basic",
			fields: []string{"component"},
			sets: []map[int64][]string{
				{64: {"updater"}},
			},
			want: map[string]float64{
				"updater": float64(64),
			},
		},
		{
			name:   "fields",
			fields: []string{"component", "source"},
			sets: []map[int64][]string{
				{64: {"updater", "prow"}},
			},
			want: map[string]float64{
				"updater|prow": float64(64),
			},
		},
		{
			name:   "serial values",
			fields: []string{"component"},
			sets: []map[int64][]string{
				{64: {"updater"}},
				{32: {"updater"}},
			},
			want: map[string]float64{
				"updater": float64(32),
			},
		},
		{
			name:   "complex",
			fields: []string{"component", "source"},
			sets: []map[int64][]string{
				{
					64: {"updater", "prow"},
					66: {"updater", "google"},
					32: {"summarizer", "google"},
				},
				{
					64: {"updater", "prow"},
					22: {"summarizer", "google"},
				},
			},
			want: map[string]float64{
				"updater|prow":      float64(64),
				"updater|google":    float64(66),
				"summarizer|google": float64(22),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mName := strings.Replace(tc.name, " ", "_", -1) + "_int"
			m := NewInt64(mName, "fake desc", tc.fields...)
			var wg sync.WaitGroup
			for _, set := range tc.sets {
				for n, fields := range set {
					wg.Add(1)
					go func(n int64, fields []string) {
						m.Set(n, fields...)
						m.(Valuer).Values() // Set and Values must be able to run concurrently
						wg.Done()
					}(n, fields)
				}
				wg.Wait()
			}
			got := m.(Valuer).Values()
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("Set() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCounterAdd(t *testing.T) {
	cases := []struct {
		name   string
		fields []string
		adds   []map[int64][]string
		want   map[string]float64
	}{
		{
			name: "zero",
			want: map[string]float64{},
		},
		{
			name:   "basic",
			fields: []string{"component"},
			adds: []map[int64][]string{
				{
					12: {"updater"},
				},
			},
			want: map[string]float64{
				"updater": float64(12),
			},
		},
		{
			name:   "fields",
			fields: []string{"component", "source"},
			adds: []map[int64][]string{
				{64: {"updater", "prow"}},
			},
			want: map[string]float64{
				"updater|prow": float64(64),
			},
		},
		{
			name:   "concurrent values",
			fields: []string{"component"},
			adds: []map[int64][]string{
				{
					32: {"updater"},
					64: {"updater"},
				},
				{32: {"updater"}},
			},
			want: map[string]float64{
				"updater": float64(32 + 32 + 64),
			},
		},
		{
			name:   "fields and values",
			fields: []string{"component", "source"},
			adds: []map[int64][]string{
				{64: {"updater", "prow"}},
				{32: {"updater", "prow"}},
			},
			want: map[string]float64{
				"updater|prow": float64(64 + 32),
			},
		},
		{
			name:   "complex",
			fields: []string{"component", "source"},
			adds: []map[int64][]string{
				{
					64: {"updater", "prow"},
					66: {"updater", "google"},
					32: {"summarizer", "google"},
					22: {"summarizer", "google"},
				},
			},
			want: map[string]float64{
				"updater|prow":      float64(64),
				"updater|google":    float64(66),
				"summarizer|google": float64(32 + 22),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mName := strings.Replace(tc.name, " ", "_", -1) + "_counter"
			m := NewCounter(mName, "fake desc", tc.fields...)
			var wg sync.WaitGroup
			for _, add := range tc.adds {
				for n, values := range add {
					wg.Add(1)
					go func(n int64, values []string) {
						m.Add(n, values...)
						m.(Valuer).Values() // Add and Values must be able to run concurrently
						wg.Done()
					}(n, values)
				}
				wg.Wait()
			}
			got := m.(Valuer).Values()
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("Add() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDurationSet(t *testing.T) {
	cases := []struct {
		name   string
		fields []string
		sets   []map[time.Duration][]string
		want   map[string]float64
	}{
		{
			name: "zero",
			want: map[string]float64{},
		},
		{
			name:   "basic",
			fields: []string{"component"},
			sets: []map[time.Duration][]string{
				{(1100 * time.Millisecond): {"updater"}},
			},
			want: map[string]float64{
				"updater": float64(1.1),
			},
		},
		{
			name:   "fields",
			fields: []string{"component", "source"},
			sets: []map[time.Duration][]string{
				{time.Second: {"updater", "prow"}},
			},
			want: map[string]float64{
				"updater|prow": float64(1),
			},
		},
		{
			name:   "serial values",
			fields: []string{"component"},
			sets: []map[time.Duration][]string{
				{time.Second: {"updater"}},
				{time.Minute: {"updater"}},
			},
			want: map[string]float64{
				"updater": float64(60),
			},
		},
		{
			name:   "complex",
			fields: []string{"component", "source"},
			sets: []map[time.Duration][]string{
				{
					time.Second:      {"updater", "prow"},
					time.Minute:      {"updater", "google"},
					time.Millisecond: {"summarizer", "google"},
				},
				{
					time.Second:       {"updater", "prow"},
					(2 * time.Second): {"summarizer", "google"},
				},
			},
			want: map[string]float64{
				"updater|prow":      float64(1),
				"updater|google":    float64(60),
				"summarizer|google": float64(2),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mName := strings.Replace(tc.name, " ", "_", -1) + "_duration"
			m := NewDuration(mName, "fake desc", tc.fields...)
			var wg sync.WaitGroup
			for _, set := range tc.sets {
				for n, fields := range set {
					wg.Add(1)
					go func(n time.Duration, fields []string) {
						m.Clock(n, fields...)
						m.(Valuer).Values() // Set and Values must be able to run concurrently
						wg.Done()
					}(n, fields)
				}
				wg.Wait()
			}
			got := m.(Valuer).Values()
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("Set() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}
