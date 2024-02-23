// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store_test

import (
	"github.com/juju/errors"
	"github.com/juju/juju/charm"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/juju/application/store/mocks"
	"github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/rpc/params"
)

type storeSuite struct {
	charmAdder *mocks.MockCharmAdder
}

var _ = gc.Suite(&storeSuite{})

func (s *storeSuite) TestAddCharmFromURLAddCharmSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectAddCharm(nil)

	curl, err := charm.ParseURL("ch:testme")
	c.Assert(err, jc.ErrorIsNil)
	origin, err := utils.MakeOrigin(charm.CharmHub, -1, charm.Channel{Risk: charm.Beta}, corecharm.Platform{Architecture: arch.DefaultArchitecture})
	c.Assert(err, jc.ErrorIsNil)

	obtainedCurl, _, err := store.AddCharmFromURL(
		s.charmAdder,
		curl,
		origin,
		true,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedCurl.String(), gc.Equals, curl.String())
}

func (s *storeSuite) TestAddCharmFromURLFailAddCharmFail(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectAddCharm(errors.NotFoundf("testing"))
	curl, err := charm.ParseURL("ch:testme")
	c.Assert(err, jc.ErrorIsNil)
	origin, err := utils.MakeOrigin(charm.CharmHub, -1, charm.Channel{Risk: charm.Beta}, corecharm.Platform{Architecture: arch.DefaultArchitecture})
	c.Assert(err, jc.ErrorIsNil)

	obtainedCurl, _, err := store.AddCharmFromURL(
		s.charmAdder,
		curl,
		origin,
		true,
	)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(obtainedCurl, gc.IsNil)
}

func (s *storeSuite) TestAddCharmFromURLFailAddCharmFailUnauthorized(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectAddCharm(&params.Error{
		Code:    params.CodeUnauthorized,
		Message: "permission denied",
	})
	curl, err := charm.ParseURL("ch:testme")
	c.Assert(err, jc.ErrorIsNil)
	origin, err := utils.MakeOrigin(charm.CharmHub, -1, charm.Channel{Risk: charm.Beta}, corecharm.Platform{Architecture: arch.DefaultArchitecture})
	c.Assert(err, jc.ErrorIsNil)

	obtainedCurl, _, err := store.AddCharmFromURL(
		s.charmAdder,
		curl,
		origin,
		true,
	)
	c.Assert(err, jc.ErrorIs, errors.Forbidden)
	c.Assert(obtainedCurl, gc.IsNil)
}

func (s *storeSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.charmAdder = mocks.NewMockCharmAdder(ctrl)
	return ctrl
}

func (s *storeSuite) expectAddCharm(err error) {
	s.charmAdder.EXPECT().AddCharm(
		gomock.AssignableToTypeOf(&charm.URL{}),
		gomock.AssignableToTypeOf(commoncharm.Origin{}),
		true,
	).DoAndReturn(
		func(_ *charm.URL, origin commoncharm.Origin, _ bool) (commoncharm.Origin, error) {
			return origin, err
		})
}
