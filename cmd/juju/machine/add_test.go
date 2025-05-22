// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"context"
	"strconv"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type AddMachineSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fakeAddMachine *fakeAddMachineAPI
}

func TestAddMachineSuite(t *stdtesting.T) {
	tc.Run(t, &AddMachineSuite{})
}

func (s *AddMachineSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.fakeAddMachine = &fakeAddMachineAPI{}
}

func (s *AddMachineSuite) TestInit(c *tc.C) {
	for i, test := range []struct {
		args        []string
		base        string
		constraints string
		placement   string
		count       int
		errorString string
	}{
		{
			count: 1,
		}, {
			args:  []string{"--base", "some-series"},
			count: 1,
			base:  "some-series",
		}, {
			args:  []string{"-n", "2"},
			count: 2,
		}, {
			args:      []string{"lxd"},
			count:     1,
			placement: "lxd:",
		}, {
			args:      []string{"lxd", "-n", "2"},
			count:     2,
			placement: "lxd:",
		}, {
			args:      []string{"lxd:4"},
			count:     1,
			placement: "lxd:4",
		}, {
			args:      []string{"ssh:user@10.10.0.3"},
			count:     1,
			placement: "ssh:user@10.10.0.3",
		}, {
			args:      []string{"winrm:user@10.10.0.3"},
			count:     1,
			placement: "winrm:user@10.10.0.3",
		}, {
			args:      []string{"ssh:user@10.10.0.3", "--private-key", "pv"},
			count:     1,
			placement: "ssh:user@10.10.0.3",
		}, {
			args:      []string{"ssh:user@10.10.0.3", "--public-key", "pb"},
			count:     1,
			placement: "ssh:user@10.10.0.3",
		}, {
			args:      []string{"ssh:user@10.10.0.3", "--private-key", "pv", "--public-key", "pb"},
			count:     1,
			placement: "ssh:user@10.10.0.3",
		}, {
			args:      []string{"zone=us-east-1a"},
			count:     1,
			placement: "model-uuid:zone=us-east-1a",
		}, {
			args:      []string{"anything-here"},
			count:     1,
			placement: "model-uuid:anything-here",
		}, {
			args:        []string{"anything", "else"},
			errorString: `unrecognized args: \["else"\]`,
		}, {
			args:      []string{"something:special"},
			count:     1,
			placement: "something:special",
		},
	} {
		c.Logf("test %d", i)
		wrappedCommand, addCmd := machine.NewAddCommandForTest(s.fakeAddMachine, s.fakeAddMachine)
		err := cmdtesting.InitCommand(wrappedCommand, test.args)
		if test.errorString == "" {
			c.Check(err, tc.ErrorIsNil)
			c.Check(addCmd.Base, tc.Equals, test.base)
			c.Check(addCmd.Constraints.String(), tc.Equals, test.constraints)
			if addCmd.Placement != nil {
				c.Check(addCmd.Placement.String(), tc.Equals, test.placement)
			} else {
				c.Check("", tc.Equals, test.placement)
			}
			c.Check(addCmd.NumMachines, tc.Equals, test.count)
		} else {
			c.Check(err, tc.ErrorMatches, test.errorString)
		}
	}
}

func (s *AddMachineSuite) run(c *tc.C, args ...string) (*cmd.Context, error) {
	add, _ := machine.NewAddCommandForTest(s.fakeAddMachine, s.fakeAddMachine)
	return cmdtesting.RunCommand(c, add, args...)
}

func (s *AddMachineSuite) TestAddMachine(c *tc.C) {
	context, err := s.run(c)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "created machine 0\n")

	c.Assert(s.fakeAddMachine.args, tc.HasLen, 1)
	c.Assert(s.fakeAddMachine.args[0].Jobs, tc.DeepEquals, []model.MachineJob{model.JobHostUnits})
}

func (s *AddMachineSuite) TestAddMachineUnauthorizedMentionsJujuGrant(c *tc.C) {
	s.fakeAddMachine.addModelGetError = &params.Error{
		Message: "permission denied",
		Code:    params.CodeUnauthorized,
	}
	ctx, _ := s.run(c)
	errString := strings.Replace(cmdtesting.Stderr(ctx), "\n", " ", -1)
	c.Assert(errString, tc.Matches, `.*juju grant.*`)
}

func (s *AddMachineSuite) TestSSHPlacement(c *tc.C) {
	s.PatchValue(machine.SSHProvisioner, func(_ context.Context, args manual.ProvisionMachineArgs) (string, error) {
		return "42", nil
	})
	context, err := s.run(c, "ssh:10.1.2.3")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "created machine 42\n")
}

func (s *AddMachineSuite) TestSSHPlacementError(c *tc.C) {
	s.PatchValue(machine.SSHProvisioner, func(_ context.Context, args manual.ProvisionMachineArgs) (string, error) {
		return "", errors.New("failed to initialize warp core")
	})
	context, err := s.run(c, "ssh:10.1.2.3")
	c.Assert(err, tc.ErrorMatches, "failed to initialize warp core")
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "")
}

func (s *AddMachineSuite) TestParamsPassedOn(c *tc.C) {
	_, err := s.run(c, "--constraints", "mem=8G", "--base=ubuntu@22.04", "zone=nz")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.fakeAddMachine.args, tc.HasLen, 1)

	param := s.fakeAddMachine.args[0]

	c.Assert(param.Placement.String(), tc.Equals, "fake-uuid:zone=nz")
	c.Assert(param.Base, tc.DeepEquals, &params.Base{Name: "ubuntu", Channel: "22.04/stable"})
	c.Assert(param.Constraints.String(), tc.Equals, "mem=8192M")
}

func (s *AddMachineSuite) TestParamsPassedOnMultipleConstraints(c *tc.C) {
	_, err := s.run(c, "--constraints", "mem=8G", "--constraints", "cores=4", "--base=ubuntu@22.04", "zone=nz")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.fakeAddMachine.args, tc.HasLen, 1)

	param := s.fakeAddMachine.args[0]

	c.Assert(param.Placement.String(), tc.Equals, "fake-uuid:zone=nz")
	c.Assert(param.Base, tc.DeepEquals, &params.Base{Name: "ubuntu", Channel: "22.04/stable"})
	c.Assert(param.Constraints.String(), tc.Equals, "cores=4 mem=8192M")
}

func (s *AddMachineSuite) TestParamsPassedOnNTimes(c *tc.C) {
	_, err := s.run(c, "-n", "3", "--constraints", "mem=8G", "--base=ubuntu@22.04")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.fakeAddMachine.args, tc.HasLen, 3)

	param := s.fakeAddMachine.args[0]
	c.Assert(param.Base, tc.DeepEquals, &params.Base{Name: "ubuntu", Channel: "22.04/stable"})

	c.Assert(param.Constraints.String(), tc.Equals, "mem=8192M")
	c.Assert(param, tc.DeepEquals, s.fakeAddMachine.args[1])
	c.Assert(param, tc.DeepEquals, s.fakeAddMachine.args[2])
}

func (s *AddMachineSuite) TestParamsPassedOnNTimesMultipleConstraints(c *tc.C) {
	_, err := s.run(c, "-n", "3", "--constraints", "mem=8G", "--constraints", "cores=4", "--base=ubuntu@22.04")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.fakeAddMachine.args, tc.HasLen, 3)

	param := s.fakeAddMachine.args[0]
	c.Assert(param.Base, tc.DeepEquals, &params.Base{Name: "ubuntu", Channel: "22.04/stable"})

	c.Assert(param.Constraints.String(), tc.Equals, "cores=4 mem=8192M")
	c.Assert(param, tc.DeepEquals, s.fakeAddMachine.args[1])
	c.Assert(param, tc.DeepEquals, s.fakeAddMachine.args[2])
}

func (s *AddMachineSuite) TestAddThreeMachinesWithTwoFailures(c *tc.C) {
	s.fakeAddMachine.successOrder = []bool{true, false, false}
	expectedOutput := `created machine 0
failed to create 2 machines
`
	context, err := s.run(c, "-n", "3")
	c.Assert(err, tc.ErrorMatches, "something went wrong, something went wrong")
	c.Assert(cmdtesting.Stderr(context), tc.Equals, expectedOutput)
}

func (s *AddMachineSuite) TestBlockedError(c *tc.C) {
	s.fakeAddMachine.addError = apiservererrors.OperationBlockedError("TestBlockedError")
	_, err := s.run(c)
	testing.AssertOperationWasBlocked(c, err, ".*TestBlockedError.*")
}

func (s *AddMachineSuite) TestAddMachineWithDisks(c *tc.C) {
	_, err := s.run(c, "--disks", "2,1G", "--disks", "2G")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.fakeAddMachine.args, tc.HasLen, 1)
	param := s.fakeAddMachine.args[0]
	c.Assert(param.Disks, tc.DeepEquals, []storage.Directive{
		{Size: 1024, Count: 2},
		{Size: 2048, Count: 1},
	})
}

type fakeAddMachineAPI struct {
	successOrder     []bool
	currentOp        int
	args             []params.AddMachineParams
	addError         error
	addModelGetError error
	providerType     string
}

func (f *fakeAddMachineAPI) Close() error {
	return nil
}

func (f *fakeAddMachineAPI) ModelUUID() (string, bool) {
	return "fake-uuid", true
}

func (f *fakeAddMachineAPI) AddMachines(ctx context.Context, args []params.AddMachineParams) ([]params.AddMachinesResult, error) {
	if f.addError != nil {
		return nil, f.addError
	}
	results := []params.AddMachinesResult{}
	for i := range args {
		f.args = append(f.args, args[i])
		if i >= len(f.successOrder) || f.successOrder[i] {
			results = append(results, params.AddMachinesResult{
				Machine: strconv.Itoa(i),
				Error:   nil,
			})
		} else {
			results = append(results, params.AddMachinesResult{
				Machine: string(rune(i)),
				Error:   &params.Error{Message: "something went wrong", Code: "1"},
			})
		}
		f.currentOp++
	}
	return results, nil
}

func (f *fakeAddMachineAPI) DestroyMachinesWithParams(ctx context.Context, force, keep, dryRun bool, maxWait *time.Duration, machines ...string) ([]params.DestroyMachineResult, error) {
	return nil, errors.NotImplementedf("ForceDestroyMachinesWithParams")
}

func (f *fakeAddMachineAPI) ProvisioningScript(ctx context.Context, p params.ProvisioningScriptParams) (script string, err error) {
	return "", errors.NotImplementedf("ProvisioningScript")
}

func (f *fakeAddMachineAPI) ModelGet(ctx context.Context) (map[string]interface{}, error) {
	if f.addModelGetError != nil {
		return nil, f.addModelGetError
	}
	providerType := "dummy"
	if f.providerType != "" {
		providerType = f.providerType
	}
	return testing.FakeConfig().Merge(map[string]interface{}{
		"type": providerType,
	}), nil
}
