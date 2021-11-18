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

package util

import (
	"container/heap"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestInit(t *testing.T) {
	now := time.Now()
	log := logrus.WithField("test", "TestInit")
	cases := []struct {
		name  string
		q     *Queue
		names []string
		when  time.Time

		next []string
	}{
		{
			name: "add",
			q:    &Queue{},
			names: []string{
				"hi",
			},
			when: now,

			next: []string{"hi"},
		},
		{
			name: "remove",
			q: func() *Queue {
				var q Queue
				q.Init(log, []string{"drop", "keep"}, now)
				return &q
			}(),
			names: []string{
				"keep",
				"add",
			},
			when: now.Add(-time.Minute),
			next: []string{
				"add",
				"keep",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.q.Init(log, tc.names, tc.when)

			var got []string
			for range tc.next {
				got = append(got, heap.Pop(&tc.q.queue).(*item).name)
			}
			if diff := cmp.Diff(tc.next, got, protocmp.Transform()); diff != "" {
				t.Errorf("FixAll() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFixAll(t *testing.T) {
	log := logrus.WithField("test", "TestFixAll")
	now := time.Now()
	cases := []struct {
		name  string
		q     *Queue
		fixes map[string]time.Time
		later bool

		next []string
		err  bool
	}{
		{
			name: "empty",
			q:    &Queue{},
		},
		{
			name: "later",
			q: func() *Queue {
				var q Queue
				q.Init(log, []string{
					"first-now-second",
					"second-now-fifth",
					"third",
					"fourth-now-first",
					"fifth-now-fourth",
				}, now)
				return &q
			}(),
			fixes: map[string]time.Time{
				"fourth-now-first": now.Add(-2 * time.Minute),
				"first-now-second": now.Add(-time.Minute),
				"second-now-fifth": now.Add(2 * time.Minute),
				"fifth-now-fourth": now.Add(time.Minute),
			},
			later: true,

			next: []string{
				"fourth-now-first",
				"first-now-second",
				"third",
				"fifth-now-fourth",
				"second-now-fifth",
			},
		},
		{
			name: "reduce",
			q: func() *Queue {
				var q Queue
				q.Init(log, []string{
					"first-now-second",
					"second-ignored-becomes-fifth",
					"third-becomes-fourth",
					"fourth-now-first",
					"fifth-ignored-becomes-fourth",
				}, now)
				return &q
			}(),
			fixes: map[string]time.Time{
				"fourth-now-first":             now.Add(-2 * time.Minute),
				"first-now-second":             now.Add(-time.Minute),
				"second-ignored-becomes-fifth": now.Add(2 * time.Minute), // noop
				"fifth-ignored-becomes-fourth": now.Add(time.Minute),     // noop
			},
			later: true,

			next: []string{
				"fourth-now-first",
				"first-now-second",
				"third-becomes-fourth",
				"fifth-ignored-becomes-fourth",
				"second-ignored-becomes-fifth",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.q.FixAll(tc.fixes, tc.later); (err != nil) != tc.err {
				t.Errorf("FixAll() got unexpected error %v, wanted err=%t", err, tc.err)
			}
			var got []string
			for range tc.next {
				got = append(got, heap.Pop(&tc.q.queue).(*item).name)
			}
			if diff := cmp.Diff(tc.next, got, protocmp.Transform()); diff != "" {
				t.Errorf("FixAll() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFix(t *testing.T) {
	now := time.Now()
	log := logrus.WithField("test", "TestFix")
	cases := []struct {
		name string

		q     *Queue
		fix   string
		when  time.Time
		later bool

		next []string
		err  bool
	}{
		{
			name: "missing",
			fix:  "missing",
			q:    &Queue{},
			err:  true,
		},
		{
			name: "later",
			fix:  "basic",
			q: func() *Queue {
				var q Queue
				q.Init(log, []string{
					"basic",
					"was-later-now-first",
				}, now)
				return &q
			}(),
			when:  now.Add(time.Minute),
			later: true,
			next: []string{
				"was-later-now-first",
				"basic",
			},
		},
		{
			name: "ignore later",
			fix:  "basic",
			q: func() *Queue {
				var q Queue
				q.Init(log, []string{
					"basic",
					"was-later-still-later",
				}, now)
				return &q
			}(),
			when: now.Add(time.Minute),
			next: []string{
				"basic",
				"was-later-still-later",
			},
		},
		{
			name: "reduce",
			fix:  "basic",
			q: func() *Queue {
				var q Queue
				q.Init(log, []string{
					"was-earlier-now-later",
					"basic",
				}, now)
				return &q
			}(),
			when: now.Add(-time.Minute),
			next: []string{
				"basic",
				"was-earlier-now-later",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.q.Fix(tc.fix, tc.when, tc.later); (err != nil) != tc.err {
				t.Errorf("Fix() got unexpected error %v, wanted err=%t", err, tc.err)
			}
			var got []string
			for range tc.next {
				got = append(got, heap.Pop(&tc.q.queue).(*item).name)
			}
			if diff := cmp.Diff(tc.next, got, protocmp.Transform()); diff != "" {
				t.Errorf("Fix() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestStatus(t *testing.T) {
	log := logrus.WithField("test", "TestStatus")
	pstr := func(s string) *string { return &s }
	now := time.Now()
	cases := []struct {
		name string
		q    *Queue

		depth int
		next  *string
		when  time.Time
	}{
		{
			name: "empty",
			q:    &Queue{},
		},
		{
			name: "single",
			q: func() *Queue {
				var q Queue
				q.Init(log, []string{"hi"}, now)
				return &q
			}(),
			depth: 1,
			next:  pstr("hi"),
			when:  now,
		},
		{
			name: "multi",
			q: func() *Queue {
				var q Queue
				q.Init(log, []string{
					"hi",
					"middle",
					"there",
				}, now)
				q.Fix("middle", now.Add(-time.Minute), true)
				return &q
			}(),
			depth: 3,
			next:  pstr("middle"),
			when:  now.Add(-time.Minute),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			depth, next, when := tc.q.Status()
			if want, got := tc.depth, depth; want != got {
				t.Errorf("Status() wanted depth %d, got %d", want, got)
			}
			if diff := cmp.Diff(tc.next, next, protocmp.Transform()); diff != "" {
				t.Errorf("Status() got unexpected next diff (-want +got):\n%s", diff)
			}
			if !when.Equal(tc.when) {
				t.Errorf("Status() wanted when %v, got %v", tc.when, when)
			}
		})
	}
}

func TestSend(t *testing.T) {
	log := logrus.WithField("test", "TestSend")
	cases := []struct {
		name      string
		q         *Queue
		receivers func(context.Context, *testing.T) (context.Context, chan<- string, func() []string)
		freq      time.Duration

		want []string
	}{
		{
			name: "empty",
			q:    &Queue{log: log},
			receivers: func(ctx context.Context, t *testing.T) (context.Context, chan<- string, func() []string) {
				ch := make(chan string)
				go func() {
					for {
						select {
						case name := <-ch:
							t.Errorf("Send() receiver got unexpected group: %v", name)
							return
						case <-ctx.Done():
							return
						}
					}
				}()

				return ctx, ch, func() []string { return nil }
			},
		},
		{
			name: "empty loop",
			q:    &Queue{log: log},
			receivers: func(ctx context.Context, t *testing.T) (context.Context, chan<- string, func() []string) {
				ch := make(chan string)
				ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
				go func() {
					for {
						select {
						case name := <-ch:
							t.Errorf("Send() receiver got unexpected name: %q", name)
							return
						case <-ctx.Done():
							cancel()
							return
						}
					}
				}()

				return ctx, ch, func() []string { return nil }
			},
			freq: time.Microsecond,
		},
		{
			name: "single",
			q: func() *Queue {
				var q Queue
				q.Init(log, []string{"hi"}, time.Now())
				return &q
			}(),
			receivers: func(ctx context.Context, t *testing.T) (context.Context, chan<- string, func() []string) {
				ch := make(chan string)
				var wg sync.WaitGroup
				wg.Add(1)
				var got []string
				go func() {
					defer wg.Done()
					for {
						select {
						case name := <-ch:
							got = append(got, name)
							return
						case <-ctx.Done():
							return
						}
					}
				}()

				return ctx, ch, func() []string {
					wg.Wait()
					return got
				}
			},
			want: []string{"hi"},
		},
		{
			name: "single loop",
			q: func() *Queue {
				var q Queue
				q.Init(log, []string{"hi"}, time.Now())
				return &q
			}(),
			receivers: func(ctx context.Context, _ *testing.T) (context.Context, chan<- string, func() []string) {
				ch := make(chan string)
				var wg sync.WaitGroup
				wg.Add(1)
				var got []string
				ctx, cancel := context.WithCancel(ctx)
				go func() {
					defer wg.Done()
					for {
						select {
						case name := <-ch:
							got = append(got, name)
							if len(got) == 3 {
								cancel()
							}
						case <-ctx.Done():
							cancel()
							return
						}
					}
				}()

				return ctx, ch, func() []string {
					wg.Wait()
					return got
				}
			},
			freq: time.Microsecond,
			want: []string{
				"hi",
				"hi",
				"hi",
			},
		},
		{
			name: "multi",
			q: func() *Queue {
				var q Queue
				q.Init(log, []string{
					"hi",
					"there",
				}, time.Now())
				return &q
			}(),
			receivers: func(ctx context.Context, _ *testing.T) (context.Context, chan<- string, func() []string) {
				ch := make(chan string)
				var wg sync.WaitGroup
				wg.Add(1)
				var got []string
				go func() {
					defer wg.Done()
					for {
						select {
						case name := <-ch:
							got = append(got, name)
							if len(got) == 2 {
								return
							}
						case <-ctx.Done():
							return
						}
					}
				}()

				return ctx, ch, func() []string {
					wg.Wait()
					return got
				}
			},
			want: []string{
				"hi",
				"there",
			},
		},
		{
			name: "multi loop",
			q: func() *Queue {
				var q Queue
				q.Init(log, []string{"hi", "there"}, time.Now())
				return &q
			}(),
			receivers: func(ctx context.Context, _ *testing.T) (context.Context, chan<- string, func() []string) {
				ch := make(chan string)
				var wg sync.WaitGroup
				wg.Add(1)
				var got []string
				ctx, cancel := context.WithCancel(ctx)
				go func() {
					defer wg.Done()
					for {
						select {
						case name := <-ch:
							got = append(got, name)
							if len(got) == 6 {
								cancel()
							}
						case <-ctx.Done():
							cancel()
							return
						}
					}
				}()

				return ctx, ch, func() []string {
					wg.Wait()
					return got
				}
			},
			freq: time.Microsecond,
			want: []string{
				"hi",
				"there",
				"hi",
				"there",
				"hi",
				"there",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			ctx, channel, get := tc.receivers(ctx, t)
			if err := tc.q.Send(ctx, channel, tc.freq); err != ctx.Err() {
				t.Errorf("Send() returned unexpected error: want %v, got %v", ctx.Err(), err)
			}
			got := get()
			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("Send() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestPriorityQueue(t *testing.T) {
	cases := []struct {
		name  string
		items []*item
		want  []string
	}{
		{
			name: "basic",
		},
		{
			name: "single",
			items: []*item{
				{
					name: "hi",
				},
			},
			want: []string{"hi"},
		},
		{
			name: "desc",
			items: []*item{
				{
					name: "young",
					when: time.Now(),
				},
				{
					name: "old",
					when: time.Now().Add(-time.Hour),
				},
			},
			want: []string{"old", "young"},
		},
		{
			name: "asc",
			items: []*item{
				{
					name: "old",
					when: time.Now().Add(-time.Hour),
				},
				{
					name: "young",
					when: time.Now(),
				},
			},
			want: []string{"old", "young"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pq := priorityQueue(tc.items)
			heap.Init(&pq)
			var got []string
			for i, w := range tc.want {
				g := pq.peek().name
				if diff := cmp.Diff(w, g, protocmp.Transform()); diff != "" {
					t.Errorf("%d peek() got unexpected diff (-want +got):\n%s", i, diff)
				}
				got = append(got, heap.Pop(&pq).(*item).name)
			}
			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("priorityQueue() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}
