// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/machine"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing"
)

type RemoveMachineSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fake          *fakeRemoveMachineAPI
	apiConnection *mockAPIConnection
}

var _ = gc.Suite(&RemoveMachineSuite{})

func (s *RemoveMachineSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.fake = &fakeRemoveMachineAPI{}
	s.apiConnection = &mockAPIConnection{}
}

func (s *RemoveMachineSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	remove, _ := machine.NewRemoveCommandForTest(s.apiConnection, s.fake)
	return cmdtesting.RunCommand(c, remove, args...)
}

func (s *RemoveMachineSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		args        []string
		machines    []string
		force       bool
		keep        bool
		errorString string
	}{
		{
			errorString: "no machines specified",
		}, {
			args:     []string{"1"},
			machines: []string{"1"},
		}, {
			args:     []string{"1", "2"},
			machines: []string{"1", "2"},
		}, {
			args:     []string{"1", "--force"},
			machines: []string{"1"},
			force:    true,
		}, {
			args:     []string{"--force", "1", "2"},
			machines: []string{"1", "2"},
			force:    true,
		}, {
			args:     []string{"--keep-instance", "1", "2"},
			machines: []string{"1", "2"},
			keep:     true,
		}, {
			args:        []string{"lxd"},
			errorString: `invalid machine id "lxd"`,
		}, {
			args:     []string{"1/lxd/2"},
			machines: []string{"1/lxd/2"},
		},
	} {
		c.Logf("test %d", i)
		wrappedCommand, removeCmd := machine.NewRemoveCommandForTest(s.apiConnection, s.fake)
		err := cmdtesting.InitCommand(wrappedCommand, test.args)
		if test.errorString == "" {
			c.Check(err, jc.ErrorIsNil)
			c.Check(removeCmd.Force, gc.Equals, test.force)
			c.Check(removeCmd.KeepInstance, gc.Equals, test.keep)
			c.Check(removeCmd.MachineIds, jc.DeepEquals, test.machines)
		} else {
			c.Check(err, gc.ErrorMatches, test.errorString)
		}
	}
}

func (s *RemoveMachineSuite) TestRemove(c *gc.C) {
	_, err := s.run(c, "1", "2/lxd/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.forced, jc.IsFalse)
	c.Assert(s.fake.machines, jc.DeepEquals, []string{"1", "2/lxd/1"})
}

func (s *RemoveMachineSuite) TestRemoveNoWaitWithoutForce(c *gc.C) {
	_, err := s.run(c, "1", "--no-wait")
	c.Assert(err, gc.ErrorMatches, `--no-wait without --force not valid`)
}

func (s *RemoveMachineSuite) TestRemoveOutput(c *gc.C) {
	s.fake.results = []params.DestroyMachineResult{{
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
	ctx, err := s.run(c, "1", "2/lxd/1")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stderr, gc.Equals, `
removing machine failed: oy vey machine 1
removing machine 2/lxd/1
- will remove unit foo/0
- will remove storage bar/1
- will detach storage baz/2
`[1:])
}

func (s *RemoveMachineSuite) TestRemoveOutputKeep(c *gc.C) {
	ctx, err := s.run(c, "--keep-instance", "1", "2")
	c.Assert(err, jc.ErrorIsNil)
	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stderr, gc.Equals, `
removing machine 1 (but retaining cloud instance)
removing machine 2 (but retaining cloud instance)
`[1:])
}

func (s *RemoveMachineSuite) TestRemoveForce(c *gc.C) {
	_, err := s.run(c, "--force", "1", "2/lxd/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.forced, jc.IsTrue)
	c.Assert(s.fake.keep, jc.IsFalse)
	c.Assert(s.fake.machines, jc.DeepEquals, []string{"1", "2/lxd/1"})
}

func (s *RemoveMachineSuite) TestRemoveKeep(c *gc.C) {
	_, err := s.run(c, "--keep-instance", "1", "2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.forced, jc.IsFalse)
	c.Assert(s.fake.keep, jc.IsTrue)
	c.Assert(s.fake.machines, jc.DeepEquals, []string{"1", "2"})
}

func (s *RemoveMachineSuite) TestRemoveWithContainers(c *gc.C) {
	s.fake.results = []params.DestroyMachineResult{{
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

	ctx, err := s.run(c, "--force", "1")
	c.Assert(err, jc.ErrorIsNil)
	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stderr, gc.Equals, `
removing machine 1/lxd/2
- will remove unit foo/1
- will remove storage bar/2
- will detach storage baz/3

removing machine 1
- will remove unit foo/0
- will remove storage bar/1
- will detach storage baz/2
`[1:])
}

func (s *RemoveMachineSuite) TestBlockedError(c *gc.C) {
	s.fake.removeError = apiservererrors.OperationBlockedError("TestBlockedError")
	_, err := s.run(c, "1")
	c.Assert(s.fake.forced, jc.IsFalse)
	testing.AssertOperationWasBlocked(c, err, ".*TestBlockedError.*")
}

func (s *RemoveMachineSuite) TestForceBlockedError(c *gc.C) {
	s.fake.removeError = apiservererrors.OperationBlockedError("TestForceBlockedError")
	_, err := s.run(c, "--force", "1")
	c.Assert(s.fake.forced, jc.IsTrue)
	testing.AssertOperationWasBlocked(c, err, ".*TestForceBlockedError.*")
}

type fakeRemoveMachineAPI struct {
	forced      bool
	keep        bool
	machines    []string
	removeError error
	results     []params.DestroyMachineResult
}

func (f *fakeRemoveMachineAPI) Close() error {
	return nil
}

func (f *fakeRemoveMachineAPI) DestroyMachinesWithParams(force, keep bool, maxWait *time.Duration, machines ...string) ([]params.DestroyMachineResult, error) {
	f.forced = force
	f.keep = keep
	f.machines = machines
	if f.removeError != nil || f.results != nil {
		return f.results, f.removeError
	}
	results := make([]params.DestroyMachineResult, len(machines))
	for i := range results {
		results[i].Info = &params.DestroyMachineInfo{MachineId: machines[i]}
	}
	return results, nil
}

type mockAPIConnection struct {
	api.Connection
}
