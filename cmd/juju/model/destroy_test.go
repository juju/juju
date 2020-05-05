// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"bytes"
	"time"

	testclock "github.com/juju/clock/testclock"
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2/httpbakery"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/cmdtest"
	"github.com/juju/juju/cmd/juju/model"
	rcmd "github.com/juju/juju/cmd/juju/romulus"
	"github.com/juju/juju/cmd/modelcmd"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/jujuclient"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type DestroySuite struct {
	testing.FakeJujuXDGDataHomeSuite
	api             *fakeAPI
	configAPI       *fakeConfigAPI
	storageAPI      *mockStorageAPI
	stub            *jutesting.Stub
	budgetAPIClient *mockBudgetAPIClient
	store           *jujuclient.MemStore

	clock *testclock.Clock
}

var _ = gc.Suite(&DestroySuite{})

// fakeDestroyAPI mocks out the client API
type fakeAPI struct {
	*jutesting.Stub
	err                error
	env                map[string]interface{}
	statusCallCount    int
	bestAPIVersion     int
	modelInfoErr       []*params.Error
	modelStatusPayload []base.ModelStatus
}

func (f *fakeAPI) Close() error { return nil }

func (f *fakeAPI) BestAPIVersion() int {
	return f.bestAPIVersion
}

func (f *fakeAPI) DestroyModel(tag names.ModelTag, destroyStorage *bool, force *bool, maxWait *time.Duration) error {
	f.MethodCall(f, "DestroyModel", tag, destroyStorage, force, maxWait)
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

	if f.modelStatusPayload == nil {
		f.modelStatusPayload = []base.ModelStatus{{
			Volumes: []base.Volume{
				{Detachable: true},
				{Detachable: true},
			},
			Filesystems: []base.Filesystem{{Detachable: true}},
		}}
	}
	return f.modelStatusPayload, err
}

// fakeConfigAPI mocks out the ModelConfigAPI.
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
	s.storageAPI = &mockStorageAPI{Stub: s.stub}
	s.clock = testclock.NewClock(time.Now())

	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = "test1"
	s.store.Controllers["test1"] = jujuclient.ControllerDetails{ControllerUUID: "test1-uuid"}
	s.store.Models["test1"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{
			"admin/test1": {ModelUUID: "test1-uuid", ModelType: coremodel.IAAS},
			"admin/test2": {ModelUUID: "test2-uuid", ModelType: coremodel.IAAS},
		},
	}
	s.store.Accounts["test1"] = jujuclient.AccountDetails{
		User: "admin",
	}

	s.budgetAPIClient = &mockBudgetAPIClient{Stub: s.stub}
	s.PatchValue(model.GetBudgetAPIClient,
		func(string, *httpbakery.Client) (model.BudgetAPIClient, error) { return s.budgetAPIClient, nil })
	s.PatchValue(&rcmd.GetMeteringURLForModelCmd,
		func(*modelcmd.ModelCommandBase) (string, error) { return "http://example.com", nil })
}

func (s *DestroySuite) runDestroyCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	command := model.NewDestroyCommandForTest(s.api, s.configAPI, s.storageAPI, s.clock, noOpRefresh, s.store)
	return cmdtesting.RunCommand(c, command, args...)
}

func (s *DestroySuite) NewDestroyCommand() cmd.Command {
	return model.NewDestroyCommandForTest(s.api, s.configAPI, s.storageAPI, s.clock, noOpRefresh, s.store)
}

func checkModelExistsInStore(c *gc.C, name string, store jujuclient.ClientStore) {
	controller, amodel := modelcmd.SplitModelName(name)
	_, err := store.ModelByName(controller, amodel)
	c.Assert(err, jc.ErrorIsNil)
}

func checkModelRemovedFromStore(c *gc.C, name string, store jujuclient.ClientStore) {
	controller, amodel := modelcmd.SplitModelName(name)
	_, err := store.ModelByName(controller, amodel)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *DestroySuite) TestDestroyNoModelNameError(c *gc.C) {
	_, err := s.runDestroyCommand(c)
	c.Assert(err, gc.ErrorMatches, "no model specified")
}

func (s *DestroySuite) TestDestroyBadFlags(c *gc.C) {
	_, err := s.runDestroyCommand(c, "-n")
	c.Assert(err, gc.ErrorMatches, "option provided but not defined: -n")
}

func (s *DestroySuite) TestDestroyUnknownArgument(c *gc.C) {
	_, err := s.runDestroyCommand(c, "model", "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}

func (s *DestroySuite) TestDestroyMaxWaitWithoutForce(c *gc.C) {
	_, err := s.runDestroyCommand(c, "model", "--no-wait")
	c.Assert(err, gc.ErrorMatches, `--no-wait without --force not valid`)
}

func (s *DestroySuite) TestDestroyUnknownModelCallsRefresh(c *gc.C) {
	called := false
	refresh := func(jujuclient.ClientStore, string) error {
		called = true
		return nil
	}

	command := model.NewDestroyCommandForTest(s.api, s.configAPI, s.storageAPI, s.clock, refresh, s.store)
	_, err := cmdtesting.RunCommand(c, command, "foo")
	c.Check(called, jc.IsTrue)
	c.Check(err, gc.ErrorMatches, `model test1:admin/foo not found`)
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
		{"DestroyModel",
			[]interface{}{names.NewModelTag("test2-uuid"), (*bool)(nil), (*bool)(nil), (*time.Duration)(nil)}},
	})
}

func (s *DestroySuite) TestDestroyWithPartModelUUID(c *gc.C) {
	checkModelExistsInStore(c, "test1:admin/test2", s.store)
	_, err := s.runDestroyCommand(c, "test2-uu", "-y")
	c.Assert(err, jc.ErrorIsNil)
	checkModelRemovedFromStore(c, "test1:admin/test2", s.store)
	s.stub.CheckCalls(c, []jutesting.StubCall{
		{"DestroyModel",
			[]interface{}{names.NewModelTag("test2-uuid"), (*bool)(nil), (*bool)(nil), (*time.Duration)(nil)}},
	})
}

func (s *DestroySuite) TestDestroyWithForce(c *gc.C) {
	checkModelExistsInStore(c, "test1:admin/test2", s.store)
	_, err := s.runDestroyCommand(c, "test2", "-y", "--force")
	c.Assert(err, jc.ErrorIsNil)
	checkModelRemovedFromStore(c, "test1:admin/test2", s.store)
	force := true
	s.stub.CheckCalls(c, []jutesting.StubCall{
		{"DestroyModel", []interface{}{names.NewModelTag("test2-uuid"), (*bool)(nil), &force, (*time.Duration)(nil)}},
	})
}

func (s *DestroySuite) TestDestroyWithForceNoWait(c *gc.C) {
	checkModelExistsInStore(c, "test1:admin/test2", s.store)
	_, err := s.runDestroyCommand(c, "test2", "-y", "--force", "--no-wait")
	c.Assert(err, jc.ErrorIsNil)
	checkModelRemovedFromStore(c, "test1:admin/test2", s.store)
	force := true
	maxWait := 0 * time.Second
	s.stub.CheckCalls(c, []jutesting.StubCall{
		{"DestroyModel", []interface{}{names.NewModelTag("test2-uuid"), (*bool)(nil), &force, &maxWait}},
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
		{"DestroyModel", []interface{}{names.NewModelTag("test2-uuid"), (*bool)(nil), (*bool)(nil), (*time.Duration)(nil)}},
		{"DeleteBudget", []interface{}{"test2-uuid"}},
	})
}

func (s *DestroySuite) TestDestroyWithSupportedSLAFailure(c *gc.C) {
	s.configAPI.slaLevel = "standard"
	s.stub.SetErrors(nil, errors.New("bah"))
	_, err := s.runDestroyCommand(c, "test2", "-y")
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []jutesting.StubCall{
		{"DestroyModel", []interface{}{names.NewModelTag("test2-uuid"), (*bool)(nil), (*bool)(nil), (*time.Duration)(nil)}},
		{"DeleteBudget", []interface{}{"test2-uuid"}},
	})
}

func (s *DestroySuite) TestDestroyDestroyStorage(c *gc.C) {
	_, err := s.runDestroyCommand(c, "test2", "-y", "--destroy-storage")
	c.Assert(err, jc.ErrorIsNil)
	destroyStorage := true
	s.stub.CheckCalls(c, []jutesting.StubCall{
		{"DestroyModel", []interface{}{names.NewModelTag("test2-uuid"), &destroyStorage, (*bool)(nil), (*time.Duration)(nil)}},
	})
}

func (s *DestroySuite) TestDestroyReleaseStorage(c *gc.C) {
	_, err := s.runDestroyCommand(c, "test2", "-y", "--release-storage")
	c.Assert(err, jc.ErrorIsNil)
	destroyStorage := false
	s.stub.CheckCalls(c, []jutesting.StubCall{
		{"DestroyModel", []interface{}{names.NewModelTag("test2-uuid"), &destroyStorage, (*bool)(nil), (*time.Duration)(nil)}},
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
command again with the "--destroy-storage" option.

To release the storage from Juju's management
without destroying it, use the "--release-storage"
option instead. The storage can then be imported
into another Juju model.

`)
}

func (s *DestroySuite) TestDestroyDestroyStorageFlagUnspecifiedOldController(c *gc.C) {
	s.api.bestAPIVersion = 3
	s.storageAPI.storage = []params.StorageDetails{{}}

	_, err := s.runDestroyCommand(c, "test2", "-y")
	c.Assert(err, gc.ErrorMatches, `cannot destroy model "test2"

Destroying this model will destroy the storage, but you
have not indicated that you want to do that.

Please run the the command again with --destroy-storage
to confirm that you want to destroy the storage along
with the model.

If instead you want to keep the storage, you must first
upgrade the controller to version 2.3 or greater.

`)
}

func (s *DestroySuite) TestDestroyDestroyStorageFlagUnspecifiedOldControllerNoStorage(c *gc.C) {
	s.api.bestAPIVersion = 3
	s.storageAPI.storage = nil // no storage in model

	_, err := s.runDestroyCommand(c, "test2", "-y")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DestroySuite) resetModel(c *gc.C) {
	s.store.Models["test1"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{
			"admin/test1": {ModelUUID: "test1-uuid", ModelType: coremodel.IAAS},
			"admin/test2": {ModelUUID: "test2-uuid", ModelType: coremodel.IAAS},
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

func (s *DestroySuite) TestDestroyCommandWait(c *gc.C) {
	checkModelExistsInStore(c, "test1:admin/test2", s.store)

	s.api.modelInfoErr = []*params.Error{nil, nil}
	s.api.modelStatusPayload = []base.ModelStatus{{
		ApplicationCount:   2,
		HostedMachineCount: 1,
		Volumes: []base.Volume{
			{Detachable: true, Status: "error", Message: "failed to destroy volume 0", Id: "0"},
			{Detachable: true, Status: "error", Message: "failed to destroy volume 1", Id: "1"},
			{Detachable: true, Status: "error", Message: "failed to destroy volume 2", Id: "2"},
		},
		Filesystems: []base.Filesystem{
			{Detachable: true, Status: "error", Message: "failed to destroy filesystem 0", Id: "0"},
			{Detachable: true, Status: "error", Message: "failed to destroy filesystem 1", Id: "1"},
		},
	}}

	done := make(chan struct{}, 1)
	outErr := make(chan error, 1)
	outStdOut := make(chan string, 1)
	outStdErr := make(chan string, 1)

	go func() {
		// run destroy model cmd, and timeout in 3s.
		ctx, err := s.runDestroyCommand(c, "test2", "-y", "-t", "3s")
		outStdOut <- cmdtesting.Stdout(ctx)
		outStdErr <- cmdtesting.Stderr(ctx)
		outErr <- err
		done <- struct{}{}
	}()

	c.Assert(s.clock.WaitAdvance(5*time.Second, testing.LongWait, 2), jc.ErrorIsNil)

	select {
	case <-done:
		c.Assert(<-outStdErr, gc.Equals, `
Destroying model
Waiting for model to be removed, 5 error(s), 1 machine(s), 2 application(s), 3 volume(s), 2 filesystems(s)....`[1:])
		c.Assert(<-outStdOut, gc.Equals, `

The following errors were encountered during destroying the model.
You can fix the problem causing the errors and run destroy-model again.

Resource    Id  Message
Filesystem  0   failed to destroy filesystem 0
            1   failed to destroy filesystem 1
Volume      0   failed to destroy volume 0
            1   failed to destroy volume 1
            2   failed to destroy volume 2
`[1:])
		// timeout after 3s.
		c.Assert(<-outErr, jc.Satisfies, errors.IsTimeout)
		checkModelExistsInStore(c, "test1:admin/test2", s.store)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for destroy cmd.")
	}
}

func (s *DestroySuite) TestDestroyCommandLeanMessage(c *gc.C) {
	checkModelExistsInStore(c, "test1:admin/test2", s.store)

	s.api.modelInfoErr = []*params.Error{nil, nil}
	s.api.modelStatusPayload = []base.ModelStatus{{
		ApplicationCount:   0,
		HostedMachineCount: 0,
		Volumes:            []base.Volume{},
		Filesystems:        []base.Filesystem{},
	}}

	done := make(chan struct{}, 1)
	outErr := make(chan error, 1)
	outStdOut := make(chan string, 1)
	outStdErr := make(chan string, 1)

	go func() {
		// run destroy model cmd, and timeout in 3s.
		ctx, err := s.runDestroyCommand(c, "test2", "-y", "-t", "3s")
		outStdOut <- cmdtesting.Stdout(ctx)
		outStdErr <- cmdtesting.Stderr(ctx)
		outErr <- err
		done <- struct{}{}
	}()

	c.Assert(s.clock.WaitAdvance(5*time.Second, testing.LongWait, 2), jc.ErrorIsNil)

	select {
	case <-done:
		c.Assert(<-outStdErr, gc.Equals, `
Destroying model
Waiting for model to be removed....`[1:])
		// timeout after 3s.
		c.Assert(<-outErr, jc.Satisfies, errors.IsTimeout)
		checkModelExistsInStore(c, "test1:admin/test2", s.store)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for destroy cmd.")
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

type mockStorageAPI struct {
	*jutesting.Stub
	storage []params.StorageDetails
}

func (*mockStorageAPI) Close() error { return nil }

func (m *mockStorageAPI) ListStorageDetails() ([]params.StorageDetails, error) {
	m.MethodCall(m, "ListStorageDetails")
	return m.storage, m.NextErr()
}
