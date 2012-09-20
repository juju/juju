package server_test

import (
	"bytes"
	"io"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/jujuc/server"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"strings"
	stdtesting "testing"
)

func TestPackage(t *stdtesting.T) {
	coretesting.ZkTestPackage(t)
}

func dummyContext(c *C) *cmd.Context {
	return &cmd.Context{c.MkDir(), nil, &bytes.Buffer{}, &bytes.Buffer{}}
}

func bufferString(w io.Writer) string {
	return w.(*bytes.Buffer).String()
}

type HookContextSuite struct {
	testing.JujuConnSuite
	ch       *state.Charm
	service  *state.Service
	unit     *state.Unit
	relunits map[int]*state.RelationUnit
	relctxs  map[int]*server.RelationContext
}

func (s *HookContextSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
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
	err = ru.EnsureJoin()
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

func setSettings(c *C, ru *state.RelationUnit, settings map[string]interface{}) {
	node, err := ru.Settings()
	c.Assert(err, IsNil)
	for _, k := range node.Keys() {
		node.Delete(k)
	}
	node.Update(settings)
	_, err = node.Write()
	c.Assert(err, IsNil)
}
