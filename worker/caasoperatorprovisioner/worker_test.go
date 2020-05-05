// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner_test

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"reflect"
	"strconv"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

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
	clock             *testclock.Clock
	modelTag          names.ModelTag
}

func (s *CAASProvisionerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = new(jujutesting.Stub)
	s.provisionerFacade = newMockProvisionerFacade(s.stub)
	s.caasClient = &mockBroker{}
	s.agentConfig = &mockAgentConfig{}
	s.modelTag = coretesting.ModelTag
	s.clock = testclock.NewClock(time.Now())
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
		Clock:       s.clock,
		Logger:      loggo.GetLogger("test"),
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

func (s *CAASProvisionerSuite) assertOperatorCreated(c *gc.C, exists, terminating, updateCerts bool) {
	s.caasClient.setTerminating(terminating)
	s.provisionerFacade.life = "alive"
	s.provisionerFacade.applicationsWatcher.changes <- []string{"myapp"}

	expectedCalls := 3
	if terminating {
		expectedCalls = 5
	}
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		nrCalls := len(s.caasClient.Calls())
		if nrCalls >= expectedCalls {
			break
		}
		if nrCalls > 0 {
			s.caasClient.setOperatorExists(false)
			s.caasClient.setTerminating(false)
			s.clock.Advance(4 * time.Second)
		}
	}
	callNames := []string{"OperatorExists", "Operator", "EnsureOperator"}
	if terminating {
		callNames = []string{"OperatorExists", "OperatorExists", "OperatorExists", "Operator", "EnsureOperator"}
	}
	s.caasClient.CheckCallNames(c, callNames...)
	c.Assert(s.caasClient.Calls(), gc.HasLen, expectedCalls)

	args := s.caasClient.Calls()[0].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], gc.Equals, "myapp")

	ensureIndex := 2
	if terminating {
		ensureIndex = 4
	}
	args = s.caasClient.Calls()[ensureIndex].Args
	c.Assert(args, gc.HasLen, 3)
	c.Assert(args[0], gc.Equals, "myapp")
	c.Assert(args[1], gc.Equals, "/var/lib/juju")
	c.Assert(args[2], gc.FitsTypeOf, &caas.OperatorConfig{})
	config := args[2].(*caas.OperatorConfig)
	c.Assert(config.OperatorImagePath, gc.Equals, "juju-operator-image")
	c.Assert(config.Version, gc.Equals, version.MustParse("2.99.0"))
	c.Assert(config.ResourceTags, jc.DeepEquals, map[string]string{"fred": "mary"})
	if s.provisionerFacade.withStorage {
		c.Assert(config.CharmStorage, jc.DeepEquals, &caas.CharmStorageParams{
			Provider:     "kubernetes",
			Size:         uint64(1024),
			ResourceTags: map[string]string{"foo": "bar"},
			Attributes:   map[string]interface{}{"key": "value"},
		})
	} else {
		c.Assert(config.CharmStorage, gc.IsNil)
	}
	if updateCerts {
		c.Assert(config.ConfigMapGeneration, gc.Equals, int64(1))
	} else {
		c.Assert(config.ConfigMapGeneration, gc.Equals, int64(0))
	}

	agentFile := filepath.Join(c.MkDir(), "agent.config")
	err := ioutil.WriteFile(agentFile, config.AgentConf, 0644)
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := agent.ReadConfig(agentFile)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.CACert(), gc.Equals, coretesting.CACert)
	addr, err := cfg.APIAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, jc.DeepEquals, []string{"10.0.0.1:17070", "192.18.1.1:17070"})

	operatorInfo, err := caas.UnmarshalOperatorInfo(config.OperatorInfo)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operatorInfo.CACert, gc.Equals, coretesting.CACert)
	c.Assert(operatorInfo.Cert, gc.Equals, coretesting.ServerCert)
	c.Assert(operatorInfo.PrivateKey, gc.Equals, coretesting.ServerKey)

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(s.provisionerFacade.stub.Calls()) > 0 {
			break
		}
	}

	if exists && !terminating {
		callNames := []string{"Life", "OperatorProvisioningInfo"}
		if updateCerts {
			callNames = append(callNames, "IssueOperatorCertificate")
		}
		s.provisionerFacade.stub.CheckCallNames(c, callNames...)
		c.Assert(s.provisionerFacade.stub.Calls()[0].Args[0], gc.Equals, "myapp")
		c.Assert(s.provisionerFacade.stub.Calls()[1].Args[0], gc.Equals, "myapp")
		return
	}

	s.provisionerFacade.stub.CheckCallNames(c, "Life", "OperatorProvisioningInfo", "IssueOperatorCertificate", "SetPasswords")
	c.Assert(s.provisionerFacade.stub.Calls()[0].Args[0], gc.Equals, "myapp")
	passwords := s.provisionerFacade.stub.Calls()[3].Args[0].([]apicaasprovisioner.ApplicationPassword)

	c.Assert(passwords, gc.HasLen, 1)
	c.Assert(passwords[0].Name, gc.Equals, "myapp")
	c.Assert(passwords[0].Password, gc.Not(gc.Equals), "")
}

func (s *CAASProvisionerSuite) TestNewApplicationCreatesNewOperator(c *gc.C) {
	w := s.assertWorker(c)
	defer workertest.CleanKill(c, w)

	s.assertOperatorCreated(c, false, false, false)
}

func (s *CAASProvisionerSuite) TestNewApplicationNoStorage(c *gc.C) {
	s.provisionerFacade.withStorage = false
	w := s.assertWorker(c)
	defer workertest.CleanKill(c, w)

	s.assertOperatorCreated(c, false, false, false)
}

func (s *CAASProvisionerSuite) TestNewApplicationUpdatesOperator(c *gc.C) {
	s.caasClient.operatorExists = true
	s.caasClient.config = &caas.OperatorConfig{
		OperatorImagePath: "juju-operator-image",
		Version:           version.MustParse("2.99.0"),
		AgentConf: []byte(fmt.Sprintf(`
# format 2.0
tag: application-myapp
upgradedToVersion: 2.99.0
controller: controller-deadbeef-1bad-500d-9000-4b1d0d06f00d
model: model-deadbeef-0bad-400d-8000-4b1d0d06f00d
oldpassword: wow
cacert: %s
apiaddresses:
- 10.0.0.1:17070
- 192.18.1.1:17070
oldpassword: dxKwhgZPrNzXVTrZSxY1VLHA
values: {}
mongoversion: "0.0"
`[1:], strconv.Quote(coretesting.CACert))),
		OperatorInfo: []byte(
			fmt.Sprintf(
				"private-key: %s\ncert: %s\nca-cert: %s\n",
				strconv.Quote(coretesting.ServerKey),
				strconv.Quote(coretesting.ServerCert),
				strconv.Quote(coretesting.CACert),
			),
		),
	}

	w := s.assertWorker(c)
	defer workertest.CleanKill(c, w)

	s.assertOperatorCreated(c, true, false, false)
}

func (s *CAASProvisionerSuite) TestNewApplicationUpdatesOperatorAndIssueCerts(c *gc.C) {
	s.caasClient.operatorExists = true
	s.caasClient.config = &caas.OperatorConfig{
		OperatorImagePath: "juju-operator-image",
		Version:           version.MustParse("2.99.0"),
		AgentConf: []byte(fmt.Sprintf(`
# format 2.0
tag: application-myapp
upgradedToVersion: 2.99.0
controller: controller-deadbeef-1bad-500d-9000-4b1d0d06f00d
model: model-deadbeef-0bad-400d-8000-4b1d0d06f00d
oldpassword: wow
cacert: %s
apiaddresses:
- 10.0.0.1:17070
- 192.18.1.1:17070
oldpassword: dxKwhgZPrNzXVTrZSxY1VLHA
values: {}
mongoversion: "0.0"
`[1:], strconv.Quote(coretesting.CACert))),
		ConfigMapGeneration: 1,
	}

	w := s.assertWorker(c)
	defer workertest.CleanKill(c, w)

	s.assertOperatorCreated(c, true, false, true)
}

func (s *CAASProvisionerSuite) TestNewApplicationWaitsOperatorTerminated(c *gc.C) {
	s.caasClient.operatorExists = true
	w := s.assertWorker(c)
	defer workertest.CleanKill(c, w)

	s.assertOperatorCreated(c, true, true, false)
}

func (s *CAASProvisionerSuite) TestApplicationDeletedRemovesOperator(c *gc.C) {
	w := s.assertWorker(c)
	defer workertest.CleanKill(c, w)

	s.assertOperatorCreated(c, false, false, false)
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
