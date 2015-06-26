// Copyright 2014, 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment_test

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/environment"
	"github.com/juju/juju/testing"
)

type retryProvisioningSuite struct {
	testing.FakeJujuHomeSuite
	fake *fakeRetryProvisioningClient
}

var _ = gc.Suite(&retryProvisioningSuite{})

// fakeRetryProvisioningClient contains some minimal information
// about machines in the environment to mock out the behavior
// of the real RetryProvisioning command.
type fakeRetryProvisioningClient struct {
	m   map[string]fakeMachine
	err error
}

type fakeMachine struct {
	info string
	data map[string]interface{}
}

func (f *fakeRetryProvisioningClient) Close() error {
	return nil
}

func (f *fakeRetryProvisioningClient) RetryProvisioning(machines ...names.MachineTag) (
	[]params.ErrorResult, error) {

	if f.err != nil {
		return nil, f.err
	}

	results := make([]params.ErrorResult, len(machines))

	// For each of the machines passed in, verify that we have the
	// id and that the info string is "broken".
	for i, machine := range machines {
		m, ok := f.m[machine.Id()]
		if ok {
			if m.info == "broken" {
				// The real RetryProvisioning command sets the
				// status data "transient" : true.
				m.data["transient"] = true
			} else {
				results[i].Error = common.ServerError(
					fmt.Errorf("%s is not in an error state",
						names.ReadableString(machine)))
			}
		} else {
			results[i].Error = common.ServerError(
				errors.NotFoundf("machine %s", machine.Id()))
		}
	}

	return results, nil
}

func (s *retryProvisioningSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)

	// For all tests, create machine 0 (broken) and
	// machine 1 (not broken).
	s.fake = &fakeRetryProvisioningClient{
		m: map[string]fakeMachine{
			"0": {info: "broken",
				data: make(map[string]interface{})},
			"1": {info: "",
				data: make(map[string]interface{})},
		},
	}
}

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
		stdErr: `machine 42 not found`,
	}, {
		args:   []string{"1"},
		stdErr: `machine 1 is not in an error state`,
	}, {
		args: []string{"0"},
	}, {
		args:   []string{"0", "1"},
		stdErr: `machine 1 is not in an error state`,
	}, {
		args: []string{"1", "42"},
		stdErr: `machine 1 is not in an error state` +
			`machine 42 not found`,
	}, {
		args: []string{"0/lxc/0"},
		err:  `invalid machine "0/lxc/0" retry-provisioning does not support containers`,
	},
}

func (s *retryProvisioningSuite) TestRetryProvisioning(c *gc.C) {
	for i, t := range resolvedMachineTests {
		c.Logf("test %d: %v", i, t.args)
		command := environment.NewRetryProvisioningCommand(s.fake)
		context, err := testing.RunCommand(c, envcmd.Wrap(command), t.args...)
		if t.err != "" {
			c.Check(err, gc.ErrorMatches, t.err)
			continue
		}
		c.Check(err, jc.ErrorIsNil)
		output := testing.Stderr(context)
		stripped := strings.Replace(output, "\n", "", -1)
		c.Check(stripped, gc.Equals, t.stdErr)
		if t.args[0] == "0" {
			m := s.fake.m["0"]
			c.Check(m.info, gc.Equals, "broken")
			c.Check(m.data["transient"], jc.IsTrue)
		}
	}
}

func (s *retryProvisioningSuite) TestBlockRetryProvisioning(c *gc.C) {
	s.fake.err = common.ErrOperationBlocked("TestBlockRetryProvisioning")
	command := environment.NewRetryProvisioningCommand(s.fake)

	for i, t := range resolvedMachineTests {
		c.Logf("test %d: %v", i, t.args)
		_, err := testing.RunCommand(c, envcmd.Wrap(command), t.args...)
		if t.err != "" {
			c.Check(err, gc.ErrorMatches, t.err)
			continue
		}
		c.Assert(err, gc.ErrorMatches, cmd.ErrSilent.Error())
		// msg is logged
		stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
		c.Check(stripped, gc.Matches, ".*TestBlockRetryProvisioning.*")
	}
}
