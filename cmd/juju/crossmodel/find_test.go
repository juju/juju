// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"fmt"

	"github.com/juju/charm/v7"
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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
Store   URL                   Access   Interfaces
master  fred/test.hosted-db2  consume  http:db2, http:log

`[1:],
	)
}

func (s *findSuite) TestFindDuplicateUrl(c *gc.C) {
	s.assertFindError(c, []string{"url", "--url", "urlparam"}, ".*URL term cannot be specified twice.*")
}

func (s *findSuite) TestFindOfferandUrl(c *gc.C) {
	s.assertFindError(c, []string{"--offer", "offer", "--url", "urlparam"}, ".*cannot specify both a URL term and offer term.*")
}

func (s *findSuite) TestNoResults(c *gc.C) {
	s.mockAPI.c = c
	s.mockAPI.expectedModelName = "none"
	s.mockAPI.expectedFilter = &jujucrossmodel.ApplicationOfferFilter{
		OwnerName: "bob",
		ModelName: "none",
		Endpoints: []jujucrossmodel.EndpointFilterTerm{{
			Interface: "mysql",
		}},
	}
	s.mockAPI.results = []*jujucrossmodel.ApplicationOfferDetails{}
	s.assertFindError(
		c,
		[]string{"--url", "none", "--interface", "mysql"},
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
Store   URL                    Access   Interfaces
master  fred/model.hosted-db2  consume  http:db2, http:log

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
		}},
	}
	s.mockAPI.expectedModelName = "model"
	s.assertFind(
		c,
		[]string{"--format", "tabular", "--url", "fred/model", "--interface", "mysql"},
		`
Store   URL                    Access   Interfaces
master  fred/model.hosted-db2  consume  http:db2, http:log

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
master:fred/model.hosted-db2:
  access: consume
  endpoints:
    db2:
      interface: http
      role: requirer
    log:
      interface: http
      role: provider
  users:
    bob:
      display-name: Bob
      access: consume
`[1:],
	)
}

func (s *findSuite) TestFindTabular(c *gc.C) {
	s.mockAPI.expectedModelName = "model"
	s.assertFind(
		c,
		[]string{"fred/model.hosted-db2", "--format", "tabular"},
		`
Store   URL                    Access   Interfaces
master  fred/model.hosted-db2  consume  http:db2, http:log

`[1:],
	)
}

func (s *findSuite) TestFindDifferentController(c *gc.C) {
	s.mockAPI.expectedModelName = "model"
	s.mockAPI.controllerName = "different"
	s.assertFind(
		c,
		[]string{"fred/model.hosted-db2", "--format", "tabular"},
		`
Store      URL                    Access   Interfaces
different  fred/model.hosted-db2  consume  http:db2, http:log

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
	controllerName    string
	msg, offerName    string
	expectedModelName string
	expectedFilter    *jujucrossmodel.ApplicationOfferFilter
	results           []*jujucrossmodel.ApplicationOfferDetails
}

func (s mockFindAPI) Close() error {
	return nil
}

func (s mockFindAPI) FindApplicationOffers(filters ...jujucrossmodel.ApplicationOfferFilter) ([]*jujucrossmodel.ApplicationOfferDetails, error) {
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
	store := s.controllerName
	if store == "" {
		store = "master"
	}
	offerURL := fmt.Sprintf("%s:fred/%s.%s", store, s.expectedModelName, s.offerName)
	return []*jujucrossmodel.ApplicationOfferDetails{{
		OfferURL:  offerURL,
		OfferName: s.offerName,
		Endpoints: []charm.Relation{
			{Name: "log", Interface: "http", Role: charm.RoleProvider},
			{Name: "db2", Interface: "http", Role: charm.RoleRequirer},
		},
		Users: []jujucrossmodel.OfferUserDetails{{
			UserName: "bob", DisplayName: "Bob", Access: "consume",
		}},
	}}, nil
}
