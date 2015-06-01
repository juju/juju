// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/leadership"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	agentcmd "github.com/juju/juju/cmd/jujud/agent"
	agenttesting "github.com/juju/juju/cmd/jujud/agent/testing"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/version"
)

// leadershipSuite tests the LeadershipService api calls.
type leadershipSuite struct {
	agenttesting.AgentSuite

	apiState     *api.State
	machineAgent *agentcmd.MachineAgent
	unitId       string
	serviceId    string
}

func (s *leadershipSuite) SetUpTest(c *gc.C) {

	s.AgentSuite.SetUpTest(c)

	file, _ := ioutil.TempFile("", "juju-run")
	defer file.Close()
	s.AgentSuite.PatchValue(&agentcmd.JujuRun, file.Name())

	agenttesting.InstallFakeEnsureMongo(s)
	s.PatchValue(&agentcmd.ProductionMongoWriteConcern, false)

	// Create a machine to manage the environment.
	stateServer, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		InstanceId: "id-1",
		Nonce:      agent.BootstrapNonce,
		Jobs:       []state.MachineJob{state.JobManageEnviron},
	})
	c.Assert(stateServer.PasswordValid(password), gc.Equals, true)
	c.Assert(stateServer.SetMongoPassword(password), jc.ErrorIsNil)

	// Create a machine to host some units.
	unitHostMachine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Nonce:    agent.BootstrapNonce,
		Password: password,
	})

	// Create a service and an instance of that service so that we can
	// create a client.
	service := s.Factory.MakeService(c, &factory.ServiceParams{})
	s.serviceId = service.Tag().Id()

	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Machine: unitHostMachine, Service: service})
	s.unitId = unit.UnitTag().Id()

	c.Assert(unit.SetPassword(password), jc.ErrorIsNil)
	s.apiState = s.OpenAPIAs(c, unit.Tag(), password)

	// Tweak and write out the config file for the state server.
	writeStateAgentConfig(
		c,
		s.MongoInfo(c),
		s.DataDir(),
		stateServer.Tag(),
		s.State.EnvironTag(),
		password,
		version.Current,
	)

	// Create & start a machine agent so the tests have something to call into.
	agentConf := agentcmd.NewAgentConf(s.DataDir())
	machineAgentFactory := agentcmd.MachineAgentFactoryFn(agentConf, agentConf)
	s.machineAgent = machineAgentFactory(stateServer.Id())

	// See comment in createMockJujudExecutable
	if runtime.GOOS == "windows" {
		dirToRemove := createMockJujudExecutable(c, s.DataDir(), s.machineAgent.Tag().String())
		s.AddCleanup(func(*gc.C) { os.RemoveAll(dirToRemove) })
	}

	c.Log("Starting machine agent...")
	go func() {
		err := s.machineAgent.Run(coretesting.Context(c))
		c.Assert(err, jc.ErrorIsNil)
	}()
}

func (s *leadershipSuite) TearDownTest(c *gc.C) {
	c.Log("Stopping machine agent...")
	err := s.machineAgent.Stop()
	c.Assert(err, jc.ErrorIsNil)
	os.RemoveAll(filepath.Join(s.DataDir(), "tools"))
	if s.apiState != nil {
		err := s.apiState.Close()
		c.Assert(err, jc.ErrorIsNil)
	}

	s.AgentSuite.TearDownTest(c)
}

func (s *leadershipSuite) TestClaimLeadership(c *gc.C) {

	client := leadership.NewClient(s.apiState)

	err := client.ClaimLeadership(s.serviceId, s.unitId, 10*time.Second)
	c.Assert(err, jc.ErrorIsNil)

	tokens, err := s.State.LeasePersistor.PersistedTokens()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tokens, gc.HasLen, 1)
	c.Assert(tokens[0].Namespace, gc.Equals, "mysql-leadership")
	c.Assert(tokens[0].Id, gc.Equals, "mysql/0")

	unblocked := make(chan struct{})
	go func() {
		err := client.BlockUntilLeadershipReleased(s.serviceId)
		c.Check(err, jc.ErrorIsNil)
		unblocked <- struct{}{}
	}()

	time.Sleep(coretesting.ShortWait)

	select {
	case <-time.After(15 * time.Second):
		c.Errorf("Timed out waiting for leadership to release.")
	case <-unblocked:
	}
}

func (s *leadershipSuite) TestReleaseLeadership(c *gc.C) {

	client := leadership.NewClient(s.apiState)

	err := client.ClaimLeadership(s.serviceId, s.unitId, 10*time.Second)
	c.Assert(err, jc.ErrorIsNil)

	err = client.ReleaseLeadership(s.serviceId, s.unitId)
	c.Assert(err, jc.ErrorIsNil)

	tokens, err := s.State.LeasePersistor.PersistedTokens()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tokens, gc.HasLen, 0)
}

func (s *leadershipSuite) TestUnblock(c *gc.C) {

	client := leadership.NewClient(s.apiState)

	err := client.ClaimLeadership(s.serviceId, s.unitId, 10*time.Second)
	c.Assert(err, jc.ErrorIsNil)

	unblocked := make(chan struct{})
	go func() {
		err := client.BlockUntilLeadershipReleased(s.serviceId)
		c.Check(err, jc.ErrorIsNil)
		unblocked <- struct{}{}
	}()

	time.Sleep(coretesting.ShortWait)

	err = client.ReleaseLeadership(s.serviceId, s.unitId)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-time.After(coretesting.LongWait):
		c.Errorf("Timed out waiting for leadership to release.")
	case <-unblocked:
	}
}

// uniterLeadershipSuite tests the Uniter api calls that make use of leadership
// features in the background.
type uniterLeadershipSuite struct {
	agenttesting.AgentSuite

	factory      *factory.Factory
	apiState     *api.State
	machineAgent *agentcmd.MachineAgent
	unitId       string
	serviceId    string
}

func (s *uniterLeadershipSuite) TestReadLeadershipSettings(c *gc.C) {

	// First, the unit must be elected leader; otherwise merges will be denied.
	leaderClient := leadership.NewClient(s.apiState)
	err := leaderClient.ClaimLeadership(s.serviceId, s.unitId, 10*time.Second)
	c.Assert(err, jc.ErrorIsNil)

	client := uniter.NewState(s.apiState, names.NewUnitTag(s.unitId))

	// Toss a few settings in.
	desiredSettings := map[string]string{
		"foo": "bar",
		"baz": "biz",
	}

	err = client.LeadershipSettings.Merge(s.serviceId, desiredSettings)
	c.Assert(err, jc.ErrorIsNil)

	settings, err := client.LeadershipSettings.Read(s.serviceId)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(settings, gc.DeepEquals, desiredSettings)
}

func (s *uniterLeadershipSuite) TestMergeLeadershipSettings(c *gc.C) {

	// First, the unit must be elected leader; otherwise merges will be denied.
	leaderClient := leadership.NewClient(s.apiState)
	err := leaderClient.ClaimLeadership(s.serviceId, s.unitId, 10*time.Second)
	c.Assert(err, jc.ErrorIsNil)

	client := uniter.NewState(s.apiState, names.NewUnitTag(s.unitId))

	// Grab what settings exist.
	settings, err := client.LeadershipSettings.Read(s.serviceId)
	c.Assert(err, jc.ErrorIsNil)
	// Double check that it's empty so that we don't pass the test by
	// happenstance.
	c.Assert(settings, gc.HasLen, 0)

	// Toss a few settings in.
	settings["foo"] = "bar"
	settings["baz"] = "biz"

	err = client.LeadershipSettings.Merge(s.serviceId, settings)
	c.Assert(err, jc.ErrorIsNil)

	settings, err = client.LeadershipSettings.Read(s.serviceId)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(settings["foo"], gc.Equals, "bar")
	c.Check(settings["baz"], gc.Equals, "biz")
}

func (s *uniterLeadershipSuite) TestSettingsChangeNotifier(c *gc.C) {

	// First, the unit must be elected leader; otherwise merges will be denied.
	leaderClient := leadership.NewClient(s.apiState)
	err := leaderClient.ClaimLeadership(s.serviceId, s.unitId, 10*time.Second)
	c.Assert(err, jc.ErrorIsNil)

	client := uniter.NewState(s.apiState, names.NewUnitTag(s.unitId))

	// Listen for changes
	watcher, err := client.LeadershipSettings.WatchLeadershipSettings(s.serviceId)
	c.Assert(err, jc.ErrorIsNil)

	defer statetesting.AssertStop(c, watcher)

	leadershipC := statetesting.NewNotifyWatcherC(c, s.BackingState, watcher)
	// Inital event
	leadershipC.AssertOneChange()

	// Make some changes
	err = client.LeadershipSettings.Merge(s.serviceId, map[string]string{"foo": "bar"})
	c.Assert(err, jc.ErrorIsNil)
	leadershipC.AssertOneChange()

	// And check that the changes were actually applied
	settings, err := client.LeadershipSettings.Read(s.serviceId)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(settings["foo"], gc.Equals, "bar")

	// Make a couple of changes, and then check that they have been
	// coalesced into a single event
	err = client.LeadershipSettings.Merge(s.serviceId, map[string]string{"foo": "baz"})
	c.Assert(err, jc.ErrorIsNil)
	err = client.LeadershipSettings.Merge(s.serviceId, map[string]string{"bing": "bong"})
	c.Assert(err, jc.ErrorIsNil)
	leadershipC.AssertOneChange()
}

func (s *uniterLeadershipSuite) SetUpTest(c *gc.C) {

	s.AgentSuite.SetUpTest(c)

	file, _ := ioutil.TempFile("", "juju-run")
	defer file.Close()
	s.AgentSuite.PatchValue(&agentcmd.JujuRun, file.Name())
	agenttesting.InstallFakeEnsureMongo(s)
	s.PatchValue(&agentcmd.ProductionMongoWriteConcern, false)

	s.factory = factory.NewFactory(s.State)

	// Create a machine to manage the environment, and set all
	// passwords to something known.
	stateServer, password := s.factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		InstanceId: "id-1",
		Nonce:      agent.BootstrapNonce,
		Jobs:       []state.MachineJob{state.JobManageEnviron},
	})
	c.Assert(stateServer.PasswordValid(password), gc.Equals, true)
	c.Assert(stateServer.SetMongoPassword(password), jc.ErrorIsNil)

	// Create a machine to host some units.
	unitHostMachine := s.factory.MakeMachine(c, &factory.MachineParams{
		Nonce:    agent.BootstrapNonce,
		Password: password,
	})

	// Create a service and an instance of that service so that we can
	// create a client.
	service := s.factory.MakeService(c, &factory.ServiceParams{})
	s.serviceId = service.Tag().Id()

	unit := s.factory.MakeUnit(c, &factory.UnitParams{Machine: unitHostMachine, Service: service})
	s.unitId = unit.UnitTag().Id()

	c.Assert(unit.SetPassword(password), jc.ErrorIsNil)
	s.apiState = s.OpenAPIAs(c, unit.Tag(), password)

	// Tweak and write out the config file for the state server.
	writeStateAgentConfig(
		c,
		s.MongoInfo(c),
		s.DataDir(),
		names.NewMachineTag(stateServer.Id()),
		s.State.EnvironTag(),
		password,
		version.Current,
	)

	// Create & start a machine agent so the tests have something to call into.
	agentConf := agentcmd.NewAgentConf(s.DataDir())
	machineAgentFactory := agentcmd.MachineAgentFactoryFn(agentConf, agentConf)
	s.machineAgent = machineAgentFactory(stateServer.Id())

	// See comment in createMockJujudExecutable
	if runtime.GOOS == "windows" {
		dirToRemove := createMockJujudExecutable(c, s.DataDir(), s.machineAgent.Tag().String())
		s.AddCleanup(func(*gc.C) { os.RemoveAll(dirToRemove) })
	}

	c.Log("Starting machine agent...")
	go func() {
		err := s.machineAgent.Run(coretesting.Context(c))
		c.Assert(err, jc.ErrorIsNil)
	}()
}

// When a machine agent is ran it creates a symlink to the jujud executable.
// Since we cannot create symlinks to a non-existent file on windows,
// we place a dummy executable in the expected location.
func createMockJujudExecutable(c *gc.C, dir, tag string) string {
	toolsDir := filepath.Join(dir, "tools")
	err := os.MkdirAll(filepath.Join(toolsDir, tag), 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(filepath.Join(toolsDir, tag, "jujud.exe"),
		[]byte("echo 1"), 0777)
	c.Assert(err, jc.ErrorIsNil)
	return toolsDir
}

func (s *uniterLeadershipSuite) TearDownTest(c *gc.C) {
	c.Log("Stopping machine agent...")
	err := s.machineAgent.Stop()
	c.Check(err, jc.ErrorIsNil)
	if s.apiState != nil {
		err := s.apiState.Close()
		c.Assert(err, jc.ErrorIsNil)
	}
	s.AgentSuite.TearDownTest(c)
}

func writeStateAgentConfig(
	c *gc.C,
	stateInfo *mongo.MongoInfo,
	dataDir string,
	tag names.Tag,
	environTag names.EnvironTag,
	password string,
	vers version.Binary,
) agent.ConfigSetterWriter {

	port := gitjujutesting.FindTCPPort()
	apiAddr := []string{fmt.Sprintf("localhost:%d", port)}
	conf, err := agent.NewStateMachineConfig(
		agent.AgentConfigParams{
			DataDir:           dataDir,
			Tag:               tag,
			Environment:       environTag,
			UpgradedToVersion: vers.Number,
			Password:          password,
			Nonce:             agent.BootstrapNonce,
			StateAddresses:    stateInfo.Addrs,
			APIAddresses:      apiAddr,
			CACert:            stateInfo.CACert,
		},
		params.StateServingInfo{
			Cert:         coretesting.ServerCert,
			PrivateKey:   coretesting.ServerKey,
			CAPrivateKey: coretesting.CAKey,
			StatePort:    gitjujutesting.MgoServer.Port(),
			APIPort:      port,
		})
	c.Assert(err, jc.ErrorIsNil)
	conf.SetPassword(password)
	c.Assert(conf.Write(), jc.ErrorIsNil)
	return conf
}
