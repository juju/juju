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
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/charm"
)

var (
	defaultBase = base.MustParseBaseFromString("22.04@ubuntu")
)

type IAASControllerSuite struct {
	baseSuite
}

func TestIAASControllerSuite(t *testing.T) {
	tc.Run(t, &IAASControllerSuite{})
}

func (s *IAASControllerSuite) TestPopulateIAASControllerCharmLocalCharm(c *tc.C) {
	defer s.setupMocks(c).Finish()

	origin := corecharm.Origin{
		Source: corecharm.Local,
		ID:     "deadbeef",
	}

	s.expectControllerAddress()
	s.expectCharmInfo()
	s.expectLocalDeployment(origin)
	s.expectAddApplication(origin)

	err := PopulateIAASControllerCharm(c.Context(), s.deployer)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *IAASControllerSuite) TestPopulateIAASControllerCharmLocalCharmFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectControllerAddress()
	s.expectCharmInfo()
	s.expectLocalCharmError()

	err := PopulateIAASControllerCharm(c.Context(), s.deployer)
	c.Assert(err, tc.ErrorMatches, `.*boom`)
}

func (s *IAASControllerSuite) TestPopulateIAASControllerCharmCharmhubCharm(c *tc.C) {
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

	err := PopulateIAASControllerCharm(c.Context(), s.deployer)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *IAASControllerSuite) TestPopulateControllerAlreadyExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	origin := corecharm.Origin{
		Source: corecharm.CharmHub,
		ID:     "deadbeef",
	}

	s.expectControllerAddress()
	s.expectCharmInfo()
	s.expectLocalCharmNotFound()
	s.expectCharmhubDeployment(origin)

	s.deployer.EXPECT().AddIAASControllerApplication(gomock.Any(), DeployCharmInfo{
		URL:    charm.MustParseURL("juju-controller"),
		Origin: &origin,
		Charm:  s.charm,
	}, "10.0.0.1").Return(applicationerrors.ApplicationAlreadyExists)

	err := PopulateIAASControllerCharm(c.Context(), s.deployer)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *IAASControllerSuite) expectControllerAddress() {
	s.deployer.EXPECT().ControllerAddress(gomock.Any()).Return("10.0.0.1", nil)
}

func (s *IAASControllerSuite) expectCharmInfo() {
	s.deployer.EXPECT().ControllerCharmArch().Return(arch.DefaultArchitecture)
	s.deployer.EXPECT().ControllerCharmBase().Return(defaultBase, nil)
}

func (s *IAASControllerSuite) expectLocalDeployment(origin corecharm.Origin) {
	s.deployer.EXPECT().DeployLocalCharm(gomock.Any(), arch.DefaultArchitecture, defaultBase).Return(DeployCharmInfo{
		URL:    charm.MustParseURL("juju-controller"),
		Origin: &origin,
		Charm:  s.charm,
	}, nil)
}

func (s *IAASControllerSuite) expectLocalCharmNotFound() {
	s.deployer.EXPECT().DeployLocalCharm(gomock.Any(), arch.DefaultArchitecture, defaultBase).Return(DeployCharmInfo{}, errors.NotFoundf("not found"))
}

func (s *IAASControllerSuite) expectLocalCharmError() {
	s.deployer.EXPECT().DeployLocalCharm(gomock.Any(), arch.DefaultArchitecture, defaultBase).Return(DeployCharmInfo{}, errors.Errorf("boom"))
}

func (s *IAASControllerSuite) expectCharmhubDeployment(origin corecharm.Origin) {
	s.deployer.EXPECT().DeployCharmhubCharm(gomock.Any(), arch.DefaultArchitecture, defaultBase).Return(DeployCharmInfo{
		URL:    charm.MustParseURL("juju-controller"),
		Origin: &origin,
		Charm:  s.charm,
	}, nil)
}

func (s *IAASControllerSuite) expectAddApplication(origin corecharm.Origin) {
	s.deployer.EXPECT().AddIAASControllerApplication(gomock.Any(), DeployCharmInfo{
		URL:    charm.MustParseURL("juju-controller"),
		Origin: &origin,
		Charm:  s.charm,
	}, "10.0.0.1").Return(nil)
}

type CAASControllerSuite struct {
	baseSuite
}

func TestCAASControllerSuite(t *testing.T) {
	tc.Run(t, &CAASControllerSuite{})
}

func (s *CAASControllerSuite) TestPopulateCAASControllerCharmLocalCharm(c *tc.C) {
	defer s.setupMocks(c).Finish()

	origin := corecharm.Origin{
		Source: corecharm.Local,
		ID:     "deadbeef",
	}

	s.expectControllerAddress()
	s.expectCharmInfo()
	s.expectLocalDeployment(origin)
	s.expectAddApplication(origin)
	s.expectCAASCompletion()

	err := PopulateCAASControllerCharm(c.Context(), s.deployer)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *CAASControllerSuite) TestPopulateCAASControllerCharmLocalCharmFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectControllerAddress()
	s.expectCharmInfo()
	s.expectLocalCharmError()

	err := PopulateCAASControllerCharm(c.Context(), s.deployer)
	c.Assert(err, tc.ErrorMatches, `.*boom`)
}

func (s *CAASControllerSuite) TestPopulateCAASControllerCharmCharmhubCharm(c *tc.C) {
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
	s.expectCAASCompletion()

	err := PopulateCAASControllerCharm(c.Context(), s.deployer)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *CAASControllerSuite) TestPopulateControllerAlreadyExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	origin := corecharm.Origin{
		Source: corecharm.CharmHub,
		ID:     "deadbeef",
	}

	s.expectControllerAddress()
	s.expectCharmInfo()
	s.expectLocalCharmNotFound()
	s.expectCharmhubDeployment(origin)

	s.deployer.EXPECT().AddCAASControllerApplication(gomock.Any(), DeployCharmInfo{
		URL:    charm.MustParseURL("juju-controller"),
		Origin: &origin,
		Charm:  s.charm,
	}, "10.0.0.1").Return(applicationerrors.ApplicationAlreadyExists)

	s.expectCAASCompletion()

	err := PopulateCAASControllerCharm(c.Context(), s.deployer)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *CAASControllerSuite) expectControllerAddress() {
	s.deployer.EXPECT().ControllerAddress(gomock.Any()).Return("10.0.0.1", nil)
}

func (s *CAASControllerSuite) expectCharmInfo() {
	s.deployer.EXPECT().ControllerCharmArch().Return(arch.DefaultArchitecture)
	s.deployer.EXPECT().ControllerCharmBase().Return(defaultBase, nil)
}

func (s *CAASControllerSuite) expectLocalDeployment(origin corecharm.Origin) {
	s.deployer.EXPECT().DeployLocalCharm(gomock.Any(), arch.DefaultArchitecture, defaultBase).Return(DeployCharmInfo{
		URL:    charm.MustParseURL("juju-controller"),
		Origin: &origin,
		Charm:  s.charm,
	}, nil)
}

func (s *CAASControllerSuite) expectLocalCharmNotFound() {
	s.deployer.EXPECT().DeployLocalCharm(gomock.Any(), arch.DefaultArchitecture, defaultBase).Return(DeployCharmInfo{}, errors.NotFoundf("not found"))
}

func (s *CAASControllerSuite) expectLocalCharmError() {
	s.deployer.EXPECT().DeployLocalCharm(gomock.Any(), arch.DefaultArchitecture, defaultBase).Return(DeployCharmInfo{}, errors.Errorf("boom"))
}

func (s *CAASControllerSuite) expectCharmhubDeployment(origin corecharm.Origin) {
	s.deployer.EXPECT().DeployCharmhubCharm(gomock.Any(), arch.DefaultArchitecture, defaultBase).Return(DeployCharmInfo{
		URL:    charm.MustParseURL("juju-controller"),
		Origin: &origin,
		Charm:  s.charm,
	}, nil)
}

func (s *CAASControllerSuite) expectAddApplication(origin corecharm.Origin) {
	s.deployer.EXPECT().AddCAASControllerApplication(gomock.Any(), DeployCharmInfo{
		URL:    charm.MustParseURL("juju-controller"),
		Origin: &origin,
		Charm:  s.charm,
	}, "10.0.0.1").Return(nil)
}

func (s *CAASControllerSuite) expectCAASCompletion() {
	s.deployer.EXPECT().CompleteCAASProcess(gomock.Any()).Return(nil)
}
