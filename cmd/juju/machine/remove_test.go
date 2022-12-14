// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"bytes"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/cmdtest"
	"github.com/juju/juju/cmd/juju/machine"
	"github.com/juju/juju/cmd/juju/machine/mocks"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing"
)

type RemoveMachineSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	mockApi       *mocks.MockRemoveMachineAPI
	apiConnection *mockAPIConnection

	facadeVersion int
}

var _ = gc.Suite(&RemoveMachineSuite{})

func (s *RemoveMachineSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.apiConnection = &mockAPIConnection{}
	s.facadeVersion = 10
}

func (s *RemoveMachineSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.mockApi = mocks.NewMockRemoveMachineAPI(ctrl)
	s.mockApi.EXPECT().Close().Return(nil).AnyTimes()
	s.mockApi.EXPECT().BestAPIVersion().Return(s.facadeVersion).AnyTimes()
	return ctrl
}

func (s *RemoveMachineSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	remove, _ := machine.NewRemoveCommandForTest(s.apiConnection, s.mockApi)
	return cmdtesting.RunCommand(c, remove, args...)
}

func (s *RemoveMachineSuite) runWithContext(ctx *cmd.Context, args ...string) (chan dummy.Operation, chan error) {
	remove, _ := machine.NewRemoveCommandForTest(s.apiConnection, s.mockApi)
	return cmdtest.RunCommandWithDummyProvider(ctx, remove, args...)
}

func defaultDestroyMachineResult(_, _, _ bool, _ *time.Duration, machines ...string) ([]params.DestroyMachineResult, error) {
	results := make([]params.DestroyMachineResult, len(machines))
	for i := range results {
		results[i].Info = &params.DestroyMachineInfo{MachineId: machines[i]}
	}
	return results, nil
}

func (s *RemoveMachineSuite) TestInit(c *gc.C) {
	defer s.setup(c).Finish()

	for i, test := range []struct {
		args        []string
		machines    []string
		force       bool
		keep        bool
		noPrompt    bool
		dryRun      bool
		errorString string
	}{
		{
			errorString: "no machines specified",
		}, {
			args:     []string{"1"},
			machines: []string{"1"},
			noPrompt: true,
		}, {
			args:     []string{"1", "2"},
			machines: []string{"1", "2"},
			noPrompt: true,
		}, {
			args:     []string{"1", "--force"},
			machines: []string{"1"},
			force:    true,
			noPrompt: true,
		}, {
			args:     []string{"--force", "1", "2"},
			machines: []string{"1", "2"},
			force:    true,
			noPrompt: true,
		}, {
			args:     []string{"--keep-instance", "1", "2"},
			machines: []string{"1", "2"},
			keep:     true,
			noPrompt: true,
		}, {
			args:     []string{"1", "2", "--no-prompt"},
			machines: []string{"1", "2"},
			noPrompt: true,
		}, {
			args:     []string{"1", "2", "--dry-run"},
			machines: []string{"1", "2"},
			noPrompt: true,
			dryRun:   true,
		}, {
			args:        []string{"lxd"},
			errorString: `invalid machine id "lxd"`,
			noPrompt:    true,
		}, {
			args:     []string{"1/lxd/2"},
			machines: []string{"1/lxd/2"},
			noPrompt: true,
		},
	} {
		c.Logf("test %d", i)
		wrappedCommand, removeCmd := machine.NewRemoveCommandForTest(s.apiConnection, s.mockApi)
		err := cmdtesting.InitCommand(wrappedCommand, test.args)
		if test.errorString == "" {
			c.Check(err, jc.ErrorIsNil)
			c.Check(removeCmd.Force, gc.Equals, test.force)
			c.Check(removeCmd.KeepInstance, gc.Equals, test.keep)
			c.Check(removeCmd.DryRun, gc.Equals, test.dryRun)
			c.Check(removeCmd.MachineIds, jc.DeepEquals, test.machines)
		} else {
			c.Check(err, gc.ErrorMatches, test.errorString)
		}
	}
}

func (s *RemoveMachineSuite) TestRemove(c *gc.C) {
	defer s.setup(c).Finish()

	s.mockApi.EXPECT().DestroyMachinesWithParams(false, false, false, gomock.Any(), "1", "2/lxd/1")

	_, err := s.run(c, "--no-prompt", "1", "2/lxd/1")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RemoveMachineSuite) TestRemoveNoWaitWithoutForce(c *gc.C) {
	_, err := s.run(c, "--no-prompt", "1", "--no-wait")
	c.Assert(err, gc.ErrorMatches, `--no-wait without --force not valid`)
}

func (s *RemoveMachineSuite) TestRemoveOutput(c *gc.C) {
	defer s.setup(c).Finish()

	results := []params.DestroyMachineResult{{
		Error: &params.Error{
			Message: "oy vey machine 1",
		},
	}, {
		Info: &params.DestroyMachineInfo{
			MachineId:        "2/lxd/1",
			DestroyedUnits:   []params.Entity{{"unit-foo-0"}},
			DestroyedStorage: []params.Entity{{"storage-bar-1"}},
			DetachedStorage:  []params.Entity{{"storage-baz-2"}},
		},
	}}
	s.mockApi.EXPECT().DestroyMachinesWithParams(false, false, false, gomock.Any(), "1", "2/lxd/1").Return(results, nil)

	ctx, err := s.run(c, "--no-prompt", "1", "2/lxd/1")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	stderr := cmdtesting.Stderr(ctx)
	stdout := cmdtesting.Stdout(ctx)
	c.Assert(stderr, gc.Equals, `
removing machine failed: oy vey machine 1
`[1:])
	c.Assert(stdout, gc.Equals, `
will remove machine 2/lxd/1
- will remove unit foo/0
- will remove storage bar/1
- will detach storage baz/2
`[1:])
}

func (s *RemoveMachineSuite) TestRemoveKeep(c *gc.C) {
	defer s.setup(c).Finish()

	s.mockApi.EXPECT().DestroyMachinesWithParams(false, true, false, gomock.Any(), "1", "2")

	_, err := s.run(c, "--no-prompt", "--keep-instance", "1", "2")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RemoveMachineSuite) TestRemoveOutputKeep(c *gc.C) {
	defer s.setup(c).Finish()

	s.mockApi.EXPECT().DestroyMachinesWithParams(false, true, false, gomock.Any(), "1", "2").DoAndReturn(defaultDestroyMachineResult)

	ctx, err := s.run(c, "--no-prompt", "--keep-instance", "1", "2")
	c.Assert(err, jc.ErrorIsNil)
	stdout := cmdtesting.Stdout(ctx)
	c.Assert(stdout, gc.Equals, `
will remove machine 1 (but retaining cloud instance)
will remove machine 2 (but retaining cloud instance)
`[1:])
}

func (s *RemoveMachineSuite) TestRemoveForce(c *gc.C) {
	defer s.setup(c).Finish()

	s.mockApi.EXPECT().DestroyMachinesWithParams(true, false, false, gomock.Any(), "1", "2/lxd/1")

	_, err := s.run(c, "--no-prompt", "--force", "1", "2/lxd/1")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RemoveMachineSuite) TestRemoveWithContainers(c *gc.C) {
	defer s.setup(c).Finish()

	results := []params.DestroyMachineResult{{
		Info: &params.DestroyMachineInfo{
			MachineId:        "1",
			DestroyedUnits:   []params.Entity{{"unit-foo-0"}},
			DestroyedStorage: []params.Entity{{"storage-bar-1"}},
			DetachedStorage:  []params.Entity{{"storage-baz-2"}},
			DestroyedContainers: []params.DestroyMachineResult{{
				Info: &params.DestroyMachineInfo{
					MachineId:        "1/lxd/2",
					DestroyedUnits:   []params.Entity{{"unit-foo-1"}},
					DestroyedStorage: []params.Entity{{"storage-bar-2"}},
					DetachedStorage:  []params.Entity{{"storage-baz-3"}},
				},
			}},
		},
	}}
	s.mockApi.EXPECT().DestroyMachinesWithParams(true, false, false, gomock.Any(), "1").Return(results, nil)

	ctx, err := s.run(c, "--no-prompt", "--force", "1")
	c.Assert(err, jc.ErrorIsNil)
	stdout := cmdtesting.Stdout(ctx)
	c.Assert(stdout, gc.Equals, `
will remove machine 1
- will remove unit foo/0
- will remove storage bar/1
- will detach storage baz/2
will remove machine 1/lxd/2
- will remove unit foo/1
- will remove storage bar/2
- will detach storage baz/3
`[1:])
}

func (s *RemoveMachineSuite) TestRemoveDryRun(c *gc.C) {
	defer s.setup(c).Finish()

	s.mockApi.EXPECT().DestroyMachinesWithParams(false, false, true, gomock.Any(), "1", "2")

	_, err := s.run(c, "--dry-run", "1", "2")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RemoveMachineSuite) TestRemoveOutputDryRun(c *gc.C) {
	defer s.setup(c).Finish()

	s.mockApi.EXPECT().DestroyMachinesWithParams(false, false, true, gomock.Any(), "1", "2").DoAndReturn(defaultDestroyMachineResult)

	ctx, err := s.run(c, "--dry-run", "1", "2")
	c.Assert(err, jc.ErrorIsNil)
	stderr := cmdtesting.Stderr(ctx)
	stdout := cmdtesting.Stdout(ctx)
	c.Assert(stderr, gc.Equals, `
WARNING! This command:
`[1:])
	c.Assert(stdout, gc.Equals, `
will remove machine 1
will remove machine 2
`[1:])
}

func (s *RemoveMachineSuite) TestRemoveDryRunOldFacade(c *gc.C) {
	s.facadeVersion = 9
	defer s.setup(c).Finish()

	_, err := s.run(c, "--dry-run", "1", "2")
	c.Assert(err, gc.Equals, machine.ErrDryRunNotSupported)
}

func (s *RemoveMachineSuite) TestRemovePromptOldFacade(c *gc.C) {
	s.facadeVersion = 9
	defer s.setup(c).Finish()

	var stdin bytes.Buffer
	ctx := cmdtesting.Context(c)
	ctx.Stdin = &stdin

	s.mockApi.EXPECT().DestroyMachinesWithParams(false, false, false, gomock.Any(), "1", "2")

	stdin.WriteString("y")
	_, errc := s.runWithContext(ctx, "1", "2")

	select {
	case err := <-errc:
		c.Check(err, jc.ErrorIsNil)
	case <-time.After(testing.LongWait):
		c.Fatal("command took too long")
	}
}

func (s *RemoveMachineSuite) TestRemovePrompt(c *gc.C) {
	defer s.setup(c).Finish()

	var stdin bytes.Buffer
	ctx := cmdtesting.Context(c)
	ctx.Stdin = &stdin

	s.mockApi.EXPECT().DestroyMachinesWithParams(false, false, true, gomock.Any(), "1", "2")
	s.mockApi.EXPECT().DestroyMachinesWithParams(false, false, false, gomock.Any(), "1", "2")

	stdin.WriteString("y")
	_, errc := s.runWithContext(ctx, "1", "2")

	select {
	case err := <-errc:
		c.Check(err, jc.ErrorIsNil)
	case <-time.After(testing.LongWait):
		c.Fatal("command took too long")
	}
}

func (s *RemoveMachineSuite) TestRemovePromptOldFacadeAborted(c *gc.C) {
	s.facadeVersion = 9
	defer s.setup(c).Finish()

	ctx := cmdtesting.Context(c)
	var stdin bytes.Buffer
	ctx.Stdin = &stdin

	stdin.WriteString("n")
	_, errc := s.runWithContext(ctx, "1", "2")

	select {
	case err := <-errc:
		c.Check(err, gc.ErrorMatches, "machine removal: aborted")
	case <-time.After(testing.LongWait):
		c.Fatal("command took too long")
	}
}

func (s *RemoveMachineSuite) TestRemovePromptAborted(c *gc.C) {
	defer s.setup(c).Finish()

	ctx := cmdtesting.Context(c)
	var stdin bytes.Buffer
	ctx.Stdin = &stdin

	s.mockApi.EXPECT().DestroyMachinesWithParams(false, false, true, gomock.Any(), "1", "2")

	stdin.WriteString("n")
	_, errc := s.runWithContext(ctx, "1", "2")

	select {
	case err := <-errc:
		c.Check(err, gc.ErrorMatches, "machine removal: aborted")
	case <-time.After(testing.LongWait):
		c.Fatal("command took too long")
	}
}

func (s *RemoveMachineSuite) TestBlockedError(c *gc.C) {
	defer s.setup(c).Finish()

	removeError := apiservererrors.OperationBlockedError("TestBlockedError")
	s.mockApi.EXPECT().DestroyMachinesWithParams(false, false, false, gomock.Any(), "1").Return(nil, removeError)

	_, err := s.run(c, "--no-prompt", "1")
	testing.AssertOperationWasBlocked(c, err, ".*TestBlockedError.*")
}

func (s *RemoveMachineSuite) TestForceBlockedError(c *gc.C) {
	defer s.setup(c).Finish()

	removeError := apiservererrors.OperationBlockedError("TestForceBlockedError")
	s.mockApi.EXPECT().DestroyMachinesWithParams(true, false, false, gomock.Any(), "1").Return(nil, removeError)

	_, err := s.run(c, "--no-prompt", "--force", "1")
	testing.AssertOperationWasBlocked(c, err, ".*TestForceBlockedError.*")
}

type mockAPIConnection struct {
	api.Connection
}
