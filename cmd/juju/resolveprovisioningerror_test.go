// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	gc "launchpad.net/gocheck"
	jc "github.com/juju/testing/checkers"

	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/testing"
)

type resolveProvisioningErrorSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&resolveProvisioningErrorSuite{})

func runResolveProvisioningError(c *gc.C, args []string) error {
	_, err := testing.RunCommand(c, &ResolveProvisioningErrorCommand{}, args)
	return err
}

var resolvedMachineTests = []struct {
	args []string
	err  string
}{
	{
		err: `no machine specified`,
	}, {
		args: []string{"jeremy-fisher"},
		err:  `invalid machine "jeremy-fisher"`,
	}, {
		args: []string{"42"},
		err:  `machine 42 not found`,
	}, {
		args: []string{"1"},
		err:  `machine "machine-1" is not in an error state`,
	}, {
		args: []string{"0"},
	}, {
		args: []string{"0", "roflcopter"},
		err:  `unrecognized args: \["roflcopter"\]`,
	},
}

func (s *resolveProvisioningErrorSuite) TestResolved(c *gc.C) {
	m, err := s.State.AddOneMachine(state.MachineTemplate{
		Series:      "quantal",
		Jobs:        []state.MachineJob{state.JobManageEnviron},
	})
	c.Assert(err, gc.IsNil)
	err = m.SetStatus(params.StatusError, "broken", nil)
	c.Assert(err, gc.IsNil)
	_, err = s.State.AddOneMachine(state.MachineTemplate{
		Series:      "quantal",
		Jobs:        []state.MachineJob{state.JobHostUnits},
	})
	c.Assert(err, gc.IsNil)

	for i, t := range resolvedMachineTests {
		c.Logf("test %d: %v", i, t.args)
		err := runResolveProvisioningError(c, t.args)
		if t.err != "" {
			c.Check(err, gc.ErrorMatches, t.err)
		} else {
			status, info, data, err := m.Status()
			c.Check(err, gc.IsNil)
			c.Check(status, gc.Equals, params.StatusError)
			c.Check(info, gc.Equals, "broken")
			c.Check(data["transient"], jc.IsTrue)
		}
	}
}
