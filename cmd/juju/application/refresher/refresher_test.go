// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package refresher

import (
	"fmt"
	"os"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	commoncharm "github.com/juju/juju/api/common/charm"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/internal/charm"
)

type refresherFactorySuite struct{}

func TestRefresherFactorySuite(t *testing.T) {
	tc.Run(t, &refresherFactorySuite{})
}

func (s *refresherFactorySuite) TestRefresh(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "meshuggah"
	curl := charm.MustParseURL(ref)
	cfg := RefresherConfig{}
	charmID := &CharmID{
		URL: curl,
	}

	refresher := NewMockRefresher(ctrl)
	refresher.EXPECT().Allowed(gomock.Any(), cfg).Return(true, nil)
	refresher.EXPECT().Refresh(gomock.Any()).Return(charmID, nil)

	f := &factory{
		refreshers: []RefresherFn{
			func(cfg RefresherConfig) (Refresher, error) {
				return refresher, nil
			},
		},
	}

	charmID2, err := f.Run(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(charmID2, tc.DeepEquals, charmID)
}

func (s *refresherFactorySuite) TestRefreshNotAllowed(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "meshuggah"
	cfg := RefresherConfig{
		CharmRef: ref,
	}

	refresher := NewMockRefresher(ctrl)
	refresher.EXPECT().Allowed(gomock.Any(), cfg).Return(false, nil)

	f := &factory{
		refreshers: []RefresherFn{
			func(cfg RefresherConfig) (Refresher, error) {
				return refresher, nil
			},
		},
	}

	_, err := f.Run(c.Context(), cfg)
	c.Assert(err, tc.ErrorMatches, `unable to refresh "meshuggah"`)
}

func (s *refresherFactorySuite) TestRefreshCallsAllRefreshers(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "meshuggah"
	curl := charm.MustParseURL(ref)
	cfg := RefresherConfig{}
	charmID := &CharmID{
		URL: curl,
	}

	refresher0 := NewMockRefresher(ctrl)
	refresher0.EXPECT().Allowed(gomock.Any(), cfg).Return(false, nil)

	refresher1 := NewMockRefresher(ctrl)
	refresher1.EXPECT().Allowed(gomock.Any(), cfg).Return(true, nil)
	refresher1.EXPECT().Refresh(gomock.Any()).Return(charmID, nil)

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

	charmID2, err := f.Run(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(charmID2, tc.DeepEquals, charmID)
}

func (s *refresherFactorySuite) TestRefreshCallsRefreshersEvenAfterExhaustedError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "meshuggah"
	curl := charm.MustParseURL(ref)
	cfg := RefresherConfig{}
	charmID := &CharmID{
		URL: curl,
	}

	refresher0 := NewMockRefresher(ctrl)
	refresher0.EXPECT().Allowed(gomock.Any(), cfg).Return(false, nil)

	refresher1 := NewMockRefresher(ctrl)
	refresher1.EXPECT().Allowed(gomock.Any(), cfg).Return(true, nil)
	refresher1.EXPECT().Refresh(gomock.Any()).Return(nil, ErrExhausted)

	refresher2 := NewMockRefresher(ctrl)
	refresher2.EXPECT().Allowed(gomock.Any(), cfg).Return(true, nil)
	refresher2.EXPECT().Refresh(gomock.Any()).Return(charmID, nil)

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

	charmID2, err := f.Run(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(charmID2, tc.DeepEquals, charmID)
}

type baseRefresherSuite struct{}

func TestBaseRefresherSuite(t *testing.T) {
	tc.Run(t, &baseRefresherSuite{})
}

func (s *baseRefresherSuite) TestResolveCharm(c *tc.C) {
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
	charmResolver.EXPECT().ResolveCharm(gomock.Any(), curl, origin, false).Return(newCurl, origin, []corebase.Base{}, nil)

	refresher := baseRefresher{
		charmRef:        "meshuggah",
		charmURL:        charm.MustParseURL("meshuggah"),
		charmResolver:   charmResolver,
		charmOrigin:     corecharm.Origin{Platform: corecharm.MustParsePlatform("amd64/ubuntu/22.04")},
		resolveOriginFn: charmHubOriginResolver,
		logger:          fakeLogger{},
	}
	url, obtainedOrigin, err := refresher.ResolveCharm(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(url, tc.DeepEquals, charm.MustParseURL("ch:meshuggah-1"))
	c.Assert(obtainedOrigin, tc.DeepEquals, origin)
}

func (s *baseRefresherSuite) TestResolveCharmWithSeriesError(c *tc.C) {
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
	charmResolver.EXPECT().ResolveCharm(gomock.Any(), curl, origin, false).Return(newCurl, origin, []corebase.Base{corebase.MustParseBaseFromString("ubuntu@20.04")}, nil)

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
	_, _, err := refresher.ResolveCharm(c.Context())
	c.Assert(err, tc.ErrorMatches, `cannot upgrade from single base "ubuntu@22.04" charm to a charm supporting \["ubuntu@20.04"\]. Use --force-series to override.`)
}

func (s *baseRefresherSuite) TestResolveCharmWithNoCharmURL(c *tc.C) {
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
	charmResolver.EXPECT().ResolveCharm(gomock.Any(), curl, origin, false).Return(newCurl, origin, []corebase.Base{}, nil)

	refresher := baseRefresher{
		charmRef:        "meshuggah",
		charmResolver:   charmResolver,
		charmOrigin:     corecharm.Origin{Platform: corecharm.MustParsePlatform("amd64/ubuntu/22.04")},
		resolveOriginFn: charmHubOriginResolver,
		logger:          fakeLogger{},
	}
	_, _, err := refresher.ResolveCharm(c.Context())
	c.Assert(err, tc.ErrorMatches, "unexpected charm URL")
}

type localCharmRefresherSuite struct{}

func TestLocalCharmRefresherSuite(t *testing.T) {
	tc.Run(t, &localCharmRefresherSuite{})
}
func (s *localCharmRefresherSuite) TestRefresh(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "local:meshuggah"
	curl := charm.MustParseURL(ref)

	ch := NewMockCharm(ctrl)
	ch.EXPECT().Meta().Return(&charm.Meta{
		Name: "meshuggah",
	})

	charmAdder := NewMockCharmAdder(ctrl)
	charmAdder.EXPECT().AddLocalCharm(gomock.Any(), curl, ch, false).Return(curl, nil)

	charmRepo := NewMockCharmRepository(ctrl)
	charmRepo.EXPECT().NewCharmAtPath(ref).Return(ch, curl, nil)

	cfg := refresherConfigWithOrigin(curl, ref, corecharm.MustParsePlatform("amd64/ubuntu/22.04"))

	refresher := (&factory{}).maybeReadLocal(charmAdder, charmRepo)
	task, err := refresher(cfg)
	c.Assert(err, tc.ErrorIsNil)

	charmID, err := task.Refresh(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(charmID.URL, tc.Equals, curl)
	c.Assert(charmID.Origin.Source, tc.Equals, corecharm.Local)
}

func (s *localCharmRefresherSuite) TestRefreshBecomesExhausted(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)

	_, err = task.Refresh(c.Context())
	c.Assert(err, tc.Equals, ErrExhausted)
}

func (s *localCharmRefresherSuite) TestRefreshDoesNotFindLocal(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)

	_, err = task.Refresh(c.Context())
	c.Assert(err, tc.ErrorMatches, `no charm found at "local:meshuggah"`)
}

type charmHubCharmRefresherSuite struct{}

func TestCharmHubCharmRefresherSuite(t *testing.T) {
	tc.Run(t, &charmHubCharmRefresherSuite{})
}
func (s *charmHubCharmRefresherSuite) TestRefresh(c *tc.C) {
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
	charmAdder.EXPECT().AddCharm(gomock.Any(), newCurl, origin, false).Return(actualOrigin, nil)

	charmResolver := NewMockCharmResolver(ctrl)
	charmResolver.EXPECT().ResolveCharm(gomock.Any(), curl, origin, false).Return(newCurl, origin, []corebase.Base{}, nil)

	cfg := refresherConfigWithOrigin(curl, ref, corecharm.MustParsePlatform("amd64/ubuntu/22.04"))

	refresher := (&factory{}).maybeCharmHub(charmAdder, charmResolver)
	task, err := refresher(cfg)
	c.Assert(err, tc.ErrorIsNil)

	charmID, err := task.Refresh(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(charmID, tc.DeepEquals, &CharmID{
		URL:    newCurl,
		Origin: actualOrigin.CoreCharmOrigin(),
	})
}

func (s *charmHubCharmRefresherSuite) TestRefreshWithNoOrigin(c *tc.C) {
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
	charmAdder.EXPECT().AddCharm(gomock.Any(), newCurl, origin, false).Return(origin, nil)

	charmResolver := NewMockCharmResolver(ctrl)
	charmResolver.EXPECT().ResolveCharm(gomock.Any(), curl, origin, false).Return(newCurl, origin, []corebase.Base{}, nil)

	cfg := refresherConfigWithOrigin(curl, ref, corecharm.MustParsePlatform("amd64/ubuntu/22.04"))

	refresher := (&factory{}).maybeCharmHub(charmAdder, charmResolver)
	task, err := refresher(cfg)
	c.Assert(err, tc.ErrorIsNil)

	charmID, err := task.Refresh(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(charmID, tc.DeepEquals, &CharmID{
		URL:    newCurl,
		Origin: origin.CoreCharmOrigin(),
	})
}

func (s *charmHubCharmRefresherSuite) TestRefreshWithNoUpdates(c *tc.C) {
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
	charmResolver.EXPECT().ResolveCharm(gomock.Any(), curl, origin, false).Return(curl, origin, []corebase.Base{}, nil)

	cfg := refresherConfigWithOrigin(curl, ref, corecharm.MustParsePlatform("amd64/ubuntu/22.04"))

	refresher := (&factory{}).maybeCharmHub(charmAdder, charmResolver)
	task, err := refresher(cfg)
	c.Assert(err, tc.ErrorIsNil)

	_, err = task.Refresh(c.Context())
	c.Assert(err, tc.ErrorMatches, `charm "meshuggah": already up-to-date`)
}

func (s *charmHubCharmRefresherSuite) TestRefreshWithARevision(c *tc.C) {
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
	charmResolver.EXPECT().ResolveCharm(gomock.Any(), curl, origin, false).Return(curl, origin, []corebase.Base{}, nil)

	cfg := refresherConfigWithOrigin(curl, ref, corecharm.MustParsePlatform("amd64/ubuntu/22.04"))

	refresher := (&factory{}).maybeCharmHub(charmAdder, charmResolver)
	task, err := refresher(cfg)
	c.Assert(err, tc.ErrorIsNil)

	_, err = task.Refresh(c.Context())
	c.Assert(err, tc.ErrorMatches, `charm "meshuggah", revision 1: already up-to-date`)
}

func (s *charmHubCharmRefresherSuite) TestRefreshWithOriginChannel(c *tc.C) {
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
	charmResolver.EXPECT().ResolveCharm(gomock.Any(), curl, origin, false).Return(curl, origin, []corebase.Base{}, nil)

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
	c.Assert(err, tc.ErrorIsNil)

	_, err = task.Refresh(c.Context())
	c.Assert(err, tc.ErrorMatches, `charm "meshuggah", revision 1: already up-to-date`)
}

func (s *charmHubCharmRefresherSuite) TestRefreshWithCharmSwitch(c *tc.C) {
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
	charmResolver.EXPECT().ResolveCharm(gomock.Any(), curl, origin, true).Return(curl, origin, []corebase.Base{}, nil)

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
	c.Assert(err, tc.ErrorIsNil)

	_, err = task.Refresh(c.Context())
	c.Assert(err, tc.ErrorMatches, `charm "aloupi", revision 1: already up-to-date`)
}

func (s *charmHubCharmRefresherSuite) TestAllowed(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "ch:meshuggah"
	curl := charm.MustParseURL(ref)

	charmAdder := NewMockCharmAdder(ctrl)
	charmResolver := NewMockCharmResolver(ctrl)

	cfg := refresherConfigWithOrigin(curl, ref, corecharm.MustParsePlatform("amd64/ubuntu/22.04"))

	refresher := (&factory{}).maybeCharmHub(charmAdder, charmResolver)
	task, err := refresher(cfg)
	c.Assert(err, tc.ErrorIsNil)

	allowed, err := task.Allowed(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(allowed, tc.IsTrue)
}

func (s *charmHubCharmRefresherSuite) TestAllowedWithSwitch(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "ch:meshuggah"
	curl := charm.MustParseURL(ref)

	charmAdder := NewMockCharmAdder(ctrl)
	charmAdder.EXPECT().CheckCharmPlacement(gomock.Any(), "winnie", curl).Return(nil)

	charmResolver := NewMockCharmResolver(ctrl)

	cfg := refresherConfigWithOrigin(curl, ref, corecharm.MustParsePlatform("amd64/ubuntu/22.04"))
	cfg.Switch = true

	refresher := (&factory{}).maybeCharmHub(charmAdder, charmResolver)
	task, err := refresher(cfg)
	c.Assert(err, tc.ErrorIsNil)

	allowed, err := task.Allowed(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(allowed, tc.IsTrue)
}

func (s *charmHubCharmRefresherSuite) TestAllowedError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ref := "ch:meshuggah"
	curl := charm.MustParseURL(ref)

	charmAdder := NewMockCharmAdder(ctrl)
	charmAdder.EXPECT().CheckCharmPlacement(gomock.Any(), "winnie", curl).Return(errors.Errorf("trap"))

	charmResolver := NewMockCharmResolver(ctrl)

	cfg := refresherConfigWithOrigin(curl, ref, corecharm.MustParsePlatform("amd64/ubuntu/22.04"))
	cfg.Switch = true

	refresher := (&factory{}).maybeCharmHub(charmAdder, charmResolver)
	task, err := refresher(cfg)
	c.Assert(err, tc.ErrorIsNil)

	allowed, err := task.Allowed(c.Context(), cfg)
	c.Assert(err, tc.ErrorMatches, "trap")
	c.Assert(allowed, tc.IsFalse)
}

func (s *charmHubCharmRefresherSuite) TestCharmHubResolveOriginEmpty(c *tc.C) {
	origin := corecharm.Origin{}
	channel := charm.Channel{}
	result, err := charmHubOriginResolver(nil, origin, channel)
	c.Assert(err, tc.ErrorIsNil)
	coreOrigin, err := commoncharm.CoreCharmOrigin(origin)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, coreOrigin)
}

func (s *charmHubCharmRefresherSuite) TestCharmHubResolveOrigin(c *tc.C) {
	track := "meshuggah"
	origin := corecharm.Origin{}
	channel := charm.Channel{
		Track: track,
	}
	result, err := charmHubOriginResolver(nil, origin, channel)
	c.Assert(err, tc.ErrorIsNil)
	coreOrigin, err := commoncharm.CoreCharmOrigin(corecharm.Origin{
		Channel: &charm.Channel{
			Track: track,
			Risk:  "stable",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, coreOrigin)
}

func (s *charmHubCharmRefresherSuite) TestCharmHubResolveOriginEmptyTrackNonEmptyChannel(c *tc.C) {
	origin := corecharm.Origin{
		Channel: &charm.Channel{},
	}
	channel := charm.Channel{
		Risk: "edge",
	}
	result, err := charmHubOriginResolver(nil, origin, channel)
	c.Assert(err, tc.ErrorIsNil)
	coreOrigin, err := commoncharm.CoreCharmOrigin(corecharm.Origin{
		Channel: &charm.Channel{
			Risk: "edge",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, coreOrigin)
}

func (s *charmHubCharmRefresherSuite) TestCharmHubResolveOriginEmptyTrackEmptyChannel(c *tc.C) {
	origin := corecharm.Origin{}
	channel := charm.Channel{
		Risk: "edge",
	}
	result, err := charmHubOriginResolver(nil, origin, channel)
	c.Assert(err, tc.ErrorIsNil)
	coreOrigin, err := commoncharm.CoreCharmOrigin(corecharm.Origin{
		Channel: &charm.Channel{},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, coreOrigin)
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
