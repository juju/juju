// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/storageprovisioner"
)

type ManifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) TestManifold(c *gc.C) {
	manifold := storageprovisioner.ModelManifold(storageprovisioner.ModelManifoldConfig{
		APICallerName: "grenouille",
		ClockName:     "bustopher",
	})
	c.Check(manifold.Inputs, jc.DeepEquals, []string{"grenouille", "bustopher"})
	c.Check(manifold.Output, gc.IsNil)
	c.Check(manifold.Start, gc.NotNil)
	// ...Start is *not* well-tested, in common with many manifold configs.
}

func (s *ManifoldSuite) TestMissingClock(c *gc.C) {
	manifold := storageprovisioner.ModelManifold(storageprovisioner.ModelManifoldConfig{
		APICallerName: "api-caller",
		ClockName:     "clock",
	})
	_, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"api-caller": struct{ base.APICaller }{},
		"clock":      dependency.ErrMissing,
	}))
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestMissingAPICaller(c *gc.C) {
	manifold := storageprovisioner.ModelManifold(storageprovisioner.ModelManifoldConfig{
		APICallerName: "api-caller",
		ClockName:     "clock",
	})
	_, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"api-caller": dependency.ErrMissing,
		"clock":      struct{ clock.Clock }{},
	}))
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}
