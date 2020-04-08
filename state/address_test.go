// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing/factory"
)

type AddressSuite struct{}

var _ = gc.Suite(&AddressSuite{})

func (s *AddressSuite) TestAddressConversion(c *gc.C) {
	netAddress := network.SpaceAddress{
		MachineAddress: network.MachineAddress{
			Value: "0.0.0.0",
			Type:  network.IPv4Address,
			Scope: network.ScopeUnknown,
		},
	}
	state.AssertAddressConversion(c, netAddress)
}

func (s *AddressSuite) TestHostPortConversion(c *gc.C) {
	netAddress := network.SpaceAddress{
		MachineAddress: network.MachineAddress{
			Value: "0.0.0.0",
			Type:  network.IPv4Address,
			Scope: network.ScopeUnknown,
		},
	}
	netHostPort := network.SpaceHostPort{
		SpaceAddress: netAddress,
		NetPort:      4711,
	}
	state.AssertHostPortConversion(c, netHostPort)
}

type ControllerAddressesSuite struct {
	ConnSuite
}

var _ = gc.Suite(&ControllerAddressesSuite{})

func (s *ControllerAddressesSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	// Make sure there is a machine with manage state in existence.
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageModel, state.JobHostUnits},
		Addresses: network.SpaceAddresses{
			network.NewSpaceAddress("192.168.2.144"),
			network.NewSpaceAddress("10.0.1.2"),
		},
	})
	c.Logf("machine addresses: %#v", machine.Addresses())
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
}

func (s *ControllerAddressesSuite) TestControllerModel(c *gc.C) {
	addresses, err := s.State.Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, jc.SameContents, []string{"10.0.1.2:1234"})
}

func (s *ControllerAddressesSuite) TestOtherModel(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer func() { _ = st.Close() }()
	addresses, err := st.Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, jc.SameContents, []string{"10.0.1.2:1234"})
}

func (s *ControllerAddressesSuite) TestSetAPIHostPortsNoMgmtSpace(c *gc.C) {
	addrs, err := s.State.APIHostPortsForClients()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.HasLen, 0)

	newHostPorts := []network.SpaceHostPorts{{{
		SpaceAddress: network.NewScopedSpaceAddress("0.2.4.6", network.ScopeCloudLocal),
		NetPort:      1,
	}, {
		SpaceAddress: network.NewScopedSpaceAddress("0.4.8.16", network.ScopePublic),
		NetPort:      2,
	}}, {{
		SpaceAddress: network.NewScopedSpaceAddress("0.6.1.2", network.ScopeCloudLocal),
		NetPort:      5,
	}}}
	err = s.State.SetAPIHostPorts(newHostPorts)
	c.Assert(err, jc.ErrorIsNil)

	gotHostPorts, err := s.State.APIHostPortsForClients()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotHostPorts, jc.DeepEquals, newHostPorts)

	gotHostPorts, err = s.State.APIHostPortsForAgents()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotHostPorts, jc.DeepEquals, newHostPorts)

	newHostPorts = []network.SpaceHostPorts{{{
		SpaceAddress: network.NewScopedSpaceAddress("0.2.4.6", network.ScopeCloudLocal),
		NetPort:      13,
	}}}
	err = s.State.SetAPIHostPorts(newHostPorts)
	c.Assert(err, jc.ErrorIsNil)

	gotHostPorts, err = s.State.APIHostPortsForClients()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotHostPorts, jc.DeepEquals, newHostPorts)

	gotHostPorts, err = s.State.APIHostPortsForAgents()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotHostPorts, jc.DeepEquals, newHostPorts)
}

func (s *ControllerAddressesSuite) TestSetAPIHostPortsNoMgmtSpaceConcurrentSame(c *gc.C) {
	hostPorts := []network.SpaceHostPorts{{{
		SpaceAddress: network.NewScopedSpaceAddress("0.4.8.16", network.ScopePublic),
		NetPort:      2,
	}}, {{
		SpaceAddress: network.NewScopedSpaceAddress("0.2.4.6", network.ScopeCloudLocal),
		NetPort:      1,
	}}}

	// API host ports are concurrently changed to the same
	// desired value; second arrival will fail its assertion,
	// refresh finding nothing to do, and then issue a
	// read-only assertion that succeeds.
	ctrC := state.ControllersC
	var prevRevno int64
	var prevAgentsRevno int64
	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.State.SetAPIHostPorts(hostPorts)
		c.Assert(err, jc.ErrorIsNil)
		revno, err := state.TxnRevno(s.State, ctrC, "apiHostPorts")
		c.Assert(err, jc.ErrorIsNil)
		prevRevno = revno
		revno, err = state.TxnRevno(s.State, ctrC, "apiHostPortsForAgents")
		c.Assert(err, jc.ErrorIsNil)
		prevAgentsRevno = revno
	}).Check()

	err := s.State.SetAPIHostPorts(hostPorts)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(prevRevno, gc.Not(gc.Equals), 0)

	revno, err := state.TxnRevno(s.State, ctrC, "apiHostPorts")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(revno, gc.Equals, prevRevno)

	revno, err = state.TxnRevno(s.State, ctrC, "apiHostPortsForAgents")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(revno, gc.Equals, prevAgentsRevno)
}

func (s *ControllerAddressesSuite) TestSetAPIHostPortsNoMgmtSpaceConcurrentDifferent(c *gc.C) {
	hostPorts0 := []network.SpaceHostPort{{
		SpaceAddress: network.NewScopedSpaceAddress("0.4.8.16", network.ScopePublic),
		NetPort:      2,
	}}
	hostPorts1 := []network.SpaceHostPort{{
		SpaceAddress: network.NewScopedSpaceAddress("0.2.4.6", network.ScopeCloudLocal),
		NetPort:      1,
	}}

	// API host ports are concurrently changed to different
	// values; second arrival will fail its assertion, refresh
	// finding and reattempt.

	ctrC := state.ControllersC
	var prevRevno int64
	var prevAgentsRevno int64
	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.State.SetAPIHostPorts([]network.SpaceHostPorts{hostPorts0})
		c.Assert(err, jc.ErrorIsNil)
		revno, err := state.TxnRevno(s.State, ctrC, "apiHostPorts")
		c.Assert(err, jc.ErrorIsNil)
		prevRevno = revno
		revno, err = state.TxnRevno(s.State, ctrC, "apiHostPortsForAgents")
		c.Assert(err, jc.ErrorIsNil)
		prevAgentsRevno = revno
	}).Check()

	err := s.State.SetAPIHostPorts([]network.SpaceHostPorts{hostPorts1})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(prevRevno, gc.Not(gc.Equals), 0)

	revno, err := state.TxnRevno(s.State, ctrC, "apiHostPorts")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(revno, gc.Not(gc.Equals), prevRevno)

	revno, err = state.TxnRevno(s.State, ctrC, "apiHostPortsForAgents")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(revno, gc.Not(gc.Equals), prevAgentsRevno)

	hostPorts, err := s.State.APIHostPortsForClients()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hostPorts, gc.DeepEquals, []network.SpaceHostPorts{hostPorts1})

	hostPorts, err = s.State.APIHostPortsForAgents()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hostPorts, gc.DeepEquals, []network.SpaceHostPorts{hostPorts1})
}

func (s *ControllerAddressesSuite) TestSetAPIHostPortsWithMgmtSpace(c *gc.C) {
	sp, err := s.State.AddSpace("mgmt01", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	s.SetJujuManagementSpace(c, "mgmt01")

	addrs, err := s.State.APIHostPortsForClients()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.HasLen, 0)

	hostPort1 := network.SpaceHostPort{
		SpaceAddress: network.NewScopedSpaceAddress("0.2.4.6", network.ScopeCloudLocal),
		NetPort:      1,
	}
	hostPort2 := network.SpaceHostPort{
		SpaceAddress: network.SpaceAddress{
			MachineAddress: network.MachineAddress{
				Value: "0.4.8.16",
				Type:  network.IPv4Address,
				Scope: network.ScopePublic,
			},
			SpaceID: sp.Id(),
		},
		NetPort: 2,
	}
	hostPort3 := network.SpaceHostPort{
		SpaceAddress: network.NewScopedSpaceAddress("0.4.1.2", network.ScopeCloudLocal),
		NetPort:      5,
	}
	newHostPorts := []network.SpaceHostPorts{{hostPort1, hostPort2}, {hostPort3}}

	err = s.State.SetAPIHostPorts(newHostPorts)
	c.Assert(err, jc.ErrorIsNil)

	gotHostPorts, err := s.State.APIHostPortsForClients()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotHostPorts, jc.DeepEquals, newHostPorts)

	gotHostPorts, err = s.State.APIHostPortsForAgents()
	c.Assert(err, jc.ErrorIsNil)
	// First slice filtered down to the address in the management space.
	// Second filtered to zero elements, so retains the supplied slice.
	c.Assert(gotHostPorts, jc.DeepEquals, []network.SpaceHostPorts{{hostPort2}, {hostPort3}})
}

func (s *ControllerAddressesSuite) TestSetAPIHostPortsForAgentsNoDocument(c *gc.C) {
	addrs, err := s.State.APIHostPortsForClients()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.HasLen, 0)

	newHostPorts := []network.SpaceHostPorts{{{
		SpaceAddress: network.NewScopedSpaceAddress("0.2.4.6", network.ScopeCloudLocal),
		NetPort:      1,
	}}}

	// Delete the addresses for agents document before setting.
	col := s.State.MongoSession().DB("juju").C(state.ControllersC)
	key := "apiHostPortsForAgents"
	err = col.RemoveId(key)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(col.FindId(key).One(&bson.D{}), gc.Equals, mgo.ErrNotFound)

	err = s.State.SetAPIHostPorts(newHostPorts)
	c.Assert(err, jc.ErrorIsNil)

	gotHostPorts, err := s.State.APIHostPortsForAgents()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotHostPorts, jc.DeepEquals, newHostPorts)
}

func (s *ControllerAddressesSuite) TestAPIHostPortsForAgentsNoDocument(c *gc.C) {
	addrs, err := s.State.APIHostPortsForClients()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.HasLen, 0)

	newHostPorts := []network.SpaceHostPorts{{{
		SpaceAddress: network.NewScopedSpaceAddress("0.2.4.6", network.ScopeCloudLocal),
		NetPort:      1,
	}}}

	err = s.State.SetAPIHostPorts(newHostPorts)
	c.Assert(err, jc.ErrorIsNil)

	// Delete the addresses for agents document after setting.
	col := s.State.MongoSession().DB("juju").C(state.ControllersC)
	key := "apiHostPortsForAgents"
	err = col.RemoveId(key)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(col.FindId(key).One(&bson.D{}), gc.Equals, mgo.ErrNotFound)

	gotHostPorts, err := s.State.APIHostPortsForAgents()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotHostPorts, jc.DeepEquals, newHostPorts)
}

func (s *ControllerAddressesSuite) TestWatchAPIHostPortsForClients(c *gc.C) {
	w := s.State.WatchAPIHostPortsForClients()
	defer statetesting.AssertStop(c, w)

	// Initial event.
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	err := s.State.SetAPIHostPorts([]network.SpaceHostPorts{network.NewSpaceHostPorts(99, "0.1.2.3")})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertOneChange()

	// Stop, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *ControllerAddressesSuite) TestWatchAPIHostPortsForAgents(c *gc.C) {
	sp, err := s.State.AddSpace("mgmt01", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	s.SetJujuManagementSpace(c, "mgmt01")

	w := s.State.WatchAPIHostPortsForAgents()
	defer statetesting.AssertStop(c, w)

	// Initial event.
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	mgmtHP := network.SpaceHostPort{
		SpaceAddress: network.SpaceAddress{
			MachineAddress: network.MachineAddress{
				Value: "0.4.8.16",
				Type:  network.IPv4Address,
				Scope: network.ScopeCloudLocal,
			},
			SpaceID: sp.Id(),
		},
		NetPort: 2,
	}

	err = s.State.SetAPIHostPorts([]network.SpaceHostPorts{{mgmtHP}})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// This should cause no change to APIHostPortsForAgents.
	// We expect only one watcher notification.
	err = s.State.SetAPIHostPorts([]network.SpaceHostPorts{{
		mgmtHP,
		network.SpaceHostPort{
			SpaceAddress: network.NewScopedSpaceAddress("0.1.2.3", network.ScopeCloudLocal),
			NetPort:      99,
		},
	}})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Stop, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

type CAASAddressesSuite struct {
	statetesting.StateSuite
}

var _ = gc.Suite(&CAASAddressesSuite{})

func (s *CAASAddressesSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	state.SetModelTypeToCAAS(c, s.State, s.Model)
}

func (s *CAASAddressesSuite) TestAPIHostPortsCloudLocalOnly(c *gc.C) {
	machineAddr := network.MachineAddress{
		Value: "10.10.10.10",
		Type:  network.IPv4Address,
		Scope: network.ScopeCloudLocal,
	}

	_, err := s.State.SaveCloudService(state.SaveCloudServiceArgs{
		Id:         s.Model.ControllerUUID(),
		ProviderId: "whatever",
		Addresses:  network.SpaceAddresses{{MachineAddress: machineAddr}},
	})
	c.Assert(err, jc.ErrorIsNil)

	exp := []network.SpaceHostPorts{{{
		SpaceAddress: network.SpaceAddress{MachineAddress: machineAddr},
		NetPort:      17777,
	}}}

	// Make a new non-system state to ensure everything
	//works from any model.
	st := s.Factory.MakeCAASModel(c, nil)
	defer func() { st.Close() }()

	addrs, err := st.APIHostPortsForAgents()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.DeepEquals, exp)

	addrs, err = st.APIHostPortsForClients()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.DeepEquals, exp)
}

func (s *CAASAddressesSuite) TestAPIHostPortsPublicOnly(c *gc.C) {
	machineAddr := network.MachineAddress{
		Value: "10.10.10.10",
		Type:  network.IPv4Address,
		Scope: network.ScopePublic,
	}

	_, err := s.State.SaveCloudService(state.SaveCloudServiceArgs{
		Id:         s.Model.ControllerUUID(),
		ProviderId: "whatever",
		Addresses:  network.SpaceAddresses{{MachineAddress: machineAddr}},
	})
	c.Assert(err, jc.ErrorIsNil)

	exp := []network.SpaceHostPorts{{{
		SpaceAddress: network.SpaceAddress{MachineAddress: machineAddr},
		NetPort:      17777,
	}}}

	// Make a new non-system state to ensure everything
	//works from any model.
	st := s.Factory.MakeCAASModel(c, nil)
	defer func() { st.Close() }()

	addrs, err := st.APIHostPortsForAgents()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.DeepEquals, exp)

	addrs, err = st.APIHostPortsForClients()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.DeepEquals, exp)
}

func (s *CAASAddressesSuite) TestAPIHostPortsMultiple(c *gc.C) {
	machineAddr1 := network.MachineAddress{
		Value: "10.10.10.1",
		Type:  network.IPv4Address,
		Scope: network.ScopePublic,
	}
	machineAddr2 := network.MachineAddress{
		Value: "10.10.10.2",
		Type:  network.IPv4Address,
		Scope: network.ScopePublic,
	}
	machineAddr3 := network.MachineAddress{
		Value: "100.10.10.1",
		Type:  network.IPv4Address,
		Scope: network.ScopeCloudLocal,
	}
	machineAddr4 := network.MachineAddress{
		Value: "100.10.10.2",
		Type:  network.IPv4Address,
		Scope: network.ScopeCloudLocal,
	}

	_, err := s.State.SaveCloudService(state.SaveCloudServiceArgs{
		Id:         s.Model.ControllerUUID(),
		ProviderId: "whatever",
		Addresses: network.SpaceAddresses{
			{MachineAddress: machineAddr1},
			{MachineAddress: machineAddr2},
			{MachineAddress: machineAddr3},
			{MachineAddress: machineAddr4},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	// Make a new non-system state to ensure everything
	//works from any model.
	st := s.Factory.MakeCAASModel(c, nil)
	defer func() { st.Close() }()

	addrs, err := st.APIHostPortsForAgents()
	c.Assert(err, jc.ErrorIsNil)

	// Local-cloud addresses must come first.
	c.Assert(addrs[0][:2], jc.SameContents, network.SpaceHostPorts{
		{
			SpaceAddress: network.SpaceAddress{MachineAddress: machineAddr3},
			NetPort:      17777,
		},
		{
			SpaceAddress: network.SpaceAddress{MachineAddress: machineAddr4},
			NetPort:      17777,
		},
	})

	exp := network.SpaceHostPorts{
		{
			SpaceAddress: network.SpaceAddress{MachineAddress: machineAddr1},
			NetPort:      17777,
		},
		{
			SpaceAddress: network.SpaceAddress{MachineAddress: machineAddr2},
			NetPort:      17777,
		},
	}

	// Public ones should also follow.
	c.Assert(addrs[0][2:], jc.SameContents, exp)

	// Only the public ones should be returned.
	addrs, err = st.APIHostPortsForClients()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.DeepEquals, []network.SpaceHostPorts{exp})
}
