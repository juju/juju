// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/state"
)

type machineSuite struct{}

var _ = gc.Suite(&machineSuite{})

func (s *machineSuite) TestMachineJobFromParams(c *gc.C) {
	var tests = []struct {
		name model.MachineJob
		want state.MachineJob
		err  string
	}{{
		name: model.JobHostUnits,
		want: state.JobHostUnits,
	}, {
		name: model.JobManageModel,
		want: state.JobManageModel,
	}, {
		name: "invalid",
		want: -1,
		err:  `invalid machine job "invalid"`,
	}}
	for _, test := range tests {
		got, err := common.MachineJobFromParams(test.name)
		if err != nil {
			c.Check(err, gc.ErrorMatches, test.err)
		}
		c.Check(got, gc.Equals, test.want)
	}
}
