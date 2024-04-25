// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader

import (
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/watcher"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/charmdownloader/mocks"
)

type charmDownloaderSuite struct {
	api     *mocks.MockCharmDownloaderAPI
	watcher *mocks.MockStringsWatcher
}

var _ = gc.Suite(&charmDownloaderSuite{})

func (s *charmDownloaderSuite) TestAsyncDownloadTrigger(c *gc.C) {
	defer s.setupMocks(c).Finish()

	changeCh := make(chan []string, 1)
	changeCh <- []string{"ufo", "cons", "piracy"}
	close(changeCh)
	s.watcher.EXPECT().Changes().DoAndReturn(func() watcher.StringsChannel {
		return changeCh
	}).AnyTimes()

	s.api.EXPECT().WatchApplicationsWithPendingCharms().DoAndReturn(func() (watcher.StringsWatcher, error) {
		return s.watcher, nil
	})
	s.api.EXPECT().DownloadApplicationCharms([]names.ApplicationTag{
		names.NewApplicationTag("ufo"),
		names.NewApplicationTag("cons"),
		names.NewApplicationTag("piracy"),
	}).Return(nil)

	worker, err := NewCharmDownloader(Config{
		Logger:             loggertesting.WrapCheckLog(c),
		CharmDownloaderAPI: s.api,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Wait for the worker to process the changes and exit when it detects
	// that changeCh has been closed.
	_ = worker.Wait()
}

func (s *charmDownloaderSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.api = mocks.NewMockCharmDownloaderAPI(ctrl)
	s.watcher = mocks.NewMockStringsWatcher(ctrl)
	s.watcher.EXPECT().Wait().Return(nil).AnyTimes()
	s.watcher.EXPECT().Kill().Return().AnyTimes()

	return ctrl
}
