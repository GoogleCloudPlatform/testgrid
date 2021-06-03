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
	"github.com/sirupsen/logrus/hooks/test"
	"testing"
)

func TestInt64Set(t *testing.T) {
	metricName := "testMetric"
	cases := []struct {
		name       string
		n          int64
		fieldNames []string
		fields     []string
		want       string
		err        bool
	}{
		{
			name: "zero",
			want: "int64 \"testMetric\".Set(0)",
		},
		{
			name: "basic",
			n:    3,
			want: "int64 \"testMetric\".Set(3)",
		},
		{
			name:       "fields",
			n:          3,
			fieldNames: []string{"user"},
			fields:     []string{"someone"},
			want:       "int64 \"testMetric\".Set(3)",
		},
		{
			name:       "not enough fields",
			n:          3,
			fieldNames: []string{"user"},
			fields:     []string{},
			err:        true,
		},
		{
			name:       "too many fields",
			n:          3,
			fieldNames: []string{"user"},
			fields:     []string{"someone", "ohno"},
			err:        true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			log, hook := test.NewNullLogger()
			m := NewLogInt64(metricName, "", log, tc.fieldNames...)
			m.Set(tc.n, tc.fields...)
			got := hook.LastEntry().Message
			if tc.err {
				level := hook.LastEntry().Level
				if level != logrus.ErrorLevel {
					t.Errorf("expected an error-level log, got a %s-level log: %v", level, got)
				}
			} else if tc.want != got {
				t.Errorf("mismatched logs; want %q, got %q", tc.want, got)
			}
		})
	}
}

func TestCounterAdd(t *testing.T) {
	metricName := "testMetric"
	cases := []struct {
		name       string
		n          int64
		fieldNames []string
		fields     []string
		want       string
		err        bool
	}{
		{
			name: "zero",
			want: "counter \"testMetric\".Add(0)",
		},
		{
			name: "basic",
			n:    3,
			want: "counter \"testMetric\".Add(3)",
		},
		{
			name:       "fields",
			n:          3,
			fieldNames: []string{"user"},
			fields:     []string{"someone"},
			want:       "counter \"testMetric\".Add(3)",
		},
		{
			name: "negative",
			n:    -3,
			err:  true,
		},
		{
			name:       "not enough fields",
			n:          3,
			fieldNames: []string{"user"},
			fields:     []string{},
			err:        true,
		},
		{
			name:       "too many fields",
			n:          3,
			fieldNames: []string{"user"},
			fields:     []string{"someone", "ohno"},
			err:        true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			log, hook := test.NewNullLogger()
			m := NewLogCounter(metricName, "", log, tc.fieldNames...)
			m.Add(tc.n, tc.fields...)
			got := hook.LastEntry().Message
			if tc.err {
				level := hook.LastEntry().Level
				if level != logrus.ErrorLevel {
					t.Errorf("expected an error-level log, got a %s-level log: %v", level, got)
				}
			} else if tc.want != got {
				t.Errorf("mismatched logs; want %q, got %q", tc.want, got)
			}
		})
	}
}
