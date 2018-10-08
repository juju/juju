// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"os"

	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/featureflag"
	gc "gopkg.in/check.v1"
	charm "gopkg.in/juju/charm.v6"

	apicharms "github.com/juju/juju/api/charms"
	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/cmd/juju/application/mocks"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju/osenv"
)

type ValidateLXDProfileCharmSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ValidateLXDProfileCharmSuite{})

func (s *ValidateLXDProfileCharmSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	err := os.Setenv(osenv.JujuFeatureFlagEnvKey, feature.LXDProfile)
	c.Assert(err, jc.ErrorIsNil)
	defer os.Unsetenv(osenv.JujuFeatureFlagEnvKey)
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}

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
