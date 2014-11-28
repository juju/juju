// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type retryProvisioningSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&retryProvisioningSuite{})

var resolvedMachineTests = []struct {
	args   []string
	err    string
	stdErr string
}{
	{
		err: `no machine specified`,
	}, {
		args: []string{"jeremy-fisher"},
		err:  `invalid machine "jeremy-fisher"`,
	}, {
		args:   []string{"42"},
		stdErr: `cannot retry provisioning "machine-42": machine 42 not found`,
	}, {
		args:   []string{"1"},
		stdErr: `cannot retry provisioning "machine-1": "machine-1" is not in an error state`,
	}, {
		args: []string{"0"},
	}, {
		args:   []string{"0", "1"},
		stdErr: `cannot retry provisioning "machine-1": "machine-1" is not in an error state`,
	},
}

func (s *retryProvisioningSuite) TestResolved(c *gc.C) {
	m, err := s.State.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobManageEnviron},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetStatus(state.StatusError, "broken", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	})
	c.Assert(err, jc.ErrorIsNil)

	for i, t := range resolvedMachineTests {
		c.Logf("test %d: %v", i, t.args)
		context, err := testing.RunCommand(c, envcmd.Wrap(&RetryProvisioningCommand{}), t.args...)
		if t.err != "" {
			c.Check(err, gc.ErrorMatches, t.err)
			continue
		} else {
			c.Check(err, jc.ErrorIsNil)
		}
		output := testing.Stderr(context)
		stripped := strings.Replace(output, "\n", "", -1)
		c.Check(stripped, gc.Equals, t.stdErr)
		if t.args[0] == "0" {
			status, info, data, err := m.Status()
			c.Check(err, jc.ErrorIsNil)
			c.Check(status, gc.Equals, state.StatusError)
			c.Check(info, gc.Equals, "broken")
			c.Check(data["transient"], jc.IsTrue)
		}
	}
}
