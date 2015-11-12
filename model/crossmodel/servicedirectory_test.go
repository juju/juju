// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/model/crossmodel"
	"github.com/juju/juju/testing"
)

type serviceDirectorySuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&serviceDirectorySuite{})

type mockServiceOfficeLister struct {
	results []crossmodel.ServiceOffer
}

func (m *mockServiceOfficeLister) ListOffers(filter ...crossmodel.ServiceOfferFilter) ([]crossmodel.ServiceOffer, error) {
	return m.results, nil
}

func (s *serviceDirectorySuite) TestServiceForURL(c *gc.C) {
	offers := []crossmodel.ServiceOffer{
		{
			ServiceURL:  "local:/u/user/name",
			ServiceName: "service",
		},
	}
	offerLister := &mockServiceOfficeLister{offers}
	wrapper := crossmodel.ServiceDirectoryWrapper{offerLister}
	result, err := wrapper.ServiceForURL("local:/u/user/name", "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, offers[0])
}

func (s *serviceDirectorySuite) TestServiceForURLNoneOrNoAccess(c *gc.C) {
	offerLister := &mockServiceOfficeLister{[]crossmodel.ServiceOffer{}}
	wrapper := crossmodel.ServiceDirectoryWrapper{offerLister}
	_, err := wrapper.ServiceForURL("local:/u/user/name", "foo")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}
