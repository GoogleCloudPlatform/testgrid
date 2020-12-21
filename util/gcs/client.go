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

package gcs

import (
	"context"
	"io"
	"strings"

	"cloud.google.com/go/storage"
)

// Uploader adds upload capabilities to a GCS client.
type Uploader interface {
	Upload(context.Context, Path, []byte, bool, string) error
}

// Downloader can list files and open them for reading.
type Downloader interface {
	Lister
	Opener
}

// A Lister returns objects under a prefix.
type Lister interface {
	Objects(ctx context.Context, prefix Path, delimiter, start string) Iterator
}

// An iterator returns the attributes of the listed objects or an iterator.Done error.
type Iterator interface {
	Next() (*storage.ObjectAttrs, error)
}

// An Opener opens a path for reading.
type Opener interface {
	Open(ctx context.Context, path Path) (io.ReadCloser, error)
}

// A Stater can stat an object and get its attributes.
type Stater interface {
	Stat(ctx context.Context, prefix Path) (*storage.ObjectAttrs, error)
}

// A Copier can cloud copy an object to a new location.
type Copier interface {
	// Copy an object to the specified path
	Copy(ctx context.Context, from, to Path) error
}

// A Client can upload, download and stat.
type Client interface {
	Uploader
	Downloader
	Stater
	Copier
}

// A ConditionalClient can limit actions to those matching conditions.
type ConditionalClient interface {
	Client
	// If specifies conditions on the object read from and/or written to.
	If(read, write *storage.Conditions) ConditionalClient
}

// NewClient returns a GCSUploadClient for the storage.Client.
func NewClient(client *storage.Client) ConditionalClient {
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
