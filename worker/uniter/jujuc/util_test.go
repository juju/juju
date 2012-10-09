package jujuc_test

import (
	"bytes"
	"io"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/uniter"
	"strings"
	stdtesting "testing"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

func dummyContext(c *C) *cmd.Context {
	return &cmd.Context{c.MkDir(), nil, &bytes.Buffer{}, &bytes.Buffer{}}
}

func bufferString(w io.Writer) string {
	return w.(*bytes.Buffer).String()
}

// HookContextSuite is due for replacement; a forthcoming branch will
// allow us to run jujuc tests without using the state package at all.
type HookContextSuite struct {
	testing.JujuConnSuite
	ch       *state.Charm
	service  *state.Service
	unit     *state.Unit
	relunits map[int]*state.RelationUnit
	relctxs  map[int]*uniter.ContextRelation
}

func (s *HookContextSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
	s.ch = s.AddTestingCharm(c, "dummy")
	var err error
	s.service, err = s.State.AddService("u", s.ch)
	c.Assert(err, IsNil)
	s.unit = s.AddUnit(c)
	s.relunits = map[int]*state.RelationUnit{}
	s.relctxs = map[int]*uniter.ContextRelation{}
	s.AddContextRelation(c, "peer0")
	s.AddContextRelation(c, "peer1")
}

func (s *HookContextSuite) AddUnit(c *C) *state.Unit {
	unit, err := s.service.AddUnit()
	c.Assert(err, IsNil)
	name := strings.Replace(unit.Name(), "/", "-", 1)
	err = unit.SetPrivateAddress(name + ".example.com")
	c.Assert(err, IsNil)
	return unit
}

func (s *HookContextSuite) AddContextRelation(c *C, name string) {
	ep := state.RelationEndpoint{
		s.service.Name(), "ifce", name, state.RolePeer, charm.ScopeGlobal,
	}
	rel, err := s.State.AddRelation(ep)
	c.Assert(err, IsNil)
	ru, err := rel.Unit(s.unit)
	c.Assert(err, IsNil)
	s.relunits[rel.Id()] = ru
	err = ru.EnterScope()
	c.Assert(err, IsNil)
	s.relctxs[rel.Id()] = uniter.NewContextRelation(ru, nil)
}

func (s *HookContextSuite) GetHookContext(c *C, relid int, remote string) *uniter.HookContext {
	if relid != -1 {
		_, found := s.relctxs[relid]
		c.Assert(found, Equals, true)
	}
	return &uniter.HookContext{
		Service:         s.service,
		Unit:            s.unit,
		Id:              "TestCtx",
		RelationId:      relid,
		RemoteUnitName_: remote,
		Relations:       s.relctxs,
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
