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

package updater

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/config"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/pkg/pubsub"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/sirupsen/logrus"
)

// FixGCS listens for changes to GCS files and schedules another update of those groups ~immediately.
//
// Limited to test groups with a gcs_config result_source that includes pubsub info.
// Returns when the context is canceled or a processing error occurs.
func FixGCS(subscriber pubsub.Subscriber) Fixer {
	return func(ctx context.Context, log logrus.FieldLogger, q *config.TestGroupQueue, groups []*configpb.TestGroup) error {
		paths, subs, err := gcsSubscribedPaths(groups)
		if err != nil {
			return fmt.Errorf("group paths: %v", err)
		}
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		var wg sync.WaitGroup
		ch := make(chan *pubsub.Notification)
		wg.Add(1)
		go func() {
			defer wg.Done()
			subscribeGCS(ctx, log, subscriber, ch, subs...)
		}()
		return processGCSNotifications(ctx, log, q, paths, ch)
	}
}

func gcsSubscribedPaths(tgs []*configpb.TestGroup) (map[gcs.Path][]string, []subscription, error) {
	paths := make(map[gcs.Path][]string, len(tgs))
	subscriptions := map[subscription]bool{}

	for _, tg := range tgs {
		sub := groupSubscription(tg)
		if sub == nil {
			continue
		}
		subscriptions[*sub] = true
		name := tg.Name
		gps, err := groupPaths(tg)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %v", name, err)
		}
		for _, gp := range gps {
			paths[gp] = append(paths[gp], name)
		}
	}
	var subs []subscription
	if n := len(subscriptions); n > 0 {
		subs = make([]subscription, 0, n)
		for sub := range subscriptions {
			subs = append(subs, sub)
		}
	}
	return paths, subs, nil
}

func groupSubscription(tg *configpb.TestGroup) *subscription {
	cfg := tg.GetResultSource().GetGcsConfig()
	if cfg == nil {
		return manualGroupSubscription(tg)
	}
	proj, sub := cfg.PubsubProject, cfg.PubsubSubscription
	if proj == "" || sub == "" {
		return nil
	}
	return &subscription{proj, sub}
}

var manualSubs map[string]subscription

// AddManualSubscription allows injecting additional subscriptions that are not
// specified by the test group itself.
//
// Likely to be removed (or migrated into the config.proto) in a future version.
func AddManualSubscription(projID, subID, prefix string) {
	if manualSubs == nil {
		manualSubs = map[string]subscription{}
	}
	manualSubs[prefix] = subscription{projID, subID}
}

func manualGroupSubscription(tg *configpb.TestGroup) *subscription {
	gp := gcsPrefix(tg)
	for prefix, sub := range manualSubs {
		if strings.HasPrefix(gp, prefix) {
			return &sub
		}
	}
	return nil
}

type subscription struct {
	proj string
	sub  string
}

func (s subscription) String() string {
	return fmt.Sprintf("pubsub://%s/%s", s.proj, s.sub)
}

// Begin sending notifications for this subscription to the channel.
//
// Automatically cancels an existing routine listening to this subscription.
func subscribeGCS(ctx context.Context, log logrus.FieldLogger, client pubsub.Subscriber, receivers chan<- *pubsub.Notification, subs ...subscription) {
	var wg sync.WaitGroup
	wg.Add(len(subs))
	defer wg.Wait()
	for _, sub := range subs {
		log := log.WithField("subscription", sub.String())
		projID, subID := sub.proj, sub.sub
		log.Debug("Subscribed to GCS changes")
		go func() {
			defer wg.Done()
			for {
				err := pubsub.SendGCS(ctx, log, client, projID, subID, nil, receivers)
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
	}
}

func processGCSNotifications(ctx context.Context, log logrus.FieldLogger, q *config.TestGroupQueue, paths map[gcs.Path][]string, senders <-chan *pubsub.Notification) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case notice := <-senders:
			groups, delay := processNotification(paths, notice)
			if len(groups) == 0 {
				break
			}
			when := notice.Time.Add(delay)
			log.WithFields(logrus.Fields{
				"groups":       groups,
				"when":         when,
				"notification": notice,
			}).Trace("Fixing groups from gcs notifcation")
			if len(groups) == 1 {
				name := groups[0]
				if err := q.Fix(name, when, false); err != nil {
					return fmt.Errorf("fix %q: %w", name, err)
				}
				continue
			}
			whens := make(map[string]time.Time, len(groups))
			for _, g := range groups {
				whens[g] = when
			}
			if err := q.FixAll(whens, false); err != nil {
				return fmt.Errorf("fix all: %w", err)
			}
		}
	}
}

var namedDurations = map[string]time.Duration{
	"podinfo.json":  time.Second,     // Done
	"finished.json": time.Minute,     // Container done, wait for prowjob to finish
	"metadata.json": 5 * time.Minute, // Should finish soon
	"started.json":  time.Second,     // Running
}

// Try to balance providing up-to-date info with minimal redundant processing.
// In particular, when the job finishes the sidecar container will upload:
// * a bunch of junit files, then finished.json
// Soon after this crier will notice the prowjob has been finalized and the gcsreporter should:
// * upload podinfo.json
//
// Ideally in this scenario we give the system time to upload everything and process this data once.
func processNotification(paths map[gcs.Path][]string, n *pubsub.Notification) ([]string, time.Duration) {
	var out []string
	b, obj := n.Path.Bucket(), n.Path.Object()
	base := path.Base(obj)
	dur, ok := namedDurations[base]
	if !ok { // Maybe it is a junit file, should finish soon
		if !strings.HasPrefix(base, "junit") || !strings.HasSuffix(base, ".xml") {
			return nil, 0
		}
		dur = 5 * time.Minute
	}

	for path, groups := range paths {
		if path.Bucket() != b {
			continue
		}
		if !strings.HasPrefix(obj, path.Object()) {
			continue
		}
		out = append(out, groups...)
	}
	if len(out) == 0 {
		return nil, 0
	}
	sort.Strings(out)
	return out, dur
}
