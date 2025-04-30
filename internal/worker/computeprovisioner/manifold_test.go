// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package computeprovisioner_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/environs"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/computeprovisioner"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	stub testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) makeManifold(c *gc.C) dependency.Manifold {
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

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.stub.ResetCalls()
}

func (s *ManifoldSuite) TestManifold(c *gc.C) {
	manifold := s.makeManifold(c)
	c.Check(manifold.Inputs, jc.SameContents, []string{"agent", "api-caller", "environ", "fake-domain-services"})
	c.Check(manifold.Output, gc.IsNil)
	c.Check(manifold.Start, gc.NotNil)
}

func (s *ManifoldSuite) TestMissingAgent(c *gc.C) {
	manifold := s.makeManifold(c)
	w, err := manifold.Start(context.Background(), dt.StubGetter(map[string]interface{}{
		"agent":      dependency.ErrMissing,
		"api-caller": struct{ base.APICaller }{},
		"environ":    struct{ environs.Environ }{},
	}))
	c.Check(w, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestMissingAPICaller(c *gc.C) {
	manifold := s.makeManifold(c)
	w, err := manifold.Start(context.Background(), dt.StubGetter(map[string]interface{}{
		"agent":      struct{ agent.Agent }{},
		"api-caller": dependency.ErrMissing,
		"environ":    struct{ environs.Environ }{},
	}))
	c.Check(w, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestMissingEnviron(c *gc.C) {
	manifold := s.makeManifold(c)
	w, err := manifold.Start(context.Background(), dt.StubGetter(map[string]interface{}{
		"agent":      struct{ agent.Agent }{},
		"api-caller": struct{ base.APICaller }{},
		"environ":    dependency.ErrMissing,
	}))
	c.Check(w, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStarts(c *gc.C) {
	manifold := s.makeManifold(c)
	w, err := manifold.Start(context.Background(), dt.StubGetter(map[string]interface{}{
		"agent":      new(fakeAgent),
		"api-caller": apitesting.APICallerFunc(nil),
		"environ":    struct{ environs.Environ }{},
	}))
	c.Check(w, gc.NotNil)
	c.Check(err, jc.ErrorIsNil)
	s.stub.CheckCallNames(c, "GetMachineService", "NewProvisionerFunc")
}

type fakeAgent struct {
	agent.Agent
}

func (a *fakeAgent) CurrentConfig() agent.Config {
	return nil
}
