// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/storageprovisioner"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) TestManifold(c *tc.C) {
	manifold := storageprovisioner.ModelManifold(storageprovisioner.ModelManifoldConfig{
		APICallerName:       "grenouille",
		StorageRegistryName: "environ",
	})
	c.Check(manifold.Inputs, tc.DeepEquals, []string{"grenouille", "environ"})
	c.Check(manifold.Output, tc.IsNil)
	c.Check(manifold.Start, tc.NotNil)
	// ...Start is *not* well-tested, in common with many manifold configs.
}

func (s *ManifoldSuite) TestMissingAPICaller(c *tc.C) {
	manifold := storageprovisioner.ModelManifold(storageprovisioner.ModelManifoldConfig{
		APICallerName:       "api-caller",
		StorageRegistryName: "environ",
	})
	_, err := manifold.Start(c.Context(), dt.StubGetter(map[string]interface{}{
		"api-caller": dependency.ErrMissing,
		"clock":      struct{ clock.Clock }{},
		"environ":    struct{ environs.Environ }{},
	}))
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestMissingEnviron(c *tc.C) {
	manifold := storageprovisioner.ModelManifold(storageprovisioner.ModelManifoldConfig{
		APICallerName:       "api-caller",
		StorageRegistryName: "environ",
	})
	_, err := manifold.Start(c.Context(), dt.StubGetter(map[string]interface{}{
		"api-caller": struct{ base.APICaller }{},
		"clock":      struct{ clock.Clock }{},
		"environ":    dependency.ErrMissing,
	}))
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}
