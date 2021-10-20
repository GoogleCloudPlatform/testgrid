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
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Queue can send names to receivers at a specific frequency.
//
// Also contains the ability to modify the next time to send names.
// First call must be to Init().
// Exported methods are safe to call concurrently.
type Queue struct {
	queue  priorityQueue
	items  map[string]*item
	lock   sync.RWMutex
	signal chan struct{}
}

// Init (or reinit) the queue with the specified groups, which should be updated at frequency.
func (q *Queue) Init(names []string, when time.Time) {
	n := len(names)
	found := make(map[string]bool, n)

	q.lock.Lock()
	defer q.lock.Unlock()
	defer q.rouse()

	if q.signal == nil {
		q.signal = make(chan struct{})
	}

	if q.items == nil {
		q.items = make(map[string]*item, n)
	}
	if q.queue == nil {
		q.queue = make(priorityQueue, 0, n)
	}
	items := q.items

	for _, name := range names {
		found[name] = true
		if _, ok := items[name]; ok {
			continue
		}
		it := &item{
			name:  name,
			when:  when,
			index: len(q.queue),
		}
		heap.Push(&q.queue, it)
		items[name] = it
		logrus.WithFields(logrus.Fields{
			"when": when,
			"name": name,
		}).Info("Adding name to queue")
	}

	for name, it := range items {
		if found[name] {
			continue
		}
		logrus.WithField("name", name).Info("Removing name from queue")
		heap.Remove(&q.queue, it.index)
		delete(q.items, name)
	}
}

// FixAll will fix multiple groups inside a single critical section.
//
// If later is set then it will move out the next update time, otherwise
// it will only reduce it.
func (q *Queue) FixAll(whens map[string]time.Time, later bool) error {
	q.lock.Lock()
	defer q.lock.Unlock()
	var missing []string
	defer q.rouse()

	reduced := map[string]time.Time{}
	fixed := map[string]time.Time{}

	for name, when := range whens {
		it, ok := q.items[name]
		if !ok {
			missing = append(missing, name)
			continue
		}
		if when.Before(it.when) {
			reduced[name] = when
		} else if later && !when.Equal(it.when) {
			fixed[name] = when
		} else {
			continue
		}
		it.when = when
	}
	var log logrus.FieldLogger = logrus.New()
	var any bool
	if n := len(reduced); n > 0 {
		log = log.WithField("reduced", n)
		any = true
	}
	if n := len(fixed); n > 0 {
		log = log.WithField("fixed", n)
		any = true
	}
	heap.Init(&q.queue)
	if len(missing) > 0 {
		return fmt.Errorf("not found: %v", missing)
	}
	if any {
		log.Info("Fixed all names")
	}
	return nil
}

// Fix the next time to send the group to receivers.
//
// If later is set then it will move out the next update time, otherwise
// it will only reduce it.
func (q *Queue) Fix(name string, when time.Time, later bool) error {
	q.lock.Lock()
	defer q.lock.Unlock()
	defer q.rouse()

	it, ok := q.items[name]
	if !ok {
		return errors.New("not found")
	}
	log := logrus.WithFields(logrus.Fields{
		"group": name,
		"when":  when,
	})
	if when.Before(it.when) {
		log = log.WithField("reduced minutes", it.when.Sub(when).Round(time.Second).Minutes())
	} else if later && !when.Equal(it.when) {
		log = log.WithField("delayed minutes", when.Sub(it.when).Round(time.Second).Minutes())
	} else {
		return nil
	}
	it.when = when
	heap.Fix(&q.queue, it.index)
	log.Info("Fixed names")
	return nil
}

// Status of the queue: depth, next item and when the next item is ready.
func (q *Queue) Status() (int, *string, time.Time) {
	q.lock.RLock()
	defer q.lock.RUnlock()
	var who *string
	var when time.Time
	if it := q.queue.peek(); it != nil {
		who = &it.name
		when = it.when
	}
	return len(q.queue), who, when
}

func (q *Queue) rouse() {
	select {
	case q.signal <- struct{}{}: // wake up early
	default: // not sleeping
	}
}

func (q *Queue) sleep(d time.Duration) {
	log := logrus.WithFields(logrus.Fields{
		"seconds": d.Round(100 * time.Millisecond).Seconds(),
	})
	if d > 5*time.Second {
		log.Info("Sleeping...")
	} else {
		log.Debug("Sleeping...")
	}
	sleep := time.NewTimer(d)
	start := time.Now()
	select {
	case <-q.signal:
		if !sleep.Stop() {
			<-sleep.C
		}
		dur := time.Now().Sub(start)
		log := log.WithField("after", dur.Round(time.Millisecond))
		switch {
		case dur > 10*time.Second:
			log.Info("Roused")
		case dur > time.Second:
			log.Debug("Roused")
		default:
			log.Trace("Roused")
		}
	case <-sleep.C:
	}
}

// Send test groups to receivers until the context expires.
//
// Pops items off the queue when frequency is zero.
// Otherwise reschedules the item after the specified frequency has elapsed.
func (q *Queue) Send(ctx context.Context, receivers chan<- string, frequency time.Duration) error {
	var next func() (*string, time.Time)
	if frequency == 0 {
		next = func() (*string, time.Time) {
			if len(q.queue) == 0 {
				return nil, time.Time{}
			}
			it := heap.Pop(&q.queue).(*item)
			return &it.name, it.when
		}
	} else {
		next = func() (*string, time.Time) {
			it := q.queue.peek()
			if it == nil {
				return nil, time.Time{}
			}
			when := it.when
			it.when = time.Now().Add(frequency)
			heap.Fix(&q.queue, it.index)
			return &it.name, when
		}
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		q.lock.Lock()
		who, when := next()
		q.lock.Unlock()

		if who == nil {
			if frequency == 0 {
				return nil
			}
			q.sleep(time.Second)
			continue
		}

		if dur := time.Until(when); dur > 0 {
			q.sleep(dur)
		}
		select {
		case receivers <- *who:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

type priorityQueue []*item

func (pq priorityQueue) Len() int { return len(pq) }
func (pq priorityQueue) Less(i, j int) bool {
	return pq[i].when.Before(pq[j].when)
}
func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *priorityQueue) Push(something interface{}) {
	it := something.(*item)
	it.index = len(*pq)
	*pq = append(*pq, it)
}

func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	it := old[n-1]
	it.index = -1
	old[n-1] = nil
	*pq = old[0 : n-1]
	return it
}

func (pq priorityQueue) peek() *item {
	n := len(pq)
	if n == 0 {
		return nil
	}
	return pq[0]
}

type item struct {
	name  string
	when  time.Time
	index int
}
