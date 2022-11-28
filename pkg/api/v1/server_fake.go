/*
Copyright 2022 The TestGrid Authors.

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

package v1

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/golang/protobuf/proto"

	"github.com/GoogleCloudPlatform/testgrid/config"
	pb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
)

var (
	serverDefaultBucket  = "gs://default"
	serverGridPathPrefix = "grid"
	serverTabPathPrefix  = ""
)

func setupTestServer(t *testing.T, configurations map[string]*pb.Configuration, grids map[string]*statepb.Grid) Server {
	t.Helper()

	fc := fakeClient{
		Datastore: map[gcs.Path][]byte{},
	}

	config.InitCache()
	for p, cfg := range configurations {
		path, err := gcs.NewPath(p)
		if err != nil {
			t.Fatalf("setupTestServer() can't generate path: %v", err)
		}

		fc.Datastore[*path], err = proto.Marshal(cfg)
		if err != nil {
			t.Fatalf("Could not serialize proto: %v\n\nProto:\n%s", err, cfg.String())
		}
	}

	for p, grid := range grids {
		path, err := gcs.NewPath(p)
		if err != nil {
			t.Fatalf("setupTestServer() can't generate path: %v", err)
		}

		fc.Datastore[*path], err = gcs.MarshalGrid(grid)
		if err != nil {
			t.Fatalf("Could not serialize proto: %v\n\nProto:\n%s", err, grid.String())
		}
	}

	return Server{
		Client:         fc,
		DefaultBucket:  serverDefaultBucket,  // Needs test coverage
		GridPathPrefix: serverGridPathPrefix, // Needs test coverage
		TabPathPrefix:  serverTabPathPrefix,
		Timeout:        10 * time.Second, // Needs test coverage
	}
}

type fakeClient struct {
	Datastore map[gcs.Path][]byte
}

func (f fakeClient) Open(ctx context.Context, path gcs.Path) (io.ReadCloser, *storage.ReaderObjectAttrs, error) {
	data, exists := f.Datastore[path]
	if !exists {
		return nil, nil, fmt.Errorf("fake file %s does not exist", path.String())
	}
	return ioutil.NopCloser(bytes.NewReader(data)), &storage.ReaderObjectAttrs{}, nil
}

func (f fakeClient) Upload(ctx context.Context, path gcs.Path, bytes []byte, b bool, s string) (*storage.ObjectAttrs, error) {
	panic("fakeClient Upload not implemented")
}

func (f fakeClient) Objects(ctx context.Context, prefix gcs.Path, delimiter, start string) gcs.Iterator {
	panic("fakeClient Objects not implemented")
}

func (f fakeClient) Stat(ctx context.Context, prefix gcs.Path) (*storage.ObjectAttrs, error) {
	panic("fakeClient Stat not implemented")
}

func (f fakeClient) Copy(ctx context.Context, from, to gcs.Path) (*storage.ObjectAttrs, error) {
	panic("fakeClient Copy not implemented")
}

func (f fakeClient) If(read, write *storage.Conditions) gcs.ConditionalClient {
	return f
}
