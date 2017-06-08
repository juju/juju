// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers_test

import (
	"regexp"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/applicationoffers"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/permission"
)

type offerAccessSuite struct {
	baseSuite
	api *applicationoffers.OffersAPI
}

var _ = gc.Suite(&offerAccessSuite{})

func (s *offerAccessSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)
	s.authorizer.Tag = names.NewUserTag("admin")
	getApplicationOffers := func(interface{}) jujucrossmodel.ApplicationOffers {
		return &stubApplicationOffers{}
	}

	resources := common.NewResources()
	resources.RegisterNamed("dataDir", common.StringResource(c.MkDir()))
	var err error
	s.api, err = applicationoffers.CreateOffersAPI(
		getApplicationOffers, nil, s.mockState, s.mockStatePool, s.authorizer, resources,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *offerAccessSuite) modifyAccess(
	c *gc.C, user names.UserTag,
	action params.OfferAction,
	access params.OfferAccessPermission,
	offerURL string,
) error {
	args := params.ModifyOfferAccessRequest{
		Changes: []params.ModifyOfferAccess{{
			UserTag:  user.String(),
			Action:   action,
			Access:   access,
			OfferURL: offerURL,
		}}}

	result, err := s.api.ModifyOfferAccess(args)
	if err != nil {
		return err
	}
	return result.OneError()
}

func (s *offerAccessSuite) grant(c *gc.C, user names.UserTag, access params.OfferAccessPermission, offerURL string) error {
	return s.modifyAccess(c, user, params.GrantOfferAccess, access, offerURL)
}

func (s *offerAccessSuite) revoke(c *gc.C, user names.UserTag, access params.OfferAccessPermission, offerURL string) error {
	return s.modifyAccess(c, user, params.RevokeOfferAccess, access, offerURL)
}

func (s *offerAccessSuite) setupOffer(modelUUID, modelName, owner, offerName string) {
	s.mockState.allmodels = []applicationoffers.Model{
		&mockModel{uuid: modelUUID, name: modelName, owner: owner}}
	st := &mockState{
		modelUUID:         modelUUID,
		applicationOffers: make(map[string]jujucrossmodel.ApplicationOffer),
		users:             set.NewStrings(),
		accessPerms:       make(map[offerAccess]permission.Access),
	}
	s.mockStatePool.st[modelUUID] = st
	st.applicationOffers[offerName] = jujucrossmodel.ApplicationOffer{}
}

func (s *offerAccessSuite) TestGrantMissingUserFails(c *gc.C) {
	s.setupOffer("uuid", "test", "admin", "someoffer")
	user := names.NewUserTag("foobar")
	err := s.grant(c, user, params.OfferReadAccess, "test.someoffer")
	expectedErr := `could not grant offer access: user "foobar" not found`
	c.Assert(err, gc.ErrorMatches, expectedErr)
}

func (s *offerAccessSuite) TestGrantMissingOfferFails(c *gc.C) {
	s.setupOffer("uuid", "test", "admin", "differentoffer")
	user := names.NewUserTag("foobar")
	err := s.grant(c, user, params.OfferReadAccess, "test.someoffer")
	expectedErr := `.*application offer "someoffer" not found`
	c.Assert(err, gc.ErrorMatches, expectedErr)
}

func (s *offerAccessSuite) TestRevokeAdminLeavesReadAccess(c *gc.C) {
	s.setupOffer("uuid", "test", "admin", "someoffer")
	st := s.mockStatePool.st["uuid"]
	st.(*mockState).users.Add("foobar")

	user := names.NewUserTag("foobar")
	offer := names.NewApplicationOfferTag("someoffer")
	err := st.CreateOfferAccess(offer, user, permission.ConsumeAccess)
	c.Assert(err, jc.ErrorIsNil)

	err = s.revoke(c, user, params.OfferConsumeAccess, "test.someoffer")
	c.Assert(err, jc.ErrorIsNil)

	access, err := st.GetOfferAccess(offer, user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.ReadAccess)
}

func (s *offerAccessSuite) TestRevokeReadRemovesPermission(c *gc.C) {
	s.setupOffer("uuid", "test", "admin", "someoffer")
	st := s.mockStatePool.st["uuid"]
	st.(*mockState).users.Add("foobar")

	user := names.NewUserTag("foobar")
	offer := names.NewApplicationOfferTag("someoffer")
	err := st.CreateOfferAccess(offer, user, permission.ConsumeAccess)
	c.Assert(err, jc.ErrorIsNil)

	err = s.revoke(c, user, params.OfferReadAccess, "test.someoffer")
	c.Assert(err, gc.IsNil)

	_, err = st.GetOfferAccess(offer, user)
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *offerAccessSuite) TestRevokeMissingUser(c *gc.C) {
	s.setupOffer("uuid", "test", "admin", "someoffer")
	st := s.mockStatePool.st["uuid"]

	user := names.NewUserTag("bob")
	err := s.revoke(c, user, params.OfferReadAccess, "test.someoffer")
	c.Assert(err, gc.ErrorMatches, `could not revoke offer access: offer user "bob" does not exist`)

	offer := names.NewApplicationOfferTag("someoffer")
	_, err = st.GetOfferAccess(offer, user)
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *offerAccessSuite) TestGrantOnlyGreaterAccess(c *gc.C) {
	s.setupOffer("uuid", "test", "admin", "someoffer")
	st := s.mockStatePool.st["uuid"]
	st.(*mockState).users.Add("foobar")

	user := names.NewUserTag("foobar")
	err := s.grant(c, user, params.OfferReadAccess, "test.someoffer")
	c.Assert(err, jc.ErrorIsNil)

	err = s.grant(c, user, params.OfferReadAccess, "test.someoffer")
	c.Assert(err, gc.ErrorMatches, `user already has "read" access or greater`)
}

func (s *offerAccessSuite) assertGrantOfferAddUser(c *gc.C, user names.UserTag) {
	s.setupOffer("uuid", "test", "superuser-bob", "someoffer")
	st := s.mockStatePool.st["uuid"]
	st.(*mockState).users.Add("other")
	st.(*mockState).users.Add(user.Name())

	apiUser := names.NewUserTag("superuser-bob")
	s.authorizer.Tag = apiUser

	err := s.grant(c, user, params.OfferReadAccess, "superuser-bob/test.someoffer")
	c.Assert(err, jc.ErrorIsNil)

	offer := names.NewApplicationOfferTag("someoffer")
	access, err := st.GetOfferAccess(offer, user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.ReadAccess)
}

func (s *offerAccessSuite) TestGrantOfferAddLocalUser(c *gc.C) {
	s.assertGrantOfferAddUser(c, names.NewLocalUserTag("bob"))
}

func (s *offerAccessSuite) TestGrantOfferAddRemoteUser(c *gc.C) {
	s.assertGrantOfferAddUser(c, names.NewUserTag("bob@remote"))
}

func (s *offerAccessSuite) TestGrantOfferSuperUser(c *gc.C) {
	s.setupOffer("uuid", "test", "superuser-bob", "someoffer")
	st := s.mockStatePool.st["uuid"]
	st.(*mockState).users.Add("other")

	user := names.NewUserTag("superuser-bob")
	s.authorizer.Tag = user

	other := names.NewUserTag("other")
	err := s.grant(c, other, params.OfferReadAccess, "superuser-bob/test.someoffer")
	c.Assert(err, jc.ErrorIsNil)

	offer := names.NewApplicationOfferTag("someoffer")
	access, err := st.GetOfferAccess(offer, other)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.ReadAccess)
}

func (s *offerAccessSuite) TestGrantIncreaseAccess(c *gc.C) {
	s.setupOffer("uuid", "test", "other", "someoffer")
	st := s.mockStatePool.st["uuid"]
	st.(*mockState).users.Add("other")

	user := names.NewUserTag("other")
	s.authorizer.Tag = user
	s.authorizer.AdminTag = user

	offer := names.NewApplicationOfferTag("someoffer")
	err := st.CreateOfferAccess(offer, user, permission.ReadAccess)
	c.Assert(err, jc.ErrorIsNil)

	err = s.grant(c, user, params.OfferConsumeAccess, "other/test.someoffer")
	c.Assert(err, jc.ErrorIsNil)

	access, err := st.GetOfferAccess(offer, user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.ConsumeAccess)
}

func (s *offerAccessSuite) TestGrantToOfferNoAccess(c *gc.C) {
	s.setupOffer("uuid", "test", "bob@remote", "someoffer")
	st := s.mockStatePool.st["uuid"]
	st.(*mockState).users.Add("other")
	st.(*mockState).users.Add("bob")

	user := names.NewUserTag("bob@remote")
	s.authorizer.Tag = user

	other := names.NewUserTag("other@remote")
	err := s.grant(c, other, params.OfferReadAccess, "bob@remote/test.someoffer")
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *offerAccessSuite) assertGrantToOffer(c *gc.C, userAccess permission.Access) {
	s.setupOffer("uuid", "test", "bob@remote", "someoffer")
	st := s.mockStatePool.st["uuid"]
	st.(*mockState).users.Add("other")
	st.(*mockState).users.Add("bob")

	user := names.NewUserTag("bob@remote")
	s.authorizer.Tag = user

	offer := names.NewApplicationOfferTag("someoffer")
	err := st.CreateOfferAccess(offer, user, userAccess)
	c.Assert(err, jc.ErrorIsNil)

	other := names.NewUserTag("other@remote")
	err = s.grant(c, other, params.OfferReadAccess, "bob@remote/test.someoffer")
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *offerAccessSuite) TestGrantToOfferReadAccess(c *gc.C) {
	s.assertGrantToOffer(c, permission.ReadAccess)
}

func (s *offerAccessSuite) TestGrantToOfferConsumeAccess(c *gc.C) {
	s.assertGrantToOffer(c, permission.ConsumeAccess)
}

func (s *offerAccessSuite) TestGrantToOfferAdminAccess(c *gc.C) {
	s.setupOffer("uuid", "test", "foobar", "someoffer")
	st := s.mockStatePool.st["uuid"]
	st.(*mockState).users.Add("other")
	st.(*mockState).users.Add("foobar")

	user := names.NewUserTag("foobar")
	s.authorizer.Tag = user
	s.authorizer.AdminTag = user
	offer := names.NewApplicationOfferTag("someoffer")
	err := st.CreateOfferAccess(offer, user, permission.AdminAccess)
	c.Assert(err, jc.ErrorIsNil)

	other := names.NewUserTag("other")
	err = s.grant(c, other, params.OfferReadAccess, "foobar/test.someoffer")
	c.Assert(err, jc.ErrorIsNil)

	access, err := st.GetOfferAccess(offer, other)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.ReadAccess)
}

func (s *offerAccessSuite) TestGrantOfferInvalidUserTag(c *gc.C) {
	s.setupOffer("uuid", "test", "admin", "someoffer")
	for _, testParam := range []struct {
		tag      string
		validTag bool
	}{{
		tag:      "unit-foo/0",
		validTag: true,
	}, {
		tag:      "application-foo",
		validTag: true,
	}, {
		tag:      "relation-wordpress:db mysql:db",
		validTag: true,
	}, {
		tag:      "machine-0",
		validTag: true,
	}, {
		tag:      "user",
		validTag: false,
	}, {
		tag:      "user-Mua^h^h^h^arh",
		validTag: true,
	}, {
		tag:      "user@",
		validTag: false,
	}, {
		tag:      "user@ubuntuone",
		validTag: false,
	}, {
		tag:      "user@ubuntuone",
		validTag: false,
	}, {
		tag:      "@ubuntuone",
		validTag: false,
	}, {
		tag:      "in^valid.",
		validTag: false,
	}, {
		tag:      "",
		validTag: false,
	},
	} {
		var expectedErr string
		errPart := `could not modify offer access: "` + regexp.QuoteMeta(testParam.tag) + `" is not a valid `

		if testParam.validTag {
			// The string is a valid tag, but not a user tag.
			expectedErr = errPart + `user tag`
		} else {
			// The string is not a valid tag of any kind.
			expectedErr = errPart + `tag`
		}

		args := params.ModifyOfferAccessRequest{
			Changes: []params.ModifyOfferAccess{{
				UserTag:  testParam.tag,
				Action:   params.GrantOfferAccess,
				Access:   params.OfferReadAccess,
				OfferURL: "test.someoffer",
			}}}

		result, err := s.api.ModifyOfferAccess(args)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result.OneError(), gc.ErrorMatches, expectedErr)
	}
}

func (s *offerAccessSuite) TestModifyOfferAccessEmptyArgs(c *gc.C) {
	s.setupOffer("uuid", "test", "admin", "someoffer")
	args := params.ModifyOfferAccessRequest{
		Changes: []params.ModifyOfferAccess{{OfferURL: "test.someoffer"}}}

	result, err := s.api.ModifyOfferAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := `could not modify offer access: "" offer access not valid`
	c.Assert(result.OneError(), gc.ErrorMatches, expectedErr)
}

func (s *offerAccessSuite) TestModifyOfferAccessInvalidAction(c *gc.C) {
	s.setupOffer("uuid", "test", "admin", "someoffer")

	var dance params.OfferAction = "dance"
	args := params.ModifyOfferAccessRequest{
		Changes: []params.ModifyOfferAccess{{
			UserTag:  "user-user",
			Action:   dance,
			Access:   params.OfferReadAccess,
			OfferURL: "test.someoffer",
		}}}

	result, err := s.api.ModifyOfferAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := `unknown action "dance"`
	c.Assert(result.OneError(), gc.ErrorMatches, expectedErr)
}
