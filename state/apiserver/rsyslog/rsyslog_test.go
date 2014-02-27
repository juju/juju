// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rsyslog_test

import (
	"encoding/pem"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	apirsyslog "launchpad.net/juju-core/state/api/rsyslog"
	"launchpad.net/juju-core/state/apiserver/common"
	commontesting "launchpad.net/juju-core/state/apiserver/common/testing"
	"launchpad.net/juju-core/state/apiserver/rsyslog"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
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

func verifyRsyslogCACert(c *gc.C, st *apirsyslog.State, expected []byte) {
	cfg, err := st.EnvironConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(cfg.RsyslogCACert(), gc.DeepEquals, expected)
}

func (s *rsyslogSuite) TestSetRsyslogCert(c *gc.C) {
	st, _ := s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	err := st.Rsyslog().SetRsyslogCert([]byte(coretesting.CACert))
	c.Assert(err, gc.IsNil)
	verifyRsyslogCACert(c, st.Rsyslog(), []byte(coretesting.CACert))
}

func (s *rsyslogSuite) TestSetRsyslogCertNil(c *gc.C) {
	st, _ := s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	err := st.Rsyslog().SetRsyslogCert(nil)
	c.Assert(err, gc.ErrorMatches, "no certificates found")
	verifyRsyslogCACert(c, st.Rsyslog(), nil)
}

func (s *rsyslogSuite) TestSetRsyslogCertInvalid(c *gc.C) {
	st, _ := s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	err := st.Rsyslog().SetRsyslogCert(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: []byte("not a valid certificate"),
	}))
	c.Assert(err, gc.ErrorMatches, ".*structure error.*")
	verifyRsyslogCACert(c, st.Rsyslog(), nil)
}

func (s *rsyslogSuite) TestSetRsyslogCertPerms(c *gc.C) {
	st, _ := s.OpenAPIAsNewMachine(c, state.JobHostUnits)
	err := st.Rsyslog().SetRsyslogCert([]byte(coretesting.CACert))
	c.Assert(err, gc.ErrorMatches, "invalid entity name or password")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
	// Verify no change was effected.
	verifyRsyslogCACert(c, st.Rsyslog(), nil)
}
