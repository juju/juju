// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/crossmodel"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type offerSuite struct {
	BaseCrossModelSuite
	store   *jujuclienttesting.MemStore
	mockAPI *mockOfferAPI
	args    []string
}

var _ = gc.Suite(&offerSuite{})

func (s *offerSuite) SetUpTest(c *gc.C) {
	s.BaseCrossModelSuite.SetUpTest(c)

	s.mockAPI = NewMockOfferAPI()
	s.args = nil

	controllerName := "test-master"
	s.store = jujuclienttesting.NewMemStore()
	s.store.CurrentControllerName = controllerName
	s.store.Controllers[controllerName] = jujuclient.ControllerDetails{}
	s.store.Models[controllerName] = &jujuclient.ControllerModels{
		CurrentModel: "testing",
	}
	s.store.Accounts[controllerName] = jujuclient.AccountDetails{
		User: "bob",
	}
}

func (s *offerSuite) TestOfferNoArgs(c *gc.C) {
	s.assertOfferErrorOutput(c, ".*an offer must at least specify application endpoint.*")
}

func (s *offerSuite) TestOfferInvalidApplication(c *gc.C) {
	s.args = []string{"123:"}
	s.assertOfferErrorOutput(c, `.*application name "123" not valid.*`)
}

func (s *offerSuite) TestOfferInvalidEndpoints(c *gc.C) {
	s.args = []string{"tst/123"}
	s.assertOfferErrorOutput(c, `.*endpoints must conform to format.*`)
}

func (s *offerSuite) TestOfferNoEndpoints(c *gc.C) {
	s.args = []string{"tst:"}
	s.assertOfferErrorOutput(c, `.*specify endpoints for tst.*`)
}

func (s *offerSuite) assertOfferErrorOutput(c *gc.C, expected string) {
	_, err := s.runOffer(c, s.args...)
	c.Assert(errors.Cause(err), gc.ErrorMatches, expected)
}

func (s *offerSuite) runOffer(c *gc.C, args ...string) (*cmd.Context, error) {
	return testing.RunCommand(c, crossmodel.NewOfferCommandForTest(s.store, s.mockAPI), args...)
}

func (s *offerSuite) TestOfferCallErred(c *gc.C) {
	s.args = []string{"tst:db"}
	s.mockAPI.errCall = true
	s.assertOfferErrorOutput(c, ".*aborted.*")
}

func (s *offerSuite) TestOfferDataErred(c *gc.C) {
	s.args = []string{"tst:db"}
	s.mockAPI.errData = true
	s.assertOfferErrorOutput(c, ".*failed.*")
}

func (s *offerSuite) TestOfferValid(c *gc.C) {
	s.args = []string{"tst:db"}
	s.assertOfferOutput(c, "tst", []string{"db"}, "local:/u/bob/testing/tst", nil)
}

func (s *offerSuite) TestOfferWithURL(c *gc.C) {
	s.args = []string{"tst:db", "/u/user/offer"}
	s.assertOfferOutput(c, "tst", []string{"db"}, "/u/user/offer", nil)
}

func (s *offerSuite) TestOfferToInvalidUser(c *gc.C) {
	s.args = []string{"tst:db", "--to", "b_b"}
	s.assertOfferErrorOutput(c, `.*user name "b_b" not valid.*`)
}

func (s *offerSuite) TestOfferToUser(c *gc.C) {
	s.args = []string{"tst:db", "--to", "blah"}
	s.assertOfferOutput(c, "tst", []string{"db"}, "local:/u/bob/testing/tst", []string{"user-blah"})
}

func (s *offerSuite) TestOfferToUsers(c *gc.C) {
	s.args = []string{"tst:db", "--to", "blah,fluff"}
	s.assertOfferOutput(c, "tst", []string{"db"}, "local:/u/bob/testing/tst", []string{"user-blah", "user-fluff"})
}

func (s *offerSuite) TestOfferMultipleEndpoints(c *gc.C) {
	s.args = []string{"tst:db,admin"}
	s.assertOfferOutput(c, "tst", []string{"db", "admin"}, "local:/u/bob/testing/tst", nil)
}

func (s *offerSuite) TestOfferAllArgs(c *gc.C) {
	s.args = []string{"tst:db", "/u/user/offer", "--to", "blah"}
	s.assertOfferOutput(c, "tst", []string{"db"}, "/u/user/offer", []string{"user-blah"})
}

func (s *offerSuite) assertOfferOutput(c *gc.C, expectedApplication string, endpoints []string, url string, users []string) {
	_, err := s.runOffer(c, s.args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mockAPI.offers[expectedApplication], jc.SameContents, endpoints)
	c.Assert(s.mockAPI.urls[expectedApplication], jc.DeepEquals, url)
	c.Assert(s.mockAPI.users[expectedApplication], jc.SameContents, users)
}

type mockOfferAPI struct {
	errCall, errData bool
	offers           map[string][]string
	users            map[string][]string
	urls             map[string]string
	descs            map[string]string
}

func NewMockOfferAPI() *mockOfferAPI {
	mock := &mockOfferAPI{}
	mock.offers = make(map[string][]string)
	mock.users = make(map[string][]string)
	mock.urls = make(map[string]string)
	mock.descs = make(map[string]string)
	return mock
}

func (s mockOfferAPI) Close() error {
	return nil
}

func (s mockOfferAPI) Offer(application string, endpoints []string, url string, users []string, desc string) ([]params.ErrorResult, error) {
	if s.errCall {
		return nil, errors.New("aborted")
	}
	result := make([]params.ErrorResult, 1)
	if s.errData {
		result[0].Error = common.ServerError(errors.New("failed"))
		return result, nil
	}
	s.offers[application] = endpoints
	s.urls[application] = url
	s.users[application] = users
	s.descs[application] = desc
	return result, nil
}
