// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/charm"
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

	s.expectControllerAddress()
	s.expectCharmInfo()
	s.expectLocalDeployment(origin)
	s.expectAddApplication(origin)
	s.expectCompletion()

	err := PopulateControllerCharm(c.Context(), s.deployer)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ControllerSuite) TestPopulateControllerCharmLocalCharmFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectControllerAddress()
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

	s.expectControllerAddress()
	s.expectCharmInfo()
	s.expectLocalCharmNotFound()
	s.expectCharmhubDeployment(origin)
	s.expectAddApplication(origin)
	s.expectCompletion()

	err := PopulateControllerCharm(c.Context(), s.deployer)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ControllerSuite) TestPopulateControllerAlreadyExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	origin := corecharm.Origin{
		Source: corecharm.CharmHub,
		ID:     "deadbeef",
	}

	s.expectControllerAddress()
	s.expectCharmInfo()
	s.expectLocalCharmNotFound()
	s.expectCharmhubDeployment(origin)
	s.deployer.EXPECT().AddControllerApplication(gomock.Any(), DeployCharmInfo{
		URL:    charm.MustParseURL("juju-controller"),
		Origin: &origin,
		Charm:  s.charm,
	}, "10.0.0.1").Return(coreunit.Name("controller/0"), applicationerrors.ApplicationAlreadyExists)
	s.expectCompletion()

	err := PopulateControllerCharm(c.Context(), s.deployer)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ControllerSuite) expectControllerAddress() {
	s.deployer.EXPECT().ControllerAddress(gomock.Any()).Return("10.0.0.1", nil)
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

func (s *ControllerSuite) expectAddApplication(origin corecharm.Origin) {
	s.deployer.EXPECT().AddControllerApplication(gomock.Any(), DeployCharmInfo{
		URL:    charm.MustParseURL("juju-controller"),
		Origin: &origin,
		Charm:  s.charm,
	}, "10.0.0.1").Return(coreunit.Name("controller/0"), nil)
}

func (s *ControllerSuite) expectCompletion() {
	s.deployer.EXPECT().CompleteProcess(gomock.Any(), coreunit.Name("controller/0")).Return(nil)
}
