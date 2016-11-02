// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/crossmodel"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/testing"
)

type findSuite struct {
	BaseCrossModelSuite
	mockAPI *mockFindAPI
}

var _ = gc.Suite(&findSuite{})

func (s *findSuite) SetUpTest(c *gc.C) {
	s.BaseCrossModelSuite.SetUpTest(c)

	s.mockAPI = &mockFindAPI{
		serviceName: "hosted-db2",
	}
}

func (s *findSuite) runFind(c *gc.C, args ...string) (*cmd.Context, error) {
	return testing.RunCommand(c, crossmodel.NewFindEndpointsCommandForTest(s.store, s.mockAPI), args...)
}

func (s *findSuite) TestFindNoArgs(c *gc.C) {
	s.mockAPI.c = c
	s.mockAPI.expectedFilter = &jujucrossmodel.ApplicationOfferFilter{
		ApplicationOffer: jujucrossmodel.ApplicationOffer{
			ApplicationURL: "local:",
		},
	}
	s.assertFind(
		c,
		[]string{},
		`
URL                       Interfaces
local:/u/fred/hosted-db2  http:db2, http:log

`[1:],
	)
}

func (s *findSuite) TestFindDuplicateUrl(c *gc.C) {
	s.assertFindError(c, []string{"url", "--url", "urlparam"}, ".*URL term cannot be specified twice.*")
}

func (s *findSuite) TestNoResults(c *gc.C) {
	s.mockAPI.c = c
	s.mockAPI.expectedFilter = &jujucrossmodel.ApplicationOfferFilter{
		ApplicationOffer: jujucrossmodel.ApplicationOffer{
			ApplicationURL: "local:/u/none",
		},
	}
	s.mockAPI.results = []params.ApplicationOffer{}
	s.assertFindError(
		c,
		[]string{"--url", "local:/u/none"},
		`no matching application offers found`,
	)
}

func (s *findSuite) TestSimpleFilter(c *gc.C) {
	s.mockAPI.c = c
	s.mockAPI.expectedFilter = &jujucrossmodel.ApplicationOfferFilter{
		ApplicationOffer: jujucrossmodel.ApplicationOffer{
			ApplicationURL: "local:/u/fred",
		},
	}
	s.assertFind(
		c,
		[]string{"--format", "tabular", "--url", "local:/u/fred"},
		`
URL                       Interfaces
local:/u/fred/hosted-db2  http:db2, http:log

`[1:],
	)
}

func (s *findSuite) TestEndpointFilter(c *gc.C) {
	s.mockAPI.c = c
	s.mockAPI.expectedFilter = &jujucrossmodel.ApplicationOfferFilter{
		ApplicationOffer: jujucrossmodel.ApplicationOffer{
			ApplicationURL: "local:/u/fred",
			Endpoints: []charm.Relation{{
				Interface: "mysql",
				Name:      "db",
			}},
		},
	}
	s.assertFind(
		c,
		[]string{"--format", "tabular", "--url", "local:/u/fred", "--endpoint", "db", "--interface", "mysql"},
		`
URL                       Interfaces
local:/u/fred/hosted-db2  http:db2, http:log

`[1:],
	)
}

func (s *findSuite) TestFindApiError(c *gc.C) {
	s.mockAPI.msg = "fail"
	s.assertFindError(c, []string{"local:/u/fred/db2"}, ".*fail.*")
}

func (s *findSuite) TestFindYaml(c *gc.C) {
	s.assertFind(
		c,
		[]string{"local:/u/fred/db2", "--format", "yaml"},
		`
local:/u/fred/hosted-db2:
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
	s.assertFind(
		c,
		[]string{"local:/u/fred/db2", "--format", "tabular"},
		`
URL                       Interfaces
local:/u/fred/hosted-db2  http:db2, http:log

`[1:],
	)
}

func (s *findSuite) assertFind(c *gc.C, args []string, expected string) {
	context, err := s.runFind(c, args...)
	c.Assert(err, jc.ErrorIsNil)

	obtained := testing.Stdout(context)
	c.Assert(obtained, gc.Matches, expected)
}

func (s *findSuite) assertFindError(c *gc.C, args []string, expected string) {
	_, err := s.runFind(c, args...)
	c.Assert(err, gc.ErrorMatches, expected)
}

type mockFindAPI struct {
	c                *gc.C
	msg, serviceName string
	expectedFilter   *jujucrossmodel.ApplicationOfferFilter
	results          []params.ApplicationOffer
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
		ApplicationURL:  fmt.Sprintf("local:/u/fred/%s", s.serviceName),
		ApplicationName: s.serviceName,
		Endpoints: []params.RemoteEndpoint{
			{Name: "log", Interface: "http", Role: charm.RoleProvider},
			{Name: "db2", Interface: "http", Role: charm.RoleRequirer},
		},
	}}, nil
}
