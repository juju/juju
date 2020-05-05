// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type ApplicationOfferUserSuite struct {
	ConnSuite
}

var _ = gc.Suite(&ApplicationOfferUserSuite{})

func (s *ApplicationOfferUserSuite) makeOffer(c *gc.C, access permission.Access) (*crossmodel.ApplicationOffer, names.UserTag) {
	s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	offers := state.NewApplicationOffers(s.State)
	offer, err := offers.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "someoffer",
		ApplicationName: "mysql",
		Owner:           "test-admin",
		HasRead:         []string{"everyone@external"},
	})
	c.Assert(err, jc.ErrorIsNil)

	user := s.Factory.MakeUser(c,
		&factory.UserParams{
			Name:   "validusername",
			Access: permission.ReadAccess,
		})

	// Initially no access.
	_, err = s.State.GetOfferAccess(offer.OfferUUID, user.UserTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	err = s.State.CreateOfferAccess(names.NewApplicationOfferTag(offer.OfferName), user.UserTag(), access)
	c.Assert(err, jc.ErrorIsNil)
	return offer, user.UserTag()
}

func (s *ApplicationOfferUserSuite) assertAddOffer(c *gc.C, wantedAccess permission.Access) string {
	offer, user := s.makeOffer(c, wantedAccess)

	access, err := s.State.GetOfferAccess(offer.OfferUUID, user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, wantedAccess)

	// Creator of offer has admin.
	access, err = s.State.GetOfferAccess(offer.OfferUUID, names.NewUserTag("test-admin"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.AdminAccess)

	// Everyone has read.
	access, err = s.State.GetOfferAccess(offer.OfferUUID, names.NewUserTag("everyone@external"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.ReadAccess)
	return offer.OfferUUID
}

func (s *ApplicationOfferUserSuite) TestAddReadOnlyOfferUser(c *gc.C) {
	s.assertAddOffer(c, permission.ReadAccess)
}

func (s *ApplicationOfferUserSuite) TestAddConsumeOfferUser(c *gc.C) {
	s.assertAddOffer(c, permission.ConsumeAccess)
}

func (s *ApplicationOfferUserSuite) TestGetOfferAccess(c *gc.C) {
	offerUUID := s.assertAddOffer(c, permission.ConsumeAccess)
	users, err := s.State.GetOfferUsers(offerUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(users, jc.DeepEquals, map[string]permission.Access{
		"everyone@external": permission.ReadAccess,
		"test-admin":        permission.AdminAccess,
		"validusername":     permission.ConsumeAccess,
	})
}

func (s *ApplicationOfferUserSuite) TestAddAdminModelUser(c *gc.C) {
	s.assertAddOffer(c, permission.AdminAccess)
}

func (s *ApplicationOfferUserSuite) TestUpdateOfferAccess(c *gc.C) {
	offer, user := s.makeOffer(c, permission.AdminAccess)
	err := s.State.UpdateOfferAccess(names.NewApplicationOfferTag(offer.OfferName), user, permission.ReadAccess)
	c.Assert(err, jc.ErrorIsNil)

	access, err := s.State.GetOfferAccess(offer.OfferUUID, user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.ReadAccess)
}

func (s *ApplicationOfferUserSuite) setupOfferRelation(c *gc.C, offerUUID, user string) *state.Relation {
	// Make a relation to the offer.
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	mysql, err := s.State.Application("mysql")
	c.Assert(err, jc.ErrorIsNil)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: utils.MustNewUUID().String(),
		OfferUUID:       offerUUID,
		RelationKey:     rel.Tag().Id(),
		RelationId:      rel.Id(),
		Username:        user,
	})
	c.Assert(err, jc.ErrorIsNil)
	return rel
}

func (s *ApplicationOfferUserSuite) TestUpdateOfferAccessSetsRelationSuspended(c *gc.C) {
	offer, user := s.makeOffer(c, permission.ConsumeAccess)
	rel := s.setupOfferRelation(c, offer.OfferUUID, user.Name())

	// Downgrade consume access and check the relation is suspended.
	err := s.State.UpdateOfferAccess(names.NewApplicationOfferTag(offer.OfferName), user, permission.ReadAccess)
	c.Assert(err, jc.ErrorIsNil)
	err = rel.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.Suspended(), jc.IsTrue)
}

func (s *ApplicationOfferUserSuite) TestUpdateOfferAccessSetsRelationSuspendedRace(c *gc.C) {
	offer, user := s.makeOffer(c, permission.ConsumeAccess)
	rel := s.setupOfferRelation(c, offer.OfferUUID, user.Name())
	var rel2 *state.Relation

	defer state.SetBeforeHooks(c, s.State, func() {
		// Add another relation to the offered app.
		curl := charm.MustParseURL("local:quantal/quantal-wordpress-3")
		wpch, err := s.State.Charm(curl)
		c.Assert(err, jc.ErrorIsNil)
		wordpress2 := s.AddTestingApplication(c, "wordpress2", wpch)
		wordpressEP, err := wordpress2.Endpoint("db")
		c.Assert(err, jc.ErrorIsNil)
		mysql, err := s.State.Application("mysql")
		c.Assert(err, jc.ErrorIsNil)
		mysqlEP, err := mysql.Endpoint("server")
		c.Assert(err, jc.ErrorIsNil)
		rel2, err = s.State.AddRelation(wordpressEP, mysqlEP)
		c.Assert(err, jc.ErrorIsNil)
		_, err = s.State.AddOfferConnection(state.AddOfferConnectionParams{
			SourceModelUUID: utils.MustNewUUID().String(),
			OfferUUID:       offer.OfferUUID,
			RelationKey:     rel2.Tag().Id(),
			RelationId:      rel2.Id(),
			Username:        user.Name(),
		})
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	// Downgrade consume access and check both relations are suspended.
	err := s.State.UpdateOfferAccess(names.NewApplicationOfferTag(offer.OfferName), user, permission.ReadAccess)
	c.Assert(err, jc.ErrorIsNil)
	err = rel.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.Suspended(), jc.IsTrue)
	err = rel2.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel2.Suspended(), jc.IsTrue)
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
	offer, user := s.makeOffer(c, permission.ConsumeAccess)

	err := s.State.RemoveOfferAccess(names.NewApplicationOfferTag(offer.OfferName), user)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.GetOfferAccess(offer.OfferUUID, user)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationOfferUserSuite) TestRemoveOfferAccessNoUser(c *gc.C) {
	offer, _ := s.makeOffer(c, permission.ConsumeAccess)
	err := s.State.RemoveOfferAccess(names.NewApplicationOfferTag(offer.OfferName), names.NewUserTag("fred"))
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationOfferUserSuite) TestRemoveOfferAccessSetsRelationSuspended(c *gc.C) {
	offer, user := s.makeOffer(c, permission.ConsumeAccess)
	rel := s.setupOfferRelation(c, offer.OfferUUID, user.Name())

	// Remove any access and check the relation is suspended.
	err := s.State.RemoveOfferAccess(names.NewApplicationOfferTag(offer.OfferName), user)
	c.Assert(err, jc.ErrorIsNil)
	err = rel.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.Suspended(), jc.IsTrue)
}
