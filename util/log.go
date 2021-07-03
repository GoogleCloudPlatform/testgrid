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

// Package util has convenience functions for use throughout TestGrid.
package util

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

// Progress log every duration, including an ETA for completion.
// Returns a function for updating the current index
func Progress(ctx context.Context, log logrus.FieldLogger, every time.Duration, total int, msg string) func(int) {
	start := time.Now()
	ch := make(chan int, 1)
	go func() {
		timer := time.NewTimer(every)
		defer timer.Stop()
		var current int
		for {
			select {
			case <-ctx.Done():
				return
			case current = <-ch:
				// updated index
			case now := <-timer.C:
				elapsed := now.Sub(start)
				rate := elapsed / time.Duration(current)
				eta := time.Duration(total-current) * rate

				log.WithFields(logrus.Fields{
					"current": current,
					"total":   total,
					"percent": (100 * current) / total,
					"remain":  eta.Round(time.Minute),
					"eta":     now.Add(eta).Round(time.Minute),
					"start":   start.Round(time.Minute),
				}).Info(msg)
				timer.Reset(every)
			}
		}
	}()

	return func(idx int) {
		select {
		case ch <- idx:
		default:
		}
	}
}
