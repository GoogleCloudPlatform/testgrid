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

	"cloud.google.com/go/storage"
)

// Uploader adds upload capabilities to a GCS client.
type Uploader interface {
	Upload(ctx context.Context, path Path, buf []byte, public bool, cacheControl string) (*storage.ObjectAttrs, error)
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

// An Iterator returns the attributes of the listed objects or an iterator.Done error.
type Iterator interface {
	Next() (*storage.ObjectAttrs, error)
}

// An Opener opens a path for reading.
type Opener interface {
	Open(ctx context.Context, path Path) (io.ReadCloser, *storage.ReaderObjectAttrs, error)
}

// A Stater can stat an object and get its attributes.
type Stater interface {
	Stat(ctx context.Context, prefix Path) (*storage.ObjectAttrs, error)
}

// A Copier can cloud copy an object to a new location.
type Copier interface {
	// Copy an object to the specified path
	Copy(ctx context.Context, from, to Path) (*storage.ObjectAttrs, error)
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

type gcsClient struct {
	gcs   *realGCSClient
	local *localClient
}

// NewClient returns a flexible (local or GCS) storage client.
func NewClient(client *storage.Client) ConditionalClient {
	return gcsClient{
		gcs:   &realGCSClient{client, nil, nil},
		local: &localClient{nil, nil},
	}
}

// If returns a flexible (local or GCS) conditional client.
func (gc gcsClient) If(read, write *storage.Conditions) ConditionalClient {
	return gcsClient{
		gcs:   &realGCSClient{gc.gcs.client, read, write},
		local: &localClient{nil, nil},
	}
}

func (gc gcsClient) clientFromPath(path Path) ConditionalClient {
	if path.URL().Scheme == "gs" {
		return gc.gcs
	}
	return gc.local
}

// Copy copies the contents of 'from' into 'to'.
func (gc gcsClient) Copy(ctx context.Context, from, to Path) (*storage.ObjectAttrs, error) {
	client := gc.clientFromPath(from)
	return client.Copy(ctx, from, to)
}

// Open returns a handle for a given path.
func (gc gcsClient) Open(ctx context.Context, path Path) (io.ReadCloser, *storage.ReaderObjectAttrs, error) {
	client := gc.clientFromPath(path)
	return client.Open(ctx, path)
}

// Objects returns an iterator of objects under a given path.
func (gc gcsClient) Objects(ctx context.Context, path Path, delimiter, startOffset string) Iterator {
	client := gc.clientFromPath(path)
	return client.Objects(ctx, path, delimiter, startOffset)
}

// Upload writes content to the given path.
func (gc gcsClient) Upload(ctx context.Context, path Path, buf []byte, worldReadable bool, cacheControl string) (*storage.ObjectAttrs, error) {
	client := gc.clientFromPath(path)
	return client.Upload(ctx, path, buf, worldReadable, cacheControl)
}

// Stat returns object attributes for a given path.
func (gc gcsClient) Stat(ctx context.Context, path Path) (*storage.ObjectAttrs, error) {
	client := gc.clientFromPath(path)
	return client.Stat(ctx, path)
}
