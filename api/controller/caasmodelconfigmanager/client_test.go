// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager_test

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/caasmodelconfigmanager"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/rpc/params"
)

type caasmodelconfigmanagerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&caasmodelconfigmanagerSuite{})

func newClient(f basetesting.APICallerFunc) (*caasmodelconfigmanager.Client, error) {
	return caasmodelconfigmanager.NewClient(basetesting.BestVersionCaller{APICallerFunc: f, BestVersion: 1})
}

func (s *caasmodelconfigmanagerSuite) TestControllerConfig(c *gc.C) {
	client, err := newClient(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASModelConfigManager")
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ControllerConfig")
		c.Assert(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.ControllerConfigResult{})
		*(result.(*params.ControllerConfigResult)) = params.ControllerConfigResult{
			Config: params.ControllerConfig{
				"caas-image-repo": `
{
    "serveraddress": "quay.io",
    "auth": "xxxxx==",
    "repository": "test-account"
}
`[1:],
			},
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := client.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, jc.DeepEquals, controller.Config{
		"caas-image-repo": `
{
    "serveraddress": "quay.io",
    "auth": "xxxxx==",
    "repository": "test-account"
}
`[1:],
	})
}
