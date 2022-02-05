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
	"time"

	"bitbucket.org/creachadair/stringset"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/util/queue"
	"github.com/sirupsen/logrus"
)

// DashboardQueue sends dashboard names at a specific frequency.
type DashboardQueue struct {
	queue.Queue
	dashboards map[string]*configpb.Dashboard
	groups     map[string]*stringset.Set

	lock sync.RWMutex
}

// Init (or reinit) the queue with the specified configuration.
func (q *DashboardQueue) Init(log logrus.FieldLogger, dashboards []*configpb.Dashboard, when time.Time) {
	n := len(dashboards)
	names := make([]string, n)
	namedDashboards := make(map[string]*configpb.Dashboard, n)
	groups := make(map[string]*stringset.Set, n)
	for i, d := range dashboards {
		name := d.Name
		names[i] = name
		for _, tab := range d.DashboardTab {
			if groups[tab.TestGroupName] == nil {
				ns := stringset.New()
				groups[tab.TestGroupName] = &ns
			}
			groups[tab.TestGroupName].Add(name)
		}
	}
	q.lock.Lock()
	q.Queue.Init(log, names, when)
	q.dashboards = namedDashboards
	q.groups = groups
	q.lock.Unlock()
}

// FixTestGroups will fix all the dashboards associated with the groups.
func (q *DashboardQueue) FixTestGroups(when time.Time, later bool, groups ...string) error {
	q.lock.RLock()
	defer q.lock.RUnlock()
	dashboards := make(map[string]time.Time, len(groups))
	for _, groupName := range groups {
		dashes := q.groups[groupName]
		if dashes == nil {
			continue
		}
		for dashName := range *dashes {
			dashboards[dashName] = when
		}
	}
	return q.FixAll(dashboards, later)
}

// TestGroupQueue can send test groups to receivers at a specific frequency.
//
// Also contains the ability to modify the next time to send groups.
// First call must be to Init().
// Exported methods are safe to call concurrently.
type TestGroupQueue struct {
	queue.Queue
	groups map[string]*configpb.TestGroup
	lock   sync.RWMutex
}

// Init (or reinit) the queue with the specified groups, which should be updated at frequency.
func (q *TestGroupQueue) Init(log logrus.FieldLogger, testGroups []*configpb.TestGroup, when time.Time) {
	n := len(testGroups)
	groups := make(map[string]*configpb.TestGroup, n)
	names := make([]string, n)

	for i, tg := range testGroups {
		name := tg.Name
		names[i] = name
		groups[name] = tg
	}

	q.lock.Lock()
	q.Queue.Init(log, names, when)
	q.groups = groups
	q.lock.Unlock()
}

// Status of the queue: depth, next item and when the next item is ready.
func (q *TestGroupQueue) Status() (int, *configpb.TestGroup, time.Time) {
	q.lock.RLock()
	defer q.lock.RUnlock()
	var tg *configpb.TestGroup
	var when time.Time
	n, who, when := q.Queue.Status()
	if who != nil {
		tg = q.groups[*who]
	}
	return n, tg, when
}

// Send test groups to receivers until the context expires.
//
// Pops items off the queue when frequency is zero.
// Otherwise reschedules the item after the specified frequency has elapsed.
func (q *TestGroupQueue) Send(ctx context.Context, receivers chan<- *configpb.TestGroup, frequency time.Duration) error {
	ch := make(chan string)
	var err error
	go func() {
		err = q.Queue.Send(ctx, ch, frequency)
		close(ch)
	}()

	for who := range ch {
		q.lock.RLock()
		tg := q.groups[who]
		q.lock.RUnlock()
		if tg == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case receivers <- tg:
		}
	}
	return err
}
