// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/v2/api/base/testing"
	"github.com/juju/juju/v2/api/client/modelupgrader"
	"github.com/juju/juju/v2/rpc/params"
	coretesting "github.com/juju/juju/v2/testing"
)

type UpgradeModelSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&UpgradeModelSuite{})

func (s *UpgradeModelSuite) TestAbortModelUpgrade(c *gc.C) {
	called := false
	apiCaller := basetesting.BestVersionCaller{
		BestVersion: 1,
		APICallerFunc: func(objType string, version int, id, request string, args, result interface{}) error {
			c.Check(objType, gc.Equals, "ModelUpgrader")
			c.Check(request, gc.Equals, "AbortModelUpgrade")
			in, ok := args.(params.ModelParam)
			c.Assert(ok, jc.IsTrue)
			c.Assert(in, gc.DeepEquals, params.ModelParam{
				ModelTag: coretesting.ModelTag.String(),
			})
			c.Assert(result, gc.IsNil)
			called = true
			return nil
		},
	}

	client := modelupgrader.NewClient(apiCaller)
	err := client.AbortModelUpgrade(coretesting.ModelTag.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *UpgradeModelSuite) TestUpgradeModel(c *gc.C) {
	called := false
	apiCaller := basetesting.BestVersionCaller{
		BestVersion: 1,
		APICallerFunc: func(objType string, facadeVersion int, id, request string, args, result interface{}) error {
			c.Check(objType, gc.Equals, "ModelUpgrader")
			c.Check(request, gc.Equals, "UpgradeModel")
			c.Check(facadeVersion, gc.Equals, 1)
			in, ok := args.(params.UpgradeModel)
			c.Assert(ok, jc.IsTrue)
			c.Assert(in, gc.DeepEquals, params.UpgradeModel{
				ModelTag:            coretesting.ModelTag.String(),
				ToVersion:           version.MustParse("2.9.1"),
				IgnoreAgentVersions: true,
				DryRun:              true,
			})
			c.Assert(result, gc.IsNil)
			called = true
			return nil
		},
	}

	client := modelupgrader.NewClient(apiCaller)
	err := client.UpgradeModel(coretesting.ModelTag.Id(), version.MustParse("2.9.1"), "", true, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}
