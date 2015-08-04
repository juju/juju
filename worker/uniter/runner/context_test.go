// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	"os"
	"runtime"
	"syscall"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/exec"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type InterfaceSuite struct {
	HookContextSuite
	stub testing.Stub
}

var _ = gc.Suite(&InterfaceSuite{})

func (s *InterfaceSuite) TestUnitName(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	c.Assert(ctx.UnitName(), gc.Equals, "u/0")
}

func (s *InterfaceSuite) TestHookRelation(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	r, ok := ctx.HookRelation()
	c.Assert(ok, jc.IsFalse)
	c.Assert(r, gc.IsNil)
}

func (s *InterfaceSuite) TestRemoteUnitName(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	name, found := ctx.RemoteUnitName()
	c.Assert(found, jc.IsFalse)
	c.Assert(name, gc.Equals, "")
}

func (s *InterfaceSuite) TestRelationIds(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	relIds := ctx.RelationIds()
	c.Assert(relIds, gc.HasLen, 2)
	r, found := ctx.Relation(0)
	c.Assert(found, jc.IsTrue)
	c.Assert(r.Name(), gc.Equals, "db")
	c.Assert(r.FakeId(), gc.Equals, "db:0")
	r, found = ctx.Relation(123)
	c.Assert(found, jc.IsFalse)
	c.Assert(r, gc.IsNil)
}

func (s *InterfaceSuite) TestRelationContext(c *gc.C) {
	ctx := s.GetContext(c, 1, "")
	r, ok := ctx.HookRelation()
	c.Assert(ok, jc.IsTrue)
	c.Assert(r.Name(), gc.Equals, "db")
	c.Assert(r.FakeId(), gc.Equals, "db:1")
}

func (s *InterfaceSuite) TestRelationContextWithRemoteUnitName(c *gc.C) {
	ctx := s.GetContext(c, 1, "u/123")
	name, found := ctx.RemoteUnitName()
	c.Assert(found, jc.IsTrue)
	c.Assert(name, gc.Equals, "u/123")
}

func (s *InterfaceSuite) TestAddingMetricsWhenNotEnabledFails(c *gc.C) {
	ctx := s.GetContext(c, 1, "u/123")
	err := ctx.AddMetric("key", "123", time.Now())
	c.Assert(err, gc.ErrorMatches, "metrics disabled")
}

func (s *InterfaceSuite) TestAddingMetrics(c *gc.C) {
	uuid := utils.MustNewUUID()
	ctx := s.getMeteredHookContext(c, uuid.String(), -1, "", noProxies, true, s.metricsDefinition("key"), NewRealPaths(c))
	cleanup := runner.PatchMetricsRecorder(ctx, &StubMetricsRecorder{&s.stub})
	defer cleanup()

	now := time.Now()
	err := ctx.AddMetric("key", "123", now)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCalls(c,
		[]testing.StubCall{{
			FuncName: "AddMetric",
			Args:     []interface{}{"key", "123", now},
		}})
}

func (s *InterfaceSuite) TestAvailabilityZone(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	zone, ok := ctx.AvailabilityZone()
	c.Check(ok, jc.IsTrue)
	c.Check(zone, gc.Equals, "a-zone")
}

func (s *InterfaceSuite) TestUnitStatus(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	defer runner.PatchCachedStatus(ctx.(runner.Context), "maintenance", "working", map[string]interface{}{"hello": "world"})()
	status, err := ctx.UnitStatus()
	c.Check(err, jc.ErrorIsNil)
	c.Check(status.Status, gc.Equals, "maintenance")
	c.Check(status.Info, gc.Equals, "working")
	c.Check(status.Data, gc.DeepEquals, map[string]interface{}{"hello": "world"})
}

func (s *InterfaceSuite) TestSetUnitStatus(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	status := jujuc.StatusInfo{
		Status: "maintenance",
		Info:   "doing work",
	}
	err := ctx.SetUnitStatus(status)
	c.Check(err, jc.ErrorIsNil)
	unitStatus, err := ctx.UnitStatus()
	c.Check(err, jc.ErrorIsNil)
	c.Check(unitStatus.Status, gc.Equals, "maintenance")
	c.Check(unitStatus.Info, gc.Equals, "doing work")
	c.Check(unitStatus.Data, gc.DeepEquals, map[string]interface{}{})
}

func (s *InterfaceSuite) TestSetUnitStatusUpdatesFlag(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	c.Assert(ctx.(runner.Context).HasExecutionSetUnitStatus(), jc.IsFalse)
	status := jujuc.StatusInfo{
		Status: "maintenance",
		Info:   "doing work",
	}
	err := ctx.SetUnitStatus(status)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(ctx.(runner.Context).HasExecutionSetUnitStatus(), jc.IsTrue)
}

func (s *InterfaceSuite) TestUnitStatusCaching(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	status, err := ctx.UnitStatus()
	c.Check(err, jc.ErrorIsNil)
	c.Check(status.Status, gc.Equals, "unknown")
	c.Check(status.Data, gc.DeepEquals, map[string]interface{}{})

	// Change remote state.
	err = s.unit.SetStatus(state.StatusActive, "it works", nil)
	c.Assert(err, jc.ErrorIsNil)

	// Local view is unchanged.
	status, err = ctx.UnitStatus()
	c.Check(err, jc.ErrorIsNil)
	c.Check(status.Status, gc.Equals, "unknown")
	c.Check(status.Data, gc.DeepEquals, map[string]interface{}{})
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
	err := s.machine.SetProviderAddresses(
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
	c.Check(actionData.Failed, jc.IsTrue)
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

func (s *InterfaceSuite) TestStorageAddConstraints(c *gc.C) {
	expected := map[string][]params.StorageConstraints{
		"data": []params.StorageConstraints{
			params.StorageConstraints{},
		},
	}

	ctx := runner.HookContext{}
	addStorageToContext(&ctx, "data", params.StorageConstraints{})
	assertStorageAddInContext(c, ctx, expected)
}

var two = uint64(2)

func (s *InterfaceSuite) TestStorageAddConstraintsSameStorage(c *gc.C) {
	expected := map[string][]params.StorageConstraints{
		"data": []params.StorageConstraints{
			params.StorageConstraints{},
			params.StorageConstraints{Count: &two},
		},
	}

	ctx := runner.HookContext{}
	addStorageToContext(&ctx, "data", params.StorageConstraints{})
	addStorageToContext(&ctx, "data", params.StorageConstraints{Count: &two})
	assertStorageAddInContext(c, ctx, expected)
}

func (s *InterfaceSuite) TestStorageAddConstraintsDifferentStorage(c *gc.C) {
	expected := map[string][]params.StorageConstraints{
		"data": []params.StorageConstraints{params.StorageConstraints{}},
		"diff": []params.StorageConstraints{
			params.StorageConstraints{Count: &two}},
	}

	ctx := runner.HookContext{}
	addStorageToContext(&ctx, "data", params.StorageConstraints{})
	addStorageToContext(&ctx, "diff", params.StorageConstraints{Count: &two})
	assertStorageAddInContext(c, ctx, expected)
}

func addStorageToContext(ctx *runner.HookContext,
	name string,
	cons params.StorageConstraints,
) {
	addOne := map[string]params.StorageConstraints{name: cons}
	ctx.AddUnitStorage(addOne)
}

func assertStorageAddInContext(c *gc.C,
	ctx runner.HookContext, expected map[string][]params.StorageConstraints,
) {
	obtained := ctx.StorageAddConstraints()
	c.Assert(len(obtained), gc.Equals, len(expected))
	for k, v := range obtained {
		c.Assert(v, jc.SameContents, expected[k])
	}
}
