// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/worker/uniter/context"
	"github.com/juju/juju/worker/uniter/jujuc"
)

type InterfaceSuite struct {
	HookContextSuite
}

var _ = gc.Suite(&InterfaceSuite{})

func (s *InterfaceSuite) GetContext(c *gc.C, relId int,
	remoteName string) jujuc.Context {
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	return s.HookContextSuite.getHookContext(c, uuid.String(), relId, remoteName, noProxies, false)
}

func (s *InterfaceSuite) TestUtils(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	c.Assert(ctx.UnitName(), gc.Equals, "u/0")
	r, found := ctx.HookRelation()
	c.Assert(found, jc.IsFalse)
	c.Assert(r, gc.IsNil)
	name, found := ctx.RemoteUnitName()
	c.Assert(found, jc.IsFalse)
	c.Assert(name, gc.Equals, "")
	c.Assert(ctx.RelationIds(), gc.HasLen, 2)
	r, found = ctx.Relation(0)
	c.Assert(found, jc.IsTrue)
	c.Assert(r.Name(), gc.Equals, "db")
	c.Assert(r.FakeId(), gc.Equals, "db:0")
	r, found = ctx.Relation(123)
	c.Assert(found, jc.IsFalse)
	c.Assert(r, gc.IsNil)

	ctx = s.GetContext(c, 1, "")
	r, found = ctx.HookRelation()
	c.Assert(found, jc.IsTrue)
	c.Assert(r.Name(), gc.Equals, "db")
	c.Assert(r.FakeId(), gc.Equals, "db:1")

	ctx = s.GetContext(c, 1, "u/123")
	name, found = ctx.RemoteUnitName()
	c.Assert(found, jc.IsTrue)
	c.Assert(name, gc.Equals, "u/123")
}

func (s *InterfaceSuite) TestUnitCaching(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	pr, ok := ctx.PrivateAddress()
	c.Assert(ok, jc.IsTrue)
	c.Assert(pr, gc.Equals, "u-0.testing.invalid")
	pa, ok := ctx.PublicAddress()
	c.Assert(ok, jc.IsTrue)
	// Initially the public address is the same as the private address since
	// the "most public" address is chosen.
	c.Assert(pr, gc.Equals, pa)

	// Change remote state.
	err := s.machine.SetAddresses(
		network.NewAddress("blah.testing.invalid", network.ScopePublic))
	c.Assert(err, gc.IsNil)

	// Local view is unchanged.
	pr, ok = ctx.PrivateAddress()
	c.Assert(ok, jc.IsTrue)
	c.Assert(pr, gc.Equals, "u-0.testing.invalid")
	pa, ok = ctx.PublicAddress()
	c.Assert(ok, jc.IsTrue)
	c.Assert(pr, gc.Equals, pa)
}

func (s *InterfaceSuite) TestConfigCaching(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	settings, err := ctx.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"blog-title": "My Title"})

	// Change remote config.
	err = s.service.UpdateConfigSettings(charm.Settings{
		"blog-title": "Something Else",
	})
	c.Assert(err, gc.IsNil)

	// Local view is not changed.
	settings, err = ctx.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"blog-title": "My Title"})
}

func (s *InterfaceSuite) TestValidatePortRange(c *gc.C) {
	tests := []struct {
		about     string
		proto     string
		ports     []int
		portRange network.PortRange
		expectErr string
	}{{
		about:     "invalid range - 0-0/tcp",
		proto:     "tcp",
		ports:     []int{0, 0},
		expectErr: "invalid port range 0-0/tcp",
	}, {
		about:     "invalid range - 0-1/tcp",
		proto:     "tcp",
		ports:     []int{0, 1},
		expectErr: "invalid port range 0-1/tcp",
	}, {
		about:     "invalid range - -1-1/tcp",
		proto:     "tcp",
		ports:     []int{-1, 1},
		expectErr: "invalid port range -1-1/tcp",
	}, {
		about:     "invalid range - 1-99999/tcp",
		proto:     "tcp",
		ports:     []int{1, 99999},
		expectErr: "invalid port range 1-99999/tcp",
	}, {
		about:     "invalid range - 88888-99999/tcp",
		proto:     "tcp",
		ports:     []int{88888, 99999},
		expectErr: "invalid port range 88888-99999/tcp",
	}, {
		about:     "invalid protocol - 1-65535/foo",
		proto:     "foo",
		ports:     []int{1, 65535},
		expectErr: `invalid protocol "foo", expected "tcp" or "udp"`,
	}, {
		about: "valid range - 100-200/udp",
		proto: "UDP",
		ports: []int{100, 200},
		portRange: network.PortRange{
			FromPort: 100,
			ToPort:   200,
			Protocol: "udp",
		},
	}, {
		about: "valid single port - 100/tcp",
		proto: "TCP",
		ports: []int{100, 100},
		portRange: network.PortRange{
			FromPort: 100,
			ToPort:   100,
			Protocol: "tcp",
		},
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.about)
		portRange, err := context.ValidatePortRange(
			test.proto,
			test.ports[0],
			test.ports[1],
		)
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
			c.Check(portRange, jc.DeepEquals, network.PortRange{})
		} else {
			c.Check(err, gc.IsNil)
			c.Check(portRange, jc.DeepEquals, test.portRange)
		}
	}
}

func makeMachinePorts(
	unitName, proto string, fromPort, toPort int,
) map[network.PortRange]params.RelationUnit {
	result := make(map[network.PortRange]params.RelationUnit)
	portRange := network.PortRange{
		FromPort: fromPort,
		ToPort:   toPort,
		Protocol: proto,
	}
	unitTag := ""
	if unitName != "invalid" {
		unitTag = names.NewUnitTag(unitName).String()
	} else {
		unitTag = unitName
	}
	result[portRange] = params.RelationUnit{
		Unit: unitTag,
	}
	return result
}

func makePendingPorts(
	proto string, fromPort, toPort int, shouldOpen bool,
) map[context.PortRange]context.PortRangeInfo {
	result := make(map[context.PortRange]context.PortRangeInfo)
	portRange := network.PortRange{
		FromPort: fromPort,
		ToPort:   toPort,
		Protocol: proto,
	}
	key := context.PortRange{
		Ports:      portRange,
		RelationId: -1,
	}
	result[key] = context.PortRangeInfo{
		ShouldOpen: shouldOpen,
	}
	return result
}

type portsTest struct {
	about         string
	proto         string
	ports         []int
	machinePorts  map[network.PortRange]params.RelationUnit
	pendingPorts  map[context.PortRange]context.PortRangeInfo
	expectErr     string
	expectPending map[context.PortRange]context.PortRangeInfo
}

func (p portsTest) withDefaults(proto string, fromPort, toPort int) portsTest {
	if p.proto == "" {
		p.proto = proto
	}
	if len(p.ports) != 2 {
		p.ports = []int{fromPort, toPort}
	}
	if p.pendingPorts == nil {
		p.pendingPorts = make(map[context.PortRange]context.PortRangeInfo)
	}
	return p
}

func (s *InterfaceSuite) TestTryOpenPorts(c *gc.C) {
	tests := []portsTest{{
		about:     "invalid port range",
		ports:     []int{0, 0},
		expectErr: "invalid port range 0-0/tcp",
	}, {
		about:     "invalid protocol - 10-20/foo",
		proto:     "foo",
		expectErr: `invalid protocol "foo", expected "tcp" or "udp"`,
	}, {
		about:         "open a new range (no machine ports yet)",
		expectPending: makePendingPorts("tcp", 10, 20, true),
	}, {
		about:         "open an existing range (ignored)",
		machinePorts:  makeMachinePorts("u/0", "tcp", 10, 20),
		expectPending: map[context.PortRange]context.PortRangeInfo{},
	}, {
		about:         "open a range pending to be closed already",
		pendingPorts:  makePendingPorts("tcp", 10, 20, false),
		expectPending: makePendingPorts("tcp", 10, 20, true),
	}, {
		about:         "open a range pending to be opened already (ignored)",
		pendingPorts:  makePendingPorts("tcp", 10, 20, true),
		expectPending: makePendingPorts("tcp", 10, 20, true),
	}, {
		about:        "try opening a range when machine ports has invalid unit tag",
		machinePorts: makeMachinePorts("invalid", "tcp", 80, 90),
		expectErr:    `machine ports 80-90/tcp contain invalid unit tag: "invalid" is not a valid tag`,
	}, {
		about:        "try opening a range conflicting with another unit",
		machinePorts: makeMachinePorts("u/1", "tcp", 10, 20),
		expectErr:    `cannot open 10-20/tcp \(unit "u/0"\): conflicts with existing 10-20/tcp \(unit "u/1"\)`,
	}, {
		about:         "open a range conflicting with the same unit (ignored)",
		machinePorts:  makeMachinePorts("u/0", "tcp", 10, 20),
		expectPending: map[context.PortRange]context.PortRangeInfo{},
	}, {
		about:        "try opening a range conflicting with another pending range",
		pendingPorts: makePendingPorts("tcp", 5, 25, true),
		expectErr:    `cannot open 10-20/tcp \(unit "u/0"\): conflicts with 5-25/tcp requested earlier`,
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.about)

		test = test.withDefaults("tcp", 10, 20)
		err := context.TryOpenPorts(
			test.proto,
			test.ports[0],
			test.ports[1],
			names.NewUnitTag("u/0"),
			test.machinePorts,
			test.pendingPorts,
		)
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
		} else {
			c.Check(err, gc.IsNil)
			c.Check(test.pendingPorts, jc.DeepEquals, test.expectPending)
		}
	}
}

func (s *InterfaceSuite) TestTryClosePorts(c *gc.C) {
	tests := []portsTest{{
		about:     "invalid port range",
		ports:     []int{0, 0},
		expectErr: "invalid port range 0-0/tcp",
	}, {
		about:     "invalid protocol - 10-20/foo",
		proto:     "foo",
		expectErr: `invalid protocol "foo", expected "tcp" or "udp"`,
	}, {
		about:         "close a new range (no machine ports yet; ignored)",
		expectPending: map[context.PortRange]context.PortRangeInfo{},
	}, {
		about:         "close an existing range",
		machinePorts:  makeMachinePorts("u/0", "tcp", 10, 20),
		expectPending: makePendingPorts("tcp", 10, 20, false),
	}, {
		about:         "close a range pending to be opened already (removed from pending)",
		pendingPorts:  makePendingPorts("tcp", 10, 20, true),
		expectPending: map[context.PortRange]context.PortRangeInfo{},
	}, {
		about:         "close a range pending to be closed already (ignored)",
		pendingPorts:  makePendingPorts("tcp", 10, 20, false),
		expectPending: makePendingPorts("tcp", 10, 20, false),
	}, {
		about:        "try closing an existing range when machine ports has invalid unit tag",
		machinePorts: makeMachinePorts("invalid", "tcp", 10, 20),
		expectErr:    `machine ports 10-20/tcp contain invalid unit tag: "invalid" is not a valid tag`,
	}, {
		about:        "try closing a range of another unit",
		machinePorts: makeMachinePorts("u/1", "tcp", 10, 20),
		expectErr:    `cannot close 10-20/tcp \(opened by "u/1"\) from "u/0"`,
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.about)

		test = test.withDefaults("tcp", 10, 20)
		err := context.TryClosePorts(
			test.proto,
			test.ports[0],
			test.ports[1],
			names.NewUnitTag("u/0"),
			test.machinePorts,
			test.pendingPorts,
		)
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
		} else {
			c.Check(err, gc.IsNil)
			c.Check(test.pendingPorts, jc.DeepEquals, test.expectPending)
		}
	}
}

// TestNonActionCallsToActionMethodsFail does exactly what its name says:
// it simply makes sure that Action-related calls to HookContexts with a nil
// actionData member error out correctly.
func (s *InterfaceSuite) TestNonActionCallsToActionMethodsFail(c *gc.C) {
	ctx := context.HookContext{}
	_, err := ctx.ActionParams()
	c.Check(err, gc.ErrorMatches, "not running an action")
	err = ctx.SetActionFailed()
	c.Check(err, gc.ErrorMatches, "not running an action")
	err = ctx.SetActionMessage("foo")
	c.Check(err, gc.ErrorMatches, "not running an action")
	err = ctx.RunAction("asdf", "fdsa", "qwerty", "uiop")
	c.Check(err, gc.ErrorMatches, "not running an action")
	err = ctx.UpdateActionResults([]string{"1", "2", "3"}, "value")
	c.Check(err, gc.ErrorMatches, "not running an action")
}

// TestUpdateActionResults demonstrates that UpdateActionResults functions
// as expected.
func (s *InterfaceSuite) TestUpdateActionResults(c *gc.C) {
	tests := []struct {
		initial  map[string]interface{}
		keys     []string
		value    string
		expected map[string]interface{}
	}{{
		initial: map[string]interface{}{},
		keys:    []string{"foo"},
		value:   "bar",
		expected: map[string]interface{}{
			"foo": "bar",
		},
	}, {
		initial: map[string]interface{}{
			"foo": "bar",
		},
		keys:  []string{"foo", "bar"},
		value: "baz",
		expected: map[string]interface{}{
			"foo": map[string]interface{}{
				"bar": "baz",
			},
		},
	}, {
		initial: map[string]interface{}{
			"foo": map[string]interface{}{
				"bar": "baz",
			},
		},
		keys:  []string{"foo"},
		value: "bar",
		expected: map[string]interface{}{
			"foo": "bar",
		},
	}}

	for i, t := range tests {
		c.Logf("UpdateActionResults test %d: %#v: %#v", i, t.keys, t.value)
		hctx := context.GetStubActionContext(t.initial)
		err := hctx.UpdateActionResults(t.keys, t.value)
		c.Assert(err, gc.IsNil)
		c.Check(hctx.ActionResultsMap(), jc.DeepEquals, t.expected)
	}
}

// TestSetActionFailed ensures SetActionFailed works properly.
func (s *InterfaceSuite) TestSetActionFailed(c *gc.C) {
	hctx := context.GetStubActionContext(nil)
	err := hctx.SetActionFailed()
	c.Assert(err, gc.IsNil)
	c.Check(hctx.ActionFailed(), jc.IsTrue)
}

// TestSetActionMessage ensures SetActionMessage works properly.
func (s *InterfaceSuite) TestSetActionMessage(c *gc.C) {
	hctx := context.GetStubActionContext(nil)
	err := hctx.SetActionMessage("because reasons")
	c.Assert(err, gc.IsNil)
	message, err := hctx.ActionMessage()
	c.Check(err, gc.IsNil)
	c.Check(message, gc.Equals, "because reasons")
}

func convertSettings(settings params.RelationSettings) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range settings {
		result[k] = v
	}
	return result
}

func convertMap(settingsMap map[string]interface{}) params.RelationSettings {
	result := make(params.RelationSettings)
	for k, v := range settingsMap {
		result[k] = v.(string)
	}
	return result
}
