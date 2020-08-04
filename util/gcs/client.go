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

type Client interface {
	Uploader
	Downloader
}

// NewClient returns a GCSUploadClient for the storage.Client.
func NewClient(client *storage.Client) Client {
	return realGCSClient{client}
}

type realGCSClient struct {
	client *storage.Client
}

func (rgc realGCSClient) Open(ctx context.Context, path Path) (io.ReadCloser, error) {
	r, err := rgc.client.Bucket(path.Bucket()).Object(path.Object()).NewReader(ctx)
	return r, err
}

func (rgc realGCSClient) Objects(ctx context.Context, path Path, delimiter string) Iterator {
	p := path.Object()
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return rgc.client.Bucket(path.Bucket()).Objects(ctx, &storage.Query{
		Delimiter: delimiter,
		Prefix:    p,
	})
}

func (rgc realGCSClient) Upload(ctx context.Context, path Path, buf []byte, worldReadable bool, cacheControl string) error {
	return Upload(ctx, rgc.client, path, buf, worldReadable, cacheControl)
}
