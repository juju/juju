// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package refresher

import (
	"fmt"
	"os"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	"github.com/juju/charmrepo/v6"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
)

type refresherFactorySuite struct{}

var _ = gc.Suite(&refresherFactorySuite{})

func (s *refresherFactorySuite) TestRefresh(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "meshuggah"
	curl := charm.MustParseURL(ref)
	cfg := RefresherConfig{}
	charmID := &CharmID{
		URL: curl,
	}

	refresher := NewMockRefresher(ctrl)
	refresher.EXPECT().Allowed(cfg).Return(true, nil)
	refresher.EXPECT().Refresh().Return(charmID, nil)

	f := &factory{
		refreshers: []RefresherFn{
			func(cfg RefresherConfig) (Refresher, error) {
				return refresher, nil
			},
		},
	}

	charmID, err := f.Run(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charmID, gc.DeepEquals, charmID)
}

func (s *refresherFactorySuite) TestRefreshNotAllowed(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "meshuggah"
	cfg := RefresherConfig{
		CharmRef: ref,
	}

	refresher := NewMockRefresher(ctrl)
	refresher.EXPECT().Allowed(cfg).Return(false, nil)

	f := &factory{
		refreshers: []RefresherFn{
			func(cfg RefresherConfig) (Refresher, error) {
				return refresher, nil
			},
		},
	}

	_, err := f.Run(cfg)
	c.Assert(err, gc.ErrorMatches, `unable to refresh "meshuggah"`)
}

func (s *refresherFactorySuite) TestRefreshCallsAllRefreshers(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "meshuggah"
	curl := charm.MustParseURL(ref)
	cfg := RefresherConfig{}
	charmID := &CharmID{
		URL: curl,
	}

	refresher0 := NewMockRefresher(ctrl)
	refresher0.EXPECT().Allowed(cfg).Return(false, nil)

	refresher1 := NewMockRefresher(ctrl)
	refresher1.EXPECT().Allowed(cfg).Return(true, nil)
	refresher1.EXPECT().Refresh().Return(charmID, nil)

	f := &factory{
		refreshers: []RefresherFn{
			func(cfg RefresherConfig) (Refresher, error) {
				return refresher0, nil
			},
			func(cfg RefresherConfig) (Refresher, error) {
				return refresher1, nil
			},
		},
	}

	charmID, err := f.Run(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charmID, gc.DeepEquals, charmID)
}

func (s *refresherFactorySuite) TestRefreshCallsRefreshersEvenAfterExhaustedError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "meshuggah"
	curl := charm.MustParseURL(ref)
	cfg := RefresherConfig{}
	charmID := &CharmID{
		URL: curl,
	}

	refresher0 := NewMockRefresher(ctrl)
	refresher0.EXPECT().Allowed(cfg).Return(false, nil)

	refresher1 := NewMockRefresher(ctrl)
	refresher1.EXPECT().Allowed(cfg).Return(true, nil)
	refresher1.EXPECT().Refresh().Return(nil, ErrExhausted)

	refresher2 := NewMockRefresher(ctrl)
	refresher2.EXPECT().Allowed(cfg).Return(true, nil)
	refresher2.EXPECT().Refresh().Return(charmID, nil)

	f := &factory{
		refreshers: []RefresherFn{
			func(cfg RefresherConfig) (Refresher, error) {
				return refresher0, nil
			},
			func(cfg RefresherConfig) (Refresher, error) {
				return refresher1, nil
			},
			func(cfg RefresherConfig) (Refresher, error) {
				return refresher2, nil
			},
		},
	}

	charmID, err := f.Run(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charmID, gc.DeepEquals, charmID)
}

type localCharmRefresherSuite struct{}

var _ = gc.Suite(&localCharmRefresherSuite{})

func (s *localCharmRefresherSuite) TestRefresh(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "local:meshuggah"
	curl := charm.MustParseURL(ref)

	ch := NewMockCharm(ctrl)
	ch.EXPECT().Meta().Return(&charm.Meta{
		Name: "meshuggah",
	})

	charmAdder := NewMockCharmAdder(ctrl)
	charmAdder.EXPECT().AddLocalCharm(curl, ch, false).Return(curl, nil)

	charmRepo := NewMockCharmRepository(ctrl)
	charmRepo.EXPECT().NewCharmAtPathForceSeries(ref, "", false).Return(ch, curl, nil)

	cfg := basicRefresherConfig(curl, ref)

	refresher := (&factory{}).maybeReadLocal(charmAdder, charmRepo)
	task, err := refresher(cfg)
	c.Assert(err, jc.ErrorIsNil)

	charmID, err := task.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charmID, gc.DeepEquals, &CharmID{
		URL: curl,
	})
}

func (s *localCharmRefresherSuite) TestRefreshBecomesExhausted(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "local:meshuggah"
	curl := charm.MustParseURL(ref)

	charmAdder := NewMockCharmAdder(ctrl)
	charmRepo := NewMockCharmRepository(ctrl)
	charmRepo.EXPECT().NewCharmAtPathForceSeries(ref, "", false).Return(nil, nil, os.ErrNotExist)

	cfg := basicRefresherConfig(curl, ref)

	refresher := (&factory{}).maybeReadLocal(charmAdder, charmRepo)
	task, err := refresher(cfg)
	c.Assert(err, jc.ErrorIsNil)

	_, err = task.Refresh()
	c.Assert(err, gc.Equals, ErrExhausted)
}

func (s *localCharmRefresherSuite) TestRefreshDoesNotFindLocal(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "local:meshuggah"
	curl := charm.MustParseURL(ref)

	charmAdder := NewMockCharmAdder(ctrl)
	charmRepo := NewMockCharmRepository(ctrl)
	charmRepo.EXPECT().NewCharmAtPathForceSeries(ref, "", false).Return(nil, nil, &charmrepo.NotFoundError{})

	cfg := basicRefresherConfig(curl, ref)

	refresher := (&factory{}).maybeReadLocal(charmAdder, charmRepo)
	task, err := refresher(cfg)
	c.Assert(err, jc.ErrorIsNil)

	_, err = task.Refresh()
	c.Assert(err, gc.ErrorMatches, `no charm found at "local:meshuggah"`)
}

type charmStoreCharmRefresherSuite struct{}

var _ = gc.Suite(&charmStoreCharmRefresherSuite{})

func (s *charmStoreCharmRefresherSuite) TestRefresh(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "cs:meshuggah"
	curl := charm.MustParseURL(ref)
	newCurl := charm.MustParseURL(fmt.Sprintf("%s-1", ref))
	origin := commoncharm.Origin{
		Source:       commoncharm.OriginCharmStore,
		Architecture: arch.DefaultArchitecture,
	}

	authorizer := NewMockMacaroonGetter(ctrl)

	charmAdder := NewMockCharmAdder(ctrl)
	charmAdder.EXPECT().AddCharm(newCurl, origin, false).Return(origin, nil)

	charmResolver := NewMockCharmResolver(ctrl)
	charmResolver.EXPECT().ResolveCharm(curl, origin).Return(newCurl, origin, []string{}, nil)

	cfg := basicRefresherConfig(curl, ref)

	refresher := (&factory{}).maybeCharmStore(authorizer, charmAdder, charmResolver)
	task, err := refresher(cfg)
	c.Assert(err, jc.ErrorIsNil)

	charmID, err := task.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charmID, gc.DeepEquals, &CharmID{
		URL:    newCurl,
		Origin: origin.CoreCharmOrigin(),
	})
}

func (s *charmStoreCharmRefresherSuite) TestRefreshWithNoUpdates(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "cs:meshuggah"
	curl := charm.MustParseURL(ref)
	origin := commoncharm.Origin{
		Source:       commoncharm.OriginCharmStore,
		Architecture: arch.DefaultArchitecture,
	}

	authorizer := NewMockMacaroonGetter(ctrl)
	charmAdder := NewMockCharmAdder(ctrl)

	charmResolver := NewMockCharmResolver(ctrl)
	charmResolver.EXPECT().ResolveCharm(curl, origin).Return(curl, origin, []string{}, nil)

	cfg := basicRefresherConfig(curl, ref)

	refresher := (&factory{}).maybeCharmStore(authorizer, charmAdder, charmResolver)
	task, err := refresher(cfg)
	c.Assert(err, jc.ErrorIsNil)

	_, err = task.Refresh()
	c.Assert(err, gc.ErrorMatches, `already running latest charm "meshuggah"`)
}

func (s *charmStoreCharmRefresherSuite) TestRefreshWithARevision(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "cs:meshuggah-1"
	curl := charm.MustParseURL(ref)
	origin := commoncharm.Origin{
		Source:       commoncharm.OriginCharmStore,
		Architecture: arch.DefaultArchitecture,
	}

	authorizer := NewMockMacaroonGetter(ctrl)
	charmAdder := NewMockCharmAdder(ctrl)

	charmResolver := NewMockCharmResolver(ctrl)
	charmResolver.EXPECT().ResolveCharm(curl, origin).Return(curl, origin, []string{}, nil)

	cfg := basicRefresherConfig(curl, ref)

	refresher := (&factory{}).maybeCharmStore(authorizer, charmAdder, charmResolver)
	task, err := refresher(cfg)
	c.Assert(err, jc.ErrorIsNil)

	_, err = task.Refresh()
	c.Assert(err, gc.ErrorMatches, `already running specified charm "meshuggah", revision 1`)
}

type charmHubCharmRefresherSuite struct{}

var _ = gc.Suite(&charmHubCharmRefresherSuite{})

func (s *charmHubCharmRefresherSuite) TestRefresh(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "ch:meshuggah"
	curl := charm.MustParseURL(ref)
	newCurl := charm.MustParseURL(fmt.Sprintf("%s-1", ref))
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Series: "bionic",
	}

	charmAdder := NewMockCharmAdder(ctrl)
	charmAdder.EXPECT().AddCharm(newCurl, origin, false).Return(origin, nil)

	charmResolver := NewMockCharmResolver(ctrl)
	charmResolver.EXPECT().ResolveCharm(curl, origin).Return(newCurl, origin, []string{}, nil)

	cfg := refresherConfigWithOrigin(curl, ref, "bionic")
	cfg.DeployedSeries = "bionic"

	refresher := (&factory{}).maybeCharmHub(charmAdder, charmResolver)
	task, err := refresher(cfg)
	c.Assert(err, jc.ErrorIsNil)

	charmID, err := task.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charmID, gc.DeepEquals, &CharmID{
		URL:    newCurl,
		Origin: origin.CoreCharmOrigin(),
	})
}

func (s *charmHubCharmRefresherSuite) TestRefreshWithNoUpdates(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "ch:meshuggah"
	curl := charm.MustParseURL(ref)
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
	}

	charmAdder := NewMockCharmAdder(ctrl)

	charmResolver := NewMockCharmResolver(ctrl)
	charmResolver.EXPECT().ResolveCharm(curl, origin).Return(curl, origin, []string{}, nil)

	cfg := refresherConfigWithOrigin(curl, ref, "")

	refresher := (&factory{}).maybeCharmHub(charmAdder, charmResolver)
	task, err := refresher(cfg)
	c.Assert(err, jc.ErrorIsNil)

	_, err = task.Refresh()
	c.Assert(err, gc.ErrorMatches, `already running latest charm "meshuggah"`)
}

func (s *charmHubCharmRefresherSuite) TestRefreshWithARevision(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "ch:meshuggah-1"
	curl := charm.MustParseURL(ref)
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
	}

	charmAdder := NewMockCharmAdder(ctrl)

	charmResolver := NewMockCharmResolver(ctrl)
	charmResolver.EXPECT().ResolveCharm(curl, origin).Return(curl, origin, []string{}, nil)

	cfg := refresherConfigWithOrigin(curl, ref, "")

	refresher := (&factory{}).maybeCharmHub(charmAdder, charmResolver)
	task, err := refresher(cfg)
	c.Assert(err, jc.ErrorIsNil)

	_, err = task.Refresh()
	c.Assert(err, gc.ErrorMatches, `already running specified charm "meshuggah", revision 1`)
}

func (s *charmHubCharmRefresherSuite) TestRefreshWithOriginChannel(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "ch:meshuggah-1"
	curl := charm.MustParseURL(ref)
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Risk:   "beta",
	}

	charmAdder := NewMockCharmAdder(ctrl)

	charmResolver := NewMockCharmResolver(ctrl)
	charmResolver.EXPECT().ResolveCharm(curl, origin).Return(curl, origin, []string{}, nil)

	cfg := basicRefresherConfig(curl, ref)
	cfg.CharmOrigin = corecharm.Origin{
		Source: corecharm.CharmHub,
		Channel: &charm.Channel{
			Risk: charm.Edge,
		},
	}
	cfg.Channel = charm.Channel{
		Risk: charm.Beta,
	}

	refresher := (&factory{}).maybeCharmHub(charmAdder, charmResolver)
	task, err := refresher(cfg)
	c.Assert(err, jc.ErrorIsNil)

	_, err = task.Refresh()
	c.Assert(err, gc.ErrorMatches, `already running specified charm "meshuggah", revision 1`)
}

func (s *charmHubCharmRefresherSuite) TestAllowed(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "ch:meshuggah"
	curl := charm.MustParseURL(ref)

	charmAdder := NewMockCharmAdder(ctrl)
	charmResolver := NewMockCharmResolver(ctrl)

	cfg := refresherConfigWithOrigin(curl, ref, "")
	cfg.DeployedSeries = "bionic"

	refresher := (&factory{}).maybeCharmHub(charmAdder, charmResolver)
	task, err := refresher(cfg)
	c.Assert(err, jc.ErrorIsNil)

	allowed, err := task.Allowed(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allowed, jc.IsTrue)
}

func (s *charmHubCharmRefresherSuite) TestAllowedWithSwitch(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "ch:meshuggah"
	curl := charm.MustParseURL(ref)

	charmAdder := NewMockCharmAdder(ctrl)
	charmAdder.EXPECT().CheckCharmPlacement("winnie", curl).Return(nil)

	charmResolver := NewMockCharmResolver(ctrl)

	cfg := refresherConfigWithOrigin(curl, ref, "")
	cfg.DeployedSeries = "bionic"
	cfg.Switch = true

	refresher := (&factory{}).maybeCharmHub(charmAdder, charmResolver)
	task, err := refresher(cfg)
	c.Assert(err, jc.ErrorIsNil)

	allowed, err := task.Allowed(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allowed, jc.IsTrue)
}

func (s *charmHubCharmRefresherSuite) TestAllowedError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "ch:meshuggah"
	curl := charm.MustParseURL(ref)

	charmAdder := NewMockCharmAdder(ctrl)
	charmAdder.EXPECT().CheckCharmPlacement("winnie", curl).Return(errors.Errorf("trap"))

	charmResolver := NewMockCharmResolver(ctrl)

	cfg := refresherConfigWithOrigin(curl, ref, "")
	cfg.DeployedSeries = "bionic"
	cfg.Switch = true

	refresher := (&factory{}).maybeCharmHub(charmAdder, charmResolver)
	task, err := refresher(cfg)
	c.Assert(err, jc.ErrorIsNil)

	allowed, err := task.Allowed(cfg)
	c.Assert(err, gc.ErrorMatches, "trap")
	c.Assert(allowed, jc.IsFalse)
}

func basicRefresherConfig(curl *charm.URL, ref string) RefresherConfig {
	return RefresherConfig{
		ApplicationName: "winnie",
		CharmURL:        curl,
		CharmRef:        ref,
		Logger:          &fakeLogger{},
	}
}

func refresherConfigWithOrigin(curl *charm.URL, ref, series string) RefresherConfig {
	rc := basicRefresherConfig(curl, ref)
	rc.CharmOrigin = corecharm.Origin{
		Source:  corecharm.CharmHub,
		Channel: &charm.Channel{},
		Platform: corecharm.Platform{
			Series: series,
		},
	}
	return rc
}

type fakeLogger struct {
}

func (fakeLogger) Infof(_ string, _ ...interface{})    {}
func (fakeLogger) Warningf(_ string, _ ...interface{}) {}
func (fakeLogger) Verbosef(_ string, _ ...interface{}) {}
