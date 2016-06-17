// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"errors"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/modelcmd"
	_ "github.com/juju/juju/juju"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	coretesting "github.com/juju/juju/testing"
)

type SwitchSimpleSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	testing.Stub
	store     *jujuclienttesting.MemStore
	stubStore *jujuclienttesting.StubStore
	onRefresh func()
}

var _ = gc.Suite(&SwitchSimpleSuite{})

func (s *SwitchSimpleSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.Stub.ResetCalls()
	s.store = jujuclienttesting.NewMemStore()
	s.stubStore = jujuclienttesting.WrapClientStore(s.store)
	s.onRefresh = nil
}

func (s *SwitchSimpleSuite) refreshModels(store jujuclient.ClientStore, controllerName, accountName string) error {
	s.MethodCall(s, "RefreshModels", store, controllerName, accountName)
	if s.onRefresh != nil {
		s.onRefresh()
	}
	return s.NextErr()
}

func (s *SwitchSimpleSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	cmd := &switchCommand{
		Store:         s.stubStore,
		RefreshModels: s.refreshModels,
	}
	return coretesting.RunCommand(c, modelcmd.WrapBase(cmd), args...)
}

func (s *SwitchSimpleSuite) TestNoArgs(c *gc.C) {
	_, err := s.run(c)
	c.Assert(err, gc.ErrorMatches, "no currently specified model")
}

func (s *SwitchSimpleSuite) TestNoArgsCurrentController(c *gc.C) {
	s.addController(c, "a-controller")
	s.store.CurrentControllerName = "a-controller"
	ctx, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stdout(ctx), gc.Equals, "a-controller\n")
}

func (s *SwitchSimpleSuite) TestUnknownControllerNameReturnsError(c *gc.C) {
	s.addController(c, "a-controller")
	s.store.CurrentControllerName = "a-controller"
	_, err := s.run(c, "another-controller:modela")
	c.Assert(err, gc.ErrorMatches, "controller another-controller not found")
}

func (s *SwitchSimpleSuite) TestNoArgsCurrentModel(c *gc.C) {
	s.addController(c, "a-controller")
	s.store.CurrentControllerName = "a-controller"
	s.store.Models["a-controller"] = jujuclient.ControllerAccountModels{
		map[string]*jujuclient.AccountModels{
			"admin@local": {
				Models:       map[string]jujuclient.ModelDetails{"mymodel": {}},
				CurrentModel: "mymodel",
			},
		},
	}
	ctx, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stdout(ctx), gc.Equals, "a-controller:mymodel\n")
}

func (s *SwitchSimpleSuite) TestSwitchWritesCurrentController(c *gc.C) {
	s.addController(c, "a-controller")
	context, err := s.run(c, "a-controller")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(context), gc.Equals, " -> a-controller (controller)\n")
	s.stubStore.CheckCalls(c, []testing.StubCall{
		{"CurrentController", nil},
		{"ControllerByName", []interface{}{"a-controller"}},
		{"CurrentAccount", []interface{}{"a-controller"}},
		{"CurrentModel", []interface{}{"a-controller", "admin@local"}},
		{"SetCurrentController", []interface{}{"a-controller"}},
	})
}

func (s *SwitchSimpleSuite) TestSwitchWithCurrentController(c *gc.C) {
	s.store.CurrentControllerName = "old"
	s.addController(c, "new")
	context, err := s.run(c, "new")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(context), gc.Equals, "old (controller) -> new (controller)\n")
}

func (s *SwitchSimpleSuite) TestSwitchLocalControllerWithCurrent(c *gc.C) {
	s.store.CurrentControllerName = "old"
	s.addController(c, "new")
	context, err := s.run(c, "new")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(context), gc.Equals, "old (controller) -> new (controller)\n")
}

func (s *SwitchSimpleSuite) TestSwitchSameController(c *gc.C) {
	s.store.CurrentControllerName = "same"
	s.addController(c, "same")
	context, err := s.run(c, "same")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(context), gc.Equals, "same (controller) (no change)\n")
	s.stubStore.CheckCalls(c, []testing.StubCall{
		{"CurrentController", nil},
		{"CurrentAccount", []interface{}{"same"}},
		{"CurrentModel", []interface{}{"same", "admin@local"}},
		{"ControllerByName", []interface{}{"same"}},
	})
}

func (s *SwitchSimpleSuite) TestSwitchControllerToModel(c *gc.C) {
	s.store.CurrentControllerName = "ctrl"
	s.addController(c, "ctrl")
	s.store.Models["ctrl"] = jujuclient.ControllerAccountModels{
		map[string]*jujuclient.AccountModels{
			"admin@local": {
				Models: map[string]jujuclient.ModelDetails{"mymodel": {}},
			},
		},
	}
	context, err := s.run(c, "mymodel")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(context), gc.Equals, "ctrl (controller) -> ctrl:mymodel\n")
	s.stubStore.CheckCalls(c, []testing.StubCall{
		{"CurrentController", nil},
		{"CurrentAccount", []interface{}{"ctrl"}},
		{"CurrentModel", []interface{}{"ctrl", "admin@local"}},
		{"ControllerByName", []interface{}{"mymodel"}},
		{"CurrentAccount", []interface{}{"ctrl"}},
		{"SetCurrentModel", []interface{}{"ctrl", "admin@local", "mymodel"}},
	})
	c.Assert(s.store.Models["ctrl"].AccountModels["admin@local"].CurrentModel, gc.Equals, "mymodel")
}

func (s *SwitchSimpleSuite) TestSwitchControllerToModelDifferentController(c *gc.C) {
	s.store.CurrentControllerName = "old"
	s.addController(c, "new")
	s.store.Models["new"] = jujuclient.ControllerAccountModels{
		map[string]*jujuclient.AccountModels{
			"admin@local": {
				Models: map[string]jujuclient.ModelDetails{"mymodel": {}},
			},
		},
	}
	context, err := s.run(c, "new:mymodel")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(context), gc.Equals, "old (controller) -> new:mymodel\n")
	s.stubStore.CheckCalls(c, []testing.StubCall{
		{"CurrentController", nil},
		{"CurrentAccount", []interface{}{"old"}},
		{"ControllerByName", []interface{}{"new:mymodel"}},
		{"ControllerByName", []interface{}{"new"}},
		{"CurrentAccount", []interface{}{"new"}},
		{"SetCurrentModel", []interface{}{"new", "admin@local", "mymodel"}},
		{"SetCurrentController", []interface{}{"new"}},
	})
	c.Assert(s.store.Models["new"].AccountModels["admin@local"].CurrentModel, gc.Equals, "mymodel")
}

func (s *SwitchSimpleSuite) TestSwitchLocalControllerToModelDifferentController(c *gc.C) {
	s.store.CurrentControllerName = "old"
	s.addController(c, "new")
	s.store.Models["new"] = jujuclient.ControllerAccountModels{
		map[string]*jujuclient.AccountModels{
			"admin@local": {
				Models: map[string]jujuclient.ModelDetails{"mymodel": {}},
			},
		},
	}
	context, err := s.run(c, "new:mymodel")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(context), gc.Equals, "old (controller) -> new:mymodel\n")
	s.stubStore.CheckCalls(c, []testing.StubCall{
		{"CurrentController", nil},
		{"CurrentAccount", []interface{}{"old"}},
		{"ControllerByName", []interface{}{"new:mymodel"}},
		{"ControllerByName", []interface{}{"new"}},
		{"CurrentAccount", []interface{}{"new"}},
		{"SetCurrentModel", []interface{}{"new", "admin@local", "mymodel"}},
		{"SetCurrentController", []interface{}{"new"}},
	})
	c.Assert(s.store.Models["new"].AccountModels["admin@local"].CurrentModel, gc.Equals, "mymodel")
}

func (s *SwitchSimpleSuite) TestSwitchControllerToDifferentControllerCurrentModel(c *gc.C) {
	s.store.CurrentControllerName = "old"
	s.addController(c, "new")
	s.store.Models["new"] = jujuclient.ControllerAccountModels{
		map[string]*jujuclient.AccountModels{
			"admin@local": {
				Models:       map[string]jujuclient.ModelDetails{"mymodel": {}},
				CurrentModel: "mymodel",
			},
		},
	}
	context, err := s.run(c, "new:mymodel")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(context), gc.Equals, "old (controller) -> new:mymodel\n")
	s.stubStore.CheckCalls(c, []testing.StubCall{
		{"CurrentController", nil},
		{"CurrentAccount", []interface{}{"old"}},
		{"ControllerByName", []interface{}{"new:mymodel"}},
		{"ControllerByName", []interface{}{"new"}},
		{"CurrentAccount", []interface{}{"new"}},
		{"SetCurrentModel", []interface{}{"new", "admin@local", "mymodel"}},
		{"SetCurrentController", []interface{}{"new"}},
	})
}

func (s *SwitchSimpleSuite) TestSwitchUnknownNoCurrentController(c *gc.C) {
	_, err := s.run(c, "unknown")
	c.Assert(err, gc.ErrorMatches, `"unknown" is not the name of a model or controller`)
	s.stubStore.CheckCalls(c, []testing.StubCall{
		{"CurrentController", nil},
		{"ControllerByName", []interface{}{"unknown"}},
	})
}

func (s *SwitchSimpleSuite) TestSwitchUnknownCurrentControllerRefreshModels(c *gc.C) {
	s.store.CurrentControllerName = "ctrl"
	s.addController(c, "ctrl")
	s.onRefresh = func() {
		s.store.Models["ctrl"] = jujuclient.ControllerAccountModels{
			map[string]*jujuclient.AccountModels{
				"admin@local": {
					Models: map[string]jujuclient.ModelDetails{"unknown": {}},
				},
			},
		}
	}
	ctx, err := s.run(c, "unknown")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(ctx), gc.Equals, "ctrl (controller) -> ctrl:unknown\n")
	s.CheckCalls(c, []testing.StubCall{
		{"RefreshModels", []interface{}{s.stubStore, "ctrl", "admin@local"}},
	})
}

func (s *SwitchSimpleSuite) TestSwitchUnknownCurrentControllerRefreshModelsStillUnknown(c *gc.C) {
	s.store.CurrentControllerName = "ctrl"
	s.addController(c, "ctrl")
	_, err := s.run(c, "unknown")
	c.Assert(err, gc.ErrorMatches, `"unknown" is not the name of a model or controller`)
	s.CheckCalls(c, []testing.StubCall{
		{"RefreshModels", []interface{}{s.stubStore, "ctrl", "admin@local"}},
	})
}

func (s *SwitchSimpleSuite) TestSwitchUnknownCurrentControllerRefreshModelsFails(c *gc.C) {
	s.store.CurrentControllerName = "ctrl"
	s.addController(c, "ctrl")
	s.SetErrors(errors.New("not very refreshing"))
	_, err := s.run(c, "unknown")
	c.Assert(err, gc.ErrorMatches, "refreshing models cache: not very refreshing")
	s.CheckCalls(c, []testing.StubCall{
		{"RefreshModels", []interface{}{s.stubStore, "ctrl", "admin@local"}},
	})
}

func (s *SwitchSimpleSuite) TestSettingWhenEnvVarSet(c *gc.C) {
	os.Setenv("JUJU_MODEL", "using-model")
	_, err := s.run(c, "erewhemos-2")
	c.Assert(err, gc.ErrorMatches, `cannot switch when JUJU_MODEL is overriding the model \(set to "using-model"\)`)
}

func (s *SwitchSimpleSuite) TestTooManyParams(c *gc.C) {
	_, err := s.run(c, "foo", "bar")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: ."bar".`)
}

func (s *SwitchSimpleSuite) addController(c *gc.C, name string) {
	s.store.Controllers[name] = jujuclient.ControllerDetails{}
	s.store.Accounts[name] = &jujuclient.ControllerAccounts{
		CurrentAccount: "admin@local",
	}
}
