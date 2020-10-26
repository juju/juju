// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/plugins/juju-wait-for/query"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
)

type unitScopeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&unitScopeSuite{})

func (s *unitScopeSuite) TestGetIdentValue(c *gc.C) {
	tests := []struct {
		Field    string
		UnitInfo *params.UnitInfo
		Expected query.Box
	}{{
		Field:    "name",
		UnitInfo: &params.UnitInfo{Name: "model name"},
		Expected: query.NewString("model name"),
	}, {
		Field:    "application",
		UnitInfo: &params.UnitInfo{Application: "app-name"},
		Expected: query.NewString("app-name"),
	}, {
		Field:    "series",
		UnitInfo: &params.UnitInfo{Series: "series-name"},
		Expected: query.NewString("series-name"),
	}, {
		Field:    "charm-url",
		UnitInfo: &params.UnitInfo{CharmURL: "charm-url-value"},
		Expected: query.NewString("charm-url-value"),
	}, {
		Field:    "life",
		UnitInfo: &params.UnitInfo{Life: life.Alive},
		Expected: query.NewString("alive"),
	}, {
		Field:    "public-address",
		UnitInfo: &params.UnitInfo{PublicAddress: "public-address-1"},
		Expected: query.NewString("public-address-1"),
	}, {
		Field:    "private-address",
		UnitInfo: &params.UnitInfo{PrivateAddress: "private-address-1"},
		Expected: query.NewString("private-address-1"),
	}, {
		Field:    "machine-id",
		UnitInfo: &params.UnitInfo{MachineId: "machine-id-1"},
		Expected: query.NewString("machine-id-1"),
	}, {
		Field:    "principal",
		UnitInfo: &params.UnitInfo{Principal: "principal-1"},
		Expected: query.NewString("principal-1"),
	}, {
		Field:    "subordinate",
		UnitInfo: &params.UnitInfo{Subordinate: true},
		Expected: query.NewBool(true),
	}, {
		Field: "workload-status",
		UnitInfo: &params.UnitInfo{WorkloadStatus: params.StatusInfo{
			Current: status.Active,
		}},
		Expected: query.NewString("active"),
	}, {
		Field: "agent-status",
		UnitInfo: &params.UnitInfo{AgentStatus: params.StatusInfo{
			Current: status.Active,
		}},
		Expected: query.NewString("active"),
	}}
	for i, test := range tests {
		c.Logf("%d: GetIdentValue %q", i, test.Field)
		scope := UnitScope{
			Scope:    NewGenericScope(),
			UnitInfo: test.UnitInfo,
		}
		result, err := scope.GetIdentValue(test.Field)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result, gc.DeepEquals, test.Expected)
	}
}

func (s *unitScopeSuite) TestGetIdentValueError(c *gc.C) {
	scope := UnitScope{
		Scope:    NewGenericScope(),
		UnitInfo: &params.UnitInfo{},
	}
	result, err := scope.GetIdentValue("bad")
	c.Assert(err, gc.ErrorMatches, `Runtime Error: identifier "bad" not found on UnitInfo: invalid identifer`)
	c.Assert(result, gc.IsNil)
}
