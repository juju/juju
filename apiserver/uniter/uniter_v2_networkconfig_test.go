// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/apiserver/uniter"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	jujuFactory "github.com/juju/juju/testing/factory"
)

//TODO run all common V0 and V1 tests.
type uniterV2NetworkConfigSuite struct {
	uniterBaseSuite

	uniter *uniter.UniterAPIV2
}

var _ = gc.Suite(&uniterV2NetworkConfigSuite{})

func (s *uniterV2NetworkConfigSuite) SetUpTest(c *gc.C) {
	s.uniterBaseSuite.JujuConnSuite.SetUpTest(c)

	var err error
	s.machine0, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddSpace("internal", "internal", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("admin", "admin", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	providerAddresses := []network.Address{
		network.NewAddressOnSpace("admin", "8.8.8.8"),
		network.NewAddressOnSpace("", "8.8.4.4"),
		network.NewAddressOnSpace("internal", "10.0.0.1"),
		network.NewAddressOnSpace("internal", "10.0.0.2"),
		network.NewAddressOnSpace("admin", "fc00::1"),
	}

	err = s.machine0.SetProviderAddresses(providerAddresses...)
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine0.SetInstanceInfo("i-am", "fake_nonce", nil, nil, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	factory := jujuFactory.NewFactory(s.State)
	s.wpCharm = factory.MakeCharm(c, &jujuFactory.CharmParams{
		Name: "wordpress",
		URL:  "cs:quantal/wordpress-3",
	})
	s.wordpress, err = s.State.AddService(state.AddServiceArgs{
		Name:  "wordpress",
		Charm: s.wpCharm,
		Owner: s.AdminUserTag(c).String(),
		EndpointBindings: map[string]string{
			"db": "internal",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.wordpressUnit = factory.MakeUnit(c, &jujuFactory.UnitParams{
		Service: s.wordpress,
		Machine: s.machine0,
	})

	s.machine1 = factory.MakeMachine(c, &jujuFactory.MachineParams{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	})
	mysqlCharm := factory.MakeCharm(c, &jujuFactory.CharmParams{
		Name: "mysql",
	})
	s.mysql = factory.MakeService(c, &jujuFactory.ServiceParams{
		Name:    "mysql",
		Charm:   mysqlCharm,
		Creator: s.AdminUserTag(c),
	})
	s.wordpressUnit = factory.MakeUnit(c, &jujuFactory.UnitParams{
		Service: s.wordpress,
		Machine: s.machine0,
	})
	s.mysqlUnit = factory.MakeUnit(c, &jujuFactory.UnitParams{
		Service: s.mysql,
		Machine: s.machine1,
	})

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming unit 0 has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.wordpressUnit.Tag(),
	}

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	uniterAPIV2, err := uniter.NewUniterAPIV2(
		s.State,
		s.resources,
		s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.uniter = uniterAPIV2
}

func (s *uniterV2NetworkConfigSuite) TestNetworkConfig(c *gc.C) {

	// Add a relation between wordpress and mysql and enter scope with
	// mysqlUnit.
	rel := s.addRelation(c, "wordpress", "mysql")
	wpRelUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = wpRelUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, wpRelUnit, true)

	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-42", Unit: "unit-foo-0"},
		{Relation: rel.Tag().String(), Unit: s.wordpressUnit.Tag().String()},
		{Relation: rel.Tag().String(), Unit: "unit-mysql-0"},
		{Relation: "relation-42", Unit: s.wordpressUnit.Tag().String()},
	}}

	// For the relation "wordpress:db mysql:server" we expect to see only
	// addresses bound to the "internal" space, where the "db" endpoint itself
	// is bound to.
	expectedConfig := []params.NetworkConfig{{
		Address: "10.0.0.1",
	}, {
		Address: "10.0.0.2",
	}}

	result, err := s.uniter.NetworkConfig(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.UnitNetworkConfigResults{
		Results: []params.UnitNetworkConfigResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Config: expectedConfig},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ServerError(`"relation-42" is not a valid relation tag`)},
		},
	})
}
