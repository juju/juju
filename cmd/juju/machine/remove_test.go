// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/machine"
	"github.com/juju/juju/testing"
)

type RemoveMachineSuite struct {
	testing.FakeJujuHomeSuite
	fake *fakeRemoveMachineAPI
}

var _ = gc.Suite(&RemoveMachineSuite{})

func (s *RemoveMachineSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.fake = &fakeRemoveMachineAPI{}
}

func (s *RemoveMachineSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	remove := machine.NewRemoveCommand(s.fake)
	return testing.RunCommand(c, envcmd.Wrap(remove), args...)
}

func (s *RemoveMachineSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		args        []string
		machines    []string
		force       bool
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
			args:        []string{"lxc"},
			errorString: `invalid machine id "lxc"`,
		}, {
			args:     []string{"1/lxc/2"},
			machines: []string{"1/lxc/2"},
		},
	} {
		c.Logf("test %d", i)
		removeCmd := &machine.RemoveCommand{}
		err := testing.InitCommand(removeCmd, test.args)
		if test.errorString == "" {
			c.Check(err, jc.ErrorIsNil)
			c.Check(removeCmd.Force, gc.Equals, test.force)
			c.Check(removeCmd.MachineIds, jc.DeepEquals, test.machines)
		} else {
			c.Check(err, gc.ErrorMatches, test.errorString)
		}
	}
}

func (s *RemoveMachineSuite) TestRemove(c *gc.C) {
	_, err := s.run(c, "1", "2/lxc/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.forced, jc.IsFalse)
	c.Assert(s.fake.machines, jc.DeepEquals, []string{"1", "2/lxc/1"})
}

func (s *RemoveMachineSuite) TestRemoveForce(c *gc.C) {
	_, err := s.run(c, "--force", "1", "2/lxc/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.forced, jc.IsTrue)
	c.Assert(s.fake.machines, jc.DeepEquals, []string{"1", "2/lxc/1"})
}

func (s *RemoveMachineSuite) TestBlockedError(c *gc.C) {
	s.fake.removeError = common.ErrOperationBlocked("TestBlockedError")
	_, err := s.run(c, "1")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(s.fake.forced, jc.IsFalse)
	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Assert(stripped, gc.Matches, ".*TestBlockedError.*")
}

func (s *RemoveMachineSuite) TestForceBlockedError(c *gc.C) {
	s.fake.removeError = common.ErrOperationBlocked("TestForceBlockedError")
	_, err := s.run(c, "--force", "1")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(s.fake.forced, jc.IsTrue)
	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Assert(stripped, gc.Matches, ".*TestForceBlockedError.*")
}

type fakeRemoveMachineAPI struct {
	forced      bool
	machines    []string
	removeError error
}

func (f *fakeRemoveMachineAPI) Close() error {
	return nil
}

func (f *fakeRemoveMachineAPI) DestroyMachines(machines ...string) error {
	f.forced = false
	f.machines = machines
	return f.removeError
}

func (f *fakeRemoveMachineAPI) ForceDestroyMachines(machines ...string) error {
	f.forced = true
	f.machines = machines
	return f.removeError
}
