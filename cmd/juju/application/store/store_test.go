// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store_test

import (
	"net/url"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/juju/application/store/mocks"
	"github.com/juju/juju/cmd/juju/application/utils"
)

type storeSuite struct {
	charmAdder     *mocks.MockCharmAdder
	macaroonGetter *mocks.MockMacaroonGetter
}

var _ = gc.Suite(&storeSuite{})

func (s *storeSuite) TestAddCharmFromURLAddCharmSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectAddCharm(nil)

	curl, err := charm.ParseURL("cs:testme")
	c.Assert(err, jc.ErrorIsNil)
	origin, err := utils.DeduceOrigin(curl, csparams.BetaChannel)
	c.Assert(err, jc.ErrorIsNil)

	obtainedCurl, obtainedMac, _, err := store.AddCharmFromURL(
		s.charmAdder,
		s.macaroonGetter,
		curl,
		origin,
		true,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedMac, gc.IsNil)
	c.Assert(obtainedCurl.String(), gc.Equals, curl.String())
}

func (s *storeSuite) TestAddCharmFromURLFailAddCharmFail(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectAddCharm(errors.NotFoundf("testing"))
	curl, err := charm.ParseURL("cs:testme")
	c.Assert(err, jc.ErrorIsNil)
	origin, err := utils.DeduceOrigin(curl, csparams.BetaChannel)
	c.Assert(err, jc.ErrorIsNil)

	obtainedCurl, obtainedMac, _, err := store.AddCharmFromURL(
		s.charmAdder,
		s.macaroonGetter,
		curl,
		origin,
		true,
	)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(obtainedMac, gc.IsNil)
	c.Assert(obtainedCurl, gc.IsNil)
}

func (s *storeSuite) TestAddCharmFromURLFailAddCharmFailUnauthorized(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectAddCharm(&params.Error{
		Code:    params.CodeUnauthorized,
		Message: "permission denied",
	})
	curl, err := charm.ParseURL("cs:testme")
	c.Assert(err, jc.ErrorIsNil)
	origin, err := utils.DeduceOrigin(curl, csparams.BetaChannel)
	c.Assert(err, jc.ErrorIsNil)
	s.expectGet("/delegatable-macaroon?id=" + url.QueryEscape(curl.String()))
	s.expectAddCharmWithAuthorization()

	obtainedCurl, obtainedMac, _, err := store.AddCharmFromURL(
		s.charmAdder,
		s.macaroonGetter,
		curl,
		origin,
		true,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedMac, gc.IsNil)
	c.Assert(obtainedCurl.String(), gc.Equals, curl.String())
}

func (s *storeSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.charmAdder = mocks.NewMockCharmAdder(ctrl)
	s.macaroonGetter = mocks.NewMockMacaroonGetter(ctrl)
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

func (s *storeSuite) expectAddCharmWithAuthorization() {
	s.charmAdder.EXPECT().AddCharmWithAuthorization(
		gomock.AssignableToTypeOf(&charm.URL{}),
		gomock.AssignableToTypeOf(commoncharm.Origin{}),
		gomock.AssignableToTypeOf(&macaroon.Macaroon{}),
		true,
	).DoAndReturn(
		func(_ *charm.URL, origin commoncharm.Origin, _ *macaroon.Macaroon, _ bool) (commoncharm.Origin, error) {
			return origin, nil
		})
}

func (s *storeSuite) expectGet(path string) {
	s.macaroonGetter.EXPECT().Get(path, gomock.Any()).Return(nil)
}
