// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rsyslog_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/rsyslog"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
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
	err := s.machine.SetAddresses(network.NewAddress("0.1.2.3", network.ScopeUnknown))
	c.Assert(err, gc.IsNil)

	// Create the rsyslog API facade
	s.rsyslog = s.st.Rsyslog()
	c.Assert(s.rsyslog, gc.NotNil)
}

func (s *rsyslogSuite) TestGetRsyslogConfig(c *gc.C) {
	err := s.APIState.Client().EnvironmentSet(map[string]interface{}{"rsyslog-ca-cert": coretesting.CACert})
	c.Assert(err, gc.IsNil)

	cfg, err := s.rsyslog.GetRsyslogConfig(s.machine.Tag().String())
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
	w, err := s.rsyslog.WatchForRsyslogChanges(s.machine.Tag().String())
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, w)

	wc := statetesting.NewNotifyWatcherC(c, s.BackingState, w)
	// Initial event
	wc.AssertOneChange()

	// change the API HostPorts
	newHostPorts := network.AddressesWithPort(network.NewAddresses("127.0.0.1"), 6541)
	err = s.State.SetAPIHostPorts([][]network.HostPort{newHostPorts})
	c.Assert(err, gc.IsNil)

	// assert we get notified
	wc.AssertOneChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

// SetRsyslogCACert is tested in apiserver/rsyslog
