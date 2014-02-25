// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rsyslog_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state/apiserver/common"
	commontesting "launchpad.net/juju-core/state/apiserver/common/testing"
	"launchpad.net/juju-core/state/apiserver/rsyslog"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
)

type rsyslogSuite struct {
	testing.JujuConnSuite
	*commontesting.EnvironWatcherTest
	authorizer apiservertesting.FakeAuthorizer
	resources  *common.Resources
}

var _ = gc.Suite(&rsyslogSuite{})

func (s *rsyslogSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.authorizer = apiservertesting.FakeAuthorizer{
		LoggedIn:       true,
		EnvironManager: true,
	}
	s.resources = common.NewResources()
	api, err := rsyslog.NewRsyslogAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)
	s.EnvironWatcherTest = commontesting.NewEnvironWatcherTest(
		api, s.State, s.resources, commontesting.NoSecrets)
}

func (s *rsyslogSuite) TestSetRsyslogCert(c *gc.C) {
	st := s.APIState.Rsyslog()
	err := st.SetRsyslogCert(nil)
	// TODO(axw) finish me. this should fail due to being an invalid cert
	c.Assert(err, gc.IsNil)
}

func (s *rsyslogSuite) TestSetRsyslogCertPerms(c *gc.C) {
	// TODO(axw) SetRsyslogCert requires that the
	// caller is a state server.
}
