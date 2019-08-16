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
	netAddress := network.Address{
		Value: "0.0.0.0",
		Type:  network.IPv4Address,
		Scope: network.ScopeUnknown,
	}
	state.AssertAddressConversion(c, netAddress)
}

func (s *AddressSuite) TestHostPortConversion(c *gc.C) {
	netAddress := network.Address{
		Value: "0.0.0.0",
		Type:  network.IPv4Address,
		Scope: network.ScopeUnknown,
	}
	netHostPort := network.HostPort{
		Address: netAddress,
		Port:    4711,
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
		Addresses: []network.Address{
			{Value: "192.168.2.144", Type: network.IPv4Address},
			{Value: "10.0.1.2", Type: network.IPv4Address},
		},
	})
	c.Logf("machine addresses: %#v", machine.Addresses())
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

	newHostPorts := [][]network.HostPort{{{
		Address: network.Address{
			Value: "0.2.4.6",
			Type:  network.IPv4Address,
			Scope: network.ScopeCloudLocal,
		},
		Port: 1,
	}, {
		Address: network.Address{
			Value: "0.4.8.16",
			Type:  network.IPv4Address,
			Scope: network.ScopePublic,
		},
		Port: 2,
	}}, {{
		Address: network.Address{
			Value: "0.6.1.2",
			Type:  network.IPv4Address,
			Scope: network.ScopeCloudLocal,
		},
		Port: 5,
	}}}
	err = s.State.SetAPIHostPorts(newHostPorts)
	c.Assert(err, jc.ErrorIsNil)

	gotHostPorts, err := s.State.APIHostPortsForClients()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotHostPorts, jc.DeepEquals, newHostPorts)

	gotHostPorts, err = s.State.APIHostPortsForAgents()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotHostPorts, jc.DeepEquals, newHostPorts)

	newHostPorts = [][]network.HostPort{{{
		Address: network.Address{
			Value: "0.2.4.6",
			Type:  network.IPv6Address,
			Scope: network.ScopeCloudLocal,
		},
		Port: 13,
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
	hostPorts := [][]network.HostPort{{{
		Address: network.Address{
			Value: "0.4.8.16",
			Type:  network.IPv4Address,
			Scope: network.ScopePublic,
		},
		Port: 2,
	}}, {{
		Address: network.Address{
			Value: "0.2.4.6",
			Type:  network.IPv4Address,
			Scope: network.ScopeCloudLocal,
		},
		Port: 1,
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
	hostPorts0 := []network.HostPort{{
		Address: network.Address{
			Value: "0.4.8.16",
			Type:  network.IPv4Address,
			Scope: network.ScopePublic,
		},
		Port: 2,
	}}
	hostPorts1 := []network.HostPort{{
		Address: network.Address{
			Value: "0.2.4.6",
			Type:  network.IPv4Address,
			Scope: network.ScopeCloudLocal,
		},
		Port: 1,
	}}

	// API host ports are concurrently changed to different
	// values; second arrival will fail its assertion, refresh
	// finding and reattempt.

	ctrC := state.ControllersC
	var prevRevno int64
	var prevAgentsRevno int64
	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.State.SetAPIHostPorts([][]network.HostPort{hostPorts0})
		c.Assert(err, jc.ErrorIsNil)
		revno, err := state.TxnRevno(s.State, ctrC, "apiHostPorts")
		c.Assert(err, jc.ErrorIsNil)
		prevRevno = revno
		revno, err = state.TxnRevno(s.State, ctrC, "apiHostPortsForAgents")
		c.Assert(err, jc.ErrorIsNil)
		prevAgentsRevno = revno
	}).Check()

	err := s.State.SetAPIHostPorts([][]network.HostPort{hostPorts1})
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
	c.Assert(hostPorts, gc.DeepEquals, [][]network.HostPort{hostPorts1})

	hostPorts, err = s.State.APIHostPortsForAgents()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hostPorts, gc.DeepEquals, [][]network.HostPort{hostPorts1})
}

func (s *ControllerAddressesSuite) TestSetAPIHostPortsWithMgmtSpace(c *gc.C) {
	_, err := s.State.AddSpace("mgmt01", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	s.SetJujuManagementSpace(c, "mgmt01")

	addrs, err := s.State.APIHostPortsForClients()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.HasLen, 0)

	hostPort1 := network.HostPort{
		Address: network.Address{
			Value: "0.2.4.6",
			Type:  network.IPv4Address,
			Scope: network.ScopeCloudLocal,
		},
		Port: 1,
	}
	hostPort2 := network.HostPort{
		Address: network.Address{
			Value:     "0.4.8.16",
			Type:      network.IPv4Address,
			Scope:     network.ScopePublic,
			SpaceName: "mgmt01",
		},
		Port: 2,
	}
	hostPort3 := network.HostPort{
		Address: network.Address{
			Value: "0.6.1.2",
			Type:  network.IPv4Address,
			Scope: network.ScopeCloudLocal,
		},
		Port: 5,
	}
	newHostPorts := [][]network.HostPort{{hostPort1, hostPort2}, {hostPort3}}

	err = s.State.SetAPIHostPorts(newHostPorts)
	c.Assert(err, jc.ErrorIsNil)

	gotHostPorts, err := s.State.APIHostPortsForClients()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotHostPorts, jc.DeepEquals, newHostPorts)

	gotHostPorts, err = s.State.APIHostPortsForAgents()
	c.Assert(err, jc.ErrorIsNil)
	// First slice filtered down to the address in the management space.
	// Second filtered to zero elements, so retains the supplied slice.
	c.Assert(gotHostPorts, jc.DeepEquals, [][]network.HostPort{{hostPort2}, {hostPort3}})
}

func (s *ControllerAddressesSuite) TestSetAPIHostPortsForAgentsNoDocument(c *gc.C) {
	addrs, err := s.State.APIHostPortsForClients()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.HasLen, 0)

	newHostPorts := [][]network.HostPort{{{
		Address: network.Address{
			Value: "0.2.4.6",
			Type:  network.IPv4Address,
			Scope: network.ScopeCloudLocal,
		},
		Port: 1,
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

	newHostPorts := [][]network.HostPort{{{
		Address: network.Address{
			Value: "0.2.4.6",
			Type:  network.IPv4Address,
			Scope: network.ScopeCloudLocal,
		},
		Port: 1,
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

	err := s.State.SetAPIHostPorts([][]network.HostPort{
		network.NewHostPorts(99, "0.1.2.3"),
	})
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertOneChange()

	// Stop, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *ControllerAddressesSuite) TestWatchAPIHostPortsForAgents(c *gc.C) {
	_, err := s.State.AddSpace("mgmt01", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	s.SetJujuManagementSpace(c, "mgmt01")

	w := s.State.WatchAPIHostPortsForAgents()
	defer statetesting.AssertStop(c, w)

	// Initial event.
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	mgmtHP := network.HostPort{
		Address: network.Address{
			Value:     "0.4.8.16",
			Type:      network.IPv4Address,
			Scope:     network.ScopeCloudLocal,
			SpaceName: "mgmt01",
		},
		Port: 2,
	}

	err = s.State.SetAPIHostPorts([][]network.HostPort{{
		mgmtHP,
	}})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// This should cause no change to APIHostPortsForAgents.
	// We expect only one watcher notification.
	err = s.State.SetAPIHostPorts([][]network.HostPort{{
		mgmtHP,
		network.HostPort{
			Address: network.Address{
				Value: "0.1.2.3",
				Type:  network.IPv4Address,
				Scope: network.ScopeCloudLocal,
			},
			Port: 99,
		},
	}})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Stop, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}
