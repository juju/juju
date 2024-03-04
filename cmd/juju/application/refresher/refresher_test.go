// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package refresher

import (
	"fmt"
	"os"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	commoncharm "github.com/juju/juju/api/common/charm"
	corebase "github.com/juju/juju/core/base"
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

	charmID2, err := f.Run(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charmID2, gc.DeepEquals, charmID)
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

	charmID2, err := f.Run(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charmID2, gc.DeepEquals, charmID)
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

	charmID2, err := f.Run(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charmID2, gc.DeepEquals, charmID)
}

type baseRefresherSuite struct{}

var _ = gc.Suite(&baseRefresherSuite{})

func (s *baseRefresherSuite) TestResolveCharm(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "meshuggah"
	curl := charm.MustParseURL(ref)
	newCurl := charm.MustParseURL(fmt.Sprintf("%s-1", ref))
	origin := commoncharm.Origin{
		Architecture: "amd64",
		Base:         corebase.MakeDefaultBase("ubuntu", "22.04"),
	}

	charmResolver := NewMockCharmResolver(ctrl)
	charmResolver.EXPECT().ResolveCharm(curl, origin, false).Return(newCurl, origin, []corebase.Base{}, nil)

	refresher := baseRefresher{
		charmRef:        "meshuggah",
		charmURL:        charm.MustParseURL("meshuggah"),
		charmResolver:   charmResolver,
		charmOrigin:     corecharm.Origin{Platform: corecharm.MustParsePlatform("amd64/ubuntu/22.04")},
		resolveOriginFn: charmHubOriginResolver,
		logger:          fakeLogger{},
	}
	url, obtainedOrigin, err := refresher.ResolveCharm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(url, gc.DeepEquals, charm.MustParseURL("ch:meshuggah-1"))
	c.Assert(obtainedOrigin, gc.DeepEquals, origin)
}

func (s *baseRefresherSuite) TestResolveCharmWithSeriesError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "meshuggah"
	curl := charm.MustParseURL(ref)
	newCurl := charm.MustParseURL(fmt.Sprintf("%s-1", ref))
	origin := commoncharm.Origin{
		Architecture: "amd64",
		Base:         corebase.MakeDefaultBase("ubuntu", "22.04"),
	}

	charmResolver := NewMockCharmResolver(ctrl)
	charmResolver.EXPECT().ResolveCharm(curl, origin, false).Return(newCurl, origin, []corebase.Base{corebase.MustParseBaseFromString("ubuntu@20.04")}, nil)

	refresher := baseRefresher{
		charmRef: "meshuggah",
		charmOrigin: corecharm.Origin{
			Platform: corecharm.MustParsePlatform("amd64/ubuntu/22.04"),
		},
		charmURL:        charm.MustParseURL("meshuggah"),
		charmResolver:   charmResolver,
		resolveOriginFn: charmHubOriginResolver,
		logger:          fakeLogger{},
	}
	_, _, err := refresher.ResolveCharm()
	c.Assert(err, gc.ErrorMatches, `cannot upgrade from single base "ubuntu@22.04" charm to a charm supporting \["ubuntu@20.04"\]. Use --force-series to override.`)
}

func (s *baseRefresherSuite) TestResolveCharmWithNoCharmURL(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "meshuggah"
	curl := charm.MustParseURL(ref)
	newCurl := charm.MustParseURL(fmt.Sprintf("%s-1", ref))
	origin := commoncharm.Origin{
		Architecture: "amd64",
		Base:         corebase.MakeDefaultBase("ubuntu", "22.04"),
	}

	charmResolver := NewMockCharmResolver(ctrl)
	charmResolver.EXPECT().ResolveCharm(curl, origin, false).Return(newCurl, origin, []corebase.Base{}, nil)

	refresher := baseRefresher{
		charmRef:        "meshuggah",
		charmResolver:   charmResolver,
		charmOrigin:     corecharm.Origin{Platform: corecharm.MustParsePlatform("amd64/ubuntu/22.04")},
		resolveOriginFn: charmHubOriginResolver,
		logger:          fakeLogger{},
	}
	_, _, err := refresher.ResolveCharm()
	c.Assert(err, gc.ErrorMatches, "unexpected charm URL")
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
	charmRepo.EXPECT().NewCharmAtPath(ref).Return(ch, curl, nil)

	cfg := refresherConfigWithOrigin(curl, ref, corecharm.MustParsePlatform("amd64/ubuntu/22.04"))

	refresher := (&factory{}).maybeReadLocal(charmAdder, charmRepo)
	task, err := refresher(cfg)
	c.Assert(err, jc.ErrorIsNil)

	charmID, err := task.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charmID.URL, gc.Equals, curl)
	c.Assert(charmID.Origin.Source, gc.Equals, corecharm.Local)
}

func (s *localCharmRefresherSuite) TestRefreshBecomesExhausted(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "local:meshuggah"
	curl := charm.MustParseURL(ref)

	charmAdder := NewMockCharmAdder(ctrl)
	charmRepo := NewMockCharmRepository(ctrl)
	charmRepo.EXPECT().NewCharmAtPath(ref).Return(nil, nil, os.ErrNotExist)

	cfg := refresherConfigWithOrigin(curl, ref, corecharm.MustParsePlatform("amd64/ubuntu/22.04"))

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
	charmRepo.EXPECT().NewCharmAtPath(ref).Return(nil, nil, errors.NotFoundf("fail"))

	cfg := refresherConfigWithOrigin(curl, ref, corecharm.MustParsePlatform("amd64/ubuntu/22.04"))

	refresher := (&factory{}).maybeReadLocal(charmAdder, charmRepo)
	task, err := refresher(cfg)
	c.Assert(err, jc.ErrorIsNil)

	_, err = task.Refresh()
	c.Assert(err, gc.ErrorMatches, `no charm found at "local:meshuggah"`)
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
		Source:       commoncharm.OriginCharmHub,
		Architecture: "amd64",
		Base:         corebase.MakeDefaultBase("ubuntu", "22.04"),
	}
	actualOrigin := origin
	actualOrigin.ID = "charmid"

	charmAdder := NewMockCharmAdder(ctrl)
	charmAdder.EXPECT().AddCharm(newCurl, origin, false).Return(actualOrigin, nil)

	charmResolver := NewMockCharmResolver(ctrl)
	charmResolver.EXPECT().ResolveCharm(curl, origin, false).Return(newCurl, origin, []corebase.Base{}, nil)

	cfg := refresherConfigWithOrigin(curl, ref, corecharm.MustParsePlatform("amd64/ubuntu/22.04"))

	refresher := (&factory{}).maybeCharmHub(charmAdder, charmResolver)
	task, err := refresher(cfg)
	c.Assert(err, jc.ErrorIsNil)

	charmID, err := task.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charmID, gc.DeepEquals, &CharmID{
		URL:    newCurl,
		Origin: actualOrigin.CoreCharmOrigin(),
	})
}

func (s *charmHubCharmRefresherSuite) TestRefreshWithNoOrigin(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "ch:meshuggah"
	curl := charm.MustParseURL(ref)
	newCurl := charm.MustParseURL(fmt.Sprintf("%s-1", ref))
	origin := commoncharm.Origin{
		Source:       commoncharm.OriginCharmHub,
		Architecture: "amd64",
		Base:         corebase.MakeDefaultBase("ubuntu", "22.04"),
	}

	charmAdder := NewMockCharmAdder(ctrl)
	charmAdder.EXPECT().AddCharm(newCurl, origin, false).Return(origin, nil)

	charmResolver := NewMockCharmResolver(ctrl)
	charmResolver.EXPECT().ResolveCharm(curl, origin, false).Return(newCurl, origin, []corebase.Base{}, nil)

	cfg := refresherConfigWithOrigin(curl, ref, corecharm.MustParsePlatform("amd64/ubuntu/22.04"))

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
		Source:       commoncharm.OriginCharmHub,
		Architecture: "amd64",
		Base:         corebase.MakeDefaultBase("ubuntu", "22.04"),
	}

	charmAdder := NewMockCharmAdder(ctrl)

	charmResolver := NewMockCharmResolver(ctrl)
	charmResolver.EXPECT().ResolveCharm(curl, origin, false).Return(curl, origin, []corebase.Base{}, nil)

	cfg := refresherConfigWithOrigin(curl, ref, corecharm.MustParsePlatform("amd64/ubuntu/22.04"))

	refresher := (&factory{}).maybeCharmHub(charmAdder, charmResolver)
	task, err := refresher(cfg)
	c.Assert(err, jc.ErrorIsNil)

	_, err = task.Refresh()
	c.Assert(err, gc.ErrorMatches, `charm "meshuggah": already up-to-date`)
}

func (s *charmHubCharmRefresherSuite) TestRefreshWithARevision(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "ch:meshuggah-1"
	curl := charm.MustParseURL(ref)
	origin := commoncharm.Origin{
		Source:       commoncharm.OriginCharmHub,
		Architecture: "amd64",
		Base:         corebase.MakeDefaultBase("ubuntu", "22.04"),
	}

	charmAdder := NewMockCharmAdder(ctrl)

	charmResolver := NewMockCharmResolver(ctrl)
	charmResolver.EXPECT().ResolveCharm(curl, origin, false).Return(curl, origin, []corebase.Base{}, nil)

	cfg := refresherConfigWithOrigin(curl, ref, corecharm.MustParsePlatform("amd64/ubuntu/22.04"))

	refresher := (&factory{}).maybeCharmHub(charmAdder, charmResolver)
	task, err := refresher(cfg)
	c.Assert(err, jc.ErrorIsNil)

	_, err = task.Refresh()
	c.Assert(err, gc.ErrorMatches, `charm "meshuggah", revision 1: already up-to-date`)
}

func (s *charmHubCharmRefresherSuite) TestRefreshWithOriginChannel(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "ch:meshuggah-1"
	curl := charm.MustParseURL(ref)
	origin := commoncharm.Origin{
		Source:       commoncharm.OriginCharmHub,
		Risk:         "beta",
		Architecture: "amd64",
		Base:         corebase.MakeDefaultBase("ubuntu", "22.04"),
	}

	charmAdder := NewMockCharmAdder(ctrl)

	charmResolver := NewMockCharmResolver(ctrl)
	charmResolver.EXPECT().ResolveCharm(curl, origin, false).Return(curl, origin, []corebase.Base{}, nil)

	cfg := refresherConfigWithOrigin(curl, ref, corecharm.MustParsePlatform("amd64/ubuntu/22.04"))
	cfg.CharmOrigin.Channel = &charm.Channel{
		Risk: charm.Edge,
	}
	cfg.CharmOrigin.Source = corecharm.CharmHub
	cfg.Channel = charm.Channel{
		Risk: charm.Beta,
	}

	refresher := (&factory{}).maybeCharmHub(charmAdder, charmResolver)
	task, err := refresher(cfg)
	c.Assert(err, jc.ErrorIsNil)

	_, err = task.Refresh()
	c.Assert(err, gc.ErrorMatches, `charm "meshuggah", revision 1: already up-to-date`)
}

func (s *charmHubCharmRefresherSuite) TestRefreshWithCharmSwitch(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "ch:aloupi-1"
	curl := charm.MustParseURL(ref)
	origin := commoncharm.Origin{
		Source:       commoncharm.OriginCharmHub,
		Risk:         "beta",
		Architecture: "amd64",
		Revision:     &curl.Revision,
		Base:         corebase.MakeDefaultBase("ubuntu", "22.04"),
	}

	charmAdder := NewMockCharmAdder(ctrl)

	charmResolver := NewMockCharmResolver(ctrl)
	charmResolver.EXPECT().ResolveCharm(curl, origin, true).Return(curl, origin, []corebase.Base{}, nil)

	cfg := refresherConfigWithOrigin(curl, ref, corecharm.MustParsePlatform("amd64/ubuntu/22.04"))
	cfg.Switch = true // flag this as a refresh --switch operation
	cfg.CharmOrigin.Channel = &charm.Channel{
		Risk: charm.Edge,
	}
	cfg.CharmOrigin.Source = corecharm.CharmHub
	cfg.Channel = charm.Channel{
		Risk: charm.Beta,
	}

	refresher := (&factory{}).maybeCharmHub(charmAdder, charmResolver)
	task, err := refresher(cfg)
	c.Assert(err, jc.ErrorIsNil)

	_, err = task.Refresh()
	c.Assert(err, gc.ErrorMatches, `charm "aloupi", revision 1: already up-to-date`)
}

func (s *charmHubCharmRefresherSuite) TestAllowed(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "ch:meshuggah"
	curl := charm.MustParseURL(ref)

	charmAdder := NewMockCharmAdder(ctrl)
	charmResolver := NewMockCharmResolver(ctrl)

	cfg := refresherConfigWithOrigin(curl, ref, corecharm.MustParsePlatform("amd64/ubuntu/22.04"))

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

	cfg := refresherConfigWithOrigin(curl, ref, corecharm.MustParsePlatform("amd64/ubuntu/22.04"))
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

	cfg := refresherConfigWithOrigin(curl, ref, corecharm.MustParsePlatform("amd64/ubuntu/22.04"))
	cfg.Switch = true

	refresher := (&factory{}).maybeCharmHub(charmAdder, charmResolver)
	task, err := refresher(cfg)
	c.Assert(err, jc.ErrorIsNil)

	allowed, err := task.Allowed(cfg)
	c.Assert(err, gc.ErrorMatches, "trap")
	c.Assert(allowed, jc.IsFalse)
}

func (s *charmHubCharmRefresherSuite) TestCharmHubResolveOriginEmpty(c *gc.C) {
	origin := corecharm.Origin{}
	channel := charm.Channel{}
	result, err := charmHubOriginResolver(nil, origin, channel)
	c.Assert(err, jc.ErrorIsNil)
	coreOrigin, err := commoncharm.CoreCharmOrigin(origin)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, coreOrigin)
}

func (s *charmHubCharmRefresherSuite) TestCharmHubResolveOrigin(c *gc.C) {
	track := "meshuggah"
	origin := corecharm.Origin{}
	channel := charm.Channel{
		Track: track,
	}
	result, err := charmHubOriginResolver(nil, origin, channel)
	c.Assert(err, jc.ErrorIsNil)
	coreOrigin, err := commoncharm.CoreCharmOrigin(corecharm.Origin{
		Channel: &charm.Channel{
			Track: track,
			Risk:  "stable",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, coreOrigin)
}

func (s *charmHubCharmRefresherSuite) TestCharmHubResolveOriginEmptyTrackNonEmptyChannel(c *gc.C) {
	origin := corecharm.Origin{
		Channel: &charm.Channel{},
	}
	channel := charm.Channel{
		Risk: "edge",
	}
	result, err := charmHubOriginResolver(nil, origin, channel)
	c.Assert(err, jc.ErrorIsNil)
	coreOrigin, err := commoncharm.CoreCharmOrigin(corecharm.Origin{
		Channel: &charm.Channel{
			Risk: "edge",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, coreOrigin)
}

func (s *charmHubCharmRefresherSuite) TestCharmHubResolveOriginEmptyTrackEmptyChannel(c *gc.C) {
	origin := corecharm.Origin{}
	channel := charm.Channel{
		Risk: "edge",
	}
	result, err := charmHubOriginResolver(nil, origin, channel)
	c.Assert(err, jc.ErrorIsNil)
	coreOrigin, err := commoncharm.CoreCharmOrigin(corecharm.Origin{
		Channel: &charm.Channel{},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, coreOrigin)
}

func refresherConfigWithOrigin(curl *charm.URL, ref string, platform corecharm.Platform) RefresherConfig {
	rc := RefresherConfig{
		ApplicationName: "winnie",
		CharmURL:        curl,
		CharmRef:        ref,
		Logger:          &fakeLogger{},
	}
	rc.CharmOrigin = corecharm.Origin{
		Source:   corecharm.CharmHub,
		Channel:  &charm.Channel{},
		Platform: platform,
	}
	return rc
}

type fakeLogger struct {
}

func (fakeLogger) Infof(_ string, _ ...interface{})    {}
func (fakeLogger) Warningf(_ string, _ ...interface{}) {}
func (fakeLogger) Verbosef(_ string, _ ...interface{}) {}
