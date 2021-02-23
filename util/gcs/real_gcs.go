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

package gcs

import (
	"context"
	"io"
	"strings"

	"cloud.google.com/go/storage"
)

var (
	_ Client = &realGCSClient{} // Ensure this implements interface
)

// NewGCSClient returns a GCSUploadClient for the storage.Client.
func NewGCSClient(client *storage.Client) ConditionalClient {
	return realGCSClient{client, nil, nil}
}

type realGCSClient struct {
	client    *storage.Client
	readCond  *storage.Conditions
	writeCond *storage.Conditions
}

func (rgc realGCSClient) If(read, write *storage.Conditions) ConditionalClient {
	return realGCSClient{
		client:    rgc.client,
		readCond:  read,
		writeCond: write,
	}
}

func (rgc realGCSClient) handle(path Path, cond *storage.Conditions) *storage.ObjectHandle {
	oh := rgc.client.Bucket(path.Bucket()).Object(path.Object())
	if cond == nil {
		return oh
	}
	return oh.If(*cond)
}

func (rgc realGCSClient) Copy(ctx context.Context, from, to Path) error {
	fromH := rgc.handle(from, rgc.readCond)
	_, err := rgc.handle(to, rgc.writeCond).CopierFrom(fromH).Run(ctx)
	return err
}

func (rgc realGCSClient) Open(ctx context.Context, path Path) (io.ReadCloser, error) {
	r, err := rgc.handle(path, rgc.readCond).NewReader(ctx)
	return r, err
}

func (rgc realGCSClient) Objects(ctx context.Context, path Path, delimiter, startOffset string) Iterator {
	p := path.Object()
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return rgc.client.Bucket(path.Bucket()).Objects(ctx, &storage.Query{
		Delimiter:   delimiter,
		Prefix:      p,
		StartOffset: startOffset,
	})
}

func (rgc realGCSClient) Upload(ctx context.Context, path Path, buf []byte, worldReadable bool, cacheControl string) error {
	return UploadHandle(ctx, rgc.handle(path, rgc.writeCond), buf, worldReadable, cacheControl)
}

func (rgc realGCSClient) Stat(ctx context.Context, path Path) (*storage.ObjectAttrs, error) {
	return rgc.handle(path, rgc.readCond).Attrs(ctx)
}
