// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"fmt"
	"net/url"
	"time"

	"github.com/juju/charm/v8"
	"github.com/juju/charmrepo/v6"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/replicaset/v2"
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	apiclient "github.com/juju/juju/api/client/client"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/apiserver/facades/client/client"
	"github.com/juju/juju/apiserver/facades/client/client/mocks"
	"github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/multiwatcher"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/docker"
	"github.com/juju/juju/docker/registry"
	"github.com/juju/juju/docker/registry/image"
	registrymocks "github.com/juju/juju/docker/registry/mocks"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
)

var validVersion = version.MustParse(fmt.Sprintf("%d.66.666", jujuversion.Current.Major))

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
	client.SetNewEnviron(apiserverClient, func() (environs.BootstrapEnviron, error) {
		return s.newEnviron()
	})
	// Wrap in a happy replicaset.
	session := &fakeSession{
		status: &replicaset.Status{
			Name: "test",
			Members: []replicaset.MemberStatus{
				{
					Id:      1,
					State:   replicaset.PrimaryState,
					Address: "192.168.42.1",
				},
			},
		},
	}
	client.OverrideClientBackendMongoSession(apiserverClient, session)
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

func (s *serverSuite) TestModelInfo(c *gc.C) {
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	conf, _ := s.Model.ModelConfig()
	// Model info is available to read-only users.
	client := s.authClientForState(c, s.State, testing.FakeAuthorizer{
		Tag:        names.NewUserTag("read"),
		Controller: true,
	})
	err = model.SetSLA("advanced", "who", []byte(""))
	c.Assert(err, jc.ErrorIsNil)
	info, err := client.ModelInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.DefaultSeries, gc.Equals, config.PreferredSeries(conf))
	c.Assert(info.CloudRegion, gc.Equals, model.CloudRegion())
	c.Assert(info.ProviderType, gc.Equals, conf.Type())
	c.Assert(info.Name, gc.Equals, conf.Name())
	c.Assert(info.Type, gc.Equals, string(model.Type()))
	c.Assert(info.UUID, gc.Equals, model.UUID())
	c.Assert(info.OwnerTag, gc.Equals, model.Owner().String())
	c.Assert(info.Life, gc.Equals, life.Alive)
	expectedAgentVersion, _ := conf.AgentVersion()
	c.Assert(info.AgentVersion, gc.DeepEquals, &expectedAgentVersion)
	c.Assert(info.SLA, gc.DeepEquals, &params.ModelSLAInfo{
		Level: "advanced",
		Owner: "who",
	})
	c.Assert(info.ControllerUUID, gc.Equals, "controller-deadbeef-1bad-500d-9000-4b1d0d06f00d")
	c.Assert(info.IsController, gc.Equals, model.IsControllerModel())
}

func (s *serverSuite) TestAddMachineVariantsReadOnlyDenied(c *gc.C) {
	user := s.makeLocalModelUser(c, "read", "Read Only")
	api := s.authClientForState(c, s.State, testing.FakeAuthorizer{Tag: user.UserTag})

	_, err := api.AddMachines(params.AddMachines{})
	c.Check(err, gc.ErrorMatches, "permission denied")

	_, err = api.AddMachinesV2(params.AddMachines{})
	c.Check(err, gc.ErrorMatches, "permission denied")

	_, err = api.InjectMachines(params.AddMachines{})
	c.Check(err, gc.ErrorMatches, "permission denied")
}

func (s *serverSuite) makeLocalModelUser(c *gc.C, username, displayname string) permission.UserAccess {
	// factory.MakeUser will create an ModelUser for a local user by default.
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: username, DisplayName: displayname})
	modelUser, err := s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	return modelUser
}

func (s *serverSuite) assertModelVersion(c *gc.C, st *state.State, expectedVersion, expectedStream string) {
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	modelConfig, err := m.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, found := modelConfig.AllAttrs()["agent-version"].(string)
	c.Assert(found, jc.IsTrue)
	c.Assert(agentVersion, gc.Equals, expectedVersion)
	var agentStream string
	agentStream, found = modelConfig.AllAttrs()["agent-stream"].(string)
	c.Assert(found, jc.IsTrue)
	c.Assert(agentStream, gc.Equals, expectedStream)

}

func (s *serverSuite) TestSetModelAgentVersion(c *gc.C) {
	args := params.SetModelAgentVersion{
		Version:     version.MustParse(validVersion.String()),
		AgentStream: "proposed",
	}
	err := s.client.SetModelAgentVersion(args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertModelVersion(c, s.State, validVersion.String(), "proposed")
}

func (s *serverSuite) TestSetModelAgentVersionOldModels(c *gc.C) {
	err := s.State.SetModelAgentVersion(version.MustParse("2.8.0"), nil, false)
	c.Assert(err, jc.ErrorIsNil)
	args := params.SetModelAgentVersion{
		Version: version.MustParse("3.0.0"),
	}
	err = s.client.SetModelAgentVersion(args)
	// TODO: (hml) 18-10-2022
	// Change back when upgrades from 2.9 to 3.0 enabled again.
	//	c.Assert(err, gc.ErrorMatches, `
	//these models must first be upgraded to at least 2.9.35 before upgrading the controller:
	// -admin/controller`[1:])
	c.Assert(err, gc.ErrorMatches, `upgrade to \"3.0.0\" is not supported from \"2.8.0\"`)
}

func (s *serverSuite) TestSetModelAgentVersionForced(c *gc.C) {
	// Get the agent-version set in the model.
	cfg, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := cfg.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	currentVersion := agentVersion.String()

	// Add a machine with the current version and a unit with a different version
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	service, err := s.State.AddApplication(state.AddApplicationArgs{Name: "wordpress", Charm: s.AddTestingCharm(c, "wordpress")})
	c.Assert(err, jc.ErrorIsNil)
	unit, err := service.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetAgentVersion(version.MustParseBinary(currentVersion + "-ubuntu-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetAgentVersion(version.MustParseBinary("1.0.2-ubuntu-amd64"))
	c.Assert(err, jc.ErrorIsNil)

	// This should be refused because an agent doesn't match "currentVersion"
	args := params.SetModelAgentVersion{
		Version: version.MustParse(validVersion.String()),
	}
	err = s.client.SetModelAgentVersion(args)
	c.Check(err, gc.ErrorMatches, "some agents have not upgraded to the current model version .*: unit-wordpress-0")
	// Version hasn't changed
	s.assertModelVersion(c, s.State, currentVersion, "released")
	// But we can force it
	to := validVersion
	to.Minor++
	args = params.SetModelAgentVersion{
		Version:             to,
		IgnoreAgentVersions: true,
	}
	err = s.client.SetModelAgentVersion(args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertModelVersion(c, s.State, to.String(), "released")
}

func (s *serverSuite) makeMigratingModel(c *gc.C, name string, mode state.MigrationMode) {
	otherSt := s.Factory.MakeModel(c, &factory.ModelParams{
		Name:  name,
		Owner: names.NewUserTag("some-user"),
	})
	defer otherSt.Close()
	model, err := otherSt.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.SetMigrationMode(mode)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serverSuite) TestControllerModelSetModelAgentVersionBlockedByImportingModel(c *gc.C) {
	s.Factory.MakeUser(c, &factory.UserParams{Name: "some-user"})
	s.makeMigratingModel(c, "to-migrate", state.MigrationModeImporting)
	args := params.SetModelAgentVersion{
		Version: version.MustParse(validVersion.String()),
	}
	err := s.client.SetModelAgentVersion(args)
	c.Assert(err, gc.ErrorMatches, `model "some-user/to-migrate" is importing, upgrade blocked`)
}

func (s *serverSuite) TestControllerModelSetModelAgentVersionBlockedByExportingModel(c *gc.C) {
	s.Factory.MakeUser(c, &factory.UserParams{Name: "some-user"})
	s.makeMigratingModel(c, "to-migrate", state.MigrationModeExporting)
	args := params.SetModelAgentVersion{
		Version: version.MustParse(validVersion.String()),
	}
	err := s.client.SetModelAgentVersion(args)
	c.Assert(err, gc.ErrorMatches, `model "some-user/to-migrate" is exporting, upgrade blocked`)
}

func (s *serverSuite) TestUserModelSetModelAgentVersionNotAffectedByMigration(c *gc.C) {
	s.Factory.MakeUser(c, &factory.UserParams{Name: "some-user"})
	otherSt := s.Factory.MakeModel(c, nil)
	defer otherSt.Close()

	s.makeMigratingModel(c, "exporting-model", state.MigrationModeExporting)
	s.makeMigratingModel(c, "importing-model", state.MigrationModeImporting)
	args := params.SetModelAgentVersion{
		Version: version.MustParse("2.0.4"),
	}
	client := s.clientForState(c, otherSt)

	s.newEnviron = func() (environs.BootstrapEnviron, error) {
		return &mockEnviron{}, nil
	}

	err := client.SetModelAgentVersion(args)
	c.Assert(err, jc.ErrorIsNil)

	s.assertModelVersion(c, otherSt, "2.0.4", "released")
}

func (s *serverSuite) TestControllerModelSetModelAgentVersionChecksReplicaset(c *gc.C) {
	// Wrap in a very unhappy replicaset.
	session := &fakeSession{
		err: errors.New("boom"),
	}
	client.OverrideClientBackendMongoSession(s.client, session)
	args := params.SetModelAgentVersion{
		Version: version.MustParse(validVersion.String()),
	}
	err := s.client.SetModelAgentVersion(args)
	c.Assert(err.Error(), gc.Equals, "checking replicaset status: boom")
}

func (s *serverSuite) TestUserModelSetModelAgentVersionSkipsMongoCheck(c *gc.C) {
	s.Factory.MakeUser(c, &factory.UserParams{Name: "some-user"})
	otherSt := s.Factory.MakeModel(c, nil)
	defer otherSt.Close()

	args := params.SetModelAgentVersion{
		Version: version.MustParse("2.0.4"),
	}
	apiserverClient := s.clientForState(c, otherSt)
	// Wrap in a very unhappy replicaset.
	session := &fakeSession{
		err: errors.New("boom"),
	}
	client.OverrideClientBackendMongoSession(apiserverClient, session)
	s.newEnviron = func() (environs.BootstrapEnviron, error) {
		return &mockEnviron{}, nil
	}

	err := apiserverClient.SetModelAgentVersion(args)
	c.Assert(err, jc.ErrorIsNil)

	s.assertModelVersion(c, otherSt, "2.0.4", "released")
}

type mockEnviron struct {
	environs.Environ
	validateCloudEndpointCalled bool
	err                         error
}

func (m *mockEnviron) ValidateCloudEndpoint(context.ProviderCallContext) error {
	m.validateCloudEndpointCalled = true
	return m.err
}

func (s *serverSuite) assertCheckProviderAPI(c *gc.C, envError error, expectErr string) {
	env := &mockEnviron{err: envError}
	s.newEnviron = func() (environs.BootstrapEnviron, error) {
		return env, nil
	}
	args := params.SetModelAgentVersion{
		Version: version.MustParse(validVersion.String()),
	}
	err := s.client.SetModelAgentVersion(args)
	c.Assert(env.validateCloudEndpointCalled, jc.IsTrue)
	if expectErr != "" {
		c.Assert(err, gc.ErrorMatches, expectErr)
	} else {
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *serverSuite) TestCheckProviderAPISuccess(c *gc.C) {
	s.assertCheckProviderAPI(c, nil, "")
}

func (s *serverSuite) TestCheckProviderAPIFail(c *gc.C) {
	s.assertCheckProviderAPI(c, errors.New("failme"), "cannot make API call to provider: failme")
}

func (s *serverSuite) assertSetModelAgentVersion(c *gc.C) {
	args := params.SetModelAgentVersion{
		Version: version.MustParse(validVersion.String()),
	}
	err := s.client.SetModelAgentVersion(args)
	c.Assert(err, jc.ErrorIsNil)
	modelConfig, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, found := modelConfig.AllAttrs()["agent-version"]
	c.Assert(found, jc.IsTrue)
	c.Assert(agentVersion, gc.Equals, validVersion.String())
}

func (s *serverSuite) assertSetModelAgentVersionBlocked(c *gc.C, msg string) {
	args := params.SetModelAgentVersion{
		Version: version.MustParse(validVersion.String()),
	}
	err := s.client.SetModelAgentVersion(args)
	s.AssertBlocked(c, err, msg)
}

func (s *serverSuite) TestBlockDestroySetModelAgentVersion(c *gc.C) {
	s.BlockDestroyModel(c, "TestBlockDestroySetModelAgentVersion")
	s.assertSetModelAgentVersion(c)
}

func (s *serverSuite) TestBlockRemoveSetModelAgentVersion(c *gc.C) {
	s.BlockRemoveObject(c, "TestBlockRemoveSetModelAgentVersion")
	s.assertSetModelAgentVersion(c)
}

func (s *serverSuite) TestBlockChangesSetModelAgentVersion(c *gc.C) {
	s.BlockAllChanges(c, "TestBlockChangesSetModelAgentVersion")
	s.assertSetModelAgentVersionBlocked(c, "TestBlockChangesSetModelAgentVersion")
}

func (s *serverSuite) TestAbortCurrentUpgrade(c *gc.C) {
	// Create a provisioned controller.
	machine, err := s.State.AddMachine("series", state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned(instance.Id("i-blah"), "", "fake-nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	// Start an upgrade.
	_, err = s.State.EnsureUpgradeInfo(
		machine.Id(),
		version.MustParse("2.0.0"),
		version.MustParse(validVersion.String()),
	)
	c.Assert(err, jc.ErrorIsNil)
	isUpgrading, err := s.State.IsUpgrading()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isUpgrading, jc.IsTrue)

	// Abort it.
	err = s.client.AbortCurrentUpgrade()
	c.Assert(err, jc.ErrorIsNil)

	isUpgrading, err = s.State.IsUpgrading()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isUpgrading, jc.IsFalse)
}

func (s *serverSuite) assertAbortCurrentUpgradeBlocked(c *gc.C, msg string) {
	err := s.client.AbortCurrentUpgrade()
	s.AssertBlocked(c, err, msg)
}

func (s *serverSuite) assertAbortCurrentUpgrade(c *gc.C) {
	err := s.client.AbortCurrentUpgrade()
	c.Assert(err, jc.ErrorIsNil)
	isUpgrading, err := s.State.IsUpgrading()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isUpgrading, jc.IsFalse)
}

func (s *serverSuite) setupAbortCurrentUpgradeBlocked(c *gc.C) {
	// Create a provisioned controller.
	machine, err := s.State.AddMachine("series", state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned(instance.Id("i-blah"), "", "fake-nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	// Start an upgrade.
	_, err = s.State.EnsureUpgradeInfo(
		machine.Id(),
		version.MustParse("2.0.0"),
		version.MustParse(validVersion.String()),
	)
	c.Assert(err, jc.ErrorIsNil)
	isUpgrading, err := s.State.IsUpgrading()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isUpgrading, jc.IsTrue)
}

func (s *serverSuite) TestBlockDestroyAbortCurrentUpgrade(c *gc.C) {
	s.setupAbortCurrentUpgradeBlocked(c)
	s.BlockDestroyModel(c, "TestBlockDestroyAbortCurrentUpgrade")
	s.assertAbortCurrentUpgrade(c)
}

func (s *serverSuite) TestBlockRemoveAbortCurrentUpgrade(c *gc.C) {
	s.setupAbortCurrentUpgradeBlocked(c)
	s.BlockRemoveObject(c, "TestBlockRemoveAbortCurrentUpgrade")
	s.assertAbortCurrentUpgrade(c)
}

func (s *serverSuite) TestBlockChangesAbortCurrentUpgrade(c *gc.C) {
	s.setupAbortCurrentUpgradeBlocked(c)
	s.BlockAllChanges(c, "TestBlockChangesAbortCurrentUpgrade")
	s.assertAbortCurrentUpgradeBlocked(c, "TestBlockChangesAbortCurrentUpgrade")
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

func (s *clientSuite) testClientUnitResolved(c *gc.C, retry bool, expectedResolvedMode state.ResolvedMode) {
	// Setup:
	s.setUpScenario(c)
	u, err := s.State.Unit("wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Error,
		Message: "gaaah",
		Since:   &now,
	}
	err = u.SetAgentStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	// Code under test:
	err = apiclient.NewClient(s.APIState, coretesting.NoopLogger{}).Resolved("wordpress/0", retry)
	c.Assert(err, jc.ErrorIsNil)
	// Freshen the unit's state.
	err = u.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	// And now the actual test assertions: we set the unit as resolved via
	// the API so it should have a resolved mode set.
	mode := u.Resolved()
	c.Assert(mode, gc.Equals, expectedResolvedMode)
}

func (s *clientSuite) TestClientUnitResolved(c *gc.C) {
	s.testClientUnitResolved(c, false, state.ResolvedNoHooks)
}

func (s *clientSuite) TestClientUnitResolvedRetry(c *gc.C) {
	s.testClientUnitResolved(c, true, state.ResolvedRetryHooks)
}

func (s *clientSuite) setupResolved(c *gc.C) *state.Unit {
	s.setUpScenario(c)
	u, err := s.State.Unit("wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Error,
		Message: "gaaah",
		Since:   &now,
	}
	err = u.SetAgentStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	return u
}

func (s *clientSuite) assertResolved(c *gc.C, u *state.Unit) {
	err := apiclient.NewClient(s.APIState, coretesting.NoopLogger{}).Resolved("wordpress/0", true)
	c.Assert(err, jc.ErrorIsNil)
	// Freshen the unit's state.
	err = u.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	// And now the actual test assertions: we set the unit as resolved via
	// the API so it should have a resolved mode set.
	mode := u.Resolved()
	c.Assert(mode, gc.Equals, state.ResolvedRetryHooks)
}

func (s *clientSuite) assertResolvedBlocked(c *gc.C, u *state.Unit, msg string) {
	err := apiclient.NewClient(s.APIState, coretesting.NoopLogger{}).Resolved("wordpress/0", false)
	s.AssertBlocked(c, err, msg)
}

func (s *clientSuite) TestBlockDestroyUnitResolved(c *gc.C) {
	u := s.setupResolved(c)
	s.BlockDestroyModel(c, "TestBlockDestroyUnitResolved")
	s.assertResolved(c, u)
}

func (s *clientSuite) TestBlockRemoveUnitResolved(c *gc.C) {
	u := s.setupResolved(c)
	s.BlockRemoveObject(c, "TestBlockRemoveUnitResolved")
	s.assertResolved(c, u)
}

func (s *clientSuite) TestBlockChangeUnitResolved(c *gc.C) {
	u := s.setupResolved(c)
	s.BlockAllChanges(c, "TestBlockChangeUnitResolved")
	s.assertResolvedBlocked(c, u, "TestBlockChangeUnitResolved")
}

type mockRepo struct {
	charmrepo.Interface
	*jtesting.CallMocker
}

func (m *mockRepo) Resolve(ref *charm.URL) (canonRef *charm.URL, supportedSeries []string, err error) {
	results := m.MethodCall(m, "Resolve", ref)
	if results == nil {
		entity := "charm or bundle"
		if ref.Series != "" {
			entity = "charm"
		}
		return nil, nil, errors.NotFoundf(`cannot resolve URL %q: %s`, ref, entity)
	}
	return results[0].(*charm.URL), []string{"bionic"}, nil
}

func (m *mockRepo) DownloadCharm(downloadURL, archivePath string) (*charm.CharmArchive, error) {
	m.MethodCall(m, "DownloadCharm", downloadURL, archivePath)
	return nil, nil
}

func (m *mockRepo) FindDownloadURL(curl *charm.URL, origin corecharm.Origin) (*url.URL, corecharm.Origin, error) {
	m.MethodCall(m, "FindDownloadURL", curl, origin)
	return nil, corecharm.Origin{}, nil
}

type clientRepoSuite struct {
	baseSuite
	repo *mockRepo
}

var _ = gc.Suite(&clientRepoSuite{})

func (s *clientRepoSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)
	c.Assert(s.APIState, gc.NotNil)

	var logger loggo.Logger
	s.repo = &mockRepo{
		CallMocker: jtesting.NewCallMocker(logger),
	}

	s.PatchValue(&application.OpenCSRepo, func(args application.OpenCSRepoParams) (application.Repository, error) {
		return s.repo, nil
	})
}

func (s *clientRepoSuite) UploadCharm(url string) {
	resultURL := charm.MustParseURL(url)
	baseURL := *resultURL
	baseURL.Series = ""
	baseURL.Revision = -1
	norevURL := *resultURL
	norevURL.Revision = -1
	for _, url := range []*charm.URL{resultURL, &baseURL, &norevURL} {
		s.repo.Call("Resolve", url).Returns(
			resultURL,
		)
	}
}

func (s *clientSuite) TestClientWatchAllReadPermission(c *gc.C) {
	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)
	// A very simple end-to-end test, because
	// all the logic is tested elsewhere.
	m, err := s.State.AddMachine("quantal", state.JobManageModel)
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
			Series:                  "quantal",
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
	m, err := s.State.AddMachine("quantal", state.JobManageModel)
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
			Series:                  "quantal",
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

func (s *serverSuite) TestCheckMongoStatusForUpgradeNonHAGood(c *gc.C) {
	session := &fakeSession{
		status: &replicaset.Status{
			Name: "test",
			Members: []replicaset.MemberStatus{
				{
					Id:      1,
					State:   replicaset.PrimaryState,
					Address: "192.168.42.1",
				},
			},
		},
	}
	err := s.client.CheckMongoStatusForUpgrade(session)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serverSuite) TestCheckMongoStatusForUpgradeHAGood(c *gc.C) {
	session := &fakeSession{
		status: &replicaset.Status{
			Name: "test",
			Members: []replicaset.MemberStatus{
				{
					Id:      1,
					State:   replicaset.PrimaryState,
					Address: "192.168.42.1",
				}, {
					Id:      2,
					State:   replicaset.SecondaryState,
					Address: "192.168.42.2",
				}, {
					Id:      3,
					State:   replicaset.SecondaryState,
					Address: "192.168.42.3",
				},
			},
		},
	}
	err := s.client.CheckMongoStatusForUpgrade(session)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serverSuite) TestCheckMongoStatusForUpgradeHANodeDown(c *gc.C) {
	session := &fakeSession{
		status: &replicaset.Status{
			Name: "test",
			Members: []replicaset.MemberStatus{
				{
					Id:      1,
					State:   replicaset.PrimaryState,
					Address: "192.168.42.1",
				}, {
					Id:      2,
					State:   replicaset.DownState,
					Address: "192.168.42.2",
				}, {
					Id:      3,
					State:   replicaset.SecondaryState,
					Address: "192.168.42.3",
				},
			},
		},
	}
	err := s.client.CheckMongoStatusForUpgrade(session)
	c.Assert(err.Error(), gc.Equals, "unable to upgrade, database node 2 (192.168.42.2) has state DOWN")
}

func (s *serverSuite) TestCheckMongoStatusForUpgradeHANodeRecovering(c *gc.C) {
	session := &fakeSession{
		status: &replicaset.Status{
			Name: "test",
			Members: []replicaset.MemberStatus{
				{
					Id:      1,
					State:   replicaset.RecoveringState,
					Address: "192.168.42.1",
				}, {
					Id:      2,
					State:   replicaset.PrimaryState,
					Address: "192.168.42.2",
				}, {
					Id:      3,
					State:   replicaset.SecondaryState,
					Address: "192.168.42.3",
				},
			},
		},
	}
	err := s.client.CheckMongoStatusForUpgrade(session)
	c.Assert(err.Error(), gc.Equals, "unable to upgrade, database node 1 (192.168.42.1) has state RECOVERING")
}

type fakeSession struct {
	status *replicaset.Status
	err    error
}

func (s fakeSession) CurrentStatus() (*replicaset.Status, error) {
	return s.status, s.err
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

	simpleStreams := params.FindToolsResult{
		List: []*tools.Tools{
			{Version: version.MustParseBinary("2.9.6-ubuntu-amd64")},
		},
	}

	gomock.InOrder(
		authorizer.EXPECT().AuthClient().Return(true),
		backend.EXPECT().ControllerTag().Return(coretesting.ControllerTag),
		authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(true, nil),
		backend.EXPECT().ModelTag().Return(coretesting.ModelTag),
		authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(true, nil),

		backend.EXPECT().Model().Return(model, nil),
		toolsFinder.EXPECT().FindTools(params.FindToolsParams{MajorVersion: 2}).
			Return(simpleStreams, nil),
		model.EXPECT().Type().Return(state.ModelTypeIAAS),
	)

	api, err := client.NewClient(
		backend,
		nil, nil, nil,
		authorizer, nil, toolsFinder,
		nil, nil, nil, nil, nil, nil, nil,
		func(docker.ImageRepoDetails) (registry.Registry, error) {
			return registryProvider, nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	result, err := api.FindTools(params.FindToolsParams{MajorVersion: 2})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, simpleStreams)
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

	simpleStreams := params.FindToolsResult{
		List: []*tools.Tools{
			{Version: version.MustParseBinary("2.9.9-ubuntu-amd64")},
			{Version: version.MustParseBinary("2.9.10-ubuntu-amd64")},
			{Version: version.MustParseBinary("2.9.11-ubuntu-amd64")},
		},
	}
	s.PatchValue(&coreos.HostOS, func() coreos.OSType { return coreos.Ubuntu })

	gomock.InOrder(
		authorizer.EXPECT().AuthClient().Return(true),
		backend.EXPECT().ControllerTag().Return(coretesting.ControllerTag),
		authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(true, nil),
		backend.EXPECT().ModelTag().Return(coretesting.ModelTag),
		authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(true, nil),

		backend.EXPECT().Model().Return(model, nil),
		toolsFinder.EXPECT().FindTools(params.FindToolsParams{MajorVersion: 2}).
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
		backend,
		nil, nil, nil,
		authorizer, nil, toolsFinder,
		nil, nil, nil, nil, nil, nil, nil,
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
	result, err := api.FindTools(params.FindToolsParams{MajorVersion: 2})
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

	simpleStreams := params.FindToolsResult{
		List: []*tools.Tools{
			{Version: version.MustParseBinary("2.9.9-ubuntu-amd64")},
			{Version: version.MustParseBinary("2.9.10-ubuntu-amd64")},
			{Version: version.MustParseBinary("2.9.11-ubuntu-amd64")},
			{Version: version.MustParseBinary("2.9.12-ubuntu-amd64")},
		},
	}
	s.PatchValue(&coreos.HostOS, func() coreos.OSType { return coreos.Ubuntu })

	gomock.InOrder(
		authorizer.EXPECT().AuthClient().Return(true),
		backend.EXPECT().ControllerTag().Return(coretesting.ControllerTag),
		authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(true, nil),
		backend.EXPECT().ModelTag().Return(coretesting.ModelTag),
		authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(true, nil),

		backend.EXPECT().Model().Return(model, nil),
		toolsFinder.EXPECT().FindTools(params.FindToolsParams{MajorVersion: 2, AgentStream: envtools.DevelStream}).
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
		backend,
		nil, nil, nil,
		authorizer, nil, toolsFinder,
		nil, nil, nil, nil, nil, nil, nil,
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
