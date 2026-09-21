// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"testing"

	gomock "github.com/canonical/gomock/gomock"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/domain/deployment/charm"
)

var (
	defaultBase = base.MustParseBaseFromString("22.04@ubuntu")
)

type ControllerSuite struct {
	baseSuite
}

func TestControllerSuite(t *testing.T) {
	tc.Run(t, &ControllerSuite{})
}

func (s *ControllerSuite) TestPopulateControllerCharmLocalCharm(c *tc.C) {
	defer s.setupMocks(c).Finish()

	origin := corecharm.Origin{
		Source: corecharm.Local,
		ID:     "deadbeef",
	}

	s.expectCharmInfo()
	s.expectLocalDeployment(origin)
	s.expectEnsureApplication(origin)

	err := PopulateControllerCharm(c.Context(), s.deployer)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ControllerSuite) TestPopulateControllerCharmLocalCharmFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectCharmInfo()
	s.expectLocalCharmError()

	err := PopulateControllerCharm(c.Context(), s.deployer)
	c.Assert(err, tc.ErrorMatches, `.*boom`)
}

func (s *ControllerSuite) TestPopulateControllerCharmCharmhubCharm(c *tc.C) {
	defer s.setupMocks(c).Finish()

	origin := corecharm.Origin{
		Source: corecharm.CharmHub,
		ID:     "deadbeef",
	}

	s.expectCharmInfo()
	s.expectLocalCharmNotFound()
	s.expectCharmhubDeployment(origin)
	s.expectEnsureApplication(origin)

	err := PopulateControllerCharm(c.Context(), s.deployer)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ControllerSuite) TestPopulateControllerApplicationFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	origin := corecharm.Origin{
		Source: corecharm.CharmHub,
		ID:     "deadbeef",
	}

	s.expectCharmInfo()
	s.expectLocalCharmNotFound()
	s.expectCharmhubDeployment(origin)

	expectedErr := errors.New("cannot complete controller application")
	s.deployer.EXPECT().EnsureControllerApplication(gomock.Any(), DeployCharmInfo{
		URL:    charm.MustParseURL("juju-controller"),
		Origin: &origin,
		Charm:  s.charm,
	}).Return(expectedErr)
	err := PopulateControllerCharm(c.Context(), s.deployer)
	c.Assert(err, tc.ErrorIs, expectedErr)
}

func (s *ControllerSuite) expectCharmInfo() {
	s.deployer.EXPECT().ControllerCharmArch().Return(arch.DefaultArchitecture)
	s.deployer.EXPECT().ControllerCharmBase().Return(defaultBase, nil)
}

func (s *ControllerSuite) expectLocalDeployment(origin corecharm.Origin) {
	s.deployer.EXPECT().DeployLocalCharm(gomock.Any(), arch.DefaultArchitecture, defaultBase).Return(DeployCharmInfo{
		URL:    charm.MustParseURL("juju-controller"),
		Origin: &origin,
		Charm:  s.charm,
	}, nil)
}

func (s *ControllerSuite) expectLocalCharmNotFound() {
	s.deployer.EXPECT().DeployLocalCharm(gomock.Any(), arch.DefaultArchitecture, defaultBase).Return(DeployCharmInfo{}, errors.NotFoundf("not found"))
}

func (s *ControllerSuite) expectLocalCharmError() {
	s.deployer.EXPECT().DeployLocalCharm(gomock.Any(), arch.DefaultArchitecture, defaultBase).Return(DeployCharmInfo{}, errors.Errorf("boom"))
}

func (s *ControllerSuite) expectCharmhubDeployment(origin corecharm.Origin) {
	s.deployer.EXPECT().DeployCharmhubCharm(gomock.Any(), arch.DefaultArchitecture, defaultBase).Return(DeployCharmInfo{
		URL:    charm.MustParseURL("juju-controller"),
		Origin: &origin,
		Charm:  s.charm,
	}, nil)
}

func (s *ControllerSuite) expectEnsureApplication(origin corecharm.Origin) {
	s.deployer.EXPECT().EnsureControllerApplication(gomock.Any(), DeployCharmInfo{
		URL:    charm.MustParseURL("juju-controller"),
		Origin: &origin,
		Charm:  s.charm,
	}).Return(nil)
}
