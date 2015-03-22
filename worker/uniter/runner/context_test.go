// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	"os"
	"runtime"
	"syscall"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/exec"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/network"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type InterfaceSuite struct {
	HookContextSuite
}

var _ = gc.Suite(&InterfaceSuite{})

func (s *InterfaceSuite) GetContext(
	c *gc.C, relId int, remoteName string,
) jujuc.Context {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
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

func (s *InterfaceSuite) TestAvailabilityZone(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	zone, ok := ctx.AvailabilityZone()
	c.Check(ok, jc.IsTrue)
	c.Check(zone, gc.Equals, "a-zone")
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
		network.NewScopedAddress("blah.testing.invalid", network.ScopePublic),
	)
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"blog-title": "My Title"})

	// Change remote config.
	err = s.service.UpdateConfigSettings(charm.Settings{
		"blog-title": "Something Else",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Local view is not changed.
	settings, err = ctx.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"blog-title": "My Title"})
}

// TestNonActionCallsToActionMethodsFail does exactly what its name says:
// it simply makes sure that Action-related calls to HookContexts with a nil
// actionData member error out correctly.
func (s *InterfaceSuite) TestNonActionCallsToActionMethodsFail(c *gc.C) {
	ctx := runner.HookContext{}
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
		hctx := runner.GetStubActionContext(t.initial)
		err := hctx.UpdateActionResults(t.keys, t.value)
		c.Assert(err, jc.ErrorIsNil)
		actionData, err := hctx.ActionData()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(actionData.ResultsMap, jc.DeepEquals, t.expected)
	}
}

// TestSetActionFailed ensures SetActionFailed works properly.
func (s *InterfaceSuite) TestSetActionFailed(c *gc.C) {
	hctx := runner.GetStubActionContext(nil)
	err := hctx.SetActionFailed()
	c.Assert(err, jc.ErrorIsNil)
	actionData, err := hctx.ActionData()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(actionData.ActionFailed, jc.IsTrue)
}

// TestSetActionMessage ensures SetActionMessage works properly.
func (s *InterfaceSuite) TestSetActionMessage(c *gc.C) {
	hctx := runner.GetStubActionContext(nil)
	err := hctx.SetActionMessage("because reasons")
	c.Assert(err, jc.ErrorIsNil)
	actionData, err := hctx.ActionData()
	c.Check(err, jc.ErrorIsNil)
	c.Check(actionData.ResultsMessage, gc.Equals, "because reasons")
}

func (s *InterfaceSuite) startProcess(c *gc.C) *os.Process {
	command := exec.RunParams{
		Commands: "trap 'exit 0' SIGTERM; while true;do sleep 1;done",
	}
	err := command.Run()
	c.Assert(err, jc.ErrorIsNil)
	p := command.Process()
	s.AddCleanup(func(c *gc.C) { p.Kill() })
	return p
}

func (s *InterfaceSuite) TestRequestRebootAfterHook(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: Cannot send sigterm on windows")
	}
	ctx := runner.HookContext{}
	p := s.startProcess(c)
	ctx.SetProcess(p)
	err := ctx.RequestReboot(jujuc.RebootAfterHook)
	c.Assert(err, jc.ErrorIsNil)
	err = p.Signal(syscall.SIGTERM)
	c.Assert(err, jc.ErrorIsNil)
	_, err = p.Wait()
	c.Assert(err, jc.ErrorIsNil)
	priority := ctx.GetRebootPriority()
	c.Assert(priority, gc.Equals, jujuc.RebootAfterHook)
}

func (s *InterfaceSuite) TestRequestRebootNow(c *gc.C) {
	ctx := runner.HookContext{}
	p := s.startProcess(c)
	ctx.SetProcess(p)
	go func() {
		_, err := p.Wait()
		c.Assert(err, jc.ErrorIsNil)
	}()
	err := ctx.RequestReboot(jujuc.RebootNow)
	c.Assert(err, jc.ErrorIsNil)
	priority := ctx.GetRebootPriority()
	c.Assert(priority, gc.Equals, jujuc.RebootNow)
}

func (s *InterfaceSuite) TestRequestRebootNowNoProcess(c *gc.C) {
	// A normal hook run or a juju-run command will record the *os.Process
	// object of the running command, in HookContext. When requesting a
	// reboot with the --now flag, the process is killed and only
	// then will we set the reboot priority. This test basically simulates
	// the case when the process calling juju-reboot is not recorded.
	ctx := runner.HookContext{}
	err := ctx.RequestReboot(jujuc.RebootNow)
	c.Assert(err, gc.ErrorMatches, "no process to kill")
	priority := ctx.GetRebootPriority()
	c.Assert(priority, gc.Equals, jujuc.RebootNow)
}
