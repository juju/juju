// Copyright 2012, 2013 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/context/jujuc"
)

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
		c.Assert(found, jc.IsTrue)
	}
	return &Context{
		relid:  relid,
		remote: remote,
		rels:   s.rels,
	}
}

func setSettings(c *gc.C, ru *state.RelationUnit, settings map[string]interface{}) {
	node, err := ru.Settings()
	c.Assert(err, jc.ErrorIsNil)
	for _, k := range node.Keys() {
		node.Delete(k)
	}
	node.Update(settings)
	_, err = node.Write()
	c.Assert(err, jc.ErrorIsNil)
}

type Context struct {
	ports          []network.PortRange
	relid          int
	remote         string
	rels           map[int]*ContextRelation
	metrics        []jujuc.Metric
	canAddMetrics  bool
	rebootPriority jujuc.RebootPriority
	shouldError    bool
}

func (c *Context) AddMetric(key, value string, created time.Time) error {
	if !c.canAddMetrics {
		return fmt.Errorf("metrics disabled")
	}
	c.metrics = append(c.metrics, jujuc.Metric{key, value, created})
	return nil
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

func (c *Context) OpenPorts(protocol string, fromPort, toPort int) error {
	c.ports = append(c.ports, network.PortRange{
		Protocol: protocol,
		FromPort: fromPort,
		ToPort:   toPort,
	})
	network.SortPortRanges(c.ports)
	return nil
}

func (c *Context) ClosePorts(protocol string, fromPort, toPort int) error {
	portRange := network.PortRange{
		Protocol: protocol,
		FromPort: fromPort,
		ToPort:   toPort,
	}
	for i, port := range c.ports {
		if port == portRange {
			c.ports = append(c.ports[:i], c.ports[i+1:]...)
			break
		}
	}
	network.SortPortRanges(c.ports)
	return nil
}

func (c *Context) OpenedPorts() []network.PortRange {
	return c.ports
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

func (c *Context) ActionParams() (map[string]interface{}, error) {
	return nil, fmt.Errorf("not running an action")
}

func (c *Context) UpdateActionResults(keys []string, value string) error {
	return fmt.Errorf("not running an action")
}

func (c *Context) SetActionFailed() error {
	return fmt.Errorf("not running an action")
}

func (c *Context) SetActionMessage(message string) error {
	return fmt.Errorf("not running an action")
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

func (c *Context) RequestReboot(priority jujuc.RebootPriority) error {
	c.rebootPriority = priority
	if c.shouldError {
		return fmt.Errorf("RequestReboot error!")
	} else {
		return nil
	}
}

func cmdString(cmd string) string {
	return cmd + jujuc.CmdSuffix
}
