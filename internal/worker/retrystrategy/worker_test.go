// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package retrystrategy_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/retrystrategy"
)

type WorkerSuite struct {
	testhelpers.IsolationSuite
}

func TestWorkerSuite(t *stdtesting.T) { tc.Run(t, &WorkerSuite{}) }
func (s *WorkerSuite) testValidate(c *tc.C, config retrystrategy.WorkerConfig, errMsg string) {
	check := func(err error) {
		c.Check(err, tc.ErrorMatches, errMsg)
		c.Check(err, tc.ErrorIs, errors.NotValid)
	}

	err := config.Validate()
	check(err)

	worker, err := retrystrategy.NewRetryStrategyWorker(config)
	check(err)
	c.Check(worker, tc.IsNil)
}

func (s *WorkerSuite) TestValidateInvalidFacade(c *tc.C) {
	s.testValidate(c, retrystrategy.WorkerConfig{}, "nil Facade not valid")
}

func (s *WorkerSuite) TestValidateInvalidAgentTag(c *tc.C) {
	s.testValidate(c, retrystrategy.WorkerConfig{
		Facade: &stubFacade{},
	}, "nil AgentTag not valid")
}

func (s *WorkerSuite) TestValidateInvalidRetryStrategy(c *tc.C) {
	s.testValidate(c, retrystrategy.WorkerConfig{
		Facade:   &stubFacade{},
		AgentTag: &stubTag{},
	}, "empty RetryStrategy not valid")
}

func (s *WorkerSuite) TestWatchError(c *tc.C) {
	fix := newFixture(c, errors.New("supersonybunduru"))
	fix.Run(c, func(w worker.Worker) {
		err := w.Wait()
		c.Assert(err, tc.ErrorMatches, "supersonybunduru")
	})
	fix.CheckCallNames(c, "WatchRetryStrategy")
}

func (s *WorkerSuite) TestGetStrategyError(c *tc.C) {
	fix := newFixture(c, nil, errors.New("blackfridaybunduru"))
	fix.Run(c, func(w worker.Worker) {
		err := w.Wait()
		c.Assert(err, tc.ErrorMatches, "blackfridaybunduru")
	})
	fix.CheckCallNames(c, "WatchRetryStrategy", "RetryStrategy")
}

func (s *WorkerSuite) TestBounce(c *tc.C) {
	fix := newFixture(c, nil, nil, nil)
	fix.Run(c, func(w worker.Worker) {
		err := w.Wait()
		c.Assert(err, tc.ErrorMatches, "restart immediately")
	})
	fix.CheckCallNames(c, "WatchRetryStrategy", "RetryStrategy", "RetryStrategy")
}
