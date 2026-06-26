// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package computeprovisioner_test

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/dependency"
	dt "github.com/juju/worker/v5/dependency/testing"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/environs"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/computeprovisioner"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite
	stub testhelpers.Stub
}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) makeManifold(c *tc.C) dependency.Manifold {
	fakeNewProvFunc := func(computeprovisioner.ControllerAPI, computeprovisioner.MachineService, computeprovisioner.MachinesAPI, computeprovisioner.ToolsFinder,
		computeprovisioner.DistributionGroupFinder, names.Tag, logger.Logger, computeprovisioner.Environ,
	) (computeprovisioner.Provisioner, error) {
		s.stub.AddCall("NewProvisionerFunc")
		return struct{ computeprovisioner.Provisioner }{}, nil
	}
	fakeGetMachineServiceFunc := func(getter dependency.Getter, name string) (computeprovisioner.MachineService, error) {
		s.stub.AddCall("GetMachineService")
		return struct {
			computeprovisioner.MachineService
		}{}, nil
	}
	fakeGetDomainServicesFunc := func(getter dependency.Getter, name string) (computeprovisioner.DomainServices, error) {
		s.stub.AddCall("GetDomainServices")
		return computeprovisioner.DomainServices{}, nil
	}
	return computeprovisioner.Manifold(computeprovisioner.ManifoldConfig{
		Logger:             loggertesting.WrapCheckLog(c),
		EnvironName:        "environ",
		DomainServicesName: "fake-domain-services",
		GetMachineService:  fakeGetMachineServiceFunc,
		GetDomainServices:  fakeGetDomainServicesFunc,
		AgentTag:           names.NewMachineTag("0"),
		ModelUUID:          "fake-model-uuid",
		NewProvisionerFunc: fakeNewProvFunc,
	})
}

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.stub.ResetCalls()
}

func (s *ManifoldSuite) TestManifold(c *tc.C) {
	manifold := s.makeManifold(c)
	c.Check(manifold.Inputs, tc.SameContents, []string{"environ", "fake-domain-services"})
	c.Check(manifold.Output, tc.IsNil)
	c.Check(manifold.Start, tc.NotNil)
}

func (s *ManifoldSuite) TestMissingEnviron(c *tc.C) {
	manifold := s.makeManifold(c)
	w, err := manifold.Start(c.Context(), dt.StubGetter(map[string]any{
		"environ": dependency.ErrMissing,
	}))
	c.Check(w, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStarts(c *tc.C) {
	manifold := s.makeManifold(c)
	w, err := manifold.Start(c.Context(), dt.StubGetter(map[string]any{
		"environ": struct{ environs.Environ }{},
	}))
	c.Check(w, tc.NotNil)
	c.Check(err, tc.ErrorIsNil)
	s.stub.CheckCallNames(c, "GetDomainServices", "GetMachineService", "NewProvisionerFunc")
}
