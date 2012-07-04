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

type TruthErrorSuite struct{}

var _ = Suite(&TruthErrorSuite{})

var truthErrorTests = []struct {
	value interface{}
	err   error
}{
	{0, cmd.ErrSilent},
	{int8(0), cmd.ErrSilent},
	{int16(0), cmd.ErrSilent},
	{int32(0), cmd.ErrSilent},
	{int64(0), cmd.ErrSilent},
	{uint(0), cmd.ErrSilent},
	{uint8(0), cmd.ErrSilent},
	{uint16(0), cmd.ErrSilent},
	{uint32(0), cmd.ErrSilent},
	{uint64(0), cmd.ErrSilent},
	{uintptr(0), cmd.ErrSilent},
	{123, nil},
	{int8(123), nil},
	{int16(123), nil},
	{int32(123), nil},
	{int64(123), nil},
	{uint(123), nil},
	{uint8(123), nil},
	{uint16(123), nil},
	{uint32(123), nil},
	{uint64(123), nil},
	{uintptr(123), nil},
	{0.0, cmd.ErrSilent},
	{float32(0.0), cmd.ErrSilent},
	{123.45, nil},
	{float32(123.45), nil},
	{nil, cmd.ErrSilent},
	{"", cmd.ErrSilent},
	{"blah", nil},
	{true, nil},
	{false, cmd.ErrSilent},
	{[]string{}, cmd.ErrSilent},
	{[]string{""}, nil},
	{[]bool{}, cmd.ErrSilent},
	{[]bool{false}, nil},
	{map[string]string{}, cmd.ErrSilent},
	{map[string]string{"": ""}, nil},
	{map[bool]bool{}, cmd.ErrSilent},
	{map[bool]bool{false: false}, nil},
	{struct{ x bool }{false}, nil},
}

func (s *TruthErrorSuite) TestTruthError(c *C) {
	for _, t := range truthErrorTests {
		c.Assert(server.TruthError(t.value), Equals, t.err)
	}
}
