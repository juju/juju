// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dblogpruner_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/worker/dblogpruner"
	"github.com/juju/juju/worker/workertest"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	stub   testing.Stub
	config dblogpruner.ManifoldConfig
	worker worker.Worker
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.stub.ResetCalls()
	s.config = s.validConfig()
	s.worker = worker.NewRunner(worker.RunnerParams{})
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.worker) })
}

func (s *ManifoldSuite) validConfig() dblogpruner.ManifoldConfig {
	return dblogpruner.ManifoldConfig{
		ClockName:     "clock",
		StateName:     "state",
		PruneInterval: time.Hour,
		NewWorker: func(config dblogpruner.Config) (worker.Worker, error) {
			s.stub.AddCall("NewWorker", config)
			return s.worker, s.stub.NextErr()
		},
	}
}

func (s *ManifoldSuite) TestValid(c *gc.C) {
	c.Check(s.config.Validate(), jc.ErrorIsNil)
}

func (s *ManifoldSuite) TestMissingClockName(c *gc.C) {
	s.config.ClockName = ""
	s.checkNotValid(c, "empty ClockName not valid")
}

func (s *ManifoldSuite) TestMissingStateName(c *gc.C) {
	s.config.StateName = ""
	s.checkNotValid(c, "empty StateName not valid")
}

func (s *ManifoldSuite) TestZeroPruneInterval(c *gc.C) {
	s.config.PruneInterval = 0
	s.checkNotValid(c, "non-positive PruneInterval not valid")
}

func (s *ManifoldSuite) TestMissingNewWorker(c *gc.C) {
	s.config.NewWorker = nil
	s.checkNotValid(c, "nil NewWorker not valid")
}

func (s *ManifoldSuite) checkNotValid(c *gc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, gc.ErrorMatches, expect)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}
