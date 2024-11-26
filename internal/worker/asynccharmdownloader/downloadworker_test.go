// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package asynccharmdownloader

import (
	"context"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type charmDownloaderSuite struct {
	applicationService *MockApplicationService
}

var _ = gc.Suite(&charmDownloaderSuite{})

func (s *charmDownloaderSuite) TestAsyncDownloadTrigger(c *gc.C) {
	defer s.setupMocks(c).Finish()

	changeCh := make(chan []string)
	w := watchertest.NewMockStringsWatcher(changeCh)

	s.applicationService.EXPECT().WatchApplicationsWithPendingCharms(gomock.Any()).Return(w, nil)

	done := make(chan struct{})

	var apps []string
	for i := 0; i < 3; i++ {
		appID := applicationtesting.GenApplicationUUID(c)
		apps = append(apps, appID.String())

		s.applicationService.EXPECT().DownloadApplicationCharm(gomock.Any(), appID).DoAndReturn(func(ctx context.Context, at application.ID) error {
			if i == 2 {
				close(done)
			}
			return nil
		})
	}

	worker, err := NewWorker(Config{
		Logger:             loggertesting.WrapCheckLog(c),
		ApplicationService: s.applicationService,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)

	go func() {
		changeCh <- []string{}
		changeCh <- apps
	}()

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for download trigger")
	}

	workertest.CleanKill(c, worker)
}

func (s *charmDownloaderSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.applicationService = NewMockApplicationService(ctrl)

	return ctrl
}
