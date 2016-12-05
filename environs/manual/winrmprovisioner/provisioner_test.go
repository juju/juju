package winrmprovisioner_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/manual"
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

// func (s *provisionerSuite) TestProvisionMachine(c *gc.C) {
// 	args := s.getArgs(c)
// 	var stdin, stderr, stdout bytes.Buffer
// 	args.Stdin, args.Stdout, args.Stderr = &stdin, &stderr, &stdout
//
// 	// test provisioning machine without scope
// 	placement := &instance.Placement{
// 		Scope: "no-scope",
// 	}
// 	machineId, err := manual.ProvisionMachine(args, placement)
// 	c.Assert(machineId, jc.DeepEquals, "")
// 	c.Assert(err, gc.Equals, manual.ErrNoProtoScope)
//
// 	// make sure every call on ping, run its runing as a successfull command
// 	// and make the machine act like it'a already provisioned
// 	provisioner := windows.NewProvisioner(args)
// 	provisioner.SecureClient = &fakeWinRM{
// 		password: "",
// 		err:      nil,
// 		fakePing: func() error {
// 			return nil
// 		},
// 		fakeRun: func(cmd string, stdout, stderr io.Writer) error {
// 			fmt.Fprintf(stdout, "Yes")
// 			return nil
// 		},
// 	}
//
// 	// this should return that the machine is already provisioned
// 	machineId, err = provisioner.Provision()
// 	c.Assert(machineId, gc.Equals, "")
// 	c.Assert(err, jc.DeepEquals, common.ErrProvisioned)
//
// 	provisioner.SecureClient = &fakeWinRM{
// 		password: "",
// 		err:      nil,
// 		fakePing: func() error {
// 			return nil
// 		},
// 		fakeRun: func(cmd string, stdout, stderr io.Writer) error {
// 			return nil
// 		},
// 	}
//
// 	// this should return that no maching harware info are actually found.
// 	machineId, err = provisioner.Provision()
// 	c.Assert(err, gc.NotNil)
// 	c.Assert(machineId, jc.DeepEquals, "")
//
// 	provisioner.SecureClient = &fakeWinRM{
// 		password: "",
// 		err:      nil,
// 		fakePing: func() error {
// 			return nil
// 		},
// 		fakeRun: func(cmd string, stdout, stderr io.Writer) error {
// 			c.Assert((len(cmd) > 0), gc.Equals, true)
// 			fmt.Fprintf(stdout, "amd64\r\n")
// 			fmt.Fprintf(stdout, "16\r\n")
// 			fmt.Fprintf(stdout, "win2012r2\r\n")
// 			fmt.Fprintf(stdout, "4\r\n")
// 			return nil
// 		},
// 	}
//
// 	// this should return that no matching tools are actually found.
// 	machineId, err = provisioner.Provision()
// 	c.Assert(err, gc.NotNil)
// 	c.Assert(machineId, jc.DeepEquals, "")
//
// 	arch := "amd64"
// 	series := "win2012r2"
// 	defaultToolsURL := envtools.DefaultBaseURL
// 	envtools.DefaultBaseURL = ""
//
// 	cfg := s.Environ.Config()
// 	number, ok := cfg.AgentVersion()
// 	c.Assert(ok, jc.IsTrue)
// 	binVersion := version.Binary{
// 		Number: number,
// 		Series: series,
// 		Arch:   arch,
// 	}
// 	envtools.DefaultBaseURL = defaultToolsURL
// 	envtesting.AssertUploadFakeToolsVersions(c, s.DefaultToolsStorage, "released", "released", binVersion)
// }
