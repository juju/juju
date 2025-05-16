// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package computeprovisioner_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
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

func TestManifoldSuite(t *stdtesting.T) { tc.Run(t, &ManifoldSuite{}) }
func (s *ManifoldSuite) makeManifold(c *tc.C) dependency.Manifold {
	fakeNewProvFunc := func(computeprovisioner.ControllerAPI, computeprovisioner.MachineService, computeprovisioner.MachinesAPI, computeprovisioner.ToolsFinder,
		computeprovisioner.DistributionGroupFinder, agent.Config, logger.Logger, computeprovisioner.Environ,
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
	return computeprovisioner.Manifold(computeprovisioner.ManifoldConfig{
		AgentName:          "agent",
		APICallerName:      "api-caller",
		Logger:             loggertesting.WrapCheckLog(c),
		EnvironName:        "environ",
		DomainServicesName: "fake-domain-services",
		GetMachineService:  fakeGetMachineServiceFunc,
		NewProvisionerFunc: fakeNewProvFunc,
	})
}

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.stub.ResetCalls()
}

func (s *ManifoldSuite) TestManifold(c *tc.C) {
	manifold := s.makeManifold(c)
	c.Check(manifold.Inputs, tc.SameContents, []string{"agent", "api-caller", "environ", "fake-domain-services"})
	c.Check(manifold.Output, tc.IsNil)
	c.Check(manifold.Start, tc.NotNil)
}

func (s *ManifoldSuite) TestMissingAgent(c *tc.C) {
	manifold := s.makeManifold(c)
	w, err := manifold.Start(c.Context(), dt.StubGetter(map[string]interface{}{
		"agent":      dependency.ErrMissing,
		"api-caller": struct{ base.APICaller }{},
		"environ":    struct{ environs.Environ }{},
	}))
	c.Check(w, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestMissingAPICaller(c *tc.C) {
	manifold := s.makeManifold(c)
	w, err := manifold.Start(c.Context(), dt.StubGetter(map[string]interface{}{
		"agent":      struct{ agent.Agent }{},
		"api-caller": dependency.ErrMissing,
		"environ":    struct{ environs.Environ }{},
	}))
	c.Check(w, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestMissingEnviron(c *tc.C) {
	manifold := s.makeManifold(c)
	w, err := manifold.Start(c.Context(), dt.StubGetter(map[string]interface{}{
		"agent":      struct{ agent.Agent }{},
		"api-caller": struct{ base.APICaller }{},
		"environ":    dependency.ErrMissing,
	}))
	c.Check(w, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStarts(c *tc.C) {
	manifold := s.makeManifold(c)
	w, err := manifold.Start(c.Context(), dt.StubGetter(map[string]interface{}{
		"agent":      new(fakeAgent),
		"api-caller": apitesting.APICallerFunc(nil),
		"environ":    struct{ environs.Environ }{},
	}))
	c.Check(w, tc.NotNil)
	c.Check(err, tc.ErrorIsNil)
	s.stub.CheckCallNames(c, "GetMachineService", "NewProvisionerFunc")
}

type fakeAgent struct {
	agent.Agent
}

func (a *fakeAgent) CurrentConfig() agent.Config {
	return nil
}
