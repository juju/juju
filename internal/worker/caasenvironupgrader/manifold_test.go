// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasenvironupgrader_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/caasenvironupgrader"
	"github.com/juju/juju/internal/worker/gate"
)

type ManifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ManifoldSuite{})

func (*ManifoldSuite) TestInputs(c *gc.C) {
	manifold := caasenvironupgrader.Manifold(caasenvironupgrader.ManifoldConfig{
		GateName: "gate",
	})
	c.Check(manifold.Inputs, jc.DeepEquals, []string{"gate"})
}

func (*ManifoldSuite) TestMissingGateName(c *gc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"gate": dependency.ErrMissing,
	})
	manifold := caasenvironupgrader.Manifold(caasenvironupgrader.ManifoldConfig{
		GateName: "gate",
	})

	worker, err := manifold.Start(context.Background(), getter)
	c.Check(worker, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (*ManifoldSuite) TestNewWorkerError(c *gc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"gate": struct{ gate.Unlocker }{},
	})
	manifold := caasenvironupgrader.Manifold(caasenvironupgrader.ManifoldConfig{
		GateName: "gate",
		NewWorker: func(config caasenvironupgrader.Config) (worker.Worker, error) {
			return nil, errors.New("error")
		},
	})

	worker, err := manifold.Start(context.Background(), getter)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "error")
}
