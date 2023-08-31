// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"bytes"

	"github.com/juju/charm/v11"
	charmresource "github.com/juju/charm/v11/resource"
	"github.com/juju/clock"
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/gnuflag"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/application"
	"github.com/juju/juju/api/client/resources"
	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/cmd/juju/application/deployer/mocks"
	"github.com/juju/juju/cmd/modelcmd"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/testing"
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
	s.modelCommand.EXPECT().Filesystem().Return(s.filesystem).AnyTimes()
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
	s.expectDeployerAPIModelGet(c, corebase.Base{})

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

	err := repoCharm.PrepareAndDeploy(s.ctx, s.deployerAPI, s.resolver)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmSuite) TestRepositoryCharmDeployDryRunDefaultSeriesForce(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.resolver = mocks.NewMockResolver(ctrl)
	s.expectResolveChannel()
	s.expectDeployerAPIModelGet(c, corebase.MustParseBaseFromString("ubuntu@22.04"))

	dCharm := s.newDeployCharm()
	dCharm.dryRun = true
	dCharm.force = true
	dCharm.validateCharmSeriesWithName = func(series, name string, imageStream string) error {
		return nil
	}
	repoCharm := &repositoryCharm{
		deployCharm:      *dCharm,
		userRequestedURL: s.url,
		clock:            clock.WallClock,
	}

	stdOut := mocks.NewMockWriter(ctrl)
	stdErr := mocks.NewMockWriter(ctrl)
	output := bytes.NewBuffer([]byte{})
	logOutput := func(p []byte) {
		c.Logf("%q", p)
		output.Write(p)
	}
	stdOut.EXPECT().Write(gomock.Any()).Return(0, nil).AnyTimes().Do(logOutput)
	stdErr.EXPECT().Write(gomock.Any()).Return(0, nil).AnyTimes().Do(logOutput)

	ctx := &cmd.Context{
		Stderr: stdErr,
		Stdout: stdOut,
	}

	err := repoCharm.PrepareAndDeploy(ctx, s.deployerAPI, s.resolver)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(output.String(), gc.Equals, "\"testme\" from  charm \"testme\", revision -1 on ubuntu@22.04 would be deployed\n")
}

func (s *charmSuite) newDeployCharm() *deployCharm {
	return &deployCharm{
		configOptions: s.configFlag,
		deployResources: func(
			string,
			resources.CharmID,
			map[string]string,
			map[string]charmresource.Meta,
			base.APICallCloser,
			modelcmd.Filesystem,
		) (ids map[string]string, err error) {
			return s.deployResourceIDs, nil
		},
		id: application.CharmID{
			URL:    s.url,
			Origin: commoncharm.Origin{Base: corebase.MakeDefaultBase("ubuntu", "20.04")},
		},
		flagSet:  &gnuflag.FlagSet{},
		model:    s.modelCommand,
		numUnits: 0,
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

func (s *charmSuite) expectDeployerAPIModelGet(c *gc.C, defaultBase corebase.Base) {
	cfg, err := config.New(true, minimalModelConfig())
	c.Assert(err, jc.ErrorIsNil)
	attrs := cfg.AllAttrs()
	attrs["default-base"] = defaultBase.String()
	s.deployerAPI.EXPECT().ModelGet().Return(attrs, nil)
}

func minimalModelConfig() map[string]interface{} {
	return map[string]interface{}{
		"name":            "test",
		"type":            "manual",
		"uuid":            coretesting.ModelTag.Id(),
		"controller-uuid": coretesting.ControllerTag.Id(),
		"firewall-mode":   "instance",
		"secret-backend":  "auto",
		// While the ca-cert bits aren't entirely minimal, they avoid the need
		// to set up a fake home.
		"ca-cert":        coretesting.CACert,
		"ca-private-key": coretesting.CAKey,
		"image-stream":   "testing",
	}
}
