// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"strconv"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/machine"
	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type AddMachineSuite struct {
	testing.FakeJujuHomeSuite
	fakeAddMachine     *fakeAddMachineAPI
	fakeMachineManager *fakeMachineManagerAPI
}

var _ = gc.Suite(&AddMachineSuite{})

func (s *AddMachineSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.fakeAddMachine = &fakeAddMachineAPI{}
	s.fakeAddMachine.agentVersion = "1.21.0"
	s.fakeMachineManager = &fakeMachineManagerAPI{}
}

func (s *AddMachineSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		args        []string
		series      string
		constraints string
		placement   string
		count       int
		errorString string
	}{
		{
			count: 1,
		}, {
			args:   []string{"--series", "some-series"},
			count:  1,
			series: "some-series",
		}, {
			args:  []string{"-n", "2"},
			count: 2,
		}, {
			args:      []string{"lxc"},
			count:     1,
			placement: "lxc:",
		}, {
			args:      []string{"lxc", "-n", "2"},
			count:     2,
			placement: "lxc:",
		}, {
			args:      []string{"lxc:4"},
			count:     1,
			placement: "lxc:4",
		}, {
			args:        []string{"--constraints", "mem=8G"},
			count:       1,
			constraints: "mem=8192M",
		}, {
			args:        []string{"--constraints", "container=lxc"},
			errorString: `container constraint "lxc" not allowed when adding a machine`,
		}, {
			args:      []string{"ssh:user@10.10.0.3"},
			count:     1,
			placement: "ssh:user@10.10.0.3",
		}, {
			args:      []string{"zone=us-east-1a"},
			count:     1,
			placement: "env-uuid:zone=us-east-1a",
		}, {
			args:      []string{"anything-here"},
			count:     1,
			placement: "env-uuid:anything-here",
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
		addCmd := &machine.AddCommand{}
		err := testing.InitCommand(addCmd, test.args)
		if test.errorString == "" {
			c.Check(err, jc.ErrorIsNil)
			c.Check(addCmd.Series, gc.Equals, test.series)
			c.Check(addCmd.Constraints.String(), gc.Equals, test.constraints)
			if addCmd.Placement != nil {
				c.Check(addCmd.Placement.String(), gc.Equals, test.placement)
			} else {
				c.Check("", gc.Equals, test.placement)
			}
			c.Check(addCmd.NumMachines, gc.Equals, test.count)
		} else {
			c.Check(err, gc.ErrorMatches, test.errorString)
		}
	}
}

func (s *AddMachineSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	add := machine.NewAddCommand(s.fakeAddMachine, s.fakeMachineManager)
	return testing.RunCommand(c, envcmd.Wrap(add), args...)
}

func (s *AddMachineSuite) TestAddMachine(c *gc.C) {
	context, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stderr(context), gc.Equals, "created machine 0\n")

	c.Assert(s.fakeAddMachine.args, gc.HasLen, 1)
	param := s.fakeAddMachine.args[0]
	c.Assert(param.Jobs, jc.DeepEquals, []multiwatcher.MachineJob{
		multiwatcher.JobHostUnits,
		multiwatcher.JobManageNetworking,
	})
}

func (s *AddMachineSuite) TestSSHPlacement(c *gc.C) {
	s.PatchValue(machine.ManualProvisioner, func(args manual.ProvisionMachineArgs) (string, error) {
		return "42", nil
	})
	context, err := s.run(c, "ssh:10.1.2.3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stderr(context), gc.Equals, "created machine 42\n")
}

func (s *AddMachineSuite) TestSSHPlacementError(c *gc.C) {
	s.PatchValue(machine.ManualProvisioner, func(args manual.ProvisionMachineArgs) (string, error) {
		return "", errors.New("failed to initialize warp core")
	})
	context, err := s.run(c, "ssh:10.1.2.3")
	c.Assert(err, gc.ErrorMatches, "failed to initialize warp core")
	c.Assert(testing.Stderr(context), gc.Equals, "")
}

func (s *AddMachineSuite) TestParamsPassedOn(c *gc.C) {
	_, err := s.run(c, "--constraints", "mem=8G", "--series=special", "zone=nz")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fakeAddMachine.args, gc.HasLen, 1)
	param := s.fakeAddMachine.args[0]
	c.Assert(param.Placement.String(), gc.Equals, "fake-uuid:zone=nz")
	c.Assert(param.Series, gc.Equals, "special")
	c.Assert(param.Constraints.String(), gc.Equals, "mem=8192M")
}

func (s *AddMachineSuite) TestParamsPassedOnNTimes(c *gc.C) {
	_, err := s.run(c, "-n", "3", "--constraints", "mem=8G", "--series=special")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fakeAddMachine.args, gc.HasLen, 3)
	param := s.fakeAddMachine.args[0]
	c.Assert(param.Series, gc.Equals, "special")
	c.Assert(param.Constraints.String(), gc.Equals, "mem=8192M")
	c.Assert(s.fakeAddMachine.args[0], jc.DeepEquals, s.fakeAddMachine.args[1])
	c.Assert(s.fakeAddMachine.args[0], jc.DeepEquals, s.fakeAddMachine.args[2])
}

func (s *AddMachineSuite) TestAddThreeMachinesWithTwoFailures(c *gc.C) {
	s.fakeAddMachine.successOrder = []bool{true, false, false}
	expectedOutput := `created machine 0
failed to create 2 machines
`
	context, err := s.run(c, "-n", "3")
	c.Assert(err, gc.ErrorMatches, "something went wrong, something went wrong")
	c.Assert(testing.Stderr(context), gc.Equals, expectedOutput)
}

func (s *AddMachineSuite) TestBlockedError(c *gc.C) {
	s.fakeAddMachine.addError = common.ErrOperationBlocked("TestBlockedError")
	_, err := s.run(c)
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Check(stripped, gc.Matches, ".*TestBlockedError.*")
}

func (s *AddMachineSuite) TestServerIsPreJobManageNetworking(c *gc.C) {
	s.fakeAddMachine.agentVersion = "1.18.1"
	_, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.fakeAddMachine.args, gc.HasLen, 1)
	param := s.fakeAddMachine.args[0]
	c.Assert(param.Jobs, jc.DeepEquals, []multiwatcher.MachineJob{
		multiwatcher.JobHostUnits,
	})
}

func (s *AddMachineSuite) TestAddMachineWithDisks(c *gc.C) {
	s.fakeMachineManager.apiVersion = 1
	_, err := s.run(c, "--disks", "2,1G", "--disks", "2G")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fakeAddMachine.args, gc.HasLen, 0)
	c.Assert(s.fakeMachineManager.args, gc.HasLen, 1)
	param := s.fakeMachineManager.args[0]
	c.Assert(param.Disks, gc.DeepEquals, []storage.Constraints{
		{Size: 1024, Count: 2},
		{Size: 2048, Count: 1},
	})
}

func (s *AddMachineSuite) TestAddMachineWithDisksUnsupported(c *gc.C) {
	_, err := s.run(c, "--disks", "2,1G", "--disks", "2G")
	c.Assert(err, gc.ErrorMatches, "cannot add machines with disks: not supported by the API server")
}

type fakeAddMachineAPI struct {
	successOrder []bool
	currentOp    int
	args         []params.AddMachineParams
	addError     error
	agentVersion interface{}
}

func (f *fakeAddMachineAPI) Close() error {
	return nil
}

func (f *fakeAddMachineAPI) EnvironmentUUID() string {
	return "fake-uuid"
}

func (f *fakeAddMachineAPI) AddMachines(args []params.AddMachineParams) ([]params.AddMachinesResult, error) {
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
				Machine: string(i),
				Error:   &params.Error{"something went wrong", "1"},
			})
		}
		f.currentOp++
	}
	return results, nil
}

func (f *fakeAddMachineAPI) AddMachines1dot18(args []params.AddMachineParams) ([]params.AddMachinesResult, error) {
	return f.AddMachines(args)
}

func (f *fakeAddMachineAPI) ForceDestroyMachines(machines ...string) error {
	return errors.NotImplementedf("ForceDestroyMachines")
}

func (f *fakeAddMachineAPI) ProvisioningScript(params.ProvisioningScriptParams) (script string, err error) {
	return "", errors.NotImplementedf("ProvisioningScript")
}

func (f *fakeAddMachineAPI) EnvironmentGet() (map[string]interface{}, error) {
	return map[string]interface{}{"agent-version": f.agentVersion}, nil
}

type fakeMachineManagerAPI struct {
	apiVersion int
	fakeAddMachineAPI
}

func (f *fakeMachineManagerAPI) BestAPIVersion() int {
	return f.apiVersion
}
