// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"github.com/golang/mock/gomock"
	charm "github.com/juju/charm/v7"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	apicharms "github.com/juju/juju/api/charms"
	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/cmd/juju/application/mocks"
)

type ValidateLXDProfileCharmSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ValidateLXDProfileCharmSuite{})

func (*ValidateLXDProfileCharmSuite) TestRunPreWithNoLXDProfile(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deployInfo := application.DeploymentInfo{
		CharmInfo: &apicharms.CharmInfo{},
	}

	mockDeployStepAPI := mocks.NewMockDeployStepAPI(ctrl)

	validate := &application.ValidateLXDProfileCharm{}
	err := validate.RunPre(mockDeployStepAPI, nil, nil, deployInfo)
	c.Assert(err, gc.IsNil)
}

func (*ValidateLXDProfileCharmSuite) TestRunPreWithLXDProfile(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deployInfo := application.DeploymentInfo{
		CharmInfo: &apicharms.CharmInfo{
			LXDProfile: &charm.LXDProfile{
				Config: map[string]string{
					"security.nesting": "true",
				},
			},
		},
	}

	mockDeployStepAPI := mocks.NewMockDeployStepAPI(ctrl)

	validate := &application.ValidateLXDProfileCharm{}
	err := validate.RunPre(mockDeployStepAPI, nil, nil, deployInfo)
	c.Assert(err, gc.IsNil)
}

func (*ValidateLXDProfileCharmSuite) TestRunPreWithInvalidLXDProfile(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deployInfo := application.DeploymentInfo{
		CharmInfo: &apicharms.CharmInfo{
			LXDProfile: &charm.LXDProfile{
				Config: map[string]string{
					"boot.autostart": "true",
				},
			},
		},
	}

	mockDeployStepAPI := mocks.NewMockDeployStepAPI(ctrl)

	validate := &application.ValidateLXDProfileCharm{}
	err := validate.RunPre(mockDeployStepAPI, nil, nil, deployInfo)
	c.Assert(err, gc.ErrorMatches, "invalid lxd-profile.yaml: contains config value \"boot.autostart\"")
}

func (*ValidateLXDProfileCharmSuite) TestRunPreWithNoLXDProfileAndForce(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deployInfo := application.DeploymentInfo{
		CharmInfo: &apicharms.CharmInfo{},
		Force:     true,
	}

	mockDeployStepAPI := mocks.NewMockDeployStepAPI(ctrl)

	validate := &application.ValidateLXDProfileCharm{}
	err := validate.RunPre(mockDeployStepAPI, nil, nil, deployInfo)
	c.Assert(err, gc.IsNil)
}

func (*ValidateLXDProfileCharmSuite) TestRunPreWithLXDProfileAndForce(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deployInfo := application.DeploymentInfo{
		CharmInfo: &apicharms.CharmInfo{
			LXDProfile: &charm.LXDProfile{
				Config: map[string]string{
					"security.nesting": "true",
				},
			},
		},
		Force: true,
	}

	mockDeployStepAPI := mocks.NewMockDeployStepAPI(ctrl)

	validate := &application.ValidateLXDProfileCharm{}
	err := validate.RunPre(mockDeployStepAPI, nil, nil, deployInfo)
	c.Assert(err, gc.IsNil)
}

func (*ValidateLXDProfileCharmSuite) TestRunPreWithInvalidLXDProfileAndForce(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deployInfo := application.DeploymentInfo{
		CharmInfo: &apicharms.CharmInfo{
			LXDProfile: &charm.LXDProfile{
				Config: map[string]string{
					"boot.autostart": "true",
				},
			},
		},
		Force: true,
	}

	mockDeployStepAPI := mocks.NewMockDeployStepAPI(ctrl)

	validate := &application.ValidateLXDProfileCharm{}
	err := validate.RunPre(mockDeployStepAPI, nil, nil, deployInfo)
	c.Assert(err, gc.IsNil)
}
