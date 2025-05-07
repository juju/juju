// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"bytes"
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/api/base"
	apicontroller "github.com/juju/juju/api/controller/controller"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/cmd/modelcmd"
	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	_ "github.com/juju/juju/internal/provider/dummy"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
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

var _ = tc.Suite(&DestroySuite{})

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
	jujutesting.Stub
	cloud        environscloudspec.CloudSpec
	blocks       []params.ModelBlockInfo
	envStatus    map[string]base.ModelStatus
	allModels    []base.UserModel
	hostedConfig []apicontroller.HostedConfig
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
	return f.NextErr()
}

func (f *fakeDestroyAPI) ListBlockedModels(ctx context.Context) ([]params.ModelBlockInfo, error) {
	f.MethodCall(f, "ListBlockedModels")
	return f.blocks, f.NextErr()
}

func (f *fakeDestroyAPI) ModelStatus(_ context.Context, tags ...names.ModelTag) ([]base.ModelStatus, error) {
	f.MethodCall(f, "ModelStatus", tags)
	status := make([]base.ModelStatus, len(tags))
	for i, tag := range tags {
		status[i] = f.envStatus[tag.Id()]
	}
	return status, f.NextErr()
}

func (f *fakeDestroyAPI) AllModels(ctx context.Context) ([]base.UserModel, error) {
	f.MethodCall(f, "AllModels")
	return f.allModels, f.NextErr()
}

// fakeModelConfigAPI mocks out the controller model config API
type fakeModelConfigAPI struct {
	jujutesting.Stub
	env map[string]interface{}
}

func (f *fakeModelConfigAPI) Close() error {
	f.MethodCall(f, "Close")
	return f.NextErr()
}

func (f *fakeModelConfigAPI) ModelGet(ctx context.Context) (map[string]interface{}, error) {
	f.MethodCall(f, "ModelGet")
	if err := f.NextErr(); err != nil {
		return nil, err
	}
	return f.env, nil
}

func createBootstrapInfo(c *tc.C, name string) map[string]interface{} {
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"type":       "dummy",
		"name":       name,
		"uuid":       testing.ModelTag.Id(),
		"controller": "true",
	})
	c.Assert(err, jc.ErrorIsNil)
	return cfg.AllAttrs()
}

func (s *baseDestroySuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	owner := names.NewUserTag("owner")
	s.api = &fakeDestroyAPI{
		cloud:     testing.FakeCloudSpec(),
		envStatus: map[string]base.ModelStatus{},
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
		bootstrapCfg   map[string]interface{}
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
			Name:  model.name,
			UUID:  uuid,
			Owner: owner.Id(),
		})
		s.api.envStatus[model.modelUUID] = base.ModelStatus{
			UUID:               uuid,
			Life:               life.Dead,
			HostedMachineCount: 0,
			ApplicationCount:   0,
			Owner:              owner.Id(),
		}
	}
}

func (s *DestroySuite) runDestroyCommand(c *tc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, s.newDestroyCommand(), args...)
}

func (s *DestroySuite) newDestroyCommand() cmd.Command {
	return controller.NewDestroyCommandForTest(
		s.api, s.store, s.apierror, s.controllerModelConfigAPI,
		s.environsDestroy,
	)
}

func checkControllerExistsInStore(c *tc.C, name string, store jujuclient.ControllerGetter) {
	_, err := store.ControllerByName(name)
	c.Assert(err, jc.ErrorIsNil)
}

func checkControllerRemovedFromStore(c *tc.C, name string, store jujuclient.ControllerGetter) {
	_, err := store.ControllerByName(name)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
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
	c.Check(c.GetTestLog(), jc.Contains, "If the controller is unusable")
	checkControllerExistsInStore(c, "test1", s.store)
}

func (s *DestroySuite) TestDestroyCannotConnectToAPI(c *tc.C) {
	s.apierror = errors.New("connection refused")
	_, err := s.runDestroyCommand(c, "test1", "--no-prompt")
	c.Assert(err, tc.ErrorMatches, "cannot connect to API: connection refused")
	c.Check(c.GetTestLog(), jc.Contains, "If the controller is unusable")
	checkControllerExistsInStore(c, "test1", s.store)
}

func (s *DestroySuite) TestDestroy(c *tc.C) {
	_, err := s.runDestroyCommand(c, "test1", "--no-prompt")
	c.Assert(err, jc.ErrorIsNil)
	checkControllerRemovedFromStore(c, "test1", s.store)
}

func (s *DestroySuite) TestDestroyAlias(c *tc.C) {
	_, err := s.runDestroyCommand(c, "test1", "--no-prompt")
	c.Assert(err, jc.ErrorIsNil)
	checkControllerRemovedFromStore(c, "test1", s.store)
}

func (s *DestroySuite) TestDestroyWithDestroyAllModelsFlag(c *tc.C) {
	_, err := s.runDestroyCommand(c, "test1", "--no-prompt", "--destroy-all-models")
	c.Assert(err, jc.ErrorIsNil)
	s.api.CheckCallNames(c, "AllModels", "ModelStatus", "DestroyController", "AllModels", "ModelStatus", "Close")
	s.api.CheckCall(c, 2, "DestroyController", apicontroller.DestroyControllerParams{
		DestroyModels: true,
	})
	checkControllerRemovedFromStore(c, "test1", s.store)
}

func (s *DestroySuite) TestDestroyWithDestroyDestroyStorageFlag(c *tc.C) {
	_, err := s.runDestroyCommand(c, "test1", "--no-prompt", "--destroy-storage")
	c.Assert(err, jc.ErrorIsNil)
	destroyStorage := true
	s.api.CheckCall(c, 2, "DestroyController", apicontroller.DestroyControllerParams{
		DestroyStorage: &destroyStorage,
	})
}

func (s *DestroySuite) TestDestroyWithDestroyTimeout(c *tc.C) {
	_, err := s.runDestroyCommand(c, "test1", "--no-prompt", "--force", "--model-timeout", "30m")
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
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
	for uuid, status := range s.api.envStatus {
		status.Life = life.Alive
		status.Volumes = append(status.Volumes, base.Volume{Detachable: true})
		if !haveFilesystem {
			haveFilesystem = true
			status.Filesystems = append(
				status.Filesystems, base.Filesystem{Detachable: true},
			)
		}
		s.api.envStatus[uuid] = status
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
	for uuid, status := range s.api.envStatus {
		status.Life = life.Alive
		s.api.envStatus[uuid] = status
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
	owner/test2:test2 (alive)
	owner/test3:admin (alive)
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
	c.Assert(err, jc.ErrorIsNil)
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
	testLog := c.GetTestLog()
	c.Check(testLog, tc.Matches, "(.|\n)*WARNING.*test1(.|\n)*")
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
	testLog = c.GetTestLog()
	c.Check(testLog, tc.Matches, "(.|\n)*WARNING.*test1(.|\n)*")
	checkControllerExistsInStore(c, "test1", s.store)

	answer := "test1"
	stdin.Reset()
	stdout.Reset()
	stderr.Reset()
	stdin.WriteString(answer)
	errc = cmdtesting.RunCommandWithContext(ctx, s.newDestroyCommand(), "test1")
	select {
	case err := <-errc:
		c.Check(err, jc.ErrorIsNil)
	case <-time.After(testing.LongWait):
		c.Fatalf("command took too long")
	}
	checkControllerRemovedFromStore(c, "test1", s.store)

	// Add the test1 controller back into the store for the next test
	s.resetController(c)
}

func (s *DestroySuite) TestBlockedDestroy(c *tc.C) {
	s.api.SetErrors(
		errors.New("cannot destroy controller \"test1\""),
		&params.Error{Code: params.CodeOperationBlocked},
	)
	s.runDestroyCommand(c, "test1", "--no-prompt")
	testLog := c.GetTestLog()
	c.Check(testLog, jc.Contains, "To enable controller destruction, please run:")
	c.Check(testLog, jc.Contains, "juju enable-destroy-controller")
}

func (s *DestroySuite) TestDestroyListBlocksError(c *tc.C) {
	s.api.SetErrors(
		errors.New("cannot destroy controller \"test1\""),
		&params.Error{Code: params.CodeOperationBlocked},
		errors.New("unexpected api error"),
	)
	s.runDestroyCommand(c, "test1", "--no-prompt")
	testLog := c.GetTestLog()
	c.Check(testLog, jc.Contains, "To enable controller destruction, please run:")
	c.Check(testLog, jc.Contains, "juju enable-destroy-controller")
	c.Check(testLog, jc.Contains, "Unable to list models: unexpected api error")
}

func (s *DestroySuite) TestDestroyReturnsBlocks(c *tc.C) {
	s.api.SetErrors(
		errors.New("there are models with disabled commands preventing controller destruction"),
		&params.Error{Code: params.CodeOperationBlocked},
	)
	s.api.blocks = []params.ModelBlockInfo{
		{
			Name:     "test1",
			UUID:     test1UUID,
			OwnerTag: "user-cheryl",
			Blocks: []string{
				"BlockDestroy",
			},
		},
		{
			Name:     "test2",
			UUID:     test2UUID,
			OwnerTag: "user-bob",
			Blocks: []string{
				"BlockDestroy",
				"BlockChange",
			},
		},
	}
	ctx, _ := s.runDestroyCommand(c, "test1", "--no-prompt", "--destroy-all-models")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "Unable to get the controller summary from the API: there are models with disabled commands preventing controller destruction.\n"+
		"Destroying controller\n"+
		"Name   Model UUID                            Owner   Disabled commands\n"+
		"test1  1871299e-1370-4f3e-83ab-1849ed7b1076  cheryl  destroy-model\n"+
		"test2  c59d0e3b-2bd7-4867-b1b9-f1ef8a0bb004  bob     all, destroy-model\n")
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
}
