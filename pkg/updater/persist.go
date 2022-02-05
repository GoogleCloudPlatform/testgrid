/*
Copyright 2022 The Kubernetes Authors.

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

package updater

import (
	"context"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/config"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/GoogleCloudPlatform/testgrid/util/queue"
	"github.com/sirupsen/logrus"
)

// FixPersistent persists the updater queue using queue.FixPersistent.
func FixPersistent(log logrus.FieldLogger, client queue.PersistClient, path gcs.Path, tick <-chan time.Time) Fixer {
	fix := queue.FixPersistent(log, client, path, tick)
	return func(ctx context.Context, _ logrus.FieldLogger, q *config.TestGroupQueue, _ []*configpb.TestGroup) error {
		return fix(ctx, &q.Queue)
	}
}
