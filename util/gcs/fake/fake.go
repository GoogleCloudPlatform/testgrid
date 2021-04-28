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

func (cc ConditionalClient) Copy(ctx context.Context, from, to gcs.Path) error {
	if err := cc.check(ctx, &from, &to); err != nil {
		return err
	}

	gen := cc.Uploader[to].Generation + 1
	if err := cc.UploadClient.Copy(ctx, from, to); err != nil {
		return err
	}
	u := cc.Uploader[to]
	u.Generation = gen
	cc.Uploader[to] = u
	return nil
}

func (cc ConditionalClient) Upload(ctx context.Context, path gcs.Path, buf []byte, worldRead bool, cache string) error {
	if err := cc.check(ctx, nil, &path); err != nil {
		return err
	}

	gen := cc.Uploader[path].Generation + 1
	err := cc.UploadClient.Upload(ctx, path, buf, worldRead, cache)
	if err != nil {
		return err
	}

	u := cc.Uploader[path]
	u.Generation = gen
	cc.Uploader[path] = u
	return nil
}

func (cc ConditionalClient) If(read, write *storage.Conditions) gcs.ConditionalClient {
	return ConditionalClient{
		UploadClient: cc.UploadClient,
		read:         read,
		write:        write,
	}
}

type UploadClient struct {
	Client
	Uploader
	Stater
}

func (fuc UploadClient) If(read, write *storage.Conditions) gcs.ConditionalClient {
	return fuc
}

type Stat struct {
	Err   error
	Attrs storage.ObjectAttrs
}

type Stater map[gcs.Path]Stat

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

type Uploader map[gcs.Path]Upload

func (fu Uploader) Copy(ctx context.Context, from, to gcs.Path) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("injected interrupt: %w", err)
	}
	u, present := fu[from]
	if !present {
		return storage.ErrObjectNotExist
	}
	if err := u.Err; err != nil {
		return fmt.Errorf("injected from error: %w", err)
	}

	u.Generation++
	fu[to] = u
	return nil
}

func (fuc Uploader) Upload(ctx context.Context, path gcs.Path, buf []byte, worldRead bool, cacheControl string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("injected interrupt: %w", err)
	}
	if err := fuc[path].Err; err != nil {
		return fmt.Errorf("injected upload error: %w", err)
	}

	fuc[path] = Upload{
		Buf:          buf,
		CacheControl: cacheControl,
		WorldRead:    worldRead,
	}
	return nil
}

type Upload struct {
	Buf          []byte
	CacheControl string
	WorldRead    bool
	Err          error
	Generation   int64
}

type Opener map[gcs.Path]Object

func (fo Opener) Open(ctx context.Context, path gcs.Path) (io.ReadCloser, error) {
	o, ok := fo[path]
	if !ok {
		return nil, fmt.Errorf("wrap not exist: %w", storage.ErrObjectNotExist)
	}
	if o.OpenErr != nil {
		return nil, o.OpenErr
	}
	return &Reader{
		Buf:      bytes.NewBufferString(o.Data),
		ReadErr:  o.ReadErr,
		CloseErr: o.CloseErr,
	}, nil
}

type Object struct {
	Data     string
	OpenErr  error
	ReadErr  error
	CloseErr error
}

type Reader struct {
	Buf      *bytes.Buffer
	ReadErr  error
	CloseErr error
}

func (fr *Reader) Read(p []byte) (int, error) {
	if fr.ReadErr != nil {
		return 0, fr.ReadErr
	}
	return fr.Buf.Read(p)
}

func (fr *Reader) Close() error {
	if fr.CloseErr != nil {
		return fr.CloseErr
	}
	fr.ReadErr = errors.New("already closed")
	fr.CloseErr = fr.ReadErr
	return nil
}

type Lister map[gcs.Path]Iterator

func (fl Lister) Objects(ctx context.Context, path gcs.Path, _, offset string) gcs.Iterator {
	f := fl[path]
	f.ctx = ctx
	return &f
}

type Iterator struct {
	Objects []storage.ObjectAttrs
	Idx     int
	Err     int // must be > 0
	ctx     context.Context
	Offset  string
}

type Client struct {
	Lister
	Opener
}

func (fi *Iterator) Next() (*storage.ObjectAttrs, error) {
	if fi.ctx.Err() != nil {
		return nil, fi.ctx.Err()
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
