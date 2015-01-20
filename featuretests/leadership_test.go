// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"fmt"
	"io/ioutil"
	"time"

	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/leadership"
	leadershipapi "github.com/juju/juju/apiserver/leadership"
	"github.com/juju/juju/apiserver/params"
	agentcmd "github.com/juju/juju/cmd/jujud/agent"
	agenttesting "github.com/juju/juju/cmd/jujud/agent/testing"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/version"
)

type leadershipSuite struct {
	agenttesting.AgentSuite

	clientFacade base.ClientFacade
	facadeCaller base.FacadeCaller
	machineAgent *agentcmd.MachineAgent
	unitId       string
	serviceId    string
}

func (s *leadershipSuite) SetUpTest(c *gc.C) {

	s.AgentSuite.SetUpTest(c)

	file, _ := ioutil.TempFile("", "juju-run")
	defer file.Close()
	s.AgentSuite.PatchValue(&agentcmd.JujuRun, file.Name())

	fakeEnsureMongo := agenttesting.FakeEnsure{}
	s.AgentSuite.PatchValue(&cmdutil.EnsureMongoServer, fakeEnsureMongo.FakeEnsureMongo)

	// Create a machine to manage the environment, and set all
	// passwords to something known.
	const password = "machine-password-1234567890"
	stateServer := s.Factory.MakeMachine(c, &factory.MachineParams{
		InstanceId: "id-1",
		Nonce:      agent.BootstrapNonce,
		Jobs:       []state.MachineJob{state.JobManageEnviron},
		Password:   password,
	})
	c.Assert(stateServer.PasswordValid(password), gc.Equals, true)
	c.Assert(stateServer.SetMongoPassword(password), gc.IsNil)

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

	c.Assert(unit.SetPassword(password), gc.IsNil)
	unitState := s.OpenAPIAs(c, unit.Tag(), password)

	// Create components needed to construct a client.
	s.clientFacade, s.facadeCaller = base.NewClientFacade(unitState, leadershipapi.FacadeName)
	c.Assert(s.clientFacade, gc.NotNil)
	c.Assert(s.facadeCaller, gc.NotNil)

	// Tweak and write out the config file for the state server.
	s.writeStateAgentConfig(
		c,
		stateServer.Tag(),
		password,
		version.Current,
	)

	// Create & start a machine agent so the tests have something to call into.
	agentConf := agentcmd.AgentConf{DataDir: s.DataDir()}
	machineAgentFactory := agentcmd.MachineAgentFactoryFn(&agentConf, &agentConf)
	s.machineAgent = machineAgentFactory(stateServer.Id())

	c.Log("Starting machine agent...")
	go func() {
		err := s.machineAgent.Run(coretesting.Context(c))
		c.Assert(err, gc.IsNil)
	}()
}

func (s *leadershipSuite) TearDownTest(c *gc.C) {
	c.Log("Stopping machine agent...")
	err := s.machineAgent.Stop()
	c.Assert(err, gc.IsNil)

	s.AgentSuite.TearDownTest(c)
}

func (s *leadershipSuite) TestClaimLeadership(c *gc.C) {

	client := leadership.NewClient(s.clientFacade, s.facadeCaller)
	defer func() { err := client.Close(); c.Assert(err, gc.IsNil) }()

	duration, err := client.ClaimLeadership(s.serviceId, s.unitId)

	c.Assert(err, gc.IsNil)
	c.Check(duration, gc.Equals, 30*time.Second)
}

func (s *leadershipSuite) TestReleaseLeadership(c *gc.C) {

	client := leadership.NewClient(s.clientFacade, s.facadeCaller)
	defer func() { err := client.Close(); c.Assert(err, gc.IsNil) }()

	_, err := client.ClaimLeadership(s.serviceId, s.unitId)
	c.Assert(err, gc.IsNil)

	err = client.ReleaseLeadership(s.serviceId, s.unitId)
	c.Assert(err, gc.IsNil)
}

func (s *leadershipSuite) TestUnblock(c *gc.C) {

	client := leadership.NewClient(s.clientFacade, s.facadeCaller)
	defer func() { err := client.Close(); c.Assert(err, gc.IsNil) }()

	_, err := client.ClaimLeadership(s.serviceId, s.unitId)
	c.Assert(err, gc.IsNil)

	unblocked := make(chan struct{})
	go func() {
		err = client.BlockUntilLeadershipReleased(s.serviceId)
		c.Check(err, gc.IsNil)
		unblocked <- struct{}{}
	}()

	time.Sleep(coretesting.ShortWait)

	err = client.ReleaseLeadership(s.serviceId, s.unitId)
	c.Assert(err, gc.IsNil)

	select {
	case <-time.After(coretesting.LongWait):
		c.Errorf("Timed out waiting for leadership to release.")
	case <-unblocked:
	}
}

func (s *leadershipSuite) writeStateAgentConfig(
	c *gc.C,
	tag names.Tag,
	password string,
	vers version.Binary,
) agent.ConfigSetterWriter {

	stateInfo := s.MongoInfo(c)
	port := gitjujutesting.FindTCPPort()
	apiAddr := []string{fmt.Sprintf("localhost:%d", port)}
	conf, err := agent.NewStateMachineConfig(
		agent.AgentConfigParams{
			DataDir:           s.DataDir(),
			Tag:               tag,
			Environment:       s.State.EnvironTag(),
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
	c.Assert(err, gc.IsNil)
	conf.SetPassword(password)
	c.Assert(conf.Write(), gc.IsNil)
	return conf
}
