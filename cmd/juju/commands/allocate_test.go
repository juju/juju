// Copyright 2015 Canonical Ltd. All rights reserved.

package commands

import (
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/apiserver/params"
)

var _ = gc.Suite(&allocationSuite{})

type allocationSuite struct {
	testing.CleanupSuite
	stub      *testing.Stub
	apiClient *mockAPIClient
	allocate  DeployStep
}

func (s *allocationSuite) SetUpTest(c *gc.C) {
	s.CleanupSuite.SetUpTest(c)
	s.stub = &testing.Stub{}
	s.apiClient = &mockAPIClient{Stub: s.stub}
	s.allocate = &AllocateBudget{AllocationSpec: "personal:100"}
	s.PatchValue(&getApiClient, func(*http.Client) (apiClient, error) { return s.apiClient, nil })
}

func (s *allocationSuite) TestMeteredCharm(c *gc.C) {
	client := httpbakery.NewClient().Client
	d := DeploymentInfo{
		CharmURL:    charm.MustParseURL("local:quantal/metered-1"),
		ServiceName: "service name",
		EnvUUID:     "model uuid",
	}
	err := s.allocate.RunPre(&mockAPIConnection{Stub: s.stub}, client, d)
	c.Assert(err, jc.ErrorIsNil)
	err = s.allocate.RunPost(&mockAPIConnection{Stub: s.stub}, client, d, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []testing.StubCall{{
		"APICall", []interface{}{"Charms", "IsMetered", params.CharmInfo{CharmURL: "local:quantal/metered-1"}},
	}, {
		"CreateAllocation", []interface{}{"personal", "100", "model uuid", []string{"service name"}}}})

}

func (s *allocationSuite) TestMeteredCharmInvalidAllocation(c *gc.C) {
	client := httpbakery.NewClient().Client
	d := DeploymentInfo{
		CharmURL:    charm.MustParseURL("local:quantal/metered-1"),
		ServiceName: "service name",
		EnvUUID:     "model uuid",
	}
	s.allocate = &AllocateBudget{AllocationSpec: ""}
	err := s.allocate.RunPre(&mockAPIConnection{Stub: s.stub}, client, d)
	c.Assert(err, gc.ErrorMatches, `invalid budget specification, expecting <budget>:<limit>`)
	err = s.allocate.RunPost(&mockAPIConnection{Stub: s.stub}, client, d, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []testing.StubCall{{
		"APICall", []interface{}{"Charms", "IsMetered", params.CharmInfo{CharmURL: "local:quantal/metered-1"}},
	}})

}

func (s *allocationSuite) TestMeteredCharmRemoveAllocation(c *gc.C) {
	client := httpbakery.NewClient().Client
	d := DeploymentInfo{
		CharmURL:    charm.MustParseURL("local:quantal/metered-1"),
		ServiceName: "service name",
		EnvUUID:     "model uuid",
	}
	err := s.allocate.RunPre(&mockAPIConnection{Stub: s.stub}, client, d)
	c.Assert(err, jc.ErrorIsNil)
	err = s.allocate.RunPost(&mockAPIConnection{Stub: s.stub}, client, d, errors.New("deployment failed"))
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []testing.StubCall{{
		"APICall", []interface{}{"Charms", "IsMetered", params.CharmInfo{CharmURL: "local:quantal/metered-1"}},
	}, {
		"CreateAllocation", []interface{}{"personal", "100", "model uuid", []string{"service name"}}}, {
		"DeleteAllocation", []interface{}{"model uuid", "service name"}},
	})

}

func (s *allocationSuite) TestUnmeteredCharm(c *gc.C) {
	client := httpbakery.NewClient().Client
	d := DeploymentInfo{
		CharmURL:    charm.MustParseURL("local:quantal/unmetered-1"),
		ServiceName: "service name",
		EnvUUID:     "environment uuid",
	}
	err := s.allocate.RunPre(&mockAPIConnection{Stub: s.stub}, client, d)
	c.Assert(err, jc.ErrorIsNil)
	err = s.allocate.RunPost(&mockAPIConnection{Stub: s.stub}, client, d, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []testing.StubCall{{
		"APICall", []interface{}{"Charms", "IsMetered", params.CharmInfo{CharmURL: "local:quantal/unmetered-1"}},
	}})
}

type mockAPIClient struct {
	*testing.Stub
	resp string
}

// CreateAllocation implements apiClient.
func (c *mockAPIClient) CreateAllocation(budget, limit, model string, services []string) (string, error) {
	c.MethodCall(c, "CreateAllocation", budget, limit, model, services)
	return c.resp, c.NextErr()
}

// DeleteAllocation implements apiClient.
func (c *mockAPIClient) DeleteAllocation(model, service string) (string, error) {
	c.MethodCall(c, "DeleteAllocation", model, service)
	return c.resp, c.NextErr()
}
