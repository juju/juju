// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v9"
	charmresource "github.com/juju/charm/v9/resource"
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/base"
	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/api/resources/client"
	"github.com/juju/juju/cmd/juju/application/deployer/mocks"
	"github.com/juju/juju/cmd/modelcmd"
)

type charmSuite struct {
	deployerAPI  *mocks.MockDeployerAPI
	modelCommand *mocks.MockModelCommand
	configFlag   *mocks.MockDeployConfigFlag
	filesystem   *mocks.MockFilesystem

	ctx               *cmd.Context
	deployResourceIDs map[string]string
	charmInfo         *charms.CharmInfo
	url               *charm.URL
}

var _ = gc.Suite(&charmSuite{})

func (s *charmSuite) SetUpTest(c *gc.C) {
	s.ctx = cmdtesting.Context(c)
	s.deployResourceIDs = make(map[string]string)
	s.url = charm.MustParseURL("testme")
	s.charmInfo = &charms.CharmInfo{
		Revision: 7,
		URL:      s.url.WithRevision(7).String(),
		Meta: &charm.Meta{
			Name: s.url.Name,
		},
	}
}

func (s *charmSuite) TestSimpleCharmDeploy(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.deployerAPI.EXPECT().Deploy(gomock.Any()).Return(nil)

	err := s.newDeployCharm().deploy(s.ctx, s.deployerAPI)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmSuite) TestCharmWithRevisionResourcesDeploy(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.deployerAPI.EXPECT().Deploy(gomock.Any()).Return(nil)

	s.charmInfo.Meta.Resources = map[string]charmresource.Meta{
		"resource-foo": {Type: charmresource.TypeFile},
	}
	newDeployCharm := s.newDeployCharm()
	newDeployCharm.resources = map[string]string{"resource-foo": "3"}
	newDeployCharm.revision = 7

	err := newDeployCharm.deploy(s.ctx, s.deployerAPI)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmSuite) TestCharmWithRevisionResourcesDeployIgnoreContainerImage(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.deployerAPI.EXPECT().Deploy(gomock.Any()).Return(nil)

	s.charmInfo.Meta.Resources = map[string]charmresource.Meta{
		"resource-foo": {Type: charmresource.TypeContainerImage},
	}
	newDeployCharm := s.newDeployCharm()
	newDeployCharm.revision = 7

	err := newDeployCharm.deploy(s.ctx, s.deployerAPI)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmSuite) TestStoreCharmWithRevisionResourcesDeploy(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.deployerAPI.EXPECT().Deploy(gomock.Any()).Return(nil)

	s.charmInfo.Meta.Resources = map[string]charmresource.Meta{
		"resource-foo": {Type: charmresource.TypeFile},
	}
	curl := charm.MustParseURL("cs:ubuntu-7")
	s.charmInfo.URL = curl.String()
	newDeployCharm := s.newDeployCharm()
	newDeployCharm.revision = 7

	newDeployCharm.id.URL = curl

	err := newDeployCharm.deploy(s.ctx, s.deployerAPI)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmSuite) TestCharmWithRevisionResourcesDeployFail(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.deployerAPI = mocks.NewMockDeployerAPI(ctrl)

	s.charmInfo.Meta.Resources = map[string]charmresource.Meta{
		"resource-foo": {Name: "resource-foo"},
	}
	s.deployerAPI.EXPECT().CharmInfo(gomock.Any()).Return(s.charmInfo, nil)

	newDeployCharm := s.newDeployCharm()
	newDeployCharm.revision = 7

	err := newDeployCharm.deploy(s.ctx, s.deployerAPI)
	c.Assert(err, jc.Satisfies, errors.IsBadRequest)
}

func (s *charmSuite) newDeployCharm() *deployCharm {
	return &deployCharm{
		configOptions: s.configFlag,
		deployResources: func(
			string,
			client.CharmID,
			*macaroon.Macaroon,
			map[string]string,
			map[string]charmresource.Meta,
			base.APICallCloser,
			modelcmd.Filesystem,
		) (ids map[string]string, err error) {
			return s.deployResourceIDs, nil
		},
		id: application.CharmID{
			URL:    s.url,
			Origin: commoncharm.Origin{},
		},
		flagSet:  nil,
		model:    s.modelCommand,
		numUnits: 0,
		origin:   commoncharm.Origin{},
		revision: -1,
		series:   "focal",
		steps:    []DeployStep{},
	}
}

func (s *charmSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.deployerAPI = mocks.NewMockDeployerAPI(ctrl)
	s.deployerAPI.EXPECT().BestFacadeVersion("Application").Return(7).AnyTimes()
	s.deployerAPI.EXPECT().CharmInfo(gomock.Any()).Return(s.charmInfo, nil)
	s.deployerAPI.EXPECT().ModelUUID().Return("dead-beef", true)

	s.modelCommand = mocks.NewMockModelCommand(ctrl)
	s.modelCommand.EXPECT().BakeryClient().Return(nil, nil)
	s.modelCommand.EXPECT().Filesystem().Return(s.filesystem)

	// Except to charm config.
	s.configFlag = mocks.NewMockDeployConfigFlag(ctrl)
	s.configFlag.EXPECT().AbsoluteFileNames(gomock.Any()).Return(nil, nil)
	s.configFlag.EXPECT().ReadConfigPairs(gomock.Any()).Return(nil, nil)

	return ctrl
}
