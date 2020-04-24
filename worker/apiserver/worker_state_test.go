// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"github.com/juju/collections/set"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	coreapiserver "github.com/juju/juju/apiserver"
	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/auditlog"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/apiserver"
)

type WorkerStateSuite struct {
	workerFixture
	statetesting.StateSuite
}

var _ = gc.Suite(&WorkerStateSuite{})

func (s *WorkerStateSuite) SetUpSuite(c *gc.C) {
	s.workerFixture.SetUpSuite(c)

	err := testing.MgoServer.Start(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.workerFixture.AddCleanup(func(*gc.C) { testing.MgoServer.Destroy() })

	s.StateSuite.SetUpSuite(c)
}

func (s *WorkerStateSuite) TearDownSuite(c *gc.C) {
	s.StateSuite.TearDownSuite(c)
	s.workerFixture.TearDownSuite(c)
}

func (s *WorkerStateSuite) SetUpTest(c *gc.C) {
	s.workerFixture.SetUpTest(c)
	s.StateSuite.SetUpTest(c)
	s.config.StatePool = s.StatePool
	s.config.GetAuditConfig = func() auditlog.Config {
		return auditlog.Config{
			Enabled:        true,
			CaptureAPIArgs: true,
			MaxSizeMB:      200,
			MaxBackups:     5,
			ExcludeMethods: set.NewStrings("Exclude.This"),
			Target:         &apitesting.FakeAuditLog{},
		}
	}
}

func (s *WorkerStateSuite) TearDownTest(c *gc.C) {
	s.StateSuite.TearDownTest(c)
	s.workerFixture.TearDownTest(c)
}

func (s *WorkerStateSuite) TestStart(c *gc.C) {
	w, err := apiserver.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	// The server is started some time after the worker
	// starts, not necessarily as soon as NewWorker returns.
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(s.stub.Calls()) == 0 {
			continue
		}
		break
	}
	if !s.stub.CheckCallNames(c, "NewServer") {
		return
	}
	args := s.stub.Calls()[0].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], gc.FitsTypeOf, coreapiserver.ServerConfig{})
	config := args[0].(coreapiserver.ServerConfig)

	c.Assert(config.RegisterIntrospectionHandlers, gc.NotNil)
	config.RegisterIntrospectionHandlers = nil

	c.Assert(config.UpgradeComplete, gc.NotNil)
	config.UpgradeComplete = nil

	c.Assert(config.RestoreStatus, gc.NotNil)
	config.RestoreStatus = nil

	c.Assert(config.NewObserver, gc.NotNil)
	config.NewObserver = nil

	c.Assert(config.GetAuditConfig, gc.NotNil)
	// Set the audit config getter to Nil because we don't want to
	// compare it.
	config.GetAuditConfig = nil

	c.Assert(config.Presence, gc.NotNil)
	config.Presence = nil

	logSinkConfig := coreapiserver.DefaultLogSinkConfig()

	c.Assert(config, jc.DeepEquals, coreapiserver.ServerConfig{
		StatePool:           s.StatePool,
		Authenticator:       s.authenticator,
		Mux:                 s.mux,
		Clock:               s.clock,
		Controller:          s.controller,
		MultiwatcherFactory: s.multiwatcherFactory,
		Tag:                 s.agentConfig.Tag(),
		DataDir:             s.agentConfig.DataDir(),
		LogDir:              s.agentConfig.LogDir(),
		Hub:                 &s.hub,
		PublicDNSName:       "",
		AllowModelAccess:    false,
		LogSinkConfig:       &logSinkConfig,
		LeaseManager:        s.leaseManager,
		MetricsCollector:    s.metricsCollector,
	})
}
