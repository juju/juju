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

type winrmprovisionerSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&winrmprovisionerSuite{})

func (w *winrmprovisionerSuite) TestInitAdministratorError(c *gc.C) {
	var stdout, stderr bytes.Buffer
	args := manual.ProvisionMachineArgs{
		Host:   winrmListenerAddr,
		User:   "Administrator",
		Stdout: &stdout,
		Stderr: &stderr,
		WClient: &fakeWinRM{
			fakePing: func() error {
				return nil
			},
		},
	}

	err := winrmprovisioner.InitAdministratorUser(args)
	c.Assert(err, gc.IsNil)

	args.WClient = &fakeWinRM{
		fakePing: func() error {
			return fmt.Errorf("Ping Error")
		},
	}

	// this should return to ioctl device error
	err = winrmprovisioner.InitAdministratorUser(args)
	c.Assert(err, gc.NotNil)
}

func (w *winrmprovisionerSuite) TestDetectSeriesAndHardwareCharacteristics(c *gc.C) {
	arch := "amd64"
	mem := uint64(16)
	series := "win2012r2"
	cores := uint64(4)

	fakeCli := &fakeWinRM{
		fakeRun: func(cmd string, stdout, stderr io.Writer) error {
			c.Assert((len(cmd) > 0), gc.Equals, true)
			fmt.Fprintf(stdout, "amd64\r\n")
			fmt.Fprintf(stdout, "16\r\n")
			fmt.Fprintf(stdout, "win2012r2\r\n")
			fmt.Fprintf(stdout, "4\r\n")
			return nil
		},
	}

	hc, ser, err := winrmprovisioner.DetectSeriesAndHardwareCharacteristics(winrmListenerAddr, fakeCli)
	c.Assert(err, gc.IsNil)
	c.Assert((len(ser) > 0), gc.Equals, true)
	c.Assert(hc, gc.NotNil)
	c.Assert(ser, jc.DeepEquals, series)
	c.Assert(*hc.Arch, jc.DeepEquals, arch)
	c.Assert(*hc.Mem, jc.DeepEquals, mem)
	c.Assert(*hc.CpuCores, jc.DeepEquals, cores)
	c.Assert(err, gc.IsNil)
}

func (w *winrmprovisionerSuite) TestRunProvisionScript(c *gc.C) {
	var stdin, stderr bytes.Buffer
	fakeCli := &fakeWinRM{
		fakeRun: func(cmd string, stdout, stderr io.Writer) error {
			c.Assert((len(cmd) > 0), gc.Equals, true)
			return nil
		},
	}
	err := winrmprovisioner.RunProvisionScript("echo hi!", fakeCli, &stdin, &stderr)
	c.Assert(err, gc.IsNil)
}
