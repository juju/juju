// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner_test

import (
	"io/ioutil"
	"path/filepath"
	"reflect"

	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/agent"
	apicaasprovisioner "github.com/juju/juju/api/caasoperatorprovisioner"
	"github.com/juju/juju/caas"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasoperatorprovisioner"
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
	w, err := caasoperatorprovisioner.NewProvisionerWorker(caasoperatorprovisioner.Config{
		Facade:      s.provisionerFacade,
		Broker:      s.caasClient,
		ModelTag:    s.modelTag,
		AgentConfig: s.agentConfig,
	})
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

func (s *CAASProvisionerSuite) assertOperatorCreated(c *gc.C, exists bool) {
	s.provisionerFacade.life = "alive"
	s.provisionerFacade.applicationsWatcher.changes <- []string{"myapp"}

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(s.caasClient.Calls()) == 2 {
			break
		}
	}
	s.caasClient.CheckCallNames(c, "OperatorExists", "EnsureOperator")
	c.Assert(s.caasClient.Calls(), gc.HasLen, 2)

	args := s.caasClient.Calls()[0].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], gc.Equals, "myapp")

	args = s.caasClient.Calls()[1].Args
	c.Assert(args, gc.HasLen, 3)
	c.Assert(args[0], gc.Equals, "myapp")
	c.Assert(args[1], gc.Equals, "/var/lib/juju")
	c.Assert(args[2], gc.FitsTypeOf, &caas.OperatorConfig{})
	config := args[2].(*caas.OperatorConfig)
	c.Assert(config.OperatorImagePath, gc.Equals, "juju-operator-image")
	c.Assert(config.Version, gc.Equals, version.MustParse("2.99.0"))
	c.Assert(config.ResourceTags, jc.DeepEquals, map[string]string{"fred": "mary"})
	c.Assert(config.CharmStorage, jc.DeepEquals, caas.CharmStorageParams{
		Provider:     "kubernetes",
		Size:         uint64(1024),
		ResourceTags: map[string]string{"foo": "bar"},
		Attributes:   map[string]interface{}{"key": "value"},
	})

	if !exists {
		agentFile := filepath.Join(c.MkDir(), "agent.config")
		err := ioutil.WriteFile(agentFile, []byte(config.AgentConf), 0644)
		c.Assert(err, jc.ErrorIsNil)
		cfg, err := agent.ReadConfig(agentFile)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(cfg.CACert(), gc.Equals, coretesting.CACert)
		addr, err := cfg.APIAddresses()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(addr, jc.DeepEquals, []string{"10.0.0.1:17070", "192.18.1.1:17070"})
	} else {
		c.Assert(config.AgentConf, gc.IsNil)
	}

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(s.provisionerFacade.stub.Calls()) > 0 {
			break
		}
	}

	if exists {
		s.provisionerFacade.stub.CheckCallNames(c, "Life", "OperatorProvisioningInfo")
		c.Assert(s.provisionerFacade.stub.Calls()[0].Args[0], gc.Equals, "myapp")
		return
	}

	s.provisionerFacade.stub.CheckCallNames(c, "Life", "OperatorProvisioningInfo", "SetPasswords")
	c.Assert(s.provisionerFacade.stub.Calls()[0].Args[0], gc.Equals, "myapp")
	passwords := s.provisionerFacade.stub.Calls()[2].Args[0].([]apicaasprovisioner.ApplicationPassword)

	c.Assert(passwords, gc.HasLen, 1)
	c.Assert(passwords[0].Name, gc.Equals, "myapp")
	c.Assert(passwords[0].Password, gc.Not(gc.Equals), "")
}

func (s *CAASProvisionerSuite) TestNewApplicationCreatesNewOperator(c *gc.C) {
	w := s.assertWorker(c)
	defer workertest.CleanKill(c, w)

	s.assertOperatorCreated(c, false)
}

func (s *CAASProvisionerSuite) TestNewApplicationUpdatesOperator(c *gc.C) {
	s.caasClient.operatorExists = true
	w := s.assertWorker(c)
	defer workertest.CleanKill(c, w)

	s.assertOperatorCreated(c, true)
}

func (s *CAASProvisionerSuite) TestApplicationDeletedRemovesOperator(c *gc.C) {
	w := s.assertWorker(c)
	defer workertest.CleanKill(c, w)

	s.assertOperatorCreated(c, false)
	s.caasClient.ResetCalls()
	s.provisionerFacade.stub.SetErrors(errors.NotFoundf("myapp"))
	s.provisionerFacade.life = "dead"
	s.provisionerFacade.applicationsWatcher.changes <- []string{"myapp"}

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(s.caasClient.Calls()) > 0 {
			break
		}
	}
	s.caasClient.CheckCallNames(c, "DeleteOperator")
	c.Assert(s.caasClient.Calls()[0].Args[0], gc.Equals, "myapp")
}
