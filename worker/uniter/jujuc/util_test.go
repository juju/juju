package jujuc_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/worker/uniter/jujuc"
	"sort"
	"testing"
)

func TestPackage(t *testing.T) { TestingT(t) }

func dummyFlagSet() *gnuflag.FlagSet {
	f := gnuflag.NewFlagSet("", gnuflag.ContinueOnError)
	f.SetOutput(ioutil.Discard)
	return f
}

func dummyContext(c *C) *cmd.Context {
	return &cmd.Context{c.MkDir(), nil, &bytes.Buffer{}, &bytes.Buffer{}}
}

func bufferString(w io.Writer) string {
	return w.(*bytes.Buffer).String()
}

type ContextSuite struct {
	rels map[int]*ContextRelation
}

func (s *ContextSuite) SetUpTest(c *C) {
	s.rels = map[int]*ContextRelation{
		0: {
			id:   0,
			name: "peer0",
			units: map[string]Settings{
				"u/0": {"private-address": "u-0.example.com"},
			},
		},
		1: {
			id:   1,
			name: "peer1",
			units: map[string]Settings{
				"u/0": {"private-address": "u-0.example.com"},
			},
		},
	}
}

func (s *ContextSuite) GetHookContext(c *C, relid int, remote string) *Context {
	if relid != -1 {
		_, found := s.rels[relid]
		c.Assert(found, Equals, true)
	}
	return &Context{
		ports:  map[string]bool{},
		relid:  relid,
		remote: remote,
		rels:   s.rels,
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

type Context struct {
	ports  map[string]bool
	relid  int
	remote string
	rels   map[int]*ContextRelation
}

func (c *Context) UnitName() string {
	return "u/0"
}

func (c *Context) PublicAddress() (string, error) {
	return "gimli.minecraft.example.com", nil
}

func (c *Context) PrivateAddress() (string, error) {
	return "192.168.0.99", nil
}

func (c *Context) OpenPort(protocol string, port int) error {
	c.ports[fmt.Sprintf("%d/%s", port, protocol)] = true
	return nil
}

func (c *Context) ClosePort(protocol string, port int) error {
	delete(c.ports, fmt.Sprintf("%d/%s", port, protocol))
	return nil
}

func (c *Context) Config() (map[string]interface{}, error) {
	return map[string]interface{}{
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
	s := []string{}
	for name := range r.units {
		s = append(s, name)
	}
	sort.Strings(s)
	return s
}

func (r *ContextRelation) ReadSettings(name string) (map[string]interface{}, error) {
	s, found := r.units[name]
	if !found {
		return nil, fmt.Errorf("unknown unit %s", name)
	}
	return s.Map(), nil
}

type Settings map[string]interface{}

func (s Settings) Get(k string) (interface{}, bool) {
	v, f := s[k]
	return v, f
}

func (s Settings) Set(k string, v interface{}) {
	s[k] = v
}

func (s Settings) Delete(k string) {
	delete(s, k)
}

func (s Settings) Map() map[string]interface{} {
	r := map[string]interface{}{}
	for k, v := range s {
		r[k] = v
	}
	return r
}
