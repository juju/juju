// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprovisioner_test

import (
	"io/ioutil"
	"path/filepath"
	"reflect"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasprovisioner"
	"github.com/juju/juju/worker/workertest"
)

var _ = gc.Suite(&CAASProvisionerSuite{})

type CAASProvisionerSuite struct {
	coretesting.BaseSuite
	stub *jujutesting.Stub

	provisionerFacade *mockProvisionerFacade
	caasClient        *mockBroker
	agentConfig       agent.Config
	modelTag          names.ModelTag
}

func (s *CAASProvisionerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = new(jujutesting.Stub)
	s.provisionerFacade = newMockProvisionerFacade(s.stub)
	s.caasClient = &mockBroker{}
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
	w, err := caasprovisioner.NewProvisionerWorker(s.provisionerFacade, s.caasClient, s.modelTag, s.agentConfig)
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

func (s *CAASProvisionerSuite) TestOperatorCreated(c *gc.C) {
	w := s.assertWorker(c)
	defer workertest.CleanKill(c, w)

	s.provisionerFacade.applicationsWatcher.changes <- []string{"myApp"}

	waitForResult := func() bool {
		if s.caasClient.appName != "myApp" {
			return false
		}
		if s.caasClient.agentPath != "/var/lib/juju" {
			return false
		}
		return true
	}
	gotResult := false
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if gotResult = waitForResult(); gotResult {
			return
		}
	}
	c.Assert(gotResult, jc.IsTrue)
	agentFile := filepath.Join(c.MkDir(), "agent.config")
	err := ioutil.WriteFile(agentFile, []byte(s.caasClient.config.AgentConf), 0644)
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := agent.ReadConfig(agentFile)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.CACert(), gc.Equals, coretesting.CACert)
	addr, err := cfg.APIAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, jc.DeepEquals, []string{"10.0.0.1:17070"})
}
