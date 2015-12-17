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

	networks := []state.NetworkInfo{{
		Name:       "net1",
		ProviderId: "net1",
		CIDR:       "0.1.2.0/24",
		VLANTag:    0,
	}, {
		Name:       "vlan42",
		ProviderId: "vlan42",
		CIDR:       "0.2.2.0/24",
		VLANTag:    42,
	}, {
		Name:       "vlan69",
		ProviderId: "vlan69",
		CIDR:       "0.3.2.0/24",
		VLANTag:    69,
	}, {
		Name:       "vlan123",
		ProviderId: "vlan123",
		CIDR:       "0.4.2.0/24",
		VLANTag:    123,
	}, {
		Name:       "net2",
		ProviderId: "net2",
		CIDR:       "0.5.2.0/24",
		VLANTag:    0,
	}}

	machineIfaces := []state.NetworkInterfaceInfo{{
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		InterfaceName: "eth0",
		NetworkName:   "net1",
		IsVirtual:     false,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		InterfaceName: "eth1",
		NetworkName:   "net1",
		IsVirtual:     false,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		InterfaceName: "eth1.42",
		NetworkName:   "vlan42",
		IsVirtual:     true,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		InterfaceName: "eth0.69",
		NetworkName:   "vlan69",
		IsVirtual:     true,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f2",
		InterfaceName: "eth2",
		NetworkName:   "net2",
		IsVirtual:     false,
		Disabled:      true,
	}}

	err = s.machine0.SetInstanceInfo("i-am", "fake_nonce", nil, networks, machineIfaces, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	factory := jujuFactory.NewFactory(s.State)
	s.wpCharm = factory.MakeCharm(c, &jujuFactory.CharmParams{
		Name: "wordpress",
		URL:  "cs:quantal/wordpress-3",
	})
	s.wordpress = factory.MakeService(c, &jujuFactory.ServiceParams{
		Name:    "wordpress",
		Charm:   s.wpCharm,
		Creator: s.AdminUserTag(c),
	})
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

	expectedConfig := []params.NetworkConfig{
		{
			MACAddress:    "aa:bb:cc:dd:ee:f0",
			NetworkName:   "net1",
			InterfaceName: "eth0",
			Disabled:      false,
		}, {
			MACAddress:    "aa:bb:cc:dd:ee:f1",
			NetworkName:   "net1",
			InterfaceName: "eth1",
			Disabled:      false,
		}, {
			MACAddress:    "aa:bb:cc:dd:ee:f1",
			NetworkName:   "vlan42",
			InterfaceName: "eth1",
			Disabled:      false,
		}, {
			MACAddress:    "aa:bb:cc:dd:ee:f0",
			NetworkName:   "vlan69",
			InterfaceName: "eth0",
			Disabled:      false,
		}, {
			MACAddress:    "aa:bb:cc:dd:ee:f2",
			NetworkName:   "net2",
			InterfaceName: "eth2",
			Disabled:      true,
		},
	}

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
