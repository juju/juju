// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner_test

import (
	"io/ioutil"
	"path/filepath"
	"reflect"
	"time"

	"github.com/juju/juju/watcher/watchertest"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	apicaasprovisioner "github.com/juju/juju/api/caasoperatorprovisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasoperatorprovisioner"
	"github.com/juju/juju/worker/workertest"
)

var _ = gc.Suite(&CAASProvisionerSuite{})

type CAASProvisionerSuite struct {
	coretesting.BaseSuite
	stub *jujutesting.Stub

	clock             *jujutesting.Clock
	unitsChanges      chan struct{}
	provisionerFacade *mockProvisionerFacade
	caasClient        *mockBroker
	agentConfig       agent.Config
	modelTag          names.ModelTag
}

func (s *CAASProvisionerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = new(jujutesting.Stub)
	s.provisionerFacade = newMockProvisionerFacade(s.stub)
	s.unitsChanges = make(chan struct{})
	s.caasClient = &mockBroker{
		unitsWatcher: watchertest.NewMockNotifyWatcher(s.unitsChanges),
	}
	s.agentConfig = &mockAgentConfig{}
	s.modelTag = coretesting.ModelTag
}

func (s *CAASProvisionerSuite) waitForWorkerStubCalls(c *gc.C, expected []jujutesting.StubCall) {
	waitForStubCalls(c, s.stub, expected)
}

func waitForStubCalls(c *gc.C, stub *jujutesting.Stub, expected []jujutesting.StubCall) {
	var calls []jujutesting.StubCall
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		calls = stub.Calls()
		if reflect.DeepEqual(calls, expected) {
			return
		}
	}
	c.Fatalf("failed to see expected calls. saw: %v", calls)
}

func (s *CAASProvisionerSuite) assertWorker(c *gc.C) worker.Worker {
	s.clock = jujutesting.NewClock(time.Now())
	w, err := caasoperatorprovisioner.NewProvisionerWorker(caasoperatorprovisioner.Config{
		Facade:      s.provisionerFacade,
		Broker:      s.caasClient,
		ModelTag:    s.modelTag,
		AgentConfig: s.agentConfig,
		Clock:       s.clock})
	c.Assert(err, jc.ErrorIsNil)
	expected := []jujutesting.StubCall{
		{"WatchApplications", nil},
	}
	s.waitForWorkerStubCalls(c, expected)
	s.stub.ResetCalls()
	return w
}

func (s *CAASProvisionerSuite) TestWorkerStarts(c *gc.C) {
	w := s.assertWorker(c)
	workertest.CleanKill(c, w)
}

func (s *CAASProvisionerSuite) assertOperatorCreated(c *gc.C) {
	s.provisionerFacade.applicationsWatcher.changes <- []string{"myapp"}

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(s.caasClient.Calls()) > 0 {
			break
		}
	}
	s.caasClient.CheckCallNames(c, "EnsureOperator", "WatchUnits")

	args := s.caasClient.Calls()[0].Args
	c.Assert(args, gc.HasLen, 3)
	c.Assert(args[0], gc.Equals, "myapp")
	c.Assert(args[1], gc.Equals, "/var/lib/juju")
	c.Assert(args[2], gc.FitsTypeOf, &caas.OperatorConfig{})
	config := args[2].(*caas.OperatorConfig)
	c.Assert(s.caasClient.Calls()[1].Args, gc.DeepEquals, []interface{}{"myapp"})

	agentFile := filepath.Join(c.MkDir(), "agent.config")
	err := ioutil.WriteFile(agentFile, []byte(config.AgentConf), 0644)
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := agent.ReadConfig(agentFile)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.CACert(), gc.Equals, coretesting.CACert)
	addr, err := cfg.APIAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, jc.DeepEquals, []string{"10.0.0.1:17070"})

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(s.provisionerFacade.stub.Calls()) > 0 {
			break
		}
	}
	s.provisionerFacade.stub.CheckCallNames(c, "SetPasswords")
	passwords := s.provisionerFacade.stub.Calls()[0].Args[0].([]apicaasprovisioner.ApplicationPassword)

	c.Assert(passwords, gc.HasLen, 1)
	c.Assert(passwords[0].Name, gc.Equals, "myapp")
	c.Assert(passwords[0].Password, gc.Not(gc.Equals), "")
}

func (s *CAASProvisionerSuite) TestOperatorCreated(c *gc.C) {
	w := s.assertWorker(c)
	defer workertest.CleanKill(c, w)

	s.assertOperatorCreated(c)
}

func (s *CAASProvisionerSuite) TestUnitsChange(c *gc.C) {
	w := s.assertWorker(c)
	defer workertest.CleanKill(c, w)

	s.assertOperatorCreated(c)
	s.caasClient.ResetCalls()
	s.provisionerFacade.stub.ResetCalls()

	select {
	case s.unitsChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending units change")
	}

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(s.caasClient.Calls()) > 0 {
			break
		}
	}
	s.caasClient.CheckCallNames(c, "Units")
	c.Assert(s.caasClient.Calls()[0].Args, jc.DeepEquals, []interface{}{"myapp"})

	s.provisionerFacade.stub.CheckCallNames(c, "UpdateUnits")
	c.Assert(s.provisionerFacade.stub.Calls()[0].Args, jc.DeepEquals, []interface{}{
		params.UpdateApplicationUnits{
			ApplicationTag: names.NewApplicationTag("myapp").String(),
			Units: []params.ApplicationUnitParams{
				{Id: "u1", Address: "10.0.0.1", Ports: []string(nil), Status: "allocating"},
			},
		},
	})
}
