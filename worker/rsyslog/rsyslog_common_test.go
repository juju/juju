// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rsyslog_test

import (
	"crypto/tls"
	"io/ioutil"
	"os"
	"path/filepath"
	stdtesting "testing"
	"time"

	"github.com/juju/syslog"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/rsyslog"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type RsyslogSuite struct {
	jujutesting.JujuConnSuite

	st       *api.State
	machine  *state.Machine
	dialTags []string
}

var _ = gc.Suite(&RsyslogSuite{})

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
	s.dialTags = nil
	s.PatchValue(rsyslog.DialSyslog, func(network, raddr string, priority syslog.Priority, tag string, tlsCfg *tls.Config) (*syslog.Writer, error) {
		s.dialTags = append(s.dialTags, tag)
		return &syslog.Writer{}, nil
	})
	s.PatchValue(rsyslog.LogDir, c.MkDir())
	s.PatchValue(rsyslog.RsyslogConfDir, c.MkDir())

	s.st, s.machine = s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	err := s.machine.SetAddresses(network.NewAddress("0.1.2.3"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RsyslogSuite) TestModeForwarding(c *gc.C) {
	err := s.APIState.Client().EnvironmentSet(map[string]interface{}{
		"rsyslog-ca-cert": coretesting.CACert,
		"rsyslog-ca-key":  coretesting.CAKey,
	})
	c.Assert(err, jc.ErrorIsNil)
	st, m := s.OpenAPIAsNewMachine(c, state.JobHostUnits)
	addrs := []string{"0.1.2.3", "0.2.4.6"}
	worker, err := rsyslog.NewRsyslogConfigWorker(st.Rsyslog(), rsyslog.RsyslogModeForwarding, m.Tag(), "foo", addrs)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	// We should get a ca-cert.pem with the contents introduced into state config.
	waitForFile(c, filepath.Join(*rsyslog.LogDir, "ca-cert.pem"))
	caCertPEM, err := ioutil.ReadFile(filepath.Join(*rsyslog.LogDir, "ca-cert.pem"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(caCertPEM), gc.DeepEquals, coretesting.CACert)

	c.Assert(*rsyslog.SyslogTargets, gc.HasLen, 2)
	for _, dialTag := range s.dialTags {
		c.Assert(dialTag, gc.Equals, "juju-foo-"+m.Tag().String())
	}
}
