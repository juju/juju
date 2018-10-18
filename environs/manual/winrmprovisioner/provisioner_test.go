// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package winrmprovisioner_test

import (
	"bytes"
	"fmt"
	"io"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/environs/manual/winrmprovisioner"
)

type TestClientAPI struct{}

func (t TestClientAPI) AddMachines(p []params.AddMachineParams) ([]params.AddMachinesResult, error) {
	return make([]params.AddMachinesResult, 1, 1), nil
}

func (t TestClientAPI) ForceDestroyMachines(machines ...string) error {
	if machines == nil {
		return fmt.Errorf("epty machines")
	}
	return nil
}

func (t TestClientAPI) ProvisioningScript(param params.ProvisioningScriptParams) (script string, err error) {
	return "magnifi script", nil
}

type provisionerSuite struct {
	client *TestClientAPI
}

var _ = gc.Suite(&provisionerSuite{})

func (s *provisionerSuite) getArgs(c *gc.C) manual.ProvisionMachineArgs {
	s.client = &TestClientAPI{}

	return manual.ProvisionMachineArgs{
		Host:   winrmListenerAddr,
		User:   "Administrator",
		Client: s.client,
	}
}

func (s *provisionerSuite) TestProvisionMachine(c *gc.C) {
	var err error
	args := s.getArgs(c)
	var stdin, stderr, stdout bytes.Buffer
	args.Stdin, args.Stdout, args.Stderr = &stdin, &stderr, &stdout

	args.WinRM = manual.WinRMArgs{}
	args.WinRM.Client = &fakeWinRM{
		fakePing: func() error {
			return nil
		},
		fakeRun: func(cmd string, stdout, stderr io.Writer) error {
			return nil
		},
	}

	// this should return this error
	// No hardware fields on running the powershell deteciton script
	machineId, err := winrmprovisioner.ProvisionMachine(args)
	c.Assert(err, gc.NotNil)
	c.Assert(machineId, jc.DeepEquals, "")

	args.WinRM.Client = &fakeWinRM{
		fakePing: func() error {
			return nil
		},

		fakeRun: func(cmd string, stdout, stderr io.Writer) error {
			c.Assert((len(cmd) > 0), gc.Equals, true)
			fmt.Fprintf(stdout, "amd64\r\n")
			fmt.Fprintf(stdout, "16\r\n")
			fmt.Fprintf(stdout, "win2012r2\r\n")
			fmt.Fprintf(stdout, "4\r\n")
			return nil
		},
	}

	machineId, err = winrmprovisioner.ProvisionMachine(args)
	c.Assert(err, gc.IsNil)
	c.Assert(machineId, jc.DeepEquals, "")

	// this should return that the machine is already provisioned
	args.WinRM.Client = &fakeWinRM{
		fakePing: func() error {
			return nil
		},

		fakeRun: func(cmd string, stdout, stderr io.Writer) error {
			c.Assert((len(cmd) > 0), gc.Equals, true)
			fmt.Fprintf(stdout, "Yes\r\n")
			return nil
		},
	}
	machineId, err = winrmprovisioner.ProvisionMachine(args)
	c.Assert(err.Error(), jc.DeepEquals, "machine is already provisioned")
	c.Assert(machineId, jc.DeepEquals, "")
}
