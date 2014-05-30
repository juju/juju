// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rsyslog_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/rsyslog"
	statetesting "launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
)

type rsyslogSuite struct {
	testing.JujuConnSuite

	st      *api.State
	machine *state.Machine
	rsyslog *rsyslog.State
}

var _ = gc.Suite(&rsyslogSuite{})

func (s *rsyslogSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.st, s.machine = s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	err := s.machine.SetAddresses(instance.NewAddress("0.1.2.3", instance.NetworkUnknown))
	c.Assert(err, gc.IsNil)

	// Create the rsyslog API facade
	s.rsyslog = s.st.Rsyslog()
	c.Assert(s.rsyslog, gc.NotNil)
}

func (s *rsyslogSuite) TestGetRsyslogConfig(c *gc.C) {
	err := s.APIState.Client().EnvironmentSet(map[string]interface{}{"rsyslog-ca-cert": coretesting.CACert})
	c.Assert(err, gc.IsNil)

	cfg, err := s.rsyslog.GetRsyslogConfig(s.machine.Tag())
	c.Assert(err, gc.IsNil)
	c.Assert(cfg, gc.NotNil)

	c.Assert(cfg.CACert, gc.Equals, coretesting.CACert)
	c.Assert(cfg.HostPorts, gc.HasLen, 1)
	hostPort := cfg.HostPorts[0]
	c.Assert(hostPort.Address.Value, gc.Equals, "0.1.2.3")

	// the rsyslog port is set by the provider/dummy/environs.go
	c.Assert(hostPort.Port, gc.Equals, 2345)
}

func (s *rsyslogSuite) TestWatchForRsyslogChanges(c *gc.C) {
	w, err := s.rsyslog.WatchForRsyslogChanges(s.machine.Tag())
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, w)

	wc := statetesting.NewNotifyWatcherC(c, s.BackingState, w)
	// Initial event
	wc.AssertOneChange()

	// change the API HostPorts
	newHostPorts := instance.AddressesWithPort(instance.NewAddresses("127.0.0.1"), 6541)
	err = s.State.SetAPIHostPorts([][]instance.HostPort{newHostPorts})
	c.Assert(err, gc.IsNil)

	// assert we get notified
	wc.AssertOneChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

// SetRsyslogCACert is tested in state/apiserver/rsyslog
