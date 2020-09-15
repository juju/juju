// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package refresher

import (
	"fmt"
	"os"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	"github.com/juju/charmrepo/v6"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	commoncharm "github.com/juju/juju/api/common/charm"
)

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

	charmRepo := NewMockCharmRepo(ctrl)
	charmRepo.EXPECT().NewCharmAtPathForceSeries(ref, "", false).Return(ch, curl, nil)

	cfg := RefresherConfig{
		CharmURL: curl,
		CharmRef: ref,
	}

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
	charmRepo := NewMockCharmRepo(ctrl)
	charmRepo.EXPECT().NewCharmAtPathForceSeries(ref, "", false).Return(nil, nil, os.ErrNotExist)

	cfg := RefresherConfig{
		CharmURL: curl,
		CharmRef: ref,
	}

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
	charmRepo := NewMockCharmRepo(ctrl)
	charmRepo.EXPECT().NewCharmAtPathForceSeries(ref, "", false).Return(nil, nil, &charmrepo.NotFoundError{})

	cfg := RefresherConfig{
		CharmURL: curl,
		CharmRef: ref,
	}

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
		Source: commoncharm.OriginCharmStore,
	}

	authorizer := NewMockMacaroonGetter(ctrl)

	charmAdder := NewMockCharmAdder(ctrl)
	charmAdder.EXPECT().AddCharm(newCurl, origin, false).Return(origin, nil)

	charmResolver := NewMockCharmResolver(ctrl)
	charmResolver.EXPECT().ResolveCharm(curl, origin).Return(newCurl, origin, []string{}, nil)

	cfg := RefresherConfig{
		CharmURL: curl,
		CharmRef: ref,
	}

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
		Source: commoncharm.OriginCharmStore,
	}

	authorizer := NewMockMacaroonGetter(ctrl)
	charmAdder := NewMockCharmAdder(ctrl)

	charmResolver := NewMockCharmResolver(ctrl)
	charmResolver.EXPECT().ResolveCharm(curl, origin).Return(curl, origin, []string{}, nil)

	cfg := RefresherConfig{
		CharmURL: curl,
		CharmRef: ref,
	}

	refresher := (&factory{}).maybeCharmStore(authorizer, charmAdder, charmResolver)
	task, err := refresher(cfg)
	c.Assert(err, jc.ErrorIsNil)

	_, err = task.Refresh()
	c.Assert(err, gc.ErrorMatches, `already running latest charm "cs:meshuggah"`)
}
