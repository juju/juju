// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/testing"
)

func (s *environSuite) TestSupportsInstanceRole(c *gc.C) {
	env, ok := s.openEnviron(c).(environs.InstanceRole)
	c.Assert(ok, jc.IsTrue)
	c.Assert(env.SupportsInstanceRoles(s.callCtx), jc.IsTrue)
}

func (s *environSuite) TestCreateAutoInstanceRole(c *gc.C) {
	env, ok := s.openEnviron(c).(environs.InstanceRole)
	c.Assert(ok, jc.IsTrue)

	s.sender = s.initResourceGroupSenders(resourceGroupName)

	deployments := []*armresources.DeploymentExtended{{
		Name: to.Ptr("identity"),
		Properties: &armresources.DeploymentPropertiesExtended{
			ProvisioningState: to.Ptr(armresources.ProvisioningStateSucceeded),
		},
	}}
	s.sender = append(s.sender,
		makeSender("/deployments", armresources.DeploymentListResult{Value: deployments}),
	)
	p := environs.BootstrapParams{
		ControllerConfig: map[string]interface{}{
			controller.ControllerUUIDKey: testing.ControllerTag.Id(),
		},
	}
	res, err := env.CreateAutoInstanceRole(s.callCtx, p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.Equals, "juju-controller-"+testing.ControllerTag.Id())
}
