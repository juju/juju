// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/provisioner"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/tags"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

func (s *withoutControllerSuite) TestStubSuite(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- ProvisioningInfo for endpoint bindings to multiple spaces.
  In particular the EndpointBindings and ProvisioningNetworkTopology should be
  populated and included relevant space, subnet and zone data.
- ProvisioningInfo where there are endpoint bindings, but the alpha space has no
  subnets/zones, which causes an error saying it cannot be used as a deployment
  target due to having no subnets.
- ProvisioningInfo where a negative space constraint conflicts with endpoint
  bindings causing an error to be returned.
- ProvisioningInfo where the alpha space is explicitly set for all bindings and
  as a constraint, causing ProvisioningNetworkTopology to have only subnets and
  zones from the alpha space.
- TestProvisioningInfoWithStorage: Check ProvisioningInfo for includes volume
  attachment information
- TestProvisioningInfoRootDiskVolume: Check ProvisioningInfo for includes root
  disk information
- TestProvisioningInfoWithMultiplePositiveSpaceConstraints: Check ProvisioningInfo
  includes network topology information
- TestProvisioningInfoWithUnsuitableSpacesConstraints: Check ProvisioningInfo
  returns errors for spaces constraints that are not suitable for deployment
  target
- TestStorageProviderFallbackToType: Check ProvisioningInfo for includes volume
  attachment information
- TestStorageProviderVolumes: Check ProvisioningInfo for includes volume
  attachment information
- TestProviderInfoCloudInitUserData: Check ProvisioningInfo for includes cloud
  init user data
`)
}

func (s *withoutControllerSuite) TestProvisioningInfoPermissions(c *tc.C) {
	domainServices := s.ControllerDomainServices(c)

	// Login as a machine agent for machine 0.
	anAuthorizer := s.authorizer
	anAuthorizer.Controller = false
	anAuthorizer.Tag = s.machines[0].Tag()
	aProvisioner, err := provisioner.MakeProvisionerAPI(c.Context(), facadetest.ModelContext{
		Auth_:           anAuthorizer,
		State_:          s.ControllerModel(c).State(),
		StatePool_:      s.StatePool(),
		Resources_:      s.resources,
		DomainServices_: domainServices,
		Logger_:         loggertesting.WrapCheckLog(c),
		ControllerUUID_: coretesting.ControllerTag.Id(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(aProvisioner, tc.NotNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[0].Tag().String() + "-lxd-0"},
		{Tag: "machine-42"},
		{Tag: s.machines[1].Tag().String()},
		{Tag: "application-bar"},
	}}

	// Only machine 0 and containers therein can be accessed.
	results, err := aProvisioner.ProvisioningInfo(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)

	controllerCfg, err := domainServices.ControllerConfig().ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(results, tc.DeepEquals, params.ProvisioningInfoResults{
		Results: []params.ProvisioningInfoResult{
			{Result: &params.ProvisioningInfo{
				ControllerConfig: controllerCfg,
				Base:             params.Base{Name: "ubuntu", Channel: "12.10/stable"},
				Jobs:             []model.MachineJob{model.JobHostUnits},
				Tags: map[string]string{
					tags.JujuController: coretesting.ControllerTag.Id(),
					tags.JujuModel:      coretesting.ModelTag.Id(),
					tags.JujuMachine:    "controller-machine-0",
				},
				EndpointBindings: make(map[string]string),
			},
			},
			{Error: apiservertesting.NotFoundError("machine 0/lxd/0")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}
