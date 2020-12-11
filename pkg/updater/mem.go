/*
Copyright 2020 The TestGrid Authors.

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

package updater

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/mem"
	"github.com/sirupsen/logrus"
)

// MemThrottle returns a group and build channel that ensures insufficient memory exists.
func MemThrottle(ctx context.Context, groupSize, buildSize uint64) (<-chan int, <-chan int) {
	group := make(chan int)
	build := make(chan int)
	go func() {
		defer close(group)
		defer close(build)

		var lastNote string
		var log logrus.FieldLogger
		note := func(s string) {
			if lastNote == s {
				log.Debug(s)
			} else {
				log.Info(s)
			}
			lastNote = s
		}
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			vm, err := mem.VirtualMemory()
			if err != nil {
				logrus.WithError(err).Warning("Failed to determine free memory")
				time.Sleep(time.Second)
				continue
			}
			avail := vm.Available
			log = logrus.WithField("available", avail)
			switch {
			case avail > groupSize:
				note("Starting group or build")
				select {
				case <-ctx.Done():
					return
				case group <- 1:
				case build <- 1:
				}
			case avail > buildSize:
				note("Staring build")
				select {
				case <-ctx.Done():
					return
				case build <- 1:
				}
			default:
				note("Waiting for free memory")
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()
	return group, build
}
