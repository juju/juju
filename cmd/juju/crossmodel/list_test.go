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

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/crossmodel"
	"github.com/juju/juju/testing"
)

type ListSuite struct {
	BaseCrossModelSuite

	mockAPI *mockListAPI

	services  map[string][]params.ListEndpointsServiceItemResult
	endpoints []params.RemoteEndpoint
}

var _ = gc.Suite(&ListSuite{})

func (s *ListSuite) SetUpTest(c *gc.C) {
	s.BaseCrossModelSuite.SetUpTest(c)

	s.endpoints = []params.RemoteEndpoint{
		params.RemoteEndpoint{Name: "mysql", Interface: "db2", Role: charm.RoleRequirer},
		params.RemoteEndpoint{Name: "log", Interface: "http", Role: charm.RoleProvider},
	}

	s.services = map[string][]params.ListEndpointsServiceItemResult{
		"LOCAL": []params.ListEndpointsServiceItemResult{{Result: s.createServiceItem("local", 0)}},
	}

	s.mockAPI = &mockListAPI{
		list: func(filters map[string][]string) (map[string][]params.ListEndpointsServiceItemResult, error) {
			return s.services, nil
		},
	}
}

func (s *ListSuite) TestListError(c *gc.C) {
	msg := "fail api"

	s.mockAPI.list = func(filters map[string][]string) (map[string][]params.ListEndpointsServiceItemResult, error) {
		return nil, errors.New(msg)
	}

	_, err := s.runList(c, nil)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
}

func (s *ListSuite) TestListDirectories(c *gc.C) {
	unorderedOne := s.createServiceItem("different_store", 33)
	unorderedOne.ApplicationName = "aaafred/prod/hosted-db2"
	s.services["VENDOR"] = []params.ListEndpointsServiceItemResult{
		{Result: s.createServiceItem("vendor", 23)},
		{Result: unorderedOne},
	}

	s.assertValidList(
		c,
		nil,
		// Default format is tabular
		`
LOCAL
APPLICATION           CHARM  CONNECTED  STORE  URL                     ENDPOINT  INTERFACE  ROLE
fred/prod/hosted-db2  db2    0          local  u/fred/prod/hosted-db2  log       http       provider
                                                                       mysql     db2        requirer
VENDOR
APPLICATION              CHARM  CONNECTED  STORE            URL                     ENDPOINT  INTERFACE  ROLE
aaafred/prod/hosted-db2  db2    33         different_store  u/fred/prod/hosted-db2  log       http       provider
                                                                                    mysql     db2        requirer
fred/prod/hosted-db2     db2    23         vendor           u/fred/prod/hosted-db2  log       http       provider
                                                                                    mysql     db2        requirer

`[1:],
		"",
	)
}

func (s *ListSuite) TestListWithErrors(c *gc.C) {
	s.services["test directory"] = []params.ListEndpointsServiceItemResult{{
		Error: &params.Error{
			Message: "here is the error",
		}},
	}

	s.assertValidList(
		c,
		nil,
		`
LOCAL
APPLICATION           CHARM  CONNECTED  STORE  URL                     ENDPOINT  INTERFACE  ROLE
fred/prod/hosted-db2  db2    0          local  u/fred/prod/hosted-db2  log       http       provider
                                                                       mysql     db2        requirer

`[1:],
		"here is the error",
	)
}

func (s *ListSuite) TestListYAML(c *gc.C) {
	// Since services are in the map and ordering is unreliable, ensure that there is only one endpoint.
	// We only need one to demonstrate display anyway :D
	s.services["LOCAL"][0].Result.Endpoints = []params.RemoteEndpoint{{Name: "mysql", Interface: "db2", Role: charm.RoleRequirer}}

	s.assertValidList(
		c,
		[]string{"--format", "yaml"},
		`
LOCAL:
  fred/prod/hosted-db2:
    charm: db2
    connected: 0
    store: local
    url: u/fred/prod/hosted-db2
    endpoints:
      mysql:
        interface: db2
        role: requirer
`[1:],
		"",
	)
}

func (s *ListSuite) createServiceItem(store string, count int) *params.ListEndpointsServiceItem {
	return &params.ListEndpointsServiceItem{
		ApplicationName: "fred/prod/hosted-db2",
		CharmName:       "db2",
		Store:           store,
		Location:        "u/fred/prod/hosted-db2",
		Endpoints:       s.endpoints,
		UsersCount:      count,
	}
}

func (s *ListSuite) runList(c *gc.C, args []string) (*cmd.Context, error) {
	return testing.RunCommand(c, crossmodel.NewListEndpointsCommandForTest(s.mockAPI), args...)
}

func (s *ListSuite) assertValidList(c *gc.C, args []string, expectedValid, expectedErr string) {
	context, err := s.runList(c, args)
	c.Assert(err, jc.ErrorIsNil)

	obtainedErr := strings.Replace(testing.Stderr(context), "\n", "", -1)
	c.Assert(obtainedErr, gc.Matches, expectedErr)

	obtainedValid := testing.Stdout(context)
	c.Assert(obtainedValid, gc.Matches, expectedValid)
}

type mockListAPI struct {
	list func(filters map[string][]string) (map[string][]params.ListEndpointsServiceItemResult, error)
}

func (s mockListAPI) Close() error {
	return nil
}

func (s mockListAPI) List(filters map[string][]string) (map[string][]params.ListEndpointsServiceItemResult, error) {
	return s.list(filters)
}
