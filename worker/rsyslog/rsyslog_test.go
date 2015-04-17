// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
// +build !windows

package rsyslog_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cert"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/utils/syslog"
	"github.com/juju/juju/worker/rsyslog"
)

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

func assertPathExists(c *gc.C, path string) {
	_, err := os.Stat(path)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RsyslogSuite) TestStartStop(c *gc.C) {
	st, m := s.OpenAPIAsNewMachine(c, state.JobHostUnits)
	worker, err := rsyslog.NewRsyslogConfigWorker(st.Rsyslog(), rsyslog.RsyslogModeForwarding, m.Tag(), "", []string{"0.1.2.3"})
	c.Assert(err, jc.ErrorIsNil)
	worker.Kill()
	c.Assert(worker.Wait(), gc.IsNil)
}

func (s *RsyslogSuite) TestTearDown(c *gc.C) {
	st, m := s.st, s.machine
	worker, err := rsyslog.NewRsyslogConfigWorker(st.Rsyslog(), rsyslog.RsyslogModeAccumulate, m.Tag(), "", []string{"0.1.2.3"})
	c.Assert(err, jc.ErrorIsNil)
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

func (s *RsyslogSuite) TestRsyslogCert(c *gc.C) {
	st, m := s.st, s.machine
	err := s.machine.SetAddresses(network.NewAddress("example.com"))
	c.Assert(err, jc.ErrorIsNil)

	worker, err := rsyslog.NewRsyslogConfigWorker(st.Rsyslog(), rsyslog.RsyslogModeAccumulate, m.Tag(), "", []string{"0.1.2.3"})
	c.Assert(err, jc.ErrorIsNil)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()
	waitForFile(c, filepath.Join(*rsyslog.LogDir, "rsyslog-cert.pem"))

	rsyslogCertPEM, err := ioutil.ReadFile(filepath.Join(*rsyslog.LogDir, "rsyslog-cert.pem"))
	c.Assert(err, jc.ErrorIsNil)

	cert, err := cert.ParseCert(string(rsyslogCertPEM))
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(cert.DNSNames, gc.DeepEquals, []string{"example.com", "*"})

	subject := cert.Subject
	c.Assert(subject.CommonName, gc.Equals, "*")
	c.Assert(subject.Organization, gc.DeepEquals, []string{"juju"})

	issuer := cert.Issuer
	c.Assert(issuer.CommonName, gc.Equals, "juju-generated CA for environment \"rsyslog\"")
	c.Assert(issuer.Organization, gc.DeepEquals, []string{"juju"})
}

func (s *RsyslogSuite) TestModeAccumulate(c *gc.C) {
	st, m := s.st, s.machine
	worker, err := rsyslog.NewRsyslogConfigWorker(st.Rsyslog(), rsyslog.RsyslogModeAccumulate, m.Tag(), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()
	waitForFile(c, filepath.Join(*rsyslog.LogDir, "ca-cert.pem"))

	// We should have ca-cert.pem, rsyslog-cert.pem, and rsyslog-key.pem.
	caCertPEM, err := ioutil.ReadFile(filepath.Join(*rsyslog.LogDir, "ca-cert.pem"))
	c.Assert(err, jc.ErrorIsNil)
	rsyslogCertPEM, err := ioutil.ReadFile(filepath.Join(*rsyslog.LogDir, "rsyslog-cert.pem"))
	c.Assert(err, jc.ErrorIsNil)
	rsyslogKeyPEM, err := ioutil.ReadFile(filepath.Join(*rsyslog.LogDir, "rsyslog-key.pem"))
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = cert.ParseCertAndKey(string(rsyslogCertPEM), string(rsyslogKeyPEM))
	c.Assert(err, jc.ErrorIsNil)
	err = cert.Verify(string(rsyslogCertPEM), string(caCertPEM), time.Now().UTC())
	c.Assert(err, jc.ErrorIsNil)

	// Verify rsyslog configuration.
	waitForFile(c, filepath.Join(*rsyslog.RsyslogConfDir, "25-juju.conf"))
	rsyslogConf, err := ioutil.ReadFile(filepath.Join(*rsyslog.RsyslogConfDir, "25-juju.conf"))
	c.Assert(err, jc.ErrorIsNil)

	syslogPort := s.Environ.Config().SyslogPort()
	syslogConfig := syslog.NewAccumulateConfig(m.Tag().String(), *rsyslog.LogDir, syslogPort, "", []string{})
	syslogConfig.ConfigDir = *rsyslog.RsyslogConfDir
	rendered, err := syslogConfig.Render()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(string(rsyslogConf), gc.DeepEquals, string(rendered))

	// Verify logrotate files
	assertPathExists(c, filepath.Join(*rsyslog.LogDir, "logrotate.conf"))
	assertPathExists(c, filepath.Join(*rsyslog.LogDir, "logrotate.run"))

}

func (s *RsyslogSuite) TestAccumulateHA(c *gc.C) {
	m := s.machine
	syslogConfig := syslog.NewAccumulateConfig(m.Tag().String(), *rsyslog.LogDir, 6541, "", []string{"192.168.1", "127.0.0.1"})
	rendered, err := syslogConfig.Render()
	c.Assert(err, jc.ErrorIsNil)

	stateServer1Config := ":syslogtag, startswith, \"juju-\" @@192.168.1:6541;LongTagForwardFormat"
	stateServer2Config := ":syslogtag, startswith, \"juju-\" @@127.0.0.1:6541;LongTagForwardFormat"

	c.Assert(strings.Contains(string(rendered), stateServer1Config), jc.IsTrue)
	c.Assert(strings.Contains(string(rendered), stateServer2Config), jc.IsTrue)
}

func (s *RsyslogSuite) TestNamespace(c *gc.C) {
	st := s.st
	// set the rsyslog cert
	err := s.APIState.Client().EnvironmentSet(map[string]interface{}{"rsyslog-ca-cert": coretesting.CACert})
	c.Assert(err, jc.ErrorIsNil)

	// namespace only takes effect in filenames
	// for machine-0; all others assume isolation.
	s.testNamespace(c, st, names.NewMachineTag("0"), "", "25-juju.conf", *rsyslog.LogDir)
	s.testNamespace(c, st, names.NewMachineTag("0"), "mynamespace", "25-juju-mynamespace.conf", *rsyslog.LogDir+"-mynamespace")
	s.testNamespace(c, st, names.NewMachineTag("1"), "", "25-juju.conf", *rsyslog.LogDir)
	s.testNamespace(c, st, names.NewMachineTag("1"), "mynamespace", "25-juju.conf", *rsyslog.LogDir)
	s.testNamespace(c, st, names.NewUnitTag("myservice/0"), "", "26-juju-unit-myservice-0.conf", *rsyslog.LogDir)
	s.testNamespace(c, st, names.NewUnitTag("myservice/0"), "mynamespace", "26-juju-unit-myservice-0.conf", *rsyslog.LogDir)
}

// testNamespace starts a worker and ensures that
// the rsyslog config file has the expected filename,
// and the appropriate log dir is used.
func (s *RsyslogSuite) testNamespace(c *gc.C, st *api.State, tag names.Tag, namespace, expectedFilename, expectedLogDir string) {
	restarted := make(chan struct{}, 2) // once for create, once for teardown
	s.PatchValue(rsyslog.RestartRsyslog, func() error {
		restarted <- struct{}{}
		return nil
	})

	err := os.MkdirAll(expectedLogDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	worker, err := rsyslog.NewRsyslogConfigWorker(st.Rsyslog(), rsyslog.RsyslogModeAccumulate, tag, namespace, []string{"0.1.2.3"})
	c.Assert(err, jc.ErrorIsNil)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	// change the API HostPorts to trigger an rsyslog restart
	newHostPorts := network.NewHostPorts(6541, "127.0.0.1")
	err = s.State.SetAPIHostPorts([][]network.HostPort{newHostPorts})
	c.Assert(err, jc.ErrorIsNil)

	// Wait for rsyslog to be restarted, so we can check to see
	// what the name of the config file is.
	waitForRestart(c, restarted)

	// Ensure that ca-cert.pem gets written to the expected log dir.
	waitForFile(c, filepath.Join(expectedLogDir, "ca-cert.pem"))

	dir, err := os.Open(*rsyslog.RsyslogConfDir)
	c.Assert(err, jc.ErrorIsNil)
	names, err := dir.Readdirnames(-1)
	dir.Close()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(names, gc.HasLen, 1)
	c.Assert(names[0], gc.Equals, expectedFilename)
}
