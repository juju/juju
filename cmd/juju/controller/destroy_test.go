// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"bytes"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	apicontroller "github.com/juju/juju/api/controller"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/cmdtest"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
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

var _ = gc.Suite(&DestroySuite{})

type baseDestroySuite struct {
	testing.FakeJujuXDGDataHomeSuite
	api        *fakeDestroyAPI
	clientapi  *fakeDestroyAPIClient
	storageAPI *mockStorageAPI
	store      *jujuclient.MemStore
	apierror   error

	controllerCredentialAPI *mockCredentialAPI
	environsDestroy         func(string, environs.ControllerDestroyer, context.ProviderCallContext, jujuclient.ControllerStore) error
}

// fakeDestroyAPI mocks out the controller API
type fakeDestroyAPI struct {
	gitjujutesting.Stub
	cloud          environs.CloudSpec
	env            map[string]interface{}
	blocks         []params.ModelBlockInfo
	envStatus      map[string]base.ModelStatus
	allModels      []base.UserModel
	hostedConfig   []apicontroller.HostedConfig
	bestAPIVersion int
}

func (f *fakeDestroyAPI) BestAPIVersion() int {
	return f.bestAPIVersion
}

func (f *fakeDestroyAPI) Close() error {
	f.MethodCall(f, "Close")
	return f.NextErr()
}

func (f *fakeDestroyAPI) CloudSpec(tag names.ModelTag) (environs.CloudSpec, error) {
	f.MethodCall(f, "CloudSpec", tag)
	if err := f.NextErr(); err != nil {
		return environs.CloudSpec{}, err
	}
	return f.cloud, nil
}

func (f *fakeDestroyAPI) ModelConfig() (map[string]interface{}, error) {
	f.MethodCall(f, "ModelConfig")
	if err := f.NextErr(); err != nil {
		return nil, err
	}
	return f.env, nil
}

func (f *fakeDestroyAPI) HostedModelConfigs() ([]apicontroller.HostedConfig, error) {
	f.MethodCall(f, "HostedModelConfigs")
	if err := f.NextErr(); err != nil {
		return nil, err
	}
	return f.hostedConfig, nil
}

func (f *fakeDestroyAPI) DestroyController(args apicontroller.DestroyControllerParams) error {
	f.MethodCall(f, "DestroyController", args)
	return f.NextErr()
}

func (f *fakeDestroyAPI) ListBlockedModels() ([]params.ModelBlockInfo, error) {
	f.MethodCall(f, "ListBlockedModels")
	return f.blocks, f.NextErr()
}

func (f *fakeDestroyAPI) ModelStatus(tags ...names.ModelTag) ([]base.ModelStatus, error) {
	f.MethodCall(f, "ModelStatus", tags)
	status := make([]base.ModelStatus, len(tags))
	for i, tag := range tags {
		status[i] = f.envStatus[tag.Id()]
	}
	return status, f.NextErr()
}

func (f *fakeDestroyAPI) AllModels() ([]base.UserModel, error) {
	f.MethodCall(f, "AllModels")
	return f.allModels, f.NextErr()
}

// fakeDestroyAPIClient mocks out the client API
type fakeDestroyAPIClient struct {
	err            error
	modelgetcalled bool
	destroycalled  bool
}

func (f *fakeDestroyAPIClient) Close() error { return nil }

func (f *fakeDestroyAPIClient) ModelGet() (map[string]interface{}, error) {
	f.modelgetcalled = true
	if f.err != nil {
		return nil, f.err
	}
	return map[string]interface{}{}, nil
}

func (f *fakeDestroyAPIClient) DestroyModel() error {
	f.destroycalled = true
	return f.err
}

func createBootstrapInfo(c *gc.C, name string) map[string]interface{} {
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"type":       "dummy",
		"name":       name,
		"uuid":       testing.ModelTag.Id(),
		"controller": "true",
	})
	c.Assert(err, jc.ErrorIsNil)
	return cfg.AllAttrs()
}

func (s *baseDestroySuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.clientapi = &fakeDestroyAPIClient{}
	owner := names.NewUserTag("owner")
	s.api = &fakeDestroyAPI{
		cloud:          dummy.SampleCloudSpec(),
		envStatus:      map[string]base.ModelStatus{},
		bestAPIVersion: 4,
	}
	s.apierror = nil

	s.storageAPI = &mockStorageAPI{}
	s.controllerCredentialAPI = &mockCredentialAPI{}
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
			Life:               string(params.Dead),
			HostedMachineCount: 0,
			ApplicationCount:   0,
			Owner:              owner.Id(),
		}
	}
}

func (s *DestroySuite) runDestroyCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, s.newDestroyCommand(), args...)
}

func (s *DestroySuite) newDestroyCommand() cmd.Command {
	return controller.NewDestroyCommandForTest(
		s.api, s.clientapi, s.storageAPI, s.store, s.apierror,
		func() (controller.CredentialAPI, error) { return s.controllerCredentialAPI, nil },
		s.environsDestroy,
	)
}

func checkControllerExistsInStore(c *gc.C, name string, store jujuclient.ControllerGetter) {
	_, err := store.ControllerByName(name)
	c.Assert(err, jc.ErrorIsNil)
}

func checkControllerRemovedFromStore(c *gc.C, name string, store jujuclient.ControllerGetter) {
	_, err := store.ControllerByName(name)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *DestroySuite) TestDestroyNoControllerNameError(c *gc.C) {
	_, err := s.runDestroyCommand(c)
	c.Assert(err, gc.ErrorMatches, "no controller specified")
}

func (s *DestroySuite) TestDestroyBadFlags(c *gc.C) {
	_, err := s.runDestroyCommand(c, "-n")
	c.Assert(err, gc.ErrorMatches, "flag provided but not defined: -n")
}

func (s *DestroySuite) TestDestroyUnknownArgument(c *gc.C) {
	_, err := s.runDestroyCommand(c, "model", "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}

func (s *DestroySuite) TestDestroyUnknownController(c *gc.C) {
	_, err := s.runDestroyCommand(c, "foo")
	c.Assert(err, gc.ErrorMatches, `controller foo not found`)
}

func (s *DestroySuite) TestDestroyControllerNotFoundNotRemovedFromStore(c *gc.C) {
	s.apierror = errors.NotFoundf("test1")
	_, err := s.runDestroyCommand(c, "test1", "-y")
	c.Assert(err, gc.ErrorMatches, "cannot connect to API: test1 not found")
	c.Check(c.GetTestLog(), jc.Contains, "If the controller is unusable")
	checkControllerExistsInStore(c, "test1", s.store)
}

func (s *DestroySuite) TestDestroyCannotConnectToAPI(c *gc.C) {
	s.apierror = errors.New("connection refused")
	_, err := s.runDestroyCommand(c, "test1", "-y")
	c.Assert(err, gc.ErrorMatches, "cannot connect to API: connection refused")
	c.Check(c.GetTestLog(), jc.Contains, "If the controller is unusable")
	checkControllerExistsInStore(c, "test1", s.store)
}

func (s *DestroySuite) TestDestroy(c *gc.C) {
	_, err := s.runDestroyCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.clientapi.destroycalled, jc.IsFalse)
	checkControllerRemovedFromStore(c, "test1", s.store)
}

func (s *DestroySuite) TestDestroyAlias(c *gc.C) {
	_, err := s.runDestroyCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.clientapi.destroycalled, jc.IsFalse)
	checkControllerRemovedFromStore(c, "test1", s.store)
}

func (s *DestroySuite) TestDestroyWithDestroyAllModelsFlag(c *gc.C) {
	_, err := s.runDestroyCommand(c, "test1", "-y", "--destroy-all-models")
	c.Assert(err, jc.ErrorIsNil)
	s.api.CheckCallNames(c, "DestroyController", "AllModels", "ModelStatus", "Close")
	s.api.CheckCall(c, 0, "DestroyController", apicontroller.DestroyControllerParams{
		DestroyModels: true,
	})
	checkControllerRemovedFromStore(c, "test1", s.store)
}

func (s *DestroySuite) TestDestroyWithDestroyDestroyStorageFlag(c *gc.C) {
	_, err := s.runDestroyCommand(c, "test1", "-y", "--destroy-storage")
	c.Assert(err, jc.ErrorIsNil)
	destroyStorage := true
	s.api.CheckCall(c, 0, "DestroyController", apicontroller.DestroyControllerParams{
		DestroyStorage: &destroyStorage,
	})
}

func (s *DestroySuite) TestDestroyWithDestroyReleaseStorageFlag(c *gc.C) {
	_, err := s.runDestroyCommand(c, "test1", "-y", "--release-storage")
	c.Assert(err, jc.ErrorIsNil)
	destroyStorage := false
	s.api.CheckCall(c, 0, "DestroyController", apicontroller.DestroyControllerParams{
		DestroyStorage: &destroyStorage,
	})
}

func (s *DestroySuite) TestDestroyWithDestroyDestroyReleaseStorageFlagsMutuallyExclusive(c *gc.C) {
	_, err := s.runDestroyCommand(c, "test1", "-y", "--destroy-storage", "--release-storage")
	c.Assert(err, gc.ErrorMatches, "--destroy-storage and --release-storage cannot both be specified")
}

func (s *DestroySuite) TestDestroyWithDestroyDestroyStorageFlagUnspecified(c *gc.C) {
	var haveFilesystem bool
	for uuid, status := range s.api.envStatus {
		status.Life = string(params.Alive)
		status.Volumes = append(status.Volumes, base.Volume{Detachable: true})
		if !haveFilesystem {
			haveFilesystem = true
			status.Filesystems = append(
				status.Filesystems, base.Filesystem{Detachable: true},
			)
		}
		s.api.envStatus[uuid] = status
	}

	s.api.SetErrors(&params.Error{Code: params.CodeHasPersistentStorage})
	_, err := s.runDestroyCommand(c, "test1", "-y", "--destroy-all-models")
	c.Assert(err.Error(), gc.Equals, `cannot destroy controller "test1"

The controller has persistent storage remaining:
	3 volumes and 1 filesystem across 3 models

To destroy the storage, run the destroy-controller
command again with the "--destroy-storage" flag.

To release the storage from Juju's management
without destroying it, use the "--release-storage"
flag instead. The storage can then be imported
into another Juju model.

`)
}

func (s *DestroySuite) TestDestroyWithDestroyDestroyStorageFlagUnspecifiedOldController(c *gc.C) {
	s.api.bestAPIVersion = 3
	s.storageAPI.storage = []params.StorageDetails{{}}

	_, err := s.runDestroyCommand(c, "test1", "-y")
	c.Assert(err, gc.ErrorMatches, `cannot destroy controller "test1"

Destroying this controller will destroy the storage,
but you have not indicated that you want to do that.

Please run the the command again with --destroy-storage
to confirm that you want to destroy the storage along
with the controller.

If instead you want to keep the storage, you must first
upgrade the controller to version 2.3 or greater.

`)
}

func (s *DestroySuite) TestDestroyWithDestroyDestroyStorageFlagUnspecifiedOldControllerNoStorage(c *gc.C) {
	s.api.bestAPIVersion = 3
	s.storageAPI.storage = nil // no storage

	_, err := s.runDestroyCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DestroySuite) TestDestroyControllerGetFails(c *gc.C) {
	s.api.SetErrors(errors.NotFoundf(`controller "test3"`))
	_, err := s.runDestroyCommand(c, "test3", "-y")
	c.Assert(err, gc.ErrorMatches,
		"getting controller environ: getting model config from API: controller \"test3\" not found",
	)
	checkControllerExistsInStore(c, "test3", s.store)
}

func (s *DestroySuite) TestFailedDestroyController(c *gc.C) {
	s.api.SetErrors(errors.New("permission denied"))
	_, err := s.runDestroyCommand(c, "test1", "-y")
	c.Assert(err, gc.ErrorMatches, "cannot destroy controller: permission denied")
	checkControllerExistsInStore(c, "test1", s.store)
}

func (s *DestroySuite) TestDestroyControllerAliveModels(c *gc.C) {
	for uuid, status := range s.api.envStatus {
		status.Life = string(params.Alive)
		s.api.envStatus[uuid] = status
	}
	s.api.SetErrors(&params.Error{Code: params.CodeHasHostedModels})
	_, err := s.runDestroyCommand(c, "test1", "-y")
	c.Assert(err.Error(), gc.Equals, `cannot destroy controller "test1"

The controller has live hosted models. If you want
to destroy all hosted models in the controller,
run this command again with the --destroy-all-models
flag.

Models:
	owner/test2:test2 (alive)
	owner/test3:admin (alive)
`)

}

func (s *DestroySuite) TestDestroyControllerReattempt(c *gc.C) {
	// The first attempt to destroy should yield an error
	// saying that the controller has hosted models. After
	// checking, we find there are only dead hosted models,
	// and reattempt the destroy the controller; this time
	// it succeeds.
	s.api.SetErrors(&params.Error{Code: params.CodeHasHostedModels})
	_, err := s.runDestroyCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	s.api.CheckCallNames(c,
		"DestroyController",
		"AllModels",
		"ModelStatus",
		"DestroyController",
		"AllModels",
		"ModelStatus",
		"Close",
	)
}

func (s *DestroySuite) resetController(c *gc.C) {
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

func (s *DestroySuite) TestDestroyCommandConfirmation(c *gc.C) {
	var stdin, stdout bytes.Buffer
	ctx := cmdtesting.Context(c)
	ctx.Stdout = &stdout
	ctx.Stdin = &stdin

	// Ensure confirmation is requested if "-y" is not specified.
	stdin.WriteString("n")
	_, errc := cmdtest.RunCommandWithDummyProvider(ctx, s.newDestroyCommand(), "test1")
	select {
	case err := <-errc:
		c.Check(err, gc.ErrorMatches, "controller destruction aborted")
	case <-time.After(testing.LongWait):
		c.Fatalf("command took too long")
	}
	c.Check(cmdtesting.Stdout(ctx), gc.Matches, "WARNING!.*test1(.|\n)*")
	checkControllerExistsInStore(c, "test1", s.store)

	// EOF on stdin: equivalent to answering no.
	stdin.Reset()
	stdout.Reset()
	_, errc = cmdtest.RunCommandWithDummyProvider(ctx, s.newDestroyCommand(), "test1")
	select {
	case err := <-errc:
		c.Check(err, gc.ErrorMatches, "controller destruction aborted")
	case <-time.After(testing.LongWait):
		c.Fatalf("command took too long")
	}
	c.Check(cmdtesting.Stdout(ctx), gc.Matches, "WARNING!.*test1(.|\n)*")
	checkControllerExistsInStore(c, "test1", s.store)

	for _, answer := range []string{"y", "Y", "yes", "YES"} {
		stdin.Reset()
		stdout.Reset()
		stdin.WriteString(answer)
		_, errc = cmdtest.RunCommandWithDummyProvider(ctx, s.newDestroyCommand(), "test1")
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
}

func (s *DestroySuite) TestBlockedDestroy(c *gc.C) {
	s.api.SetErrors(&params.Error{Code: params.CodeOperationBlocked})
	s.runDestroyCommand(c, "test1", "-y")
	testLog := c.GetTestLog()
	c.Check(testLog, jc.Contains, "To enable controller destruction, please run:")
	c.Check(testLog, jc.Contains, "juju enable-destroy-controller")
}

func (s *DestroySuite) TestDestroyListBlocksError(c *gc.C) {
	s.api.SetErrors(
		&params.Error{Code: params.CodeOperationBlocked},
		errors.New("unexpected api error"),
	)
	s.runDestroyCommand(c, "test1", "-y")
	testLog := c.GetTestLog()
	c.Check(testLog, jc.Contains, "To enable controller destruction, please run:")
	c.Check(testLog, jc.Contains, "juju enable-destroy-controller")
	c.Check(testLog, jc.Contains, "Unable to list models: unexpected api error")
}

func (s *DestroySuite) TestDestroyReturnsBlocks(c *gc.C) {
	s.api.SetErrors(&params.Error{Code: params.CodeOperationBlocked})
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
	ctx, _ := s.runDestroyCommand(c, "test1", "-y", "--destroy-all-models")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "Destroying controller\n"+
		"Name   Model UUID                            Owner   Disabled commands\n"+
		"test1  1871299e-1370-4f3e-83ab-1849ed7b1076  cheryl  destroy-model\n"+
		"test2  c59d0e3b-2bd7-4867-b1b9-f1ef8a0bb004  bob     all, destroy-model\n")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}

func (s *DestroySuite) TestDestroyWithInvalidCredentialCallbackExecutingSuccessfully(c *gc.C) {
	s.destroyAndInvalidateCredential(c)
}

func (s *DestroySuite) destroyAndInvalidateCredential(c *gc.C) {
	s.destroyAndInvalidateCredentialWithError(c, "")
}

func (s *DestroySuite) destroyAndInvalidateCredentialWithError(c *gc.C, expectedErr string) {
	called := false
	// Make sure that the invalidate credential callback in the cloud context
	// is called.
	s.environsDestroy = func(controllerName string,
		env environs.ControllerDestroyer,
		ctx context.ProviderCallContext,
		store jujuclient.ControllerStore,
	) error {
		called = true
		err := ctx.InvalidateCredential("testing now")
		if expectedErr == "" {
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, expectedErr)
		}
		return environs.Destroy(controllerName, env, ctx, store)
	}
	_, err := s.runDestroyCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	s.controllerCredentialAPI.CheckCallNames(c, "InvalidateModelCredential", "Close")
}

func (s *DestroySuite) TestDestroyWithInvalidCredentialCallbackFailing(c *gc.C) {
	msg := "unexpected creds callback error"
	s.controllerCredentialAPI.SetErrors(errors.New(msg))
	// As we are throwing the error on within the callback,
	// the actual call to destroy should succeed.
	s.destroyAndInvalidateCredentialWithError(c, msg)
}

func (s *DestroySuite) TestDestroyWithInvalidCredentialCallbackFailingToCloseAPI(c *gc.C) {
	s.controllerCredentialAPI.SetErrors(
		nil, // call to invalidate credential succeeds
		errors.New("unexpected creds callback error"), // call to close api client fails
	)
	// As we are throwing the error on api.Close for callback,
	// the actual call to destroy should succeed.
	s.destroyAndInvalidateCredential(c)
}

type mockStorageAPI struct {
	gitjujutesting.Stub
	storage []params.StorageDetails
}

func (m *mockStorageAPI) Close() error {
	m.MethodCall(m, "Close")
	return m.NextErr()
}

func (m *mockStorageAPI) ListStorageDetails() ([]params.StorageDetails, error) {
	m.MethodCall(m, "ListStorageDetails")
	return m.storage, m.NextErr()
}

type mockCredentialAPI struct {
	gitjujutesting.Stub
}

func (m *mockCredentialAPI) InvalidateModelCredential(reason string) error {
	m.MethodCall(m, "InvalidateModelCredential", reason)
	return m.NextErr()
}

func (m *mockCredentialAPI) Close() error {
	m.MethodCall(m, "Close")
	return m.NextErr()
}
