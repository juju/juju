// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v9"
	charmresource "github.com/juju/charm/v9/resource"
	"github.com/juju/clock"
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/gnuflag"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/v3/api/base"
	"github.com/juju/juju/v3/api/client/application"
	"github.com/juju/juju/v3/api/client/resources"
	commoncharm "github.com/juju/juju/v3/api/common/charm"
	"github.com/juju/juju/v3/api/common/charms"
	"github.com/juju/juju/v3/cmd/juju/application/deployer/mocks"
	"github.com/juju/juju/v3/cmd/modelcmd"
	"github.com/juju/juju/v3/environs/config"
	coretesting "github.com/juju/juju/v3/testing"
)

type charmSuite struct {
	deployerAPI  *mocks.MockDeployerAPI
	modelCommand *mocks.MockModelCommand
	configFlag   *mocks.MockDeployConfigFlag
	filesystem   *mocks.MockFilesystem
	resolver     *mocks.MockResolver

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
	s.modelCommand.EXPECT().BakeryClient().Return(nil, nil)
	s.modelCommand.EXPECT().Filesystem().Return(s.filesystem)
	s.configFlag.EXPECT().AbsoluteFileNames(gomock.Any()).Return(nil, nil)
	s.configFlag.EXPECT().ReadConfigPairs(gomock.Any()).Return(nil, nil)
	s.deployerAPI.EXPECT().Deploy(gomock.Any()).Return(nil)

	err := s.newDeployCharm().deploy(s.ctx, s.deployerAPI)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmSuite) TestRepositoryCharmDeployDryRun(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.resolver = mocks.NewMockResolver(ctrl)
	s.expectResolveChannel()
	s.expectDeployerAPIModelGet(c)

	dCharm := s.newDeployCharm()
	dCharm.dryRun = true
	dCharm.validateCharmSeriesWithName = func(series, name string, imageStream string) error {
		return nil
	}
	repoCharm := &repositoryCharm{
		deployCharm:      *dCharm,
		userRequestedURL: s.url,
		clock:            clock.WallClock,
	}

	err := repoCharm.PrepareAndDeploy(s.ctx, s.deployerAPI, s.resolver, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmSuite) newDeployCharm() *deployCharm {
	return &deployCharm{
		configOptions: s.configFlag,
		deployResources: func(
			string,
			resources.CharmID,
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
		flagSet:  &gnuflag.FlagSet{},
		model:    s.modelCommand,
		numUnits: 0,
		series:   "focal",
		steps:    []DeployStep{},
	}
}

func (s *charmSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.deployerAPI = mocks.NewMockDeployerAPI(ctrl)
	s.deployerAPI.EXPECT().CharmInfo(gomock.Any()).Return(s.charmInfo, nil).AnyTimes()
	s.deployerAPI.EXPECT().ModelUUID().Return("dead-beef", true).AnyTimes()

	s.modelCommand = mocks.NewMockModelCommand(ctrl)
	s.configFlag = mocks.NewMockDeployConfigFlag(ctrl)
	return ctrl
}

func (s *charmSuite) expectResolveChannel() {
	s.resolver.EXPECT().ResolveCharm(
		gomock.AssignableToTypeOf(&charm.URL{}),
		gomock.AssignableToTypeOf(commoncharm.Origin{}),
		false,
	).DoAndReturn(
		// Ensure the same curl that is provided, is returned.
		func(curl *charm.URL, requestedOrigin commoncharm.Origin, _ bool) (*charm.URL, commoncharm.Origin, []string, error) {
			return curl, requestedOrigin, []string{"bionic", "focal", "xenial"}, nil
		}).AnyTimes()
}

func (s *charmSuite) expectDeployerAPIModelGet(c *gc.C) {
	cfg, err := config.New(true, minimalModelConfig())
	c.Assert(err, jc.ErrorIsNil)
	s.deployerAPI.EXPECT().ModelGet().Return(cfg.AllAttrs(), nil)
}

func minimalModelConfig() map[string]interface{} {
	return map[string]interface{}{
		"name":            "test",
		"type":            "manual",
		"uuid":            coretesting.ModelTag.Id(),
		"controller-uuid": coretesting.ControllerTag.Id(),
		"firewall-mode":   "instance",
		// While the ca-cert bits aren't entirely minimal, they avoid the need
		// to set up a fake home.
		"ca-cert":        coretesting.CACert,
		"ca-private-key": coretesting.CAKey,
		"image-stream":   "testing",
	}
}
