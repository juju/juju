// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"fmt"
	"net"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/observer/fakeobserver"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/workertest"
)

const (
	ownerPassword = "very very secret"
)

type apiserverBaseSuite struct {
	statetesting.StateSuite
}

func (s *apiserverBaseSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	u, err := s.State.User(s.Owner)
	c.Assert(err, jc.ErrorIsNil)
	err = u.SetPassword(ownerPassword)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *apiserverBaseSuite) sampleConfig(c *gc.C) apiserver.ServerConfig {
	return apiserver.ServerConfig{
		Clock:       clock.WallClock,
		Cert:        []byte(coretesting.ServerCert),
		Key:         []byte(coretesting.ServerKey),
		Tag:         names.NewMachineTag("0"),
		LogDir:      c.MkDir(),
		NewObserver: func() observer.Observer { return &fakeobserver.Instance{} },
	}
}

func (s *apiserverBaseSuite) newServerNoCleanup(c *gc.C, config apiserver.ServerConfig) *apiserver.Server {
	listener, err := net.Listen("tcp", ":0")
	c.Assert(err, jc.ErrorIsNil)
	srv, err := apiserver.NewServer(s.State, listener, config)
	c.Assert(err, jc.ErrorIsNil)
	return srv
}

func (s *apiserverBaseSuite) newServer(c *gc.C, config apiserver.ServerConfig) *apiserver.Server {
	srv := s.newServerNoCleanup(c, config)
	s.AddCleanup(func(c *gc.C) {
		workertest.CleanKill(c, srv)
	})
	return srv
}

func (s *apiserverBaseSuite) newServerDirtyKill(c *gc.C, config apiserver.ServerConfig) *apiserver.Server {
	srv := s.newServerNoCleanup(c, config)
	s.AddCleanup(func(c *gc.C) {
		workertest.DirtyKill(c, srv)
	})
	return srv
}

// APIInfo returns an info struct that has the server's address and ca-cert
// populated.
func (s *apiserverBaseSuite) APIInfo(server *apiserver.Server) *api.Info {
	address := fmt.Sprintf("localhost:%d", server.Addr().Port)
	return &api.Info{
		Addrs:  []string{address},
		CACert: coretesting.CACert,
	}
}

func (s *apiserverBaseSuite) openAPIAs(c *gc.C, srv *apiserver.Server, tag names.Tag, password, nonce string, controllerOnly bool) api.Connection {
	apiInfo := s.APIInfo(srv)
	apiInfo.Tag = tag
	apiInfo.Password = password
	apiInfo.Nonce = nonce
	if !controllerOnly {
		apiInfo.ModelTag = s.State.ModelTag()
	}
	conn, err := api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(conn, gc.NotNil)
	s.AddCleanup(func(c *gc.C) {
		conn.Close()
	})
	return conn
}

// OpenAPIAsNewMachine creates a new client connection logging in as the
// controller owner. The returned api.Connection should not be closed by the
// caller as a cleanup function has been registered to do that.
func (s *apiserverBaseSuite) OpenAPIAsAdmin(c *gc.C, srv *apiserver.Server) api.Connection {
	return s.openAPIAs(c, srv, s.Owner, ownerPassword, "", false)
}

// OpenAPIAsNewMachine creates a new machine entry that lives in system state,
// and then uses that to open the API. The returned api.Connection should not be
// closed by the caller as a cleanup function has been registered to do that.
// The machine will run the supplied jobs; if none are given, JobHostUnits is assumed.
func (s *apiserverBaseSuite) OpenAPIAsNewMachine(c *gc.C, srv *apiserver.Server, jobs ...state.MachineJob) (api.Connection, *state.Machine) {
	if len(jobs) == 0 {
		jobs = []state.MachineJob{state.JobHostUnits}
	}
	machine, err := s.State.AddMachine("quantal", jobs...)
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	return s.openAPIAs(c, srv, machine.Tag(), password, "fake_nonce", false), machine
}
