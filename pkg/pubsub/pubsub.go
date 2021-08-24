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

// Package pubsub exports messages for interacting with pubsub.
package pubsub

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/sirupsen/logrus"
)

// Subscriber creates Senders that attach to subscriptions with the specified settings.
type Subscriber interface {
	Subscribe(projID, subID string, setttings *pubsub.ReceiveSettings) Sender
}

// Client wraps a pubsub client into a Subscriber that creates Senders for pubsub subscriptions.
type Client pubsub.Client

// NewClient converts a raw pubsub client into a Subscriber.
func NewClient(client *pubsub.Client) *Client {
	return (*Client)(client)
}

// Subscribe to the specified id in the project using optional receive settings.
func (c *Client) Subscribe(projID, subID string, settings *pubsub.ReceiveSettings) Sender {
	sub := (*pubsub.Client)(c).SubscriptionInProject(subID, projID)
	if settings != nil {
		sub.ReceiveSettings = *settings
	}
	return sub.Receive
}

// Sender forwards pubsub messages to the receive function until the send context expires.
type Sender func(sendCtx context.Context, receive func(context.Context, *pubsub.Message)) error

const (
	keyBucket     = "bucketId"
	keyObject     = "objectId"
	keyEvent      = "eventType"
	keyTime       = "eventTime"
	keyGeneration = "objectGeneration"
)

// Event specifies what happened to the GCS object.
//
// See https://cloud.google.com/storage/docs/pubsub-notifications#events
type Event string

// Well-known event types.
const (
	Finalize Event = "OBJECT_FINALIZE"
	Delete   Event = "OBJECT_DELETE"
)

// Notification captures information about a change to a GCS object.
type Notification struct {
	Path       gcs.Path
	Event      Event
	Time       time.Time
	Generation int64
}

func (n Notification) String() string {
	return fmt.Sprintf("%s#%d %s at %s", n.Path, n.Generation, n.Event, n.Time)
}

// SendGCS converts GCS pubsub messages into Notification structs and sends them to receivers.
//
// Connects to the specified subscription with optionally specified settings.
// Receives pubsub messages from this subscription and converts it into a Notification struct.
//   - Nacks any message it cannot parse.
// Sends the notification to the receivers channel.
//   - Nacks messages associated with any unsent Notifications.
//   - Acks as soon as the Notification is sent.
//
// More info: https://cloud.google.com/storage/docs/pubsub-notifications#overview
func SendGCS(ctx context.Context, log logrus.FieldLogger, client Subscriber, projectID, subID string, settings *pubsub.ReceiveSettings, receivers chan<- *Notification) error {
	l := log.WithField("subscription", "pubsub://"+projectID+"/"+subID)
	send := client.Subscribe(projectID, subID, settings)
	l.Trace("Subscribing...")
	return sendToReceivers(ctx, l, send, receivers, realAcker{})
}

type acker interface {
	Ack(*pubsub.Message)
	Nack(*pubsub.Message)
}

type realAcker struct{}

func (ra realAcker) Ack(m *pubsub.Message) {
	m.Ack()
}

func (ra realAcker) Nack(m *pubsub.Message) {
	m.Nack()
}

func sendToReceivers(ctx context.Context, log logrus.FieldLogger, send Sender, receivers chan<- *Notification, result acker) error {
	return send(ctx, func(ctx context.Context, msg *pubsub.Message) {
		bucket, obj := msg.Attributes[keyBucket], msg.Attributes[keyObject]
		path, err := gcs.NewPath("gs://" + bucket + "/" + obj)
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"bucket": bucket,
				"object": obj,
				"id":     msg.ID,
			}).Error("Failed to parse path")
			result.Nack(msg)
			return
		}
		when, err := time.Parse(time.RFC3339, msg.Attributes[keyTime])
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"time": msg.Attributes[keyTime],
				"id":   msg.ID,
			}).Error("Failed to parse time")
			result.Nack(msg)
			return
		}
		gen, err := strconv.ParseInt(msg.Attributes[keyGeneration], 10, 64)
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"generation": msg.Attributes[keyGeneration],
				"id":         msg.ID,
			}).Error("Failed to parse generation")
			result.Nack(msg)
			return
		}
		notice := Notification{
			Path:       *path,
			Event:      Event(msg.Attributes[keyEvent]),
			Time:       when,
			Generation: gen,
		}
		select {
		case <-ctx.Done():
			result.Nack(msg)
		case receivers <- &notice:
			result.Ack(msg)
		}
	})

}
