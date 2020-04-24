// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package retrystrategy_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/retrystrategy"
)

type WorkerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) testValidate(c *gc.C, config retrystrategy.WorkerConfig, errMsg string) {
	check := func(err error) {
		c.Check(err, gc.ErrorMatches, errMsg)
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	}

	err := config.Validate()
	check(err)

	worker, err := retrystrategy.NewRetryStrategyWorker(config)
	check(err)
	c.Check(worker, gc.IsNil)
}

func (s WorkerSuite) TestValidateInvalidFacade(c *gc.C) {
	s.testValidate(c, retrystrategy.WorkerConfig{}, "nil Facade not valid")
}

func (s WorkerSuite) TestValidateInvalidAgentTag(c *gc.C) {
	s.testValidate(c, retrystrategy.WorkerConfig{
		Facade: &stubFacade{},
	}, "nil AgentTag not valid")
}

func (s WorkerSuite) TestValidateInvalidRetryStrategy(c *gc.C) {
	s.testValidate(c, retrystrategy.WorkerConfig{
		Facade:   &stubFacade{},
		AgentTag: &stubTag{},
	}, "empty RetryStrategy not valid")
}

func (s WorkerSuite) TestWatchError(c *gc.C) {
	fix := newFixture(c, errors.New("supersonybunduru"))
	fix.Run(c, func(w worker.Worker) {
		err := w.Wait()
		c.Assert(err, gc.ErrorMatches, "supersonybunduru")
	})
	fix.CheckCallNames(c, "WatchRetryStrategy")
}

func (s WorkerSuite) TestGetStrategyError(c *gc.C) {
	fix := newFixture(c, nil, errors.New("blackfridaybunduru"))
	fix.Run(c, func(w worker.Worker) {
		err := w.Wait()
		c.Assert(err, gc.ErrorMatches, "blackfridaybunduru")
	})
	fix.CheckCallNames(c, "WatchRetryStrategy", "RetryStrategy")
}

func (s WorkerSuite) TestBounce(c *gc.C) {
	fix := newFixture(c, nil, nil, nil)
	fix.Run(c, func(w worker.Worker) {
		err := w.Wait()
		c.Assert(err, gc.ErrorMatches, "restart immediately")
	})
	fix.CheckCallNames(c, "WatchRetryStrategy", "RetryStrategy", "RetryStrategy")
}
