// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/testing"
)

type applicationOffersSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&applicationOffersSuite{})

type mockApplicationOfferLister struct {
	results []crossmodel.ApplicationOffer
}

func (m *mockApplicationOfferLister) ListOffers(directory string, filter ...crossmodel.ApplicationOfferFilter) ([]crossmodel.ApplicationOffer, error) {
	return m.results, nil
}

func (s *applicationOffersSuite) TestServiceForURL(c *gc.C) {
	offers := []crossmodel.ApplicationOffer{
		{
			ApplicationURL:  "local:/u/user/applicationname",
			ApplicationName: "application",
		},
	}
	offerLister := &mockApplicationOfferLister{offers}
	result, err := crossmodel.ApplicationOfferForURL(offerLister, "local:/u/user/applicationname", "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, offers[0])
}

func (s *applicationOffersSuite) TestServiceForURLNoneOrNoAccess(c *gc.C) {
	offerLister := &mockApplicationOfferLister{[]crossmodel.ApplicationOffer{}}
	_, err := crossmodel.ApplicationOfferForURL(offerLister, "local:/u/user/applicationname", "foo")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}
