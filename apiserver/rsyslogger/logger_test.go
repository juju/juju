// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rsyslogger_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	logger "github.com/juju/juju/apiserver/rsyslogger"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
)

type loggerSuite struct {
	jujutesting.JujuConnSuite

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine
	logger     *logger.RsyslogConfigAPI
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&loggerSuite{})

func (s *loggerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	// Create a machine to work with
	var err error
	s.rawMachine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	// The default auth is as the machine agent
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.rawMachine.Tag(),
	}
	s.logger, err = logger.NewRsyslogConfigAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *loggerSuite) TestNewLoggerAPIRefusesNonAgent(c *gc.C) {
	// We aren't even a machine agent
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = s.AdminUserTag(c)
	endPoint, err := logger.NewRsyslogConfigAPI(s.State, s.resources, anAuthorizer)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *loggerSuite) TestNewLoggerAPIRefusesUnitAgent(c *gc.C) {
	// We aren't even a machine agent
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewUnitTag("germany/7")
	endPoint, err := logger.NewRsyslogConfigAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(endPoint, gc.IsNil)
}

func (s *loggerSuite) TestWatchLoggingConfigNothing(c *gc.C) {
	// Not an error to watch nothing
	results := s.logger.WatchRsyslogConfig(params.Entities{})
	c.Assert(results.Results, gc.HasLen, 0)
}

func (s *loggerSuite) setRSysLoggingConfig(c *gc.C, rsyslogURL, rsyslogCACert, rsyslogClientCert, rsyslogClientKey string) {
	err := s.State.UpdateModelConfig(map[string]interface{}{
		"rsyslog-url":         rsyslogURL,
		"rsyslog-ca-cert":     rsyslogCACert,
		"rsyslog-client-cert": rsyslogClientCert,
		"rsyslog-client-key":  rsyslogClientKey,
	}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	envConfig, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	u, ok := envConfig.RsyslogURL()
	c.Assert(ok, jc.IsTrue)
	c.Assert(u, gc.Equals, rsyslogURL)
	cert, ok := envConfig.RsyslogCACert()
	c.Assert(ok, jc.IsTrue)
	c.Assert(cert, gc.Equals, rsyslogCACert)
	clientCert, ok := envConfig.RsyslogClientCert()
	c.Assert(ok, jc.IsTrue)
	c.Assert(clientCert, gc.Equals, rsyslogClientCert)
	clientKey, ok := envConfig.RsyslogClientKey()
	c.Assert(ok, jc.IsTrue)
	c.Assert(clientKey, gc.Equals, rsyslogClientKey)
}

func (s *loggerSuite) TestWatchLoggingConfig(c *gc.C) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}},
	}
	results := s.logger.WatchRsyslogConfig(args)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].NotifyWatcherId, gc.Not(gc.Equals), "")
	c.Assert(results.Results[0].Error, gc.IsNil)
	resource := s.resources.Get(results.Results[0].NotifyWatcherId)
	c.Assert(resource, gc.NotNil)

	w := resource.(state.NotifyWatcher)
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertNoChange()

	s.setRSysLoggingConfig(c, "https://localhost:1234", coretesting.CACert, coretesting.OtherCACert, coretesting.OtherCAKey)

	wc.AssertOneChange()
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *loggerSuite) TestWatchRsyslogConfigRefusesWrongAgent(c *gc.C) {
	// We are a machine agent, but not the one we are trying to track
	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-12354"}},
	}
	results := s.logger.WatchRsyslogConfig(args)
	// It is not an error to make the request, but the specific item is rejected
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].NotifyWatcherId, gc.Equals, "")
	c.Assert(results.Results[0].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *loggerSuite) TestLoggingConfigForNoone(c *gc.C) {
	// Not an error to request nothing, dumb, but not an error.
	results := s.logger.RsyslogConfig(params.Entities{})
	c.Assert(results.Results, gc.HasLen, 0)
}

func (s *loggerSuite) TestLoggingConfigRefusesWrongAgent(c *gc.C) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-12354"}},
	}
	results := s.logger.RsyslogConfig(args)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *loggerSuite) TestLoggingConfigForAgent(c *gc.C) {
	s.setRSysLoggingConfig(c, "https://localhost:1234", coretesting.CACert, coretesting.OtherCACert, coretesting.OtherCAKey)

	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}},
	}
	results := s.logger.RsyslogConfig(args)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.URL, gc.Equals, "https://localhost:1234")
	c.Assert(result.CACert, gc.Equals, coretesting.CACert)
	c.Assert(result.ClientCert, gc.Equals, coretesting.OtherCACert)
	c.Assert(result.ClientKey, gc.Equals, coretesting.OtherCAKey)
}
