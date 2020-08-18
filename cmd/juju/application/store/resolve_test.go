// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/juju/application/store/mocks"
)

type resolveSuite struct {
	urlResolver *mocks.MockURLResolver
}

var _ = gc.Suite(&resolveSuite{})

func (s *resolveSuite) TestResolveCharm(c *gc.C) {
	curl, err := charm.ParseURL("cs:testme-3")
	c.Assert(err, jc.ErrorIsNil)
	resolveWithChannelFunc := func(url *charm.URL, channel csparams.Channel) (*charm.URL, csparams.Channel, []string, error) {
		c.Assert(url, gc.Equals, curl)
		url.Series = "bionic"
		return url, csparams.EdgeChannel, []string{"bionic", "focal"}, nil
	}
	obtainedURL, obtainedChannel, obtainedSeries, err := store.ResolveCharm(resolveWithChannelFunc, curl, csparams.BetaChannel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedChannel, gc.Equals, csparams.EdgeChannel)
	c.Assert(obtainedSeries, jc.SameContents, []string{"bionic", "focal"})
	c.Assert(obtainedURL, gc.Equals, curl)
}

func (s *resolveSuite) TestResolveCharmNotCSCharm(c *gc.C) {
	curl, err := charm.ParseURL("local:bionic/testme-3")
	c.Assert(err, jc.ErrorIsNil)
	_, obtainedChannel, _, err := store.ResolveCharm(nil, curl, csparams.BetaChannel)
	c.Assert(err, gc.NotNil)
	c.Assert(obtainedChannel, gc.Equals, csparams.NoChannel)
}

func (s *resolveSuite) TestResolveCharmFailResolveWithChannel(c *gc.C) {
	curl, err := charm.ParseURL("cs:testme-3")
	c.Assert(err, jc.ErrorIsNil)
	resolveWithChannelFunc := func(url *charm.URL, channel csparams.Channel) (*charm.URL, csparams.Channel, []string, error) {
		c.Assert(url, gc.Equals, curl)
		return url, csparams.BetaChannel, nil, errors.New("fail the test")
	}
	_, obtainedChannel, _, err := store.ResolveCharm(resolveWithChannelFunc, curl, csparams.BetaChannel)
	c.Assert(err, gc.NotNil)
	c.Assert(obtainedChannel, gc.Equals, csparams.NoChannel)
}

type resolveBundleSuite struct {
	urlResolver *mocks.MockURLResolver
}

var _ = gc.Suite(&resolveBundleSuite{})

func (s *resolveBundleSuite) TestResolveBundleURLNotBundle(c *gc.C) {
	defer s.setupMocks(c).Finish()
	_, _, err := store.ResolveBundleURL(s.urlResolver, "url parse fail", csparams.EdgeChannel)
	c.Assert(err, gc.NotNil)
}

func (s *resolveBundleSuite) TestResolveBundle(c *gc.C) {
	defer s.setupMocks(c).Finish()
	curl, err := charm.ParseURL("cs:testme-3")
	c.Assert(err, jc.ErrorIsNil)
	s.expectResolveWithPreferredChannelBundle(curl)

	obtainedURL, obtainedChannel, err := store.ResolveBundleURL(s.urlResolver, curl.String(), csparams.EdgeChannel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedChannel, gc.Equals, csparams.EdgeChannel)
	c.Assert(obtainedURL, gc.Equals, curl)
}

func (s *resolveBundleSuite) TestResolveNotBundle(c *gc.C) {
	defer s.setupMocks(c).Finish()
	curl, err := charm.ParseURL("cs:testme-3")
	c.Assert(err, jc.ErrorIsNil)
	s.expectResolveWithPreferredChannelNotBundle(curl)

	_, _, err = store.ResolveBundleURL(s.urlResolver, curl.String(), csparams.EdgeChannel)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (s *resolveBundleSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.urlResolver = mocks.NewMockURLResolver(ctrl)
	return ctrl
}

func (s *resolveBundleSuite) expectResolveWithPreferredChannelBundle(curl *charm.URL) {
	curl.Series = "bundle"
	s.urlResolver.EXPECT().ResolveWithPreferredChannel(gomock.AssignableToTypeOf(&charm.URL{}), csparams.EdgeChannel).Return(curl, csparams.EdgeChannel, []string{"bionic", "focal"}, nil)
}

func (s *resolveBundleSuite) expectResolveWithPreferredChannelNotBundle(curl *charm.URL) {
	curl.Series = "bionic"
	s.urlResolver.EXPECT().ResolveWithPreferredChannel(gomock.AssignableToTypeOf(&charm.URL{}), csparams.EdgeChannel).Return(curl, csparams.EdgeChannel, nil, nil)

}
