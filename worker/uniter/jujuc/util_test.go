// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils/set"
	"launchpad.net/juju-core/worker/uniter/jujuc"
)

func TestPackage(t *stdtesting.T) { gc.TestingT(t) }

func bufferBytes(stream io.Writer) []byte {
	return stream.(*bytes.Buffer).Bytes()
}

func bufferString(w io.Writer) string {
	return w.(*bytes.Buffer).String()
}

type ContextSuite struct {
	testing.BaseSuite
	rels map[int]*ContextRelation
}

func (s *ContextSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.rels = map[int]*ContextRelation{
		0: {
			id:   0,
			name: "peer0",
			units: map[string]Settings{
				"u/0": {"private-address": "u-0.testing.invalid"},
			},
		},
		1: {
			id:   1,
			name: "peer1",
			units: map[string]Settings{
				"u/0": {"private-address": "u-0.testing.invalid"},
			},
		},
	}
}

func (s *ContextSuite) GetHookContext(c *gc.C, relid int, remote string) *Context {
	if relid != -1 {
		_, found := s.rels[relid]
		c.Assert(found, gc.Equals, true)
	}
	return &Context{
		relid:  relid,
		remote: remote,
		rels:   s.rels,
	}
}

func setSettings(c *gc.C, ru *state.RelationUnit, settings map[string]interface{}) {
	node, err := ru.Settings()
	c.Assert(err, gc.IsNil)
	for _, k := range node.Keys() {
		node.Delete(k)
	}
	node.Update(settings)
	_, err = node.Write()
	c.Assert(err, gc.IsNil)
}

type Context struct {
	ports  set.Strings
	relid  int
	remote string
	rels   map[int]*ContextRelation
}

func (c *Context) UnitName() string {
	return "u/0"
}

func (c *Context) PublicAddress() (string, bool) {
	return "gimli.minecraft.testing.invalid", true
}

func (c *Context) PrivateAddress() (string, bool) {
	return "192.168.0.99", true
}

func (c *Context) OpenPort(protocol string, port int) error {
	c.ports.Add(fmt.Sprintf("%d/%s", port, protocol))
	return nil
}

func (c *Context) ClosePort(protocol string, port int) error {
	c.ports.Remove(fmt.Sprintf("%d/%s", port, protocol))
	return nil
}

func (c *Context) ConfigSettings() (charm.Settings, error) {
	return charm.Settings{
		"empty":               nil,
		"monsters":            false,
		"spline-reticulation": 45.0,
		"title":               "My Title",
		"username":            "admin001",
	}, nil
}

func (c *Context) HookRelation() (jujuc.ContextRelation, bool) {
	return c.Relation(c.relid)
}

func (c *Context) RemoteUnitName() (string, bool) {
	return c.remote, c.remote != ""
}

func (c *Context) Relation(id int) (jujuc.ContextRelation, bool) {
	r, found := c.rels[id]
	return r, found
}

func (c *Context) RelationIds() []int {
	ids := []int{}
	for id := range c.rels {
		ids = append(ids, id)
	}
	return ids
}

func (c *Context) OwnerTag() string {
	return "test-owner"
}

type ContextRelation struct {
	id    int
	name  string
	units map[string]Settings
}

func (r *ContextRelation) Id() int {
	return r.id
}

func (r *ContextRelation) Name() string {
	return r.name
}

func (r *ContextRelation) FakeId() string {
	return fmt.Sprintf("%s:%d", r.name, r.id)
}

func (r *ContextRelation) Settings() (jujuc.Settings, error) {
	return r.units["u/0"], nil
}

func (r *ContextRelation) UnitNames() []string {
	var s []string // initially nil to match the true context.
	for name := range r.units {
		s = append(s, name)
	}
	sort.Strings(s)
	return s
}

func (r *ContextRelation) ReadSettings(name string) (params.RelationSettings, error) {
	s, found := r.units[name]
	if !found {
		return nil, fmt.Errorf("unknown unit %s", name)
	}
	return s.Map(), nil
}

type Settings params.RelationSettings

func (s Settings) Get(k string) (interface{}, bool) {
	v, f := s[k]
	return v, f
}

func (s Settings) Set(k, v string) {
	s[k] = v
}

func (s Settings) Delete(k string) {
	delete(s, k)
}

func (s Settings) Map() params.RelationSettings {
	r := params.RelationSettings{}
	for k, v := range s {
		r[k] = v
	}
	return r
}
