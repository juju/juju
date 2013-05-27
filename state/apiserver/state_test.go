// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"time"
)

func (s *suite) TestStateAllMachines(c *C) {
	stMachines := make([]*state.Machine, 3)
	var err error
	for i := 0; i < len(stMachines); i++ {
		job := state.JobHostUnits
		if i == 0 {
			job = state.JobManageEnviron
		}
		stMachines[i], err = s.State.AddMachine("series", job)
		c.Assert(err, IsNil)
		setDefaultPassword(c, stMachines[i])
	}
	// TODO (dimitern): If we change the permissions for
	// State.AllMachines to be laxer, change this test accordingly.
	st := s.openAs(c, stMachines[0].Tag())
	defer st.Close()

	ids, err := st.AllMachines()
	c.Assert(err, IsNil)
	c.Assert(ids, HasLen, 3)
	for i := 0; i < len(ids); i++ {
		c.Assert(ids[i].Id(), Equals, fmt.Sprintf("%d", i))
	}

	err = stMachines[1].EnsureDead()
	c.Assert(err, IsNil)
	err = stMachines[1].Remove()
	c.Assert(err, IsNil)

	ids, err = st.AllMachines()
	c.Assert(err, IsNil)
	c.Assert(ids, HasLen, 2)
	c.Assert(ids[0].Id(), Equals, "0")
	c.Assert(ids[1].Id(), Equals, "2")
}

func (s *suite) TestPing(c *C) {
	stm, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm)

	st := s.openAs(c, stm.Tag())
	defer st.Close()

	err = st.Ping()
	c.Assert(err, IsNil)
}

func (s *suite) TestStateWatchMachines(c *C) {
	stMachines := make([]*state.Machine, 3)
	var err error
	for i := range stMachines {
		job := state.JobHostUnits
		if i == 0 {
			job = state.JobManageEnviron
		}
		stMachines[i], err = s.State.AddMachine("series", job)
		c.Assert(err, IsNil)
		setDefaultPassword(c, stMachines[i])
	}
	st := s.openAs(c, stMachines[0].Tag())
	defer st.Close()

	// Start watching.
	machinesWatcher := st.WatchMachines()

	// Initial event.
	ids, ok := chanReadStrings(c, machinesWatcher.Changes(), "machines watcher")
	c.Assert(ok, Equals, true)
	c.Assert(ids, DeepEquals, []string{"0", "1", "2"})

	// No subsequent event until something changes.
	select {
	case <-machinesWatcher.Changes():
		c.Fatalf("unexpected value on machines watcher")
	case <-time.After(20 * time.Millisecond):
	}

	// Trigger a change.
	m, err := st.Machine(stMachines[1].Id())
	c.Assert(err, IsNil)
	err = m.EnsureDead()
	c.Assert(err, IsNil)

	// Next event.
	ids, ok = chanReadStrings(c, machinesWatcher.Changes(), "machines watcher")
	c.Assert(ok, Equals, true)
	c.Assert(ids, DeepEquals, []string{"1"})

	// Check the watcher stops cleanly.
	err = machinesWatcher.Stop()
	c.Check(err, IsNil)

	_, ok = chanReadStrings(c, machinesWatcher.Changes(), "machines watcher")
	c.Assert(ok, Equals, false)
}

func (s *suite) TestStateWatchEnvironConfig(c *C) {
	stm, err := s.State.AddMachine("series", state.JobManageEnviron)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm)

	st := s.openAs(c, stm.Tag())
	defer st.Close()

	currentConfig, err := s.State.EnvironConfig()
	c.Assert(err, IsNil)

	// Start watching.
	envConfigWatcher := st.WatchEnvironConfig()

	// Initial event.
	envConfig, ok := chanReadConfig(c, envConfigWatcher.Changes(), "environ config watcher")
	c.Assert(ok, Equals, true)
	c.Assert(envConfig, DeepEquals, currentConfig)

	// No subsequent event until something changes.
	select {
	case <-envConfigWatcher.Changes():
		c.Fatalf("unexpected value on environ config watcher")
	case <-time.After(200 * time.Millisecond):
	}

	// Trigger a change.
	attrs := currentConfig.AllAttrs()
	attrs["foo"] = "bar"
	currentConfig, err = config.New(attrs)
	c.Assert(err, IsNil)
	err = s.State.SetEnvironConfig(currentConfig)
	c.Assert(err, IsNil)

	// Next event.
	envConfig, ok = chanReadConfig(c, envConfigWatcher.Changes(), "environ config watcher")
	c.Assert(ok, Equals, true)
	c.Assert(envConfig, DeepEquals, currentConfig)

	// Check the watcher stops cleanly.
	err = envConfigWatcher.Stop()
	c.Assert(err, IsNil)

	_, ok = chanReadConfig(c, envConfigWatcher.Changes(), "environ config watcher")
	c.Assert(ok, Equals, false)
}

func (s *suite) TestSetDeadline(c *C) {
	stm, err := s.State.AddMachine("series", state.JobManageEnviron)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm)

	st := s.openAs(c, stm.Tag())
	defer func() {
		st.Close()
	}()

	// First try a ping without a deadline
	err = st.Ping()
	c.Assert(err, IsNil)

	// Set a very short deadline.
	err = st.SetDeadline(time.Now())
	c.Assert(err, IsNil)

	// Try a ping, should fail immediately with a timeout.
	err = st.Ping()
	c.Assert(err, NotNil)
	c.Assert(api.IsTimeout(err), Equals, true)

	// Now the connection is shut down, so reestablish.
	st.Close()
	st = s.openAs(c, stm.Tag())

	// Set a longer deadline.
	err = st.SetDeadline(time.Now().Add(10 * time.Second))
	c.Assert(err, IsNil)

	err = st.Ping()
	c.Assert(err, IsNil)

	// Set no deadline (default).
	err = st.SetDeadline(time.Time{})
	c.Assert(err, IsNil)

	err = st.Ping()
	c.Assert(err, IsNil)
}
