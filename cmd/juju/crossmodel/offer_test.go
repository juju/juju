// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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

	s.mockAPI = NewMockOfferAPI()
	s.args = nil
}

func (s *offerSuite) TestOfferNoArgs(c *gc.C) {
	s.assertOfferErrorOutput(c, ".*an offer must at least specify service endpoint.*")
}

func (s *offerSuite) TestOfferInvalidService(c *gc.C) {
	s.args = []string{"123:"}
	s.assertOfferErrorOutput(c, `.*service name "123" is not valid.*`)
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
	return testing.RunCommand(c, crossmodel.NewOfferCommandForTest(s.mockAPI), args...)
}

func (s *offerSuite) TestOfferErred(c *gc.C) {
	s.args = []string{"tst:db"}
	s.mockAPI.abort = true
	s.assertOfferErrorOutput(c, ".*aborted.*")
}

func (s *offerSuite) TestOfferValid(c *gc.C) {
	s.args = []string{"tst:db"}
	s.assertOfferOutput(c, "service-tst", []string{"db"}, "", nil)
}

func (s *offerSuite) TestOfferWithURL(c *gc.C) {
	s.args = []string{"tst:db", "valid url"}
	s.assertOfferOutput(c, "service-tst", []string{"db"}, "valid url", nil)
}

func (s *offerSuite) TestOfferToInvalidUser(c *gc.C) {
	s.args = []string{"tst:db", "--to", "b_b"}
	s.assertOfferErrorOutput(c, `.*user name "b_b" is not valid.*`)
}

func (s *offerSuite) TestOfferToUser(c *gc.C) {
	s.args = []string{"tst:db", "--to", "blah"}
	s.assertOfferOutput(c, "service-tst", []string{"db"}, "", []string{"user-blah"})
}

func (s *offerSuite) TestOfferToUsers(c *gc.C) {
	s.args = []string{"tst:db", "--to", "blah,fluff"}
	s.assertOfferOutput(c, "service-tst", []string{"db"}, "", []string{"user-blah", "user-fluff"})
}

func (s *offerSuite) TestOfferMultipleEndpoints(c *gc.C) {
	s.args = []string{"tst:db,admin"}
	s.assertOfferOutput(c, "service-tst", []string{"db", "admin"}, "", nil)
}

func (s *offerSuite) TestOfferAllArgs(c *gc.C) {
	s.args = []string{"tst:db", "valid url", "--to", "blah"}
	s.assertOfferOutput(c, "service-tst", []string{"db"}, "valid url", []string{"user-blah"})
}

func (s *offerSuite) assertOfferOutput(c *gc.C, expectedService string, endpoints []string, url string, users []string) {
	_, err := s.runOffer(c, s.args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mockAPI.offers[expectedService], jc.SameContents, endpoints)
	c.Assert(s.mockAPI.urls[expectedService], jc.DeepEquals, url)
	c.Assert(s.mockAPI.users[expectedService], jc.SameContents, users)
}

type mockOfferAPI struct {
	abort  bool
	offers map[string][]string
	users  map[string][]string
	urls   map[string]string
}

func NewMockOfferAPI() *mockOfferAPI {
	mock := &mockOfferAPI{}
	mock.offers = make(map[string][]string)
	mock.users = make(map[string][]string)
	mock.urls = make(map[string]string)
	return mock
}

func (s mockOfferAPI) Close() error {
	return nil
}

func (s mockOfferAPI) Offer(service string, endpoints []string, url string, users []string) error {
	if s.abort {
		return errors.New("aborted")
	}
	s.offers[service] = endpoints
	s.urls[service] = url
	s.users[service] = users
	return nil
}
