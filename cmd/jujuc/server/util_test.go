package server_test

import (
	"bytes"
	"io"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/jujuc/server"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	stdtesting "testing"
)

func TestPackage(t *stdtesting.T) {
	coretesting.ZkTestPackage(t)
}

func dummyContext(c *C) *cmd.Context {
	return &cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}}
}

func bufferString(w io.Writer) string {
	return w.(*bytes.Buffer).String()
}

type UnitSuite struct {
	testing.StateSuite
	ctx     *server.ClientContext
	service *state.Service
	unit    *state.Unit
}

func (s *UnitSuite) SetUpTest(c *C) {
	s.StateSuite.SetUpTest(c)
	s.ctx = &server.ClientContext{
		Id:            "TestCtx",
		State:         s.State,
		LocalUnitName: "minecraft/0",
	}
	var err error
	s.service, err = s.State.AddService("minecraft", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, IsNil)
	s.unit, err = s.service.AddUnit()
	c.Assert(err, IsNil)
}

func (s *UnitSuite) AssertUnitCommand(c *C, name string) {
	ctx := &server.ClientContext{Id: "TestCtx", State: s.State}
	com, err := ctx.NewCommand(name)
	c.Assert(com, IsNil)
	c.Assert(err, ErrorMatches, "context TestCtx is not attached to a unit")

	ctx = &server.ClientContext{Id: "TestCtx", LocalUnitName: s.ctx.LocalUnitName}
	com, err = ctx.NewCommand(name)
	c.Assert(com, IsNil)
	c.Assert(err, ErrorMatches, "context TestCtx cannot access state")
}
