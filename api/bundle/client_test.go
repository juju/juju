// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/application"
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

func newClient(f basetesting.APICallerFunc) *bundle.Client {
	return bundle.NewClient(basetesting.BestVersionCaller{f, 1})
}

// TODO: vinu2003 to be implemented when the server Facade is done.
func newClientv2(f basetesting.APICallerFunc) *application.Client {
	return application.NewClient(basetesting.BestVersionCaller{f, 2})
}

func (s *bundleMockSuite) TestPanicExportBundlev1(c *gc.C) {
	client := newClient(
		func(objType string, version int,
			id,
			request string,
			a,
			response interface{},
		) error {
			c.Check(objType, gc.Equals, "Bundle")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ExportBundle")
			c.Assert(response, gc.FitsTypeOf, &params.StringResult{})
			result := response.(*params.StringResult)
			result.Result = ""
			return nil
		},
	)
	result, err := client.ExportBundle()
	c.Assert(err, gc.ErrorMatches, "command not supported on v1")
	c.Assert(result, jc.DeepEquals, "")
}
