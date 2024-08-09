// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	apiclient "github.com/juju/juju/api/client/client"
	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/client/client"
	"github.com/juju/juju/apiserver/facades/client/client/mocks"
	"github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/multiwatcher"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/docker"
	"github.com/juju/juju/docker/registry"
	"github.com/juju/juju/docker/registry/image"
	registrymocks "github.com/juju/juju/docker/registry/mocks"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/tools"
)

type serverSuite struct {
	baseSuite
	client     *client.Client
	newEnviron func() (environs.BootstrapEnviron, error)
}

var _ = gc.Suite(&serverSuite{})

func (s *serverSuite) SetUpTest(c *gc.C) {
	s.ConfigAttrs = map[string]interface{}{
		"authorized-keys": coretesting.FakeAuthKeys,
	}
	s.baseSuite.SetUpTest(c)
	s.client = s.clientForState(c, s.State)
}

func (s *serverSuite) authClientForState(c *gc.C, st *state.State, auth facade.Authorizer) *client.Client {
	context := &facadetest.Context{
		Controller_: s.Controller,
		State_:      st,
		StatePool_:  s.StatePool,
		Auth_:       auth,
		Resources_:  common.NewResources(),
	}
	apiserverClient, err := client.NewFacade(context)
	c.Assert(err, jc.ErrorIsNil)

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	s.newEnviron = func() (environs.BootstrapEnviron, error) {
		return environs.GetEnviron(stateenvirons.EnvironConfigGetter{Model: m}, environs.New)
	}
	return apiserverClient
}

func (s *serverSuite) clientForState(c *gc.C, st *state.State) *client.Client {
	return s.authClientForState(c, st, testing.FakeAuthorizer{
		Tag:        s.AdminUserTag(c),
		Controller: true,
	})
}

func (s *serverSuite) TestNewFacadeWaitsForCachedModel(c *gc.C) {
	setGenerationsControllerConfig(c, s.State)
	state := s.Factory.MakeModel(c, nil)
	defer state.Close()
	// When run in a stress situation, we should hit the race where
	// the model exists in the database but the cache hasn't been updated
	// before we ask for the client.
	_ = s.clientForState(c, state)
}

type clientSuite struct {
	baseSuite

	mgmtSpace *state.Space
}

func (s *clientSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)

	var err error
	s.mgmtSpace, err = s.State.AddSpace("mgmt01", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.UpdateControllerConfig(map[string]interface{}{controller.JujuManagementSpace: "mgmt01"}, nil)
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
	status, err := apiclient.NewClient(s.APIState, coretesting.NoopLogger{}).Status(nil)
	clearSinceTimes(status)
	clearContollerTimestamp(status)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, jc.DeepEquals, scenarioStatus)
}

func (s *clientSuite) TestClientStatusControllerTimestamp(c *gc.C) {
	s.setUpScenario(c)
	status, err := apiclient.NewClient(s.APIState, coretesting.NoopLogger{}).Status(nil)
	clearSinceTimes(status)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.ControllerTimestamp, gc.NotNil)
}

func (s *clientSuite) TestClientWatchAllReadPermission(c *gc.C) {
	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)
	// A very simple end-to-end test, because
	// all the logic is tested elsewhere.
	m, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetProvisioned("i-0", "", agent.BootstrapNonce, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())
	user := s.Factory.MakeUser(c, &factory.UserParams{
		Password: "ro-password",
	})
	c.Assert(err, jc.ErrorIsNil)
	roClient := apiclient.NewClient(s.OpenAPIAs(c, user.UserTag(), "ro-password"), coretesting.NoopLogger{})
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
			ModelUUID:  s.State.ModelUUID(),
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

func (s *clientSuite) TestClientWatchAllAdminPermission(c *gc.C) {
	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)
	loggo.GetLogger("juju.state.allwatcher").SetLogLevel(loggo.TRACE)
	// A very simple end-to-end test, because
	// all the logic is tested elsewhere.
	m, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetProvisioned("i-0", "", agent.BootstrapNonce, nil)
	c.Assert(err, jc.ErrorIsNil)
	// Include a remote app that needs admin access to see.

	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
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

	watcher, err := apiclient.NewClient(s.APIState, coretesting.NoopLogger{}).WatchAll()
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
			ModelUUID:  s.State.ModelUUID(),
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
			ModelUUID: s.State.ModelUUID(),
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

func (s *clientSuite) AssertBlocked(c *gc.C, err error, msg string) {
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue, gc.Commentf("error: %#v", err))
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: msg,
		Code:    "operation is blocked",
	})
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
		toolsFinder.EXPECT().FindAgents(common.FindAgentsParams{MajorVersion: 2}).
			Return(simpleStreams, nil),
		model.EXPECT().Type().Return(state.ModelTypeIAAS),
	)

	api, err := client.NewClientV7(
		backend,
		nil,
		nil,
		authorizer,
		nil,
		toolsFinder,
		nil,
		nil,
		nil,
		func(docker.ImageRepoDetails) (registry.Registry, error) {
			return registryProvider, nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	result, err := api.FindTools(params.FindToolsParams{MajorVersion: 2})
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

func (s *findToolsSuite) TestFindToolsCAASReleasedDefault(c *gc.C) {
	s.assertFindToolsCAASReleased(c, "", "amd64")
}

func (s *findToolsSuite) TestFindToolsCAASReleased(c *gc.C) {
	s.assertFindToolsCAASReleased(c, "arm64", "arm64")
}

func (s *findToolsSuite) assertFindToolsCAASReleased(c *gc.C, wantArch, expectArch string) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	backend := mocks.NewMockBackend(ctrl)
	model := mocks.NewMockModel(ctrl)
	authorizer := mocks.NewMockAuthorizer(ctrl)
	registryProvider := registrymocks.NewMockRegistry(ctrl)
	toolsFinder := mocks.NewMockToolsFinder(ctrl)

	simpleStreams := []*tools.Tools{
		{Version: version.MustParseBinary("2.9.9-ubuntu-amd64")},
		{Version: version.MustParseBinary("2.9.10-ubuntu-amd64")},
		{Version: version.MustParseBinary("2.9.11-ubuntu-amd64")},
		{Version: version.MustParseBinary("2.9.9-ubuntu-arm64")},
		{Version: version.MustParseBinary("2.9.10-ubuntu-arm64")},
		{Version: version.MustParseBinary("2.9.11-ubuntu-arm64")},
	}
	s.PatchValue(&coreos.HostOS, func() ostype.OSType { return ostype.Ubuntu })

	gomock.InOrder(
		authorizer.EXPECT().AuthClient().Return(true),
		backend.EXPECT().ControllerTag().Return(coretesting.ControllerTag),
		authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(authentication.ErrorEntityMissingPermission),
		backend.EXPECT().ModelTag().Return(coretesting.ModelTag),
		authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil),

		backend.EXPECT().Model().Return(model, nil),
		toolsFinder.EXPECT().FindAgents(common.FindAgentsParams{MajorVersion: 2, Arch: wantArch}).
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
		registryProvider.EXPECT().GetArchitectures("jujud-operator", "2.9.10").Return([]string{"amd64", "arm64"}, nil),
		registryProvider.EXPECT().GetArchitectures("jujud-operator", "2.9.11").Return([]string{"amd64", "arm64"}, nil),
		registryProvider.EXPECT().Close().Return(nil),
	)

	api, err := client.NewClientV7(
		backend,
		nil,
		nil,
		authorizer,
		nil,
		toolsFinder,
		nil,
		nil,
		nil,
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
	result, err := api.FindTools(params.FindToolsParams{MajorVersion: 2, Arch: wantArch})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.FindToolsResult{
		List: []*tools.Tools{
			{Version: version.MustParseBinary("2.9.10-ubuntu-" + expectArch)},
			{Version: version.MustParseBinary("2.9.11-ubuntu-" + expectArch)},
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

	simpleStreams := []*tools.Tools{
		{Version: version.MustParseBinary("2.9.9-ubuntu-amd64")},
		{Version: version.MustParseBinary("2.9.10-ubuntu-amd64")},
		{Version: version.MustParseBinary("2.9.11-ubuntu-amd64")},
		{Version: version.MustParseBinary("2.9.12-ubuntu-amd64")},
	}
	s.PatchValue(&coreos.HostOS, func() ostype.OSType { return ostype.Ubuntu })

	gomock.InOrder(
		authorizer.EXPECT().AuthClient().Return(true),
		backend.EXPECT().ControllerTag().Return(coretesting.ControllerTag),
		authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(authentication.ErrorEntityMissingPermission),
		backend.EXPECT().ModelTag().Return(coretesting.ModelTag),
		authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil),

		backend.EXPECT().Model().Return(model, nil),
		toolsFinder.EXPECT().FindAgents(common.FindAgentsParams{MajorVersion: 2, AgentStream: envtools.DevelStream}).
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
		registryProvider.EXPECT().GetArchitectures("jujud-operator", "2.9.10.1").Return([]string{"amd64", "arm64"}, nil),
		registryProvider.EXPECT().GetArchitectures("jujud-operator", "2.9.10").Return([]string{"amd64", "arm64"}, nil),
		registryProvider.EXPECT().GetArchitectures("jujud-operator", "2.9.11").Return([]string{"amd64", "arm64"}, nil),
		registryProvider.EXPECT().GetArchitectures("jujud-operator", "2.9.12").Return(nil, errors.NotFoundf("2.9.12")), // This can only happen on a non-official registry account.
		registryProvider.EXPECT().Close().Return(nil),
	)

	api, err := client.NewClientV7(
		backend,
		nil,
		nil,
		authorizer,
		nil,
		toolsFinder,
		nil,
		nil,
		nil,
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
	result, err := api.FindTools(params.FindToolsParams{MajorVersion: 2, AgentStream: envtools.DevelStream})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.FindToolsResult{
		List: []*tools.Tools{
			{Version: version.MustParseBinary("2.9.10.1-ubuntu-amd64")},
			{Version: version.MustParseBinary("2.9.10-ubuntu-amd64")},
			{Version: version.MustParseBinary("2.9.11-ubuntu-amd64")},
		},
	})
}
