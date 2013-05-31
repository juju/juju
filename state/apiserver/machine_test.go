// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"time"
)

func (s *suite) TestMachineLogin(c *C) {
	stm, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = stm.SetPassword("machine-password")
	c.Assert(err, IsNil)
	err = stm.SetProvisioned("i-foo", "fake_nonce")
	c.Assert(err, IsNil)

	_, info, err := s.APIConn.Environ.StateInfo()
	c.Assert(err, IsNil)

	info.Tag = stm.Tag()
	info.Password = "machine-password"

	st, err := api.Open(info)
	c.Assert(err, IsNil)
	defer st.Close()

	m, err := st.Machine(stm.Id())
	c.Assert(err, IsNil)

	instId, ok := m.InstanceId()
	c.Assert(ok, Equals, true)
	c.Assert(instId, Equals, "i-foo")
}

func (s *suite) TestMachineInstanceId(c *C) {
	stm, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm)

	st := s.openAs(c, stm.Tag())
	defer st.Close()

	m, err := st.Machine(stm.Id())
	c.Assert(err, IsNil)

	instId, ok := m.InstanceId()
	c.Check(instId, Equals, "")
	c.Check(ok, Equals, false)

	err = stm.SetProvisioned("foo", "fake_nonce")
	c.Assert(err, IsNil)

	instId, ok = m.InstanceId()
	c.Check(instId, Equals, "")
	c.Check(ok, Equals, false)

	err = m.Refresh()
	c.Assert(err, IsNil)

	instId, ok = m.InstanceId()
	c.Check(ok, Equals, true)
	c.Assert(instId, Equals, "foo")
}

func (s *suite) TestMachineSetProvisioned(c *C) {
	// TODO (dimitern): If we change the permissions for
	// Machine.SetProvisioned to be laxer, change this test accordingly.
	stm, err := s.State.AddMachine("series", state.JobManageEnviron)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm)

	st := s.openAs(c, stm.Tag())
	defer st.Close()

	m, err := st.Machine(stm.Id())
	c.Assert(err, IsNil)

	instId, ok := stm.InstanceId()
	c.Assert(instId, Equals, state.InstanceId(""))
	c.Assert(ok, Equals, false)
	c.Assert(stm.CheckProvisioned("fake_nonce"), Equals, false)

	err = m.SetProvisioned("foo", "fake_nonce")
	c.Assert(err, IsNil)

	instId, ok = stm.InstanceId()
	c.Assert(instId, Equals, state.InstanceId(""))
	c.Assert(ok, Equals, false)

	err = stm.Refresh()
	c.Assert(err, IsNil)

	instId, ok = stm.InstanceId()
	c.Assert(ok, Equals, true)
	c.Assert(instId, Equals, state.InstanceId("foo"))
	c.Assert(stm.CheckProvisioned("fake_nonce"), Equals, true)
}

func (s *suite) TestMachineSeries(c *C) {
	stm, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm)

	st := s.openAs(c, stm.Tag())
	defer st.Close()

	m, err := st.Machine(stm.Id())
	c.Assert(err, IsNil)

	c.Assert(m.Series(), Equals, "series")
}

func (s *suite) TestMachineConstraints(c *C) {
	// NOTE (dimitern): If we change the permissions for
	// Machine.Constraints to be laxer, change this test accordingly.
	stm, err := s.State.AddMachine("series", state.JobManageEnviron)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm)
	machineConstraints := constraints.MustParse("mem=1G")

	err = stm.SetConstraints(machineConstraints)
	c.Assert(err, IsNil)

	st := s.openAs(c, stm.Tag())
	defer st.Close()

	m, err := st.Machine(stm.Id())
	c.Assert(err, IsNil)

	cons, err := m.Constraints()
	c.Assert(err, IsNil)
	c.Assert(cons, DeepEquals, machineConstraints)
}

func (s *suite) TestMachineRemove(c *C) {
	// TODO (dimitern): If we change the permissions for
	// Machine.Remove to be laxer, change this test accordingly.
	stm0, err := s.State.AddMachine("series", state.JobManageEnviron)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm0)

	stm1, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm1)

	st := s.openAs(c, stm0.Tag())
	defer st.Close()

	m0, err := st.Machine(stm0.Id())
	c.Assert(err, IsNil)
	m1, err := st.Machine(stm1.Id())
	c.Assert(err, IsNil)

	c.Assert(stm0.Life(), Equals, state.Alive)
	c.Assert(stm1.Life(), Equals, state.Alive)

	err = m0.Remove()
	c.Assert(err, ErrorMatches, "cannot remove machine 0: machine is not dead")
	err = m1.Remove()
	c.Assert(err, ErrorMatches, "cannot remove machine 1: machine is not dead")

	err = stm0.EnsureDead()
	c.Assert(err, ErrorMatches, "machine 0 is required by the environment")
	err = stm1.EnsureDead()
	c.Assert(err, IsNil)

	err = stm0.Refresh()
	c.Assert(err, IsNil)
	err = stm1.Refresh()
	c.Assert(err, IsNil)

	c.Assert(stm0.Life(), Equals, state.Alive)
	c.Assert(stm1.Life(), Equals, state.Dead)

	err = m1.Remove()
	c.Assert(err, IsNil)

	err = stm1.Refresh()
	c.Assert(errors.IsNotFoundError(err), Equals, true)
}

func (s *suite) TestMachineLife(c *C) {
	stm, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm)

	st := s.openAs(c, stm.Tag())
	defer st.Close()

	m, err := st.Machine(stm.Id())
	c.Assert(err, IsNil)

	life := m.Life()
	c.Assert(string(life), Equals, "alive")

	err = stm.EnsureDead()
	c.Assert(err, IsNil)

	life = m.Life()
	c.Assert(string(life), Equals, "alive")

	err = m.Refresh()
	c.Assert(err, IsNil)

	life = m.Life()
	c.Assert(string(life), Equals, "dead")
}

func (s *suite) TestMachineEnsureDead(c *C) {
	stm, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm)

	st := s.openAs(c, stm.Tag())
	defer st.Close()

	m, err := st.Machine(stm.Id())
	c.Assert(err, IsNil)

	c.Assert(stm.Life(), Equals, state.Alive)

	err = m.EnsureDead()
	c.Assert(err, IsNil)

	err = stm.Refresh()
	c.Assert(err, IsNil)

	c.Assert(stm.Life(), Equals, state.Dead)
}

func (s *suite) TestMachineEnsureDeadWithAssignedUnit(c *C) {
	stm, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm)

	svc, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	unit, err := svc.AddUnit()
	c.Assert(err, IsNil)
	err = unit.AssignToMachine(stm)
	c.Assert(err, IsNil)

	st := s.openAs(c, stm.Tag())
	defer st.Close()

	m, err := st.Machine(stm.Id())
	c.Assert(err, IsNil)

	c.Assert(stm.Life(), Equals, state.Alive)

	err = m.EnsureDead()
	c.Assert(api.ErrCode(err), Equals, api.CodeHasAssignedUnits)
	c.Assert(err, ErrorMatches, `machine 0 has unit "wordpress/0" assigned`)

	err = stm.Refresh()
	c.Assert(err, IsNil)

	c.Assert(stm.Life(), Equals, state.Alive)

	// Once no unit is assigned, lifecycle can advance.
	err = unit.UnassignFromMachine()
	c.Assert(err, IsNil)

	err = m.EnsureDead()
	c.Assert(err, IsNil)

	c.Assert(stm.Life(), Equals, state.Alive)

	err = stm.Refresh()
	c.Assert(err, IsNil)

	c.Assert(stm.Life(), Equals, state.Dead)
}

func (s *suite) TestMachineSetAgentAlive(c *C) {
	stm, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm)

	st := s.openAs(c, stm.Tag())
	defer st.Close()

	m, err := st.Machine(stm.Id())
	c.Assert(err, IsNil)

	alive, err := stm.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	pinger, err := m.SetAgentAlive()
	c.Assert(err, IsNil)
	c.Assert(pinger, NotNil)

	s.State.Sync()
	alive, err = stm.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)

	err = pinger.Stop()
	c.Assert(err, IsNil)
}

func (s *suite) TestMachineStatus(c *C) {
	// TODO (dimitern): If we change the permissions for
	// Machine.Status to be laxer, change this test accordingly.
	stm, err := s.State.AddMachine("series", state.JobManageEnviron)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm)

	err = stm.SetStatus(params.StatusStopped, "blah")
	c.Assert(err, IsNil)

	st := s.openAs(c, stm.Tag())
	defer st.Close()

	m, err := st.Machine(stm.Id())
	c.Assert(err, IsNil)

	status, info, err := m.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, params.StatusStopped)
	c.Assert(info, Equals, "blah")
}

func (s *suite) TestMachineSetStatus(c *C) {
	stm, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm)

	st := s.openAs(c, stm.Tag())
	defer st.Close()

	m, err := st.Machine(stm.Id())
	c.Assert(err, IsNil)

	status, info, err := stm.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, params.StatusPending)
	c.Assert(info, Equals, "")

	err = m.SetStatus(params.StatusStarted, "blah")
	c.Assert(err, IsNil)

	status, info, err = stm.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, params.StatusStarted)
	c.Assert(info, Equals, "blah")
}

func (s *suite) TestMachineRefresh(c *C) {
	// Add a machine and get its instance id (it's empty at first).
	stm, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	oldId, _ := stm.InstanceId()
	c.Assert(oldId, Equals, state.InstanceId(""))

	// Now open the state connection for that machine.
	setDefaultPassword(c, stm)
	st := s.openAs(c, stm.Tag())
	defer st.Close()

	// Get the machine through the API.
	m, err := st.Machine(stm.Id())
	c.Assert(err, IsNil)
	// Set the original machine's instance id and nonce.
	err = stm.SetProvisioned("foo", "fake_nonce")
	c.Assert(err, IsNil)
	newId, _ := stm.InstanceId()
	c.Assert(newId, Equals, state.InstanceId("foo"))

	// Get the instance id of the machine through the API,
	// it should match the oldId, before the refresh.
	mId, _ := m.InstanceId()
	c.Assert(state.InstanceId(mId), Equals, oldId)
	err = m.Refresh()
	c.Assert(err, IsNil)
	// Now the instance id should be the new one.
	mId, _ = m.InstanceId()
	c.Assert(state.InstanceId(mId), Equals, newId)
}

func (s *suite) TestMachineSetPassword(c *C) {
	stm, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm)

	st := s.openAs(c, stm.Tag())
	defer st.Close()
	m, err := st.Machine(stm.Id())
	c.Assert(err, IsNil)

	err = m.SetPassword("foo")
	c.Assert(err, IsNil)

	err = stm.Refresh()
	c.Assert(err, IsNil)
	c.Assert(stm.PasswordValid("foo"), Equals, true)
}

func (s *suite) TestMachineSetPasswordInMongo(c *C) {
	allowStateAccess := map[state.MachineJob]bool{
		state.JobManageEnviron: true,
		state.JobServeAPI:      true,
		state.JobHostUnits:     false,
	}

	for job, canOpenState := range allowStateAccess {
		stm, err := s.State.AddMachine("series", job)
		c.Assert(err, IsNil)
		setDefaultPassword(c, stm)

		st := s.openAs(c, stm.Tag())
		defer st.Close()
		m, err := st.Machine(stm.Id())
		c.Assert(err, IsNil)

		// Sanity check to start with.
		err = s.tryOpenState(c, m, defaultPassword(stm))
		c.Assert(errors.IsUnauthorizedError(err), Equals, true, Commentf("%v", err))

		err = m.SetPassword("foo")
		c.Assert(err, IsNil)

		err = s.tryOpenState(c, m, "foo")
		if canOpenState {
			c.Assert(err, IsNil)
		} else {
			c.Assert(errors.IsUnauthorizedError(err), Equals, true, Commentf("%v", err))
		}
	}
}

func (s *suite) TestMachineTag(c *C) {
	c.Assert(api.MachineTag("2"), Equals, "machine-2")

	stm, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm)
	st := s.openAs(c, "machine-0")
	defer st.Close()
	m, err := st.Machine("0")
	c.Assert(err, IsNil)
	c.Assert(m.Tag(), Equals, "machine-0")
}

func (s *suite) TestMachineWatch(c *C) {
	stm, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm)

	st := s.openAs(c, stm.Tag())
	defer st.Close()
	m, err := st.Machine(stm.Id())
	c.Assert(err, IsNil)
	w0 := m.Watch()
	w1 := m.Watch()

	// Initial event.
	ok := chanReadEmpty(c, w0.Changes(), "watcher 0")
	c.Assert(ok, Equals, true)

	ok = chanReadEmpty(c, w1.Changes(), "watcher 1")
	c.Assert(ok, Equals, true)

	// No subsequent event until something changes.
	select {
	case <-w0.Changes():
		c.Fatalf("unexpected value on watcher 0")
	case <-w1.Changes():
		c.Fatalf("unexpected value on watcher 1")
	case <-time.After(20 * time.Millisecond):
	}

	err = stm.SetProvisioned("foo", "fake_nonce")
	c.Assert(err, IsNil)

	// Next event.
	ok = chanReadEmpty(c, w0.Changes(), "watcher 0")
	c.Assert(ok, Equals, true)
	ok = chanReadEmpty(c, w1.Changes(), "watcher 1")
	c.Assert(ok, Equals, true)

	err = w0.Stop()
	c.Check(err, IsNil)
	err = w1.Stop()
	c.Check(err, IsNil)

	ok = chanReadEmpty(c, w0.Changes(), "watcher 0")
	c.Assert(ok, Equals, false)
	ok = chanReadEmpty(c, w1.Changes(), "watcher 1")
	c.Assert(ok, Equals, false)
}
