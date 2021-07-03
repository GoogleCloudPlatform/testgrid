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

package config

import (
	"container/heap"
	"context"
	"sync"
	"testing"
	"time"

	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestInit(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name   string
		q      *TestGroupQueue
		groups []*configpb.TestGroup
		when   time.Time

		next []*configpb.TestGroup
	}{
		{
			name: "add",
			q:    &TestGroupQueue{},
			groups: []*configpb.TestGroup{
				{
					Name: "hi",
				},
			},
			when: now,

			next: []*configpb.TestGroup{
				{
					Name: "hi",
				},
			},
		},
		{
			name: "remove",
			q: func() *TestGroupQueue {
				var q TestGroupQueue
				q.Init([]*configpb.TestGroup{
					{
						Name: "drop",
					},
					{
						Name: "keep",
					},
				}, now)
				return &q
			}(),
			groups: []*configpb.TestGroup{
				{
					Name: "keep",
				},
				{
					Name: "add",
				},
			},
			when: now.Add(-time.Minute),

			next: []*configpb.TestGroup{
				{
					Name: "add",
				},
				{
					Name: "keep",
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.q.Init(tc.groups, tc.when)

			var got []*configpb.TestGroup
			for range tc.next {
				got = append(got, heap.Pop(&tc.q.queue).(*item).tg)
			}
			if diff := cmp.Diff(tc.next, got, protocmp.Transform()); diff != "" {
				t.Errorf("FixAll() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFixAll(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name  string
		q     *TestGroupQueue
		fixes map[string]time.Time

		next []*configpb.TestGroup
		err  bool
	}{
		{
			name: "empty",
			q:    &TestGroupQueue{},
		},
		{
			name: "basic",
			q: func() *TestGroupQueue {
				var q TestGroupQueue
				q.Init([]*configpb.TestGroup{
					{
						Name: "first-now-second",
					},
					{
						Name: "second-now-fifth",
					},
					{
						Name: "third",
					},
					{
						Name: "fourth-now-first",
					},
					{
						Name: "fifth-now-fourth",
					},
				}, now)
				return &q
			}(),
			fixes: map[string]time.Time{
				"fourth-now-first": now.Add(-2 * time.Minute),
				"first-now-second": now.Add(-time.Minute),
				"second-now-fifth": now.Add(2 * time.Minute),
				"fifth-now-fourth": now.Add(time.Minute),
			},

			next: []*configpb.TestGroup{
				{
					Name: "fourth-now-first",
				},
				{
					Name: "first-now-second",
				},
				{
					Name: "third",
				},
				{
					Name: "fifth-now-fourth",
				},
				{
					Name: "second-now-fifth",
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.q.FixAll(tc.fixes); (err != nil) != tc.err {
				t.Errorf("FixAll() got unexpected error %v, wanted err=%t", err, tc.err)
			}
			var got []*configpb.TestGroup
			for range tc.next {
				got = append(got, heap.Pop(&tc.q.queue).(*item).tg)
			}
			if diff := cmp.Diff(tc.next, got, protocmp.Transform()); diff != "" {
				t.Errorf("FixAll() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFix(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		fix  string
		when time.Time
		q    *TestGroupQueue

		next []*configpb.TestGroup
		err  bool
	}{
		{
			name: "missing",
			fix:  "missing",
			q:    &TestGroupQueue{},
			err:  true,
		},
		{
			name: "basic",
			fix:  "basic",
			q: func() *TestGroupQueue {
				var q TestGroupQueue
				q.Init([]*configpb.TestGroup{
					{
						Name: "basic",
					},
					{
						Name: "was-later-now-first",
					},
				}, now)
				return &q
			}(),
			when: now.Add(time.Minute),
			next: []*configpb.TestGroup{
				{
					Name: "was-later-now-first",
				},
				{
					Name: "basic",
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.q.Fix(tc.fix, tc.when); (err != nil) != tc.err {
				t.Errorf("Fix() got unexpected error %v, wanted err=%t", err, tc.err)
			}
			var got []*configpb.TestGroup
			for range tc.next {
				got = append(got, heap.Pop(&tc.q.queue).(*item).tg)
			}
			if diff := cmp.Diff(tc.next, got, protocmp.Transform()); diff != "" {
				t.Errorf("Fix() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestStatus(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		q    *TestGroupQueue

		depth int
		next  *configpb.TestGroup
		when  time.Time
	}{
		{
			name: "empty",
			q:    &TestGroupQueue{},
		},
		{
			name: "single",
			q: func() *TestGroupQueue {
				var q TestGroupQueue
				q.Init([]*configpb.TestGroup{
					{
						Name: "hi",
					},
				}, now)
				return &q
			}(),
			depth: 1,
			next: &configpb.TestGroup{
				Name: "hi",
			},
			when: now,
		},
		{
			name: "multi",
			q: func() *TestGroupQueue {
				var q TestGroupQueue
				q.Init([]*configpb.TestGroup{
					{
						Name: "hi",
					},
					{
						Name: "middle",
					},
					{
						Name: "there",
					},
				}, now)
				q.Fix("middle", now.Add(-time.Minute))
				return &q
			}(),
			depth: 3,
			next: &configpb.TestGroup{
				Name: "middle",
			},
			when: now.Add(-time.Minute),
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
	cases := []struct {
		name      string
		q         *TestGroupQueue
		receivers func(context.Context) (context.Context, chan<- *configpb.TestGroup, func() []*configpb.TestGroup)
		freq      time.Duration

		want []*configpb.TestGroup
	}{
		{
			name: "empty",
			q:    &TestGroupQueue{},
			receivers: func(ctx context.Context) (context.Context, chan<- *configpb.TestGroup, func() []*configpb.TestGroup) {
				ch := make(chan *configpb.TestGroup)
				go func() {
					for {
						select {
						case tg := <-ch:
							t.Fatalf("Send() receiver got unexpected group: %v", tg)
						case <-ctx.Done():
							return
						}
					}
				}()

				return ctx, ch, func() []*configpb.TestGroup { return nil }
			},
		},
		{
			name: "empty loop",
			q:    &TestGroupQueue{},
			receivers: func(ctx context.Context) (context.Context, chan<- *configpb.TestGroup, func() []*configpb.TestGroup) {
				ch := make(chan *configpb.TestGroup)
				ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
				go func() {
					for {
						select {
						case tg := <-ch:
							t.Fatalf("Send() receiver got unexpected group: %v", tg)
						case <-ctx.Done():
							cancel()
							return
						}
					}
				}()

				return ctx, ch, func() []*configpb.TestGroup { return nil }
			},
			freq: time.Microsecond,
		},
		{
			name: "single",
			q: func() *TestGroupQueue {
				var q TestGroupQueue
				q.Init([]*configpb.TestGroup{
					{
						Name: "hi",
					},
				}, time.Now())
				return &q
			}(),
			receivers: func(ctx context.Context) (context.Context, chan<- *configpb.TestGroup, func() []*configpb.TestGroup) {
				ch := make(chan *configpb.TestGroup)
				var wg sync.WaitGroup
				wg.Add(1)
				var got []*configpb.TestGroup
				ctx, cancel := context.WithCancel(ctx)
				go func() {
					defer wg.Done()
					for {
						select {
						case tg := <-ch:
							got = append(got, tg)
							cancel()
						case <-ctx.Done():
							cancel()
							return
						}
					}
				}()

				return ctx, ch, func() []*configpb.TestGroup {
					wg.Wait()
					return got
				}
			},
			want: []*configpb.TestGroup{
				{
					Name: "hi",
				},
			},
		},
		{
			name: "single loop",
			q: func() *TestGroupQueue {
				var q TestGroupQueue
				q.Init([]*configpb.TestGroup{
					{
						Name: "hi",
					},
				}, time.Now())
				return &q
			}(),
			receivers: func(ctx context.Context) (context.Context, chan<- *configpb.TestGroup, func() []*configpb.TestGroup) {
				ch := make(chan *configpb.TestGroup)
				var wg sync.WaitGroup
				wg.Add(1)
				var got []*configpb.TestGroup
				ctx, cancel := context.WithCancel(ctx)
				go func() {
					defer wg.Done()
					for {
						select {
						case tg := <-ch:
							got = append(got, tg)
							if len(got) == 3 {
								cancel()
							}
						case <-ctx.Done():
							cancel()
							return
						}
					}
				}()

				return ctx, ch, func() []*configpb.TestGroup {
					wg.Wait()
					return got
				}
			},
			freq: time.Microsecond,
			want: []*configpb.TestGroup{
				{
					Name: "hi",
				},
				{
					Name: "hi",
				},
				{
					Name: "hi",
				},
			},
		},
		{
			name: "multi",
			q: func() *TestGroupQueue {
				var q TestGroupQueue
				q.Init([]*configpb.TestGroup{
					{
						Name: "hi",
					},
					{
						Name: "there",
					},
				}, time.Now())
				return &q
			}(),
			receivers: func(ctx context.Context) (context.Context, chan<- *configpb.TestGroup, func() []*configpb.TestGroup) {
				ch := make(chan *configpb.TestGroup)
				var wg sync.WaitGroup
				wg.Add(1)
				var got []*configpb.TestGroup
				ctx, cancel := context.WithCancel(ctx)
				go func() {
					defer wg.Done()
					for {
						select {
						case tg := <-ch:
							got = append(got, tg)
							if len(got) == 2 {
								cancel()
							}
						case <-ctx.Done():
							cancel()
							return
						}
					}
				}()

				return ctx, ch, func() []*configpb.TestGroup {
					wg.Wait()
					return got
				}
			},
			want: []*configpb.TestGroup{
				{
					Name: "hi",
				},
				{
					Name: "there",
				},
			},
		},
		{
			name: "multi loop",
			q: func() *TestGroupQueue {
				var q TestGroupQueue
				q.Init([]*configpb.TestGroup{
					{
						Name: "hi",
					},
					{
						Name: "there",
					},
				}, time.Now())
				return &q
			}(),
			receivers: func(ctx context.Context) (context.Context, chan<- *configpb.TestGroup, func() []*configpb.TestGroup) {
				ch := make(chan *configpb.TestGroup)
				var wg sync.WaitGroup
				wg.Add(1)
				var got []*configpb.TestGroup
				ctx, cancel := context.WithCancel(ctx)
				go func() {
					defer wg.Done()
					for {
						select {
						case tg := <-ch:
							got = append(got, tg)
							if len(got) == 6 {
								cancel()
							}
						case <-ctx.Done():
							cancel()
							return
						}
					}
				}()

				return ctx, ch, func() []*configpb.TestGroup {
					wg.Wait()
					return got
				}
			},
			freq: time.Microsecond,
			want: []*configpb.TestGroup{
				{
					Name: "hi",
				},
				{
					Name: "there",
				},
				{
					Name: "hi",
				},
				{
					Name: "there",
				},
				{
					Name: "hi",
				},
				{
					Name: "there",
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			ctx, channel, get := tc.receivers(ctx)
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
		want  []*configpb.TestGroup
	}{
		{
			name: "basic",
		},
		{
			name: "single",
			items: []*item{
				{
					tg: &configpb.TestGroup{
						Name: "hi",
					},
				},
			},
			want: []*configpb.TestGroup{
				{
					Name: "hi",
				},
			},
		},
		{
			name: "desc",
			items: []*item{
				{
					tg: &configpb.TestGroup{
						Name: "young",
					},
					when: time.Now(),
				},
				{
					tg: &configpb.TestGroup{
						Name: "old",
					},
					when: time.Now().Add(-time.Hour),
				},
			},
			want: []*configpb.TestGroup{
				{
					Name: "old",
				},
				{
					Name: "young",
				},
			},
		},
		{
			name: "asc",
			items: []*item{
				{
					tg: &configpb.TestGroup{
						Name: "old",
					},
					when: time.Now().Add(-time.Hour),
				},
				{
					tg: &configpb.TestGroup{
						Name: "young",
					},
					when: time.Now(),
				},
			},
			want: []*configpb.TestGroup{
				{
					Name: "old",
				},
				{
					Name: "young",
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pq := priorityQueue(tc.items)
			heap.Init(&pq)
			var got []*configpb.TestGroup
			for i, w := range tc.want {
				g := pq.peek().tg
				if diff := cmp.Diff(w, g, protocmp.Transform()); diff != "" {
					t.Errorf("%d peek() got unexpected diff (-want +got):\n%s", i, diff)
				}
				got = append(got, heap.Pop(&pq).(*item).tg)
			}
			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("priorityQueue() got unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}
