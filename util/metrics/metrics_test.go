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
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestCyclic(t *testing.T) {
	testcases := []struct {
		name          string
		method        func(Cyclic)
		expectSeconds int64
		expectFails   int64
		expectSuccess int64
		expectSkips   int64
	}{
		{
			name: "success",
			method: func(c Cyclic) {
				fin := c.Start()
				time.Sleep(1 * time.Second)
				fin.Success()
			},
			expectSeconds: 1,
			expectSuccess: 1,
		},
		{
			name: "error",
			method: func(c Cyclic) {
				fin := c.Start()
				time.Sleep(2 * time.Second)
				fin.Fail()
			},
			expectSeconds: 2,
			expectFails:   1,
		},
		{
			name: "skips",
			method: func(c Cyclic) {
				fin := c.Start()
				fin.Skip()
			},
			expectSkips: 1,
		},
		{
			name: "counting",
			method: func(c Cyclic) {
				for i := 0; i < 5; i++ {
					ok := c.Start()
					ok.Success()
					no := c.Start()
					no.Fail()
				}
			},
			expectSuccess: 5,
			expectFails:   5,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			var errorCounter, skipCounter, successCounter FakeCounter
			cycleInt64 := FakeInt64{read: true}

			cyclic := Cyclic{
				errors:       &errorCounter,
				skips:        &skipCounter,
				successes:    &successCounter,
				cycleSeconds: &cycleInt64,
			}

			test.method(cyclic)

			if test.expectFails != errorCounter.count {
				t.Errorf("Want %d errors, got %d", test.expectFails, errorCounter.count)
			}
			if test.expectSkips != skipCounter.count {
				t.Errorf("Want %d skips, got %d", test.expectSkips, skipCounter.count)
			}
			if test.expectSuccess != successCounter.count {
				t.Errorf("Want %d successes, got %d", test.expectSuccess, successCounter.count)
			}
			if test.expectSeconds != cycleInt64.last {
				t.Errorf("Expected %d seconds, got %d", test.expectSeconds, cycleInt64.last)
			}
		})
	}

}

func TestCyclic_TolerateNilCounters(t *testing.T) {
	for a, errorCounter := range []Counter{nil, &FakeCounter{}} {
		for b, skipCounter := range []Counter{nil, &FakeCounter{}} {
			for c, successCounter := range []Counter{nil, &FakeCounter{}} {
				for d, cycleInt64 := range []Int64{nil, &FakeInt64{}} {
					t.Run(fmt.Sprintf("Error-%d Skip-%d Success-%d Cycle-%d", a, b, c, d), func(t *testing.T) {
						cyclic := Cyclic{
							errors:       errorCounter,
							skips:        skipCounter,
							successes:    successCounter,
							cycleSeconds: cycleInt64,
						}
						var wg sync.WaitGroup
						wg.Add(3)
						go func() {
							f := cyclic.Start()
							f.Success()
							wg.Done()
						}()
						go func() {
							f := cyclic.Start()
							f.Skip()
							wg.Done()
						}()
						go func() {
							f := cyclic.Start()
							f.Fail()
							wg.Done()
						}()
						wg.Wait()
					})
				}
			}
		}
	}
}

type FakeCounter struct {
	count int64
}

func (f *FakeCounter) Name() string {
	return "FakeCounter"
}

func (f *FakeCounter) Add(n int64, _ ...string) {
	f.count += n
}

type FakeInt64 struct {
	read bool // Fake implementation not concurrent-safe
	last int64
}

func (f *FakeInt64) Name() string {
	return "FakeCounter"
}

func (f *FakeInt64) Set(n int64, _ ...string) {
	if f.read {
		f.last = n
	}
}
