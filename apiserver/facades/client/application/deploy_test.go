// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/charm"
)

type deploySuite struct {
	baseSuite
}

func TestDeploySuite(t *testing.T) {
	tc.Run(t, &deploySuite{})
}

func (s *deploySuite) TestDeployIAASApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	args := DeployApplicationParams{
		Charm: &domainCharm{
			charm: s.charm,
		},
		ApplicationName: "myapp",
	}
	s.charm.EXPECT().
		Meta().Return(&charm.Meta{}).AnyTimes()
	s.applicationService.EXPECT().
		CreateIAASApplication(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(tc.Must(c, application.NewUUID), nil)

	// Act
	err := DeployApplication(c.Context(),
		model.IAAS,
		s.applicationService,
		s.storageService,
		s.objectStore,
		args,
		nil,
		clock.WallClock,
	)
	// Assert
	c.Assert(err, tc.IsNil)
}

func (s *deploySuite) TestDeployCAASApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	args := DeployApplicationParams{
		Charm: &domainCharm{
			charm: s.charm,
		},
		ApplicationName: "myapp-caas",
	}
	s.charm.EXPECT().
		Meta().Return(&charm.Meta{}).AnyTimes()

	s.charm.EXPECT().
		Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Risk: charm.Stable,
				},
				Architectures: []string{"amd64"},
			},
		},
	}).AnyTimes()
	s.applicationService.EXPECT().
		CreateCAASApplication(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(tc.Must(c, application.NewUUID), nil)

	// Act
	err := DeployApplication(c.Context(),
		model.CAAS,
		s.applicationService,
		s.storageService,
		s.objectStore,
		args,
		nil,
		clock.WallClock,
	)
	// Assert
	c.Assert(err, tc.IsNil)
}
