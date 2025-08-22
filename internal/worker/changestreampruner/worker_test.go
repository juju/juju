// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package changestreampruner

import (
	"testing"

	clock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type workerSuite struct {
	baseSuite
}

func TestWorkerSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig(c, nil)
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg = s.getConfig(c, nil)
	cfg.Clock = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c, nil)
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c, nil)
	cfg.DBGetter = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c, nil)
	cfg.NewModelPruner = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *workerSuite) TestPrune(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	s.expectTimerImmediate()
	s.expectControllerDBGet()

	ch := make(chan string, 1)

	pruner := s.newPruner(c, ch)
	defer workertest.CleanKill(c, pruner)

	select {
	case <-c.Context().Done():
		c.Fatal("context closed before pruner could start")
	case ns := <-ch:
		c.Check(ns, tc.Equals, coredatabase.ControllerNS)
	}
}

func (s *workerSuite) getConfig(c *tc.C, ch chan string) WorkerConfig {
	return WorkerConfig{
		DBGetter: s.dbGetter,
		NewModelPruner: func(
			_ coredatabase.TxnRunner,
			namespace string,
			_ window,
			_ WindowUpdaterFunc,
			_ clock.Clock,
			_ logger.Logger,
		) worker.Worker {
			ch <- namespace
			return workertest.NewErrorWorker(nil)
		},
		Clock:  s.clock,
		Logger: loggertesting.WrapCheckLog(c),
	}
}

func (s *workerSuite) newPruner(c *tc.C, ch chan string) *Pruner {
	w, err := newWorker(s.getConfig(c, ch))
	c.Assert(err, tc.ErrorIsNil)
	return w
}
