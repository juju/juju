// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rsyslog_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/instance"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils/syslog"
	"launchpad.net/juju-core/worker/rsyslog"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type RsyslogSuite struct {
	jujutesting.JujuConnSuite

	st      *api.State
	machine *state.Machine
}

var _ = gc.Suite(&RsyslogSuite{})

func (s *RsyslogSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)
	// TODO(waigani) 2014-03-19 bug 1294462
	// Add patch for suite functions
	restore := testing.PatchValue(rsyslog.LookupUser, func(username string) (uid, gid int, err error) {
		// worker will not attempt to chown files if uid/gid is 0
		return 0, 0, nil
	})
	s.AddSuiteCleanup(func(*gc.C) { restore() })
}

func (s *RsyslogSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.PatchValue(rsyslog.RestartRsyslog, func() error { return nil })
	s.PatchValue(rsyslog.LogDir, c.MkDir())
	s.PatchValue(rsyslog.RsyslogConfDir, c.MkDir())

	s.st, s.machine = s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	err := s.machine.SetAddresses(instance.NewAddress("0.1.2.3", instance.NetworkUnknown))
	c.Assert(err, gc.IsNil)
}

func waitForFile(c *gc.C, file string) {
	timeout := time.After(coretesting.LongWait)
	for {
		select {
		case <-timeout:
			c.Fatalf("timed out waiting for %s to be written", file)
		case <-time.After(coretesting.ShortWait):
			if _, err := os.Stat(file); err == nil {
				return
			}
		}
	}
}

func waitForRestart(c *gc.C, restarted chan struct{}) {
	timeout := time.After(coretesting.LongWait)
	for {
		select {
		case <-timeout:
			c.Fatalf("timed out waiting for rsyslog to be restarted")
		case <-restarted:
			return
		}
	}
}

func (s *RsyslogSuite) TestStartStop(c *gc.C) {
	st, m := s.OpenAPIAsNewMachine(c, state.JobHostUnits)
	worker, err := rsyslog.NewRsyslogConfigWorker(st.Rsyslog(), rsyslog.RsyslogModeForwarding, m.Tag(), "", []string{"0.1.2.3"})
	c.Assert(err, gc.IsNil)
	worker.Kill()
	c.Assert(worker.Wait(), gc.IsNil)
}

func (s *RsyslogSuite) TestTearDown(c *gc.C) {
	st, m := s.st, s.machine
	worker, err := rsyslog.NewRsyslogConfigWorker(st.Rsyslog(), rsyslog.RsyslogModeAccumulate, m.Tag(), "", []string{"0.1.2.3"})
	c.Assert(err, gc.IsNil)
	confFile := filepath.Join(*rsyslog.RsyslogConfDir, "25-juju.conf")
	// On worker teardown, the rsyslog config file should be removed.
	defer func() {
		_, err := os.Stat(confFile)
		c.Assert(err, jc.Satisfies, os.IsNotExist)
	}()
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()
	waitForFile(c, confFile)
}

func (s *RsyslogSuite) TestModeForwarding(c *gc.C) {
	err := s.APIState.Client().EnvironmentSet(map[string]interface{}{"rsyslog-ca-cert": coretesting.CACert})
	c.Assert(err, gc.IsNil)
	st, m := s.OpenAPIAsNewMachine(c, state.JobHostUnits)
	addrs := []string{"0.1.2.3", "0.2.4.6"}
	worker, err := rsyslog.NewRsyslogConfigWorker(st.Rsyslog(), rsyslog.RsyslogModeForwarding, m.Tag(), "", addrs)
	c.Assert(err, gc.IsNil)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	// We should get a ca-cert.pem with the contents introduced into state config.
	waitForFile(c, filepath.Join(*rsyslog.LogDir, "ca-cert.pem"))
	caCertPEM, err := ioutil.ReadFile(filepath.Join(*rsyslog.LogDir, "ca-cert.pem"))
	c.Assert(err, gc.IsNil)
	c.Assert(string(caCertPEM), gc.DeepEquals, coretesting.CACert)

	// Verify rsyslog configuration.
	waitForFile(c, filepath.Join(*rsyslog.RsyslogConfDir, "25-juju.conf"))
	rsyslogConf, err := ioutil.ReadFile(filepath.Join(*rsyslog.RsyslogConfDir, "25-juju.conf"))
	c.Assert(err, gc.IsNil)

	syslogPort := s.Conn.Environ.Config().SyslogPort()
	syslogConfig := syslog.NewForwardConfig(m.Tag(), *rsyslog.LogDir, syslogPort, "", addrs)
	syslogConfig.ConfigDir = *rsyslog.RsyslogConfDir
	rendered, err := syslogConfig.Render()
	c.Assert(err, gc.IsNil)
	c.Assert(string(rsyslogConf), gc.DeepEquals, string(rendered))
}

func (s *RsyslogSuite) TestModeAccumulate(c *gc.C) {
	st, m := s.st, s.machine
	worker, err := rsyslog.NewRsyslogConfigWorker(st.Rsyslog(), rsyslog.RsyslogModeAccumulate, m.Tag(), "", nil)
	c.Assert(err, gc.IsNil)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()
	waitForFile(c, filepath.Join(*rsyslog.LogDir, "ca-cert.pem"))

	// We should have ca-cert.pem, rsyslog-cert.pem, and rsyslog-key.pem.
	caCertPEM, err := ioutil.ReadFile(filepath.Join(*rsyslog.LogDir, "ca-cert.pem"))
	c.Assert(err, gc.IsNil)
	rsyslogCertPEM, err := ioutil.ReadFile(filepath.Join(*rsyslog.LogDir, "rsyslog-cert.pem"))
	c.Assert(err, gc.IsNil)
	rsyslogKeyPEM, err := ioutil.ReadFile(filepath.Join(*rsyslog.LogDir, "rsyslog-key.pem"))
	c.Assert(err, gc.IsNil)
	_, _, err = cert.ParseCertAndKey(string(rsyslogCertPEM), string(rsyslogKeyPEM))
	c.Assert(err, gc.IsNil)
	err = cert.Verify(string(rsyslogCertPEM), string(caCertPEM), time.Now().UTC())
	c.Assert(err, gc.IsNil)

	// Verify rsyslog configuration.
	waitForFile(c, filepath.Join(*rsyslog.RsyslogConfDir, "25-juju.conf"))
	rsyslogConf, err := ioutil.ReadFile(filepath.Join(*rsyslog.RsyslogConfDir, "25-juju.conf"))
	c.Assert(err, gc.IsNil)

	syslogPort := s.Conn.Environ.Config().SyslogPort()
	syslogConfig := syslog.NewAccumulateConfig(m.Tag(), *rsyslog.LogDir, syslogPort, "", []string{})
	syslogConfig.ConfigDir = *rsyslog.RsyslogConfDir
	rendered, err := syslogConfig.Render()
	c.Assert(err, gc.IsNil)

	c.Assert(string(rsyslogConf), gc.DeepEquals, string(rendered))
}

func (s *RsyslogSuite) TestAccumulateHA(c *gc.C) {
	m := s.machine
	syslogConfig := syslog.NewAccumulateConfig(m.Tag(), *rsyslog.LogDir, 6541, "", []string{"192.168.1", "127.0.0.1"})
	rendered, err := syslogConfig.Render()
	c.Assert(err, gc.IsNil)

	stateServer1Config := ":syslogtag, startswith, \"juju-\" @@192.168.1:6541;LongTagForwardFormat"
	stateServer2Config := ":syslogtag, startswith, \"juju-\" @@127.0.0.1:6541;LongTagForwardFormat"

	c.Assert(strings.Contains(string(rendered), stateServer1Config), gc.Equals, true)
	c.Assert(strings.Contains(string(rendered), stateServer2Config), gc.Equals, true)
}

func (s *RsyslogSuite) TestNamespace(c *gc.C) {
	st := s.st
	// set the rsyslog cert
	err := s.APIState.Client().EnvironmentSet(map[string]interface{}{"rsyslog-ca-cert": coretesting.CACert})
	c.Assert(err, gc.IsNil)

	// namespace only takes effect in filenames
	// for machine-0; all others assume isolation.
	s.testNamespace(c, st, "machine-0", "", "25-juju.conf", *rsyslog.LogDir)
	s.testNamespace(c, st, "machine-0", "mynamespace", "25-juju-mynamespace.conf", *rsyslog.LogDir+"-mynamespace")
	s.testNamespace(c, st, "machine-1", "", "25-juju.conf", *rsyslog.LogDir)
	s.testNamespace(c, st, "machine-1", "mynamespace", "25-juju.conf", *rsyslog.LogDir)
	s.testNamespace(c, st, "unit-myservice-0", "", "26-juju-unit-myservice-0.conf", *rsyslog.LogDir)
	s.testNamespace(c, st, "unit-myservice-0", "mynamespace", "26-juju-unit-myservice-0.conf", *rsyslog.LogDir)
}

// testNamespace starts a worker and ensures that
// the rsyslog config file has the expected filename,
// and the appropriate log dir is used.
func (s *RsyslogSuite) testNamespace(c *gc.C, st *api.State, tag, namespace, expectedFilename, expectedLogDir string) {
	restarted := make(chan struct{}, 2) // once for create, once for teardown
	s.PatchValue(rsyslog.RestartRsyslog, func() error {
		restarted <- struct{}{}
		return nil
	})

	err := os.MkdirAll(expectedLogDir, 0755)
	c.Assert(err, gc.IsNil)
	worker, err := rsyslog.NewRsyslogConfigWorker(st.Rsyslog(), rsyslog.RsyslogModeForwarding, tag, namespace, []string{"0.1.2.3"})
	c.Assert(err, gc.IsNil)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	// change the API HostPorts to trigger an rsyslog restart
	newHostPorts := instance.AddressesWithPort(instance.NewAddresses("127.0.0.1"), 6541)
	err = s.State.SetAPIHostPorts([][]instance.HostPort{newHostPorts})
	c.Assert(err, gc.IsNil)

	// Wait for rsyslog to be restarted, so we can check to see
	// what the name of the config file is.
	waitForRestart(c, restarted)

	// Ensure that ca-cert.pem gets written to the expected log dir.
	waitForFile(c, filepath.Join(expectedLogDir, "ca-cert.pem"))

	dir, err := os.Open(*rsyslog.RsyslogConfDir)
	c.Assert(err, gc.IsNil)
	names, err := dir.Readdirnames(-1)
	dir.Close()
	c.Assert(err, gc.IsNil)
	c.Assert(names, gc.HasLen, 1)
	c.Assert(names[0], gc.Equals, expectedFilename)
}
