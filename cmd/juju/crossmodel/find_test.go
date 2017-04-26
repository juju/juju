// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/crossmodel"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
)

type findSuite struct {
	BaseCrossModelSuite
	mockAPI *mockFindAPI
}

var _ = gc.Suite(&findSuite{})

func (s *findSuite) SetUpTest(c *gc.C) {
	s.BaseCrossModelSuite.SetUpTest(c)

	s.mockAPI = &mockFindAPI{
		offerName:         "hosted-db2",
		expectedModelName: "test",
	}
}

func (s *findSuite) runFind(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, crossmodel.NewFindEndpointsCommandForTest(s.store, s.mockAPI), args...)
}

func (s *findSuite) TestFindNoArgs(c *gc.C) {
	s.mockAPI.c = c
	s.mockAPI.expectedFilter = &jujucrossmodel.ApplicationOfferFilter{}
	s.assertFind(
		c,
		[]string{},
		`
URL                   Access   Interfaces
fred/test.hosted-db2  consume  http:db2, http:log

`[1:],
	)
}

func (s *findSuite) TestFindDuplicateUrl(c *gc.C) {
	s.assertFindError(c, []string{"url", "--url", "urlparam"}, ".*URL term cannot be specified twice.*")
}

func (s *findSuite) TestFindDifferentController(c *gc.C) {
	s.assertFindError(c, []string{"different:user/model.offer"}, `finding endpoints from another controller "different" not supported`)
}

func (s *findSuite) TestNoResults(c *gc.C) {
	s.mockAPI.c = c
	s.mockAPI.expectedModelName = "none"
	s.mockAPI.expectedFilter = &jujucrossmodel.ApplicationOfferFilter{
		OwnerName: "bob",
		ModelName: "none",
	}
	s.mockAPI.results = []params.ApplicationOffer{}
	s.assertFindError(
		c,
		[]string{"--url", "none"},
		`no matching application offers found`,
	)
}

func (s *findSuite) TestSimpleFilter(c *gc.C) {
	s.mockAPI.c = c
	s.mockAPI.expectedModelName = "model"
	s.mockAPI.expectedFilter = &jujucrossmodel.ApplicationOfferFilter{
		OfferName: "hosted-db2",
		OwnerName: "fred",
		ModelName: "model",
	}
	s.mockAPI.expectedModelName = "model"
	s.assertFind(
		c,
		[]string{"--format", "tabular", "--url", "fred/model.hosted-db2"},
		`
URL                    Access   Interfaces
fred/model.hosted-db2  consume  http:db2, http:log

`[1:],
	)
}

func (s *findSuite) TestEndpointFilter(c *gc.C) {
	s.mockAPI.c = c
	s.mockAPI.expectedFilter = &jujucrossmodel.ApplicationOfferFilter{
		OwnerName: "fred",
		ModelName: "model",
		Endpoints: []jujucrossmodel.EndpointFilterTerm{{
			Interface: "mysql",
			Name:      "db",
		}},
	}
	s.mockAPI.expectedModelName = "model"
	s.assertFind(
		c,
		[]string{"--format", "tabular", "--url", "fred/model", "--endpoint", "db", "--interface", "mysql"},
		`
URL                    Access   Interfaces
fred/model.hosted-db2  consume  http:db2, http:log

`[1:],
	)
}

func (s *findSuite) TestFindApiError(c *gc.C) {
	s.mockAPI.msg = "fail"
	s.assertFindError(c, []string{"fred/model.db2"}, ".*fail.*")
}

func (s *findSuite) TestFindYaml(c *gc.C) {
	s.mockAPI.expectedModelName = "model"
	s.assertFind(
		c,
		[]string{"fred/model.hosted-db2", "--format", "yaml"},
		`
fred/model.hosted-db2:
  access: consume
  endpoints:
    db2:
      interface: http
      role: requirer
    log:
      interface: http
      role: provider
`[1:],
	)
}

func (s *findSuite) TestFindTabular(c *gc.C) {
	s.mockAPI.expectedModelName = "model"
	s.assertFind(
		c,
		[]string{"fred/model.hosted-db2", "--format", "tabular"},
		`
URL                    Access   Interfaces
fred/model.hosted-db2  consume  http:db2, http:log

`[1:],
	)
}

func (s *findSuite) assertFind(c *gc.C, args []string, expected string) {
	context, err := s.runFind(c, args...)
	c.Assert(err, jc.ErrorIsNil)

	obtained := cmdtesting.Stdout(context)
	c.Assert(obtained, gc.Matches, expected)
}

func (s *findSuite) assertFindError(c *gc.C, args []string, expected string) {
	_, err := s.runFind(c, args...)
	c.Assert(err, gc.ErrorMatches, expected)
}

type mockFindAPI struct {
	c                 *gc.C
	msg, offerName    string
	expectedModelName string
	expectedFilter    *jujucrossmodel.ApplicationOfferFilter
	results           []params.ApplicationOffer
}

func (s mockFindAPI) Close() error {
	return nil
}

func (s mockFindAPI) FindApplicationOffers(filters ...jujucrossmodel.ApplicationOfferFilter) ([]params.ApplicationOffer, error) {
	if s.msg != "" {
		return nil, errors.New(s.msg)
	}
	if s.expectedFilter != nil {
		s.c.Assert(filters, gc.HasLen, 1)
		s.c.Assert(filters[0], jc.DeepEquals, *s.expectedFilter)
	}

	if s.results != nil {
		return s.results, nil
	}
	return []params.ApplicationOffer{{
		OfferURL:  fmt.Sprintf("fred/%s.%s", s.expectedModelName, s.offerName),
		OfferName: s.offerName,
		Endpoints: []params.RemoteEndpoint{
			{Name: "log", Interface: "http", Role: charm.RoleProvider},
			{Name: "db2", Interface: "http", Role: charm.RoleRequirer},
		},
		Access: "consume",
	}}, nil
}
