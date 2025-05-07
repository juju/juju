// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"context"
	"strings"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/core/crossmodel"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

type grantRevokeSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fakeModelAPI  *fakeModelGrantRevokeAPI
	fakeOffersAPI *fakeOffersGrantRevokeAPI
	cmdFactory    func(*fakeModelGrantRevokeAPI, *fakeOffersGrantRevokeAPI) cmd.Command
	store         *jujuclient.MemStore
}

const (
	fooModelUUID    = "0701e916-3274-46e4-bd12-c31aff89cee3"
	barModelUUID    = "0701e916-3274-46e4-bd12-c31aff89cee4"
	bazModelUUID    = "0701e916-3274-46e4-bd12-c31aff89cee5"
	model1ModelUUID = "0701e916-3274-46e4-bd12-c31aff89cee6"
	model2ModelUUID = "0701e916-3274-46e4-bd12-c31aff89cee7"
)

func (s *grantRevokeSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.fakeModelAPI = &fakeModelGrantRevokeAPI{}
	s.fakeOffersAPI = &fakeOffersGrantRevokeAPI{}

	// Set up the current controller, and write just enough info
	// so we don't try to refresh
	controllerName := "test-master"

	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = controllerName
	s.store.Controllers[controllerName] = jujuclient.ControllerDetails{}
	s.store.Accounts[controllerName] = jujuclient.AccountDetails{
		User: "bob",
	}
	s.store.Models = map[string]*jujuclient.ControllerModels{
		controllerName: {
			Models: map[string]jujuclient.ModelDetails{
				"bob/foo":    {ModelUUID: fooModelUUID, ModelType: coremodel.IAAS},
				"bob/bar":    {ModelUUID: barModelUUID, ModelType: coremodel.IAAS},
				"bob/baz":    {ModelUUID: bazModelUUID, ModelType: coremodel.IAAS},
				"bob/model1": {ModelUUID: model1ModelUUID, ModelType: coremodel.IAAS},
				"bob/model2": {ModelUUID: model2ModelUUID, ModelType: coremodel.IAAS},
			},
		},
	}
}

func (s *grantRevokeSuite) run(c *tc.C, args ...string) (*cmd.Context, error) {
	command := s.cmdFactory(s.fakeModelAPI, s.fakeOffersAPI)
	return cmdtesting.RunCommand(c, command, args...)
}

func (s *grantRevokeSuite) TestPassesModelValues(c *tc.C) {
	user := "sam"
	models := []string{fooModelUUID, barModelUUID, bazModelUUID}
	_, err := s.run(c, "sam", "read", "foo", "bar", "baz")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fakeModelAPI.user, jc.DeepEquals, user)
	c.Assert(s.fakeModelAPI.modelUUIDs, jc.DeepEquals, models)
	c.Assert(s.fakeModelAPI.access, tc.Equals, "read")
}

func (s *grantRevokeSuite) TestPassesOfferValues(c *tc.C) {
	offers := []string{"bob/foo.hosted-mysql", "bob/bar.mysql", "bob/baz.hosted-db2"}
	_, err := s.run(c, "sam", "read", offers[0], offers[1], offers[2])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fakeOffersAPI.user, jc.DeepEquals, "sam")
	c.Assert(s.fakeOffersAPI.offerURLs, jc.SameContents, []string{"bob/foo.hosted-mysql", "bob/bar.mysql", "bob/baz.hosted-db2"})
	c.Assert(s.fakeOffersAPI.access, tc.Equals, "read")
}

func (s *grantRevokeSuite) TestPassesOfferWithDefaultModelUser(c *tc.C) {
	offer := "foo.hosted-mysql"
	_, err := s.run(c, "sam", "read", offer)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fakeOffersAPI.user, jc.DeepEquals, "sam")
	c.Assert(s.fakeOffersAPI.offerURLs, jc.SameContents, []string{"bob/foo.hosted-mysql"})
	c.Assert(s.fakeOffersAPI.access, tc.Equals, "read")
}

func (s *grantRevokeSuite) TestModelAccess(c *tc.C) {
	sam := "sam"
	_, err := s.run(c, "sam", "write", "model1", "model2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fakeModelAPI.user, jc.DeepEquals, sam)
	c.Assert(s.fakeModelAPI.modelUUIDs, jc.DeepEquals, []string{model1ModelUUID, model2ModelUUID})
	c.Assert(s.fakeModelAPI.access, tc.Equals, "write")
}

func (s *grantRevokeSuite) TestModelBlockGrant(c *tc.C) {
	s.fakeModelAPI.err = apiservererrors.OperationBlockedError("TestBlockGrant")
	_, err := s.run(c, "sam", "read", "foo")
	testing.AssertOperationWasBlocked(c, err, ".*TestBlockGrant.*")
}

type grantSuite struct {
	grantRevokeSuite
}

var _ = tc.Suite(&grantSuite{})

func (s *grantSuite) SetUpTest(c *tc.C) {
	s.grantRevokeSuite.SetUpTest(c)
	s.cmdFactory = func(fakeModelAPI *fakeModelGrantRevokeAPI, fakeOfferAPI *fakeOffersGrantRevokeAPI) cmd.Command {
		c, _ := model.NewGrantCommandForTest(fakeModelAPI, fakeOfferAPI, s.store)
		return c
	}
}

func (s *grantSuite) TestInitModels(c *tc.C) {
	wrappedCmd, grantCmd := model.NewGrantCommandForTest(nil, nil, s.store)
	err := cmdtesting.InitCommand(wrappedCmd, []string{})
	c.Assert(err, tc.ErrorMatches, "no user specified")

	err = cmdtesting.InitCommand(wrappedCmd, []string{"bob", "read", "model1", "model2"})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(grantCmd.User, tc.Equals, "bob")
	c.Assert(grantCmd.ModelNames, jc.DeepEquals, []string{"model1", "model2"})
	c.Assert(grantCmd.OfferURLs, tc.HasLen, 0)

	err = cmdtesting.InitCommand(wrappedCmd, []string{})
	c.Assert(err, tc.ErrorMatches, `no user specified`)
}

func (s *grantSuite) TestInitOffers(c *tc.C) {
	wrappedCmd, grantCmd := model.NewGrantCommandForTest(nil, nil, s.store)

	err := cmdtesting.InitCommand(wrappedCmd, []string{"bob", "read", "fred/model.offer1", "mary/model.offer2"})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(grantCmd.User, tc.Equals, "bob")
	url1, err := crossmodel.ParseOfferURL("fred/model.offer1")
	c.Assert(err, jc.ErrorIsNil)
	url2, err := crossmodel.ParseOfferURL("mary/model.offer2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(grantCmd.OfferURLs, jc.DeepEquals, []*crossmodel.OfferURL{url1, url2})
	c.Assert(grantCmd.ModelNames, tc.HasLen, 0)
}

type revokeSuite struct {
	grantRevokeSuite
}

var _ = tc.Suite(&revokeSuite{})

func (s *revokeSuite) SetUpTest(c *tc.C) {
	s.grantRevokeSuite.SetUpTest(c)
	s.cmdFactory = func(fakeModelAPI *fakeModelGrantRevokeAPI, fakeOffersAPI *fakeOffersGrantRevokeAPI) cmd.Command {
		c, _ := model.NewRevokeCommandForTest(fakeModelAPI, fakeOffersAPI, s.store)
		return c
	}
}

func (s *revokeSuite) TestInit(c *tc.C) {
	wrappedCmd, revokeCmd := model.NewRevokeCommandForTest(nil, nil, s.store)
	err := cmdtesting.InitCommand(wrappedCmd, []string{})
	c.Assert(err, tc.ErrorMatches, "no user specified")

	err = cmdtesting.InitCommand(wrappedCmd, []string{"bob", "read", "model1", "model2"})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(revokeCmd.User, tc.Equals, "bob")
	c.Assert(revokeCmd.ModelNames, jc.DeepEquals, []string{"model1", "model2"})

	err = cmdtesting.InitCommand(wrappedCmd, []string{})
	c.Assert(err, tc.ErrorMatches, `no user specified`)

}

func (s *grantSuite) TestModelAccessForController(c *tc.C) {
	wrappedCmd, _ := model.NewRevokeCommandForTest(nil, nil, s.store)
	err := cmdtesting.InitCommand(wrappedCmd, []string{"bob", "write"})
	msg := strings.Replace(err.Error(), "\n", "", -1)
	c.Check(msg, tc.Matches, `You have specified a model access permission "write".*`)
}

func (s *grantSuite) TestControllerAccessForModel(c *tc.C) {
	wrappedCmd, _ := model.NewRevokeCommandForTest(nil, nil, s.store)
	err := cmdtesting.InitCommand(wrappedCmd, []string{"bob", "superuser", "default"})
	msg := strings.Replace(err.Error(), "\n", "", -1)
	c.Check(msg, tc.Matches, `You have specified a controller access permission "superuser".*`)
}

func (s *grantSuite) TestControllerAccessForOffer(c *tc.C) {
	wrappedCmd, _ := model.NewRevokeCommandForTest(nil, nil, s.store)
	err := cmdtesting.InitCommand(wrappedCmd, []string{"bob", "superuser", "fred/default.mysql"})
	msg := strings.Replace(err.Error(), "\n", "", -1)
	c.Check(msg, tc.Matches, `You have specified a controller access permission "superuser".*`)
}

type fakeModelGrantRevokeAPI struct {
	err        error
	user       string
	access     string
	modelUUIDs []string
}

func (f *fakeModelGrantRevokeAPI) Close() error { return nil }

func (f *fakeModelGrantRevokeAPI) GrantModel(ctx context.Context, user, access string, modelUUIDs ...string) error {
	return f.fake(user, access, modelUUIDs...)
}

func (f *fakeModelGrantRevokeAPI) RevokeModel(ctx context.Context, user, access string, modelUUIDs ...string) error {
	return f.fake(user, access, modelUUIDs...)
}

func (f *fakeModelGrantRevokeAPI) fake(user, access string, modelUUIDs ...string) error {
	f.user = user
	f.access = access
	f.modelUUIDs = modelUUIDs
	return f.err
}

type fakeOffersGrantRevokeAPI struct {
	err       error
	user      string
	access    string
	offerURLs []string
}

func (f *fakeOffersGrantRevokeAPI) Close() error { return nil }

func (f *fakeOffersGrantRevokeAPI) GrantOffer(ctx context.Context, user, access string, offerURLs ...string) error {
	return f.fake(user, access, offerURLs...)
}

func (f *fakeOffersGrantRevokeAPI) RevokeOffer(ctx context.Context, user, access string, offerURLs ...string) error {
	return f.fake(user, access, offerURLs...)
}

func (f *fakeOffersGrantRevokeAPI) fake(user, access string, offerURLs ...string) error {
	f.user = user
	f.access = access
	f.offerURLs = append(f.offerURLs, offerURLs...)
	return f.err
}
