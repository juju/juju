// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"bytes"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/cmd/modelcmd"
	cmdtesting "github.com/juju/juju/cmd/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
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
	api       *fakeDestroyAPI
	clientapi *fakeDestroyAPIClient
	store     *jujuclienttesting.MemStore
	apierror  error
}

// fakeDestroyAPI mocks out the controller API
type fakeDestroyAPI struct {
	gitjujutesting.Stub
	cloud      environs.CloudSpec
	env        map[string]interface{}
	destroyAll bool
	blocks     []params.ModelBlockInfo
	envStatus  map[string]base.ModelStatus
	allModels  []base.UserModel
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

func (f *fakeDestroyAPI) DestroyController(destroyAll bool) error {
	f.MethodCall(f, "DestroyController", destroyAll)
	f.destroyAll = destroyAll
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
		cloud:     dummy.SampleCloudSpec(),
		envStatus: map[string]base.ModelStatus{},
	}
	s.apierror = nil

	s.store = jujuclienttesting.NewMemStore()
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
		User: "admin@local",
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
			Owner: owner.Canonical(),
		})
		s.api.envStatus[model.modelUUID] = base.ModelStatus{
			UUID:               uuid,
			Life:               string(params.Dead),
			HostedMachineCount: 0,
			ServiceCount:       0,
			Owner:              owner.Canonical(),
		}
	}
}

func (s *DestroySuite) runDestroyCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	return testing.RunCommand(c, s.newDestroyCommand(), args...)
}

func (s *DestroySuite) newDestroyCommand() cmd.Command {
	return controller.NewDestroyCommandForTest(s.api, s.clientapi, s.store, s.apierror)
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
	c.Assert(s.api.destroyAll, jc.IsFalse)
	c.Assert(s.clientapi.destroycalled, jc.IsFalse)
	checkControllerRemovedFromStore(c, "test1", s.store)
}

func (s *DestroySuite) TestDestroyAlias(c *gc.C) {
	_, err := s.runDestroyCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.destroyAll, jc.IsFalse)
	c.Assert(s.clientapi.destroycalled, jc.IsFalse)
	checkControllerRemovedFromStore(c, "test1", s.store)
}

func (s *DestroySuite) TestDestroyWithDestroyAllModelsFlag(c *gc.C) {
	_, err := s.runDestroyCommand(c, "test1", "-y", "--destroy-all-models")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.destroyAll, jc.IsTrue)
	checkControllerRemovedFromStore(c, "test1", s.store)
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
	c.Assert(s.api.destroyAll, jc.IsFalse)
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
	owner@local/test2:test2 (alive)
	owner@local/test3:admin (alive)
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
		"ModelStatus",
		"DestroyController",
		"AllModels",
		"ModelStatus",
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
		User: "admin@local",
	}
	s.store.BootstrapConfig["test1"] = jujuclient.BootstrapConfig{
		ControllerModelUUID: test1UUID,
		Config:              createBootstrapInfo(c, "admin"),
		CloudType:           "dummy",
	}
}

func (s *DestroySuite) TestDestroyCommandConfirmation(c *gc.C) {
	var stdin, stdout bytes.Buffer
	ctx := testing.Context(c)
	ctx.Stdout = &stdout
	ctx.Stdin = &stdin

	// Ensure confirmation is requested if "-y" is not specified.
	stdin.WriteString("n")
	_, errc := cmdtesting.RunCommand(ctx, s.newDestroyCommand(), "test1")
	select {
	case err := <-errc:
		c.Check(err, gc.ErrorMatches, "controller destruction aborted")
	case <-time.After(testing.LongWait):
		c.Fatalf("command took too long")
	}
	c.Check(testing.Stdout(ctx), gc.Matches, "WARNING!.*test1(.|\n)*")
	checkControllerExistsInStore(c, "test1", s.store)

	// EOF on stdin: equivalent to answering no.
	stdin.Reset()
	stdout.Reset()
	_, errc = cmdtesting.RunCommand(ctx, s.newDestroyCommand(), "test1")
	select {
	case err := <-errc:
		c.Check(err, gc.ErrorMatches, "controller destruction aborted")
	case <-time.After(testing.LongWait):
		c.Fatalf("command took too long")
	}
	c.Check(testing.Stdout(ctx), gc.Matches, "WARNING!.*test1(.|\n)*")
	checkControllerExistsInStore(c, "test1", s.store)

	for _, answer := range []string{"y", "Y", "yes", "YES"} {
		stdin.Reset()
		stdout.Reset()
		stdin.WriteString(answer)
		_, errc = cmdtesting.RunCommand(ctx, s.newDestroyCommand(), "test1")
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
		params.ModelBlockInfo{
			Name:     "test1",
			UUID:     test1UUID,
			OwnerTag: "user-cheryl@local",
			Blocks: []string{
				"BlockDestroy",
			},
		},
		params.ModelBlockInfo{
			Name:     "test2",
			UUID:     test2UUID,
			OwnerTag: "user-bob@local",
			Blocks: []string{
				"BlockDestroy",
				"BlockChange",
			},
		},
	}
	ctx, _ := s.runDestroyCommand(c, "test1", "-y", "--destroy-all-models")
	c.Assert(testing.Stderr(ctx), gc.Equals, "Destroying controller\n"+
		"NAME   MODEL UUID                            OWNER         DISABLED COMMANDS\n"+
		"test1  1871299e-1370-4f3e-83ab-1849ed7b1076  cheryl@local  destroy-model\n"+
		"test2  c59d0e3b-2bd7-4867-b1b9-f1ef8a0bb004  bob@local     all, destroy-model\n")
	c.Assert(testing.Stdout(ctx), gc.Equals, "")
}
