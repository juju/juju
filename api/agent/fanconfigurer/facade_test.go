// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fanconfigurer_test

import (
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/fanconfigurer"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&FanConfigurerSuite{})

type FanConfigurerSuite struct {
	coretesting.BaseSuite
}

func (s *FanConfigurerSuite) TestFanConfig(c *gc.C) {
	input := "172.31.0.0/16=253.0.0.0/8"
	result, err := network.ParseFanConfig(input)
	c.Assert(err, jc.ErrorIsNil)

	var callCount int
	apiCaller := testing.BestVersionCaller{APICallerFunc: func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "FanConfigurer")
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "FanConfig")
		c.Check(arg, gc.DeepEquals, params.Entity{
			Tag: "machine-0",
		})
		c.Assert(result, gc.FitsTypeOf, &params.FanConfigResult{})
		*(result.(*params.FanConfigResult)) = params.FanConfigResult{
			Fans: []params.FanConfigEntry{{
				Underlay: "172.31.0.0/16",
				Overlay:  "253.0.0.0/8",
			}},
		}
		callCount++
		return nil
	}, BestVersion: 2}

	client := fanconfigurer.NewFacade(apiCaller)
	cfg, err := client.FanConfig(names.NewMachineTag("0"))
	c.Check(err, jc.ErrorIsNil)
	c.Assert(cfg, jc.DeepEquals, result)
}
