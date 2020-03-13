// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/os/series"
	"github.com/juju/replicaset"
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v4"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/apiserver/facades/client/client"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/manual/sshprovisioner"
	toolstesting "github.com/juju/juju/environs/tools/testing"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	jujuversion "github.com/juju/juju/version"
)

type serverSuite struct {
	baseSuite
	client     *client.Client
	newEnviron func() (environs.BootstrapEnviron, error)
	newBroker  func() (environs.BootstrapEnviron, error)
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

func (s *serverSuite) TestModelUsersInfo(c *gc.C) {
	testAdmin := s.AdminUserTag(c)
	owner, err := s.State.UserAccess(testAdmin, s.Model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	localUser1 := s.makeLocalModelUser(c, "ralphdoe", "Ralph Doe")
	localUser2 := s.makeLocalModelUser(c, "samsmith", "Sam Smith")
	remoteUser1 := s.Factory.MakeModelUser(c, &factory.ModelUserParams{User: "bobjohns@ubuntuone", DisplayName: "Bob Johns", Access: permission.WriteAccess})
	remoteUser2 := s.Factory.MakeModelUser(c, &factory.ModelUserParams{User: "nicshaw@idprovider", DisplayName: "Nic Shaw", Access: permission.WriteAccess})

	results, err := s.client.ModelUserInfo()
	c.Assert(err, jc.ErrorIsNil)
	var expected params.ModelUserInfoResults
	for _, r := range []struct {
		user permission.UserAccess
		info *params.ModelUserInfo
	}{
		{
			owner,
			&params.ModelUserInfo{
				UserName:    owner.UserName,
				DisplayName: owner.DisplayName,
				Access:      "admin",
			},
		}, {
			localUser1,
			&params.ModelUserInfo{
				UserName:    "ralphdoe",
				DisplayName: "Ralph Doe",
				Access:      "admin",
			},
		}, {
			localUser2,
			&params.ModelUserInfo{
				UserName:    "samsmith",
				DisplayName: "Sam Smith",
				Access:      "admin",
			},
		}, {
			remoteUser1,
			&params.ModelUserInfo{
				UserName:    "bobjohns@ubuntuone",
				DisplayName: "Bob Johns",
				Access:      "write",
			},
		}, {
			remoteUser2,
			&params.ModelUserInfo{
				UserName:    "nicshaw@idprovider",
				DisplayName: "Nic Shaw",
				Access:      "write",
			},
		},
	} {
		r.info.LastConnection = lastConnPointer(c, r.user, s.State)
		expected.Results = append(expected.Results, params.ModelUserInfoResult{Result: r.info})
	}

	sort.Sort(ByUserName(expected.Results))
	sort.Sort(ByUserName(results.Results))
	c.Assert(results, jc.DeepEquals, expected)
}

func lastConnPointer(c *gc.C, modelUser permission.UserAccess, st *state.State) *time.Time {
	model, err := st.Model()
	if err != nil {
		c.Fatal(err)
	}

	lastConn, err := model.LastModelConnection(modelUser.UserTag)
	if err != nil {
		if state.IsNeverConnectedError(err) {
			return nil
		}
		c.Fatal(err)
	}
	return &lastConn
}

// ByUserName implements sort.Interface for []params.ModelUserInfoResult based on
// the UserName field.
type ByUserName []params.ModelUserInfoResult

func (a ByUserName) Len() int           { return len(a) }
func (a ByUserName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByUserName) Less(i, j int) bool { return a[i].Result.UserName < a[j].Result.UserName }

func (s *serverSuite) makeLocalModelUser(c *gc.C, username, displayname string) permission.UserAccess {
	// factory.MakeUser will create an ModelUser for a local user by default.
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: username, DisplayName: displayname})
	modelUser, err := s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	return modelUser
}

func (s *serverSuite) assertModelVersion(c *gc.C, st *state.State, expected string) {
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	modelConfig, err := m.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, found := modelConfig.AllAttrs()["agent-version"]
	c.Assert(found, jc.IsTrue)
	c.Assert(agentVersion, gc.Equals, expected)
}

func (s *serverSuite) TestSetModelAgentVersion(c *gc.C) {
	args := params.SetModelAgentVersion{
		Version: version.MustParse("9.8.7"),
	}
	err := s.client.SetModelAgentVersion(args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertModelVersion(c, s.State, "9.8.7")
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

	err = machine.SetAgentVersion(version.MustParseBinary(currentVersion + "-quantal-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetAgentVersion(version.MustParseBinary("1.0.2-quantal-amd64"))
	c.Assert(err, jc.ErrorIsNil)

	// This should be refused because an agent doesn't match "currentVersion"
	args := params.SetModelAgentVersion{
		Version: version.MustParse("9.8.7"),
	}
	err = s.client.SetModelAgentVersion(args)
	c.Check(err, gc.ErrorMatches, "some agents have not upgraded to the current model version .*: unit-wordpress-0")
	// Version hasn't changed
	s.assertModelVersion(c, s.State, currentVersion)
	// But we can force it
	args = params.SetModelAgentVersion{
		Version:             version.MustParse("7.8.6"),
		IgnoreAgentVersions: true,
	}
	err = s.client.SetModelAgentVersion(args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertModelVersion(c, s.State, "7.8.6")
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
		Version: version.MustParse("9.8.7"),
	}
	err := s.client.SetModelAgentVersion(args)
	c.Assert(err, gc.ErrorMatches, `model "some-user/to-migrate" is importing, upgrade blocked`)
}

func (s *serverSuite) TestControllerModelSetModelAgentVersionBlockedByExportingModel(c *gc.C) {
	s.Factory.MakeUser(c, &factory.UserParams{Name: "some-user"})
	s.makeMigratingModel(c, "to-migrate", state.MigrationModeExporting)
	args := params.SetModelAgentVersion{
		Version: version.MustParse("9.8.7"),
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

	s.assertModelVersion(c, otherSt, "2.0.4")
}

func (s *serverSuite) TestControllerModelSetModelAgentVersionChecksReplicaset(c *gc.C) {
	// Wrap in a very unhappy replicaset.
	session := &fakeSession{
		err: errors.New("boom"),
	}
	client.OverrideClientBackendMongoSession(s.client, session)
	args := params.SetModelAgentVersion{
		Version: version.MustParse("9.8.7"),
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

	s.assertModelVersion(c, otherSt, "2.0.4")
}

type mockEnviron struct {
	environs.Environ
	allInstancesCalled bool
	err                error
}

func (m *mockEnviron) AllInstances(context.ProviderCallContext) ([]instances.Instance, error) {
	m.allInstancesCalled = true
	return nil, m.err
}

func (s *serverSuite) assertCheckProviderAPI(c *gc.C, envError error, expectErr string) {
	env := &mockEnviron{err: envError}
	s.newEnviron = func() (environs.BootstrapEnviron, error) {
		return env, nil
	}
	args := params.SetModelAgentVersion{
		Version: version.MustParse("9.8.7"),
	}
	err := s.client.SetModelAgentVersion(args)
	c.Assert(env.allInstancesCalled, jc.IsTrue)
	if expectErr != "" {
		c.Assert(err, gc.ErrorMatches, expectErr)
	} else {
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *serverSuite) TestCheckProviderAPISuccess(c *gc.C) {
	s.assertCheckProviderAPI(c, nil, "")
	s.assertCheckProviderAPI(c, environs.ErrPartialInstances, "")
	s.assertCheckProviderAPI(c, environs.ErrNoInstances, "")
}

func (s *serverSuite) TestCheckProviderAPIFail(c *gc.C) {
	s.assertCheckProviderAPI(c, errors.New("instances error"), "cannot make API call to provider: instances error")
}

type mockBroker struct {
	caas.Broker
	getMetadataCalled bool
	err               error
}

func (m *mockBroker) GetClusterMetadata(storageClass string) (result *caas.ClusterMetadata, err error) {
	m.getMetadataCalled = true
	return nil, m.err
}

func (s *serverSuite) assertCheckCAASProviderAPI(c *gc.C, envError error, expectErr string) {
	env := &mockBroker{err: envError}
	s.newEnviron = func() (environs.BootstrapEnviron, error) {
		return env, nil
	}
	args := params.SetModelAgentVersion{
		Version: version.MustParse("9.8.7"),
	}
	err := s.client.SetModelAgentVersion(args)
	c.Assert(env.getMetadataCalled, jc.IsTrue)
	if expectErr != "" {
		c.Assert(err, gc.ErrorMatches, expectErr)
	} else {
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *serverSuite) TestCheckCAASProviderAPISuccess(c *gc.C) {
	s.assertCheckCAASProviderAPI(c, nil, "")
}

func (s *serverSuite) TestCheckCAASProviderAPIFail(c *gc.C) {
	s.assertCheckCAASProviderAPI(c, errors.New("metadata error"), "cannot make API call to provider: metadata error")
}

func (s *serverSuite) assertSetModelAgentVersion(c *gc.C) {
	args := params.SetModelAgentVersion{
		Version: version.MustParse("9.8.7"),
	}
	err := s.client.SetModelAgentVersion(args)
	c.Assert(err, jc.ErrorIsNil)
	modelConfig, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, found := modelConfig.AllAttrs()["agent-version"]
	c.Assert(found, jc.IsTrue)
	c.Assert(agentVersion, gc.Equals, "9.8.7")
}

func (s *serverSuite) assertSetModelAgentVersionBlocked(c *gc.C, msg string) {
	args := params.SetModelAgentVersion{
		Version: version.MustParse("9.8.7"),
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
		version.MustParse("1.2.3"),
		version.MustParse("9.8.7"),
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
		version.MustParse("1.2.3"),
		version.MustParse("9.8.7"),
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
	s.setUpScenario(c)
	status, err := s.APIState.Client().Status(nil)
	clearSinceTimes(status)
	clearContollerTimestamp(status)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, jc.DeepEquals, scenarioStatus)
}

func (s *clientSuite) TestClientStatusControllerTimestamp(c *gc.C) {
	s.setUpScenario(c)
	status, err := s.APIState.Client().Status(nil)
	clearSinceTimes(status)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.ControllerTimestamp, gc.NotNil)
}

func assertLife(c *gc.C, entity state.Living, life state.Life) {
	err := entity.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity.Life(), gc.Equals, life)
}

func assertRemoved(c *gc.C, entity state.Living) {
	err := entity.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *clientSuite) setupDestroyMachinesTest(c *gc.C) (*state.Machine, *state.Machine, *state.Machine, *state.Unit) {
	m0, err := s.State.AddMachine("quantal", state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	m1, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	m2, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	sch := s.AddTestingCharm(c, "wordpress")
	wordpress := s.AddTestingApplication(c, "wordpress", sch)
	u, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(m1)
	c.Assert(err, jc.ErrorIsNil)

	return m0, m1, m2, u
}

func (s *clientSuite) TestDestroyMachines(c *gc.C) {
	m0, m1, m2, u := s.setupDestroyMachinesTest(c)
	s.assertDestroyMachineSuccess(c, u, m0, m1, m2)
}

func (s *clientSuite) TestForceDestroyMachines(c *gc.C) {
	s.assertForceDestroyMachines(c)
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
	err = s.APIState.Client().Resolved("wordpress/0", retry)
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
	err := s.APIState.Client().Resolved("wordpress/0", true)
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
	err := s.APIState.Client().Resolved("wordpress/0", false)
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

	s.PatchValue(&application.OpenCSRepo, func(args application.OpenCSRepoParams) (charmrepo.Interface, error) {
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
	roClient := s.OpenAPIAs(c, user.UserTag(), "ro-password").Client()
	defer roClient.Close()

	watcher, err := roClient.WatchAll()
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		err := watcher.Stop()
		c.Assert(err, jc.ErrorIsNil)
	}()
	deltas, err := watcher.Next()
	c.Assert(err, jc.ErrorIsNil)
	// Model and machine deltas returned.
	c.Assert(len(deltas), gc.Equals, 2)
	var d0 *params.MachineInfo
	for _, delta := range deltas {
		d, ok := delta.Entity.(*params.MachineInfo)
		if ok {
			d0 = d
			break
		}
	}
	c.Assert(d0, gc.NotNil)
	d0.AgentStatus.Since = nil
	d0.InstanceStatus.Since = nil
	if !c.Check(d0, jc.DeepEquals, &params.MachineInfo{
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
		Jobs:                    []model.MachineJob{state.JobManageModel.ToParams()},
		Addresses:               []params.Address{},
		HardwareCharacteristics: &instance.HardwareCharacteristics{},
		HasVote:                 false,
		WantsVote:               true,
	}) {
		c.Logf("got:")
		for _, d := range deltas {
			c.Logf("%#v\n", d.Entity)
		}
	}
}

func (s *clientSuite) TestClientWatchAllAdminPermission(c *gc.C) {
	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)
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
	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())

	watcher, err := s.APIState.Client().WatchAll()
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		err := watcher.Stop()
		c.Assert(err, jc.ErrorIsNil)
	}()
	deltas, err := watcher.Next()
	c.Assert(err, jc.ErrorIsNil)
	// model, machine, and remote application
	c.Assert(len(deltas), gc.Equals, 3)
	var dMachine *params.MachineInfo
	var dApp *params.RemoteApplicationUpdate
	for i := 0; (dMachine == nil || dApp == nil) && i < len(deltas); i++ {
		entity := deltas[i].Entity
		switch entity.EntityId().Kind {
		case multiwatcher.MachineKind:
			dMachine = entity.(*params.MachineInfo)
		case multiwatcher.RemoteApplicationKind:
			dApp = entity.(*params.RemoteApplicationUpdate)
		default:
			// don't worry about the model
		}
	}
	c.Assert(dMachine, gc.NotNil)
	c.Assert(dApp, gc.NotNil)

	dMachine.AgentStatus.Since = nil
	dMachine.InstanceStatus.Since = nil
	dApp.Status.Since = nil

	if !c.Check(dMachine, jc.DeepEquals, &params.MachineInfo{
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
		Jobs:                    []model.MachineJob{state.JobManageModel.ToParams()},
		Addresses:               []params.Address{},
		HardwareCharacteristics: &instance.HardwareCharacteristics{},
		HasVote:                 false,
		WantsVote:               true,
	}) {
		c.Logf("got:")
		for _, d := range deltas {
			c.Logf("%#v\n", d.Entity)
		}
	}
	if !c.Check(dApp, jc.DeepEquals, &params.RemoteApplicationUpdate{
		Name:      "remote-db2",
		ModelUUID: s.State.ModelUUID(),
		OfferUUID: "offer-uuid",
		OfferURL:  "admin/prod.db2",
		Life:      "alive",
		Status: params.StatusInfo{
			Current: status.Unknown,
		},
	}) {
		c.Logf("got:")
		for _, d := range deltas {
			c.Logf("%#v\n", d.Entity)
		}
	}
}

func (s *clientSuite) TestClientSetModelConstraints(c *gc.C) {
	// Set constraints for the model.
	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().SetModelConstraints(cons)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the constraints have been correctly updated.
	obtained, err := s.State.ModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, cons)
}

func (s *clientSuite) assertSetModelConstraints(c *gc.C) {
	// Set constraints for the model.
	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().SetModelConstraints(cons)
	c.Assert(err, jc.ErrorIsNil)
	// Ensure the constraints have been correctly updated.
	obtained, err := s.State.ModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, cons)
}

func (s *clientSuite) assertSetModelConstraintsBlocked(c *gc.C, msg string) {
	// Set constraints for the model.
	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().SetModelConstraints(cons)
	s.AssertBlocked(c, err, msg)
}

func (s *clientSuite) TestBlockDestroyClientSetModelConstraints(c *gc.C) {
	s.BlockDestroyModel(c, "TestBlockDestroyClientSetModelConstraints")
	s.assertSetModelConstraints(c)
}

func (s *clientSuite) TestBlockRemoveClientSetModelConstraints(c *gc.C) {
	s.BlockRemoveObject(c, "TestBlockRemoveClientSetModelConstraints")
	s.assertSetModelConstraints(c)
}

func (s *clientSuite) TestBlockChangesClientSetModelConstraints(c *gc.C) {
	s.BlockAllChanges(c, "TestBlockChangesClientSetModelConstraints")
	s.assertSetModelConstraintsBlocked(c, "TestBlockChangesClientSetModelConstraints")
}

func (s *clientSuite) TestClientGetModelConstraints(c *gc.C) {
	// Set constraints for the model.
	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.SetModelConstraints(cons)
	c.Assert(err, jc.ErrorIsNil)

	// Check we can get the constraints.
	obtained, err := s.APIState.Client().GetModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, cons)
}

func (s *clientSuite) TestClientPublicAddressErrors(c *gc.C) {
	s.setUpScenario(c)
	_, err := s.APIState.Client().PublicAddress("wordpress")
	c.Assert(err, gc.ErrorMatches, `unknown unit or machine "wordpress"`)
	_, err = s.APIState.Client().PublicAddress("0")
	c.Assert(err, gc.ErrorMatches, `error fetching address for machine "0": no public address\(es\)`)
	_, err = s.APIState.Client().PublicAddress("wordpress/0")
	c.Assert(err, gc.ErrorMatches, `error fetching address for unit "wordpress/0": no public address\(es\)`)
}

func (s *clientSuite) TestClientPublicAddressMachine(c *gc.C) {
	s.setUpScenario(c)

	// Internally, network.SelectPublicAddress is used; the "most public"
	// address is returned.
	m1, err := s.State.Machine("1")
	c.Assert(err, jc.ErrorIsNil)
	cloudLocalAddress := network.NewScopedSpaceAddress("cloudlocal", network.ScopeCloudLocal)
	publicAddress := network.NewScopedSpaceAddress("public", network.ScopePublic)
	err = m1.SetProviderAddresses(cloudLocalAddress)
	c.Assert(err, jc.ErrorIsNil)
	addr, err := s.APIState.Client().PublicAddress("1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, "cloudlocal")
	err = m1.SetProviderAddresses(cloudLocalAddress, publicAddress)
	c.Assert(err, jc.ErrorIsNil)
	addr, err = s.APIState.Client().PublicAddress("1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, "public")
}

func (s *clientSuite) TestClientPublicAddressUnit(c *gc.C) {
	s.setUpScenario(c)

	m1, err := s.State.Machine("1")
	c.Assert(err, jc.ErrorIsNil)
	publicAddress := network.NewScopedSpaceAddress("public", network.ScopePublic)
	err = m1.SetProviderAddresses(publicAddress)
	c.Assert(err, jc.ErrorIsNil)
	addr, err := s.APIState.Client().PublicAddress("wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, "public")
}

func (s *clientSuite) TestClientPrivateAddressErrors(c *gc.C) {
	s.setUpScenario(c)
	_, err := s.APIState.Client().PrivateAddress("wordpress")
	c.Assert(err, gc.ErrorMatches, `unknown unit or machine "wordpress"`)
	_, err = s.APIState.Client().PrivateAddress("0")
	c.Assert(err, gc.ErrorMatches, `error fetching address for machine "0": no private address\(es\)`)
	_, err = s.APIState.Client().PrivateAddress("wordpress/0")
	c.Assert(err, gc.ErrorMatches, `error fetching address for unit "wordpress/0": no private address\(es\)`)
}

func (s *clientSuite) TestClientPrivateAddress(c *gc.C) {
	s.setUpScenario(c)

	// Internally, network.SelectInternalAddress is used; the public
	// address if no cloud-local one is available.
	m1, err := s.State.Machine("1")
	c.Assert(err, jc.ErrorIsNil)
	cloudLocalAddress := network.NewScopedSpaceAddress("cloudlocal", network.ScopeCloudLocal)
	publicAddress := network.NewScopedSpaceAddress("public", network.ScopePublic)
	err = m1.SetProviderAddresses(publicAddress)
	c.Assert(err, jc.ErrorIsNil)
	addr, err := s.APIState.Client().PrivateAddress("1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, "public")
	err = m1.SetProviderAddresses(cloudLocalAddress, publicAddress)
	c.Assert(err, jc.ErrorIsNil)
	addr, err = s.APIState.Client().PrivateAddress("1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, "cloudlocal")
}

func (s *clientSuite) TestClientPrivateAddressUnit(c *gc.C) {
	s.setUpScenario(c)

	m1, err := s.State.Machine("1")
	c.Assert(err, jc.ErrorIsNil)
	privateAddress := network.NewScopedSpaceAddress("private", network.ScopeCloudLocal)
	err = m1.SetProviderAddresses(privateAddress)
	c.Assert(err, jc.ErrorIsNil)
	addr, err := s.APIState.Client().PrivateAddress("wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, "private")
}

func (s *clientSuite) TestClientFindTools(c *gc.C) {
	result, err := s.APIState.Client().FindTools(99, -1, "", "", "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, jc.Satisfies, params.IsCodeNotFound)
	toolstesting.UploadToStorage(c, s.DefaultToolsStorage, "released", version.MustParseBinary("2.99.0-precise-amd64"))
	result, err = s.APIState.Client().FindTools(2, 99, "precise", "amd64", "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.List, gc.HasLen, 1)
	c.Assert(result.List[0].Version, gc.Equals, version.MustParseBinary("2.99.0-precise-amd64"))
	url := fmt.Sprintf("https://%s/model/%s/tools/%s",
		s.APIState.Addr(), coretesting.ModelTag.Id(), result.List[0].Version)
	c.Assert(result.List[0].URL, gc.Equals, url)

	toolstesting.UploadToStorage(c, s.DefaultToolsStorage, "pretend", version.MustParseBinary("3.0.1-precise-amd64"))
	result, err = s.APIState.Client().FindTools(3, 0, "precise", "amd64", "pretend")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.List, gc.HasLen, 1)
	c.Assert(result.List[0].Version, gc.Equals, version.MustParseBinary("3.0.1-precise-amd64"))
	url = fmt.Sprintf("https://%s/model/%s/tools/%s",
		s.APIState.Addr(), coretesting.ModelTag.Id(), result.List[0].Version)
	c.Assert(result.List[0].URL, gc.Equals, url)
}

func (s *clientSuite) checkMachine(c *gc.C, id, series, cons string) {
	// Ensure the machine was actually created.
	machine, err := s.BackingState.Machine(id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Series(), gc.Equals, series)
	c.Assert(machine.Jobs(), gc.DeepEquals, []state.MachineJob{state.JobHostUnits})
	machineConstraints, err := machine.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machineConstraints.String(), gc.Equals, cons)
}

func (s *clientSuite) TestClientAddMachinesDefaultSeries(c *gc.C) {
	apiParams := make([]params.AddMachineParams, 3)
	for i := 0; i < 3; i++ {
		apiParams[i] = params.AddMachineParams{
			Jobs: []model.MachineJob{model.JobHostUnits},
		}
	}
	machines, err := s.APIState.Client().AddMachines(apiParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 3)
	for i, machineResult := range machines {
		c.Assert(machineResult.Machine, gc.DeepEquals, strconv.Itoa(i))
		s.checkMachine(c, machineResult.Machine, series.DefaultSupportedLTS(), apiParams[i].Constraints.String())
	}
}

func (s *clientSuite) assertAddMachines(c *gc.C) {
	apiParams := make([]params.AddMachineParams, 3)
	for i := 0; i < 3; i++ {
		apiParams[i] = params.AddMachineParams{
			Jobs: []model.MachineJob{model.JobHostUnits},
		}
	}
	machines, err := s.APIState.Client().AddMachines(apiParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 3)
	for i, machineResult := range machines {
		c.Assert(machineResult.Machine, gc.DeepEquals, strconv.Itoa(i))
		s.checkMachine(c, machineResult.Machine, series.DefaultSupportedLTS(), apiParams[i].Constraints.String())
	}
}

func (s *clientSuite) assertAddMachinesBlocked(c *gc.C, msg string) {
	apiParams := make([]params.AddMachineParams, 3)
	for i := 0; i < 3; i++ {
		apiParams[i] = params.AddMachineParams{
			Jobs: []model.MachineJob{model.JobHostUnits},
		}
	}
	_, err := s.APIState.Client().AddMachines(apiParams)
	s.AssertBlocked(c, err, msg)
}

func (s *clientSuite) TestBlockDestroyClientAddMachinesDefaultSeries(c *gc.C) {
	s.BlockDestroyModel(c, "TestBlockDestroyClientAddMachinesDefaultSeries")
	s.assertAddMachines(c)
}

func (s *clientSuite) TestBlockRemoveClientAddMachinesDefaultSeries(c *gc.C) {
	s.BlockRemoveObject(c, "TestBlockRemoveClientAddMachinesDefaultSeries")
	s.assertAddMachines(c)
}

func (s *clientSuite) TestBlockChangesClientAddMachines(c *gc.C) {
	s.BlockAllChanges(c, "TestBlockChangesClientAddMachines")
	s.assertAddMachinesBlocked(c, "TestBlockChangesClientAddMachines")
}

func (s *clientSuite) TestClientAddMachinesWithSeries(c *gc.C) {
	apiParams := make([]params.AddMachineParams, 3)
	for i := 0; i < 3; i++ {
		apiParams[i] = params.AddMachineParams{
			Series: "quantal",
			Jobs:   []model.MachineJob{model.JobHostUnits},
		}
	}
	machines, err := s.APIState.Client().AddMachines(apiParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 3)
	for i, machineResult := range machines {
		c.Assert(machineResult.Machine, gc.DeepEquals, strconv.Itoa(i))
		s.checkMachine(c, machineResult.Machine, "quantal", apiParams[i].Constraints.String())
	}
}

func (s *clientSuite) TestClientAddMachineInsideMachine(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	machines, err := s.APIState.Client().AddMachines([]params.AddMachineParams{{
		Jobs:          []model.MachineJob{model.JobHostUnits},
		ContainerType: instance.LXD,
		ParentId:      "0",
		Series:        "quantal",
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)
	c.Assert(machines[0].Machine, gc.Equals, "0/lxd/0")
}

func (s *clientSuite) TestClientAddMachinesWithConstraints(c *gc.C) {
	apiParams := make([]params.AddMachineParams, 3)
	for i := 0; i < 3; i++ {
		apiParams[i] = params.AddMachineParams{
			Jobs: []model.MachineJob{model.JobHostUnits},
		}
	}
	// The last machine has some constraints.
	apiParams[2].Constraints = constraints.MustParse("mem=4G")
	machines, err := s.APIState.Client().AddMachines(apiParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 3)
	for i, machineResult := range machines {
		c.Assert(machineResult.Machine, gc.DeepEquals, strconv.Itoa(i))
		s.checkMachine(c, machineResult.Machine, series.DefaultSupportedLTS(), apiParams[i].Constraints.String())
	}
}

func (s *clientSuite) TestClientAddMachinesWithPlacement(c *gc.C) {
	apiParams := make([]params.AddMachineParams, 4)
	for i := range apiParams {
		apiParams[i] = params.AddMachineParams{
			Jobs: []model.MachineJob{model.JobHostUnits},
		}
	}
	apiParams[0].Placement = instance.MustParsePlacement("lxd")
	apiParams[1].Placement = instance.MustParsePlacement("lxd:0")
	apiParams[1].ContainerType = instance.LXD
	apiParams[2].Placement = instance.MustParsePlacement("controller:invalid")
	apiParams[3].Placement = instance.MustParsePlacement("controller:valid")
	machines, err := s.APIState.Client().AddMachines(apiParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 4)
	c.Assert(machines[0].Machine, gc.Equals, "0/lxd/0")
	c.Assert(machines[1].Error, gc.ErrorMatches, "container type and placement are mutually exclusive")
	c.Assert(machines[2].Error, gc.ErrorMatches, "cannot add a new machine: invalid placement is invalid")
	c.Assert(machines[3].Machine, gc.Equals, "1")

	m, err := s.BackingState.Machine(machines[3].Machine)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Placement(), gc.DeepEquals, apiParams[3].Placement.Directive)
}

func (s *clientSuite) TestClientAddMachinesSomeErrors(c *gc.C) {
	// Here we check that adding a number of containers correctly handles the
	// case that some adds succeed and others fail and report the errors
	// accordingly.
	// We will set up params to the AddMachines API to attempt to create 3 machines.
	// Machines 0 and 1 will be added successfully.
	// Remaining machines will fail due to different reasons.

	// Create a machine to host the requested containers.
	host, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	// The host only supports lxd containers.
	err = host.SetSupportedContainers([]instance.ContainerType{instance.LXD})
	c.Assert(err, jc.ErrorIsNil)

	// Set up params for adding 3 containers.
	apiParams := make([]params.AddMachineParams, 3)
	for i := range apiParams {
		apiParams[i] = params.AddMachineParams{
			Jobs: []model.MachineJob{model.JobHostUnits},
		}
	}
	// This will cause a add-machine to fail due to an unsupported container.
	apiParams[2].ContainerType = instance.KVM
	apiParams[2].ParentId = host.Id()
	machines, err := s.APIState.Client().AddMachines(apiParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 3)

	// Check the results - machines 2 and 3 will have errors.
	c.Check(machines[0].Machine, gc.Equals, "1")
	c.Check(machines[0].Error, gc.IsNil)
	c.Check(machines[1].Machine, gc.Equals, "2")
	c.Check(machines[1].Error, gc.IsNil)
	c.Check(machines[2].Error, gc.ErrorMatches, "cannot add a new machine: machine 0 cannot host kvm containers")
}

func (s *clientSuite) TestClientAddMachinesWithInstanceIdSomeErrors(c *gc.C) {
	apiParams := make([]params.AddMachineParams, 3)
	addrs := network.NewProviderAddresses("1.2.3.4")
	hc := instance.MustParseHardware("mem=4G")
	for i := 0; i < 3; i++ {
		apiParams[i] = params.AddMachineParams{
			Jobs:                    []model.MachineJob{model.JobHostUnits},
			InstanceId:              instance.Id(fmt.Sprintf("1234-%d", i)),
			Nonce:                   "foo",
			HardwareCharacteristics: hc,
			Addrs:                   params.FromProviderAddresses(addrs...),
		}
	}
	// This will cause the last add-machine to fail.
	apiParams[2].Nonce = ""
	machines, err := s.APIState.Client().AddMachines(apiParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 3)
	for i, machineResult := range machines {
		if i == 2 {
			c.Assert(machineResult.Error, gc.NotNil)
			c.Assert(machineResult.Error, gc.ErrorMatches, "cannot add a new machine: cannot add a machine with an instance id and no nonce")
		} else {
			c.Assert(machineResult.Machine, gc.DeepEquals, strconv.Itoa(i))
			s.checkMachine(c, machineResult.Machine, series.DefaultSupportedLTS(), apiParams[i].Constraints.String())
			instanceId := fmt.Sprintf("1234-%d", i)
			s.checkInstance(c, machineResult.Machine, instanceId, "foo", hc, network.NewSpaceAddresses("1.2.3.4"))
		}
	}
}

func (s *clientSuite) checkInstance(c *gc.C, id, instanceId, nonce string,
	hc instance.HardwareCharacteristics, addr network.SpaceAddresses) {

	machine, err := s.BackingState.Machine(id)
	c.Assert(err, jc.ErrorIsNil)
	machineInstanceId, err := machine.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.CheckProvisioned(nonce), jc.IsTrue)
	c.Assert(machineInstanceId, gc.Equals, instance.Id(instanceId))
	machineHardware, err := machine.HardwareCharacteristics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machineHardware.String(), gc.Equals, hc.String())
	c.Assert(machine.Addresses(), gc.DeepEquals, addr)
}

func (s *clientSuite) TestInjectMachinesStillExists(c *gc.C) {
	results := new(params.AddMachinesResults)
	// We need to use Call directly because the client interface
	// no longer refers to InjectMachine.
	args := params.AddMachines{
		MachineParams: []params.AddMachineParams{{
			Jobs:       []model.MachineJob{model.JobHostUnits},
			InstanceId: "i-foo",
			Nonce:      "nonce",
		}},
	}
	err := s.APIState.APICall("Client", 1, "", "AddMachines", args, &results)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Machines, gc.HasLen, 1)
}

func (s *clientSuite) TestProvisioningScript(c *gc.C) {
	// Inject a machine and then call the ProvisioningScript API.
	// The result should be the same as when calling MachineConfig,
	// converting it to a cloudinit.MachineConfig, and disabling
	// apt_upgrade.
	apiParams := params.AddMachineParams{
		Jobs:                    []model.MachineJob{model.JobHostUnits},
		InstanceId:              instance.Id("1234"),
		Nonce:                   "foo",
		HardwareCharacteristics: instance.MustParseHardware("arch=amd64"),
	}
	machines, err := s.APIState.Client().AddMachines([]params.AddMachineParams{apiParams})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 1)
	machineId := machines[0].Machine
	// Call ProvisioningScript. Normally ProvisioningScript and
	// MachineConfig are mutually exclusive; both of them will
	// allocate a api password for the machine agent.
	script, err := s.APIState.Client().ProvisioningScript(params.ProvisioningScriptParams{
		MachineId: machineId,
		Nonce:     apiParams.Nonce,
	})
	c.Assert(err, jc.ErrorIsNil)
	icfg, err := client.InstanceConfig(s.State, machineId, apiParams.Nonce, "")
	c.Assert(err, jc.ErrorIsNil)
	provisioningScript, err := sshprovisioner.ProvisioningScript(icfg)
	c.Assert(err, jc.ErrorIsNil)
	// ProvisioningScript internally calls MachineConfig,
	// which allocates a new, random password. Everything
	// about the scripts should be the same other than
	// the line containing "oldpassword" from agent.conf.
	scriptLines := strings.Split(script, "\n")
	provisioningScriptLines := strings.Split(provisioningScript, "\n")
	c.Assert(scriptLines, gc.HasLen, len(provisioningScriptLines))
	for i, line := range scriptLines {
		if strings.Contains(line, "oldpassword") {
			continue
		}
		c.Assert(line, gc.Equals, provisioningScriptLines[i])
	}
}

func (s *clientSuite) TestProvisioningScriptDisablePackageCommands(c *gc.C) {
	apiParams := params.AddMachineParams{
		Jobs:                    []model.MachineJob{model.JobHostUnits},
		InstanceId:              instance.Id("1234"),
		Nonce:                   "foo",
		HardwareCharacteristics: instance.MustParseHardware("arch=amd64"),
	}
	machines, err := s.APIState.Client().AddMachines([]params.AddMachineParams{apiParams})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 1)
	machineId := machines[0].Machine

	provParams := params.ProvisioningScriptParams{
		MachineId: machineId,
		Nonce:     apiParams.Nonce,
	}

	setUpdateBehavior := func(update, upgrade bool) {
		s.Model.UpdateModelConfig(
			map[string]interface{}{
				"enable-os-upgrade":        upgrade,
				"enable-os-refresh-update": update,
			},
			nil,
		)
	}

	// Test enabling package commands
	provParams.DisablePackageCommands = false
	setUpdateBehavior(true, true)
	script, err := s.APIState.Client().ProvisioningScript(provParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(script, jc.Contains, "apt-get update")
	c.Check(script, jc.Contains, "apt-get upgrade")

	// Test disabling package commands
	provParams.DisablePackageCommands = true
	setUpdateBehavior(false, false)
	script, err = s.APIState.Client().ProvisioningScript(provParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(script, gc.Not(jc.Contains), "apt-get update")
	c.Check(script, gc.Not(jc.Contains), "apt-get upgrade")

	// Test client-specified DisablePackageCommands trumps environment
	// config variables.
	provParams.DisablePackageCommands = true
	setUpdateBehavior(true, true)
	script, err = s.APIState.Client().ProvisioningScript(provParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(script, gc.Not(jc.Contains), "apt-get update")
	c.Check(script, gc.Not(jc.Contains), "apt-get upgrade")

	// Test that in the abasence of a client-specified
	// DisablePackageCommands we use what's set in environment config.
	provParams.DisablePackageCommands = false
	setUpdateBehavior(false, false)
	//provParams.UpdateBehavior = &params.UpdateBehavior{false, false}
	script, err = s.APIState.Client().ProvisioningScript(provParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(script, gc.Not(jc.Contains), "apt-get update")
	c.Check(script, gc.Not(jc.Contains), "apt-get upgrade")
}

func (s *clientRepoSuite) TestResolveCharm(c *gc.C) {
	resolveCharmTests := []struct {
		about      string
		url        string
		resolved   string
		parseErr   string
		resolveErr string
	}{{
		about:    "wordpress resolved",
		url:      "cs:wordpress",
		resolved: "cs:trusty/wordpress",
	}, {
		about:    "mysql resolved",
		url:      "cs:mysql",
		resolved: "cs:precise/mysql",
	}, {
		about:    "fully qualified char reference",
		url:      "cs:utopic/riak-5",
		resolved: "cs:utopic/riak-5",
	}, {
		about:    "charm with series and no revision",
		url:      "cs:precise/wordpress",
		resolved: "cs:precise/wordpress",
	}, {
		about:      "fully qualified reference not found",
		url:        "cs:utopic/riak-42",
		resolveErr: `cannot resolve URL "cs:utopic/riak-42": charm not found`,
	}, {
		about:      "reference not found",
		url:        "cs:no-such",
		resolveErr: `cannot resolve URL "cs:no-such": charm or bundle not found`,
	}, {
		about: "invalid charm name",
		url:   "cs:",
		// go-1.9 replaces 'cs:' with 'cs://', but not go-1.10
		parseErr: `cannot parse URL "cs:(\/\/)?": name "" not valid`,
	}, {
		about:      "local charm",
		url:        "local:wordpress",
		resolveErr: `only charm store charm references are supported, with cs: schema`,
	}}

	// Add some charms to be resolved later.
	for _, url := range []string{
		"precise/wordpress-1",
		"trusty/wordpress-2",
		"precise/mysql-3",
		"trusty/riak-4",
		"utopic/riak-5",
	} {
		s.UploadCharm(url)
	}

	// Run the tests.
	for i, test := range resolveCharmTests {
		c.Logf("test %d: %s", i, test.about)

		client := s.APIState.Client()
		ref, err := charm.ParseURL(test.url)
		if test.parseErr == "" {
			if c.Check(err, jc.ErrorIsNil) == false {
				continue
			}
		} else {
			if c.Check(err, gc.NotNil) == false {
				continue
			}
			c.Check(err, gc.ErrorMatches, test.parseErr)
			continue
		}

		curl, err := client.ResolveCharm(ref)
		if test.resolveErr == "" {
			if c.Check(err, jc.ErrorIsNil) == false {
				continue
			}
			c.Check(curl.String(), gc.Equals, test.resolved)
			continue
		}
		c.Check(err, gc.ErrorMatches, test.resolveErr)
		c.Check(curl, gc.IsNil)
	}
}

func (s *clientSuite) TestRetryProvisioning(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Error,
		Message: "error",
		Since:   &now,
	}
	err = machine.SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.APIState.Client().RetryProvisioning(machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)

	statusInfo, err := machine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Error)
	c.Assert(statusInfo.Message, gc.Equals, "error")
	c.Assert(statusInfo.Data["transient"], jc.IsTrue)
}

func (s *clientSuite) setupRetryProvisioning(c *gc.C) *state.Machine {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Error,
		Message: "error",
		Since:   &now,
	}
	err = machine.SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	return machine
}

func (s *clientSuite) assertRetryProvisioning(c *gc.C, machine *state.Machine) {
	_, err := s.APIState.Client().RetryProvisioning(machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	statusInfo, err := machine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Error)
	c.Assert(statusInfo.Message, gc.Equals, "error")
	c.Assert(statusInfo.Data["transient"], jc.IsTrue)
}

func (s *clientSuite) assertRetryProvisioningBlocked(c *gc.C, machine *state.Machine, msg string) {
	_, err := s.APIState.Client().RetryProvisioning(machine.Tag().(names.MachineTag))
	s.AssertBlocked(c, err, msg)
}

func (s *clientSuite) TestBlockDestroyRetryProvisioning(c *gc.C) {
	m := s.setupRetryProvisioning(c)
	s.BlockDestroyModel(c, "TestBlockDestroyRetryProvisioning")
	s.assertRetryProvisioning(c, m)
}

func (s *clientSuite) TestBlockRemoveRetryProvisioning(c *gc.C) {
	m := s.setupRetryProvisioning(c)
	s.BlockRemoveObject(c, "TestBlockRemoveRetryProvisioning")
	s.assertRetryProvisioning(c, m)
}

func (s *clientSuite) TestBlockChangesRetryProvisioning(c *gc.C) {
	m := s.setupRetryProvisioning(c)
	s.BlockAllChanges(c, "TestBlockChangesRetryProvisioning")
	s.assertRetryProvisioningBlocked(c, m, "TestBlockChangesRetryProvisioning")
}

func (s *clientSuite) TestAPIHostPorts(c *gc.C) {
	server1Addresses := []network.SpaceAddress{
		network.NewScopedSpaceAddress("server-1", network.ScopePublic),
		network.NewScopedSpaceAddress("10.0.0.1", network.ScopeCloudLocal),
	}
	server1Addresses[1].SpaceID = s.mgmtSpace.Id()

	server2Addresses := []network.SpaceAddress{
		network.NewScopedSpaceAddress("::1", network.ScopeMachineLocal),
	}
	stateAPIHostPorts := []network.SpaceHostPorts{
		network.SpaceAddressesWithPort(server1Addresses, 123),
		network.SpaceAddressesWithPort(server2Addresses, 456),
	}

	err := s.State.SetAPIHostPorts(stateAPIHostPorts)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that address filtering by management space occurred.
	agentHostPorts, err := s.State.APIHostPortsForAgents()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(agentHostPorts, gc.Not(gc.DeepEquals), stateAPIHostPorts)

	apiHostPorts, err := s.APIState.Client().APIHostPorts()
	c.Assert(err, jc.ErrorIsNil)

	// We need to compare SpaceHostPorts with MachineHostPorts.
	// They should be congruent.
	c.Assert(len(apiHostPorts), gc.Equals, len(stateAPIHostPorts))
	for i, apiHPs := range apiHostPorts {
		c.Assert(len(apiHPs), gc.Equals, len(stateAPIHostPorts[i]))
		for j, apiHP := range apiHPs {
			c.Assert(apiHP.MachineAddress, gc.DeepEquals, stateAPIHostPorts[i][j].MachineAddress)
			c.Assert(apiHP.NetPort, gc.Equals, stateAPIHostPorts[i][j].NetPort)
		}
	}

}

func (s *clientSuite) TestClientAgentVersion(c *gc.C) {
	current := version.MustParse("1.2.0")
	s.PatchValue(&jujuversion.Current, current)
	result, err := s.APIState.Client().AgentVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, current)
}

func (s *clientSuite) assertDestroyMachineSuccess(c *gc.C, u *state.Unit, m0, m1, m2 *state.Machine) {
	err := s.APIState.Client().DestroyMachines("0", "1", "2")
	c.Assert(err, gc.ErrorMatches, `some machines were not destroyed: controller 0 is the only controller; machine 1 has unit "wordpress/0" assigned`)
	assertLife(c, m0, state.Alive)
	assertLife(c, m1, state.Alive)
	assertLife(c, m2, state.Dying)

	err = u.UnassignFromMachine()
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().DestroyMachines("0", "1", "2")
	c.Assert(err, gc.ErrorMatches, `some machines were not destroyed: controller 0 is the only controller`)
	assertLife(c, m0, state.Alive)
	assertLife(c, m1, state.Dying)
	assertLife(c, m2, state.Dying)
}

func (s *clientSuite) assertBlockedErrorAndLiveliness(
	c *gc.C,
	err error,
	msg string,
	living1 state.Living,
	living2 state.Living,
	living3 state.Living,
	living4 state.Living,
) {
	s.AssertBlocked(c, err, msg)
	assertLife(c, living1, state.Alive)
	assertLife(c, living2, state.Alive)
	assertLife(c, living3, state.Alive)
	assertLife(c, living4, state.Alive)
}

func (s *clientSuite) AssertBlocked(c *gc.C, err error, msg string) {
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue, gc.Commentf("error: %#v", err))
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: msg,
		Code:    "operation is blocked",
	})
}

func (s *clientSuite) TestBlockRemoveDestroyMachines(c *gc.C) {
	m0, m1, m2, u := s.setupDestroyMachinesTest(c)
	s.BlockRemoveObject(c, "TestBlockRemoveDestroyMachines")
	err := s.APIState.Client().DestroyMachines("0", "1", "2")
	s.assertBlockedErrorAndLiveliness(c, err, "TestBlockRemoveDestroyMachines", m0, m1, m2, u)
}

func (s *clientSuite) TestBlockChangesDestroyMachines(c *gc.C) {
	m0, m1, m2, u := s.setupDestroyMachinesTest(c)
	s.BlockAllChanges(c, "TestBlockChangesDestroyMachines")
	err := s.APIState.Client().DestroyMachines("0", "1", "2")
	s.assertBlockedErrorAndLiveliness(c, err, "TestBlockChangesDestroyMachines", m0, m1, m2, u)
}

func (s *clientSuite) TestBlockDestroyDestroyMachines(c *gc.C) {
	m0, m1, m2, u := s.setupDestroyMachinesTest(c)
	s.BlockDestroyModel(c, "TestBlockDestoryDestroyMachines")
	s.assertDestroyMachineSuccess(c, u, m0, m1, m2)
}

func (s *clientSuite) TestAnyBlockForceDestroyMachines(c *gc.C) {
	// force bypasses all blocks
	s.BlockAllChanges(c, "TestAnyBlockForceDestroyMachines")
	s.BlockDestroyModel(c, "TestAnyBlockForceDestroyMachines")
	s.BlockRemoveObject(c, "TestAnyBlockForceDestroyMachines")
	s.assertForceDestroyMachines(c)
}

func (s *clientSuite) assertForceDestroyMachines(c *gc.C) {
	m0, m1, m2, u := s.setupDestroyMachinesTest(c)

	err := s.APIState.Client().ForceDestroyMachines("0", "1", "2")
	c.Assert(err, gc.ErrorMatches, `some machines were not destroyed: controller 0 is the only controller`)
	assertLife(c, m0, state.Alive)
	assertLife(c, m1, state.Alive)
	assertLife(c, m2, state.Alive)
	assertLife(c, u, state.Alive)

	err = s.State.Cleanup()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, m0, state.Alive)
	assertLife(c, m1, state.Dead)
	assertLife(c, m2, state.Dead)
	assertRemoved(c, u)
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
