// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"bytes"
	"context"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/base"
	apicontroller "github.com/juju/juju/api/controller/controller"
	"github.com/juju/juju/api/jujuclient"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/cmd/cmdtesting"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/cmd/modelcmd"
	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	_ "github.com/juju/juju/internal/provider/dummy"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

const (
	test1UUID = "1871299e-1370-4f3e-83ab-1849ed7b1076"
	test2UUID = "c59d0e3b-2bd7-4867-b1b9-f1ef8a0bb004"
	test3UUID = "82bf9738-764b-49c1-9c19-18f6ee155854"

	test1ControllerUUID = "2371299e-1370-4f3e-83ab-1849ed7b1076"
	test2ControllerUUID = "f89d0e3b-5bd7-9867-b1b9-f1ef8a0bb004"
	test3ControllerUUID = "cfbf9738-764b-49c1-9c19-18f6ee155854"
)

type DestroySuite struct {
	baseDestroySuite
}

func TestDestroySuite(t *stdtesting.T) {
	tc.Run(t, &DestroySuite{})
}

type baseDestroySuite struct {
	testing.FakeJujuXDGDataHomeSuite
	api      *fakeDestroyAPI
	store    *jujuclient.MemStore
	apierror error

	controllerModelConfigAPI *fakeModelConfigAPI

	environsDestroy func(string, environs.ControllerDestroyer, context.Context, jujuclient.ControllerStore) error
}

// fakeDestroyAPI mocks out the controller API
type fakeDestroyAPI struct {
	testhelpers.Stub
	cloud        environscloudspec.CloudSpec
	blocks       []params.ModelBlockInfo
	modelStatus  map[string]base.ModelStatus
	allModels    []base.UserModel
	hostedConfig []apicontroller.HostedConfig

	// delayModelRemoval simulates the server-side undertaker taking one
	// extra status poll to remove reaped hosted models from the
	// controller database after a successful DestroyController.
	delayModelRemoval bool
	// delayMachineRemoval simulates machines (for example machines in
	// the controller model) taking one extra status poll to be removed
	// after a successful DestroyController.
	delayMachineRemoval bool

	// The fields below hold the post-destroy state computed by
	// DestroyController. When either delay is set, that state is only
	// served from the second status poll after the destroy call.
	holdStatus              bool
	reapPending             bool
	allModelsAfterDestroy   []base.UserModel
	modelStatusAfterDestroy map[string]base.ModelStatus
}

func (f *fakeDestroyAPI) Close() error {
	f.MethodCall(f, "Close")
	return f.NextErr()
}

func (f *fakeDestroyAPI) CloudSpec(ctx context.Context, tag names.ModelTag) (environscloudspec.CloudSpec, error) {
	f.MethodCall(f, "CloudSpec", tag)
	if err := f.NextErr(); err != nil {
		return environscloudspec.CloudSpec{}, err
	}
	return f.cloud, nil
}

func (f *fakeDestroyAPI) ControllerConfig(_ context.Context) (jujucontroller.Config, error) {
	f.MethodCall(f, "ControllerConfig")
	if err := f.NextErr(); err != nil {
		return nil, err
	}
	return testing.FakeControllerConfig(), nil
}

func (f *fakeDestroyAPI) HostedModelConfigs(ctx context.Context) ([]apicontroller.HostedConfig, error) {
	f.MethodCall(f, "HostedModelConfigs")
	if err := f.NextErr(); err != nil {
		return nil, err
	}
	return f.hostedConfig, nil
}

func (f *fakeDestroyAPI) DestroyController(ctx context.Context, args apicontroller.DestroyControllerParams) error {
	f.MethodCall(f, "DestroyController", args)
	if err := f.NextErr(); err != nil {
		return err
	}
	// Simulate the server-side undertaker removing hosted models: with
	// DestroyModels all hosted models are destroyed and removed; without it,
	// already-dead models are still reaped in the background.
	remaining := make([]base.UserModel, 0, len(f.allModels))
	remainingStatus := make(map[string]base.ModelStatus, len(f.modelStatus))
	for _, m := range f.allModels {
		if m.UUID == test1UUID {
			remaining = append(remaining, m)
			remainingStatus[m.UUID] = f.modelStatus[m.UUID]
			continue
		}
		status, ok := f.modelStatus[m.UUID]
		if args.DestroyModels || (ok && status.Life == life.Dead) {
			continue
		}
		remaining = append(remaining, m)
		remainingStatus[m.UUID] = status
	}
	f.allModelsAfterDestroy = remaining
	f.modelStatusAfterDestroy = remainingStatus
	if f.delayMachineRemoval {
		// Machines are torn down after the model reap; serve the reaped
		// lists with the machines still present for one more poll.
		cleared := make(map[string]base.ModelStatus, len(remainingStatus))
		for uuid, status := range remainingStatus {
			status.HostedMachineCount = 0
			cleared[uuid] = status
		}
		f.allModels = remaining
		f.modelStatus = remainingStatus
		f.modelStatusAfterDestroy = cleared
		f.holdStatus = true
		return nil
	}
	if f.delayModelRemoval {
		// The reaped model rows are still present when
		// DestroyController returns; serve the pre-destroy lists once,
		// the reaped lists are applied on the next poll.
		f.holdStatus = true
		return nil
	}
	f.allModels = remaining
	f.modelStatus = remainingStatus
	return nil
}

func (f *fakeDestroyAPI) ListBlockedModels(ctx context.Context) ([]params.ModelBlockInfo, error) {
	f.MethodCall(f, "ListBlockedModels")
	return f.blocks, f.NextErr()
}

func (f *fakeDestroyAPI) ModelStatus(_ context.Context, tags ...names.ModelTag) ([]base.ModelStatus, error) {
	f.MethodCall(f, "ModelStatus", tags)
	status := make([]base.ModelStatus, len(tags))
	for i, tag := range tags {
		status[i] = f.modelStatus[tag.Id()]
	}
	return status, f.NextErr()
}

func (f *fakeDestroyAPI) AllModels(ctx context.Context) ([]base.UserModel, error) {
	f.MethodCall(f, "AllModels")
	if f.holdStatus {
		f.holdStatus = false
		f.reapPending = true
		return f.allModels, f.NextErr()
	}
	if f.reapPending {
		f.reapPending = false
		f.allModels = f.allModelsAfterDestroy
		f.modelStatus = f.modelStatusAfterDestroy
	}
	return f.allModels, f.NextErr()
}

// fakeModelConfigAPI mocks out the controller model config API
type fakeModelConfigAPI struct {
	testhelpers.Stub
	env map[string]any
}

func (f *fakeModelConfigAPI) Close() error {
	f.MethodCall(f, "Close")
	return f.NextErr()
}

func (f *fakeModelConfigAPI) ModelGet(ctx context.Context) (map[string]any, error) {
	f.MethodCall(f, "ModelGet")
	if err := f.NextErr(); err != nil {
		return nil, err
	}
	return f.env, nil
}

func createBootstrapInfo(c *tc.C, name string) map[string]any {
	cfg, err := config.New(config.UseDefaults, map[string]any{
		"type":       "dummy",
		"name":       name,
		"uuid":       testing.ModelTag.Id(),
		"controller": "true",
	})
	c.Assert(err, tc.ErrorIsNil)
	return cfg.AllAttrs()
}

func (s *baseDestroySuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.api = &fakeDestroyAPI{
		cloud:       testing.FakeCloudSpec(),
		modelStatus: map[string]base.ModelStatus{},
	}
	s.apierror = nil
	s.controllerModelConfigAPI = &fakeModelConfigAPI{}
	s.environsDestroy = environs.Destroy

	s.store = jujuclient.NewMemStore()
	s.store.Controllers["test1"] = jujuclient.ControllerDetails{
		APIEndpoints:   []string{"localhost"},
		CACert:         testing.CACert,
		ControllerUUID: test1ControllerUUID,
	}
	s.store.Controllers["test3"] = jujuclient.ControllerDetails{
		APIEndpoints:   []string{"localhost"},
		CACert:         testing.CACert,
		ControllerUUID: test3ControllerUUID,
	}
	s.store.Accounts["test1"] = jujuclient.AccountDetails{
		User: "admin",
	}

	var modelList = []struct {
		name           string
		controllerUUID string
		modelUUID      string
		bootstrapCfg   map[string]any
	}{
		{
			name:           "test1:admin",
			controllerUUID: test1ControllerUUID,
			modelUUID:      test1UUID,
			bootstrapCfg:   createBootstrapInfo(c, "admin"),
		}, {
			name:           "test2:test2",
			controllerUUID: test2ControllerUUID,
			modelUUID:      test2UUID,
		}, {
			name:           "test3:admin",
			controllerUUID: test3ControllerUUID,
			modelUUID:      test3UUID,
		},
	}
	for _, model := range modelList {
		controllerName, modelName := modelcmd.SplitModelName(model.name)
		s.store.UpdateController(controllerName, jujuclient.ControllerDetails{
			ControllerUUID: model.controllerUUID,
			APIEndpoints:   []string{"localhost"},
			CACert:         testing.CACert,
		})
		s.store.UpdateModel(controllerName, modelName, jujuclient.ModelDetails{
			ModelUUID: model.modelUUID,
		})
		if model.bootstrapCfg != nil {
			s.store.BootstrapConfig[controllerName] = jujuclient.BootstrapConfig{
				ControllerModelUUID: model.modelUUID,
				Config:              createBootstrapInfo(c, "admin"),
				CloudType:           "dummy",
			}
		}

		uuid := model.modelUUID
		s.api.allModels = append(s.api.allModels, base.UserModel{
			Name:      model.name,
			Qualifier: "prod",
			UUID:      uuid,
		})
		s.api.modelStatus[model.modelUUID] = base.ModelStatus{
			UUID:               uuid,
			Life:               life.Dead,
			HostedMachineCount: 0,
			ApplicationCount:   0,
			Qualifier:          "prod",
		}
	}
}

func (s *DestroySuite) runDestroyCommand(c *tc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, s.newDestroyCommand(), args...)
}

func (s *DestroySuite) newDestroyCommand() cmd.Command {
	return controller.NewDestroyCommandForTest(
		s.api, s.store, s.apierror, s.controllerModelConfigAPI, &mockClock{},
		s.environsDestroy,
	)
}

// countEnvironsDestroy wraps the suite environsDestroy func to count
// calls to it.
func (s *DestroySuite) countEnvironsDestroy() *int {
	calls := new(int)
	destroy := s.environsDestroy
	s.environsDestroy = func(controllerName string, env environs.ControllerDestroyer, ctx context.Context, store jujuclient.ControllerStore) error {
		*calls++
		return destroy(controllerName, env, ctx, store)
	}
	return calls
}

// allModelsCallCount returns how many times the fake API was asked for
// the model list.
func (s *DestroySuite) allModelsCallCount() int {
	count := 0
	for _, call := range s.api.Calls() {
		if call.FuncName == "AllModels" {
			count++
		}
	}
	return count
}

func checkControllerExistsInStore(c *tc.C, name string, store jujuclient.ControllerGetter) {
	_, err := store.ControllerByName(name)
	c.Assert(err, tc.ErrorIsNil)
}

func assertControllerRemovedFromStore(c *tc.C, name string, store jujuclient.ControllerGetter) {
	_, err := store.ControllerByName(name)
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *DestroySuite) TestDestroyNoControllerNameError(c *tc.C) {
	_, err := s.runDestroyCommand(c)
	c.Assert(err, tc.ErrorMatches, "no controller specified")
}

func (s *DestroySuite) TestDestroyBadFlags(c *tc.C) {
	_, err := s.runDestroyCommand(c, "-n")
	c.Assert(err, tc.ErrorMatches, "option provided but not defined: -n")
}

func (s *DestroySuite) TestDestroyUnknownArgument(c *tc.C) {
	_, err := s.runDestroyCommand(c, "model", "whoops")
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}

func (s *DestroySuite) TestDestroyUnknownController(c *tc.C) {
	_, err := s.runDestroyCommand(c, "foo")
	c.Assert(err, tc.ErrorMatches, `controller foo not found`)
}

func (s *DestroySuite) TestDestroyControllerNotFoundNotRemovedFromStore(c *tc.C) {
	s.apierror = errors.NotFoundf("test1")
	_, err := s.runDestroyCommand(c, "test1", "--no-prompt")
	c.Assert(err, tc.ErrorMatches, "cannot connect to API: test1 not found")
	//c.Check(c.GetTestLog(), tc.Contains, "If the controller is unusable")
	checkControllerExistsInStore(c, "test1", s.store)
}

func (s *DestroySuite) TestDestroyCannotConnectToAPI(c *tc.C) {
	s.apierror = errors.New("connection refused")
	_, err := s.runDestroyCommand(c, "test1", "--no-prompt")
	c.Assert(err, tc.ErrorMatches, "cannot connect to API: connection refused")
	//c.Check(c.GetTestLog(), tc.Contains, "If the controller is unusable")
	checkControllerExistsInStore(c, "test1", s.store)
}

func (s *DestroySuite) TestDestroy(c *tc.C) {
	_, err := s.runDestroyCommand(c, "test1", "--no-prompt")
	c.Assert(err, tc.ErrorIsNil)
	assertControllerRemovedFromStore(c, "test1", s.store)
}

func (s *DestroySuite) TestDestroyAlias(c *tc.C) {
	_, err := s.runDestroyCommand(c, "test1", "--no-prompt")
	c.Assert(err, tc.ErrorIsNil)
	assertControllerRemovedFromStore(c, "test1", s.store)
}

func (s *DestroySuite) TestDestroyWithDestroyAllModelsFlag(c *tc.C) {
	_, err := s.runDestroyCommand(c, "test1", "--no-prompt", "--destroy-all-models")
	c.Assert(err, tc.ErrorIsNil)
	s.api.CheckCallNames(c, "AllModels", "ModelStatus", "DestroyController", "AllModels", "ModelStatus", "Close")
	s.api.CheckCall(c, 2, "DestroyController", apicontroller.DestroyControllerParams{
		DestroyModels: true,
	})
	assertControllerRemovedFromStore(c, "test1", s.store)
}

func (s *DestroySuite) TestDestroyWaitsForHostedModelRemoval(c *tc.C) {
	// Simulate the undertaker needing one extra poll to reap the hosted
	// models after DestroyController returns.
	s.api.delayModelRemoval = true

	envDestroyCalls := s.countEnvironsDestroy()
	ctx, err := s.runDestroyCommand(c, "test1", "--no-prompt", "--destroy-all-models")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*envDestroyCalls, tc.Equals, 1)
	// The wait loop must poll again after DestroyController while the
	// dead hosted models are still present, and only proceed to the
	// controller teardown once they are removed.
	c.Check(s.allModelsCallCount() >= 3, tc.IsTrue)
	// The wait status reports the models that are still present,
	// including the dead ones awaiting removal.
	c.Check(cmdtesting.Stderr(ctx), tc.Contains, "Waiting for 2 models")
}

func (s *DestroySuite) TestDestroyWaitsForHostedModelRemovalWithoutDestroyAllModels(c *tc.C) {
	// The most common workflow: the models were destroyed beforehand and
	// are still present in the controller (dead) when destroy-controller
	// runs. Even without --destroy-all-models the command must wait for
	// the background reap to finish before tearing the controller down.
	s.api.delayModelRemoval = true

	envDestroyCalls := s.countEnvironsDestroy()
	_, err := s.runDestroyCommand(c, "test1", "--no-prompt")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*envDestroyCalls, tc.Equals, 1)
	s.api.CheckCall(c, 2, "DestroyController", apicontroller.DestroyControllerParams{})
	c.Check(s.allModelsCallCount() >= 3, tc.IsTrue)
}

func (s *DestroySuite) TestDestroyWaitsForDyingHostedModelRemoval(c *tc.C) {
	// A hosted model caught mid-destruction (dying, not yet dead) is
	// destroyed along with the rest; the wait loop must hold until it is
	// removed from the controller, not merely until it is dead.
	s.api.delayModelRemoval = true
	status := s.api.modelStatus[test2UUID]
	status.Life = life.Dying
	s.api.modelStatus[test2UUID] = status

	envDestroyCalls := s.countEnvironsDestroy()
	ctx, err := s.runDestroyCommand(c, "test1", "--no-prompt", "--destroy-all-models")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*envDestroyCalls, tc.Equals, 1)
	c.Check(s.allModelsCallCount() >= 3, tc.IsTrue)
	c.Check(cmdtesting.Stderr(ctx), tc.Contains, "Waiting for 2 models")
}

func (s *DestroySuite) TestDestroyWaitsForHostedMachines(c *tc.C) {
	// Machines may remain (for example in the controller model) after
	// the hosted models are gone; the wait loop must hold until they are
	// removed too.
	s.api.delayMachineRemoval = true
	status := s.api.modelStatus[test1UUID]
	status.HostedMachineCount = 1
	s.api.modelStatus[test1UUID] = status

	envDestroyCalls := s.countEnvironsDestroy()
	ctx, err := s.runDestroyCommand(c, "test1", "--no-prompt", "--destroy-all-models")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*envDestroyCalls, tc.Equals, 1)
	c.Check(s.allModelsCallCount() >= 3, tc.IsTrue)
	c.Check(cmdtesting.Stderr(ctx), tc.Contains, "1 machine")
}

func (s *DestroySuite) TestDestroyWithDestroyDestroyStorageFlag(c *tc.C) {
	_, err := s.runDestroyCommand(c, "test1", "--no-prompt", "--destroy-storage")
	c.Assert(err, tc.ErrorIsNil)
	destroyStorage := true
	s.api.CheckCall(c, 2, "DestroyController", apicontroller.DestroyControllerParams{
		DestroyStorage: &destroyStorage,
	})
}

func (s *DestroySuite) TestDestroyWithDestroyTimeout(c *tc.C) {
	_, err := s.runDestroyCommand(c, "test1", "--no-prompt", "--force", "--model-timeout", "30m")
	c.Assert(err, tc.ErrorIsNil)
	timeout := 30 * time.Minute
	force := true
	s.api.CheckCallNames(c, "AllModels", "ModelStatus", "DestroyController", "AllModels", "ModelStatus", "Close")
	s.api.CheckCall(c, 2, "DestroyController", apicontroller.DestroyControllerParams{
		ModelTimeout: &timeout,
		Force:        &force,
	})
}

func (s *DestroySuite) TestDestroyWithDestroyReleaseStorageFlag(c *tc.C) {
	_, err := s.runDestroyCommand(c, "test1", "--no-prompt", "--release-storage")
	c.Assert(err, tc.ErrorIsNil)
	destroyStorage := false
	s.api.CheckCall(c, 2, "DestroyController", apicontroller.DestroyControllerParams{
		DestroyStorage: &destroyStorage,
	})
}

func (s *DestroySuite) TestDestroyWithDestroyDestroyReleaseStorageFlagsMutuallyExclusive(c *tc.C) {
	_, err := s.runDestroyCommand(c, "test1", "--no-prompt", "--destroy-storage", "--release-storage")
	c.Assert(err, tc.ErrorMatches, "--destroy-storage and --release-storage cannot both be specified")
}

func (s *DestroySuite) TestDestroyWithForceFlag(c *tc.C) {
	_, err := s.runDestroyCommand(c, "test1", "--no-prompt", "--force", "--model-timeout", "10m")
	c.Assert(err, tc.ErrorIsNil)
	force := true
	timeout := 10 * time.Minute
	s.api.CheckCall(c, 2, "DestroyController", apicontroller.DestroyControllerParams{
		Force:        &force,
		ModelTimeout: &timeout,
	})
}

func (s *DestroySuite) TestDestroyWithModelTimeoutNoForce(c *tc.C) {
	_, err := s.runDestroyCommand(c, "test1", "--no-prompt", "--model-timeout", "10m")
	c.Assert(err, tc.ErrorMatches, `--model-timeout can only be used with --force \(dangerous\)`)
}

func (s *DestroySuite) TestDestroyWithDestroyDestroyStorageFlagUnspecified(c *tc.C) {
	var haveFilesystem bool
	for uuid, status := range s.api.modelStatus {
		status.Life = life.Alive
		status.Volumes = append(status.Volumes, base.Volume{Detachable: true})
		if !haveFilesystem {
			haveFilesystem = true
			status.Filesystems = append(
				status.Filesystems, base.Filesystem{Detachable: true},
			)
		}
		s.api.modelStatus[uuid] = status
	}

	s.api.SetErrors(
		errors.New("cannot destroy controller \"test1\""),
		&params.Error{Code: params.CodeHasPersistentStorage},
	)
	_, err := s.runDestroyCommand(c, "test1", "--no-prompt", "--destroy-all-models")
	c.Assert(err.Error(), tc.Equals, `cannot destroy controller "test1"

The controller has persistent storage remaining:
	3 volumes and 1 filesystem across 3 models

To destroy the storage, run the destroy-controller
command again with the "--destroy-storage" option.

To release the storage from Juju's management
without destroying it, use the "--release-storage"
option instead. The storage can then be imported
into another Juju model.

`)
}

func (s *DestroySuite) TestDestroyControllerGetFails(c *tc.C) {
	s.controllerModelConfigAPI.SetErrors(errors.NotFoundf(`controller "test3"`))
	_, err := s.runDestroyCommand(c, "test3", "--no-prompt")
	c.Assert(err, tc.ErrorMatches,
		"getting controller environ: getting model config from API: controller \"test3\" not found",
	)
	checkControllerExistsInStore(c, "test3", s.store)
}

func (s *DestroySuite) TestFailedDestroyController(c *tc.C) {
	s.api.SetErrors(
		errors.New("failed to destroy controller \"test1\""),
		errors.New("permission denied"),
	)
	_, err := s.runDestroyCommand(c, "test1", "--no-prompt")
	c.Assert(err, tc.ErrorMatches, "cannot destroy controller: permission denied")
	checkControllerExistsInStore(c, "test1", s.store)
}

func (s *DestroySuite) TestDestroyControllerAliveModels(c *tc.C) {
	for uuid, status := range s.api.modelStatus {
		status.Life = life.Alive
		s.api.modelStatus[uuid] = status
	}
	s.api.SetErrors(
		errors.New("cannot destroy controller \"test1\""),
		&params.Error{Code: params.CodeHasHostedModels},
	)
	_, err := s.runDestroyCommand(c, "test1", "--no-prompt")
	c.Assert(err.Error(), tc.Equals, `cannot destroy controller "test1"

The controller has live models. If you want
to destroy all models in the controller,
run this command again with the --destroy-all-models
option.

Models:
	prod/test2:test2 (alive)
	prod/test3:admin (alive)
`)
}

func (s *DestroySuite) TestDestroyControllerReattempt(c *tc.C) {
	// The first attempt to destroy should yield an error
	// saying that the controller has hosted models. After
	// checking, we find there are only dead hosted models,
	// and reattempt the destroy the controller; this time
	// it succeeds.
	s.api.SetErrors(&params.Error{Code: params.CodeHasHostedModels})
	_, err := s.runDestroyCommand(c, "test1", "--no-prompt")
	c.Assert(err, tc.ErrorIsNil)
	s.api.CheckCallNames(c,
		"AllModels",
		"DestroyController",
		"AllModels",
		"ModelStatus",
		"Close",
	)
}

func (s *DestroySuite) resetController(c *tc.C) {
	s.store.Controllers["test1"] = jujuclient.ControllerDetails{
		APIEndpoints:   []string{"localhost"},
		CACert:         testing.CACert,
		ControllerUUID: test1UUID,
	}
	s.store.Accounts["test1"] = jujuclient.AccountDetails{
		User: "admin",
	}
	s.store.BootstrapConfig["test1"] = jujuclient.BootstrapConfig{
		ControllerModelUUID: test1UUID,
		Config:              createBootstrapInfo(c, "admin"),
		CloudType:           "dummy",
	}
}

func (s *DestroySuite) TestDestroyCommandConfirmation(c *tc.C) {
	var stdin, stdout, stderr bytes.Buffer
	ctx := cmdtesting.Context(c)
	ctx.Stdout = &stdout
	ctx.Stderr = &stderr
	ctx.Stdin = &stdin

	// Ensure confirmation is requested if "--no-prompt" is not specified.
	stdin.WriteString("wrong_test1_name")
	errc := cmdtesting.RunCommandWithContext(ctx, s.newDestroyCommand(), "test1")
	select {
	case err := <-errc:
		c.Check(err, tc.ErrorMatches, "controller destruction: aborted")
	case <-time.After(testing.LongWait):
		c.Fatalf("command took too long")
	}
	//testLog := c.GetTestLog()
	//c.Check(testLog, tc.Matches, "(.|\n)*WARNING.*test1(.|\n)*")
	checkControllerExistsInStore(c, "test1", s.store)

	// EOF on stdin: equivalent to answering no.
	stdin.Reset()
	stdout.Reset()
	stderr.Reset()
	errc = cmdtesting.RunCommandWithContext(ctx, s.newDestroyCommand(), "test1")
	select {
	case err := <-errc:
		c.Check(err, tc.ErrorMatches, "controller destruction: aborted")
	case <-time.After(testing.LongWait):
		c.Fatalf("command took too long")
	}
	//testLog = c.GetTestLog()
	//c.Check(testLog, tc.Matches, "(.|\n)*WARNING.*test1(.|\n)*")
	checkControllerExistsInStore(c, "test1", s.store)

	answer := "test1"
	stdin.Reset()
	stdout.Reset()
	stderr.Reset()
	stdin.WriteString(answer)
	errc = cmdtesting.RunCommandWithContext(ctx, s.newDestroyCommand(), "test1")
	select {
	case err := <-errc:
		c.Check(err, tc.ErrorIsNil)
	case <-time.After(testing.LongWait):
		c.Fatalf("command took too long")
	}
	assertControllerRemovedFromStore(c, "test1", s.store)

	// Add the test1 controller back into the store for the next test
	s.resetController(c)
}

func (s *DestroySuite) TestBlockedDestroy(c *tc.C) {
	s.api.SetErrors(
		errors.New("cannot destroy controller \"test1\""),
		&params.Error{Code: params.CodeOperationBlocked},
	)
	s.runDestroyCommand(c, "test1", "--no-prompt")
	//testLog := c.GetTestLog()
	//c.Check(testLog, tc.Contains, "To enable controller destruction, please run:")
	//c.Check(testLog, tc.Contains, "juju enable-destroy-controller")
}

func (s *DestroySuite) TestDestroyListBlocksError(c *tc.C) {
	s.api.SetErrors(
		errors.New("cannot destroy controller \"test1\""),
		&params.Error{Code: params.CodeOperationBlocked},
		errors.New("unexpected api error"),
	)
	s.runDestroyCommand(c, "test1", "--no-prompt")
	//testLog := c.GetTestLog()
	//c.Check(testLog, tc.Contains, "To enable controller destruction, please run:")
	//c.Check(testLog, tc.Contains, "juju enable-destroy-controller")
	//c.Check(testLog, tc.Contains, "Unable to list models: unexpected api error")
}

func (s *DestroySuite) TestDestroyReturnsBlocks(c *tc.C) {
	s.api.SetErrors(
		errors.New("there are models with disabled commands preventing controller destruction"),
		&params.Error{Code: params.CodeOperationBlocked},
	)
	s.api.blocks = []params.ModelBlockInfo{
		{
			Name:      "test1",
			UUID:      test1UUID,
			Qualifier: "prod",
			Blocks: []string{
				"BlockDestroy",
			},
		},
		{
			Name:      "test2",
			UUID:      test2UUID,
			Qualifier: "staging",
			Blocks: []string{
				"BlockDestroy",
				"BlockChange",
			},
		},
	}
	ctx, _ := s.runDestroyCommand(c, "test1", "--no-prompt", "--destroy-all-models")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "Unable to get the controller summary from the API: there are models with disabled commands preventing controller destruction.\n"+
		"Destroying controller\n"+
		"Name           Model UUID                            Disabled commands\n"+
		"prod/test1     1871299e-1370-4f3e-83ab-1849ed7b1076  destroy-model\n"+
		"staging/test2  c59d0e3b-2bd7-4867-b1b9-f1ef8a0bb004  all, destroy-model\n")
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
}

func (s *DestroySuite) TestGetControllerEnvironWithCaaS(c *tc.C) {
	s.controllerModelConfigAPI.env = createBootstrapInfo(c, "test3")
	// The dummy provider isn't CaaS, so we pretend k8s is a dummy provider for now
	s.api.cloud.Type = "kubernetes"

	_, err := s.runDestroyCommand(c, "test3", "--no-prompt")
	// Make sure we're *not* getting an error during `getControllerEnviron`
	// We'll still get an error from the k8s provider since nothing is set up, but that is expected
	c.Assert(err, tc.Not(tc.ErrorMatches),
		"getting controller environ: cloud environ provider kubernetes.kubernetesEnvironProvider not valid",
	)
	checkControllerExistsInStore(c, "test3", s.store)
}
