// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dt "github.com/juju/worker/v5/dependency/testing"

	"github.com/juju/juju/environs"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/storageprovisioner"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite
}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) TestManifold(c *tc.C) {
	manifold := storageprovisioner.ModelManifold(storageprovisioner.ModelManifoldConfig{
		DomainServicesName:  "domain-services",
		StorageRegistryName: "environ",
	})
	c.Check(manifold.Inputs, tc.DeepEquals, []string{"domain-services", "environ"})
	c.Check(manifold.Output, tc.IsNil)
	c.Check(manifold.Start, tc.NotNil)
	// ...Start is *not* well-tested, in common with many manifold configs.
}

func (s *ManifoldSuite) TestMissingDomainServices(c *tc.C) {
	manifold := storageprovisioner.ModelManifold(storageprovisioner.ModelManifoldConfig{
		DomainServicesName:  "domain-services",
		StorageRegistryName: "environ",
		Clock:               struct{ clock.Clock }{},
		Logger:              loggertesting.WrapCheckLog(c),
		NewWorker:           func(storageprovisioner.Config) (worker.Worker, error) { return nil, nil },
	})
	_, err := manifold.Start(c.Context(), dt.StubGetter(map[string]any{
		"domain-services": dependency.ErrMissing,
		"environ":         struct{ environs.Environ }{},
	}))
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestMissingEnviron(c *tc.C) {
	manifold := storageprovisioner.ModelManifold(storageprovisioner.ModelManifoldConfig{
		DomainServicesName:  "domain-services",
		StorageRegistryName: "environ",
		Clock:               struct{ clock.Clock }{},
		Logger:              loggertesting.WrapCheckLog(c),
		NewWorker:           func(storageprovisioner.Config) (worker.Worker, error) { return nil, nil },
	})
	_, err := manifold.Start(c.Context(), dt.StubGetter(map[string]any{
		"domain-services": &stubDomainServices{},
		"environ":         dependency.ErrMissing,
	}))
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

// stubDomainServices is a minimal stub that satisfies services.DomainServices.
type stubDomainServices struct {
	services.ControllerDomainServices
	services.ModelDomainServices
}
