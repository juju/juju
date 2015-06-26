// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rsyslog_test

import (
	"encoding/pem"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apirsyslog "github.com/juju/juju/api/rsyslog"
	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/rsyslog"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type rsyslogSuite struct {
	testing.JujuConnSuite
	*commontesting.EnvironWatcherTest
	authorizer apiservertesting.FakeAuthorizer
	resources  *common.Resources
	rsyslog    *rsyslog.RsyslogAPI
}

var _ = gc.Suite(&rsyslogSuite{})

func (s *rsyslogSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:            names.NewMachineTag("1"),
		EnvironManager: false,
	}
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })
	api, err := rsyslog.NewRsyslogAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	s.EnvironWatcherTest = commontesting.NewEnvironWatcherTest(
		api, s.State, s.resources, commontesting.NoSecrets)
}

func verifyRsyslogCACert(c *gc.C, st *apirsyslog.State, expectedCA, expectedKey string) {
	cfg, err := st.GetRsyslogConfig("foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.CACert, gc.DeepEquals, expectedCA)
	c.Assert(cfg.CAKey, gc.DeepEquals, expectedKey)
}

func (s *rsyslogSuite) TestSetRsyslogCert(c *gc.C) {
	st, m := s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	err := m.SetProviderAddresses(network.NewAddress("0.1.2.3"))
	c.Assert(err, jc.ErrorIsNil)

	err = st.Rsyslog().SetRsyslogCert(coretesting.CACert, coretesting.CAKey)
	c.Assert(err, jc.ErrorIsNil)
	verifyRsyslogCACert(c, st.Rsyslog(), coretesting.CACert, coretesting.CAKey)
}

func (s *rsyslogSuite) TestSetRsyslogCertNil(c *gc.C) {
	st, m := s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	err := m.SetProviderAddresses(network.NewAddress("0.1.2.3"))
	c.Assert(err, jc.ErrorIsNil)

	err = st.Rsyslog().SetRsyslogCert("", "")
	c.Assert(err, gc.ErrorMatches, "no certificates found")
	verifyRsyslogCACert(c, st.Rsyslog(), "", "")
}

func (s *rsyslogSuite) TestSetRsyslogCertInvalid(c *gc.C) {
	st, m := s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	err := m.SetProviderAddresses(network.NewAddress("0.1.2.3"))
	c.Assert(err, jc.ErrorIsNil)

	err = st.Rsyslog().SetRsyslogCert(string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: []byte("not a valid certificate"),
	})), "")
	c.Assert(err, gc.ErrorMatches, ".*structure error.*")
	verifyRsyslogCACert(c, st.Rsyslog(), "", "")
}

func (s *rsyslogSuite) TestSetRsyslogCertPerms(c *gc.C) {
	// create a machine-0 so we have an addresss to log to
	m, err := s.State.AddMachine("trusty", state.JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetProviderAddresses(network.NewAddress("0.1.2.3"))
	c.Assert(err, jc.ErrorIsNil)

	unitState, _ := s.OpenAPIAsNewMachine(c, state.JobHostUnits)
	err = unitState.Rsyslog().SetRsyslogCert(coretesting.CACert, coretesting.CAKey)
	c.Assert(err, gc.ErrorMatches, "invalid entity name or password")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
	// Verify no change was effected.
	verifyRsyslogCACert(c, unitState.Rsyslog(), "", "")
}

func (s *rsyslogSuite) TestUpgraderAPIAllowsUnitAgent(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewUnitTag("seven/9")
	anUpgrader, err := rsyslog.NewRsyslogAPI(s.State, s.resources, anAuthorizer)
	c.Check(err, jc.ErrorIsNil)
	c.Check(anUpgrader, gc.NotNil)
}

func (s *rsyslogSuite) TestUpgraderAPIRefusesNonUnitNonMachineAgent(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewServiceTag("hadoop")
	anUpgrader, err := rsyslog.NewRsyslogAPI(s.State, s.resources, anAuthorizer)
	c.Check(err, gc.NotNil)
	c.Check(anUpgrader, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}
