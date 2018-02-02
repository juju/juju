// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"net"

	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	servertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/auditlog"
)

type auditConfigSuite struct {
	apiserverBaseSuite
}

var _ = gc.Suite(&auditConfigSuite{})

func (s *auditConfigSuite) TestUpdateConfig(c *gc.C) {
	config := s.sampleConfig(c)
	auditConfigChanged := make(chan auditlog.Config)
	config.AuditConfigChanged = auditConfigChanged
	config.AuditConfig = auditlog.Config{
		ExcludeMethods: set.NewStrings("Midlake.Bandits"),
	}

	srv := s.newServer(c, config)

	auditConfig := srv.GetAuditConfig()
	c.Assert(auditConfig, gc.DeepEquals, auditlog.Config{
		ExcludeMethods: set.NewStrings("Midlake.Bandits"),
	})

	fakeLog := &servertesting.FakeAuditLog{}
	newConfig := auditlog.Config{
		Enabled:        true,
		ExcludeMethods: set.NewStrings("ModelManager.ListModels"),
		Target:         fakeLog,
	}

	// Sending the config in twice is a simple way to ensure the
	// config has been picked up and applied wihout explicitly
	// waiting.
	auditConfigChanged <- newConfig
	auditConfigChanged <- newConfig

	auditConfig = srv.GetAuditConfig()
	// Check that target's right.
	err := auditConfig.Target.AddResponse(auditlog.ResponseErrors{
		ConversationID: "heynow",
	})
	c.Assert(err, jc.ErrorIsNil)
	fakeLog.CheckCallNames(c, "AddResponse")
	fakeLog.CheckCall(c, 0, "AddResponse", auditlog.ResponseErrors{
		ConversationID: "heynow",
	})

	auditConfig.Target = nil
	c.Assert(auditConfig, gc.DeepEquals, auditlog.Config{
		Enabled:        true,
		ExcludeMethods: set.NewStrings("ModelManager.ListModels"),
	})
}

func (s *auditConfigSuite) TestNewServerValidatesConfig(c *gc.C) {
	config := s.sampleConfig(c)
	config.AuditConfig = auditlog.Config{
		Enabled: true,
	}

	listener, err := net.Listen("tcp", ":0")
	c.Assert(err, jc.ErrorIsNil)
	defer listener.Close()

	srv, err := apiserver.NewServer(s.StatePool, listener, config)
	c.Assert(err, gc.ErrorMatches, "validating audit log configuration: logging enabled but no target provided")
	c.Assert(srv, gc.IsNil)
}

func (s *auditConfigSuite) TestInvalidConfigLogsAndDiscards(c *gc.C) {
	config := s.sampleConfig(c)
	auditConfigChanged := make(chan auditlog.Config)
	config.AuditConfigChanged = auditConfigChanged

	srv := s.newServer(c, config)
	newConfig := auditlog.Config{
		Enabled:        true,
		ExcludeMethods: set.NewStrings("ModelManager.ListModels"),
	}
	var logWriter loggo.TestWriter
	c.Assert(loggo.RegisterWriter("auditconfig-tests", &logWriter), jc.ErrorIsNil)
	defer func() {
		logWriter.Clear()
	}()

	auditConfigChanged <- newConfig
	auditConfigChanged <- newConfig
	loggo.RemoveWriter("auditconfig-tests")

	// Update to invalid config is discarded.
	c.Assert(srv.GetAuditConfig().Enabled, gc.Equals, false)

	messages := logWriter.Log()
	c.Assert(messages[len(messages)-1:], jc.LogMatches, []jc.SimpleMessage{{
		loggo.WARNING, "discarding invalid audit config: logging enabled but no target provided",
	}})
}
