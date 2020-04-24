// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/worker/storageprovisioner"
)

type ManifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) TestManifold(c *gc.C) {
	manifold := storageprovisioner.ModelManifold(storageprovisioner.ModelManifoldConfig{
		APICallerName:       "grenouille",
		StorageRegistryName: "environ",
	})
	c.Check(manifold.Inputs, jc.DeepEquals, []string{"grenouille", "environ"})
	c.Check(manifold.Output, gc.IsNil)
	c.Check(manifold.Start, gc.NotNil)
	// ...Start is *not* well-tested, in common with many manifold configs.
}

func (s *ManifoldSuite) TestMissingAPICaller(c *gc.C) {
	manifold := storageprovisioner.ModelManifold(storageprovisioner.ModelManifoldConfig{
		APICallerName:       "api-caller",
		StorageRegistryName: "environ",
	})
	_, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"api-caller": dependency.ErrMissing,
		"clock":      struct{ clock.Clock }{},
		"environ":    struct{ environs.Environ }{},
	}))
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestMissingEnviron(c *gc.C) {
	manifold := storageprovisioner.ModelManifold(storageprovisioner.ModelManifoldConfig{
		APICallerName:       "api-caller",
		StorageRegistryName: "environ",
	})
	_, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"api-caller": struct{ base.APICaller }{},
		"clock":      struct{ clock.Clock }{},
		"environ":    dependency.ErrMissing,
	}))
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}
