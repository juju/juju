// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package asynccharmdownloader

import (
	"context"
	time "time"

	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/charm"
	charmtesting "github.com/juju/juju/core/charm/testing"
	domainapplication "github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/charm/charmdownloader"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
)

type asyncWorkerSuite struct {
	baseSuite
}

var _ = gc.Suite(&asyncWorkerSuite{})

func (s *asyncWorkerSuite) TestDownloadWorker(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appID := applicationtesting.GenApplicationUUID(c)
	charmID := charmtesting.GenCharmID(c)

	done := make(chan struct{})

	origin := charm.Origin{
		Source: charm.CharmHub,
		Hash:   "hash",
	}
	reserveInfo := domainapplication.CharmDownloadInfo{
		CharmUUID: charmID,
		Name:      "foo",
		Origin:    origin,
	}
	downloadResult := &charmdownloader.DownloadResult{
		Path:   "path",
		Origin: origin,
		Size:   int64(123),
	}

	s.applicationService.EXPECT().ReserveCharmDownload(gomock.Any(), appID).Return(reserveInfo, nil)
	s.downloader.EXPECT().Download(gomock.Any(), "foo", origin).Return(downloadResult, nil)
	s.applicationService.EXPECT().ResolveCharmDownload(gomock.Any(), appID, domainapplication.ResolveCharmDownload{
		CharmUUID: charmID,
		Path:      "path",
		Origin:    origin,
		Size:      int64(123),
	}).DoAndReturn(func(ctx context.Context, i application.ID, rcd domainapplication.ResolveCharmDownload) error {
		close(done)
		return nil
	})

	w := s.newWorker(c, appID)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for worker to finish")
	}

	workertest.CleanKill(c, w)
}

func (s *asyncWorkerSuite) TestDownloadWorkerRetriesDownload(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appID := applicationtesting.GenApplicationUUID(c)
	charmID := charmtesting.GenCharmID(c)

	done := make(chan struct{})

	origin := charm.Origin{
		Source: charm.CharmHub,
		Hash:   "hash",
	}
	reserveInfo := domainapplication.CharmDownloadInfo{
		CharmUUID: charmID,
		Name:      "foo",
		Origin:    origin,
	}
	downloadResult := &charmdownloader.DownloadResult{
		Path:   "path",
		Origin: origin,
		Size:   int64(123),
	}

	s.applicationService.EXPECT().ReserveCharmDownload(gomock.Any(), appID).Return(reserveInfo, nil)

	// Expect the download to fail twice before succeeding.

	gomock.InOrder(
		s.downloader.EXPECT().Download(gomock.Any(), "foo", origin).Return(downloadResult, errors.Errorf("boom")).Times(retryAttempts-1),
		s.downloader.EXPECT().Download(gomock.Any(), "foo", origin).Return(downloadResult, nil),
	)

	s.applicationService.EXPECT().ResolveCharmDownload(gomock.Any(), appID, domainapplication.ResolveCharmDownload{
		CharmUUID: charmID,
		Path:      "path",
		Origin:    origin,
		Size:      int64(123),
	}).DoAndReturn(func(ctx context.Context, i application.ID, rcd domainapplication.ResolveCharmDownload) error {
		close(done)
		return nil
	})

	w := s.newWorker(c, appID)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for worker to finish")
	}

	workertest.CleanKill(c, w)
}

func (s *asyncWorkerSuite) TestDownloadWorkerRetriesDownloadAndFails(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appID := applicationtesting.GenApplicationUUID(c)
	charmID := charmtesting.GenCharmID(c)

	done := make(chan struct{})

	origin := charm.Origin{
		Source: charm.CharmHub,
		Hash:   "hash",
	}
	reserveInfo := domainapplication.CharmDownloadInfo{
		CharmUUID: charmID,
		Name:      "foo",
		Origin:    origin,
	}
	downloadResult := &charmdownloader.DownloadResult{
		Path:   "path",
		Origin: origin,
		Size:   int64(123),
	}

	s.applicationService.EXPECT().ReserveCharmDownload(gomock.Any(), appID).Return(reserveInfo, nil)

	gomock.InOrder(
		s.downloader.EXPECT().Download(gomock.Any(), "foo", origin).Return(downloadResult, errors.Errorf("boom")).Times(retryAttempts-1),
		s.downloader.EXPECT().Download(gomock.Any(), "foo", origin).DoAndReturn(func(ctx context.Context, s string, o charm.Origin) (*charmdownloader.DownloadResult, error) {
			close(done)
			return nil, errors.Errorf("boom")
		}),
	)

	w := s.newWorker(c, appID)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for worker to finish")
	}

	err := workertest.CheckKilled(c, w)
	c.Assert(err, gc.ErrorMatches, `.*boom`)
}

func (s *asyncWorkerSuite) TestDownloadWorkerAlreadyDownloaded(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appID := applicationtesting.GenApplicationUUID(c)
	charmID := charmtesting.GenCharmID(c)

	done := make(chan struct{})

	origin := charm.Origin{
		Source: charm.CharmHub,
		Hash:   "hash",
	}
	reserveInfo := domainapplication.CharmDownloadInfo{
		CharmUUID: charmID,
		Name:      "foo",
		Origin:    origin,
	}

	s.applicationService.EXPECT().ReserveCharmDownload(gomock.Any(), appID).DoAndReturn(func(ctx context.Context, i application.ID) (domainapplication.CharmDownloadInfo, error) {
		close(done)
		return reserveInfo, applicationerrors.AlreadyDownloadingCharm
	})

	w := s.newWorker(c, appID)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for worker to finish")
	}

	workertest.CleanKill(c, w)
}

func (s *asyncWorkerSuite) TestDownloadWorkerAlreadyResolved(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appID := applicationtesting.GenApplicationUUID(c)
	charmID := charmtesting.GenCharmID(c)

	done := make(chan struct{})

	origin := charm.Origin{
		Source: charm.CharmHub,
		Hash:   "hash",
	}
	reserveInfo := domainapplication.CharmDownloadInfo{
		CharmUUID: charmID,
		Name:      "foo",
		Origin:    origin,
	}
	downloadResult := &charmdownloader.DownloadResult{
		Path:   "path",
		Origin: origin,
		Size:   int64(123),
	}

	s.applicationService.EXPECT().ReserveCharmDownload(gomock.Any(), appID).Return(reserveInfo, nil)
	s.downloader.EXPECT().Download(gomock.Any(), "foo", origin).Return(downloadResult, nil)
	s.applicationService.EXPECT().ResolveCharmDownload(gomock.Any(), appID, domainapplication.ResolveCharmDownload{
		CharmUUID: charmID,
		Path:      "path",
		Origin:    origin,
		Size:      int64(123),
	}).DoAndReturn(func(ctx context.Context, i application.ID, rcd domainapplication.ResolveCharmDownload) error {
		close(done)
		return applicationerrors.CharmAlreadyResolved
	})

	w := s.newWorker(c, appID)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for worker to finish")
	}

	workertest.CleanKill(c, w)
}

func (s *asyncWorkerSuite) newWorker(c *gc.C, appID application.ID) worker.Worker {
	return NewAsyncWorker(
		appID,
		s.applicationService,
		s.downloader,
		s.clock,
		loggertesting.WrapCheckLog(c),
	)
}
