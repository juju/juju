// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"context"
	"errors"
	"os"

	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	coretesting "github.com/juju/juju/internal/testing"
	_ "github.com/juju/juju/juju"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

type SwitchSimpleSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	testing.Stub
	store     *jujuclient.MemStore
	stubStore *jujuclienttesting.StubStore
	onRefresh func()
}

var _ = tc.Suite(&SwitchSimpleSuite{})

func (s *SwitchSimpleSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.Stub.ResetCalls()
	s.store = jujuclient.NewMemStore()
	s.stubStore = jujuclienttesting.WrapClientStore(s.store)
	s.onRefresh = nil
}

func (s *SwitchSimpleSuite) refreshModels(ctx context.Context, store jujuclient.ClientStore, controllerName string) error {
	s.MethodCall(s, "RefreshModels", store, controllerName)
	if s.onRefresh != nil {
		s.onRefresh()
	}
	return s.NextErr()
}

func (s *SwitchSimpleSuite) run(c *tc.C, args ...string) (*cmd.Context, error) {
	switchCmd := &switchCommand{
		Store:         s.stubStore,
		RefreshModels: s.refreshModels,
	}
	return cmdtesting.RunCommand(c, modelcmd.WrapBase(switchCmd), args...)
}

func (s *SwitchSimpleSuite) TestNoArgs(c *tc.C) {
	_, err := s.run(c)
	c.Assert(err, tc.ErrorMatches, common.MissingModelNameError("switch").Error())
}

func (s *SwitchSimpleSuite) TestNoArgsCurrentController(c *tc.C) {
	s.addController(c, "a-controller")
	s.store.CurrentControllerName = "a-controller"
	ctx, err := s.run(c)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "a-controller\n")
}

func (s *SwitchSimpleSuite) TestUnknownControllerNameReturnsError(c *tc.C) {
	s.addController(c, "a-controller")
	s.store.CurrentControllerName = "a-controller"
	_, err := s.run(c, "another-controller:modela")
	c.Assert(err, tc.ErrorMatches, "invalid target model: controller another-controller not found")
}

func (s *SwitchSimpleSuite) TestNoArgsCurrentModel(c *tc.C) {
	s.addController(c, "a-controller")
	s.store.CurrentControllerName = "a-controller"
	s.store.Models["a-controller"] = &jujuclient.ControllerModels{
		Models:       map[string]jujuclient.ModelDetails{"admin/mymodel": {}},
		CurrentModel: "admin/mymodel",
	}
	ctx, err := s.run(c)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "a-controller:admin/mymodel\n")
}

func (s *SwitchSimpleSuite) TestSwitchWritesCurrentController(c *tc.C) {
	s.addController(c, "a-controller")
	context, err := s.run(c, "a-controller")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, " -> a-controller (controller)\n")
	s.stubStore.CheckCalls(c, []testing.StubCall{
		{FuncName: "CurrentController", Args: nil},
		{FuncName: "ControllerByName", Args: []interface{}{"a-controller"}},
		{FuncName: "CurrentModel", Args: []interface{}{"a-controller"}},
		{FuncName: "SetCurrentController", Args: []interface{}{"a-controller"}},
	})
}

func (s *SwitchSimpleSuite) TestSwitchLocalControllerWithCurrent(c *tc.C) {
	s.store.CurrentControllerName = "old"
	s.addController(c, "old")
	s.addController(c, "new")
	context, err := s.run(c, "new")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "old (controller) -> new (controller)\n")
}

func (s *SwitchSimpleSuite) TestSwitchLocalControllerWithCurrentExplicit(c *tc.C) {
	s.store.CurrentControllerName = "old"
	s.addController(c, "old")
	s.addController(c, "new")
	context, err := s.run(c, "new:")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "old (controller) -> new (controller)\n")
}

func (s *SwitchSimpleSuite) TestSwitchSameController(c *tc.C) {
	s.store.CurrentControllerName = "same"
	s.addController(c, "same")
	context, err := s.run(c, "same")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "same (controller) (no change)\n")
	s.stubStore.CheckCalls(c, []testing.StubCall{
		{FuncName: "CurrentController", Args: nil},
		{FuncName: "ControllerByName", Args: []interface{}{"same"}},
		{FuncName: "CurrentModel", Args: []interface{}{"same"}},
		{FuncName: "ControllerByName", Args: []interface{}{"same"}},
		{FuncName: "CurrentModel", Args: []interface{}{"same"}},
		{FuncName: "SetCurrentController", Args: []interface{}{"same"}},
	})
}

func (s *SwitchSimpleSuite) TestSwitchControllerToModel(c *tc.C) {
	s.store.CurrentControllerName = "ctrl"
	s.addController(c, "ctrl")
	s.store.Models["ctrl"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{"admin/mymodel": {}},
	}
	context, err := s.run(c, "mymodel")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "ctrl (controller) -> ctrl:admin/mymodel\n")
	s.stubStore.CheckCalls(c, []testing.StubCall{
		{"CurrentController", nil},
		{"ControllerByName", []interface{}{"ctrl"}},
		{"CurrentModel", []interface{}{"ctrl"}},
		{"ControllerByName", []interface{}{"mymodel"}},
		{"SetCurrentController", []interface{}{"ctrl"}},
		{"AccountDetails", []interface{}{"ctrl"}},
		{"SetCurrentModel", []interface{}{"ctrl", "admin/mymodel"}},
	})
	c.Assert(s.store.Models["ctrl"].CurrentModel, tc.Equals, "admin/mymodel")
}

func (s *SwitchSimpleSuite) TestSwitchControllerToModelDifferentController(c *tc.C) {
	s.store.CurrentControllerName = "old"
	s.addController(c, "old")
	s.addController(c, "new")
	s.store.Models["new"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{"admin/mymodel": {}},
	}
	context, err := s.run(c, "new:mymodel")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "old (controller) -> new:admin/mymodel\n")
	s.stubStore.CheckCalls(c, []testing.StubCall{
		{"CurrentController", nil},
		{"ControllerByName", []interface{}{"old"}},
		{"CurrentModel", []interface{}{"old"}},
		{"SetCurrentController", []interface{}{"new"}},
		{"AccountDetails", []interface{}{"new"}},
		{"SetCurrentModel", []interface{}{"new", "admin/mymodel"}},
	})
	c.Assert(s.store.Models["new"].CurrentModel, tc.Equals, "admin/mymodel")
}

func (s *SwitchSimpleSuite) TestSwitchControllerSameNameAsModel(c *tc.C) {
	s.store.CurrentControllerName = "old"
	s.addController(c, "new")
	s.addController(c, "old")
	s.store.Models["new"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{"admin/mymodel": {}, "admin/old": {}},
	}
	s.store.Models["old"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{"admin/somemodel": {}},
	}
	_, err := s.run(c, "new:mymodel")
	c.Assert(err, tc.ErrorIsNil)
	context, err := s.run(c, "old")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "new:admin/mymodel -> old (controller)\n")
}

func (s *SwitchSimpleSuite) TestSwitchControllerSameNameAsModelExplicitModel(c *tc.C) {
	s.store.CurrentControllerName = "old"
	s.addController(c, "new")
	s.addController(c, "old")
	s.store.Models["new"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{"admin/mymodel": {}, "admin/old": {}},
	}
	s.store.Models["old"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{"admin/somemodel": {}},
	}
	_, err := s.run(c, "new:mymodel")
	c.Assert(err, tc.ErrorIsNil)
	context, err := s.run(c, ":old")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "new:admin/mymodel -> new:admin/old\n")
}

func (s *SwitchSimpleSuite) TestSwitchLocalControllerToModelDifferentController(c *tc.C) {
	s.store.CurrentControllerName = "old"
	s.addController(c, "old")
	s.addController(c, "new")
	s.store.Models["new"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{"admin/mymodel": {}},
	}
	context, err := s.run(c, "new:mymodel")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "old (controller) -> new:admin/mymodel\n")
	s.stubStore.CheckCalls(c, []testing.StubCall{
		{"CurrentController", nil},
		{"ControllerByName", []interface{}{"old"}},
		{"CurrentModel", []interface{}{"old"}},
		{"SetCurrentController", []interface{}{"new"}},
		{"AccountDetails", []interface{}{"new"}},
		{"SetCurrentModel", []interface{}{"new", "admin/mymodel"}},
	})
	c.Assert(s.store.Models["new"].CurrentModel, tc.Equals, "admin/mymodel")
}

func (s *SwitchSimpleSuite) TestSwitchControllerToDifferentControllerCurrentModel(c *tc.C) {
	s.store.CurrentControllerName = "old"
	s.addController(c, "old")
	s.addController(c, "new")
	s.store.Models["new"] = &jujuclient.ControllerModels{
		Models:       map[string]jujuclient.ModelDetails{"admin/mymodel": {}},
		CurrentModel: "admin/mymodel",
	}
	context, err := s.run(c, "new:mymodel")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "old (controller) -> new:admin/mymodel\n")
	s.stubStore.CheckCalls(c, []testing.StubCall{
		{"CurrentController", nil},
		{"ControllerByName", []interface{}{"old"}},
		{"CurrentModel", []interface{}{"old"}},
		{"SetCurrentController", []interface{}{"new"}},
		{"AccountDetails", []interface{}{"new"}},
		{"SetCurrentModel", []interface{}{"new", "admin/mymodel"}},
	})
}

func (s *SwitchSimpleSuite) TestSwitchToModelDifferentOwner(c *tc.C) {
	s.store.CurrentControllerName = "same"
	s.addController(c, "same")
	s.store.Models["same"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{
			"admin/mymodel":  {},
			"bianca/mymodel": {},
		},
		CurrentModel: "admin/mymodel",
	}
	context, err := s.run(c, "bianca/mymodel")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "same:admin/mymodel -> same:bianca/mymodel\n")
	c.Assert(s.store.Models["same"].CurrentModel, tc.Equals, "bianca/mymodel")
}

func (s *SwitchSimpleSuite) TestSwitchUnknownNoCurrentController(c *tc.C) {
	_, err := s.run(c, "unknown")
	c.Assert(err, tc.ErrorMatches, `"unknown" is not the name of a model or controller`)
	s.stubStore.CheckCalls(c, []testing.StubCall{
		{FuncName: "CurrentController", Args: nil},
		{FuncName: "ControllerByName", Args: []interface{}{"unknown"}},
	})
}

func (s *SwitchSimpleSuite) TestSwitchUnknownCurrentControllerRefreshModels(c *tc.C) {
	s.store.CurrentControllerName = "ctrl"
	s.addController(c, "ctrl")
	s.onRefresh = func() {
		s.store.Models["ctrl"] = &jujuclient.ControllerModels{
			Models: map[string]jujuclient.ModelDetails{"admin/unknown": {}},
		}
	}
	ctx, err := s.run(c, "unknown")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "ctrl (controller) -> ctrl:admin/unknown\n")
	s.CheckCallNames(c, "RefreshModels")
}

func (s *SwitchSimpleSuite) TestSwitchUnknownCurrentControllerRefreshModelsStillUnknown(c *tc.C) {
	s.store.CurrentControllerName = "ctrl"
	s.addController(c, "ctrl")
	_, err := s.run(c, "unknown")
	c.Assert(err, tc.ErrorMatches, `cannot determine if "unknown" is a valid model: "ctrl:unknown" is not the name of a model or controller`)
	s.CheckCallNames(c, "RefreshModels")
}

func (s *SwitchSimpleSuite) TestSwitchUnknownCurrentControllerRefreshModelsFails(c *tc.C) {
	s.store.CurrentControllerName = "ctrl"
	s.addController(c, "ctrl")
	s.SetErrors(errors.New("not very refreshing"))
	_, err := s.run(c, "unknown")
	c.Assert(err, tc.ErrorMatches, "cannot determine if \"unknown\" is a valid model: refreshing models cache: not very refreshing")
	s.CheckCallNames(c, "RefreshModels")
}

func (s *SwitchSimpleSuite) TestSettingWhenModelEnvVarSet(c *tc.C) {
	err := os.Setenv("JUJU_MODEL", "using-model")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.run(c, "erewhemos-2")
	c.Assert(err, tc.ErrorMatches, `cannot switch when JUJU_MODEL is overriding the model \(set to "using-model"\)`)
}

func (s *SwitchSimpleSuite) TestSettingWhenControllerEnvVarSet(c *tc.C) {
	err := os.Setenv("JUJU_CONTROLLER", "using-controller")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.run(c, "erewhemos-2")
	c.Assert(err, tc.ErrorMatches, `cannot switch when JUJU_CONTROLLER is overriding the controller \(set to "using-controller"\)`)
}

func (s *SwitchSimpleSuite) TestTooManyParams(c *tc.C) {
	_, err := s.run(c, "foo", "bar")
	c.Assert(err, tc.ErrorMatches, `unrecognized args: ."bar".`)
}

func (s *SwitchSimpleSuite) addController(c *tc.C, name string) {
	s.store.Controllers[name] = jujuclient.ControllerDetails{}
	s.store.Accounts[name] = jujuclient.AccountDetails{
		User: "admin",
	}
}

func (s *SwitchSimpleSuite) TestSwitchCurrentModelInStore(c *tc.C) {
	s.store.CurrentControllerName = "same"
	s.addController(c, "same")
	s.store.Models["same"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{
			"admin/mymodel": {},
		},
		CurrentModel: "admin/mymodel",
	}
	context, err := s.run(c, "mymodel")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "same:admin/mymodel (no change)\n")
	s.stubStore.CheckCalls(c, []testing.StubCall{
		{"CurrentController", nil},
		{"ControllerByName", []interface{}{"same"}},
		{"CurrentModel", []interface{}{"same"}},
		{"ControllerByName", []interface{}{"mymodel"}},
		{"SetCurrentController", []interface{}{"same"}},
		{"AccountDetails", []interface{}{"same"}},
		{"SetCurrentModel", []interface{}{"same", "admin/mymodel"}},
	})
}

func (s *SwitchSimpleSuite) TestSwitchCurrentModelNoLongerInStore(c *tc.C) {
	s.store.CurrentControllerName = "same"
	s.addController(c, "same")
	s.store.Models["same"] = &jujuclient.ControllerModels{CurrentModel: "admin/mymodel"}
	_, err := s.run(c, "mymodel")
	c.Assert(err, tc.ErrorMatches, `cannot determine if "mymodel" is a valid model: "same:mymodel" is not the name of a model or controller`)
}

func (s *SwitchSimpleSuite) TestSwitchPreviousControllerAndModelThroughFlagsShouldFail(c *tc.C) {
	s.store.CurrentControllerName = "currentCtrl"
	s.store.PreviousControllerName = "previousCtrl"
	s.addController(c, "currentCtrl")
	s.addController(c, "previousCtrl")

	// juju switch -m model -c controller: # Should fails
	_, err := s.run(c, "-m", "model", "-c", "controller")

	c.Assert(err, tc.ErrorMatches, "cannot specify both a --model and --controller")
}

func (s *SwitchSimpleSuite) TestSwitchPreviousController(c *tc.C) {
	s.store.CurrentControllerName = "currentCtrl"
	s.store.PreviousControllerName = "previousCtrl"
	s.addController(c, "currentCtrl")
	s.addController(c, "previousCtrl")

	// juju switch -c - # Should switch to previous controller
	context, err := s.run(c, "-c", "-")

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "currentCtrl (controller) -> previousCtrl (controller)\n")
}

func (s *SwitchSimpleSuite) TestSwitchPreviousControllerWhichDoesntExists(c *tc.C) {
	s.store.CurrentControllerName = "currentCtrl"
	s.store.PreviousControllerName = "noCtrl"
	s.addController(c, "currentCtrl")

	// juju switch --controller - # previous controller may have been deleted, should do nothing
	_, err := s.run(c, "--controller", "-")

	c.Assert(err, tc.ErrorMatches, "invalid target controller: controller noCtrl not found")
}

func (s *SwitchSimpleSuite) TestSwitchPreviousControllerWhichIsEmpty(c *tc.C) {
	s.store.CurrentControllerName = "currentCtrl"
	s.addController(c, "currentCtrl")

	// juju switch -c - # no previous controller may have been deleted, should do nothing
	_, err := s.run(c, "-c", "-")

	c.Assert(err, tc.ErrorMatches, "interpreting \"--controller -\": previous controller not found")
}

func (s *SwitchSimpleSuite) TestSwitchPreviousModel(c *tc.C) {
	s.store.CurrentControllerName = "ctrl"
	s.addController(c, "ctrl")
	s.store.Models["ctrl"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{"admin/current-model": {},
			"admin/previous-model": {}},
		CurrentModel:  "admin/current-model",
		PreviousModel: "admin/previous-model",
	}

	// juju switch --model - # Should switch to previous model in current controller
	context, err := s.run(c, "--model", "-")

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "ctrl:admin/current-model -> ctrl:admin/previous-model\n")
	c.Assert(s.store.Models["ctrl"].CurrentModel, tc.Equals, "admin/previous-model")
	c.Assert(s.store.Models["ctrl"].PreviousModel, tc.Equals, "admin/current-model")
}

func (s *SwitchSimpleSuite) TestSwitchPreviousModelWhichDoesntExits(c *tc.C) {
	s.store.CurrentControllerName = "ctrl"
	s.addController(c, "ctrl")
	s.store.Models["ctrl"] = &jujuclient.ControllerModels{
		Models:        map[string]jujuclient.ModelDetails{"admin/current-model": {}},
		CurrentModel:  "admin/current-model",
		PreviousModel: "admin/previous-model",
	}

	// juju switch -m - # previous model may have been deleted, should do nothing
	_, err := s.run(c, "-m", "-")

	c.Assert(err, tc.ErrorMatches, `invalid target model: "ctrl:admin/previous-model" is not the name of a model or controller`)
}

func (s *SwitchSimpleSuite) TestSwitchPreviousModelWhichIsEmpty(c *tc.C) {
	s.store.CurrentControllerName = "ctrl"
	s.addController(c, "ctrl")
	s.store.Models["ctrl"] = &jujuclient.ControllerModels{
		Models:       map[string]jujuclient.ModelDetails{"admin/current-model": {}},
		CurrentModel: "admin/current-model",
	}

	// juju switch -m - # previous model may have been deleted, should do nothing
	_, err := s.run(c, "-m", "-")

	c.Assert(err, tc.ErrorMatches, `interpreting "--model -": previous model for controller ctrl not found`)
}

func (s *SwitchSimpleSuite) TestSwitchPreviousAcrossControllers(c *tc.C) {
	s.store.CurrentControllerName = "current-ctrl"
	s.store.PreviousControllerName = "previous-ctrl"
	s.store.HasControllerChangedOnPreviousSwitch = true
	s.addController(c, "current-ctrl")
	s.addController(c, "previous-ctrl")
	s.store.Models["current-ctrl"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{"admin/current-model": {},
			"admin/previous-model": {}},
		CurrentModel:  "admin/current-model",
		PreviousModel: "admin/previous-model",
	}
	s.store.Models["previous-ctrl"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{"admin/current-model": {},
			"admin/previous-model": {}},
		CurrentModel:  "admin/current-model",
		PreviousModel: "admin/previous-model",
	}

	// juju switch -
	context, err := s.run(c, "-")

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "current-ctrl:admin/current-model -> previous-ctrl:admin/current-model\n")
	c.Assert(s.store.Models["current-ctrl"].CurrentModel, tc.Equals, "admin/current-model")
	c.Assert(s.store.Models["current-ctrl"].PreviousModel, tc.Equals, "admin/previous-model")
	c.Assert(s.store.Models["previous-ctrl"].CurrentModel, tc.Equals, "admin/current-model")
	c.Assert(s.store.Models["previous-ctrl"].PreviousModel, tc.Equals, "admin/previous-model")
}

func (s *SwitchSimpleSuite) TestSwitchPreviousAcrossModels(c *tc.C) {
	s.store.CurrentControllerName = "current-ctrl"
	s.store.PreviousControllerName = "current-ctrl"
	s.store.HasControllerChangedOnPreviousSwitch = false
	s.addController(c, "current-ctrl")
	s.addController(c, "another-ctrl")
	s.store.Models["current-ctrl"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{"admin/current-model": {},
			"admin/previous-model": {}},
		CurrentModel:  "admin/current-model",
		PreviousModel: "admin/previous-model",
	}

	// juju switch -
	context, err := s.run(c, "-")

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "current-ctrl:admin/current-model -> current-ctrl:admin/previous-model\n")
	c.Assert(s.store.Models["current-ctrl"].CurrentModel, tc.Equals, "admin/previous-model")
	c.Assert(s.store.Models["current-ctrl"].PreviousModel, tc.Equals, "admin/current-model")
}

func (s *SwitchSimpleSuite) TestSwitchPreviousAcrossModels2(c *tc.C) {
	s.store.CurrentControllerName = "current-ctrl"
	s.store.PreviousControllerName = "current-ctrl"
	s.addController(c, "current-ctrl")
	s.addController(c, "another-ctrl")
	s.store.Models["current-ctrl"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{"admin/current-model": {},
			"admin/previous-model": {}},
		CurrentModel:  "admin/current-model",
		PreviousModel: "admin/previous-model",
	}

	// juju switch -
	context, err := s.run(c, "-")

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "current-ctrl:admin/current-model -> current-ctrl:admin/previous-model\n")
	c.Assert(s.store.Models["current-ctrl"].CurrentModel, tc.Equals, "admin/previous-model")
	c.Assert(s.store.Models["current-ctrl"].PreviousModel, tc.Equals, "admin/current-model")
}

func (s *SwitchSimpleSuite) TestSwitchPreviousControllerTwice(c *tc.C) {
	s.store.CurrentControllerName = "currentCtrl"
	s.store.PreviousControllerName = "previousCtrl"
	s.addController(c, "currentCtrl")
	s.addController(c, "previousCtrl")

	// juju switch --controller - && juju switch -c - # Should return to current controller
	_, err := s.run(c, "--controller", "-")
	c.Assert(err, tc.ErrorIsNil)
	context, err := s.run(c, "-c", "-")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(cmdtesting.Stderr(context), tc.Equals, "previousCtrl (controller) -> currentCtrl (controller)\n")
}

func (s *SwitchSimpleSuite) TestSwitchPreviousModelTwice(c *tc.C) {
	s.store.CurrentControllerName = "ctrl"
	s.addController(c, "ctrl")
	s.store.Models["ctrl"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{"admin/current-model": {},
			"admin/previous-model": {}},
		CurrentModel:  "admin/current-model",
		PreviousModel: "admin/previous-model",
	}

	// juju switch --model - && juju switch -m -  # Should switch to current model in current controller
	_, err := s.run(c, "--model", "-")
	c.Assert(err, tc.ErrorIsNil)
	context, err := s.run(c, "-m", "-")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(cmdtesting.Stderr(context), tc.Equals, "ctrl:admin/previous-model -> ctrl:admin/current-model\n")
	c.Assert(s.store.Models["ctrl"].CurrentModel, tc.Equals, "admin/current-model")
	c.Assert(s.store.Models["ctrl"].PreviousModel, tc.Equals, "admin/previous-model")
}

func (s *SwitchSimpleSuite) TestSwitchPreviousModelThenController(c *tc.C) {
	s.store.CurrentControllerName = "ctrl-1"
	s.store.PreviousControllerName = "ctrl-2"
	s.addController(c, "ctrl-1")
	s.addController(c, "ctrl-2")
	s.store.Models["ctrl-1"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{"admin/current-model": {},
			"admin/previous-model": {}},
		CurrentModel:  "admin/current-model",
		PreviousModel: "admin/previous-model",
	}

	// juju switch --model - && juju switch -c -  # Should switch to previous model in current controller, then go to the previous controller
	_, err := s.run(c, "--model", "-")
	c.Assert(err, tc.ErrorIsNil)
	context, err := s.run(c, "-c", "-")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(cmdtesting.Stderr(context), tc.Equals, "ctrl-1:admin/previous-model -> ctrl-2 (controller)\n")
	c.Assert(s.store.Models["ctrl-1"].CurrentModel, tc.Equals, "admin/previous-model")
	c.Assert(s.store.Models["ctrl-1"].PreviousModel, tc.Equals, "admin/current-model")
}
