package summarizer

import (
	"context"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/config"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/GoogleCloudPlatform/testgrid/util/queue"
	"github.com/sirupsen/logrus"
)

// FixPersistent persists the updater queue using queue.FixPersistent.
func FixPersistent(log logrus.FieldLogger, client queue.PersistClient, path gcs.Path, tick <-chan time.Time) Fixer {
	fix := queue.FixPersistent(log, client, path, tick)
	return func(ctx context.Context, iq *config.DashboardQueue) error {
		return fix(ctx, &iq.Queue)
	}
}
