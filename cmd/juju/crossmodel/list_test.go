// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/cmd/cmdtesting"
	"github.com/juju/juju/cmd/juju/crossmodel"
	model "github.com/juju/juju/core/crossmodel"
)

type ListSuite struct {
	BaseCrossModelSuite

	mockAPI *mockListAPI

	applications []model.ApplicationOfferDetailsResult
	endpoints    []charm.Relation
}

var _ = gc.Suite(&ListSuite{})

func (s *ListSuite) SetUpTest(c *gc.C) {
	s.BaseCrossModelSuite.SetUpTest(c)

	s.endpoints = []charm.Relation{
		{Name: "mysql", Interface: "db2", Role: charm.RoleRequirer},
		{Name: "log", Interface: "http", Role: charm.RoleProvider},
	}

	s.applications = []model.ApplicationOfferDetailsResult{
		{Result: s.createOfferItem("hosted-db2", "myctrl", 0)},
	}

	s.mockAPI = &mockListAPI{
		list: func(filters ...model.ApplicationOfferFilter) ([]model.ApplicationOfferDetailsResult, error) {
			return s.applications, nil
		},
	}
}

func (s *ListSuite) TestListError(c *gc.C) {
	msg := "fail api"

	s.mockAPI.list = func(filters ...model.ApplicationOfferFilter) ([]model.ApplicationOfferDetailsResult, error) {
		return nil, errors.New(msg)
	}

	_, err := s.runList(c, nil)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
}

func (s *ListSuite) TestListFormatError(c *gc.C) {
	s.applications = append(s.applications, model.ApplicationOfferDetailsResult{Result: s.createOfferItem("zdi^%", "different_store", 33)})

	_, err := s.runList(c, nil)
	c.Assert(err, gc.ErrorMatches, ".*failed to format.*")
}

func (s *ListSuite) TestList(c *gc.C) {
	// Insert in random order to check sorting.
	s.applications = append(s.applications, model.ApplicationOfferDetailsResult{Result: s.createOfferItem("zdiff-db2", "differentstore", 33)})
	s.applications = append(s.applications, model.ApplicationOfferDetailsResult{Result: s.createOfferItem("adiff-db2", "vendor", 23)})

	s.assertValidList(
		c,
		nil,
		// Default format is tabular
		`
Application     Charm  Connected  Store           URL                                  Endpoint  Interface  Role
app-zdiff-db2   db2    33         differentstore  differentstore:fred/model.zdiff-db2  log       http       provider
                                                                                       mysql     db2        requirer
app-hosted-db2  db2    0          myctrl          myctrl:fred/model.hosted-db2         log       http       provider
                                                                                       mysql     db2        requirer
app-adiff-db2   db2    23         vendor          vendor:fred/model.adiff-db2          log       http       provider
                                                                                       mysql     db2        requirer

`[1:],
		"",
	)
}

func (s *ListSuite) TestListWithErrors(c *gc.C) {
	msg := "here is the error"
	s.applications = append(s.applications, model.ApplicationOfferDetailsResult{Error: errors.New(msg)})

	s.assertValidList(
		c,
		nil,
		`
Application     Charm  Connected  Store   URL                           Endpoint  Interface  Role
app-hosted-db2  db2    0          myctrl  myctrl:fred/model.hosted-db2  log       http       provider
                                                                        mysql     db2        requirer

`[1:],
		msg,
	)
}

func (s *ListSuite) TestListYAML(c *gc.C) {
	// Since applications are in the map and ordering is unreliable, ensure that there is only one endpoint.
	// We only need one to demonstrate display anyway :D
	s.applications[0].Result.Endpoints = []charm.Relation{{Name: "mysql", Interface: "db2", Role: charm.RoleRequirer}}

	s.assertValidList(
		c,
		[]string{"--format", "yaml"},
		`
hosted-db2:
  store: myctrl
  charm: db2
  url: myctrl:fred/model.hosted-db2
  endpoints:
    mysql:
      interface: db2
      role: requirer
`[1:],
		"",
	)
}

func (s *ListSuite) createOfferItem(name, store string, count int) *model.ApplicationOfferDetails {
	return &model.ApplicationOfferDetails{
		ApplicationName: "app-" + name,
		OfferName:       name,
		OfferURL:        fmt.Sprintf("%s:%s.%s", store, "fred/model", name),
		CharmName:       "db2",
		Endpoints:       s.endpoints,
		ConnectedCount:  count,
	}
}

func (s *ListSuite) runList(c *gc.C, args []string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, crossmodel.NewListEndpointsCommandForTest(s.store, s.mockAPI), args...)
}

func (s *ListSuite) assertValidList(c *gc.C, args []string, expectedValid, expectedErr string) {
	context, err := s.runList(c, args)
	c.Assert(err, jc.ErrorIsNil)

	obtainedErr := strings.Replace(cmdtesting.Stderr(context), "\n", "", -1)
	c.Assert(obtainedErr, gc.Matches, expectedErr)

	obtainedValid := cmdtesting.Stdout(context)
	c.Assert(obtainedValid, gc.Matches, expectedValid)
}

type mockListAPI struct {
	list func(filters ...model.ApplicationOfferFilter) ([]model.ApplicationOfferDetailsResult, error)
}

func (s mockListAPI) Close() error {
	return nil
}

func (s mockListAPI) ListOffers(filters ...model.ApplicationOfferFilter) ([]model.ApplicationOfferDetailsResult, error) {
	return s.list(filters...)
}
