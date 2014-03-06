// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rsyslog_test

import (
	gc "launchpad.net/gocheck"

	jujutesting "launchpad.net/juju-core/juju/testing"
	commontesting "launchpad.net/juju-core/state/api/common/testing"
)

type rsyslogSuite struct {
	jujutesting.JujuConnSuite
	*commontesting.EnvironWatcherTest
}

var _ = gc.Suite(&rsyslogSuite{})

func (s *rsyslogSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	stateAPI, _ := s.OpenAPIAsNewMachine(c)
	rsyslogAPI := stateAPI.Rsyslog()
	c.Assert(rsyslogAPI, gc.NotNil)

	s.EnvironWatcherTest = commontesting.NewEnvironWatcherTest(
		rsyslogAPI,
		s.State,
		s.BackingState,
		commontesting.NoSecrets,
	)
}

// SetRsyslogCACert is tested in state/apiserver/rsyslog
