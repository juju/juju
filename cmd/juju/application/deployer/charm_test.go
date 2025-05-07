// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"bytes"
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/application"
	"github.com/juju/juju/api/client/resources"
	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/cmd/juju/application/deployer/mocks"
	"github.com/juju/juju/cmd/modelcmd"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	coretesting "github.com/juju/juju/internal/testing"
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

var _ = tc.Suite(&charmSuite{})

func (s *charmSuite) SetUpTest(c *tc.C) {
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

func (s *charmSuite) TestSimpleCharmDeploy(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.modelCommand.EXPECT().Filesystem().Return(s.filesystem).AnyTimes()
	s.configFlag.EXPECT().AbsoluteFileNames(gomock.Any()).Return(nil, nil)
	s.configFlag.EXPECT().ReadConfigPairs(gomock.Any()).Return(nil, nil)
	s.deployerAPI.EXPECT().Deploy(gomock.Any(), gomock.Any()).Return(nil)

	err := s.newDeployCharm().deploy(s.ctx, s.deployerAPI)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *charmSuite) TestRepositoryCharmDeployDryRunDefaultSeriesForce(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.modelCommand.EXPECT().Filesystem().Return(s.filesystem).AnyTimes()
	s.configFlag.EXPECT().AbsoluteFileNames(gomock.Any()).Return(nil, nil)
	s.configFlag.EXPECT().ReadConfigPairs(gomock.Any()).Return(nil, nil)
	s.resolver = mocks.NewMockResolver(ctrl)
	s.expectResolveChannel()

	dCharm := s.newDeployCharm()
	dCharm.dryRun = true
	dCharm.force = true
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

	dInfo := application.DeployInfo{
		Name:     "testme",
		Revision: 1,
		Channel:  "latest/stable",
		Base: corebase.Base{Channel: corebase.Channel{Track: "20.04"},
			OS: "ubuntu"},
	}

	repoCharm.uploadExistingPendingResources = func(_ context.Context, appName string, pendingResources []application.PendingResourceUpload, conn base.APICallCloser, filesystem modelcmd.Filesystem) error {
		c.Assert(appName, tc.Equals, dInfo.Name)
		return nil
	}

	s.deployerAPI.EXPECT().DeployFromRepository(gomock.Any(), gomock.Any()).Return(dInfo, nil, nil)

	err := repoCharm.PrepareAndDeploy(ctx, s.deployerAPI, s.resolver)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(output.String(), tc.Equals, "\"testme\" from charm-hub charm \"testme\", revision 1 in channel latest/stable on ubuntu@20.04 would be deployed\n")
}

func (s *charmSuite) TestDeployFromRepositoryCharmAppNameVSCharmName(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.modelCommand.EXPECT().Filesystem().Return(s.filesystem).AnyTimes()
	s.configFlag.EXPECT().AbsoluteFileNames(gomock.Any()).Return(nil, nil)
	s.configFlag.EXPECT().ReadConfigPairs(gomock.Any()).Return(nil, nil)

	dCharm := s.newDeployCharm()
	dCharm.applicationName = "differentThanCharmName"

	repoCharm := &repositoryCharm{
		deployCharm:      *dCharm,
		userRequestedURL: s.url,
		clock:            clock.WallClock,
	}

	stdErr := mocks.NewMockWriter(ctrl)
	output := bytes.NewBuffer([]byte{})
	logOutput := func(p []byte) {
		c.Logf("%q", p)
		output.Write(p)
	}
	stdErr.EXPECT().Write(gomock.Any()).Return(0, nil).AnyTimes().Do(logOutput)

	ctx := &cmd.Context{
		Stderr: stdErr,
	}

	dInfo := application.DeployInfo{
		Name:     dCharm.applicationName,
		Revision: 1,
		Channel:  "latest/stable",
		Base: corebase.Base{Channel: corebase.Channel{Track: "20.04"},
			OS: "ubuntu"},
	}

	repoCharm.uploadExistingPendingResources = func(_ context.Context, appName string, pendingResources []application.PendingResourceUpload, conn base.APICallCloser, filesystem modelcmd.Filesystem) error {
		c.Assert(appName, tc.Equals, dInfo.Name)
		return nil
	}

	s.deployerAPI.EXPECT().DeployFromRepository(gomock.Any(), gomock.Any()).Return(dInfo, nil, nil)

	err := repoCharm.PrepareAndDeploy(ctx, s.deployerAPI, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(output.String(), tc.Equals,
		"Deployed \"differentThanCharmName\" from charm-hub charm \"testme\", "+
			"revision 1 in channel latest/stable on ubuntu@20.04\n")
}

func (s *charmSuite) TestDeployFromRepositoryErrorNoUploadResources(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.modelCommand.EXPECT().Filesystem().Return(s.filesystem).AnyTimes()
	s.configFlag.EXPECT().AbsoluteFileNames(gomock.Any()).Return(nil, nil)
	s.configFlag.EXPECT().ReadConfigPairs(gomock.Any()).Return(nil, nil)

	dCharm := s.newDeployCharm()

	repoCharm := &repositoryCharm{
		deployCharm:      *dCharm,
		userRequestedURL: s.url,
		clock:            clock.WallClock,
	}

	writer := mocks.NewMockWriter(ctrl)
	writer.EXPECT().Write(gomock.Any()).Return(0, nil).AnyTimes()
	ctx := &cmd.Context{
		Stderr: writer,
		Stdout: writer,
	}

	repoCharm.uploadExistingPendingResources = func(_ context.Context, appName string, pendingResources []application.PendingResourceUpload, conn base.APICallCloser, filesystem modelcmd.Filesystem) error {
		c.Fatalf("Do not upload pending resources if errors")
		return nil
	}
	expectedErrors := []error{errors.NotFoundf("test errors")}
	s.deployerAPI.EXPECT().DeployFromRepository(gomock.Any(), gomock.Any()).Return(application.DeployInfo{}, nil, expectedErrors)

	err := repoCharm.PrepareAndDeploy(ctx, s.deployerAPI, nil)
	c.Assert(err, tc.ErrorMatches, "failed to deploy charm \"testme\"")
}

func (s *charmSuite) TestDeployFromPredeployed(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.modelCommand.EXPECT().Filesystem().Return(s.filesystem).AnyTimes()
	s.configFlag.EXPECT().AbsoluteFileNames(gomock.Any()).Return(nil, nil)
	s.configFlag.EXPECT().ReadConfigPairs(gomock.Any()).Return(nil, nil)
	s.deployerAPI.EXPECT().Deploy(gomock.Any(), gomock.Any()).Return(nil)

	dCharm := s.newDeployCharm()

	predeployedCharm := &predeployedLocalCharm{
		deployCharm:  *dCharm,
		userCharmURL: s.url,
		base:         corebase.MustParseBaseFromString("ubuntu@22.04"),
	}

	writer := mocks.NewMockWriter(ctrl)
	writer.EXPECT().Write(gomock.Any()).Return(0, nil).AnyTimes()
	ctx := &cmd.Context{
		Stderr: writer,
		Stdout: writer,
	}

	err := predeployedCharm.PrepareAndDeploy(ctx, s.deployerAPI, nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *charmSuite) newDeployCharm() *deployCharm {
	return &deployCharm{
		configOptions: s.configFlag,
		deployResources: func(
			context.Context,
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
			URL:    s.url.String(),
			Origin: commoncharm.Origin{Base: corebase.MakeDefaultBase("ubuntu", "20.04")},
		},
		flagSet:  &gnuflag.FlagSet{},
		model:    s.modelCommand,
		numUnits: 0,
	}
}

func (s *charmSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.deployerAPI = mocks.NewMockDeployerAPI(ctrl)
	s.deployerAPI.EXPECT().CharmInfo(gomock.Any(), gomock.Any()).Return(s.charmInfo, nil).AnyTimes()
	s.deployerAPI.EXPECT().ModelUUID().Return("dead-beef", true).AnyTimes()

	s.modelCommand = mocks.NewMockModelCommand(ctrl)
	s.modelCommand.EXPECT().ModelType(gomock.Any()).Return(model.IAAS, nil).AnyTimes()
	s.configFlag = mocks.NewMockDeployConfigFlag(ctrl)
	return ctrl
}

func (s *charmSuite) expectResolveChannel() {
	s.resolver.EXPECT().ResolveCharm(
		gomock.Any(),
		gomock.AssignableToTypeOf(&charm.URL{}),
		gomock.AssignableToTypeOf(commoncharm.Origin{}),
		false,
	).DoAndReturn(
		// Ensure the same curl that is provided, is returned.
		func(ctx context.Context, curl *charm.URL, requestedOrigin commoncharm.Origin, _ bool) (*charm.URL, commoncharm.Origin, []corebase.Base, error) {
			return curl, requestedOrigin, []corebase.Base{
				corebase.MustParseBaseFromString("ubuntu@18.04"),
				corebase.MustParseBaseFromString("ubuntu@20.04"),
				corebase.MustParseBaseFromString("ubuntu@16.04"),
			}, nil
		}).AnyTimes()
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

func strptr(s string) *string {
	return &s
}
