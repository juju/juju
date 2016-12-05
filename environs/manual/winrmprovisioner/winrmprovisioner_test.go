package winrmprovisioner_test

import (
	"github.com/juju/juju/juju/testing"

	gc "gopkg.in/check.v1"
)

type winrmprovisionerSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&winrmprovisionerSuite{})
var (
	key  = "winrmkey.pem"
	cert = "winrmcert.crt"
)

// func (w *winrmprovisionerSuite) TestInitAdministratorError(c *gc.C) {
// 	winrm.ResetClientCert()
//
// 	var stdout, stderr bytes.Buffer
// 	args := manual.ProvisionMachineArgs{
// 		Host:   winrmListenerAddr,
// 		User:   "Administrator",
// 		Stdout: &stdout,
// 		Stderr: &stderr,
// 	}
//
// 	// the intent is that it will return a valid error of
// 	// Can't retrive password from terminal: inappropriate ioctl for device
// 	// because the stdin is not a terminal
// 	err := winrmprovisioner.InitAdministratorUser(args)
// 	c.Assert((stdout.Len() > 0), gc.Equals, true) // it contins the prompt display
// 	c.Assert(stderr.Len(), gc.Equals, 0)
// 	c.Assert(err, gc.NotNil)
//
// 	// overwrite the default client using a custom get password
// 	provision.DefaultClient = winrm.NewClient(args.User, args.Host, func() (string, error) {
// 		return "Password123", nil
// 	}, args.Stdout)
// 	// it will use the default client implementation and getting a valing password
// 	// this tries to connect and it will return a error because no host is listening on that addr/port.
// 	err = windows.InitAdministratorUser(provision)
// 	c.Assert((stdout.Len() > 0), gc.Equals, true)
// 	c.Assert(stderr.Len(), gc.Equals, 0)
// 	c.Assert(err, gc.NotNil)
// 	c.Assert(winrm.Password(), jc.DeepEquals, "Password123")
//
// 	// it will succed because we will use a fake https client implementation that will
// 	// respond to all of the winrm commands as it run sucessfull
// 	provision.SecureClient = &fakeWinRM{
// 		err: nil,
// 		fakePing: func() error {
// 			return nil
// 		},
// 		fakeRun: func(cmd string, stdout, stderr io.Writer) error {
// 			ok := strings.Contains(cmd, "ECHO HI!")
// 			c.Assert(ok, gc.Equals, true)
// 			fmt.Fprintf(stdout, "HI\n")
// 			return nil
// 		},
// 	}
// 	err = windows.InitAdministratorUser(provision)
// 	c.Assert(err, gc.IsNil)
//
// 	// test if the return value is returned to the caller
// 	// test if the secure and also default one fails
// 	f := &fakeWinRM{
// 		err: nil,
// 		fakePing: func() error {
// 			return NoValue
// 		},
// 		fakeRun: func(cmd string, stdout, stderr io.Writer) error {
// 			ok := strings.Contains(cmd, "ECHO HI!")
// 			c.Assert(ok, gc.Equals, true)
// 			fmt.Fprintf(stdout, "some random stdout")
// 			return NoValue
// 		},
// 	}
// 	provision.SecureClient = f
// 	provision.DefaultClient = f
// 	err = windows.InitAdministratorUser(provision)
// 	c.Assert(provision.SecureClient.Error(), jc.DeepEquals, provision.DefaultClient.Error())
// 	ok := strings.Contains(err.Error(), NoValue.Error())
// 	c.Assert(ok, jc.IsTrue)
//
// 	err = os.RemoveAll(base) //cleanup
// 	c.Assert(err, gc.IsNil)
// 	// test if the x509 certs are properly loaded
// 	// but still expect to fail because there we will not init the x509 keys
// 	provision = windows.NewProvisioner(args)
// 	// modify just the default one to fail
// 	provision.DefaultClient = &fakeWinRM{
// 		err: nil,
// 		fakePing: func() error {
// 			return NoValue
// 		},
// 		fakeRun: func(cmd string, stdout, stderr io.Writer) error {
// 			ok := strings.Contains(cmd, "ECHO HI!")
// 			c.Assert(ok, gc.Equals, true)
// 			fmt.Fprintf(stdout, "some random stdout\n")
// 			return NoValue
// 		},
// 	}
// 	err = windows.InitAdministratorUser(provision)
// 	c.Assert(err, gc.NotNil)
// 	c.Assert(provision.SecureClient.Error(), gc.NotNil)
// 	c.Assert(provision.DefaultClient.Error(), gc.Equals, NoValue)
// 	ok = strings.Contains(err.Error(), "Can't parse this key and certs")
// 	c.Assert(ok, jc.IsTrue)
//
// 	// laod cert and test if this trows now the ErrPing
// 	err = winrm.LoadClientCert(
// 		filepath.Join(base, key),
// 		filepath.Join(base, cert),
// 	)
// 	c.Assert(err, gc.IsNil)
//
// 	provision = windows.NewProvisioner(args)
// 	provision.DefaultClient = &fakeWinRM{
// 		err: nil,
// 		fakePing: func() error {
// 			return NoValue
// 		},
// 		fakeRun: func(cmd string, stdout, stderr io.Writer) error {
// 			ok := strings.Contains(cmd, "ECHO HI!")
// 			c.Assert(ok, gc.Equals, true)
// 			fmt.Fprintf(stdout, "some random stdout\n")
// 			return NoValue
// 		},
// 	}
// 	// we now have the certs loaded into the mem
// 	// so the clients can load easly the key and cert
// 	err = windows.InitAdministratorUser(provision)
// 	c.Assert(err, gc.NotNil)
// 	ok = strings.Contains(err.Error(), winrm.ErrPing.Error())
// 	c.Assert(ok, jc.IsTrue)
// 	c.Assert(provision.DefaultClient.Error(), gc.Equals, NoValue)
// 	c.Assert(provision.SecureClient.Error(), gc.NotNil)
//
// 	winrm.ResetClientCert()
// 	err = os.RemoveAll(base) //cleanup
// 	c.Assert(err, gc.IsNil)
// }
//
// func (w *windowsSuite) TestDetectSeriesAndHardwareCharacteristics(c *gc.C) {
// 	winrm.ResetClientCert()
// 	err := os.RemoveAll(base)
// 	c.Assert(err, gc.IsNil)
// 	err = winrm.LoadClientCert(
// 		filepath.Join(base, key),
// 		filepath.Join(base, cert),
// 	)
// 	c.Assert(err, gc.IsNil)
// 	arch := "amd64"
// 	mem := uint64(16)
// 	series := "win2012r2"
// 	cores := uint64(4)
// 	winrm.UseClient(&fakeWinRM{
// 		password: "",
// 		err:      nil,
// 		fakePing: nil,
// 		fakeRun: func(cmd string, stdout, stderr io.Writer) error {
// 			c.Assert((len(cmd) > 0), gc.Equals, true)
// 			fmt.Fprintf(stdout, "amd64\r\n")
// 			fmt.Fprintf(stdout, "16\r\n")
// 			fmt.Fprintf(stdout, "win2012r2\r\n")
// 			fmt.Fprintf(stdout, "4\r\n")
// 			return nil
// 		},
// 	})
// 	hc, ser, err := windows.DetectSeriesAndHardwareCharacteristics(winrmListenerAddr)
// 	c.Assert(err, gc.IsNil)
// 	c.Assert(winrm.Error(), gc.IsNil)
// 	c.Assert(winrm.Password(), jc.DeepEquals, "")
// 	c.Assert((len(ser) > 0), gc.Equals, true)
// 	c.Assert(hc, gc.NotNil)
// 	c.Assert(ser, jc.DeepEquals, series)
// 	c.Assert(*hc.Arch, jc.DeepEquals, arch)
// 	c.Assert(*hc.Mem, jc.DeepEquals, mem)
// 	c.Assert(*hc.CpuCores, jc.DeepEquals, cores)
// 	winrm.ResetClientCert()
// 	err = os.RemoveAll(base)
// 	c.Assert(err, gc.IsNil)
// }
