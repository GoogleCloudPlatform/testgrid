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

package config

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/storage"

	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
)

var (
	cache     map[string]Config
	cacheLock sync.RWMutex
)

const cacheRefreshInterval = 5 * time.Minute

// Config holds a config proto and when it was fetched.
type Config struct {
	proto     *configpb.Configuration
	attrs     *storage.ReaderObjectAttrs
	lastFetch time.Time
}

func init() {
	cache = map[string]Config{}
}

// InitCache clears the cache of configs, forcing the next ReadGCS call to fetch fresh from GCS.
//
// Used primarily for testing
func InitCache() {
	cacheLock.Lock()
	defer cacheLock.Unlock()
	cache = map[string]Config{}
}

// ReadGCS opens the config at path and unmarshals it into a Configuration proto.
//
// If it has been read recently, a cached version will be served.
func ReadGCS(ctx context.Context, opener gcs.Opener, path gcs.Path) (*configpb.Configuration, *storage.ReaderObjectAttrs, error) {
	cacheLock.Lock()
	defer cacheLock.Unlock()
	cfg, ok := cache[path.String()]
	if ok && time.Since(cfg.lastFetch) < cacheRefreshInterval {
		return cfg.proto, cfg.attrs, nil
	}

	r, attrs, err := opener.Open(ctx, path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open config: %w", err)
	}
	p, err := Unmarshal(r)
	defer r.Close()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	cache[path.String()] = Config{
		proto:     p,
		attrs:     attrs,
		lastFetch: time.Now(),
	}
	return p, attrs, nil
}

// ReadPath reads the config from the specified local file path.
func ReadPath(path string) (*configpb.Configuration, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %v", err)
	}
	return Unmarshal(f)
}

// Read will read the Configuration proto message from a local or gs:// path.
//
// The ctx and client are only relevant when path refers to GCS.
func Read(ctx context.Context, path string, client *storage.Client) (*configpb.Configuration, error) {
	if strings.HasPrefix(path, "gs://") {
		gcsPath, err := gcs.NewPath(path)
		if err != nil {
			return nil, fmt.Errorf("bad path: %v", err)
		}
		c, _, err := ReadGCS(ctx, gcs.NewClient(client), *gcsPath)
		return c, err
	}
	return ReadPath(path)
}
