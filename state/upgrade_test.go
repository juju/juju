// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/version"
)

type UpgradeSuite struct {
	ConnSuite
	serverIdA string
}

var _ = gc.Suite(&UpgradeSuite{})

func vers(s string) version.Number {
	return version.MustParse(s)
}

func (s *UpgradeSuite) provision(c *gc.C, machineIds ...string) {
	for _, machineId := range machineIds {
		machine, err := s.State.Machine(machineId)
		c.Assert(err, jc.ErrorIsNil)
		err = machine.SetProvisioned(
			instance.Id(fmt.Sprintf("instance-%s", machineId)),
			fmt.Sprintf("nonce-%s", machineId),
			nil,
		)
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *UpgradeSuite) addStateServers(c *gc.C) (machineId1, machineId2 string) {
	changes, err := s.State.EnsureAvailability(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	return changes.Added[0], changes.Added[1]
}

func (s *UpgradeSuite) assertUpgrading(c *gc.C, expect bool) {
	upgrading, err := s.State.IsUpgrading()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(upgrading, gc.Equals, expect)
}

func (s *UpgradeSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	stateServer, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)
	pinger, err := stateServer.SetAgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		err := pinger.Stop()
		c.Check(err, jc.ErrorIsNil)
	})
	s.serverIdA = stateServer.Id()
	s.provision(c, s.serverIdA)
}

func (s *UpgradeSuite) TestEnsureUpgradeInfo(c *gc.C) {
	vPrevious := vers("1.2.3")
	vTarget := vers("2.3.4")
	vMismatch := vers("1.9.1")

	// create
	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, vPrevious, vTarget)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.PreviousVersion(), gc.DeepEquals, vPrevious)
	c.Assert(info.TargetVersion(), gc.DeepEquals, vTarget)
	c.Assert(info.Status(), gc.Equals, state.UpgradePending)
	c.Assert(info.Started().IsZero(), jc.IsFalse)
	c.Assert(info.StateServersReady(), gc.DeepEquals, []string{s.serverIdA})
	c.Assert(info.StateServersDone(), gc.HasLen, 0)

	// retrieve existing
	info, err = s.State.EnsureUpgradeInfo(s.serverIdA, vPrevious, vTarget)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.PreviousVersion(), gc.DeepEquals, vPrevious)
	c.Assert(info.TargetVersion(), gc.DeepEquals, vTarget)

	// mismatching previous
	info, err = s.State.EnsureUpgradeInfo(s.serverIdA, vMismatch, vTarget)
	c.Assert(err, gc.ErrorMatches, "current upgrade info mismatch: expected previous version 1.9.1, got 1.2.3")
	c.Assert(info, gc.IsNil)

	// mismatching target
	info, err = s.State.EnsureUpgradeInfo(s.serverIdA, vPrevious, vMismatch)
	c.Assert(err, gc.ErrorMatches, "current upgrade info mismatch: expected target version 1.9.1, got 2.3.4")
	c.Assert(info, gc.IsNil)
}

func (s *UpgradeSuite) TestStateServersReadyCopies(c *gc.C) {
	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, vers("1.2.3"), vers("2.4.5"))
	c.Assert(err, jc.ErrorIsNil)
	stateServersReady := info.StateServersReady()
	c.Assert(stateServersReady, gc.DeepEquals, []string{"0"})
	stateServersReady[0] = "lol"
	stateServersReady = info.StateServersReady()
	c.Assert(stateServersReady, gc.DeepEquals, []string{"0"})
}

func (s *UpgradeSuite) TestStateServersDoneCopies(c *gc.C) {
	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, vers("1.2.3"), vers("2.4.5"))
	c.Assert(err, jc.ErrorIsNil)
	s.setToFinishing(c, info)
	err = info.SetStateServerDone("0")
	c.Assert(err, jc.ErrorIsNil)

	info = s.getOneUpgradeInfo(c)
	stateServersDone := info.StateServersDone()
	c.Assert(stateServersDone, gc.DeepEquals, []string{"0"})
	stateServersDone[0] = "lol"
	stateServersDone = info.StateServersReady()
	c.Assert(stateServersDone, gc.DeepEquals, []string{"0"})
}

func (s *UpgradeSuite) TestEnsureUpgradeInfoDowngrade(c *gc.C) {
	v123 := vers("1.2.3")
	v111 := vers("1.1.1")

	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, v123, v111)
	c.Assert(err, gc.ErrorMatches, "cannot sanely upgrade from 1.2.3 to 1.1.1")
	c.Assert(info, gc.IsNil)

	info, err = s.State.EnsureUpgradeInfo(s.serverIdA, v123, v123)
	c.Assert(err, gc.ErrorMatches, "cannot sanely upgrade from 1.2.3 to 1.2.3")
	c.Assert(info, gc.IsNil)
}

func (s *UpgradeSuite) TestEnsureUpgradeInfoNonStateServer(c *gc.C) {
	info, err := s.State.EnsureUpgradeInfo("2345678", vers("1.2.3"), vers("2.3.4"))
	c.Assert(err, gc.ErrorMatches, `machine "2345678" is not a state server`)
	c.Assert(info, gc.IsNil)
}

func (s *UpgradeSuite) TestEnsureUpgradeInfoNotProvisioned(c *gc.C) {
	serverIdB, _ := s.addStateServers(c)
	_, err := s.State.EnsureUpgradeInfo(serverIdB, vers("1.1.1"), vers("1.2.3"))
	expectErr := fmt.Sprintf("machine %s is not provisioned and should not be participating in upgrades", serverIdB)
	c.Assert(err, gc.ErrorMatches, expectErr)
}

func (s *UpgradeSuite) TestEnsureUpgradeInfoMultipleServers(c *gc.C) {
	serverIdB, serverIdC := s.addStateServers(c)
	s.provision(c, serverIdB, serverIdC)

	v111 := vers("1.1.1")
	v123 := vers("1.2.3")
	_, err := s.State.EnsureUpgradeInfo(s.serverIdA, v111, v123)
	c.Assert(err, jc.ErrorIsNil)

	// add first new state server with bad version
	info, err := s.State.EnsureUpgradeInfo(serverIdB, v111, vers("1.2.4"))
	c.Assert(err, gc.ErrorMatches, "current upgrade info mismatch: expected target version 1.2.4, got 1.2.3")
	c.Assert(info, gc.IsNil)

	// add first new state server properly
	info, err = s.State.EnsureUpgradeInfo(serverIdB, v111, v123)
	c.Assert(err, jc.ErrorIsNil)
	expectReady := []string{s.serverIdA, serverIdB}
	c.Assert(info.StateServersReady(), jc.SameContents, expectReady)

	// add second new state server
	info, err = s.State.EnsureUpgradeInfo(serverIdC, v111, v123)
	c.Assert(err, jc.ErrorIsNil)
	expectReady = append(expectReady, serverIdC)
	c.Assert(info.StateServersReady(), jc.SameContents, expectReady)

	// add second new state server again
	info, err = s.State.EnsureUpgradeInfo(serverIdC, v111, v123)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.StateServersReady(), jc.SameContents, expectReady)
}

func (s *UpgradeSuite) TestEnsureUpgradeInfoRace(c *gc.C) {
	v100 := vers("1.0.0")
	v200 := vers("2.0.0")

	_, err := s.State.EnsureUpgradeInfo(s.serverIdA, v100, v200)
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetAfterHooks(c, s.State, func() {
		err := s.State.ClearUpgradeInfo()
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, v100, v200)
	c.Assert(err, gc.ErrorMatches, "current upgrade info not found")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(info, gc.IsNil)
}

func (s *UpgradeSuite) TestEnsureUpgradeInfoMultipleServersRace1(c *gc.C) {
	serverIdB, serverIdC := s.addStateServers(c)
	s.provision(c, serverIdB, serverIdC)

	v111 := vers("1.1.1")
	v123 := vers("1.2.3")
	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.State.EnsureUpgradeInfo(serverIdC, v111, v123)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	info, err := s.State.EnsureUpgradeInfo(serverIdB, v111, v123)
	c.Assert(err, jc.ErrorIsNil)
	expectReady := []string{serverIdB, serverIdC}
	c.Assert(info.StateServersReady(), jc.SameContents, expectReady)
}

func (s *UpgradeSuite) TestEnsureUpgradeInfoMultipleServersRace2(c *gc.C) {
	serverIdB, serverIdC := s.addStateServers(c)
	s.provision(c, serverIdB, serverIdC)

	v111 := vers("1.1.1")
	v123 := vers("1.2.3")
	_, err := s.State.EnsureUpgradeInfo(s.serverIdA, v111, v123)
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetAfterHooks(c, s.State, func() {
		_, err := s.State.EnsureUpgradeInfo(serverIdC, v111, v123)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	info, err := s.State.EnsureUpgradeInfo(serverIdB, v111, v123)
	c.Assert(err, jc.ErrorIsNil)
	expectReady := []string{s.serverIdA, serverIdB, serverIdC}
	c.Assert(info.StateServersReady(), jc.SameContents, expectReady)
}

func (s *UpgradeSuite) TestEnsureUpgradeInfoMultipleServersRace3(c *gc.C) {
	serverIdB, serverIdC := s.addStateServers(c)
	s.provision(c, serverIdB, serverIdC)

	v111 := vers("1.1.1")
	v123 := vers("1.2.3")
	v124 := vers("1.2.4")
	_, err := s.State.EnsureUpgradeInfo(s.serverIdA, v111, v123)
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, nil, func() {
		err := s.State.ClearUpgradeInfo()
		c.Assert(err, jc.ErrorIsNil)
		_, err = s.State.EnsureUpgradeInfo(serverIdC, v111, v124)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	_, err = s.State.EnsureUpgradeInfo(serverIdB, v111, v123)
	c.Assert(err, gc.ErrorMatches, "upgrade info changed during update")
}

func (s *UpgradeSuite) TestEnsureUpgradeInfoMultipleServersRace4(c *gc.C) {
	serverIdB, serverIdC := s.addStateServers(c)
	s.provision(c, serverIdB, serverIdC)

	v111 := vers("1.1.1")
	v123 := vers("1.2.3")
	v124 := vers("1.2.4")
	_, err := s.State.EnsureUpgradeInfo(s.serverIdA, v111, v123)
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetAfterHooks(c, s.State, nil, func() {
		err := s.State.ClearUpgradeInfo()
		c.Assert(err, jc.ErrorIsNil)
		_, err = s.State.EnsureUpgradeInfo(serverIdC, v111, v124)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	_, err = s.State.EnsureUpgradeInfo(serverIdB, v111, v123)
	c.Assert(err, gc.ErrorMatches, "current upgrade info mismatch: expected target version 1.2.3, got 1.2.4")
}

func (s *UpgradeSuite) TestRefresh(c *gc.C) {
	v111 := vers("1.1.1")
	v123 := vers("1.2.3")
	serverIdB, _ := s.addStateServers(c)
	s.provision(c, serverIdB)

	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, v111, v123)
	c.Assert(err, jc.ErrorIsNil)
	info2, err := s.State.EnsureUpgradeInfo(serverIdB, v111, v123)
	c.Assert(err, jc.ErrorIsNil)
	info2.SetStatus(state.UpgradeRunning)

	c.Assert(info.StateServersReady(), jc.SameContents, []string{s.serverIdA})
	c.Assert(info.Status(), gc.Equals, state.UpgradePending)

	err = info.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(info.StateServersReady(), jc.SameContents, []string{s.serverIdA, serverIdB})
	c.Assert(info.Status(), gc.Equals, state.UpgradeRunning)
}

func (s *UpgradeSuite) TestWatch(c *gc.C) {
	v111 := vers("1.1.1")
	v123 := vers("1.2.3")
	serverIdB, serverIdC := s.addStateServers(c)
	s.provision(c, serverIdB, serverIdC)

	w := s.State.WatchUpgradeInfo()
	defer statetesting.AssertStop(c, w)

	// initial event
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// single change is reported
	_, err := s.State.EnsureUpgradeInfo(s.serverIdA, v111, v123)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// non-change is not reported
	_, err = s.State.EnsureUpgradeInfo(s.serverIdA, v111, v123)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// changes are coalesced
	_, err = s.State.EnsureUpgradeInfo(serverIdB, v111, v123)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.EnsureUpgradeInfo(serverIdC, v111, v123)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// closed on stop
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *UpgradeSuite) TestWatchMethod(c *gc.C) {
	v111 := vers("1.1.1")
	v123 := vers("1.2.3")
	serverIdB, serverIdC := s.addStateServers(c)
	s.provision(c, serverIdB, serverIdC)

	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, v111, v123)
	c.Assert(err, jc.ErrorIsNil)

	w := info.Watch()
	defer statetesting.AssertStop(c, w)

	// initial event
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// single change is reported
	info, err = s.State.EnsureUpgradeInfo(serverIdB, v111, v123)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// non-change is not reported
	info, err = s.State.EnsureUpgradeInfo(serverIdB, v111, v123)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// changes are coalesced
	_, err = s.State.EnsureUpgradeInfo(serverIdC, v111, v123)
	c.Assert(err, jc.ErrorIsNil)
	err = info.SetStatus(state.UpgradeRunning)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// closed on stop
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *UpgradeSuite) TestAllProvisionedStateServersReady(c *gc.C) {
	serverIdB, serverIdC := s.addStateServers(c)
	s.provision(c, serverIdB)

	v111 := vers("1.1.1")
	v123 := vers("1.2.3")
	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, v111, v123)
	c.Assert(err, jc.ErrorIsNil)

	assertReady := func(expect bool) {
		ok, err := info.AllProvisionedStateServersReady()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(ok, gc.Equals, expect)
	}
	assertReady(false)

	info, err = s.State.EnsureUpgradeInfo(serverIdB, v111, v123)
	c.Assert(err, jc.ErrorIsNil)
	assertReady(true)

	s.provision(c, serverIdC)
	assertReady(false)

	info, err = s.State.EnsureUpgradeInfo(serverIdC, v111, v123)
	c.Assert(err, jc.ErrorIsNil)
	assertReady(true)
}

func (s *UpgradeSuite) TestAllProvisionedStateServersReadyWithPreEnvUUIDSchema(c *gc.C) {
	serverIdB, serverIdC := s.addStateServers(c)

	machines, closer := state.GetRawCollection(s.State, state.MachinesC)
	defer closer()
	instanceData, closer := state.GetRawCollection(s.State, state.InstanceDataC)
	defer closer()

	// Add minimal machine and instanceData docs for the state servers
	// that look how these documents did before the environment UUID
	// migration.
	_, err := instanceData.RemoveAll(nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = machines.RemoveAll(nil)
	c.Assert(err, jc.ErrorIsNil)

	addLegacyMachine := func(machineId string) {
		err := machines.Insert(bson.M{"_id": machineId})
		c.Assert(err, jc.ErrorIsNil)
	}
	addLegacyMachine(s.serverIdA)
	addLegacyMachine(serverIdB)
	addLegacyMachine(serverIdC)

	legacyProvision := func(machineId string) {
		err := instanceData.Insert(bson.M{"_id": machineId})
		c.Assert(err, jc.ErrorIsNil)
	}
	legacyProvision(s.serverIdA)
	legacyProvision(serverIdB)

	v111 := vers("1.1.1")
	v123 := vers("1.2.3")
	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, v111, v123)
	c.Assert(err, jc.ErrorIsNil)

	assertReady := func(expect bool) {
		ok, err := info.AllProvisionedStateServersReady()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(ok, gc.Equals, expect)
	}
	assertReady(false)

	info, err = s.State.EnsureUpgradeInfo(serverIdB, v111, v123)
	c.Assert(err, jc.ErrorIsNil)
	assertReady(true)

	legacyProvision(serverIdC)
	assertReady(false)

	info, err = s.State.EnsureUpgradeInfo(serverIdC, v111, v123)
	c.Assert(err, jc.ErrorIsNil)
	assertReady(true)
}

func (s *UpgradeSuite) TestSetStatus(c *gc.C) {
	v123 := vers("1.2.3")
	v234 := vers("2.3.4")
	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, v123, v234)
	c.Assert(err, jc.ErrorIsNil)

	assertStatus := func(expect state.UpgradeStatus) {
		info, err := s.State.EnsureUpgradeInfo(s.serverIdA, v123, v234)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(info.Status(), gc.Equals, expect)
	}
	err = info.SetStatus(state.UpgradePending)
	c.Assert(err, gc.ErrorMatches, `cannot explicitly set upgrade status to "pending"`)
	assertStatus(state.UpgradePending)

	err = info.SetStatus(state.UpgradeFinishing)
	c.Assert(err, gc.ErrorMatches, `cannot set upgrade status to "finishing": `+
		"Another status change may have occurred concurrently")
	assertStatus(state.UpgradePending)

	err = info.SetStatus(state.UpgradeComplete)
	c.Assert(err, gc.ErrorMatches, `cannot explicitly set upgrade status to "complete"`)
	assertStatus(state.UpgradePending)

	err = info.SetStatus(state.UpgradeAborted)
	c.Assert(err, gc.ErrorMatches, `cannot explicitly set upgrade status to "aborted"`)
	assertStatus(state.UpgradePending)

	err = info.SetStatus(state.UpgradeStatus("lol"))
	c.Assert(err, gc.ErrorMatches, "unknown upgrade status: lol")
	assertStatus(state.UpgradePending)

	err = info.SetStatus(state.UpgradeRunning)
	c.Assert(err, jc.ErrorIsNil)
	assertStatus(state.UpgradeRunning)
	err = info.SetStatus(state.UpgradeRunning)
	c.Assert(err, jc.ErrorIsNil)
	assertStatus(state.UpgradeRunning)

	err = info.SetStatus(state.UpgradeFinishing)
	c.Assert(err, jc.ErrorIsNil)
	assertStatus(state.UpgradeFinishing)
	err = info.SetStatus(state.UpgradeFinishing)
	c.Assert(err, jc.ErrorIsNil)
	assertStatus(state.UpgradeFinishing)
	err = info.SetStatus(state.UpgradeRunning)
	c.Assert(err, gc.ErrorMatches, `cannot set upgrade status to "running": `+
		"Another status change may have occurred concurrently")
	assertStatus(state.UpgradeFinishing)
}

func (s *UpgradeSuite) TestSetStateServerDone(c *gc.C) {
	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, vers("1.2.3"), vers("2.3.4"))
	c.Assert(err, jc.ErrorIsNil)

	err = info.SetStateServerDone(s.serverIdA)
	c.Assert(err, gc.ErrorMatches, "cannot complete upgrade: upgrade has not yet run")

	err = info.SetStatus(state.UpgradeRunning)
	c.Assert(err, jc.ErrorIsNil)
	err = info.SetStateServerDone(s.serverIdA)
	c.Assert(err, gc.ErrorMatches, "cannot complete upgrade: upgrade has not yet run")

	err = info.SetStatus(state.UpgradeFinishing)
	c.Assert(err, jc.ErrorIsNil)
	err = info.SetStateServerDone(s.serverIdA)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpgrading(c, false)

	s.checkUpgradeInfoArchived(c, info, state.UpgradeComplete, 1)
}

func (s *UpgradeSuite) TestSetStateServerDoneMultipleServers(c *gc.C) {
	v111 := vers("1.1.1")
	v123 := vers("1.2.3")
	serverIdB, serverIdC := s.addStateServers(c)
	s.provision(c, serverIdB, serverIdC)
	for _, id := range []string{serverIdB, serverIdC} {
		_, err := s.State.EnsureUpgradeInfo(id, v111, v123)
		c.Assert(err, jc.ErrorIsNil)
	}

	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, v111, v123)
	c.Assert(err, jc.ErrorIsNil)
	s.setToFinishing(c, info)

	err = info.SetStateServerDone(s.serverIdA)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpgrading(c, true)

	err = info.SetStateServerDone(s.serverIdA)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpgrading(c, true)

	err = info.SetStateServerDone(serverIdB)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpgrading(c, true)

	err = info.SetStateServerDone(serverIdC)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpgrading(c, false)

	s.checkUpgradeInfoArchived(c, info, state.UpgradeComplete, 3)
}

func (s *UpgradeSuite) TestSetStateServerDoneMultipleServersRace(c *gc.C) {
	v100 := vers("1.0.0")
	v200 := vers("2.0.0")
	serverIdB, serverIdC := s.addStateServers(c)
	s.provision(c, serverIdB, serverIdC)

	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, v100, v200)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.EnsureUpgradeInfo(serverIdB, v100, v200)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.EnsureUpgradeInfo(serverIdC, v100, v200)
	c.Assert(err, jc.ErrorIsNil)
	s.setToFinishing(c, info)

	// Interrupt the transaction for state server A twice with calls
	// from the other machines.
	defer state.SetBeforeHooks(c, s.State, func() {
		err = info.SetStateServerDone(serverIdB)
		c.Assert(err, jc.ErrorIsNil)
	}, func() {
		err = info.SetStateServerDone(serverIdC)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	err = info.SetStateServerDone(s.serverIdA)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpgrading(c, false)

	info = s.getOneUpgradeInfo(c)
	c.Assert(info.Status(), gc.Equals, state.UpgradeComplete)
	c.Assert(info.StateServersDone(), jc.SameContents, []string{"0", "1", "2"})
}

func (s *UpgradeSuite) TestAbort(c *gc.C) {
	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, vers("1.2.3"), vers("2.3.4"))
	c.Assert(err, jc.ErrorIsNil)

	err = info.Abort()
	c.Assert(err, jc.ErrorIsNil)

	s.checkUpgradeInfoArchived(c, info, state.UpgradeAborted, 0)
}

func (s *UpgradeSuite) TestAbortRace(c *gc.C) {
	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, vers("1.2.3"), vers("2.3.4"))
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		err = info.Abort()
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	err = info.Abort()
	c.Assert(err, jc.ErrorIsNil)

	s.checkUpgradeInfoArchived(c, info, state.UpgradeAborted, 0)
}

func (s *UpgradeSuite) checkUpgradeInfoArchived(
	c *gc.C,
	initialInfo *state.UpgradeInfo,
	expectedStatus state.UpgradeStatus,
	expectedStateServers int,
) {
	info := s.getOneUpgradeInfo(c)
	c.Assert(info.Status(), gc.Equals, expectedStatus)
	c.Assert(info.PreviousVersion(), gc.Equals, initialInfo.PreviousVersion())
	c.Assert(info.TargetVersion(), gc.Equals, initialInfo.TargetVersion())
	// Truncate because mongo only stores times down to millisecond resolution.
	c.Assert(info.Started().Equal(initialInfo.Started().Truncate(time.Millisecond)), jc.IsTrue)
	c.Assert(len(info.StateServersDone()), gc.Equals, expectedStateServers)
	if expectedStateServers > 0 {
		c.Assert(info.StateServersDone(), jc.SameContents, info.StateServersReady())
	}
}

func (s *UpgradeSuite) getOneUpgradeInfo(c *gc.C) *state.UpgradeInfo {
	upgradeInfos, err := state.GetAllUpgradeInfos(s.State)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(upgradeInfos), gc.Equals, 1)
	return upgradeInfos[0]
}

func (s *UpgradeSuite) TestAbortCurrentUpgrade(c *gc.C) {
	// First try with nothing to abort.
	err := s.State.AbortCurrentUpgrade()
	c.Assert(err, jc.ErrorIsNil)

	upgradeInfos, err := state.GetAllUpgradeInfos(s.State)
	c.Assert(len(upgradeInfos), gc.Equals, 0)

	// Now create a UpgradeInfo to abort.
	_, err = s.State.EnsureUpgradeInfo(s.serverIdA, vers("1.1.1"), vers("1.2.3"))
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.AbortCurrentUpgrade()
	c.Assert(err, jc.ErrorIsNil)

	info := s.getOneUpgradeInfo(c)
	c.Check(info.Status(), gc.Equals, state.UpgradeAborted)

	// It should now be possible to start another upgrade.
	_, err = s.State.EnsureUpgradeInfo(s.serverIdA, vers("1.2.3"), vers("1.3.0"))
	c.Check(err, jc.ErrorIsNil)
}

func (s *UpgradeSuite) TestClearUpgradeInfo(c *gc.C) {
	v111 := vers("1.1.1")
	v123 := vers("1.2.3")
	v153 := vers("1.5.3")

	s.assertUpgrading(c, false)
	_, err := s.State.EnsureUpgradeInfo(s.serverIdA, v111, v123)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpgrading(c, true)

	err = s.State.ClearUpgradeInfo()
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpgrading(c, false)

	_, err = s.State.EnsureUpgradeInfo(s.serverIdA, v111, v153)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpgrading(c, true)
}

func (s *UpgradeSuite) TestServiceUnitSeqToSequence(c *gc.C) {
	v123 := vers("1.2.3")
	v124 := vers("1.2.4")

	s.assertUpgrading(c, false)
	_, err := s.State.EnsureUpgradeInfo(s.serverIdA, v123, v124)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpgrading(c, true)
}

func (s *UpgradeSuite) setToFinishing(c *gc.C, info *state.UpgradeInfo) {
	err := info.SetStatus(state.UpgradeRunning)
	c.Assert(err, jc.ErrorIsNil)
	err = info.SetStatus(state.UpgradeFinishing)
	c.Assert(err, jc.ErrorIsNil)
}
