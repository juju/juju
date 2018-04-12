// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/crossmodel"
)

type offerSuite struct {
	BaseCrossModelSuite
	mockAPI *mockOfferAPI
	args    []string
}

var _ = gc.Suite(&offerSuite{})

func (s *offerSuite) SetUpTest(c *gc.C) {
	s.BaseCrossModelSuite.SetUpTest(c)

	s.mockAPI = newMockOfferAPI()
	s.args = nil
}

func (s *offerSuite) TestOfferNoArgs(c *gc.C) {
	s.assertOfferErrorOutput(c, ".*an offer must at least specify application endpoint.*")
}

func (s *offerSuite) TestOfferTooManyArgs(c *gc.C) {
	s.args = []string{"tst:db", "alias", "extra"}
	s.assertOfferErrorOutput(c, `unrecognized args: \["extra"\]`)
}

func (s *offerSuite) TestOfferInvalidApplication(c *gc.C) {
	s.args = []string{"123:"}
	s.assertOfferErrorOutput(c, `.*application name "123" not valid.*`)
}

func (s *offerSuite) TestOfferInvalidModel(c *gc.C) {
	s.args = []string{"$model.123:db"}
	s.assertOfferErrorOutput(c, `.*model name "\$model" not valid.*`)
}

func (s *offerSuite) TestOfferNoCurrentModel(c *gc.C) {
	s.store.Models["test-master"].CurrentModel = ""
	s.args = []string{"app:db"}
	s.assertOfferErrorOutput(c, `no current model, use juju switch to select a model on which to operate`)
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
	return cmdtesting.RunCommand(c, crossmodel.NewOfferCommandForTest(s.store, s.mockAPI), args...)
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
	s.assertOfferOutput(c, "test", "tst", "tst", []string{"db"})
	c.Assert(s.mockAPI.modelUUID, gc.Equals, "fred-uuid")
}

func (s *offerSuite) TestOfferWithAlias(c *gc.C) {
	s.args = []string{"tst:db", "hosted-tst"}
	s.assertOfferOutput(c, "test", "hosted-tst", "tst", []string{"db"})
}

func (s *offerSuite) TestOfferExplicitModel(c *gc.C) {
	s.args = []string{"bob/prod.tst:db"}
	s.assertOfferOutput(c, "prod", "tst", "tst", []string{"db"})
}

func (s *offerSuite) TestOfferMultipleEndpoints(c *gc.C) {
	s.args = []string{"tst:db,admin"}
	s.assertOfferOutput(c, "test", "tst", "tst", []string{"db", "admin"})
}

func (s *offerSuite) assertOfferOutput(c *gc.C, expectedModel, expectedOffer, expectedApplication string, endpoints []string) {
	_, err := s.runOffer(c, s.args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mockAPI.offers[expectedOffer], jc.SameContents, endpoints)
}

type mockOfferAPI struct {
	errCall, errData bool
	modelUUID        string
	offers           map[string][]string
	applications     map[string]string
	descs            map[string]string
}

func newMockOfferAPI() *mockOfferAPI {
	mock := &mockOfferAPI{}
	mock.offers = make(map[string][]string)
	mock.descs = make(map[string]string)
	mock.applications = make(map[string]string)
	return mock
}

func (s *mockOfferAPI) Close() error {
	return nil
}

func (s *mockOfferAPI) Offer(modelUUID, application string, endpoints []string, offerName, desc string) ([]params.ErrorResult, error) {
	if s.errCall {
		return nil, errors.New("aborted")
	}
	result := make([]params.ErrorResult, 1)
	if s.errData {
		result[0].Error = common.ServerError(errors.New("failed"))
		return result, nil
	}
	s.modelUUID = modelUUID
	if offerName == "" {
		offerName = application
	}
	s.offers[offerName] = endpoints
	s.applications[offerName] = application
	s.descs[offerName] = desc
	return result, nil
}
