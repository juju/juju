// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/juju/application/store/mocks"
	"github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/rpc/params"
)

type storeSuite struct {
	charmAdder *mocks.MockCharmAdder
}

var _ = tc.Suite(&storeSuite{})

func (s *storeSuite) TestAddCharmFromURLAddCharmSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectAddCharm(nil)

	curl, err := charm.ParseURL("ch:testme")
	c.Assert(err, jc.ErrorIsNil)
	origin, err := utils.MakeOrigin(charm.CharmHub, -1, charm.Channel{Risk: charm.Beta}, corecharm.Platform{Architecture: arch.DefaultArchitecture})
	c.Assert(err, jc.ErrorIsNil)

	obtainedCurl, _, err := store.AddCharmFromURL(
		context.Background(),
		s.charmAdder,
		curl,
		origin,
		true,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedCurl.String(), tc.Equals, curl.String())
}

func (s *storeSuite) TestAddCharmFromURLFailAddCharmFail(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectAddCharm(errors.NotFoundf("testing"))
	curl, err := charm.ParseURL("ch:testme")
	c.Assert(err, jc.ErrorIsNil)
	origin, err := utils.MakeOrigin(charm.CharmHub, -1, charm.Channel{Risk: charm.Beta}, corecharm.Platform{Architecture: arch.DefaultArchitecture})
	c.Assert(err, jc.ErrorIsNil)

	obtainedCurl, _, err := store.AddCharmFromURL(
		context.Background(),
		s.charmAdder,
		curl,
		origin,
		true,
	)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(obtainedCurl, tc.IsNil)
}

func (s *storeSuite) TestAddCharmFromURLFailAddCharmFailUnauthorized(c *tc.C) {
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
		context.Background(),
		s.charmAdder,
		curl,
		origin,
		true,
	)
	c.Assert(err, jc.ErrorIs, errors.Forbidden)
	c.Assert(obtainedCurl, tc.IsNil)
}

func (s *storeSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.charmAdder = mocks.NewMockCharmAdder(ctrl)
	return ctrl
}

func (s *storeSuite) expectAddCharm(err error) {
	s.charmAdder.EXPECT().AddCharm(
		gomock.Any(),
		gomock.AssignableToTypeOf(&charm.URL{}),
		gomock.AssignableToTypeOf(commoncharm.Origin{}),
		true,
	).DoAndReturn(
		func(ctx context.Context, _ *charm.URL, origin commoncharm.Origin, _ bool) (commoncharm.Origin, error) {
			return origin, err
		})
}
