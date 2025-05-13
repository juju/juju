// Copyright 2014, 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"context"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type retryProvisioningSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fake *fakeRetryProvisioningClient
}

var _ = tc.Suite(&retryProvisioningSuite{})

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

func (f *fakeRetryProvisioningClient) RetryProvisioning(ctx context.Context, all bool, machines ...names.MachineTag) (
	[]params.ErrorResult, error) {

	if f.err != nil {
		return nil, f.err
	}

	results := make([]params.ErrorResult, len(machines))

	if all {
		machines = []names.MachineTag{names.NewMachineTag("0")}
	}
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
				results[i].Error = apiservererrors.ServerError(
					errors.Errorf("%s is not in an error state",
						names.ReadableString(machine)))
			}
		} else {
			results[i].Error = apiservererrors.ServerError(
				errors.NotFoundf("machine %s", machine.Id()))
		}
	}

	return results, nil
}

func (s *retryProvisioningSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

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
		args: []string{"42", "--all"},
		err:  `specify machines or --all but not both`,
	}, {
		args:   []string{"1"},
		stdErr: `machine 1 is not in an error state`,
	}, {
		args: []string{"0"},
	}, {
		args: []string{"--all"},
	}, {
		args:   []string{"0", "1"},
		stdErr: `machine 1 is not in an error state`,
	}, {
		args: []string{"1", "42"},
		stdErr: `machine 1 is not in an error state` +
			`machine 42 not found`,
	}, {
		args: []string{"0/lxd/0"},
		err:  `invalid machine "0/lxd/0" retry-provisioning does not support containers`,
	},
}

func (s *retryProvisioningSuite) TestRetryProvisioning(c *tc.C) {
	for i, t := range resolvedMachineTests {
		c.Logf("test %d: %v", i, t.args)
		command := model.NewRetryProvisioningCommandForTest(s.fake)
		context, err := cmdtesting.RunCommand(c, command, t.args...)
		if t.err != "" {
			c.Check(err, tc.ErrorMatches, t.err)
			continue
		}
		c.Check(err, tc.ErrorIsNil)
		output := cmdtesting.Stderr(context)
		stripped := strings.Replace(output, "\n", "", -1)
		c.Check(stripped, tc.Equals, t.stdErr)
		if t.args[0] == "0" || t.args[0] == "all" {
			m := s.fake.m["0"]
			c.Check(m.info, tc.Equals, "broken")
			c.Check(m.data["transient"], tc.IsTrue)
		}
	}
}

func (s *retryProvisioningSuite) TestBlockRetryProvisioning(c *tc.C) {
	s.fake.err = apiservererrors.OperationBlockedError("TestBlockRetryProvisioning")

	for i, t := range resolvedMachineTests {
		c.Logf("test %d: %v", i, t.args)
		command := model.NewRetryProvisioningCommandForTest(s.fake)
		_, err := cmdtesting.RunCommand(c, command, t.args...)
		if t.err != "" {
			c.Check(err, tc.ErrorMatches, t.err)
			continue
		}
		testing.AssertOperationWasBlocked(c, err, ".*TestBlockRetryProvisioning.*")
	}
}
