// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"context"
	"strings"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/cmd/cmdtesting"
	"github.com/juju/juju/cmd/juju/machine"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
)

func TestReprovisionMachineSuite(t *stdtesting.T) {
	tc.Run(t, &reprovisionMachineSuite{})
}

type reprovisionMachineSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fake *fakeReprovisionMachineClient
}

// fakeReprovisionMachineClient mocks the API client for reprovision-machine.
type fakeReprovisionMachineClient struct {
	err        error
	machineErr *params.Error
}

func (f *fakeReprovisionMachineClient) Close() error {
	return nil
}

func (f *fakeReprovisionMachineClient) ReprovisionMachine(ctx context.Context, machine string, force bool) (params.ErrorResult, error) {
	if f.err != nil {
		return params.ErrorResult{}, f.err
	}
	if f.machineErr != nil {
		return params.ErrorResult{Error: f.machineErr}, nil
	}
	return params.ErrorResult{}, nil
}

func (s *reprovisionMachineSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.fake = &fakeReprovisionMachineClient{}
}

func (s *reprovisionMachineSuite) TestNoArgs(c *tc.C) {
	command := machine.NewReprovisionMachineCommandForTest(s.fake)
	_, err := cmdtesting.RunCommand(c, command)
	c.Check(err, tc.ErrorMatches, "no machine specified")
}

func (s *reprovisionMachineSuite) TestTooManyArgs(c *tc.C) {
	command := machine.NewReprovisionMachineCommandForTest(s.fake)
	_, err := cmdtesting.RunCommand(c, command, "0", "1")
	c.Check(err, tc.ErrorMatches, "expected exactly one machine, got 2")
}

func (s *reprovisionMachineSuite) TestInvalidMachine(c *tc.C) {
	command := machine.NewReprovisionMachineCommandForTest(s.fake)
	_, err := cmdtesting.RunCommand(c, command, "not-a-machine")
	c.Check(err, tc.ErrorMatches, `invalid machine "not-a-machine"`)
}

func (s *reprovisionMachineSuite) TestContainerMachine(c *tc.C) {
	command := machine.NewReprovisionMachineCommandForTest(s.fake)
	_, err := cmdtesting.RunCommand(c, command, "0/lxd/0")
	c.Check(err, tc.ErrorMatches, `invalid machine "0/lxd/0" reprovision-machine does not support containers`)
}

func (s *reprovisionMachineSuite) TestSuccess(c *tc.C) {
	command := machine.NewReprovisionMachineCommandForTest(s.fake)
	ctx, err := cmdtesting.RunCommand(c, command, "0", "--force")
	c.Check(err, tc.ErrorIsNil)
	output := cmdtesting.Stdout(ctx)
	c.Check(strings.TrimSpace(output), tc.Equals, "reprovisioning machine 0")
}

func (s *reprovisionMachineSuite) TestAPIError(c *tc.C) {
	s.fake.machineErr = &params.Error{Message: "API error message"}
	command := machine.NewReprovisionMachineCommandForTest(s.fake)
	ctx, err := cmdtesting.RunCommand(c, command, "0", "--force")
	c.Check(err, tc.ErrorIsNil)
	stderr := cmdtesting.Stderr(ctx)
	c.Check(strings.TrimSpace(stderr), tc.Equals, "API error message")
}

func (s *reprovisionMachineSuite) TestBlockedError(c *tc.C) {
	s.fake.err = apiservererrors.OperationBlockedError("TestBlockReprovisionMachine")
	command := machine.NewReprovisionMachineCommandForTest(s.fake)
	_, err := cmdtesting.RunCommand(c, command, "0", "--force")
	testing.AssertOperationWasBlocked(c, err, ".*TestBlockReprovisionMachine.*")
}

func (s *reprovisionMachineSuite) TestConnectionError(c *tc.C) {
	s.fake.err = errors.New("connection refused")
	command := machine.NewReprovisionMachineCommandForTest(s.fake)
	_, err := cmdtesting.RunCommand(c, command, "0", "--force")
	c.Check(err, tc.ErrorMatches, "connection refused")
}

func (s *reprovisionMachineSuite) TestNotSupported(c *tc.C) {
	s.fake.err = &rpc.RequestError{
		Message: `unknown method "ReprovisionMachine" at version 11 for facade type "MachineManager"`,
		Code:    "not implemented",
	}
	command := machine.NewReprovisionMachineCommandForTest(s.fake)
	_, err := cmdtesting.RunCommand(c, command, "0", "--force")
	c.Check(err, tc.ErrorMatches,
		"reprovision-machine is not supported by this controller; "+
			"the controller must be upgraded to a version that supports reprovisioning")
}
