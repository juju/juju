// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type ApplicationOfferUserSuite struct {
	ConnSuite
}

var _ = gc.Suite(&ApplicationOfferUserSuite{})

func (s *ApplicationOfferUserSuite) makeOffer(c *gc.C, access permission.Access) (names.ApplicationOfferTag, names.UserTag) {
	app := s.Factory.MakeApplication(c, nil)
	offers := state.NewApplicationOffers(s.State)
	_, err := offers.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "someoffer",
		ApplicationName: app.Name(),
		Owner:           "test-admin",
		HasRead:         []string{"everyone@external"},
	})
	c.Assert(err, jc.ErrorIsNil)

	user := s.Factory.MakeUser(c,
		&factory.UserParams{
			Name: "validusername",
		})
	offerTag := names.NewApplicationOfferTag("someoffer")

	// Initially no access.
	_, err = s.State.GetOfferAccess(offerTag, user.UserTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	err = s.State.CreateOfferAccess(offerTag, user.UserTag(), access)
	c.Assert(err, jc.ErrorIsNil)
	return offerTag, user.UserTag()
}

func (s *ApplicationOfferUserSuite) assertAddOffer(c *gc.C, wantedAccess permission.Access) {
	offerTag, user := s.makeOffer(c, wantedAccess)

	access, err := s.State.GetOfferAccess(offerTag, user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, wantedAccess)

	// Creator of offer has admin.
	access, err = s.State.GetOfferAccess(offerTag, names.NewUserTag("test-admin"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.AdminAccess)

	// Everyone has read.
	access, err = s.State.GetOfferAccess(offerTag, names.NewUserTag("everyone@external"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.ReadAccess)
}

func (s *ApplicationOfferUserSuite) TestAddReadOnlyOfferUser(c *gc.C) {
	s.assertAddOffer(c, permission.ReadAccess)
}

func (s *ApplicationOfferUserSuite) TestAddConsumeOfferUser(c *gc.C) {
	s.assertAddOffer(c, permission.ConsumeAccess)
}

func (s *ApplicationOfferUserSuite) TestAddAdminModelUser(c *gc.C) {
	s.assertAddOffer(c, permission.AdminAccess)
}

func (s *ApplicationOfferUserSuite) TestUpdateOfferAccess(c *gc.C) {
	offerTag, user := s.makeOffer(c, permission.AdminAccess)
	err := s.State.UpdateOfferAccess(offerTag, user, permission.ReadAccess)

	access, err := s.State.GetOfferAccess(offerTag, user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.ReadAccess)
}

func (s *ApplicationOfferUserSuite) TestCreateOfferAccessNoUserFails(c *gc.C) {
	app := s.Factory.MakeApplication(c, nil)
	offers := state.NewApplicationOffers(s.State)
	_, err := offers.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "someoffer",
		ApplicationName: app.Name(),
		Owner:           "test-admin",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.CreateOfferAccess(
		names.NewApplicationOfferTag("someoffer"),
		names.NewUserTag("validusername"), permission.ReadAccess)
	c.Assert(err, gc.ErrorMatches, `user "validusername" does not exist locally: user "validusername" not found`)
}

func (s *ApplicationOfferUserSuite) TestRemoveOfferAccess(c *gc.C) {
	offerTag, user := s.makeOffer(c, permission.ConsumeAccess)

	err := s.State.RemoveOfferAccess(offerTag, user)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.GetOfferAccess(offerTag, user)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationOfferUserSuite) TestRemoveOfferAccessNoUser(c *gc.C) {
	offerTag, _ := s.makeOffer(c, permission.ConsumeAccess)
	err := s.State.RemoveOfferAccess(offerTag, names.NewUserTag("fred"))
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}
