// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/caasmodelconfigmanager"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type caasmodelconfigmanagerSuite struct {
	testhelpers.IsolationSuite
}

func TestCaasmodelconfigmanagerSuite(t *stdtesting.T) {
	tc.Run(t, &caasmodelconfigmanagerSuite{})
}
func newClient(f basetesting.APICallerFunc) (*caasmodelconfigmanager.Client, error) {
	return caasmodelconfigmanager.NewClient(basetesting.BestVersionCaller{APICallerFunc: f, BestVersion: 1})
}

func (s *caasmodelconfigmanagerSuite) TestControllerConfig(c *tc.C) {
	client, err := newClient(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASModelConfigManager")
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "ControllerConfig")
		c.Assert(arg, tc.IsNil)
		c.Assert(result, tc.FitsTypeOf, &params.ControllerConfigResult{})
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
	c.Assert(err, tc.ErrorIsNil)

	cfg, err := client.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg, tc.DeepEquals, controller.Config{
		"caas-image-repo": `
{
    "serveraddress": "quay.io",
    "auth": "xxxxx==",
    "repository": "test-account"
}
`[1:],
	})
}
