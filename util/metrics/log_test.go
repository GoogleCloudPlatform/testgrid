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
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
)

func TestInt64Set(t *testing.T) {
	cases := []struct {
		name   string
		fields []string
		sets   []map[int64][]string
		want   map[string]map[string]interface{}
	}{
		{
			name: "zero",
			want: map[string]map[string]interface{}{},
		},
		{
			name:   "basic",
			fields: []string{"component"},
			sets: []map[int64][]string{
				{64: {"updater"}},
			},
			want: map[string]map[string]interface{}{
				"component": {
					"updater": mean{[]int64{64}},
				},
			},
		},
		{
			name:   "fields",
			fields: []string{"component", "source"},
			sets: []map[int64][]string{
				{64: {"updater", "prow"}},
			},
			want: map[string]map[string]interface{}{
				"component": {
					"updater": mean{[]int64{64}},
				},
				"source": {
					"prow": mean{[]int64{64}},
				},
			},
		},
		{
			name:   "values",
			fields: []string{"component"},
			sets: []map[int64][]string{
				{64: {"updater"}},
				{32: {"updater"}},
			},
			want: map[string]map[string]interface{}{
				"component": {
					"updater": mean{[]int64{64, 32}},
				},
			},
		},
		{
			name:   "fields and values",
			fields: []string{"component", "source"},
			sets: []map[int64][]string{
				{64: {"updater", "prow"}},
				{32: {"updater", "prow"}},
			},
			want: map[string]map[string]interface{}{
				"component": {
					"updater": mean{[]int64{64, 32}},
				},
				"source": {
					"prow": mean{[]int64{64, 32}},
				},
			},
		},
		{
			name:   "complex",
			fields: []string{"component", "source"},
			sets: []map[int64][]string{
				{64: {"updater", "prow"}},
				{66: {"updater", "google"}},
				{32: {"summarizer", "google"}},
			},
			want: map[string]map[string]interface{}{
				"component": {
					"updater":    mean{[]int64{64, 66}},
					"summarizer": mean{[]int64{32}},
				},
				"source": {
					"prow":   mean{[]int64{64}},
					"google": mean{[]int64{66, 32}},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewLogInt64("fake metric", "fake desc", logrus.WithField("nane", tc.name), tc.fields...)
			for _, set := range tc.sets {
				for n, fields := range set {
					m.Set(n, fields...)
				}
			}
			got := m.Values()
			if diff := cmp.Diff(tc.want, got, cmp.AllowUnexported(mean{}, gauge{})); diff != "" {
				t.Errorf("Set() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCounterAdd(t *testing.T) {
	when := time.Now().Add(-10 * time.Minute)
	cases := []struct {
		name   string
		fields []string
		adds   []map[int64][]string
		want   map[string]map[string]interface{}
	}{
		{
			name: "zero",
			want: map[string]map[string]interface{}{},
		},
		{
			name:   "basic",
			fields: []string{"component"},
			adds: []map[int64][]string{
				{
					12: {"updater"},
				},
			},
			want: map[string]map[string]interface{}{
				"component": {"updater": gauge{12, 10 * time.Minute}},
			},
		},
		{
			name:   "fields",
			fields: []string{"component", "source"},
			adds: []map[int64][]string{
				{64: {"updater", "prow"}},
			},
			want: map[string]map[string]interface{}{
				"component": {
					"updater": gauge{64, 10 * time.Minute},
				},
				"source": {
					"prow": gauge{64, 10 * time.Minute},
				},
			},
		},
		{
			name:   "values",
			fields: []string{"component"},
			adds: []map[int64][]string{
				{64: {"updater"}},
				{32: {"updater"}},
			},
			want: map[string]map[string]interface{}{
				"component": {
					"updater": gauge{64 + 32, 10 * time.Minute},
				},
			},
		},
		{
			name:   "fields and values",
			fields: []string{"component", "source"},
			adds: []map[int64][]string{
				{64: {"updater", "prow"}},
				{32: {"updater", "prow"}},
			},
			want: map[string]map[string]interface{}{
				"component": {
					"updater": gauge{64 + 32, 10 * time.Minute},
				},
				"source": {
					"prow": gauge{64 + 32, 10 * time.Minute},
				},
			},
		},
		{
			name:   "complex",
			fields: []string{"component", "source"},
			adds: []map[int64][]string{
				{64: {"updater", "prow"}},
				{66: {"updater", "google"}},
				{32: {"summarizer", "google"}},
			},
			want: map[string]map[string]interface{}{
				"component": {
					"updater":    gauge{64 + 66, 10 * time.Minute},
					"summarizer": gauge{32, 10 * time.Minute},
				},
				"source": {
					"prow":   gauge{64, 10 * time.Minute},
					"google": gauge{66 + 32, 10 * time.Minute},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewLogCounter("fake metric", "fake desc", logrus.WithField("name", tc.name), tc.fields...)
			m.(*logCounter).last = when
			for _, add := range tc.adds {
				for n, values := range add {
					m.Add(n, values...)
				}
			}

			got := m.Values()
			for _, got := range got {
				for key, value := range got {
					g := value.(gauge)
					g.dur = g.dur.Round(time.Minute)
					got[key] = g
				}
			}
			if diff := cmp.Diff(tc.want, got, cmp.AllowUnexported(gauge{})); diff != "" {
				t.Errorf("Add() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}
