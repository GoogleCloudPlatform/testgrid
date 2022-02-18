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
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

var (
	_ Client = &localClient{} // Ensure this implements interface
)

type localIterator struct {
	files []os.FileInfo
	dir   string
	index int
}

func convertIsNotExistsErr(err error) error {
	if os.IsNotExist(err) {
		return storage.ErrObjectNotExist
	}
	return err
}

// See https://en.wikipedia.org/wiki/File_URI_scheme#How_many_slashes
var fileRegex = regexp.MustCompile(`file:\/+`)

func cleanFilepath(path Path) string {
	p := fileRegex.ReplaceAllString(path.String(), "/")
	// TODO(michelle192837): Handle URLs vs. filepaths gracefully.
	p, err := url.PathUnescape(p)
	if err != nil {
		return ""
	}
	return p
}

func (li *localIterator) Next() (*storage.ObjectAttrs, error) {
	defer func() { li.index++ }()
	if li.index >= len(li.files) {
		return nil, iterator.Done
	}
	info := li.files[li.index]
	p, err := NewPath(filepath.Join(li.dir, info.Name()))
	if err != nil {
		return nil, err
	}
	return objectAttrs(info, *p), nil
}

// NewLocalClient returns a GCSUploadClient for the storage.Client.
func NewLocalClient() ConditionalClient {
	return localClient{nil, nil}
}

type localClient struct {
	readCond  *storage.Conditions
	writeCond *storage.Conditions
}

func (lc localClient) If(_, _ *storage.Conditions) ConditionalClient {
	return NewLocalClient()
}

func (lc localClient) Copy(ctx context.Context, from, to Path) (*storage.ObjectAttrs, error) {
	buf, err := ioutil.ReadFile(cleanFilepath(from))
	if err != nil {
		return nil, convertIsNotExistsErr(err)
	}
	return lc.Upload(ctx, to, buf, false, "")
}

func (lc localClient) Open(ctx context.Context, path Path) (io.ReadCloser, *storage.ReaderObjectAttrs, error) {
	r, err := os.Open(cleanFilepath(path))
	return r, &storage.ReaderObjectAttrs{}, convertIsNotExistsErr(err)
}

func (lc localClient) Objects(ctx context.Context, path Path, delimiter, startOffset string) Iterator {
	p := cleanFilepath(path)
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}
	files, err := ioutil.ReadDir(p)
	if err != nil {
		return &localIterator{}
	}
	return &localIterator{
		dir:   filepath.Dir(p),
		files: files,
	}
}

func (lc localClient) Upload(ctx context.Context, path Path, buf []byte, _ bool, _ string) (*storage.ObjectAttrs, error) {
	err := ioutil.WriteFile(cleanFilepath(path), buf, 0666)
	if err != nil {
		return nil, convertIsNotExistsErr(err)
	}
	return lc.Stat(ctx, path)
}

func (lc localClient) Stat(ctx context.Context, path Path) (*storage.ObjectAttrs, error) {
	info, err := os.Stat(cleanFilepath(path))
	if err != nil {
		return nil, convertIsNotExistsErr(err)
	}
	return objectAttrs(info, path), nil
}

func objectAttrs(info os.FileInfo, path Path) *storage.ObjectAttrs {
	return &storage.ObjectAttrs{
		Bucket:  path.Bucket(),
		Name:    path.Object(),
		Size:    info.Size(),
		Updated: info.ModTime(),
	}
}
