package queue

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs/fake"
	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
)

func TestFixPersistent(t *testing.T) {
	now := time.Now().Round(time.Second)
	next := now.Add(time.Minute)
	later := now.Add(time.Hour)
	cases := []struct {
		name        string
		q           *Queue
		currently   fake.Object
		ticks       []time.Time
		fixes       map[string]time.Time
		wantCurrent map[string]time.Time
		wantBuf     string
	}{
		{
			name:        "basic",
			q:           &Queue{},
			wantCurrent: map[string]time.Time{},
		},
		{
			name: "no load",
			q: func() *Queue {
				var q Queue
				q.Init(logrus.New(), []string{"foo", "bar"}, next)
				return &q
			}(),
			wantCurrent: map[string]time.Time{
				"foo": next,
				"bar": next,
			},
		},
		{
			name: "load empty",
			q: func() *Queue {
				var q Queue
				q.Init(logrus.New(), []string{"foo", "bar"}, next)
				return &q
			}(),
			ticks: []time.Time{now},
			wantCurrent: map[string]time.Time{
				"foo": next,
				"bar": next,
			},
		},
		{
			name: "load",
			q: func() *Queue {
				var q Queue
				q.Init(logrus.New(), []string{"keep-next", "bump-to-now"}, next)
				return &q
			}(),
			ticks: []time.Time{now},
			currently: fake.Object{
				Data: func() string {
					saved := map[string]time.Time{
						"keep-next":   later,
						"bump-to-now": now,
						"ignore-old":  now,
					}
					buf, err := json.Marshal(saved)
					if err != nil {
						t.Fatalf("Failed to marshal: %v", err)
					}
					return string(buf)
				}(),
				Attrs: &storage.ReaderObjectAttrs{},
			},
			wantCurrent: map[string]time.Time{
				"keep-next":   next,
				"bump-to-now": now,
			},
		},
		{
			name: "load err",
			q: func() *Queue {
				var q Queue
				q.Init(logrus.New(), []string{"keep-next", "would-bump-to-now-if-read"}, next)
				return &q
			}(),
			ticks: []time.Time{now},
			currently: fake.Object{
				Data: func() string {
					saved := map[string]time.Time{
						"keep-next":                 later,
						"would-bump-to-now-if-read": now,
					}
					buf, err := json.Marshal(saved)
					if err != nil {
						t.Fatalf("Failed to marshal: %v", err)
					}
					return string(buf)
				}(),
				Attrs:   &storage.ReaderObjectAttrs{},
				OpenErr: errors.New("fake open error"),
			},
			wantCurrent: map[string]time.Time{
				"keep-next":                 next,
				"would-bump-to-now-if-read": next,
			},
		},
		{
			name: "load and save",
			q: func() *Queue {
				var q Queue
				q.Init(logrus.New(), []string{"keep-next", "bump-to-now"}, next)
				return &q
			}(),
			ticks: []time.Time{now, now, now},
			currently: fake.Object{
				Data: func() string {
					saved := map[string]time.Time{
						"keep-next":   later,
						"bump-to-now": now,
						"ignore-old":  now,
					}
					buf, err := json.Marshal(saved)
					if err != nil {
						t.Fatalf("Failed to marshal: %v", err)
					}
					return string(buf)
				}(),
				Attrs: &storage.ReaderObjectAttrs{},
			},
			wantCurrent: map[string]time.Time{
				"keep-next":   next,
				"bump-to-now": now,
			},
			wantBuf: func() string {
				saved := map[string]time.Time{
					"keep-next":   next,
					"bump-to-now": now,
				}
				buf, err := json.MarshalIndent(saved, "", "  ")
				if err != nil {
					t.Fatalf("Failed to marshal: %v", err)
				}
				return string(buf)
			}(),
		},
	}

	path, err := gcs.NewPath("gs://fake-bucket/path/to/whatever")
	if err != nil {
		t.Fatalf("NewPath(): %v", err)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.UploadClient{
				Uploader: fake.Uploader{},
				Client: fake.Client{
					Opener: fake.Opener{
						*path: tc.currently,
					},
				},
			}
			ch := make(chan time.Time)
			fix := FixPersistent(logrus.WithField("name", tc.name), client, *path, ch)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			go func() {
				for _, tick := range tc.ticks {
					ch <- tick
				}
				cancel()
			}()
			if err := fix(ctx, tc.q); !errors.Is(err, context.Canceled) {
				t.Errorf("fix() returned unexpected error: %v", err)
			} else {
				got := tc.q.Current()
				if diff := cmp.Diff(tc.wantCurrent, got); diff != "" {
					t.Errorf("fix() got unexpected current diff (-want +got):\n%s", diff)
				}
				gotBytes := string(client.Uploader[*path].Buf)
				if diff := cmp.Diff(tc.wantBuf, gotBytes); diff != "" {
					t.Errorf("fix() got unexpected byte diff (-want +got):\n%s", diff)
				}

			}
		})
	}
}
