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

package summarizer

import (
	"context"
	"errors"
	"math/rand"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/config"
	"github.com/GoogleCloudPlatform/testgrid/pkg/pubsub"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/sirupsen/logrus"
)

// FixGCS listens for GCS changes to test groups and schedules another update of its dashboards ~immediately.
//
// Returns when the context is canceled or a processing error occurs.
func FixGCS(client pubsub.Subscriber, log logrus.FieldLogger, projID, subID string, configPath gcs.Path, tabPathPrefix string) (Fixer, error) {
	if !strings.HasSuffix(tabPathPrefix, "/") && tabPathPrefix != "" {
		tabPathPrefix += "/"
	}
	gridPath, err := configPath.ResolveReference(&url.URL{Path: tabPathPrefix})
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, q *config.DashboardQueue) error {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		ch := make(chan *pubsub.Notification)
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				err := pubsub.SendGCS(ctx, log, client, projID, subID, nil, ch)
				if err == nil {
					return
				}
				if errors.Is(err, context.Canceled) || ctx.Err() != nil {
					log.WithError(err).Trace("Subscription canceled")
					return
				}
				sleep := time.Minute + time.Duration(rand.Int63n(int64(time.Minute)))
				log.WithError(err).WithField("sleep", sleep).Error("Error receiving GCS notifications, will retry...")
				time.Sleep(sleep)
			}
		}()
		defer wg.Wait()
		return processGCSNotifications(ctx, log, q, *gridPath, tabPathPrefix != "", ch)
	}, nil
}

func processGCSNotifications(ctx context.Context, log logrus.FieldLogger, q *config.DashboardQueue, gridPrefix gcs.Path, findDashboard bool, senders <-chan *pubsub.Notification) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case notice, ok := <-senders:
			if !ok {
				return nil
			}
			group := processNotification(gridPrefix, findDashboard, notice)
			if group == nil {
				continue
			}
			const delay = 5 * time.Second
			when := notice.Time.Add(delay)
			log.WithFields(logrus.Fields{
				"group":        group,
				"when":         when,
				"notification": notice,
			}).Trace("Fixing groups from gcs notifcation")
			if err := q.FixTestGroups(when, false, *group); err != nil {
				return err
			}
		}
	}
}

// Return the test group/dashboard associated with the notification.
func processNotification(gridPrefix gcs.Path, wantDashboard bool, n *pubsub.Notification) *string {
	if gridPrefix.Bucket() != n.Path.Bucket() {
		return nil
	}

	dir, base := path.Split(n.Path.Object()) // gs://prefix/TEST_GROUP
	if wantDashboard && len(dir) > 0 {       // Actually, gs://prefix/DASH/TAB
		dir, base = path.Split(dir[:len(dir)-1])
	}
	if dir != gridPrefix.Object() {
		return nil
	}
	return &base // gs://prefix/TEST_GROUP
}
