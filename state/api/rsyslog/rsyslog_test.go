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
	cfg, err := s.rsyslog.GetRsyslogConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(cfg, gc.NotNil)
}

func (s *rsyslogSuite) TestWatchForRsyslogChanges(c *gc.C) {
	w, err := s.rsyslog.WatchForRsyslogChanges(s.machine.Tag())
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, w)
}

// SetRsyslogCACert is tested in state/apiserver/rsyslog
