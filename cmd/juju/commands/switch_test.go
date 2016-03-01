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
	store             *jujuclienttesting.MemStore
	currentController string
	onRefresh         func()
}

var _ = gc.Suite(&SwitchSimpleSuite{})

func (s *SwitchSimpleSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.Stub.ResetCalls()
	s.store = jujuclienttesting.NewMemStore()
	s.currentController = ""
	s.onRefresh = nil
}

func (s *SwitchSimpleSuite) refreshModels(store jujuclient.ClientStore, controllerName, accountName string) error {
	s.MethodCall(s, "RefreshModels", store, controllerName, accountName)
	if s.onRefresh != nil {
		s.onRefresh()
	}
	return s.NextErr()
}

func (s *SwitchSimpleSuite) readCurrentController() (string, error) {
	s.MethodCall(s, "ReadCurrentController")
	return s.currentController, s.NextErr()
}

func (s *SwitchSimpleSuite) writeCurrentController(current string) error {
	s.MethodCall(s, "WriteCurrentController", current)
	if err := s.NextErr(); err != nil {
		return err
	}
	s.currentController = current
	return nil
}

func (s *SwitchSimpleSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	cmd := &switchCommand{
		Store:                  s.store,
		RefreshModels:          s.refreshModels,
		ReadCurrentController:  s.readCurrentController,
		WriteCurrentController: s.writeCurrentController,
	}
	return coretesting.RunCommand(c, modelcmd.WrapBase(cmd), args...)
}

func (s *SwitchSimpleSuite) TestNoArgs(c *gc.C) {
	_, err := s.run(c)
	c.Assert(err, gc.ErrorMatches, "no currently specified model")
}

func (s *SwitchSimpleSuite) TestNoArgsCurrentController(c *gc.C) {
	s.addController(c, "a-controller")
	s.currentController = "a-controller"
	ctx, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stdout(ctx), gc.Equals, "a-controller\n")
}

func (s *SwitchSimpleSuite) TestNoArgsCurrentModel(c *gc.C) {
	s.addController(c, "a-controller")
	s.currentController = "a-controller"
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
	s.CheckCalls(c, []testing.StubCall{
		{"ReadCurrentController", nil},
		{"WriteCurrentController", []interface{}{"a-controller"}},
	})
}

func (s *SwitchSimpleSuite) TestSwitchWithCurrentController(c *gc.C) {
	s.currentController = "old"
	s.addController(c, "new")
	context, err := s.run(c, "new")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(context), gc.Equals, "old (controller) -> new (controller)\n")
}

func (s *SwitchSimpleSuite) TestSwitchLocalControllerWithCurrent(c *gc.C) {
	s.currentController = "old"
	s.addController(c, "local.new")
	context, err := s.run(c, "new")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(context), gc.Equals, "old (controller) -> local.new (controller)\n")
}

func (s *SwitchSimpleSuite) TestSwitchSameController(c *gc.C) {
	s.currentController = "same"
	s.addController(c, "same")
	context, err := s.run(c, "same")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(context), gc.Equals, "same (controller) (no change)\n")
	s.CheckCalls(c, []testing.StubCall{
		{"ReadCurrentController", nil},
	})
}

func (s *SwitchSimpleSuite) TestSwitchControllerToModel(c *gc.C) {
	s.currentController = "ctrl"
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
	s.CheckCalls(c, []testing.StubCall{
		{"ReadCurrentController", nil},
	})
	c.Assert(s.store.Models["ctrl"].AccountModels["admin@local"].CurrentModel, gc.Equals, "mymodel")
}

func (s *SwitchSimpleSuite) TestSwitchControllerToModelDifferentController(c *gc.C) {
	s.currentController = "old"
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
	s.CheckCalls(c, []testing.StubCall{
		{"ReadCurrentController", nil},
		{"WriteCurrentController", []interface{}{"new"}},
	})
	c.Assert(s.store.Models["new"].AccountModels["admin@local"].CurrentModel, gc.Equals, "mymodel")
}

func (s *SwitchSimpleSuite) TestSwitchLocalControllerToModelDifferentController(c *gc.C) {
	s.currentController = "old"
	s.addController(c, "local.new")
	s.store.Models["local.new"] = jujuclient.ControllerAccountModels{
		map[string]*jujuclient.AccountModels{
			"admin@local": {
				Models: map[string]jujuclient.ModelDetails{"mymodel": {}},
			},
		},
	}
	context, err := s.run(c, "new:mymodel")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(context), gc.Equals, "old (controller) -> local.new:mymodel\n")
	s.CheckCalls(c, []testing.StubCall{
		{"ReadCurrentController", nil},
		{"WriteCurrentController", []interface{}{"local.new"}},
	})
	c.Assert(s.store.Models["local.new"].AccountModels["admin@local"].CurrentModel, gc.Equals, "mymodel")
}

func (s *SwitchSimpleSuite) TestSwitchControllerToDifferentControllerCurrentModel(c *gc.C) {
	s.currentController = "old"
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
	s.CheckCalls(c, []testing.StubCall{
		{"ReadCurrentController", nil},
		{"WriteCurrentController", []interface{}{"new"}},
	})
}

func (s *SwitchSimpleSuite) TestSwitchUnknownNoCurrentController(c *gc.C) {
	_, err := s.run(c, "unknown")
	c.Assert(err, gc.ErrorMatches, `"unknown" is not the name of a model or controller`)
	s.CheckCalls(c, []testing.StubCall{
		{"ReadCurrentController", nil},
	})
}

func (s *SwitchSimpleSuite) TestSwitchUnknownCurrentControllerRefreshModels(c *gc.C) {
	s.currentController = "ctrl"
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
		{"ReadCurrentController", nil},
		{"RefreshModels", []interface{}{s.store, "ctrl", "admin@local"}},
	})
}

func (s *SwitchSimpleSuite) TestSwitchUnknownCurrentControllerRefreshModelsStillUnknown(c *gc.C) {
	s.currentController = "ctrl"
	s.addController(c, "ctrl")
	_, err := s.run(c, "unknown")
	c.Assert(err, gc.ErrorMatches, `"unknown" is not the name of a model or controller`)
	s.CheckCalls(c, []testing.StubCall{
		{"ReadCurrentController", nil},
		{"RefreshModels", []interface{}{s.store, "ctrl", "admin@local"}},
	})
}

func (s *SwitchSimpleSuite) TestSwitchUnknownCurrentControllerRefreshModelsFails(c *gc.C) {
	s.currentController = "ctrl"
	s.addController(c, "ctrl")
	s.SetErrors(nil, errors.New("not very refreshing"))
	_, err := s.run(c, "unknown")
	c.Assert(err, gc.ErrorMatches, "refreshing models cache: not very refreshing")
	s.CheckCalls(c, []testing.StubCall{
		{"ReadCurrentController", nil},
		{"RefreshModels", []interface{}{s.store, "ctrl", "admin@local"}},
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
