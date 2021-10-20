/*
Copyright 2018 The Kubernetes Authors.

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

// Package gcs provides utilities for interacting with GCS.
//
// This includes basic CRUD operations. It is primarily focused on
// reading prow build results uploaded to GCS.
package gcs

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"

	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"

	"cloud.google.com/go/storage"
	"github.com/golang/protobuf/proto"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// IsPreconditionFailed returns true when the error is an http.StatusPreconditionFailed googleapi.Error.
func IsPreconditionFailed(err error) bool {
	if err == nil {
		return false
	}
	var e *googleapi.Error
	if !errors.As(err, &e) {
		return false
	}
	return e.Code == http.StatusPreconditionFailed
}

// ClientWithCreds returns a storage client, optionally authenticated with the specified .json creds
func ClientWithCreds(ctx context.Context, creds ...string) (*storage.Client, error) {
	var options []option.ClientOption
	switch l := len(creds); l {
	case 0: // Do nothing
	case 1:
		options = append(options, option.WithCredentialsFile(creds[0]))
	default:
		return nil, fmt.Errorf("%d creds files unsupported (at most 1)", l)
	}
	return storage.NewClient(ctx, options...)
}

// Path parses gs://bucket/obj urls
type Path struct {
	url url.URL
}

// NewPath returns a new Path if it parses.
func NewPath(path string) (*Path, error) {
	var p Path
	err := p.Set(path)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// String returns the gs://bucket/obj url
func (g Path) String() string {
	return g.url.String()
}

// URL returns the url
func (g Path) URL() url.URL {
	return g.url
}

// Set updates value from a gs://bucket/obj string, validating errors.
func (g *Path) Set(v string) error {
	u, err := url.Parse(v)
	if err != nil {
		return fmt.Errorf("invalid gs:// url %s: %v", v, err)
	}
	return g.SetURL(u)
}

// SetURL updates value to the passed in gs://bucket/obj url
func (g *Path) SetURL(u *url.URL) error {
	switch {
	case u == nil:
		return errors.New("nil url")
	case u.Scheme != "gs" && u.Scheme != "" && u.Scheme != "file":
		return fmt.Errorf("must use a gs://, file://, or local filesystem url: %s", u)
	case strings.Contains(u.Host, ":"):
		return fmt.Errorf("gs://bucket may not contain a port: %s", u)
	case u.Opaque != "":
		return fmt.Errorf("url must start with gs://: %s", u)
	case u.User != nil:
		return fmt.Errorf("gs://bucket may not contain an user@ prefix: %s", u)
	case u.RawQuery != "":
		return fmt.Errorf("gs:// url may not contain a ?query suffix: %s", u)
	case u.Fragment != "":
		return fmt.Errorf("gs:// url may not contain a #fragment suffix: %s", u)
	}
	g.url = *u
	return nil
}

// MarshalJSON encodes Path as a string
func (g Path) MarshalJSON() ([]byte, error) {
	return json.Marshal(g.String())
}

// UnmarshalJSON decodes a string into Path
func (g *Path) UnmarshalJSON(buf []byte) error {
	var str string
	err := json.Unmarshal(buf, &str)
	if err != nil {
		return err
	}
	if g == nil {
		g = &Path{}
	}
	return g.Set(str)
}

// ResolveReference returns the path relative to the current path
func (g Path) ResolveReference(ref *url.URL) (*Path, error) {
	var newP Path
	if err := newP.SetURL(g.url.ResolveReference(ref)); err != nil {
		return nil, err
	}
	return &newP, nil
}

// Bucket returns bucket in gs://bucket/obj
func (g Path) Bucket() string {
	return g.url.Host
}

// Object returns path/to/something in gs://bucket/path/to/something
func (g Path) Object() string {
	if g.url.Path == "" {
		return g.url.Path
	}
	return g.url.Path[1:]
}

func calcCRC(buf []byte) uint32 {
	return crc32.Checksum(buf, crc32.MakeTable(crc32.Castagnoli))
}

const (
	// DefaultACL for this upload
	DefaultACL = false
	// PublicRead ACL for this upload.
	PublicRead = true
)

// Upload writes bytes to the specified Path by converting the client and path into an ObjectHandle.
func Upload(ctx context.Context, client *storage.Client, path Path, buf []byte, worldReadable bool, cacheControl string) (*storage.ObjectAttrs, error) {
	return realGCSClient{client: client}.Upload(ctx, path, buf, worldReadable, cacheControl)
}

// UploadHandle writes bytes to the specified ObjectHandle
func UploadHandle(ctx context.Context, handle *storage.ObjectHandle, buf []byte, worldReadable bool, cacheControl string) (*storage.ObjectAttrs, error) {
	crc := calcCRC(buf)
	w := handle.NewWriter(ctx)
	defer w.Close()
	if worldReadable {
		w.ACL = []storage.ACLRule{{Entity: storage.AllUsers, Role: storage.RoleReader}}
	}
	if cacheControl != "" {
		w.ObjectAttrs.CacheControl = cacheControl
	}
	w.SendCRC32C = true
	// Send our CRC32 to ensure google received the same data we sent.
	// See checksum example at:
	// https://godoc.org/cloud.google.com/go/storage#Writer.Write
	w.ObjectAttrs.CRC32C = crc
	w.ProgressFunc = func(bytes int64) {
		log.Printf("Uploading gs://%s/%s: %d/%d...", handle.BucketName(), handle.ObjectName(), bytes, len(buf))
	}
	if n, err := w.Write(buf); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	} else if n != len(buf) {
		return nil, fmt.Errorf("partial write: %d < %d", n, len(buf))
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("close: %w", err)
	}
	return w.Attrs(), nil
}

// DownloadGrid downloads and decompresses a grid from the specified path.
func DownloadGrid(ctx context.Context, opener Opener, path Path) (*statepb.Grid, *storage.ReaderObjectAttrs, error) {
	var g statepb.Grid
	r, attrs, err := opener.Open(ctx, path)
	if err != nil && errors.Is(err, storage.ErrObjectNotExist) {
		return &g, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("open: %w", err)
	}
	defer r.Close()
	zr, err := zlib.NewReader(r)
	if err != nil {
		return nil, nil, fmt.Errorf("open zlib: %w", err)
	}
	pbuf, err := ioutil.ReadAll(zr)
	if err != nil {
		return nil, nil, fmt.Errorf("decompress: %w", err)
	}
	err = proto.Unmarshal(pbuf, &g)
	return &g, attrs, err
}

// MarshalGrid serializes a state proto into zlib-compressed bytes.
func MarshalGrid(grid *statepb.Grid) ([]byte, error) {
	buf, err := proto.Marshal(grid)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	var zbuf bytes.Buffer
	zw := zlib.NewWriter(&zbuf)
	if _, err = zw.Write(buf); err != nil {
		return nil, fmt.Errorf("compress: %w", err)
	}
	if err = zw.Close(); err != nil {
		return nil, fmt.Errorf("close: %w", err)
	}
	return zbuf.Bytes(), nil
}
