// Copyright 2015 Canonical Ltd. All rights reserved.

package service

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/romulus/wireformat/budget"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&allocationSuite{})

type allocationSuite struct {
	testing.CleanupSuite
	stub      *testing.Stub
	apiClient *mockBudgetAPIClient
	allocate  DeployStep
	ctx       *cmd.Context
}

func (s *allocationSuite) SetUpTest(c *gc.C) {
	s.CleanupSuite.SetUpTest(c)
	s.stub = &testing.Stub{}
	s.apiClient = &mockBudgetAPIClient{Stub: s.stub}
	s.allocate = &AllocateBudget{AllocationSpec: "personal:100"}
	s.PatchValue(&getApiClient, func(*httpbakery.Client) (apiClient, error) { return s.apiClient, nil })
	s.ctx = coretesting.Context(c)
}

func (s *allocationSuite) TestMeteredCharm(c *gc.C) {
	client := httpbakery.NewClient()
	d := DeploymentInfo{
		CharmURL:    charm.MustParseURL("cs:quantal/metered-1"),
		ServiceName: "service name",
		ModelUUID:   "model uuid",
	}
	err := s.allocate.RunPre(&mockAPIConnection{Stub: s.stub}, client, s.ctx, d)
	c.Assert(err, jc.ErrorIsNil)
	err = s.allocate.RunPost(&mockAPIConnection{Stub: s.stub}, client, s.ctx, d, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []testing.StubCall{{
		"APICall", []interface{}{"Charms", "IsMetered", params.CharmInfo{CharmURL: "cs:quantal/metered-1"}},
	}, {
		"CreateAllocation", []interface{}{"personal", "100", "model uuid", []string{"service name"}},
	}})
	c.Assert(coretesting.Stdout(s.ctx), gc.Equals, "Allocation created.\n")
}

func (s *allocationSuite) TestLocalCharm(c *gc.C) {
	client := httpbakery.NewClient()
	d := DeploymentInfo{
		CharmURL:    charm.MustParseURL("local:quantal/metered-1"),
		ServiceName: "service name",
		ModelUUID:   "model uuid",
	}
	err := s.allocate.RunPre(&mockAPIConnection{Stub: s.stub}, client, s.ctx, d)
	c.Assert(err, jc.ErrorIsNil)
	err = s.allocate.RunPost(&mockAPIConnection{Stub: s.stub}, client, s.ctx, d, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []testing.StubCall{})
	c.Assert(coretesting.Stdout(s.ctx), gc.Equals, "")
}

func (s *allocationSuite) TestMeteredCharmInvalidAllocation(c *gc.C) {
	client := httpbakery.NewClient()
	d := DeploymentInfo{
		CharmURL:    charm.MustParseURL("cs:quantal/metered-1"),
		ServiceName: "service name",
		ModelUUID:   "model uuid",
	}
	s.allocate = &AllocateBudget{AllocationSpec: ""}
	err := s.allocate.RunPre(&mockAPIConnection{Stub: s.stub}, client, s.ctx, d)
	c.Assert(err, gc.ErrorMatches, `invalid allocation, expecting <budget>:<limit>`)

	err = s.allocate.RunPost(&mockAPIConnection{Stub: s.stub}, client, s.ctx, d, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []testing.StubCall{{
		"APICall", []interface{}{"Charms", "IsMetered", params.CharmInfo{CharmURL: "cs:quantal/metered-1"}},
	}})

}

func (s *allocationSuite) TestMeteredCharmServiceUnavail(c *gc.C) {
	client := httpbakery.NewClient()
	d := DeploymentInfo{
		CharmURL:    charm.MustParseURL("cs:quantal/metered-1"),
		ServiceName: "service name",
		ModelUUID:   "model uuid",
	}
	s.stub.SetErrors(nil, budget.NotAvailError{})
	err := s.allocate.RunPre(&mockAPIConnection{Stub: s.stub}, client, s.ctx, d)
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []testing.StubCall{{
		"APICall", []interface{}{"Charms", "IsMetered", params.CharmInfo{CharmURL: "cs:quantal/metered-1"}},
	}, {
		"CreateAllocation", []interface{}{"personal", "100", "model uuid", []string{"service name"}},
	}})
	c.Assert(coretesting.Stdout(s.ctx), gc.Equals, "WARNING: Budget allocation not created - service unreachable.\n")
}

func (s *allocationSuite) TestMeteredCharmRemoveAllocation(c *gc.C) {
	client := httpbakery.NewClient()
	d := DeploymentInfo{
		CharmURL:    charm.MustParseURL("cs:quantal/metered-1"),
		ServiceName: "service name",
		ModelUUID:   "model uuid",
	}
	err := s.allocate.RunPre(&mockAPIConnection{Stub: s.stub}, client, s.ctx, d)
	c.Assert(err, jc.ErrorIsNil)
	err = s.allocate.RunPost(&mockAPIConnection{Stub: s.stub}, client, s.ctx, d, errors.New("deployment failed"))
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []testing.StubCall{{
		"APICall", []interface{}{"Charms", "IsMetered", params.CharmInfo{CharmURL: "cs:quantal/metered-1"}},
	}, {
		"CreateAllocation", []interface{}{"personal", "100", "model uuid", []string{"service name"}}}, {
		"DeleteAllocation", []interface{}{"model uuid", "service name"}},
	})
	c.Assert(coretesting.Stdout(s.ctx), gc.Equals, "Allocation created.\nAllocation removed.\n")
}

func (s *allocationSuite) TestUnmeteredCharm(c *gc.C) {
	client := httpbakery.NewClient()
	d := DeploymentInfo{
		CharmURL:    charm.MustParseURL("cs:quantal/unmetered-1"),
		ServiceName: "service name",
		ModelUUID:   "environment uuid",
	}
	err := s.allocate.RunPre(&mockAPIConnection{Stub: s.stub}, client, s.ctx, d)
	c.Assert(err, jc.ErrorIsNil)
	err = s.allocate.RunPost(&mockAPIConnection{Stub: s.stub}, client, s.ctx, d, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []testing.StubCall{{
		"APICall", []interface{}{"Charms", "IsMetered", params.CharmInfo{CharmURL: "cs:quantal/unmetered-1"}},
	}})
}

type mockBudgetAPIClient struct {
	*testing.Stub
}

// CreateAllocation implements apiClient.
func (c *mockBudgetAPIClient) CreateAllocation(budget, limit, model string, services []string) (string, error) {
	c.MethodCall(c, "CreateAllocation", budget, limit, model, services)
	return "Allocation created.", c.NextErr()
}

// DeleteAllocation implements apiClient.
func (c *mockBudgetAPIClient) DeleteAllocation(model, service string) (string, error) {
	c.MethodCall(c, "DeleteAllocation", model, service)
	return "Allocation removed.", c.NextErr()
}
