// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"io"
	gc "launchpad.net/gocheck"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/rpc"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver"
	coretesting "launchpad.net/juju-core/testing"
	stdtesting "testing"
	"time"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

var fastDialOpts = api.DialOpts{}

type serverSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&serverSuite{})

func (s *serverSuite) TestStop(c *gc.C) {
	// Start our own instance of the server so we have
	// a handle on it to stop it.
	srv, err := apiserver.NewServer(s.State, "localhost:0", []byte(coretesting.ServerCert), []byte(coretesting.ServerKey))
	c.Assert(err, gc.IsNil)
	defer srv.Stop()

	stm, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = stm.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	err = stm.SetPassword("password")
	c.Assert(err, gc.IsNil)

	// Note we can't use openAs because we're not connecting to
	// s.APIConn.
	apiInfo := &api.Info{
		Tag:      stm.Tag(),
		Password: "password",
		Nonce:    "fake_nonce",
		Addrs:    []string{srv.Addr()},
		CACert:   []byte(coretesting.CACert),
	}
	st, err := api.Open(apiInfo, fastDialOpts)
	c.Assert(err, gc.IsNil)
	defer st.Close()

	_, err = st.Machiner().Machine(stm.Tag())
	c.Assert(err, gc.IsNil)

	err = srv.Stop()
	c.Assert(err, gc.IsNil)

	_, err = st.Machiner().Machine(stm.Tag())
	// The client has not necessarily seen the server shutdown yet,
	// so there are two possible errors.
	if err != rpc.ErrShutdown && err != io.ErrUnexpectedEOF {
		c.Fatalf("unexpected error from request: %v", err)
	}

	// Check it can be stopped twice.
	err = srv.Stop()
	c.Assert(err, gc.IsNil)
}

func (s *serverSuite) TestOpenAsMachineErrors(c *gc.C) {
	stm, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = stm.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	err = stm.SetPassword("password")
	c.Assert(err, gc.IsNil)

	// This does almost exactly the same as OpenAPIAsMachine but checks
	// for failures instead.
	_, info, err := s.APIConn.Environ.StateInfo()
	info.Tag = stm.Tag()
	info.Password = "password"
	info.Nonce = "invalid-nonce"
	st, err := api.Open(info, fastDialOpts)
	c.Assert(err, gc.ErrorMatches, params.CodeNotProvisioned)
	c.Assert(st, gc.IsNil)

	// Try with empty nonce as well.
	info.Nonce = ""
	st, err = api.Open(info, fastDialOpts)
	c.Assert(err, gc.ErrorMatches, params.CodeNotProvisioned)
	c.Assert(st, gc.IsNil)

	// Finally, with the correct one succeeds.
	info.Nonce = "fake_nonce"
	st, err = api.Open(info, fastDialOpts)
	c.Assert(err, gc.IsNil)
	c.Assert(st, gc.NotNil)
	st.Close()

	// Now add another machine, intentionally unprovisioned.
	stm1, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = stm1.SetPassword("password")
	c.Assert(err, gc.IsNil)

	// Try connecting, it will fail.
	info.Tag = stm1.Tag()
	info.Nonce = ""
	st, err = api.Open(info, fastDialOpts)
	c.Assert(err, gc.ErrorMatches, params.CodeNotProvisioned)
	c.Assert(st, gc.IsNil)
}

func (s *serverSuite) TestMachineLoginStartsPinger(c *gc.C) {
	// Create a new machine to verify "agent alive" behavior.
	stm, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = stm.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	err = stm.SetPassword("password")
	c.Assert(err, gc.IsNil)

	// Not alive yet.
	s.State.Sync()
	alive, err := stm.AgentAlive()
	c.Assert(err, gc.IsNil)
	c.Assert(alive, gc.Equals, false)

	// Login as the machine agent of the created machine.
	st := s.OpenAPIAsMachine(c, stm.Tag(), "password", "fake_nonce")
	defer st.Close()

	// Make sure the pinger has started.
	s.State.Sync()
	stm.WaitAgentAlive(coretesting.LongWait)
	alive, err = stm.AgentAlive()
	c.Assert(err, gc.IsNil)
	c.Assert(alive, gc.Equals, true)

	// Now make sure it stops when connection is closed.
	c.Assert(st.Close(), gc.IsNil)

	// Sync, then wait for a bit to make sure the state is updated.
	s.State.Sync()
	<-time.After(coretesting.ShortWait)
	s.State.Sync()

	c.Assert(err, gc.IsNil)
	alive, err = stm.AgentAlive()
	c.Assert(err, gc.IsNil)
	c.Assert(alive, gc.Equals, false)
}
