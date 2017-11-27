// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"os"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/worker/caasoperator"
	"github.com/juju/juju/worker/workertest"
)

type WorkerSuite struct {
	testing.IsolationSuite

	clock  *testing.Clock
	config caasoperator.Config
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.clock = testing.NewClock(time.Time{})
	s.config = caasoperator.Config{
		Application: "gitlab",
		DataDir:     c.MkDir(),
		Clock:       s.clock,
	}

	agentBinaryDir := agenttools.ToolsDir(s.config.DataDir, "application-gitlab")
	err := os.MkdirAll(agentBinaryDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *WorkerSuite) TestValidateConfig(c *gc.C) {
	s.testValidateConfig(c, func(config *caasoperator.Config) {
		config.Application = ""
	}, `application name "" not valid`)

	s.testValidateConfig(c, func(config *caasoperator.Config) {
		config.DataDir = ""
	}, `missing DataDir not valid`)

	s.testValidateConfig(c, func(config *caasoperator.Config) {
		config.Clock = nil
	}, `missing Clock not valid`)
}

func (s *WorkerSuite) testValidateConfig(c *gc.C, f func(*caasoperator.Config), expect string) {
	config := s.config
	f(&config)
	w, err := caasoperator.NewWorker(config)
	if err == nil {
		workertest.DirtyKill(c, w)
	}
	c.Check(err, gc.ErrorMatches, expect)
}

func (s *WorkerSuite) TestStartStop(c *gc.C) {
	w, err := caasoperator.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)
}
