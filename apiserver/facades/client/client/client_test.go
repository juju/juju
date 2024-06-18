// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiclient "github.com/juju/juju/api/client/client"
	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/client/client"
	"github.com/juju/juju/controller"
	coremodel "github.com/juju/juju/core/model"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/core/permission"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/docker/registry"
	"github.com/juju/juju/internal/docker/registry/image"
	registrymocks "github.com/juju/juju/internal/docker/registry/mocks"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/tools"
	jjtesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type clientSuite struct {
	baseSuite
}

func (s *clientSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)

	controllerServiceFactory := s.ControllerServiceFactory(c)
	controllerService := controllerServiceFactory.ControllerConfig()

	err := controllerService.UpdateControllerConfig(context.Background(), map[string]interface{}{controller.JujuManagementSpace: "mgmt01"}, nil)
	c.Assert(err, jc.ErrorIsNil)
}

var _ = gc.Suite(&clientSuite{})

// clearSinceTimes zeros out the updated timestamps inside status
// so we can easily check the results.
// Also set any empty status data maps to nil as there's no
// practical difference and it's easier to write tests that way.
func clearSinceTimes(status *params.FullStatus) {
	for applicationId, application := range status.Applications {
		for unitId, unit := range application.Units {
			unit.WorkloadStatus.Since = nil
			if len(unit.WorkloadStatus.Data) == 0 {
				unit.WorkloadStatus.Data = nil
			}
			unit.AgentStatus.Since = nil
			if len(unit.AgentStatus.Data) == 0 {
				unit.AgentStatus.Data = nil
			}
			for id, subord := range unit.Subordinates {
				subord.WorkloadStatus.Since = nil
				if len(subord.WorkloadStatus.Data) == 0 {
					subord.WorkloadStatus.Data = nil
				}
				subord.AgentStatus.Since = nil
				if len(subord.AgentStatus.Data) == 0 {
					subord.AgentStatus.Data = nil
				}
				unit.Subordinates[id] = subord
			}
			application.Units[unitId] = unit
		}
		application.Status.Since = nil
		if len(application.Status.Data) == 0 {
			application.Status.Data = nil
		}
		status.Applications[applicationId] = application
	}
	for applicationId, application := range status.RemoteApplications {
		application.Status.Since = nil
		if len(application.Status.Data) == 0 {
			application.Status.Data = nil
		}
		status.RemoteApplications[applicationId] = application
	}
	for id, machine := range status.Machines {
		machine.AgentStatus.Since = nil
		if len(machine.AgentStatus.Data) == 0 {
			machine.AgentStatus.Data = nil
		}
		machine.InstanceStatus.Since = nil
		if len(machine.InstanceStatus.Data) == 0 {
			machine.InstanceStatus.Data = nil
		}
		machine.ModificationStatus.Since = nil
		if len(machine.ModificationStatus.Data) == 0 {
			machine.ModificationStatus.Data = nil
		}
		status.Machines[id] = machine
	}
	for id, rel := range status.Relations {
		rel.Status.Since = nil
		if len(rel.Status.Data) == 0 {
			rel.Status.Data = nil
		}
		status.Relations[id] = rel
	}
	status.Model.ModelStatus.Since = nil
	if len(status.Model.ModelStatus.Data) == 0 {
		status.Model.ModelStatus.Data = nil
	}
}

// clearContollerTimestamp zeros out the controller timestamps inside
// status, so we can easily check the results.
func clearContollerTimestamp(status *params.FullStatus) {
	status.ControllerTimestamp = nil
}

func (s *clientSuite) TestClientStatus(c *gc.C) {
	loggo.GetLogger("juju.core.cache").SetLogLevel(loggo.TRACE)
	s.setUpScenario(c)
	conn := s.OpenModelAPIAs(c, s.ControllerModelUUID(), jjtesting.AdminUser, defaultPassword(jjtesting.AdminUser), "")
	status, err := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c)).Status(nil)
	clearSinceTimes(status)
	clearContollerTimestamp(status)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, jc.DeepEquals, scenarioStatus)
}

func (s *clientSuite) TestClientStatusControllerTimestamp(c *gc.C) {
	s.setUpScenario(c)
	conn := s.OpenModelAPIAs(c, s.ControllerModelUUID(), jjtesting.AdminUser, defaultPassword(jjtesting.AdminUser), "")
	status, err := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c)).Status(nil)
	clearSinceTimes(status)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.ControllerTimestamp, gc.NotNil)
}

type findToolsSuite struct {
	jtesting.IsolationSuite
}

var _ = gc.Suite(&findToolsSuite{})

func (s *findToolsSuite) TestFindToolsIAAS(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	backend := NewMockBackend(ctrl)
	authorizer := NewMockAuthorizer(ctrl)
	registryProvider := registrymocks.NewMockRegistry(ctrl)
	toolsFinder := NewMockToolsFinder(ctrl)
	blockDeviceService := NewMockBlockDeviceService(ctrl)
	networkService := NewMockNetworkService(ctrl)
	modelInfoService := NewMockModelInfoService(ctrl)

	simpleStreams := []*tools.Tools{
		{Version: version.MustParseBinary("2.9.6-ubuntu-amd64")},
	}

	modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(
		coremodel.ReadOnlyModel{
			Type: coremodel.IAAS,
		},
		nil,
	)

	gomock.InOrder(
		authorizer.EXPECT().AuthClient().Return(true),
		backend.EXPECT().ControllerTag().Return(coretesting.ControllerTag),
		authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(authentication.ErrorEntityMissingPermission),
		backend.EXPECT().ModelTag().Return(coretesting.ModelTag),
		authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil),

		toolsFinder.EXPECT().FindAgents(gomock.Any(), common.FindAgentsParams{MajorVersion: 2}).
			Return(simpleStreams, nil),
	)

	api, err := client.NewClient(
		backend, modelInfoService, nil,
		nil, blockDeviceService, nil, nil,
		authorizer, nil, toolsFinder,
		nil, nil, nil,
		networkService,
		func(docker.ImageRepoDetails) (registry.Registry, error) {
			return registryProvider, nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	result, err := api.FindTools(context.Background(), params.FindToolsParams{MajorVersion: 2})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.FindToolsResult{List: simpleStreams})
}

func (s *findToolsSuite) TestFindToolsCAASReleased(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	backend := NewMockBackend(ctrl)
	//model := NewMockModel(ctrl)
	authorizer := NewMockAuthorizer(ctrl)
	registryProvider := registrymocks.NewMockRegistry(ctrl)
	toolsFinder := NewMockToolsFinder(ctrl)
	controllerConfigService := NewMockControllerConfigService(ctrl)
	blockDeviceService := NewMockBlockDeviceService(ctrl)
	networkService := NewMockNetworkService(ctrl)
	modelInfoService := NewMockModelInfoService(ctrl)

	simpleStreams := []*tools.Tools{
		{Version: version.MustParseBinary("2.9.9-ubuntu-amd64")},
		{Version: version.MustParseBinary("2.9.10-ubuntu-amd64")},
		{Version: version.MustParseBinary("2.9.11-ubuntu-amd64")},
	}
	s.PatchValue(&coreos.HostOS, func() ostype.OSType { return ostype.Ubuntu })

	modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(
		coremodel.ReadOnlyModel{
			Type:         coremodel.CAAS,
			AgentVersion: version.MustParse("2.9.9"),
		},
		nil)

	gomock.InOrder(
		authorizer.EXPECT().AuthClient().Return(true),
		backend.EXPECT().ControllerTag().Return(coretesting.ControllerTag),
		authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(authentication.ErrorEntityMissingPermission),
		backend.EXPECT().ModelTag().Return(coretesting.ModelTag),
		authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil),

		//backend.EXPECT().Model().Return(model, nil),
		toolsFinder.EXPECT().FindAgents(gomock.Any(), common.FindAgentsParams{MajorVersion: 2}).
			Return(simpleStreams, nil),

		controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controller.Config{
			controller.ControllerUUIDKey: coretesting.ControllerTag.Id(),
			controller.CAASImageRepo: `
{
    "serveraddress": "quay.io",
    "auth": "xxxxx==",
    "repository": "test-account"
}
`[1:],
		}, nil),
		registryProvider.EXPECT().Tags("jujud-operator").Return(tools.Versions{
			image.NewImageInfo(version.MustParse("2.9.8")),    // skip: older than current version.
			image.NewImageInfo(version.MustParse("2.9.9")),    // skip: older than current version.
			image.NewImageInfo(version.MustParse("2.9.10.1")), // skip: current is stable build.
			image.NewImageInfo(version.MustParse("2.9.10")),
			image.NewImageInfo(version.MustParse("2.9.11")),
			image.NewImageInfo(version.MustParse("2.9.12")), // skip: it's not released in simplestream yet.
		}, nil),
		registryProvider.EXPECT().GetArchitecture("jujud-operator", "2.9.10").Return("amd64", nil),
		registryProvider.EXPECT().GetArchitecture("jujud-operator", "2.9.11").Return("amd64", nil),
		registryProvider.EXPECT().Close().Return(nil),
	)

	api, err := client.NewClient(
		backend, modelInfoService, nil,
		nil, blockDeviceService, controllerConfigService, nil,
		authorizer, nil, toolsFinder,
		nil, nil, nil,
		networkService,
		func(repo docker.ImageRepoDetails) (registry.Registry, error) {
			c.Assert(repo, gc.DeepEquals, docker.ImageRepoDetails{
				Repository:    "test-account",
				ServerAddress: "quay.io",
				BasicAuthConfig: docker.BasicAuthConfig{
					Auth: docker.NewToken("xxxxx=="),
				},
			})
			return registryProvider, nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	result, err := api.FindTools(context.Background(), params.FindToolsParams{MajorVersion: 2})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.FindToolsResult{
		List: []*tools.Tools{
			{Version: version.MustParseBinary("2.9.10-ubuntu-amd64")},
			{Version: version.MustParseBinary("2.9.11-ubuntu-amd64")},
		},
	})
}

func (s *findToolsSuite) TestFindToolsCAASNonReleased(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	backend := NewMockBackend(ctrl)
	//model := NewMockModel(ctrl)
	authorizer := NewMockAuthorizer(ctrl)
	registryProvider := registrymocks.NewMockRegistry(ctrl)
	toolsFinder := NewMockToolsFinder(ctrl)
	blockDeviceService := NewMockBlockDeviceService(ctrl)
	controllerConfigService := NewMockControllerConfigService(ctrl)
	networkService := NewMockNetworkService(ctrl)
	modelInfoService := NewMockModelInfoService(ctrl)

	simpleStreams := []*tools.Tools{
		{Version: version.MustParseBinary("2.9.9-ubuntu-amd64")},
		{Version: version.MustParseBinary("2.9.10-ubuntu-amd64")},
		{Version: version.MustParseBinary("2.9.11-ubuntu-amd64")},
		{Version: version.MustParseBinary("2.9.12-ubuntu-amd64")},
	}
	s.PatchValue(&coreos.HostOS, func() ostype.OSType { return ostype.Ubuntu })

	modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(
		coremodel.ReadOnlyModel{
			AgentVersion: version.MustParse("2.9.9.1"),
			Type:         coremodel.CAAS,
		},
		nil,
	)

	gomock.InOrder(
		authorizer.EXPECT().AuthClient().Return(true),
		backend.EXPECT().ControllerTag().Return(coretesting.ControllerTag),
		authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(authentication.ErrorEntityMissingPermission),
		backend.EXPECT().ModelTag().Return(coretesting.ModelTag),
		authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil),

		toolsFinder.EXPECT().FindAgents(gomock.Any(),
			common.FindAgentsParams{MajorVersion: 2, AgentStream: envtools.DevelStream}).
			Return(simpleStreams, nil),

		controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controller.Config{
			controller.ControllerUUIDKey: coretesting.ControllerTag.Id(),
			controller.CAASImageRepo: `
{
    "serveraddress": "quay.io",
    "auth": "xxxxx==",
    "repository": "test-account"
}
`[1:],
		}, nil),
		registryProvider.EXPECT().Tags("jujud-operator").Return(tools.Versions{
			image.NewImageInfo(version.MustParse("2.9.8")), // skip: older than current version.
			image.NewImageInfo(version.MustParse("2.9.9")), // skip: older than current version.
			image.NewImageInfo(version.MustParse("2.9.10.1")),
			image.NewImageInfo(version.MustParse("2.9.10")),
			image.NewImageInfo(version.MustParse("2.9.11")),
			image.NewImageInfo(version.MustParse("2.9.12")),
			image.NewImageInfo(version.MustParse("2.9.13")), // skip: it's not released in simplestream yet.
		}, nil),
		registryProvider.EXPECT().GetArchitecture("jujud-operator", "2.9.10.1").Return("amd64", nil),
		registryProvider.EXPECT().GetArchitecture("jujud-operator", "2.9.10").Return("amd64", nil),
		registryProvider.EXPECT().GetArchitecture("jujud-operator", "2.9.11").Return("amd64", nil),
		registryProvider.EXPECT().GetArchitecture("jujud-operator", "2.9.12").Return("", errors.NotFoundf("2.9.12")), // This can only happen on a non-official registry account.
		registryProvider.EXPECT().Close().Return(nil),
	)

	api, err := client.NewClient(
		backend, modelInfoService, nil,
		nil, blockDeviceService, controllerConfigService, nil,
		authorizer, nil, toolsFinder,
		nil, nil, nil,
		networkService,
		func(repo docker.ImageRepoDetails) (registry.Registry, error) {
			c.Assert(repo, gc.DeepEquals, docker.ImageRepoDetails{
				Repository:    "test-account",
				ServerAddress: "quay.io",
				BasicAuthConfig: docker.BasicAuthConfig{
					Auth: docker.NewToken("xxxxx=="),
				},
			})
			return registryProvider, nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	result, err := api.FindTools(context.Background(), params.FindToolsParams{MajorVersion: 2, AgentStream: envtools.DevelStream})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.FindToolsResult{
		List: []*tools.Tools{
			{Version: version.MustParseBinary("2.9.10.1-ubuntu-amd64")},
			{Version: version.MustParseBinary("2.9.10-ubuntu-amd64")},
			{Version: version.MustParseBinary("2.9.11-ubuntu-amd64")},
		},
	})
}
