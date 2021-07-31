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

package fake

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
)

// ConditionalClient is a fake conditional client that can limit actions to matching conditions.
type ConditionalClient struct {
	UploadClient
	read, write *storage.Conditions
}

func (cc ConditionalClient) check(ctx context.Context, from, to *gcs.Path) error {
	if from != nil && cc.read != nil {
		attrs, err := cc.Stat(ctx, *from)
		if err != nil {
			return err
		}
		if cc.read.GenerationMatch != 0 && cc.read.GenerationMatch != attrs.Generation {
			return fmt.Errorf("bad generation: %w", &googleapi.Error{
				Code: http.StatusPreconditionFailed,
			})
		}
	}
	if to != nil && cc.write != nil {
		attrs, err := cc.Stat(ctx, *to)
		switch {
		case err == storage.ErrObjectNotExist:
			if cc.write.GenerationMatch != 0 {
				return fmt.Errorf("bad generation: %w", &googleapi.Error{
					Code: http.StatusPreconditionFailed,
				})
			}
		case err != nil:
			return err
		case cc.write.GenerationMatch != 0 && cc.write.GenerationMatch != attrs.Generation:
			return fmt.Errorf("bad generation: %w", &googleapi.Error{
				Code: http.StatusPreconditionFailed,
			})
		}
	}
	return nil
}

// Copy copies the contents of 'from' into 'to'.
func (cc ConditionalClient) Copy(ctx context.Context, from, to gcs.Path) (*storage.ObjectAttrs, error) {
	if err := cc.check(ctx, &from, &to); err != nil {
		return nil, err
	}

	gen := cc.Uploader[to].Generation + 1
	if _, err := cc.UploadClient.Copy(ctx, from, to); err != nil {
		return nil, err
	}
	u := cc.Uploader[to]
	u.Generation = gen
	cc.Uploader[to] = u
	return u.Attrs(to), nil
}

// Upload writes content to the given path.
func (cc ConditionalClient) Upload(ctx context.Context, path gcs.Path, buf []byte, worldRead bool, cache string) (*storage.ObjectAttrs, error) {
	if err := cc.check(ctx, nil, &path); err != nil {
		return nil, err
	}

	gen := cc.Uploader[path].Generation + 1
	_, err := cc.UploadClient.Upload(ctx, path, buf, worldRead, cache)
	if err != nil {
		return nil, err
	}

	u := cc.Uploader[path]
	u.Generation = gen
	cc.Uploader[path] = u
	return u.Attrs(path), nil
}

// If returns a fake conditional client.
func (cc ConditionalClient) If(read, write *storage.Conditions) gcs.ConditionalClient {
	return ConditionalClient{
		UploadClient: cc.UploadClient,
		read:         read,
		write:        write,
	}
}

// UploadClient is a fake upload client
type UploadClient struct {
	Client
	Uploader
	Stater
}

// If returns a fake upload client.
func (fuc UploadClient) If(read, write *storage.Conditions) gcs.ConditionalClient {
	return fuc
}

// Stat contains object attributes for a given path.
type Stat struct {
	Err   error
	Attrs storage.ObjectAttrs
}

// Stater stats given paths.
type Stater map[gcs.Path]Stat

// Stat returns object attributes for a given path.
func (fs Stater) Stat(ctx context.Context, path gcs.Path) (*storage.ObjectAttrs, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("injected interrupt: %w", err)
	}

	ret, ok := fs[path]
	if !ok {
		return nil, storage.ErrObjectNotExist
	}
	if ret.Err != nil {
		return nil, fmt.Errorf("injected upload error: %w", ret.Err)
	}
	return &ret.Attrs, nil
}

// Uploader adds upload capabilities to a fake client.
type Uploader map[gcs.Path]Upload

// Copy an object to the specified path
func (fu Uploader) Copy(ctx context.Context, from, to gcs.Path) (*storage.ObjectAttrs, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("injected interrupt: %w", err)
	}
	u, present := fu[from]
	if !present {
		return nil, storage.ErrObjectNotExist
	}
	if err := u.Err; err != nil {
		return nil, fmt.Errorf("injected from error: %w", err)
	}

	u.Generation++
	fu[to] = u
	return u.Attrs(to), nil
}

// Upload writes content to the given path.
func (fu Uploader) Upload(ctx context.Context, path gcs.Path, buf []byte, worldRead bool, cacheControl string) (*storage.ObjectAttrs, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("injected interrupt: %w", err)
	}
	if err := fu[path].Err; err != nil {
		return nil, fmt.Errorf("injected upload error: %w", err)
	}

	u := Upload{
		Buf:          buf,
		CacheControl: cacheControl,
		WorldRead:    worldRead,
	}
	fu[path] = u
	return u.Attrs(path), nil
}

// Upload represents an upload.
type Upload struct {
	Buf          []byte
	CacheControl string
	WorldRead    bool
	Err          error
	Generation   int64
}

// Attrs returns file attributes.
func (u Upload) Attrs(path gcs.Path) *storage.ObjectAttrs {
	return &storage.ObjectAttrs{
		Bucket:       path.Bucket(),
		Name:         path.Object(),
		CacheControl: u.CacheControl,
		Generation:   u.Generation,
	}
}

// Opener opens given paths.
type Opener map[gcs.Path]Object

// Open returns a handle for a given path.
func (fo Opener) Open(ctx context.Context, path gcs.Path) (io.ReadCloser, *storage.ReaderObjectAttrs, error) {
	o, ok := fo[path]
	if !ok {
		return nil, nil, storage.ErrObjectNotExist
	}
	if o.OpenErr != nil {
		return nil, nil, o.OpenErr
	}
	return &Reader{
		Buf:      bytes.NewBufferString(o.Data),
		ReadErr:  o.ReadErr,
		CloseErr: o.CloseErr,
	}, o.Attrs, nil
}

// Object holds data for an object.
type Object struct {
	Data     string
	Attrs    *storage.ReaderObjectAttrs
	OpenErr  error
	ReadErr  error
	CloseErr error
}

// A Reader reads a file.
type Reader struct {
	Buf      *bytes.Buffer
	ReadErr  error
	CloseErr error
}

// Read reads a file's contents.
func (fr *Reader) Read(p []byte) (int, error) {
	if fr.ReadErr != nil {
		return 0, fr.ReadErr
	}
	return fr.Buf.Read(p)
}

// Close closes a file.
func (fr *Reader) Close() error {
	if fr.CloseErr != nil {
		return fr.CloseErr
	}
	fr.ReadErr = errors.New("already closed")
	fr.CloseErr = fr.ReadErr
	return nil
}

// A Lister returns objects under a prefix.
type Lister map[gcs.Path]Iterator

// Objects returns an iterator of objects under a given path.
func (fl Lister) Objects(ctx context.Context, path gcs.Path, _, offset string) gcs.Iterator {
	f := fl[path]
	f.ctx = ctx
	return &f
}

// An Iterator returns the attributes of the listed objects or an iterator.Done error.
type Iterator struct {
	Objects []storage.ObjectAttrs
	Idx     int
	Err     int // must be > 0
	ctx     context.Context
	Offset  string
	ErrOpen error
}

// A Client can list files and open them for reading.
type Client struct {
	Lister
	Opener
}

// Next returns the next value.
func (fi *Iterator) Next() (*storage.ObjectAttrs, error) {
	if fi.ctx.Err() != nil {
		return nil, fi.ctx.Err()
	}
	if fi.ErrOpen != nil {
		return nil, fi.ErrOpen
	}
	for fi.Idx < len(fi.Objects) {
		if fi.Offset == "" {
			break
		}
		name, prefix := fi.Objects[fi.Idx].Name, fi.Objects[fi.Idx].Prefix
		if name != "" && name < fi.Offset {
			continue
		}
		if prefix != "" && prefix < fi.Offset {
			continue
		}
		fi.Idx++
	}
	if fi.Idx >= len(fi.Objects) {
		return nil, iterator.Done
	}
	if fi.Idx > 0 && fi.Idx == fi.Err {
		return nil, errors.New("injected Iterator error")
	}

	o := fi.Objects[fi.Idx]
	fi.Idx++
	return &o, nil
}
