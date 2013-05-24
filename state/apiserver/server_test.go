// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"io"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/rpc"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/apiserver"
	coretesting "launchpad.net/juju-core/testing"
	"time"
)

func (s *suite) TestServerStopsOutstandingWatchMethod(c *C) {
	// Start our own instance of the server so we have
	// a handle on it to stop it.
	srv, err := apiserver.NewServer(s.State, "localhost:0", []byte(coretesting.ServerCert), []byte(coretesting.ServerKey))
	c.Assert(err, IsNil)

	// Set up state - add entities to watch.
	stm, err := s.State.AddMachine("series", state.JobManageEnviron)
	c.Assert(err, IsNil)
	err = stm.SetPassword("password")
	c.Assert(err, IsNil)

	// Note we can't use openAs because we're
	// not connecting to s.APIConn.
	st, err := api.Open(&api.Info{
		Tag:      stm.Tag(),
		Password: "password",
		Addrs:    []string{srv.Addr()},
		CACert:   []byte(coretesting.CACert),
	})
	c.Assert(err, IsNil)
	defer st.Close()

	m, err := st.Machine(stm.Id())
	c.Assert(err, IsNil)
	c.Assert(m.Id(), Equals, stm.Id())

	// Start one of each type of watcher concurrently.
	machineWatcher := m.Watch()
	machinesWatcher := st.WatchMachines()
	envConfigWatcher := st.WatchEnvironConfig()

	// Initial events for all watchers.
	// (We don't care about the actual data).
	ok := chanReadEmpty(c, machineWatcher.Changes(), "machine 0 watcher")
	c.Assert(ok, Equals, true)
	_, ok = chanReadStrings(c, machinesWatcher.Changes(), "machines watcher")
	c.Assert(ok, Equals, true)
	_, ok = chanReadConfig(c, envConfigWatcher.Changes(), "environ config watcher")
	c.Assert(ok, Equals, true)

	// Wait long enough for the Next request to be sent
	// so it's blocking on the server side.
	time.Sleep(50 * time.Millisecond)
	c.Logf("stopping server")
	err = srv.Stop()
	c.Assert(err, IsNil)
	c.Logf("server stopped")

	// Check each watcher was stopped.
	// (We don't care about the actual changes).
	ok = chanReadEmpty(c, machineWatcher.Changes(), "machine 0 watcher")
	c.Assert(ok, Equals, false)
	c.Logf("machine 0 watcher error is %v", machineWatcher.Err())
	c.Assert(api.ErrCode(machineWatcher.Err()), Equals, api.CodeStopped)

	_, ok = chanReadStrings(c, machinesWatcher.Changes(), "machines watcher")
	c.Assert(ok, Equals, false)
	c.Logf("machines watcher error is %v", machinesWatcher.Err())
	c.Assert(api.ErrCode(machinesWatcher.Err()), Equals, api.CodeStopped)

	_, ok = chanReadConfig(c, envConfigWatcher.Changes(), "environ config watcher")
	c.Assert(ok, Equals, false)
	c.Logf("environ config watcher error is %v", envConfigWatcher.Err())
	c.Assert(api.ErrCode(envConfigWatcher.Err()), Equals, api.CodeStopped)
}

func (s *suite) TestStop(c *C) {
	// Start our own instance of the server so we have
	// a handle on it to stop it.
	srv, err := apiserver.NewServer(s.State, "localhost:0", []byte(coretesting.ServerCert), []byte(coretesting.ServerKey))
	c.Assert(err, IsNil)

	stm, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = stm.SetProvisioned("foo", "fake_nonce")
	c.Assert(err, IsNil)
	err = stm.SetPassword("password")
	c.Assert(err, IsNil)

	// Note we can't use openAs because we're not connecting to
	// s.APIConn.
	st, err := api.Open(&api.Info{
		Tag:      stm.Tag(),
		Password: "password",
		Addrs:    []string{srv.Addr()},
		CACert:   []byte(coretesting.CACert),
	})
	c.Assert(err, IsNil)
	defer st.Close()

	m, err := st.Machine(stm.Id())
	c.Assert(err, IsNil)
	c.Assert(m.Id(), Equals, stm.Id())

	err = srv.Stop()
	c.Assert(err, IsNil)

	_, err = st.Machine(stm.Id())
	// The client has not necessarily seen the server shutdown yet,
	// so there are two possible errors.
	if err != rpc.ErrShutdown && err != io.ErrUnexpectedEOF {
		c.Fatalf("unexpected error from request: %v", err)
	}

	// Check it can be stopped twice.
	err = srv.Stop()
	c.Assert(err, IsNil)
}
