// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"fmt"
	"strings"

	"github.com/juju/charm/v7"
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/crossmodel"
	model "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/relation"
)

type ListSuite struct {
	BaseCrossModelSuite

	mockAPI *mockListAPI

	applications []*model.ApplicationOfferDetails
	endpoints    []charm.Relation
}

var _ = gc.Suite(&ListSuite{})

func (s *ListSuite) SetUpTest(c *gc.C) {
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

func (s *ListSuite) TestListNoCurrentModel(c *gc.C) {
	s.store.Models["test-master"].CurrentModel = ""
	_, err := s.runList(c, nil)
	c.Assert(err, gc.ErrorMatches, `current model for controller test-master not found`)
}

func (s *ListSuite) TestListError(c *gc.C) {
	msg := "fail api"

	s.mockAPI.list = func(filters ...model.ApplicationOfferFilter) ([]*model.ApplicationOfferDetails, error) {
		return nil, errors.New(msg)
	}

	_, err := s.runList(c, nil)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
}

func (s *ListSuite) TestListFilterArgs(c *gc.C) {
	_, err := s.runList(c, []string{
		"--interface", "mysql", "--application", "mysql-lite", "--connected-user", "user", "--allowed-consumer", "consumer"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mockAPI.filters, gc.HasLen, 1)
	c.Assert(s.mockAPI.filters[0], jc.DeepEquals, model.ApplicationOfferFilter{
		OwnerName:       "fred",
		ModelName:       "test",
		ApplicationName: "mysql-lite",
		Endpoints: []model.EndpointFilterTerm{{
			Interface: "mysql",
		}},
		ConnectedUsers:   []string{"user"},
		AllowedConsumers: []string{"consumer"},
	})
}

func (s *ListSuite) TestListOfferArg(c *gc.C) {
	_, err := s.runList(c, []string{"mysql-lite"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mockAPI.filters, gc.HasLen, 1)
	c.Assert(s.mockAPI.filters[0], jc.DeepEquals, model.ApplicationOfferFilter{
		OwnerName: "fred",
		ModelName: "test",
		OfferName: "^mysql-lite$",
	})
}

func (s *ListSuite) TestListFormatError(c *gc.C) {
	s.applications = append(s.applications, s.createOfferItem("zdi^%", "different_store", nil))

	_, err := s.runList(c, nil)
	c.Assert(err, gc.ErrorMatches, ".*failed to format.*")
}

func (s *ListSuite) TestListSummary(c *gc.C) {
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
Offer       Application     Charm     Connected  Store           URL                                  Endpoint  Interface  Role
adiff-db2   app-adiff-db2   cs:db2-5  0/2        vendor          vendor:fred/model.adiff-db2          log       http       provider
                                                                                                      mysql     db2        requirer
hosted-db2  app-hosted-db2  cs:db2-5  0/0        myctrl          myctrl:fred/model.hosted-db2         log       http       provider
                                                                                                      mysql     db2        requirer
zdiff-db2   app-zdiff-db2   cs:db2-5  1/3        differentstore  differentstore:fred/model.zdiff-db2  log       http       provider
                                                                                                      mysql     db2        requirer

`[1:],
		"",
	)
}

func (s *ListSuite) TestListTabularNoConnections(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"--format", "tabular"},
		`
Offer       User  Relation id  Status  Endpoint  Interface  Role  Ingress subnets
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

func (s *ListSuite) TestListTabular(c *gc.C) {
	s.setupListTabular()
	s.assertValidList(
		c,
		[]string{"--format", "tabular"},
		`
Offer       User  Relation id  Status  Endpoint  Interface  Role      Ingress subnets
adiff-db2   mary  3            joined  db        db2        provider  
hosted-db2  -                                                         
zdiff-db2   fred  1            joined  server    mysql      provider  
            mary  1            joined  server    mysql      provider  192.168.0.1/32,10.0.0.0/8
            mary  2            joined  db        db2        provider  

`[1:],
		"",
	)
}

func (s *ListSuite) TestListTabularActiveOnly(c *gc.C) {
	s.setupListTabular()
	s.assertValidList(
		c,
		[]string{"--format", "tabular", "--active-only"},
		`
Offer      User  Relation id  Status  Endpoint  Interface  Role      Ingress subnets
adiff-db2  mary  3            joined  db        db2        provider  
zdiff-db2  fred  1            joined  server    mysql      provider  
           mary  1            joined  server    mysql      provider  192.168.0.1/32,10.0.0.0/8
           mary  2            joined  db        db2        provider  

`[1:],
		"",
	)
}

func (s *ListSuite) TestListYAML(c *gc.C) {
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
  charm: cs:db2-5
  offer-url: myctrl:fred/model.hosted-db2
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
		OfferURL:        fmt.Sprintf("%s:%s.%s", store, "fred/model", name),
		CharmURL:        "cs:db2-5",
		Endpoints:       s.endpoints,
		Connections:     connections,
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
	filters []model.ApplicationOfferFilter
	list    func(filters ...model.ApplicationOfferFilter) ([]*model.ApplicationOfferDetails, error)
}

func (s mockListAPI) Close() error {
	return nil
}

func (s mockListAPI) ListOffers(filters ...model.ApplicationOfferFilter) ([]*model.ApplicationOfferDetails, error) {
	return s.list(filters...)
}
