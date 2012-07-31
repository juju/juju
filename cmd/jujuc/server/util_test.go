package server_test

import (
	"bytes"
	"io"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
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

type HookContextSuite struct {
	testing.StateSuite
	ch       *state.Charm
	service  *state.Service
	unit     *state.Unit
	relunits map[int]*state.RelationUnit
	relctxs  map[int]*server.RelationContext
}

func (s *HookContextSuite) SetUpTest(c *C) {
	s.StateSuite.SetUpTest(c)
	s.ch = s.AddTestingCharm(c, "dummy")
	var err error
	s.service, err = s.State.AddService("u", s.ch)
	c.Assert(err, IsNil)
	s.unit = s.AddUnit(c)
	s.relunits = map[int]*state.RelationUnit{}
	s.relctxs = map[int]*server.RelationContext{}
	s.AddRelationContext(c, "peer0")
	s.AddRelationContext(c, "peer1")
}

func (s *HookContextSuite) AddUnit(c *C) *state.Unit {
	unit, err := s.service.AddUnit()
	c.Assert(err, IsNil)
	name := strings.Replace(unit.Name(), "/", "-", 1)
	err = unit.SetPrivateAddress(name + ".example.com")
	c.Assert(err, IsNil)
	return unit
}

func (s *HookContextSuite) AddRelationContext(c *C, name string) {
	ep := state.RelationEndpoint{
		s.service.Name(), "ifce", name, state.RolePeer, charm.ScopeGlobal,
	}
	rel, err := s.State.AddRelation(ep)
	c.Assert(err, IsNil)
	ru, err := rel.Unit(s.unit)
	c.Assert(err, IsNil)
	s.relunits[rel.Id()] = ru
	p, err := ru.Join()
	c.Assert(err, IsNil)
	err = p.Kill()
	c.Assert(err, IsNil)
	s.relctxs[rel.Id()] = server.NewRelationContext(ru, nil)
}

func (s *HookContextSuite) GetHookContext(c *C, relid int, remote string) *server.HookContext {
	if relid != -1 {
		_, found := s.relctxs[relid]
		c.Assert(found, Equals, true)
	}
	return &server.HookContext{
		Service:        s.service,
		Unit:           s.unit,
		Id:             "TestCtx",
		RelationId:     relid,
		RemoteUnitName: remote,
		Relations:      s.relctxs,
	}
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
