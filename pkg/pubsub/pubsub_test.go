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

package pubsub

import (
	"context"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
)

func mustPath(t *testing.T, s string) *gcs.Path {
	p, err := gcs.NewPath(s)
	if err != nil {
		t.Fatalf("gcs.NewPath(%q): %v", s, err)
	}
	return p
}

type fakeAcker struct {
	acks  []string
	nacks []string
}

func (fa *fakeAcker) Ack(m *pubsub.Message) {
	fa.acks = append(fa.acks, m.ID)
}

func (fa *fakeAcker) Nack(m *pubsub.Message) {
	fa.nacks = append(fa.nacks, m.ID)
}

func TestSendToReceivers(t *testing.T) {
	now := time.Now().Round(time.Second)
	cases := []struct {
		name     string
		ctx      context.Context
		send     Sender
		want     []*Notification
		wantAcks fakeAcker
		err      error
	}{
		{
			name: "empty",
			ctx:  context.Background(),
			send: func(ctx context.Context, receive func(context.Context, *pubsub.Message)) error {
				/*
				   for _, msg := range []*pubsub.Message{
				       {},
				   }{
				       receive(ctx, msg)
				   }
				*/
				return nil
			},
		},
		{
			name: "basic",
			ctx:  context.Background(),
			send: func(ctx context.Context, receive func(context.Context, *pubsub.Message)) error {
				for _, msg := range []*pubsub.Message{
					{
						ID: "good-bar",
						Attributes: map[string]string{
							keyBucket:     "foo",
							keyObject:     "bar",
							keyTime:       now.Format(time.RFC3339),
							keyEvent:      string(Finalize),
							keyGeneration: "100",
						},
					},
					{
						ID: "bad-path",
						Attributes: map[string]string{
							keyBucket:     "ignor?e-bad-path",
							keyTime:       now.Add(time.Second).Format(time.RFC3339),
							keyEvent:      string(Delete),
							keyGeneration: "50",
						},
					},
					{
						ID: "bad-time",
						Attributes: map[string]string{
							keyBucket:     "foo",
							keyObject:     "old/bar",
							keyTime:       "ignore bad time",
							keyEvent:      string(Delete),
							keyGeneration: "50",
						},
					},
					{
						ID: "bad-gen",
						Attributes: map[string]string{
							keyBucket:     "foo",
							keyObject:     "old/bar",
							keyTime:       now.Add(time.Second).Format(time.RFC3339),
							keyEvent:      string(Delete),
							keyGeneration: "50.0", // bad gen
						},
					},
					{
						ID: "good-random-event",
						Attributes: map[string]string{
							keyBucket:     "foo",
							keyObject:     "old/bar",
							keyTime:       now.Add(time.Second).Format(time.RFC3339),
							keyEvent:      "random event",
							keyGeneration: "50",
						},
					},
					{
						ID: "good-old-bar",
						Attributes: map[string]string{
							keyBucket:     "foo",
							keyObject:     "old/bar",
							keyTime:       now.Add(time.Second).Format(time.RFC3339),
							keyEvent:      string(Delete),
							keyGeneration: "50",
						},
					},
				} {
					receive(ctx, msg)
				}
				return nil
			},
			wantAcks: fakeAcker{
				acks: []string{
					"good-bar",
					"good-random-event",
					"good-old-bar",
				},
				nacks: []string{
					"bad-path",
					"bad-time",
					"bad-gen",
				},
			},
			want: []*Notification{
				{
					Path:       *mustPath(t, "gs://foo/bar"),
					Event:      Finalize,
					Time:       now,
					Generation: 100,
				},
				{
					Path:       *mustPath(t, "gs://foo/old/bar"),
					Event:      Event("random event"),
					Time:       now.Add(time.Second),
					Generation: 50,
				},
				{
					Path:       *mustPath(t, "gs://foo/old/bar"),
					Event:      Delete,
					Time:       now.Add(time.Second),
					Generation: 50,
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ch := make(chan *Notification)
			var got []*Notification
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				for n := range ch {
					got = append(got, n)
				}
			}()

			var gotAcks fakeAcker
			err := sendToReceivers(tc.ctx, logrus.WithField("name", tc.name), tc.send, ch, &gotAcks)
			close(ch)
			wg.Wait()
			switch {
			case err != tc.err:
				t.Errorf("sendToReceivers() wanted error %v, got %v", tc.err, err)
			default:
				if diff := cmp.Diff(tc.want, got, cmp.AllowUnexported(gcs.Path{})); diff != "" {
					t.Errorf("sendToReceivers() got unexpected diff (-want +got):\n%s", diff)
				}
				if diff := cmp.Diff(tc.wantAcks.acks, gotAcks.acks); diff != "" {
					t.Errorf("sendToReceivers() got unexpected ack diff (-want +got):\n%s", diff)
				}
				if diff := cmp.Diff(tc.wantAcks.nacks, gotAcks.nacks); diff != "" {
					t.Errorf("sendToReceivers() got unexpected nack diff (-want +got):\n%s", diff)
				}
			}
		})
	}
}
