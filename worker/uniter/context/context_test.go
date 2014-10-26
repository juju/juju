// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"os"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/network"
	"github.com/juju/juju/worker/uniter/context"
	"github.com/juju/juju/worker/uniter/context/jujuc"
	"github.com/juju/utils/exec"
)

type InterfaceSuite struct {
	HookContextSuite
}

var _ = gc.Suite(&InterfaceSuite{})

func (s *InterfaceSuite) GetContext(
	c *gc.C, relId int, remoteName string,
) jujuc.Context {
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	return s.HookContextSuite.getHookContext(
		c, uuid.String(), relId, remoteName, noProxies,
	)
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

func (s *InterfaceSuite) startProcess(c *gc.C) *os.Process {
	command := exec.RunParams{
		Commands: "sleep 10",
	}
	err := command.Run()
	c.Assert(err, gc.IsNil)
	p := command.Process()
	return p
}

func (s *InterfaceSuite) TestRequestRebootAfterHook(c *gc.C) {
	ctx := context.HookContext{}
	p := s.startProcess(c)
	s.AddCleanup(func(c *gc.C) { c.Assert(p.Kill(), gc.IsNil) })
	ctx.SetProcess(p)
	err := ctx.RequestReboot(jujuc.RebootAfterHook)
	c.Assert(err, gc.IsNil)
	priority := ctx.GetRebootPriority()
	c.Assert(priority, gc.Equals, jujuc.RebootAfterHook)
	c.Assert(processExists(p.Pid), jc.IsTrue)
}

func (s *InterfaceSuite) TestRequestRebootNow(c *gc.C) {
	ctx := context.HookContext{}
	p := s.startProcess(c)
	s.AddCleanup(func(c *gc.C) { p.Kill() })
	ctx.SetProcess(p)
	err := ctx.RequestReboot(jujuc.RebootNow)
	c.Assert(err, gc.IsNil)
	priority := ctx.GetRebootPriority()
	c.Assert(priority, gc.Equals, jujuc.RebootNow)
	c.Assert(processExists(p.Pid), jc.IsFalse)
}

func (s *InterfaceSuite) TestRequestRebootNowNoProcess(c *gc.C) {
	ctx := context.HookContext{}
	err := ctx.RequestReboot(jujuc.RebootNow)
	c.Assert(err, gc.ErrorMatches, "no process to kill")
	priority := ctx.GetRebootPriority()
	c.Assert(priority, gc.Equals, jujuc.RebootNow)
}
