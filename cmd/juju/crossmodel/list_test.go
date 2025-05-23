// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/modelcmd"
	model "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/jujuclient"
)

func newListEndpointsCommandForTest(store jujuclient.ClientStore, api ListAPI) cmd.Command {
	aCmd := &listCommand{
		newAPIFunc: func(ctx context.Context) (ListAPI, error) {
			return api, nil
		},
		refreshModels: func(context.Context, jujuclient.ClientStore, string) error {
			return nil
		},
	}
	aCmd.SetClientStore(store)
	return modelcmd.Wrap(aCmd)
}

type ListSuite struct {
	BaseCrossModelSuite

	mockAPI *mockListAPI

	applications []*model.ApplicationOfferDetails
	endpoints    []charm.Relation
}

func TestListSuite(t *testing.T) {
	tc.Run(t, &ListSuite{})
}

func (s *ListSuite) SetUpTest(c *tc.C) {
	s.BaseCrossModelSuite.SetUpTest(c)

	s.endpoints = []charm.Relation{
		{Name: "mysql", Interface: "db2", Role: charm.RoleRequirer},
		{Name: "log", Interface: "http", Role: charm.RoleProvider},
	}

	s.applications = []*model.ApplicationOfferDetails{
		s.createOfferItem("hosted-db2", "myctrl", nil),
	}

	s.mockAPI = &mockListAPI{
		list: func(filters ...model.ApplicationOfferFilter) ([]*model.ApplicationOfferDetails, error) {
			s.mockAPI.filters = filters
			return s.applications, nil
		},
	}
}

func (s *ListSuite) TestListNoCurrentModel(c *tc.C) {
	s.store.Models["test-master"].CurrentModel = ""
	_, err := s.runList(c, nil)
	c.Assert(err, tc.ErrorMatches, `current model for controller test-master not found`)
}

func (s *ListSuite) TestListError(c *tc.C) {
	msg := "fail api"

	s.mockAPI.list = func(filters ...model.ApplicationOfferFilter) ([]*model.ApplicationOfferDetails, error) {
		return nil, errors.New(msg)
	}

	_, err := s.runList(c, nil)
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
}

func (s *ListSuite) TestListFilterArgs(c *tc.C) {
	_, err := s.runList(c, []string{
		"--interface", "mysql", "--application", "mysql-lite", "--connected-user", "user", "--allowed-consumer", "consumer"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mockAPI.filters, tc.HasLen, 1)
	c.Assert(s.mockAPI.filters[0], tc.DeepEquals, model.ApplicationOfferFilter{
		ModelNamespace:  "fred",
		ModelName:       "test",
		ApplicationName: "mysql-lite",
		Endpoints: []model.EndpointFilterTerm{{
			Interface: "mysql",
		}},
		ConnectedUsers:   []string{"user"},
		AllowedConsumers: []string{"consumer"},
	})
}

func (s *ListSuite) TestListOfferArg(c *tc.C) {
	_, err := s.runList(c, []string{"mysql-lite"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mockAPI.filters, tc.HasLen, 1)
	c.Assert(s.mockAPI.filters[0], tc.DeepEquals, model.ApplicationOfferFilter{
		ModelNamespace: "fred",
		ModelName:      "test",
		OfferName:      "^mysql-lite$",
	})
}

func (s *ListSuite) TestListFormatError(c *tc.C) {
	s.applications = append(s.applications, s.createOfferItem("zdi^%", "different_store", nil))

	_, err := s.runList(c, nil)
	c.Assert(err, tc.ErrorMatches, ".*failed to format.*")
}

func (s *ListSuite) TestListSummary(c *tc.C) {
	// For summary output, we don't care about the content, just the count.
	conns1 := []model.OfferConnection{{Status: relation.Joined}, {}, {}}
	conns2 := []model.OfferConnection{{}, {}}
	// Insert in random order to check sorting.
	s.applications = append(s.applications, s.createOfferItem("zdiff-db2", "differentstore", conns1))
	s.applications = append(s.applications, s.createOfferItem("adiff-db2", "vendor", conns2))

	s.assertValidList(
		c,
		[]string{"--format", "summary"},
		`
Offer       Application     Charm     Connected  Store           URL                                           Endpoint  Interface  Role
adiff-db2   app-adiff-db2   ch:db2-5  0/2        vendor          vendor:fred@external/model.adiff-db2          log       http       provider
                                                                                                               mysql     db2        requirer
hosted-db2  app-hosted-db2  ch:db2-5  0/0        myctrl          myctrl:fred@external/model.hosted-db2         log       http       provider
                                                                                                               mysql     db2        requirer
zdiff-db2   app-zdiff-db2   ch:db2-5  1/3        differentstore  differentstore:fred@external/model.zdiff-db2  log       http       provider
                                                                                                               mysql     db2        requirer
`[1:],
		"",
	)
}

func (s *ListSuite) TestListTabularNoConnections(c *tc.C) {
	s.assertValidList(
		c,
		[]string{"--format", "tabular"},
		`
Offer       User  Relation ID  Status  Endpoint  Interface  Role  Ingress subnets
hosted-db2  -                                                     
`[1:],
		"",
	)
}

func (s *ListSuite) setupListTabular() {
	// For summary output, we don't care about the content, just the count.
	conns1 := []model.OfferConnection{
		{
			SourceModelUUID: "model-uuid1",
			Username:        "mary",
			RelationId:      2,
			Endpoint:        "db",
			Status:          "joined",
		}, {
			SourceModelUUID: "model-uuid2",
			Username:        "fred",
			RelationId:      1,
			Endpoint:        "server",
			Status:          "joined",
		}, {
			SourceModelUUID: "model-uuid3",
			Username:        "mary",
			RelationId:      1,
			Endpoint:        "server",
			Status:          "joined",
			IngressSubnets:  []string{"192.168.0.1/32", "10.0.0.0/8"},
		},
	}
	conns2 := []model.OfferConnection{
		{
			SourceModelUUID: "model-uuid3",
			Username:        "mary",
			RelationId:      3,
			Endpoint:        "db",
			Status:          "joined",
		},
	}
	// Insert in random order to check sorting.
	s.applications = append(s.applications, s.createOfferItem("zdiff-db2", "differentstore", conns1))
	s.applications = append(s.applications, s.createOfferItem("adiff-db2", "vendor", conns2))
	s.applications[1].Endpoints = []charm.Relation{
		{Name: "db", Interface: "db2", Role: charm.RoleProvider},
		{Name: "server", Interface: "mysql", Role: charm.RoleProvider},
	}
	s.applications[2].Endpoints = []charm.Relation{
		{Name: "db", Interface: "db2", Role: charm.RoleProvider},
	}
}

func (s *ListSuite) TestListTabular(c *tc.C) {
	s.setupListTabular()
	s.assertValidList(
		c,
		[]string{"--format", "tabular"},
		`
Offer       User  Relation ID  Status  Endpoint  Interface  Role      Ingress subnets
adiff-db2   mary  3            joined  db        db2        provider  
hosted-db2  -                                                         
zdiff-db2   fred  1            joined  server    mysql      provider  
            mary  1            joined  server    mysql      provider  192.168.0.1/32,10.0.0.0/8
            mary  2            joined  db        db2        provider  
`[1:],
		"",
	)
}

func (s *ListSuite) TestListTabularActiveOnly(c *tc.C) {
	s.setupListTabular()
	s.assertValidList(
		c,
		[]string{"--format", "tabular", "--active-only"},
		`
Offer      User  Relation ID  Status  Endpoint  Interface  Role      Ingress subnets
adiff-db2  mary  3            joined  db        db2        provider  
zdiff-db2  fred  1            joined  server    mysql      provider  
           mary  1            joined  server    mysql      provider  192.168.0.1/32,10.0.0.0/8
           mary  2            joined  db        db2        provider  
`[1:],
		"",
	)
}

func (s *ListSuite) TestListYAML(c *tc.C) {
	// Since applications are in the map and ordering is unreliable, ensure that there is only one endpoint.
	// We only need one to demonstrate display anyway :D
	s.applications[0].Endpoints = []charm.Relation{{Name: "mysql", Interface: "db2", Role: charm.RoleRequirer}}
	s.applications[0].Connections = []model.OfferConnection{
		{
			SourceModelUUID: "model-uuid",
			Username:        "mary",
			Status:          "joined",
			Endpoint:        "db",
		},
		{
			SourceModelUUID: "another-model-uuid",
			Username:        "fred",
			Status:          "error",
			Message:         "firewall issue",
			RelationId:      2,
			Endpoint:        "http",
			IngressSubnets:  []string{"192.168.0.1/32", "10.0.0.0/8"},
		},
	}
	s.applications[0].Users = []model.OfferUserDetails{{
		UserName: "fred", DisplayName: "Fred", Access: "consume",
	}}

	s.assertValidList(
		c,
		[]string{"--format", "yaml"},
		`
hosted-db2:
  application: app-hosted-db2
  store: myctrl
  charm: ch:db2-5
  offer-url: myctrl:fred@external/model.hosted-db2
  endpoints:
    mysql:
      interface: db2
      role: requirer
  connections:
  - source-model-uuid: model-uuid
    username: mary
    relation-id: 0
    endpoint: db
    status:
      current: joined
  - source-model-uuid: another-model-uuid
    username: fred
    relation-id: 2
    endpoint: http
    status:
      current: error
      message: firewall issue
    ingress-subnets:
    - 192.168.0.1/32
    - 10.0.0.0/8
  users:
    fred:
      display-name: Fred
      access: consume
`[1:],
		"",
	)
}

func (s *ListSuite) createOfferItem(name, store string, connections []model.OfferConnection) *model.ApplicationOfferDetails {
	return &model.ApplicationOfferDetails{
		ApplicationName: "app-" + name,
		OfferName:       name,
		OfferURL:        fmt.Sprintf("%s:%s.%s", store, "fred@external/model", name),
		CharmURL:        "ch:db2-5",
		Endpoints:       s.endpoints,
		Connections:     connections,
	}
}

func (s *ListSuite) runList(c *tc.C, args []string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, newListEndpointsCommandForTest(s.store, s.mockAPI), args...)
}

func (s *ListSuite) assertValidList(c *tc.C, args []string, expectedValid, expectedErr string) {
	context, err := s.runList(c, args)
	c.Assert(err, tc.ErrorIsNil)

	obtainedErr := strings.Replace(cmdtesting.Stderr(context), "\n", "", -1)
	c.Assert(obtainedErr, tc.Matches, expectedErr)

	obtainedValid := cmdtesting.Stdout(context)
	c.Assert(obtainedValid, tc.Matches, expectedValid)
}

type mockListAPI struct {
	filters []model.ApplicationOfferFilter
	list    func(filters ...model.ApplicationOfferFilter) ([]*model.ApplicationOfferDetails, error)
}

func (s mockListAPI) Close() error {
	return nil
}

func (s mockListAPI) ListOffers(ctx context.Context, filters ...model.ApplicationOfferFilter) ([]*model.ApplicationOfferDetails, error) {
	return s.list(filters...)
}
