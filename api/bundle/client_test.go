// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/bundle"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

type bundleMockSuite struct {
	coretesting.BaseSuite
	bundleClient *bundle.Client
}

var _ = gc.Suite(&bundleMockSuite{})

func newClient(f basetesting.APICallerFunc, ver int) *bundle.Client {
	return bundle.NewClient(basetesting.BestVersionCaller{f, ver})
}

func (s *bundleMockSuite) TestFailExportBundlev1(c *gc.C) {
	client := newClient(
		func(objType string,
			version int,
			id,
			request string,
			args,
			response interface{},
		) error {
			c.Check(objType, gc.Equals, "Bundle")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ExportBundle")
			c.Assert(args, gc.Equals, nil)
			result := response.(*params.StringResult)
			result.Result = ""
			return nil
		}, 1,
	)
	result, err := client.ExportBundle()
	c.Assert(err, gc.ErrorMatches, "command not supported on v1")
	c.Assert(result, jc.DeepEquals, "")
}

func (s *bundleMockSuite) TestExportBundlev2(c *gc.C) {
	client := newClient(
		func(objType string, version int,
			id,
			request string,
			args,
			response interface{},
		) error {
			c.Check(objType, gc.Equals, "Bundle")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ExportBundle")
			c.Assert(args, gc.Equals, nil)
			c.Assert(response, gc.FitsTypeOf, &params.StringResult{})
			result := response.(*params.StringResult)
			result.Result = "applications:\n  " +
				"ubuntu:\n    " +
				"charm: cs:trusty/ubuntu\n    " +
				"series: trusty\n    " +
				"num_units: 1\n    " +
				"to:\n    " +
				"- \"0\"\n    " +
				"options:\n      " +
				"key: value\n" +
				"series: xenial\n" +
				"relations:\n" +
				"- []\n"
			return nil
		}, 2,
	)
	result, err := client.ExportBundle()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, "applications:\n  "+
		"ubuntu:\n    "+
		"charm: cs:trusty/ubuntu\n    "+
		"series: trusty\n    "+
		"num_units: 1\n    "+
		"to:\n    "+
		"- \"0\"\n    "+
		"options:\n      "+
		"key: value\n"+
		"series: xenial\n"+
		"relations:\n"+
		"- []\n")
}

func (s *bundleMockSuite) TestExportBundleErrorv2(c *gc.C) {
	client := newClient(
		func(objType string, version int,
			id,
			request string,
			args,
			response interface{},
		) error {
			c.Check(objType, gc.Equals, "Bundle")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ExportBundle")
			c.Assert(args, gc.Equals, nil)
			c.Assert(response, gc.FitsTypeOf, &params.StringResult{})
			result := response.(*params.StringResult)
			result.Result = ""
			return result.Error
		}, 2,
	)
	result, err := client.ExportBundle()
	c.Assert(err, gc.NotNil)
	c.Assert(result, jc.DeepEquals, "")
	c.Check(err.Error(), jc.Contains, "export failed")
}
