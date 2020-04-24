// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package txnpruner_test

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/txnpruner"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	stub   testing.Stub
	config txnpruner.ManifoldConfig
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

func (s *ManifoldSuite) validConfig() txnpruner.ManifoldConfig {
	return txnpruner.ManifoldConfig{
		ClockName:     "clock",
		StateName:     "state",
		PruneInterval: time.Hour,
		NewWorker: func(tp txnpruner.TransactionPruner, interval time.Duration, clock clock.Clock) worker.Worker {
			s.stub.AddCall("NewWorker", tp, interval, clock)
			return s.worker
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
