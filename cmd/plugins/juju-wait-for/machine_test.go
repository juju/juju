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

type machineScopeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&machineScopeSuite{})

func (s *machineScopeSuite) TestGetIdentValue(c *gc.C) {
	tests := []struct {
		Field       string
		MachineInfo *params.MachineInfo
		Expected    query.Box
	}{{
		Field:       "id",
		MachineInfo: &params.MachineInfo{Id: "0/lxd/0"},
		Expected:    query.NewString("0/lxd/0"),
	}, {
		Field:       "life",
		MachineInfo: &params.MachineInfo{Life: life.Alive},
		Expected:    query.NewString("alive"),
	}, {
		Field: "status",
		MachineInfo: &params.MachineInfo{AgentStatus: params.StatusInfo{
			Current: status.Active,
		}},
		Expected: query.NewString("active"),
	}, {
		Field: "instance-status",
		MachineInfo: &params.MachineInfo{InstanceStatus: params.StatusInfo{
			Current: status.Active,
		}},
		Expected: query.NewString("active"),
	}, {
		Field:       "series",
		MachineInfo: &params.MachineInfo{Series: "focal"},
		Expected:    query.NewString("focal"),
	}, {
		Field:       "container-type",
		MachineInfo: &params.MachineInfo{ContainerType: "lxd"},
		Expected:    query.NewString("lxd"),
	}}
	for i, test := range tests {
		c.Logf("%d: GetIdentValue %q", i, test.Field)
		scope := MachineScope{
			ctx:         MakeScopeContext(),
			MachineInfo: test.MachineInfo,
		}
		result, err := scope.GetIdentValue(test.Field)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result, gc.DeepEquals, test.Expected)
	}
}

func (s *machineScopeSuite) TestGetIdentValueError(c *gc.C) {
	scope := MachineScope{
		ctx:         MakeScopeContext(),
		MachineInfo: &params.MachineInfo{},
	}
	result, err := scope.GetIdentValue("bad")
	c.Assert(err, gc.ErrorMatches, `Runtime Error: identifier "bad" not found on MachineInfo: invalid identifer`)
	c.Assert(result, gc.IsNil)
}
