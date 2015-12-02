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
	manifold := storageprovisioner.Manifold(storageprovisioner.ManifoldConfig{
		APICallerName: "grenouille",
		ClockName:     "bustopher",
	})
	c.Check(manifold.Inputs, jc.DeepEquals, []string{"grenouille", "bustopher"})
	c.Check(manifold.Output, gc.IsNil)
	c.Check(manifold.Start, gc.NotNil)
	// ...Start is *not* well-tested, in common with many manifold configs.
	// Am starting to think that tasdomas nailed it with the metrics manifolds
	// that take constructors as config... reviewers, thoughts please?
}

func (s *ManifoldSuite) TestMissingClock(c *gc.C) {
	manifold := storageprovisioner.Manifold(storageprovisioner.ManifoldConfig{
		APICallerName: "api-caller",
		ClockName:     "clock",
	})
	_, err := manifold.Start(dt.StubGetResource(dt.StubResources{
		"api-caller": dt.StubResource{Output: struct{ base.APICaller }{}},
		"clock":      dt.StubResource{Error: dependency.ErrMissing},
	}))
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestMissingAPICaller(c *gc.C) {
	manifold := storageprovisioner.Manifold(storageprovisioner.ManifoldConfig{
		APICallerName: "api-caller",
		ClockName:     "clock",
	})
	_, err := manifold.Start(dt.StubGetResource(dt.StubResources{
		"api-caller": dt.StubResource{Error: dependency.ErrMissing},
		"clock":      dt.StubResource{Output: struct{ clock.Clock }{}},
	}))
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}
