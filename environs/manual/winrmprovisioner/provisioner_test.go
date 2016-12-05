package winrmprovisioner_test

import (
	"bytes"
	"fmt"
	"io"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/environs/manual/winrmprovisioner"
	"github.com/juju/juju/juju/testing"
)

type provisionerSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&provisionerSuite{})

func (s *provisionerSuite) getArgs(c *gc.C) manual.ProvisionMachineArgs {
	client := s.APIState.Client()
	s.AddCleanup(func(*gc.C) { client.Close() })

	return manual.ProvisionMachineArgs{
		Host:   winrmListenerAddr,
		User:   "Administrator",
		Client: client,
	}
}

func (s *provisionerSuite) TestProvisionMachine(c *gc.C) {
	var err error
	args := s.getArgs(c)
	var stdin, stderr, stdout bytes.Buffer
	args.Stdin, args.Stdout, args.Stderr = &stdin, &stderr, &stdout

	args.WClient = &fakeWinRM{
		fakePing: func() error {
			return nil
		},
		fakeRun: func(cmd string, stdout, stderr io.Writer) error {
			return nil
		},
	}

	// this should return this error
	// No hardware fields on runing the powershell deteciton script
	machineId, err := winrmprovisioner.ProvisionMachine(args)
	c.Assert(err, gc.NotNil)
	c.Assert(machineId, jc.DeepEquals, "")

	args.WClient = &fakeWinRM{
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
	// this should return this error
	// getting instance config: finding tools: no matching tools available
	machineId, err = winrmprovisioner.ProvisionMachine(args)
	c.Assert(err, gc.NotNil)
	c.Assert(machineId, jc.DeepEquals, "")

	// this should return that the machine is already provisioned
	args.WClient = &fakeWinRM{
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
