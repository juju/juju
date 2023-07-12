// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader_test

import (
	"net/http"
	"time"

	"github.com/juju/charm/v11"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/client/charms/services"
	"github.com/juju/juju/apiserver/facades/controller/charmdownloader"
	"github.com/juju/juju/apiserver/facades/controller/charmdownloader/mocks"
	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type charmDownloaderSuite struct {
	clk              *testclock.Clock
	authChecker      *mocks.MockAuthChecker
	resourcesBackend *mocks.MockResourcesBackend
	stateBackend     *mocks.MockStateBackend
	modelBackend     *mocks.MockModelBackend
	downloader       *mocks.MockDownloader
	api              *charmdownloader.CharmDownloaderAPI
}

var _ = gc.Suite(&charmDownloaderSuite{})

func (s *charmDownloaderSuite) TestWatchApplicationsWithPendingCharmsAuthChecks(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.authChecker.EXPECT().AuthController().Return(false)

	_, err := s.api.WatchApplicationsWithPendingCharms()
	c.Assert(err, gc.Equals, apiservererrors.ErrPerm, gc.Commentf("expected ErrPerm when not authenticating as the controller"))
}

func (s *charmDownloaderSuite) TestWatchApplicationsWithPendingCharms(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	watcher := mocks.NewMockStringsWatcher(ctrl)
	watcher.EXPECT().Changes().DoAndReturn(func() <-chan []string {
		ch := make(chan []string, 1)
		ch <- []string{"ufo", "cons", "piracy"}
		return ch
	})
	s.authChecker.EXPECT().AuthController().Return(true)
	s.stateBackend.EXPECT().WatchApplicationsWithPendingCharms().DoAndReturn(func() state.StringsWatcher {
		return watcher
	})
	s.resourcesBackend.EXPECT().Register(watcher).Return("42")

	got, err := s.api.WatchApplicationsWithPendingCharms()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got.StringsWatcherId, gc.Equals, "42")
	c.Assert(got.Changes, gc.DeepEquals, []string{"ufo", "cons", "piracy"})
}

func (s *charmDownloaderSuite) TestDownloadApplicationCharmsAuthChecks(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.authChecker.EXPECT().AuthController().Return(false)

	_, err := s.api.DownloadApplicationCharms(params.Entities{})
	c.Assert(err, gc.Equals, apiservererrors.ErrPerm, gc.Commentf("expected ErrPerm when not authenticating as the controller"))
}

func (s *charmDownloaderSuite) TestDownloadApplicationCharmsDeploy(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	charmURL := charm.MustParseURL("ch:focal/testme-1")
	resolvedOrigin := corecharm.Origin{
		Source: "charm-hub",
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
		},
	}

	pendingCharm := mocks.NewMockCharm(ctrl)
	pendingCharm.EXPECT().URL().Return(charmURL)

	app := mocks.NewMockApplication(ctrl)
	app.EXPECT().CharmPendingToBeDownloaded().Return(true)
	app.EXPECT().Charm().Return(pendingCharm, false, nil)
	app.EXPECT().CharmOrigin().Return(&resolvedOrigin)

	downloadedOrigin := resolvedOrigin
	downloadedOrigin.ID = "test-charm-id"
	downloadedOrigin.Hash = "test-charm-hash"
	app.EXPECT().SetDownloadedIDAndHash(downloadedOrigin.ID, downloadedOrigin.Hash).Return(nil)

	s.authChecker.EXPECT().AuthController().Return(true)
	s.stateBackend.EXPECT().Application("ufo").Return(app, nil)
	s.downloader.EXPECT().DownloadAndStore(charmURL, resolvedOrigin, false).Return(downloadedOrigin, nil)

	got, err := s.api.DownloadApplicationCharms(params.Entities{
		Entities: []params.Entity{
			{
				Tag: names.NewApplicationTag("ufo").String(),
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got.Combine(), jc.ErrorIsNil)
}

func (s *charmDownloaderSuite) TestDownloadApplicationCharmsDeployMultiAppOneCharm(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	charmURL := charm.MustParseURL("ch:focal/testme-1")
	resolvedOrigin := corecharm.Origin{
		Source: "charm-hub",
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
		},
	}

	pendingCharm := mocks.NewMockCharm(ctrl)
	pendingCharm.EXPECT().URL().Return(charmURL).AnyTimes()

	appOne := mocks.NewMockApplication(ctrl)
	appOne.EXPECT().CharmPendingToBeDownloaded().Return(true)
	appOne.EXPECT().Charm().Return(pendingCharm, false, nil)
	appOne.EXPECT().CharmOrigin().Return(&resolvedOrigin)

	appTwo := mocks.NewMockApplication(ctrl)
	appTwo.EXPECT().CharmPendingToBeDownloaded().Return(true)
	appTwo.EXPECT().Charm().Return(pendingCharm, false, nil)
	appTwo.EXPECT().CharmOrigin().Return(&resolvedOrigin)

	downloadedOrigin := resolvedOrigin
	downloadedOrigin.ID = "test-charm-id"
	downloadedOrigin.Hash = "test-charm-hash"
	appOne.EXPECT().SetDownloadedIDAndHash(downloadedOrigin.ID, downloadedOrigin.Hash).Return(nil)
	appTwo.EXPECT().SetDownloadedIDAndHash(downloadedOrigin.ID, downloadedOrigin.Hash).Return(nil)

	s.authChecker.EXPECT().AuthController().Return(true)
	s.stateBackend.EXPECT().Application("ufo").Return(appOne, nil)
	s.stateBackend.EXPECT().Application("another-ufo").Return(appTwo, nil)
	s.downloader.EXPECT().DownloadAndStore(charmURL, resolvedOrigin, false).Return(downloadedOrigin, nil).AnyTimes()

	got, err := s.api.DownloadApplicationCharms(params.Entities{
		Entities: []params.Entity{
			{
				Tag: names.NewApplicationTag("ufo").String(),
			}, {
				Tag: names.NewApplicationTag("another-ufo").String(),
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got.Combine(), jc.ErrorIsNil)
}

func (s *charmDownloaderSuite) TestDownloadApplicationCharmsRefresh(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	charmURL := charm.MustParseURL("ch:focal/testme-1")
	resolvedOrigin := corecharm.Origin{
		Source: "charm-hub",
		ID:     "test-charm-id",
		Hash:   "test-charm-hash",
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
		},
	}

	pendingCharm := mocks.NewMockCharm(ctrl)
	pendingCharm.EXPECT().URL().Return(charmURL)

	app := mocks.NewMockApplication(ctrl)
	app.EXPECT().CharmPendingToBeDownloaded().Return(true)
	app.EXPECT().Charm().Return(pendingCharm, false, nil)
	app.EXPECT().CharmOrigin().Return(&resolvedOrigin)

	downloadedOrigin := resolvedOrigin
	downloadedOrigin.Hash = "test-charm-hash-two"
	app.EXPECT().SetDownloadedIDAndHash(downloadedOrigin.ID, downloadedOrigin.Hash).Return(nil)

	s.authChecker.EXPECT().AuthController().Return(true)
	s.stateBackend.EXPECT().Application("ufo").Return(app, nil)
	s.downloader.EXPECT().DownloadAndStore(charmURL, resolvedOrigin, false).Return(downloadedOrigin, nil)

	got, err := s.api.DownloadApplicationCharms(params.Entities{
		Entities: []params.Entity{
			{
				Tag: names.NewApplicationTag("ufo").String(),
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got.Combine(), jc.ErrorIsNil)
}

func (s *charmDownloaderSuite) TestDownloadApplicationCharmsSetStatusIfDownloadFails(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	charmURL := charm.MustParseURL("ch:focal/testme-1")
	resolvedOrigin := corecharm.Origin{
		Source: "charm-hub",
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
		},
	}

	pendingCharm := mocks.NewMockCharm(ctrl)
	pendingCharm.EXPECT().URL().Return(charmURL)

	app := mocks.NewMockApplication(ctrl)
	app.EXPECT().CharmPendingToBeDownloaded().Return(true)
	app.EXPECT().Charm().Return(pendingCharm, false, nil)
	app.EXPECT().CharmOrigin().Return(&resolvedOrigin)

	s.authChecker.EXPECT().AuthController().Return(true)
	s.stateBackend.EXPECT().Application("ufo").Return(app, nil)
	s.downloader.EXPECT().DownloadAndStore(charmURL, resolvedOrigin, false).Return(corecharm.Origin{}, errors.NotFoundf("charm"))

	got, err := s.api.DownloadApplicationCharms(params.Entities{
		Entities: []params.Entity{
			{
				Tag: names.NewApplicationTag("ufo").String(),
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got.Combine(), gc.ErrorMatches, ".*charm not found.*")
}

func (s *charmDownloaderSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.clk = testclock.NewClock(time.Now())
	s.authChecker = mocks.NewMockAuthChecker(ctrl)
	s.resourcesBackend = mocks.NewMockResourcesBackend(ctrl)
	s.stateBackend = mocks.NewMockStateBackend(ctrl)
	s.modelBackend = mocks.NewMockModelBackend(ctrl)
	s.downloader = mocks.NewMockDownloader(ctrl)

	s.api = charmdownloader.NewAPI(
		s.authChecker,
		s.resourcesBackend,
		s.stateBackend,
		s.modelBackend,
		s.clk,
		http.DefaultClient,
		nil,
		func(services.CharmDownloaderConfig) (charmdownloader.Downloader, error) {
			return s.downloader, nil
		},
	)

	return ctrl
}
