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
	"github.com/juju/juju/testing"
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

func (s *offerSuite) TestOfferTooFewArgs(c *gc.C) {
	s.args = []string{"tst:db"}
	s.assertOfferErrorOutput(c, "an offer must specify a url")
}

func (s *offerSuite) TestOfferInvalidApplication(c *gc.C) {
	s.args = []string{"123:", "local:/u/bob/testing/tst"}
	s.assertOfferErrorOutput(c, `.*application name "123" not valid.*`)
}

func (s *offerSuite) TestOfferInvalidEndpoints(c *gc.C) {
	s.args = []string{"tst/123", "local:/u/bob/testing/tst"}
	s.assertOfferErrorOutput(c, `.*endpoints must conform to format.*`)
}

func (s *offerSuite) TestOfferNoEndpoints(c *gc.C) {
	s.args = []string{"tst:", "local:/u/bob/testing/tst"}
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
	s.args = []string{"tst:db", "local:/u/bob/tst"}
	s.mockAPI.errCall = true
	s.assertOfferErrorOutput(c, ".*aborted.*")
}

func (s *offerSuite) TestOfferDataErred(c *gc.C) {
	s.args = []string{"tst:db", "local:/u/bob/tst"}
	s.mockAPI.errData = true
	s.assertOfferErrorOutput(c, ".*failed.*")
}

func (s *offerSuite) TestOfferValid(c *gc.C) {
	s.args = []string{"tst:db", "local:/u/bob/tst"}
	s.assertOfferOutput(c, "test", "tst", []string{"db"}, "local:/u/bob/tst")
}

func (s *offerSuite) TestOfferExplicitModel(c *gc.C) {
	s.args = []string{"prod.tst:db", "local:/u/bob/tst"}
	s.assertOfferOutput(c, "prod", "tst", []string{"db"}, "local:/u/bob/tst")
}

func (s *offerSuite) TestOfferWithURL(c *gc.C) {
	s.args = []string{"tst:db", "/u/user/offer"}
	s.assertOfferOutput(c, "test", "tst", []string{"db"}, "/u/user/offer")
}

func (s *offerSuite) TestOfferMultipleEndpoints(c *gc.C) {
	s.args = []string{"tst:db,admin", "local:/u/bob/tst"}
	s.assertOfferOutput(c, "test", "tst", []string{"db", "admin"}, "local:/u/bob/tst")
}

func (s *offerSuite) assertOfferOutput(c *gc.C, expectedModel, expectedApplication string, endpoints []string, url string) {
	_, err := s.runOffer(c, s.args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mockAPI.offers[expectedApplication], jc.SameContents, endpoints)
	c.Assert(s.mockAPI.urls[expectedApplication], jc.DeepEquals, url)
}

type mockOfferAPI struct {
	errCall, errData bool
	offers           map[string][]string
	urls             map[string]string
	descs            map[string]string
}

func newMockOfferAPI() *mockOfferAPI {
	mock := &mockOfferAPI{}
	mock.offers = make(map[string][]string)
	mock.urls = make(map[string]string)
	mock.descs = make(map[string]string)
	return mock
}

func (s *mockOfferAPI) Close() error {
	return nil
}

func (s *mockOfferAPI) Offer(application string, endpoints []string, url string, desc string) ([]params.ErrorResult, error) {
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
	s.descs[application] = desc
	return result, nil
}
