// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/charm"
	"github.com/juju/loggo/v2"
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	apiclient "github.com/juju/juju/api/client/client"
	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/client/client"
	"github.com/juju/juju/apiserver/facades/client/client/mocks"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/multiwatcher"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/docker/registry"
	"github.com/juju/juju/internal/docker/registry/image"
	registrymocks "github.com/juju/juju/internal/docker/registry/mocks"
	"github.com/juju/juju/internal/tools"
	jjtesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type clientSuite struct {
	baseSuite

	mgmtSpace *state.Space
}

func (s *clientSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)

	st := s.ControllerModel(c).State()
	var err error
	s.mgmtSpace, err = st.AddSpace("mgmt01", "", nil)
	c.Assert(err, jc.ErrorIsNil)

	err = st.UpdateControllerConfig(map[string]interface{}{controller.JujuManagementSpace: "mgmt01"}, nil)
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
	loggo.GetLogger("juju.state.allwatcher").SetLogLevel(loggo.TRACE)
	s.setUpScenario(c)
	conn := s.OpenModelAPIAs(c, s.ControllerModelUUID(), jjtesting.AdminUser, defaultPassword(jjtesting.AdminUser), "")
	status, err := apiclient.NewClient(conn, coretesting.NoopLogger{}).Status(nil)
	clearSinceTimes(status)
	clearContollerTimestamp(status)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, jc.DeepEquals, scenarioStatus)
}

func (s *clientSuite) TestClientStatusControllerTimestamp(c *gc.C) {
	s.setUpScenario(c)
	conn := s.OpenModelAPIAs(c, s.ControllerModelUUID(), jjtesting.AdminUser, defaultPassword(jjtesting.AdminUser), "")
	status, err := apiclient.NewClient(conn, coretesting.NoopLogger{}).Status(nil)
	clearSinceTimes(status)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.ControllerTimestamp, gc.NotNil)
}

var _ = gc.Suite(&clientWatchSuite{})

type clientWatchSuite struct {
	clientSuite
}

func (s *clientWatchSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.WithMultiWatcher = true
	s.clientSuite.SetUpTest(c)
}

func (s *clientWatchSuite) TestClientWatchAllReadPermission(c *gc.C) {
	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)
	// A very simple end-to-end test, because
	// all the logic is tested elsewhere.
	m, err := s.ControllerModel(c).State().AddMachine(state.UbuntuBase("12.10"), state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetProvisioned("i-0", "", agent.BootstrapNonce, nil)
	c.Assert(err, jc.ErrorIsNil)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	user := f.MakeUser(c, &factory.UserParams{
		Password: "ro-password",
	})
	c.Assert(err, jc.ErrorIsNil)
	conn := s.OpenModelAPIAs(c, s.ControllerModelUUID(), user.UserTag(), "ro-password", "")
	roClient := apiclient.NewClient(conn, coretesting.NoopLogger{})
	defer roClient.Close()

	watcher, err := roClient.WatchAll()
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		err := watcher.Stop()
		c.Assert(err, jc.ErrorIsNil)
	}()

	deltasCh := make(chan []params.Delta)
	go func() {
		for {
			deltas, err := watcher.Next()
			if err != nil {
				return // watcher stopped
			}
			deltasCh <- deltas
		}
	}()

	machineReady := func(got *params.MachineInfo) bool {
		equal, _ := jc.DeepEqual(got, &params.MachineInfo{
			ModelUUID:  s.ControllerModelUUID(),
			Id:         m.Id(),
			InstanceId: "i-0",
			AgentStatus: params.StatusInfo{
				Current: status.Pending,
			},
			InstanceStatus: params.StatusInfo{
				Current: status.Pending,
			},
			Life:                    life.Alive,
			Base:                    "ubuntu@12.10",
			Jobs:                    []model.MachineJob{state.JobManageModel.ToParams()},
			Addresses:               []params.Address{},
			HardwareCharacteristics: &instance.HardwareCharacteristics{},
			HasVote:                 false,
			WantsVote:               true,
		})
		return equal
	}

	machineMatched := false
	timeout := time.After(coretesting.LongWait)
	i := 0
	for !machineMatched {
		select {
		case deltas := <-deltasCh:
			for _, delta := range deltas {
				entity := delta.Entity
				c.Logf("delta.Entity %d kind %s: %#v", i, entity.EntityId().Kind, entity)
				i++

				switch entity.EntityId().Kind {
				case multiwatcher.MachineKind:
					machine := entity.(*params.MachineInfo)
					machine.AgentStatus.Since = nil
					machine.InstanceStatus.Since = nil
					if machineReady(machine) {
						machineMatched = true
					} else {
						c.Log("machine delta not yet matched")
					}
				}
			}
		case <-timeout:
			c.Fatal("timed out waiting for watcher deltas to be ready")
		}
	}
}

func (s *clientWatchSuite) TestClientWatchAllAdminPermission(c *gc.C) {
	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)
	loggo.GetLogger("juju.state.allwatcher").SetLogLevel(loggo.TRACE)
	// A very simple end-to-end test, because
	// all the logic is tested elsewhere.
	st := s.ControllerModel(c).State()
	m, err := st.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetProvisioned("i-0", "", agent.BootstrapNonce, nil)
	c.Assert(err, jc.ErrorIsNil)
	// Include a remote app that needs admin access to see.

	_, err = st.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "remote-db2",
		OfferUUID:   "offer-uuid",
		URL:         "admin/prod.db2",
		SourceModel: coretesting.ModelTag,
		Endpoints: []charm.Relation{
			{
				Name:      "database",
				Interface: "db2",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeGlobal,
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	conn := s.OpenControllerModelAPI(c)
	watcher, err := apiclient.NewClient(conn, coretesting.NoopLogger{}).WatchAll()
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		err := watcher.Stop()
		c.Assert(err, jc.ErrorIsNil)
	}()

	deltasCh := make(chan []params.Delta)
	go func() {
		for {
			deltas, err := watcher.Next()
			if err != nil {
				return // watcher stopped
			}
			deltasCh <- deltas
		}
	}()

	machineReady := func(got *params.MachineInfo) bool {
		equal, _ := jc.DeepEqual(got, &params.MachineInfo{
			ModelUUID:  st.ModelUUID(),
			Id:         m.Id(),
			InstanceId: "i-0",
			AgentStatus: params.StatusInfo{
				Current: status.Pending,
			},
			InstanceStatus: params.StatusInfo{
				Current: status.Pending,
			},
			Life:                    life.Alive,
			Base:                    "ubuntu@12.10",
			Jobs:                    []model.MachineJob{state.JobManageModel.ToParams()},
			Addresses:               []params.Address{},
			HardwareCharacteristics: &instance.HardwareCharacteristics{},
			HasVote:                 false,
			WantsVote:               true,
		})
		return equal
	}

	appReady := func(got *params.RemoteApplicationUpdate) bool {
		equal, _ := jc.DeepEqual(got, &params.RemoteApplicationUpdate{
			Name:      "remote-db2",
			ModelUUID: st.ModelUUID(),
			OfferURL:  "admin/prod.db2",
			Life:      "alive",
			Status: params.StatusInfo{
				Current: status.Unknown,
			},
		})
		return equal
	}

	machineMatched := false
	appMatched := false
	timeout := time.After(coretesting.LongWait)
	i := 0
	for !machineMatched || !appMatched {
		select {
		case deltas := <-deltasCh:
			for _, delta := range deltas {
				entity := delta.Entity
				c.Logf("delta.Entity %d kind %s: %#v", i, entity.EntityId().Kind, entity)
				i++

				switch entity.EntityId().Kind {
				case multiwatcher.MachineKind:
					machine := entity.(*params.MachineInfo)
					machine.AgentStatus.Since = nil
					machine.InstanceStatus.Since = nil
					if machineReady(machine) {
						machineMatched = true
					} else {
						c.Log("machine delta not yet matched")
					}
				case multiwatcher.RemoteApplicationKind:
					app := entity.(*params.RemoteApplicationUpdate)
					app.Status.Since = nil
					if appReady(app) {
						appMatched = true
					} else {
						c.Log("remote application delta not yet matched")
					}
				}
			}
		case <-timeout:
			c.Fatal("timed out waiting for watcher deltas to be ready")
		}
	}
}

type findToolsSuite struct {
	jtesting.IsolationSuite
}

var _ = gc.Suite(&findToolsSuite{})

func (s *findToolsSuite) TestFindToolsIAAS(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	backend := mocks.NewMockBackend(ctrl)
	model := mocks.NewMockModel(ctrl)
	authorizer := mocks.NewMockAuthorizer(ctrl)
	registryProvider := registrymocks.NewMockRegistry(ctrl)
	toolsFinder := mocks.NewMockToolsFinder(ctrl)
	blockDeviceGetter := mocks.NewMockBlockDeviceGetter(ctrl)

	simpleStreams := []*tools.Tools{
		{Version: version.MustParseBinary("2.9.6-ubuntu-amd64")},
	}

	gomock.InOrder(
		authorizer.EXPECT().AuthClient().Return(true),
		backend.EXPECT().ControllerTag().Return(coretesting.ControllerTag),
		authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(authentication.ErrorEntityMissingPermission),
		backend.EXPECT().ModelTag().Return(coretesting.ModelTag),
		authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil),

		backend.EXPECT().Model().Return(model, nil),
		toolsFinder.EXPECT().FindAgents(gomock.Any(), common.FindAgentsParams{MajorVersion: 2}).
			Return(simpleStreams, nil),
		model.EXPECT().Type().Return(state.ModelTypeIAAS),
	)

	api, err := client.NewClient(
		backend, nil,
		nil, blockDeviceGetter, nil,
		authorizer, nil, toolsFinder,
		nil, nil, nil, nil,
		func(docker.ImageRepoDetails) (registry.Registry, error) {
			return registryProvider, nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	result, err := api.FindTools(context.Background(), params.FindToolsParams{MajorVersion: 2})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.FindToolsResult{List: simpleStreams})
}

func (s *findToolsSuite) getModelConfig(c *gc.C, agentVersion string) *config.Config {
	// Validate version string.
	ver, err := version.Parse(agentVersion)
	c.Assert(err, jc.ErrorIsNil)
	mCfg, err := config.New(config.UseDefaults, coretesting.FakeConfig().Merge(coretesting.Attrs{
		config.AgentVersionKey: ver.String(),
	}))
	c.Assert(err, jc.ErrorIsNil)
	return mCfg
}

func (s *findToolsSuite) TestFindToolsCAASReleased(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	backend := mocks.NewMockBackend(ctrl)
	model := mocks.NewMockModel(ctrl)
	authorizer := mocks.NewMockAuthorizer(ctrl)
	registryProvider := registrymocks.NewMockRegistry(ctrl)
	toolsFinder := mocks.NewMockToolsFinder(ctrl)
	blockDeviceGetter := mocks.NewMockBlockDeviceGetter(ctrl)

	simpleStreams := []*tools.Tools{
		{Version: version.MustParseBinary("2.9.9-ubuntu-amd64")},
		{Version: version.MustParseBinary("2.9.10-ubuntu-amd64")},
		{Version: version.MustParseBinary("2.9.11-ubuntu-amd64")},
	}
	s.PatchValue(&coreos.HostOS, func() coreos.OSType { return coreos.Ubuntu })

	gomock.InOrder(
		authorizer.EXPECT().AuthClient().Return(true),
		backend.EXPECT().ControllerTag().Return(coretesting.ControllerTag),
		authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(authentication.ErrorEntityMissingPermission),
		backend.EXPECT().ModelTag().Return(coretesting.ModelTag),
		authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil),

		backend.EXPECT().Model().Return(model, nil),
		toolsFinder.EXPECT().FindAgents(gomock.Any(), common.FindAgentsParams{MajorVersion: 2}).
			Return(simpleStreams, nil),
		model.EXPECT().Type().Return(state.ModelTypeCAAS),
		model.EXPECT().Config().Return(s.getModelConfig(c, "2.9.9"), nil),

		backend.EXPECT().ControllerConfig().Return(controller.Config{
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
		backend, nil,
		nil, blockDeviceGetter, nil,
		authorizer, nil, toolsFinder,
		nil, nil, nil, nil,
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

	backend := mocks.NewMockBackend(ctrl)
	model := mocks.NewMockModel(ctrl)
	authorizer := mocks.NewMockAuthorizer(ctrl)
	registryProvider := registrymocks.NewMockRegistry(ctrl)
	toolsFinder := mocks.NewMockToolsFinder(ctrl)
	blockDeviceGetter := mocks.NewMockBlockDeviceGetter(ctrl)

	simpleStreams := []*tools.Tools{
		{Version: version.MustParseBinary("2.9.9-ubuntu-amd64")},
		{Version: version.MustParseBinary("2.9.10-ubuntu-amd64")},
		{Version: version.MustParseBinary("2.9.11-ubuntu-amd64")},
		{Version: version.MustParseBinary("2.9.12-ubuntu-amd64")},
	}
	s.PatchValue(&coreos.HostOS, func() coreos.OSType { return coreos.Ubuntu })

	gomock.InOrder(
		authorizer.EXPECT().AuthClient().Return(true),
		backend.EXPECT().ControllerTag().Return(coretesting.ControllerTag),
		authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(authentication.ErrorEntityMissingPermission),
		backend.EXPECT().ModelTag().Return(coretesting.ModelTag),
		authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil),

		backend.EXPECT().Model().Return(model, nil),
		toolsFinder.EXPECT().FindAgents(gomock.Any(),
			common.FindAgentsParams{MajorVersion: 2, AgentStream: envtools.DevelStream}).
			Return(simpleStreams, nil),
		model.EXPECT().Type().Return(state.ModelTypeCAAS),
		model.EXPECT().Config().Return(s.getModelConfig(c, "2.9.9.1"), nil),

		backend.EXPECT().ControllerConfig().Return(controller.Config{
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
		backend, nil,
		nil, blockDeviceGetter, nil,
		authorizer, nil, toolsFinder,
		nil, nil, nil, nil,
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
