// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/logger"
	"github.com/juju/juju/core/watcher/watchertest"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

type LoggerIntegrationSuite struct {
	jujutesting.ApiServerSuite
}

var _ = gc.Suite(&LoggerIntegrationSuite{})

func (s *LoggerIntegrationSuite) TestLoggingConfig(c *gc.C) {
	root, machine := s.OpenAPIAsNewMachine(c, state.JobHostUnits)
	logging := logger.NewClient(root)

	obtained, err := logging.LoggingConfig(machine.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.Equals, "<root>=INFO")
}

func (s *LoggerIntegrationSuite) TestWatchLoggingConfig(c *gc.C) {
	root, machine := s.OpenAPIAsNewMachine(c, state.JobHostUnits)
	logging := logger.NewClient(root)

	watcher, err := logging.WatchLoggingConfig(machine.Tag())
	c.Assert(err, jc.ErrorIsNil)
	_ = watcher

	wc := watchertest.NewNotifyWatcherC(c, watcher)
	// Initial event.
	wc.AssertOneChange()

	model := s.ControllerModel(c)
	err = model.UpdateModelConfig(
		map[string]interface{}{
			"logging-config": "juju=INFO;test=TRACE",
		}, nil)
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertOneChange()
	wc.AssertStops()
}
