// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package asynccharmdownloader

import (
	"context"
	"net/url"
	stdtesting "testing"
	time "time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	charmtesting "github.com/juju/juju/core/charm/testing"
	domainapplication "github.com/juju/juju/domain/application"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/charm/charmdownloader"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
)

type asyncWorkerSuite struct {
	baseSuite
}

func TestAsyncWorkerSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &asyncWorkerSuite{})
}

func (s *asyncWorkerSuite) TestDownloadWorker(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appID := applicationtesting.GenApplicationUUID(c)
	charmID := charmtesting.GenCharmID(c)

	done := make(chan struct{})

	reserveInfo := domainapplication.CharmDownloadInfo{
		CharmUUID: charmID,
		Name:      "foo",
		SHA256:    "hash",
		DownloadInfo: applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       123,
		},
	}
	downloadResult := &charmdownloader.DownloadResult{
		Path: "path",
		Size: int64(123),
	}

	curl, err := url.Parse("https://example.com/foo")
	c.Assert(err, tc.ErrorIsNil)

	s.applicationService.EXPECT().GetAsyncCharmDownloadInfo(gomock.Any(), appID).Return(reserveInfo, nil)
	s.downloader.EXPECT().Download(gomock.Any(), curl, "hash").Return(downloadResult, nil)
	s.applicationService.EXPECT().ResolveCharmDownload(gomock.Any(), appID, domainapplication.ResolveCharmDownload{
		CharmUUID: charmID,
		Path:      "path",
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

func (s *asyncWorkerSuite) TestDownloadWorkerRetriesDownload(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appID := applicationtesting.GenApplicationUUID(c)
	charmID := charmtesting.GenCharmID(c)

	done := make(chan struct{})

	reserveInfo := domainapplication.CharmDownloadInfo{
		CharmUUID: charmID,
		Name:      "foo",
		SHA256:    "hash",
		DownloadInfo: applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       123,
		},
	}
	downloadResult := &charmdownloader.DownloadResult{
		Path: "path",
		Size: int64(123),
	}

	curl, err := url.Parse("https://example.com/foo")
	c.Assert(err, tc.ErrorIsNil)

	s.applicationService.EXPECT().GetAsyncCharmDownloadInfo(gomock.Any(), appID).Return(reserveInfo, nil)

	// Expect the download to fail twice before succeeding.

	gomock.InOrder(
		s.downloader.EXPECT().Download(gomock.Any(), curl, "hash").Return(downloadResult, errors.Errorf("boom")).Times(retryAttempts-1),
		s.downloader.EXPECT().Download(gomock.Any(), curl, "hash").Return(downloadResult, nil),
	)

	s.applicationService.EXPECT().ResolveCharmDownload(gomock.Any(), appID, domainapplication.ResolveCharmDownload{
		CharmUUID: charmID,
		Path:      "path",
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

func (s *asyncWorkerSuite) TestDownloadWorkerRetriesDownloadAndFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appID := applicationtesting.GenApplicationUUID(c)
	charmID := charmtesting.GenCharmID(c)

	done := make(chan struct{})

	reserveInfo := domainapplication.CharmDownloadInfo{
		CharmUUID: charmID,
		Name:      "foo",
		SHA256:    "hash",
		DownloadInfo: applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       123,
		},
	}
	downloadResult := &charmdownloader.DownloadResult{
		Path: "path",
		Size: int64(123),
	}

	curl, err := url.Parse("https://example.com/foo")
	c.Assert(err, tc.ErrorIsNil)

	s.applicationService.EXPECT().GetAsyncCharmDownloadInfo(gomock.Any(), appID).Return(reserveInfo, nil)

	gomock.InOrder(
		s.downloader.EXPECT().Download(gomock.Any(), curl, "hash").Return(downloadResult, errors.Errorf("boom")).Times(retryAttempts-1),
		s.downloader.EXPECT().Download(gomock.Any(), curl, "hash").DoAndReturn(func(ctx context.Context, u *url.URL, h string) (*charmdownloader.DownloadResult, error) {
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

	err = workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorMatches, `.*boom`)
}

func (s *asyncWorkerSuite) TestDownloadWorkerAlreadyDownloaded(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appID := applicationtesting.GenApplicationUUID(c)
	charmID := charmtesting.GenCharmID(c)

	done := make(chan struct{})

	reserveInfo := domainapplication.CharmDownloadInfo{
		CharmUUID: charmID,
		Name:      "foo",
		SHA256:    "hash",
		DownloadInfo: applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       123,
		},
	}

	s.applicationService.EXPECT().GetAsyncCharmDownloadInfo(gomock.Any(), appID).DoAndReturn(func(ctx context.Context, i application.ID) (domainapplication.CharmDownloadInfo, error) {
		close(done)
		return reserveInfo, applicationerrors.CharmAlreadyAvailable
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

func (s *asyncWorkerSuite) TestDownloadWorkerAlreadyResolved(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appID := applicationtesting.GenApplicationUUID(c)
	charmID := charmtesting.GenCharmID(c)

	done := make(chan struct{})

	reserveInfo := domainapplication.CharmDownloadInfo{
		CharmUUID: charmID,
		Name:      "foo",
		SHA256:    "hash",
		DownloadInfo: applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       123,
		},
	}
	downloadResult := &charmdownloader.DownloadResult{
		Path: "path",
		Size: int64(123),
	}

	curl, err := url.Parse("https://example.com/foo")
	c.Assert(err, tc.ErrorIsNil)

	s.applicationService.EXPECT().GetAsyncCharmDownloadInfo(gomock.Any(), appID).Return(reserveInfo, nil)
	s.downloader.EXPECT().Download(gomock.Any(), curl, "hash").Return(downloadResult, nil)
	s.applicationService.EXPECT().ResolveCharmDownload(gomock.Any(), appID, domainapplication.ResolveCharmDownload{
		CharmUUID: charmID,
		Path:      "path",
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

func (s *asyncWorkerSuite) newWorker(c *tc.C, appID application.ID) worker.Worker {
	return NewAsyncDownloadWorker(
		appID,
		s.applicationService,
		s.downloader,
		s.clock,
		loggertesting.WrapCheckLog(c),
	)
}
