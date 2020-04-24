// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationscaler_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/applicationscaler"
)

type WorkerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) TestValidate(c *gc.C) {
	config := applicationscaler.Config{}
	check := func(err error) {
		c.Check(err, gc.ErrorMatches, "nil Facade not valid")
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	}

	err := config.Validate()
	check(err)

	worker, err := applicationscaler.New(config)
	check(err)
	c.Check(worker, gc.IsNil)
}

func (s *WorkerSuite) TestWatchError(c *gc.C) {
	fix := newFixture(c, errors.New("zap ouch"))
	fix.Run(c, func(worker worker.Worker) {
		err := worker.Wait()
		c.Check(err, gc.ErrorMatches, "zap ouch")
	})
	fix.CheckCallNames(c, "Watch")
}

func (s *WorkerSuite) TestRescaleThenError(c *gc.C) {
	fix := newFixture(c, nil, nil, errors.New("pew squish"))
	fix.Run(c, func(worker worker.Worker) {
		err := worker.Wait()
		c.Check(err, gc.ErrorMatches, "pew squish")
	})
	fix.CheckCalls(c, []testing.StubCall{{
		FuncName: "Watch",
	}, {
		FuncName: "Rescale",
		Args:     []interface{}{[]string{"expected", "first"}},
	}, {
		FuncName: "Rescale",
		Args:     []interface{}{[]string{"expected", "second"}},
	}})
}
