// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	"github.com/juju/charm/v11"
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apicommoncharms "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/cmd/juju/application/deployer"
	"github.com/juju/juju/cmd/juju/application/deployer/mocks"
)

type ValidateLXDProfileCharmSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ValidateLXDProfileCharmSuite{})

func (*ValidateLXDProfileCharmSuite) TestRunPreWithNoLXDProfile(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deployInfo := deployer.DeploymentInfo{
		CharmInfo: &apicommoncharms.CharmInfo{},
	}

	mockDeployStepAPI := mocks.NewMockDeployStepAPI(ctrl)

	validate := &deployer.ValidateLXDProfileCharm{}
	err := validate.RunPre(mockDeployStepAPI, nil, nil, deployInfo)
	c.Assert(err, gc.IsNil)
}

func (*ValidateLXDProfileCharmSuite) TestRunPreWithLXDProfile(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deployInfo := deployer.DeploymentInfo{
		CharmInfo: &apicommoncharms.CharmInfo{
			LXDProfile: &charm.LXDProfile{
				Config: map[string]string{
					"security.nesting": "true",
				},
			},
		},
	}

	mockDeployStepAPI := mocks.NewMockDeployStepAPI(ctrl)

	validate := &deployer.ValidateLXDProfileCharm{}
	err := validate.RunPre(mockDeployStepAPI, nil, nil, deployInfo)
	c.Assert(err, gc.IsNil)
}

func (*ValidateLXDProfileCharmSuite) TestRunPreWithInvalidLXDProfile(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deployInfo := deployer.DeploymentInfo{
		CharmInfo: &apicommoncharms.CharmInfo{
			LXDProfile: &charm.LXDProfile{
				Config: map[string]string{
					"boot.autostart": "true",
				},
			},
		},
	}

	mockDeployStepAPI := mocks.NewMockDeployStepAPI(ctrl)

	validate := &deployer.ValidateLXDProfileCharm{}
	err := validate.RunPre(mockDeployStepAPI, nil, nil, deployInfo)
	c.Assert(err, gc.ErrorMatches, "invalid lxd-profile.yaml: contains config value \"boot.autostart\"")
}

func (*ValidateLXDProfileCharmSuite) TestRunPreWithNoLXDProfileAndForce(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deployInfo := deployer.DeploymentInfo{
		CharmInfo: &apicommoncharms.CharmInfo{},
		Force:     true,
	}

	mockDeployStepAPI := mocks.NewMockDeployStepAPI(ctrl)

	validate := &deployer.ValidateLXDProfileCharm{}
	err := validate.RunPre(mockDeployStepAPI, nil, nil, deployInfo)
	c.Assert(err, gc.IsNil)
}

func (*ValidateLXDProfileCharmSuite) TestRunPreWithLXDProfileAndForce(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deployInfo := deployer.DeploymentInfo{
		CharmInfo: &apicommoncharms.CharmInfo{
			LXDProfile: &charm.LXDProfile{
				Config: map[string]string{
					"security.nesting": "true",
				},
			},
		},
		Force: true,
	}

	mockDeployStepAPI := mocks.NewMockDeployStepAPI(ctrl)

	validate := &deployer.ValidateLXDProfileCharm{}
	err := validate.RunPre(mockDeployStepAPI, nil, nil, deployInfo)
	c.Assert(err, gc.IsNil)
}

func (*ValidateLXDProfileCharmSuite) TestRunPreWithInvalidLXDProfileAndForce(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deployInfo := deployer.DeploymentInfo{
		CharmInfo: &apicommoncharms.CharmInfo{
			LXDProfile: &charm.LXDProfile{
				Config: map[string]string{
					"boot.autostart": "true",
				},
			},
		},
		Force: true,
	}

	mockDeployStepAPI := mocks.NewMockDeployStepAPI(ctrl)

	validate := &deployer.ValidateLXDProfileCharm{}
	err := validate.RunPre(mockDeployStepAPI, nil, nil, deployInfo)
	c.Assert(err, gc.IsNil)
}
