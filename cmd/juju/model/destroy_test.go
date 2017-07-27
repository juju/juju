// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"bytes"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/cmdtest"
	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type DestroySuite struct {
	testing.FakeJujuXDGDataHomeSuite
	api             *fakeAPI
	configAPI       *fakeConfigAPI
	stub            *jutesting.Stub
	budgetAPIClient *mockBudgetAPIClient
	store           *jujuclient.MemStore
	sleep           func(time.Duration)
}

var _ = gc.Suite(&DestroySuite{})

// fakeDestroyAPI mocks out the cient API
type fakeAPI struct {
	*jutesting.Stub
	err             error
	env             map[string]interface{}
	statusCallCount int
	bestAPIVersion  int
	modelInfoErr    []*params.Error
}

func (f *fakeAPI) Close() error { return nil }

func (f *fakeAPI) BestAPIVersion() int {
	return f.bestAPIVersion
}

func (f *fakeAPI) DestroyModel(tag names.ModelTag, destroyStorage *bool) error {
	f.MethodCall(f, "DestroyModel", tag, destroyStorage)
	return f.NextErr()
}

func (f *fakeAPI) ModelStatus(models ...names.ModelTag) ([]base.ModelStatus, error) {
	var err error
	if f.statusCallCount < len(f.modelInfoErr) {
		modelInfoErr := f.modelInfoErr[f.statusCallCount]
		if modelInfoErr != nil {
			err = modelInfoErr
		}
	} else {
		err = &params.Error{Code: params.CodeNotFound}
	}
	f.statusCallCount++
	return []base.ModelStatus{{
		Volumes: []base.Volume{
			{Detachable: true},
			{Detachable: true},
		},
		Filesystems: []base.Filesystem{{Detachable: true}},
	}}, err
}

// faceConfiAPI mocks out the ModelConfigAPI.
type fakeConfigAPI struct {
	err      error
	slaLevel string
}

func (f *fakeConfigAPI) SLALevel() (string, error) {
	return f.slaLevel, f.err
}

func (f *fakeConfigAPI) Close() error { return nil }

func (s *DestroySuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.stub = &jutesting.Stub{}
	s.api = &fakeAPI{
		Stub:           s.stub,
		bestAPIVersion: 4,
	}
	s.configAPI = &fakeConfigAPI{}

	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = "test1"
	s.store.Controllers["test1"] = jujuclient.ControllerDetails{ControllerUUID: "test1-uuid"}
	s.store.Models["test1"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{
			"admin/test1": {"test1-uuid"},
			"admin/test2": {"test2-uuid"},
		},
	}
	s.store.Accounts["test1"] = jujuclient.AccountDetails{
		User: "admin",
	}
	s.sleep = func(time.Duration) {}

	s.budgetAPIClient = &mockBudgetAPIClient{Stub: s.stub}
	s.PatchValue(model.GetBudgetAPIClient, func(*httpbakery.Client) model.BudgetAPIClient { return s.budgetAPIClient })
}

func (s *DestroySuite) runDestroyCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	cmd := model.NewDestroyCommandForTest(s.api, s.configAPI, noOpRefresh, s.store, s.sleep)
	return cmdtesting.RunCommand(c, cmd, args...)
}

func (s *DestroySuite) NewDestroyCommand() cmd.Command {
	return model.NewDestroyCommandForTest(s.api, s.configAPI, noOpRefresh, s.store, s.sleep)
}

func checkModelExistsInStore(c *gc.C, name string, store jujuclient.ClientStore) {
	controller, model := modelcmd.SplitModelName(name)
	_, err := store.ModelByName(controller, model)
	c.Assert(err, jc.ErrorIsNil)
}

func checkModelRemovedFromStore(c *gc.C, name string, store jujuclient.ClientStore) {
	controller, model := modelcmd.SplitModelName(name)
	_, err := store.ModelByName(controller, model)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *DestroySuite) TestDestroyNoModelNameError(c *gc.C) {
	_, err := s.runDestroyCommand(c)
	c.Assert(err, gc.ErrorMatches, "no model specified")
}

func (s *DestroySuite) TestDestroyBadFlags(c *gc.C) {
	_, err := s.runDestroyCommand(c, "-n")
	c.Assert(err, gc.ErrorMatches, "flag provided but not defined: -n")
}

func (s *DestroySuite) TestDestroyUnknownArgument(c *gc.C) {
	_, err := s.runDestroyCommand(c, "model", "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}

func (s *DestroySuite) TestDestroyUnknownModelCallsRefresh(c *gc.C) {
	called := false
	refresh := func(jujuclient.ClientStore, string) error {
		called = true
		return nil
	}

	cmd := model.NewDestroyCommandForTest(s.api, s.configAPI, refresh, s.store, s.sleep)
	_, err := cmdtesting.RunCommand(c, cmd, "foo")
	c.Check(called, jc.IsTrue)
	c.Check(err, gc.ErrorMatches, `cannot read model info: model test1:admin/foo not found`)
}

func (s *DestroySuite) TestDestroyCannotConnectToAPI(c *gc.C) {
	s.stub.SetErrors(errors.New("connection refused"))
	_, err := s.runDestroyCommand(c, "test2", "-y")
	c.Assert(err, gc.ErrorMatches, "cannot destroy model: connection refused")
	c.Check(c.GetTestLog(), jc.Contains, "failed to destroy model \"test2\"")
	checkModelExistsInStore(c, "test1:admin/test2", s.store)
}

func (s *DestroySuite) TestSystemDestroyFails(c *gc.C) {
	_, err := s.runDestroyCommand(c, "test1", "-y")
	c.Assert(err, gc.ErrorMatches, `"test1" is a controller; use 'juju destroy-controller' to destroy it`)
	checkModelExistsInStore(c, "test1:admin/test1", s.store)
}

func (s *DestroySuite) TestDestroy(c *gc.C) {
	checkModelExistsInStore(c, "test1:admin/test2", s.store)
	_, err := s.runDestroyCommand(c, "test2", "-y")
	c.Assert(err, jc.ErrorIsNil)
	checkModelRemovedFromStore(c, "test1:admin/test2", s.store)
	s.stub.CheckCalls(c, []jutesting.StubCall{
		{"DestroyModel", []interface{}{names.NewModelTag("test2-uuid"), (*bool)(nil)}},
	})
}

func (s *DestroySuite) TestDestroyBlocks(c *gc.C) {
	checkModelExistsInStore(c, "test1:admin/test2", s.store)
	s.api.modelInfoErr = []*params.Error{{}, {Code: params.CodeNotFound}}
	_, err := s.runDestroyCommand(c, "test2", "-y")
	c.Assert(err, jc.ErrorIsNil)
	checkModelRemovedFromStore(c, "test1:admin/test2", s.store)
	c.Assert(s.api.statusCallCount, gc.Equals, 1)
}

func (s *DestroySuite) TestFailedDestroyModel(c *gc.C) {
	s.stub.SetErrors(errors.New("permission denied"))
	_, err := s.runDestroyCommand(c, "test1:test2", "-y")
	c.Assert(err, gc.ErrorMatches, "cannot destroy model: permission denied")
	checkModelExistsInStore(c, "test1:admin/test2", s.store)
}

func (s *DestroySuite) TestDestroyWithUnsupportedSLA(c *gc.C) {
	s.configAPI.slaLevel = "unsupported"
	_, err := s.runDestroyCommand(c, "test1:test2", "-y")
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCallNames(c, "DestroyModel")
}

func (s *DestroySuite) TestDestroyWithSupportedSLA(c *gc.C) {
	s.configAPI.slaLevel = "standard"
	_, err := s.runDestroyCommand(c, "test2", "-y")
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []jutesting.StubCall{
		{"DestroyModel", []interface{}{names.NewModelTag("test2-uuid"), (*bool)(nil)}},
		{"DeleteBudget", []interface{}{"test2-uuid"}},
	})
}

func (s *DestroySuite) TestDestroyWithSupportedSLAFailure(c *gc.C) {
	s.configAPI.slaLevel = "standard"
	s.stub.SetErrors(nil, errors.New("bah"))
	_, err := s.runDestroyCommand(c, "test2", "-y")
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []jutesting.StubCall{
		{"DestroyModel", []interface{}{names.NewModelTag("test2-uuid"), (*bool)(nil)}},
		{"DeleteBudget", []interface{}{"test2-uuid"}},
	})
}

func (s *DestroySuite) TestDestroyDestroyStorage(c *gc.C) {
	_, err := s.runDestroyCommand(c, "test2", "-y", "--destroy-storage")
	c.Assert(err, jc.ErrorIsNil)
	destroyStorage := true
	s.stub.CheckCalls(c, []jutesting.StubCall{
		{"DestroyModel", []interface{}{names.NewModelTag("test2-uuid"), &destroyStorage}},
	})
}

func (s *DestroySuite) TestDestroyReleaseStorage(c *gc.C) {
	_, err := s.runDestroyCommand(c, "test2", "-y", "--release-storage")
	c.Assert(err, jc.ErrorIsNil)
	destroyStorage := false
	s.stub.CheckCalls(c, []jutesting.StubCall{
		{"DestroyModel", []interface{}{names.NewModelTag("test2-uuid"), &destroyStorage}},
	})
}

func (s *DestroySuite) TestDestroyDestroyReleaseStorageFlagsMutuallyExclusive(c *gc.C) {
	_, err := s.runDestroyCommand(c, "test2", "-y", "--destroy-storage", "--release-storage")
	c.Assert(err, gc.ErrorMatches, "--destroy-storage and --release-storage cannot both be specified")
}

func (s *DestroySuite) TestDestroyDestroyStorageFlagUnspecified(c *gc.C) {
	s.stub.SetErrors(&params.Error{Code: params.CodeHasPersistentStorage})
	s.api.modelInfoErr = []*params.Error{nil}
	_, err := s.runDestroyCommand(c, "test2", "-y")
	c.Assert(err, gc.ErrorMatches, `cannot destroy model "test2"

The model has persistent storage remaining:
	2 volumes and 1 filesystem

To destroy the storage, run the destroy-model
command again with the "--destroy-storage" flag.

To release the storage from Juju's management
without destroying it, use the "--release-storage"
flag instead. The storage can then be imported
into another Juju model.

`)
}

func (s *DestroySuite) TestDestroyDestroyStorageFlagUnspecifiedOldController(c *gc.C) {
	s.api.bestAPIVersion = 3
	ctx, err := s.runDestroyCommand(c, "test2", "-y")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `this juju controller only supports destroying storage

Please run the the command again with --destroy-storage,
to confirm that you want to destroy the storage along
with the model.

If instead you want to keep the storage, you must first
upgrade the controller to version 2.3 or greater.

`)
}

func (s *DestroySuite) resetModel(c *gc.C) {
	s.store.Models["test1"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{
			"admin/test1": {"test1-uuid"},
			"admin/test2": {"test2-uuid"},
		},
	}
}

func (s *DestroySuite) TestDestroyCommandConfirmation(c *gc.C) {
	var stdin, stdout bytes.Buffer
	ctx, err := cmd.DefaultContext()
	c.Assert(err, jc.ErrorIsNil)
	ctx.Stdout = &stdout
	ctx.Stdin = &stdin

	// Ensure confirmation is requested if "-y" is not specified.
	stdin.WriteString("n")
	_, errc := cmdtest.RunCommandWithDummyProvider(ctx, s.NewDestroyCommand(), "test2")
	select {
	case err := <-errc:
		c.Check(err, gc.ErrorMatches, "model destruction: aborted")
	case <-time.After(testing.LongWait):
		c.Fatalf("command took too long")
	}
	c.Check(cmdtesting.Stdout(ctx), gc.Matches, "WARNING!.*test2(.|\n)*")
	checkModelExistsInStore(c, "test1:admin/test1", s.store)

	// EOF on stdin: equivalent to answering no.
	stdin.Reset()
	stdout.Reset()
	_, errc = cmdtest.RunCommandWithDummyProvider(ctx, s.NewDestroyCommand(), "test2")
	select {
	case err := <-errc:
		c.Check(err, gc.ErrorMatches, "model destruction: aborted")
	case <-time.After(testing.LongWait):
		c.Fatalf("command took too long")
	}
	c.Check(cmdtesting.Stdout(ctx), gc.Matches, "WARNING!.*test2(.|\n)*")
	checkModelExistsInStore(c, "test1:admin/test2", s.store)

	for _, answer := range []string{"y", "Y", "yes", "YES"} {
		stdin.Reset()
		stdout.Reset()
		stdin.WriteString(answer)
		_, errc = cmdtest.RunCommandWithDummyProvider(ctx, s.NewDestroyCommand(), "test2")
		select {
		case err := <-errc:
			c.Check(err, jc.ErrorIsNil)
		case <-time.After(testing.LongWait):
			c.Fatalf("command took too long")
		}
		checkModelRemovedFromStore(c, "test1:admin/test2", s.store)

		// Add the test2 model back into the store for the next test
		s.resetModel(c)
	}
}

func (s *DestroySuite) TestBlockedDestroy(c *gc.C) {
	s.stub.SetErrors(common.OperationBlockedError("TestBlockedDestroy"))
	_, err := s.runDestroyCommand(c, "test2", "-y")
	testing.AssertOperationWasBlocked(c, err, ".*TestBlockedDestroy.*")
}

// mockBudgetAPIClient implements the budgetAPIClient interface.
type mockBudgetAPIClient struct {
	*jutesting.Stub
}

func (c *mockBudgetAPIClient) DeleteBudget(model string) (string, error) {
	c.MethodCall(c, "DeleteBudget", model)
	return "Budget removed.", c.NextErr()
}
