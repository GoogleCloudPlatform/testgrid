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
	"context"
	"sync"
	"testing"
	"time"

	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestStatus(t *testing.T) {
	now := time.Now()
	log := logrus.WithField("test", "TestStatus")
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
				q.Init(log, []*configpb.TestGroup{
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
				q.Init(log, []*configpb.TestGroup{
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
				q.Fix("middle", now.Add(-time.Minute), true)
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
	log := logrus.WithField("test", "TestSend")
	cases := []struct {
		name      string
		q         *TestGroupQueue
		receivers func(context.Context, *testing.T) (context.Context, chan<- *configpb.TestGroup, func() []*configpb.TestGroup)
		freq      time.Duration

		want []*configpb.TestGroup
	}{
		{
			name: "empty",
			q: func() *TestGroupQueue {
				var q TestGroupQueue
				q.Init(log, nil, time.Now())
				return &q
			}(),
			receivers: func(ctx context.Context, t *testing.T) (context.Context, chan<- *configpb.TestGroup, func() []*configpb.TestGroup) {
				ch := make(chan *configpb.TestGroup)
				go func() {
					for {
						select {
						case tg := <-ch:
							t.Errorf("Send() receiver got unexpected group: %v", tg)
							return
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
			q: func() *TestGroupQueue {
				var q TestGroupQueue
				q.Init(log, nil, time.Now())
				return &q
			}(),
			receivers: func(ctx context.Context, t *testing.T) (context.Context, chan<- *configpb.TestGroup, func() []*configpb.TestGroup) {
				ch := make(chan *configpb.TestGroup)
				ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
				go func() {
					for {
						select {
						case tg := <-ch:
							t.Errorf("Send() receiver got unexpected group: %v", tg)
							return
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
				q.Init(log, []*configpb.TestGroup{
					{
						Name: "hi",
					},
				}, time.Now())
				return &q
			}(),
			receivers: func(ctx context.Context, t *testing.T) (context.Context, chan<- *configpb.TestGroup, func() []*configpb.TestGroup) {
				ch := make(chan *configpb.TestGroup)
				var wg sync.WaitGroup
				wg.Add(1)
				var got []*configpb.TestGroup
				go func() {
					defer wg.Done()
					for {
						select {
						case tg := <-ch:
							got = append(got, tg)
							return
						case <-ctx.Done():
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
				q.Init(log, []*configpb.TestGroup{
					{
						Name: "hi",
					},
				}, time.Now())
				return &q
			}(),
			receivers: func(ctx context.Context, _ *testing.T) (context.Context, chan<- *configpb.TestGroup, func() []*configpb.TestGroup) {
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
				q.Init(log, []*configpb.TestGroup{
					{
						Name: "hi",
					},
					{
						Name: "there",
					},
				}, time.Now())
				return &q
			}(),
			receivers: func(ctx context.Context, _ *testing.T) (context.Context, chan<- *configpb.TestGroup, func() []*configpb.TestGroup) {
				ch := make(chan *configpb.TestGroup)
				var wg sync.WaitGroup
				wg.Add(1)
				var got []*configpb.TestGroup
				go func() {
					defer wg.Done()
					for {
						select {
						case tg := <-ch:
							got = append(got, tg)
							if len(got) == 2 {
								return
							}
						case <-ctx.Done():
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
				q.Init(log, []*configpb.TestGroup{
					{
						Name: "hi",
					},
					{
						Name: "there",
					},
				}, time.Now())
				return &q
			}(),
			receivers: func(ctx context.Context, _ *testing.T) (context.Context, chan<- *configpb.TestGroup, func() []*configpb.TestGroup) {
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
