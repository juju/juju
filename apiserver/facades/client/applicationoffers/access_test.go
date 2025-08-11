// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers_test

import (
	"regexp"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facades/client/applicationoffers"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type offerAccessSuite struct {
	baseSuite
	api *applicationoffers.OffersAPIv5
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
	thirdPartyKey := bakery.MustGenerateKey()
	s.authContext, err = crossmodel.NewAuthContext(
		s.mockState, thirdPartyKey,
		crossmodel.NewOfferBakeryForTest(s.bakery, clock.WallClock),
	)
	c.Assert(err, jc.ErrorIsNil)

	api, err := applicationoffers.CreateOffersAPI(
		getApplicationOffers, nil, getFakeControllerInfo,
		s.mockState, s.mockStatePool, s.authorizer, resources, s.authContext,
	)
	c.Assert(err, jc.ErrorIsNil)

	s.api = api
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

func (s *offerAccessSuite) setupOffer(modelUUID, modelName, owner, offerName string) string {
	model := &mockModel{uuid: modelUUID, name: modelName, owner: owner, modelType: state.ModelTypeIAAS}
	s.mockState.allmodels = []applicationoffers.Model{model}
	st := &mockState{
		modelUUID:         modelUUID,
		applicationOffers: make(map[string]jujucrossmodel.ApplicationOffer),
		users:             make(map[string]applicationoffers.User),
		accessPerms:       make(map[offerAccess]permission.Access),
		model:             model,
	}
	s.mockStatePool.st[modelUUID] = st
	uuid := utils.MustNewUUID().String()
	st.applicationOffers[offerName] = jujucrossmodel.ApplicationOffer{OfferUUID: uuid}
	return uuid
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
	offerUUID := s.setupOffer("uuid", "test", "admin", "someoffer")
	st := s.mockStatePool.st["uuid"]
	st.(*mockState).users["foobar"] = &mockUser{"foobar"}

	user := names.NewUserTag("foobar")
	offer := names.NewApplicationOfferTag(offerUUID)
	err := st.CreateOfferAccess(offer, user, permission.ConsumeAccess)
	c.Assert(err, jc.ErrorIsNil)

	err = s.revoke(c, user, params.OfferConsumeAccess, "test.someoffer")
	c.Assert(err, jc.ErrorIsNil)

	access, err := st.GetOfferAccess(offer.Id(), user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.ReadAccess)
}

func (s *offerAccessSuite) TestRevokeReadRemovesPermission(c *gc.C) {
	offerUUID := s.setupOffer("uuid", "test", "admin", "someoffer")
	st := s.mockStatePool.st["uuid"]
	st.(*mockState).users["foobar"] = &mockUser{"foobar"}

	user := names.NewUserTag("foobar")
	offer := names.NewApplicationOfferTag(offerUUID)
	err := st.CreateOfferAccess(offer, user, permission.ConsumeAccess)
	c.Assert(err, jc.ErrorIsNil)

	err = s.revoke(c, user, params.OfferReadAccess, "test.someoffer")
	c.Assert(err, gc.IsNil)

	_, err = st.GetOfferAccess(offer.Id(), user)
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *offerAccessSuite) TestRevokeMissingUser(c *gc.C) {
	offerUUID := s.setupOffer("uuid", "test", "admin", "someoffer")
	st := s.mockStatePool.st["uuid"]

	user := names.NewUserTag("bob")
	err := s.revoke(c, user, params.OfferReadAccess, "test.someoffer")
	c.Assert(err, gc.ErrorMatches, `could not revoke offer access: offer user "bob" does not exist`)

	offer := names.NewApplicationOfferTag(offerUUID)
	_, err = st.GetOfferAccess(offer.Id(), user)
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *offerAccessSuite) TestGrantOnlyGreaterAccess(c *gc.C) {
	s.setupOffer("uuid", "test", "admin", "someoffer")
	st := s.mockStatePool.st["uuid"]
	st.(*mockState).users["foobar"] = &mockUser{"foobar"}

	user := names.NewUserTag("foobar")
	err := s.grant(c, user, params.OfferReadAccess, "test.someoffer")
	c.Assert(err, jc.ErrorIsNil)

	err = s.grant(c, user, params.OfferReadAccess, "test.someoffer")
	c.Assert(err, gc.ErrorMatches, `user already has "read" access or greater`)
}

func (s *offerAccessSuite) assertGrantOfferAddUser(c *gc.C, user names.UserTag) {
	offerUUID := s.setupOffer("uuid", "test", "superuser-bob", "someoffer")
	st := s.mockStatePool.st["uuid"]
	st.(*mockState).users["other"] = &mockUser{"other"}
	st.(*mockState).users[user.Name()] = &mockUser{user.Name()}

	apiUser := names.NewUserTag("superuser-bob")
	s.authorizer.Tag = apiUser

	err := s.grant(c, user, params.OfferReadAccess, "superuser-bob/test.someoffer")
	c.Assert(err, jc.ErrorIsNil)

	offer := names.NewApplicationOfferTag(offerUUID)
	access, err := st.GetOfferAccess(offer.Id(), user)
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
	offerUUID := s.setupOffer("uuid", "test", "superuser-bob", "someoffer")
	st := s.mockStatePool.st["uuid"]
	st.(*mockState).users["other"] = &mockUser{"other"}

	user := names.NewUserTag("superuser-bob")
	s.authorizer.Tag = user

	other := names.NewUserTag("other")
	err := s.grant(c, other, params.OfferReadAccess, "superuser-bob/test.someoffer")
	c.Assert(err, jc.ErrorIsNil)

	offer := names.NewApplicationOfferTag(offerUUID)
	access, err := st.GetOfferAccess(offer.Id(), other)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.ReadAccess)
}

func (s *offerAccessSuite) TestGrantIncreaseAccess(c *gc.C) {
	offerUUID := s.setupOffer("uuid", "test", "other", "someoffer")
	st := s.mockStatePool.st["uuid"]
	st.(*mockState).users["other"] = &mockUser{"other"}

	user := names.NewUserTag("other")
	s.authorizer.Tag = user
	s.authorizer.AdminTag = user

	offer := names.NewApplicationOfferTag(offerUUID)
	err := st.CreateOfferAccess(offer, user, permission.ReadAccess)
	c.Assert(err, jc.ErrorIsNil)

	err = s.grant(c, user, params.OfferConsumeAccess, "other/test.someoffer")
	c.Assert(err, jc.ErrorIsNil)

	access, err := st.GetOfferAccess(offer.Id(), user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.ConsumeAccess)
}

func (s *offerAccessSuite) TestGrantToOfferNoAccess(c *gc.C) {
	s.setupOffer("uuid", "test", "bob@remote", "someoffer")
	st := s.mockStatePool.st["uuid"]
	st.(*mockState).users["other"] = &mockUser{"other"}
	st.(*mockState).users["bob"] = &mockUser{"bob"}

	user := names.NewUserTag("bob@remote")
	s.authorizer.Tag = user

	other := names.NewUserTag("other@remote")
	err := s.grant(c, other, params.OfferReadAccess, "bob@remote/test.someoffer")
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *offerAccessSuite) assertGrantToOffer(c *gc.C, userAccess permission.Access) {
	offerUUID := s.setupOffer("uuid", "test", "bob@remote", "someoffer")
	st := s.mockStatePool.st["uuid"]
	st.(*mockState).users["other"] = &mockUser{"other"}
	st.(*mockState).users["bob"] = &mockUser{"bob"}

	user := names.NewUserTag("bob@remote")
	s.authorizer.Tag = user

	offer := names.NewApplicationOfferTag(offerUUID)
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
	offerUUID := s.setupOffer("uuid", "test", "foobar", "someoffer")
	st := s.mockStatePool.st["uuid"]
	st.(*mockState).users["other"] = &mockUser{"other"}
	st.(*mockState).users["foobar"] = &mockUser{"foobar"}

	user := names.NewUserTag("foobar")
	s.authorizer.Tag = user
	s.authorizer.AdminTag = user
	offer := names.NewApplicationOfferTag(offerUUID)
	err := st.CreateOfferAccess(offer, user, permission.AdminAccess)
	c.Assert(err, jc.ErrorIsNil)

	other := names.NewUserTag("other")
	err = s.grant(c, other, params.OfferReadAccess, "foobar/test.someoffer")
	c.Assert(err, jc.ErrorIsNil)

	access, err := st.GetOfferAccess(offer.Id(), other)
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

// TestModifyOfferAccessForModelAdminPermission tests modifying offer access when authorized as model admin.
// It validates bugfix https://bugs.launchpad.net/juju/+bug/2082494
func (s *offerAccessSuite) TestModifyOfferAccessForModelAdminPermission(c *gc.C) {
	modelUUID := utils.MustNewUUID().String()
	s.setupOffer(modelUUID, "test", "admin", "someoffer")
	st := s.mockStatePool.st[modelUUID]
	st.(*mockState).users["luke"] = &mockUser{"luke"}

	s.authorizer.Tag = names.NewUserTag("admin-model-" + modelUUID)
	args := params.ModifyOfferAccessRequest{
		Changes: []params.ModifyOfferAccess{{
			UserTag:  "user-luke",
			Action:   params.GrantOfferAccess,
			Access:   params.OfferReadAccess,
			OfferURL: "admin/test.someoffer",
		}}}

	result, err := s.api.ModifyOfferAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.OneError(), jc.ErrorIsNil)
}

type offerAdminAccessSuite struct {
	baseSuite
	authorizer *MockAuthorizer
}

var _ = gc.Suite(&offerAdminAccessSuite{})

func (s *offerAdminAccessSuite) getAPI(c *gc.C) *applicationoffers.OffersAPIv5 {
	getApplicationOffers := func(interface{}) jujucrossmodel.ApplicationOffers {
		return &stubApplicationOffers{}
	}

	resources := common.NewResources()
	resources.RegisterNamed("dataDir", common.StringResource(c.MkDir()))

	api, err := applicationoffers.CreateOffersAPI(
		getApplicationOffers, nil, getFakeControllerInfo,
		s.mockState, s.mockStatePool, s.authorizer, resources, nil,
	)
	c.Assert(err, jc.ErrorIsNil)

	return api
}

func (s *offerAdminAccessSuite) setupOffer(modelUUID, modelName, owner, offerName string) string {
	model := &mockModel{uuid: modelUUID, name: modelName, owner: owner, modelType: state.ModelTypeIAAS}
	s.mockState.allmodels = []applicationoffers.Model{model}
	st := &mockState{
		modelUUID:         modelUUID,
		applicationOffers: make(map[string]jujucrossmodel.ApplicationOffer),
		users:             make(map[string]applicationoffers.User),
		accessPerms:       make(map[offerAccess]permission.Access),
		model:             model,
	}
	s.mockStatePool.st[modelUUID] = st
	uuid := utils.MustNewUUID().String()
	st.applicationOffers[offerName] = jujucrossmodel.ApplicationOffer{OfferUUID: uuid}
	return uuid
}

// TestModifyOfferAccessForOfferAdminPermission tests modifying offer access
// when authorized as offer admin.
func (s *offerAdminAccessSuite) TestModifyOfferAccessForOfferAdminPermission(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.authorizer = NewMockAuthorizer(ctrl)

	s.authorizer.EXPECT().AuthClient().Return(true).AnyTimes()
	s.authorizer.EXPECT().GetAuthTag().Return(names.NewUserTag("admin"))
	s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, gomock.AssignableToTypeOf(names.ControllerTag{})).Return(authentication.ErrorEntityMissingPermission)
	s.authorizer.EXPECT().HasPermission(permission.AdminAccess, gomock.AssignableToTypeOf(names.ModelTag{})).Return(authentication.ErrorEntityMissingPermission)
	s.authorizer.EXPECT().HasPermission(permission.AdminAccess, gomock.AssignableToTypeOf(names.ApplicationOfferTag{})).Return(nil)

	modelUUID := utils.MustNewUUID().String()
	s.setupOffer(modelUUID, "test", "admin", "someoffer")
	st := s.mockStatePool.st[modelUUID]
	st.(*mockState).users["luke"] = &mockUser{"luke"}

	args := params.ModifyOfferAccessRequest{
		Changes: []params.ModifyOfferAccess{{
			UserTag:  "user-luke",
			Action:   params.GrantOfferAccess,
			Access:   params.OfferReadAccess,
			OfferURL: "admin/test.someoffer",
		}}}

	result, err := s.getAPI(c).ModifyOfferAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.OneError(), jc.ErrorIsNil)
}
