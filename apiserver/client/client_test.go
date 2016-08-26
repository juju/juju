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
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/series"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/client"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/modelconfig"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/manual"
	toolstesting "github.com/juju/juju/environs/tools/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/presence"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/status"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker"
)

type serverSuite struct {
	baseSuite
	client     *client.Client
	newEnviron func() (environs.Environ, error)
}

var _ = gc.Suite(&serverSuite{})

func (s *serverSuite) SetUpTest(c *gc.C) {
	s.ConfigAttrs = map[string]interface{}{
		"authorized-keys": coretesting.FakeAuthKeys,
	}
	s.baseSuite.SetUpTest(c)

	var err error
	auth := testing.FakeAuthorizer{
		Tag:            s.AdminUserTag(c),
		EnvironManager: true,
	}
	urlGetter := common.NewToolsURLGetter(s.State.ModelUUID(), s.State)
	configGetter := stateenvirons.EnvironConfigGetter{s.State}
	statusSetter := common.NewStatusSetter(s.State, common.AuthAlways())
	toolsFinder := common.NewToolsFinder(configGetter, s.State, urlGetter)
	s.newEnviron = func() (environs.Environ, error) {
		return environs.GetEnviron(configGetter, environs.New)
	}
	newEnviron := func() (environs.Environ, error) {
		return s.newEnviron()
	}
	blockChecker := common.NewBlockChecker(s.State)
	modelConfigAPI, err := modelconfig.NewModelConfigAPI(s.State, auth)
	c.Assert(err, jc.ErrorIsNil)
	s.client, err = client.NewClient(
		client.NewStateBackend(s.State),
		modelConfigAPI,
		common.NewResources(),
		auth,
		statusSetter,
		toolsFinder,
		newEnviron,
		blockChecker,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serverSuite) setAgentPresence(c *gc.C, machineId string) *presence.Pinger {
	m, err := s.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)
	pinger, err := m.SetAgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		c.Assert(worker.Stop(pinger), jc.ErrorIsNil)
	})
	s.State.StartSync()
	err = m.WaitAgentPresence(coretesting.LongWait)
	c.Assert(err, jc.ErrorIsNil)
	return pinger
}

func (s *serverSuite) TestModelInfo(c *gc.C) {
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	conf, _ := s.State.ModelConfig()
	info, err := s.client.ModelInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.DefaultSeries, gc.Equals, config.PreferredSeries(conf))
	c.Assert(info.CloudRegion, gc.Equals, model.CloudRegion())
	c.Assert(info.ProviderType, gc.Equals, conf.Type())
	c.Assert(info.Name, gc.Equals, conf.Name())
	c.Assert(info.UUID, gc.Equals, model.UUID())
	c.Assert(info.OwnerTag, gc.Equals, model.Owner().String())
	c.Assert(info.Life, gc.Equals, params.Alive)
	// The controller UUID is not returned by the ModelInfo endpoint on the
	// Client facade.
	c.Assert(info.ControllerUUID, gc.Equals, "")
}

func (s *serverSuite) TestModelUsersInfo(c *gc.C) {
	testAdmin := s.AdminUserTag(c)
	owner, err := s.State.UserAccess(testAdmin, s.State.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	localUser1 := s.makeLocalModelUser(c, "ralphdoe", "Ralph Doe")
	localUser2 := s.makeLocalModelUser(c, "samsmith", "Sam Smith")
	remoteUser1 := s.Factory.MakeModelUser(c, &factory.ModelUserParams{User: "bobjohns@ubuntuone", DisplayName: "Bob Johns", Access: description.WriteAccess})
	remoteUser2 := s.Factory.MakeModelUser(c, &factory.ModelUserParams{User: "nicshaw@idprovider", DisplayName: "Nic Shaw", Access: description.WriteAccess})

	results, err := s.client.ModelUserInfo()
	c.Assert(err, jc.ErrorIsNil)
	var expected params.ModelUserInfoResults
	for _, r := range []struct {
		user description.UserAccess
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
				UserName:    "ralphdoe@local",
				DisplayName: "Ralph Doe",
				Access:      "admin",
			},
		}, {
			localUser2,
			&params.ModelUserInfo{
				UserName:    "samsmith@local",
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

func lastConnPointer(c *gc.C, modelUser description.UserAccess, st *state.State) *time.Time {
	lastConn, err := st.LastModelConnection(modelUser.UserTag)
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

func (s *serverSuite) makeLocalModelUser(c *gc.C, username, displayname string) description.UserAccess {
	// factory.MakeUser will create an ModelUser for a local user by defalut
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: username, DisplayName: displayname})
	modelUser, err := s.State.UserAccess(user.UserTag(), s.State.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	return modelUser
}

func (s *serverSuite) TestSetEnvironAgentVersion(c *gc.C) {
	args := params.SetModelAgentVersion{
		Version: version.MustParse("9.8.7"),
	}
	err := s.client.SetModelAgentVersion(args)
	c.Assert(err, jc.ErrorIsNil)

	modelConfig, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, found := modelConfig.AllAttrs()["agent-version"]
	c.Assert(found, jc.IsTrue)
	c.Assert(agentVersion, gc.Equals, "9.8.7")
}

type mockEnviron struct {
	environs.Environ
	allInstancesCalled bool
	err                error
}

func (m *mockEnviron) AllInstances() ([]instance.Instance, error) {
	m.allInstancesCalled = true
	return nil, m.err
}

func (s *serverSuite) assertCheckProviderAPI(c *gc.C, envError error, expectErr string) {
	env := &mockEnviron{err: envError}
	s.newEnviron = func() (environs.Environ, error) {
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

func (s *serverSuite) assertSetEnvironAgentVersion(c *gc.C) {
	args := params.SetModelAgentVersion{
		Version: version.MustParse("9.8.7"),
	}
	err := s.client.SetModelAgentVersion(args)
	c.Assert(err, jc.ErrorIsNil)
	modelConfig, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, found := modelConfig.AllAttrs()["agent-version"]
	c.Assert(found, jc.IsTrue)
	c.Assert(agentVersion, gc.Equals, "9.8.7")
}

func (s *serverSuite) assertSetEnvironAgentVersionBlocked(c *gc.C, msg string) {
	args := params.SetModelAgentVersion{
		Version: version.MustParse("9.8.7"),
	}
	err := s.client.SetModelAgentVersion(args)
	s.AssertBlocked(c, err, msg)
}

func (s *serverSuite) TestBlockDestroySetEnvironAgentVersion(c *gc.C) {
	s.BlockDestroyModel(c, "TestBlockDestroySetEnvironAgentVersion")
	s.assertSetEnvironAgentVersion(c)
}

func (s *serverSuite) TestBlockRemoveSetEnvironAgentVersion(c *gc.C) {
	s.BlockRemoveObject(c, "TestBlockRemoveSetEnvironAgentVersion")
	s.assertSetEnvironAgentVersion(c)
}

func (s *serverSuite) TestBlockChangesSetEnvironAgentVersion(c *gc.C) {
	s.BlockAllChanges(c, "TestBlockChangesSetEnvironAgentVersion")
	s.assertSetEnvironAgentVersionBlocked(c, "TestBlockChangesSetEnvironAgentVersion")
}

func (s *serverSuite) TestAbortCurrentUpgrade(c *gc.C) {
	// Create a provisioned controller.
	machine, err := s.State.AddMachine("series", state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned(instance.Id("i-blah"), "fake-nonce", nil)
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
	err = machine.SetProvisioned(instance.Id("i-blah"), "fake-nonce", nil)
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
}

var _ = gc.Suite(&clientSuite{})

// clearSinceTimes zeros out the updated timestamps inside status
// so we can easily check the results.
func clearSinceTimes(status *params.FullStatus) {
	for applicationId, service := range status.Applications {
		for unitId, unit := range service.Units {
			unit.WorkloadStatus.Since = nil
			unit.AgentStatus.Since = nil
			for id, subord := range unit.Subordinates {
				subord.WorkloadStatus.Since = nil
				subord.AgentStatus.Since = nil
				unit.Subordinates[id] = subord
			}
			service.Units[unitId] = unit
		}
		service.Status.Since = nil
		status.Applications[applicationId] = service
	}
	for id, machine := range status.Machines {
		machine.AgentStatus.Since = nil
		machine.InstanceStatus.Since = nil
		status.Machines[id] = machine
	}
}

func (s *clientSuite) TestClientStatus(c *gc.C) {
	s.setUpScenario(c)
	status, err := s.APIState.Client().Status(nil)
	clearSinceTimes(status)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, jc.DeepEquals, scenarioStatus)
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
	wordpress := s.AddTestingService(c, "wordpress", sch)
	u, err := wordpress.AddUnit()
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
		Status:  status.StatusError,
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
		Status:  status.StatusError,
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
	err := s.APIState.Client().Resolved("wordpress/0", true)
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

type clientRepoSuite struct {
	baseSuite
	testing.CharmStoreSuite
}

var _ = gc.Suite(&clientRepoSuite{})

func (s *clientRepoSuite) SetUpSuite(c *gc.C) {
	s.CharmStoreSuite.SetUpSuite(c)
	s.baseSuite.SetUpSuite(c)

}

func (s *clientRepoSuite) TearDownSuite(c *gc.C) {
	s.CharmStoreSuite.TearDownSuite(c)
	s.baseSuite.TearDownSuite(c)
}

func (s *clientRepoSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)
	s.CharmStoreSuite.Session = s.baseSuite.Session
	s.CharmStoreSuite.SetUpTest(c)

	c.Assert(s.APIState, gc.NotNil)
}

func (s *clientRepoSuite) TearDownTest(c *gc.C) {
	s.CharmStoreSuite.TearDownTest(c)
	s.baseSuite.TearDownTest(c)
}

func (s *clientSuite) TestClientWatchAll(c *gc.C) {
	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)
	// A very simple end-to-end test, because
	// all the logic is tested elsewhere.
	m, err := s.State.AddMachine("quantal", state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetProvisioned("i-0", agent.BootstrapNonce, nil)
	c.Assert(err, jc.ErrorIsNil)
	watcher, err := s.APIState.Client().WatchAll()
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		err := watcher.Stop()
		c.Assert(err, jc.ErrorIsNil)
	}()
	deltas, err := watcher.Next()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(deltas), gc.Equals, 1)
	d0, ok := deltas[0].Entity.(*multiwatcher.MachineInfo)
	c.Assert(ok, jc.IsTrue)
	d0.AgentStatus.Since = nil
	d0.InstanceStatus.Since = nil
	if !c.Check(deltas, jc.DeepEquals, []multiwatcher.Delta{{
		Entity: &multiwatcher.MachineInfo{
			ModelUUID:  s.State.ModelUUID(),
			Id:         m.Id(),
			InstanceId: "i-0",
			AgentStatus: multiwatcher.StatusInfo{
				Current: status.StatusPending,
			},
			InstanceStatus: multiwatcher.StatusInfo{
				Current: status.StatusPending,
			},
			Life:                    multiwatcher.Life("alive"),
			Series:                  "quantal",
			Jobs:                    []multiwatcher.MachineJob{state.JobManageModel.ToParams()},
			Addresses:               []multiwatcher.Address{},
			HardwareCharacteristics: &instance.HardwareCharacteristics{},
			HasVote:                 false,
			WantsVote:               true,
		},
	}}) {
		c.Logf("got:")
		for _, d := range deltas {
			c.Logf("%#v\n", d.Entity)
		}
	}
}

func (s *clientSuite) TestClientSetModelConstraints(c *gc.C) {
	// Set constraints for the model.
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
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
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
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
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
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
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
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
	c.Assert(err, gc.ErrorMatches, `error fetching address for machine "0": no public address`)
	_, err = s.APIState.Client().PublicAddress("wordpress/0")
	c.Assert(err, gc.ErrorMatches, `error fetching address for unit "wordpress/0": no public address`)
}

func (s *clientSuite) TestClientPublicAddressMachine(c *gc.C) {
	s.setUpScenario(c)

	// Internally, network.SelectPublicAddress is used; the "most public"
	// address is returned.
	m1, err := s.State.Machine("1")
	c.Assert(err, jc.ErrorIsNil)
	cloudLocalAddress := network.NewScopedAddress("cloudlocal", network.ScopeCloudLocal)
	publicAddress := network.NewScopedAddress("public", network.ScopePublic)
	err = m1.SetProviderAddresses(cloudLocalAddress)
	c.Assert(err, jc.ErrorIsNil)
	addr, err := s.APIState.Client().PublicAddress("1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, "cloudlocal")
	err = m1.SetProviderAddresses(cloudLocalAddress, publicAddress)
	addr, err = s.APIState.Client().PublicAddress("1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, "public")
}

func (s *clientSuite) TestClientPublicAddressUnit(c *gc.C) {
	s.setUpScenario(c)

	m1, err := s.State.Machine("1")
	publicAddress := network.NewScopedAddress("public", network.ScopePublic)
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
	c.Assert(err, gc.ErrorMatches, `error fetching address for machine "0": no private address`)
	_, err = s.APIState.Client().PrivateAddress("wordpress/0")
	c.Assert(err, gc.ErrorMatches, `error fetching address for unit "wordpress/0": no private address`)
}

func (s *clientSuite) TestClientPrivateAddress(c *gc.C) {
	s.setUpScenario(c)

	// Internally, network.SelectInternalAddress is used; the public
	// address if no cloud-local one is available.
	m1, err := s.State.Machine("1")
	c.Assert(err, jc.ErrorIsNil)
	cloudLocalAddress := network.NewScopedAddress("cloudlocal", network.ScopeCloudLocal)
	publicAddress := network.NewScopedAddress("public", network.ScopePublic)
	err = m1.SetProviderAddresses(publicAddress)
	c.Assert(err, jc.ErrorIsNil)
	addr, err := s.APIState.Client().PrivateAddress("1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, "public")
	err = m1.SetProviderAddresses(cloudLocalAddress, publicAddress)
	addr, err = s.APIState.Client().PrivateAddress("1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, "cloudlocal")
}

func (s *clientSuite) TestClientPrivateAddressUnit(c *gc.C) {
	s.setUpScenario(c)

	m1, err := s.State.Machine("1")
	privateAddress := network.NewScopedAddress("private", network.ScopeCloudLocal)
	err = m1.SetProviderAddresses(privateAddress)
	c.Assert(err, jc.ErrorIsNil)
	addr, err := s.APIState.Client().PrivateAddress("wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, "private")
}

func (s *clientSuite) TestClientFindTools(c *gc.C) {
	result, err := s.APIState.Client().FindTools(99, -1, "", "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, jc.Satisfies, params.IsCodeNotFound)
	toolstesting.UploadToStorage(c, s.DefaultToolsStorage, "released", version.MustParseBinary("2.99.0-precise-amd64"))
	result, err = s.APIState.Client().FindTools(2, 99, "precise", "amd64")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.List, gc.HasLen, 1)
	c.Assert(result.List[0].Version, gc.Equals, version.MustParseBinary("2.99.0-precise-amd64"))
	url := fmt.Sprintf("https://%s/model/%s/tools/%s",
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
			Jobs: []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		}
	}
	machines, err := s.APIState.Client().AddMachines(apiParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 3)
	for i, machineResult := range machines {
		c.Assert(machineResult.Machine, gc.DeepEquals, strconv.Itoa(i))
		s.checkMachine(c, machineResult.Machine, series.LatestLts(), apiParams[i].Constraints.String())
	}
}

func (s *clientSuite) assertAddMachines(c *gc.C) {
	apiParams := make([]params.AddMachineParams, 3)
	for i := 0; i < 3; i++ {
		apiParams[i] = params.AddMachineParams{
			Jobs: []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		}
	}
	machines, err := s.APIState.Client().AddMachines(apiParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 3)
	for i, machineResult := range machines {
		c.Assert(machineResult.Machine, gc.DeepEquals, strconv.Itoa(i))
		s.checkMachine(c, machineResult.Machine, series.LatestLts(), apiParams[i].Constraints.String())
	}
}

func (s *clientSuite) assertAddMachinesBlocked(c *gc.C, msg string) {
	apiParams := make([]params.AddMachineParams, 3)
	for i := 0; i < 3; i++ {
		apiParams[i] = params.AddMachineParams{
			Jobs: []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
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
			Jobs:   []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
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
		Jobs:          []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		ContainerType: instance.LXD,
		ParentId:      "0",
		Series:        "quantal",
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)
	c.Assert(machines[0].Machine, gc.Equals, "0/lxd/0")
}

// updateConfig sets config variable with given key to a given value
// Asserts that no errors were encountered.
func (s *baseSuite) updateConfig(c *gc.C, key string, block bool) {
	err := s.State.UpdateModelConfig(map[string]interface{}{key: block}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientSuite) TestClientAddMachinesWithConstraints(c *gc.C) {
	apiParams := make([]params.AddMachineParams, 3)
	for i := 0; i < 3; i++ {
		apiParams[i] = params.AddMachineParams{
			Jobs: []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		}
	}
	// The last machine has some constraints.
	apiParams[2].Constraints = constraints.MustParse("mem=4G")
	machines, err := s.APIState.Client().AddMachines(apiParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 3)
	for i, machineResult := range machines {
		c.Assert(machineResult.Machine, gc.DeepEquals, strconv.Itoa(i))
		s.checkMachine(c, machineResult.Machine, series.LatestLts(), apiParams[i].Constraints.String())
	}
}

func (s *clientSuite) TestClientAddMachinesWithPlacement(c *gc.C) {
	apiParams := make([]params.AddMachineParams, 4)
	for i := range apiParams {
		apiParams[i] = params.AddMachineParams{
			Jobs: []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
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
			Jobs: []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
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
	addrs := network.NewAddresses("1.2.3.4")
	hc := instance.MustParseHardware("mem=4G")
	for i := 0; i < 3; i++ {
		apiParams[i] = params.AddMachineParams{
			Jobs:       []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
			InstanceId: instance.Id(fmt.Sprintf("1234-%d", i)),
			Nonce:      "foo",
			HardwareCharacteristics: hc,
			Addrs: params.FromNetworkAddresses(addrs...),
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
			s.checkMachine(c, machineResult.Machine, series.LatestLts(), apiParams[i].Constraints.String())
			instanceId := fmt.Sprintf("1234-%d", i)
			s.checkInstance(c, machineResult.Machine, instanceId, "foo", hc, addrs)
		}
	}
}

func (s *clientSuite) checkInstance(c *gc.C, id, instanceId, nonce string,
	hc instance.HardwareCharacteristics, addr []network.Address) {

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
			Jobs:       []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
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
		Jobs:       []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		InstanceId: instance.Id("1234"),
		Nonce:      "foo",
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
	provisioningScript, err := manual.ProvisioningScript(icfg)
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
		Jobs:       []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		InstanceId: instance.Id("1234"),
		Nonce:      "foo",
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
		s.State.UpdateModelConfig(
			map[string]interface{}{
				"enable-os-upgrade":        upgrade,
				"enable-os-refresh-update": update,
			},
			nil,
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

var resolveCharmTests = []struct {
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
	about:    "riak resolved",
	url:      "cs:riak",
	resolved: "cs:trusty/riak",
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
	about:    "invalid charm name",
	url:      "cs:",
	parseErr: `URL has invalid charm or bundle name: "cs:"`,
}, {
	about:      "local charm",
	url:        "local:wordpress",
	resolveErr: `only charm store charm references are supported, with cs: schema`,
}}

func (s *clientRepoSuite) TestResolveCharm(c *gc.C) {
	// Add some charms to be resolved later.
	for _, url := range []string{
		"precise/wordpress-1",
		"trusty/wordpress-2",
		"precise/mysql-3",
		"trusty/riak-4",
		"utopic/riak-5",
	} {
		s.UploadCharm(c, url, "wordpress")
	}

	// Run the tests.
	for i, test := range resolveCharmTests {
		c.Logf("test %d: %s", i, test.about)

		client := s.APIState.Client()
		ref, err := charm.ParseURL(test.url)
		if test.parseErr == "" {
			if !c.Check(err, jc.ErrorIsNil) {
				continue
			}
		} else {
			c.Assert(err, gc.NotNil)
			c.Check(err, gc.ErrorMatches, test.parseErr)
			continue
		}

		curl, err := client.ResolveCharm(ref)
		if test.resolveErr == "" {
			c.Assert(err, jc.ErrorIsNil)
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
		Status:  status.StatusError,
		Message: "error",
		Since:   &now,
	}
	err = machine.SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.APIState.Client().RetryProvisioning(machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)

	statusInfo, err := machine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.StatusError)
	c.Assert(statusInfo.Message, gc.Equals, "error")
	c.Assert(statusInfo.Data["transient"], jc.IsTrue)
}

func (s *clientSuite) setupRetryProvisioning(c *gc.C) *state.Machine {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.StatusError,
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
	c.Assert(statusInfo.Status, gc.Equals, status.StatusError)
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
	server1Addresses := []network.Address{{
		Value: "server-1",
		Type:  network.HostName,
		Scope: network.ScopePublic,
	}, {
		Value: "10.0.0.1",
		Type:  network.IPv4Address,
		Scope: network.ScopeCloudLocal,
	}}
	server2Addresses := []network.Address{{
		Value: "::1",
		Type:  network.IPv6Address,
		Scope: network.ScopeMachineLocal,
	}}
	stateAPIHostPorts := [][]network.HostPort{
		network.AddressesWithPort(server1Addresses, 123),
		network.AddressesWithPort(server2Addresses, 456),
	}

	err := s.State.SetAPIHostPorts(stateAPIHostPorts)
	c.Assert(err, jc.ErrorIsNil)
	apiHostPorts, err := s.APIState.Client().APIHostPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apiHostPorts, gc.DeepEquals, stateAPIHostPorts)
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
	c.Assert(err, gc.ErrorMatches, `some machines were not destroyed: machine 0 is required by the model; machine 1 has unit "wordpress/0" assigned`)
	assertLife(c, m0, state.Alive)
	assertLife(c, m1, state.Alive)
	assertLife(c, m2, state.Dying)

	err = u.UnassignFromMachine()
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().DestroyMachines("0", "1", "2")
	c.Assert(err, gc.ErrorMatches, `some machines were not destroyed: machine 0 is required by the model`)
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

func (s *clientSuite) TestBlockDestoryDestroyMachines(c *gc.C) {
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
	c.Assert(err, gc.ErrorMatches, `some machines were not destroyed: machine is required by the model`)
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
