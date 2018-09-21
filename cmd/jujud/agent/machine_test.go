// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/juju/cmd/jujud/agent/agenttest"
	"github.com/juju/os/series"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/cert"
	"github.com/juju/utils/ssh"
	sshtesting "github.com/juju/utils/ssh/testing"
	"github.com/juju/utils/symlink"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charmrepo.v3"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"
	"gopkg.in/juju/worker.v1/workertest"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apimachiner "github.com/juju/juju/api/machiner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/jujud/agent/model"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/authenticationworker"
	"github.com/juju/juju/worker/diskmanager"
	"github.com/juju/juju/worker/instancepoller"
	"github.com/juju/juju/worker/machiner"
	"github.com/juju/juju/worker/migrationmaster"
	"github.com/juju/juju/worker/storageprovisioner"
	"github.com/juju/juju/worker/upgrader"
)

type MachineSuite struct {
	commonMachineSuite
}

var _ = gc.Suite(&MachineSuite{})

func (s *MachineSuite) SetUpTest(c *gc.C) {
	s.ControllerConfigAttrs = map[string]interface{}{
		controller.AuditingEnabled: true,
		controller.CharmStoreURL:   "staging.charmstore",
		// TODO(raftlease): setting this temporarily until the startup
		// issue is resolved.
		controller.Features: []interface{}{"legacy-leases"},
	}
	s.commonMachineSuite.SetUpTest(c)
	// Most of these tests normally finish sub-second on a fast machine.
	// If any given test hits a minute, we have almost certainly become
	// wedged, so dump the logs.
	coretesting.DumpTestLogsAfter(time.Minute, c, s)
}

func (s *MachineSuite) TestParseNonsense(c *gc.C) {
	for _, args := range [][]string{
		{},
		{"--machine-id", "-4004"},
	} {
		var agentConf agentConf
		err := ParseAgentCommand(&machineAgentCmd{agentInitializer: &agentConf}, args)
		c.Assert(err, gc.ErrorMatches, "--machine-id option must be set, and expects a non-negative integer")
	}
}

func (s *MachineSuite) TestParseUnknown(c *gc.C) {
	var agentConf agentConf
	a := &machineAgentCmd{agentInitializer: &agentConf}
	err := ParseAgentCommand(a, []string{"--machine-id", "42", "blistering barnacles"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["blistering barnacles"\]`)
}

func (s *MachineSuite) TestParseSuccess(c *gc.C) {
	create := func() (cmd.Command, AgentConf) {
		agentConf := agentConf{dataDir: s.DataDir()}
		logger := s.newBufferedLogWriter()
		a := NewMachineAgentCmd(
			nil,
			NewTestMachineAgentFactory(&agentConf, logger, c.MkDir()),
			&agentConf,
			&agentConf,
		)
		a.(*machineAgentCmd).logToStdErr = true

		return a, &agentConf
	}
	a := CheckAgentCommand(c, create, []string{"--machine-id", "42"})
	c.Assert(a.(*machineAgentCmd).machineId, gc.Equals, "42")
}

func (s *MachineSuite) TestRunInvalidMachineId(c *gc.C) {
	c.Skip("agents don't yet distinguish between temporary and permanent errors")
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	err := s.newAgent(c, m).Run(nil)
	c.Assert(err, gc.ErrorMatches, "some error")
}

func (s *MachineSuite) TestUseLumberjack(c *gc.C) {
	ctx := cmdtesting.Context(c)
	agentConf := FakeAgentConfig{}
	logger := s.newBufferedLogWriter()

	a := NewMachineAgentCmd(
		ctx,
		NewTestMachineAgentFactory(&agentConf, logger, c.MkDir()),
		agentConf,
		agentConf,
	)
	// little hack to set the data that Init expects to already be set
	a.(*machineAgentCmd).machineId = "42"

	err := a.Init(nil)
	c.Assert(err, gc.IsNil)

	l, ok := ctx.Stderr.(*lumberjack.Logger)
	c.Assert(ok, jc.IsTrue)
	c.Check(l.MaxAge, gc.Equals, 0)
	c.Check(l.MaxBackups, gc.Equals, 2)
	c.Check(l.Filename, gc.Equals, filepath.FromSlash("/var/log/juju/machine-42.log"))
	c.Check(l.MaxSize, gc.Equals, 300)
}

func (s *MachineSuite) TestDontUseLumberjack(c *gc.C) {
	ctx := cmdtesting.Context(c)
	agentConf := FakeAgentConfig{}
	logger := s.newBufferedLogWriter()

	a := NewMachineAgentCmd(
		ctx,
		NewTestMachineAgentFactory(&agentConf, logger, c.MkDir()),
		agentConf,
		agentConf,
	)
	// little hack to set the data that Init expects to already be set
	a.(*machineAgentCmd).machineId = "42"

	// set the value that normally gets set by the flag parsing
	a.(*machineAgentCmd).logToStdErr = true

	err := a.Init(nil)
	c.Assert(err, gc.IsNil)

	_, ok := ctx.Stderr.(*lumberjack.Logger)
	c.Assert(ok, jc.IsFalse)
}

func (s *MachineSuite) TestRunStop(c *gc.C) {
	m, ac, _ := s.primeAgent(c, state.JobHostUnits)
	a := s.newAgent(c, m)
	done := make(chan error)
	go func() {
		done <- a.Run(nil)
	}()
	err := a.Stop()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(<-done, jc.ErrorIsNil)
	c.Assert(charmrepo.CacheDir, gc.Equals, filepath.Join(ac.DataDir(), "charmcache"))
}

func (s *MachineSuite) TestWithDeadMachine(c *gc.C) {
	m, ac, _ := s.primeAgent(c, state.JobHostUnits)
	err := m.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	a := s.newAgent(c, m)
	err = runWithTimeout(a)
	c.Assert(err, jc.ErrorIsNil)

	_, err = os.Stat(ac.DataDir())
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (s *MachineSuite) TestWithRemovedMachine(c *gc.C) {
	m, ac, _ := s.primeAgent(c, state.JobHostUnits)
	err := m.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = m.Remove()
	c.Assert(err, jc.ErrorIsNil)
	a := s.newAgent(c, m)
	err = runWithTimeout(a)
	c.Assert(err, jc.ErrorIsNil)

	_, err = os.Stat(ac.DataDir())
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (s *MachineSuite) TestDyingMachine(c *gc.C) {
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	a := s.newAgent(c, m)
	done := make(chan error)
	go func() {
		done <- a.Run(nil)
	}()
	defer func() {
		c.Check(a.Stop(), jc.ErrorIsNil)
	}()
	// Wait for configuration to be finished
	<-a.WorkersStarted()
	err := m.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	// Tearing down the dependency engine can take a non-trivial amount of
	// time.
	select {
	case err := <-done:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		// This test intermittently fails and we haven't been able to determine
		// why it gets wedged. So we will dump the goroutines before the fatal call.
		buff := bytes.Buffer{}
		err = pprof.Lookup("goroutine").WriteTo(&buff, 1)
		c.Check(err, jc.ErrorIsNil)
		c.Logf("\nagent didn't stop, here's what it was doing\n\n%s", buff)
		c.Fatalf("timed out waiting for agent to terminate")
	}
	err = m.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Life(), gc.Equals, state.Dead)
}

func (s *MachineSuite) TestManageModelRunsInstancePoller(c *gc.C) {
	s.AgentSuite.PatchValue(&instancepoller.ShortPoll, 500*time.Millisecond)
	usefulVersion := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: "quantal", // to match the charm created below
	}
	envtesting.AssertUploadFakeToolsVersions(
		c, s.DefaultToolsStorage,
		s.Environ.Config().AgentStream(),
		s.Environ.Config().AgentStream(),
		usefulVersion,
	)
	m, _, _ := s.primeAgent(c, state.JobManageModel)
	a := s.newAgent(c, m)
	defer a.Stop()
	go func() {
		c.Check(a.Run(nil), jc.ErrorIsNil)
	}()

	// Add one unit to an application;
	charm := s.AddTestingCharm(c, "dummy")
	app := s.AddTestingApplication(c, "test-application", charm)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	m, instId := s.waitProvisioned(c, unit)
	insts, err := s.Environ.Instances(context.NewCloudCallContext(), []instance.Id{instId})
	c.Assert(err, jc.ErrorIsNil)
	addrs := network.NewAddresses("1.2.3.4")
	dummy.SetInstanceAddresses(insts[0], addrs)
	dummy.SetInstanceStatus(insts[0], "running")

	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		if !attempt.HasNext() {
			c.Logf("final machine addresses: %#v", m.Addresses())
			c.Fatalf("timed out waiting for machine to get address")
		}
		err := m.Refresh()
		c.Assert(err, jc.ErrorIsNil)
		instStatus, err := m.InstanceStatus()
		c.Assert(err, jc.ErrorIsNil)
		c.Logf("found status is %q %q", instStatus.Status, instStatus.Message)
		if reflect.DeepEqual(m.Addresses(), addrs) && instStatus.Message == "running" {
			c.Logf("machine %q address updated: %+v", m.Id(), addrs)
			break
		}
		c.Logf("waiting for machine %q address to be updated", m.Id())
	}
}

func (s *MachineSuite) TestCallsUseMultipleCPUs(c *gc.C) {
	// All machine agents call UseMultipleCPUs.
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	calledChan := make(chan struct{}, 1)
	s.AgentSuite.PatchValue(&useMultipleCPUs, func() { calledChan <- struct{}{} })
	a := s.newAgent(c, m)
	defer a.Stop()
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()

	// Wait for configuration to be finished
	<-a.WorkersStarted()
	s.assertChannelActive(c, calledChan, "UseMultipleCPUs() to be called")
	c.Check(a.Stop(), jc.ErrorIsNil)
}

func (s *MachineSuite) waitProvisioned(c *gc.C, unit *state.Unit) (*state.Machine, instance.Id) {
	c.Logf("waiting for unit %q to be provisioned", unit)
	machineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)
	w := m.Watch()
	defer worker.Stop(w)
	timeout := time.After(coretesting.LongWait)
	for {
		select {
		case <-timeout:
			c.Fatalf("timed out waiting for provisioning")
		case <-time.After(coretesting.ShortWait):
			s.State.StartSync()
		case _, ok := <-w.Changes():
			c.Assert(ok, jc.IsTrue)
			err := m.Refresh()
			c.Assert(err, jc.ErrorIsNil)
			if instId, err := m.InstanceId(); err == nil {
				c.Logf("unit provisioned with instance %s", instId)
				return m, instId
			} else {
				c.Check(err, jc.Satisfies, errors.IsNotProvisioned)
			}
		}
	}
}

func (s *MachineSuite) testUpgradeRequest(c *gc.C, agent runner, tag string, currentTools *tools.Tools) {
	newVers := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: series.MustHostSeries(),
	}
	newVers.Patch++
	newTools := envtesting.AssertUploadFakeToolsVersions(
		c, s.DefaultToolsStorage, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(), newVers)[0]
	err := s.State.SetModelAgentVersion(newVers.Number, true)
	c.Assert(err, jc.ErrorIsNil)
	err = runWithTimeout(agent)
	envtesting.CheckUpgraderReadyError(c, err, &upgrader.UpgradeReadyError{
		AgentName: tag,
		OldTools:  currentTools.Version,
		NewTools:  newTools.Version,
		DataDir:   s.DataDir(),
	})
}

func (s *MachineSuite) TestUpgradeRequest(c *gc.C) {
	m, _, currentTools := s.primeAgent(c, state.JobManageModel, state.JobHostUnits)
	a := s.newAgent(c, m)
	s.testUpgradeRequest(c, a, m.Tag().String(), currentTools)
	c.Assert(a.initialUpgradeCheckComplete.IsUnlocked(), jc.IsFalse)
}

func (s *MachineSuite) TestNoUpgradeRequired(c *gc.C) {
	m, _, _ := s.primeAgent(c, state.JobManageModel, state.JobHostUnits)
	a := s.newAgent(c, m)
	done := make(chan error)
	go func() { done <- a.Run(nil) }()
	select {
	case <-a.initialUpgradeCheckComplete.Unlocked():
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout waiting for upgrade check")
	}
	defer a.Stop() // in case of failure
	s.waitStopped(c, state.JobManageModel, a, done)
	c.Assert(a.initialUpgradeCheckComplete.IsUnlocked(), jc.IsTrue)
}

func (s *MachineSuite) waitStopped(c *gc.C, job state.MachineJob, a *MachineAgent, done chan error) {
	err := a.Stop()
	if job == state.JobManageModel {
		// When shutting down, the API server can be shut down before
		// the other workers that connect to it, so they get an error so
		// they then die, causing Stop to return an error.  It's not
		// easy to control the actual error that's received in this
		// circumstance so we just log it rather than asserting that it
		// is not nil.
		if err != nil {
			c.Logf("error shutting down state manager: %v", err)
		}
	} else {
		c.Assert(err, jc.ErrorIsNil)
	}

	select {
	case err := <-done:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for agent to terminate")
	}
}

func (s *MachineSuite) assertJobWithState(
	c *gc.C,
	job state.MachineJob,
	test func(agent.Config, *state.State),
) {
	paramsJob := job.ToParams()
	if !paramsJob.NeedsState() {
		c.Fatalf("%v does not use state", paramsJob)
	}
	s.assertAgentOpensState(c, job, test)
}

// assertAgentOpensState asserts that a machine agent started with the
// given job. The agent's configuration and the agent's state.State are
// then passed to the test function for further checking.
func (s *MachineSuite) assertAgentOpensState(c *gc.C, job state.MachineJob, test func(agent.Config, *state.State)) {
	stm, conf, _ := s.primeAgent(c, job)
	a := s.newAgent(c, stm)
	defer a.Stop()
	logger.Debugf("new agent %#v", a)

	// All state jobs currently also run an APIWorker, so no
	// need to check for that here, like in assertJobWithState.
	st, done := s.waitForOpenState(c, a)
	test(conf, st)
	s.waitStopped(c, job, a, done)
}

func (s *MachineSuite) waitForOpenState(c *gc.C, a *MachineAgent) (*state.State, chan error) {
	agentAPIs := make(chan *state.State, 1)
	s.AgentSuite.PatchValue(&reportOpenedState, func(st *state.State) {
		select {
		case agentAPIs <- st:
		default:
		}
	})

	done := make(chan error)
	go func() {
		done <- a.Run(nil)
	}()

	select {
	case agentAPI := <-agentAPIs:
		c.Assert(agentAPI, gc.NotNil)
		return agentAPI, done
	case <-time.After(coretesting.LongWait):
		c.Fatalf("API not opened")
	}
	panic("can't happen")
}

func (s *MachineSuite) TestManageModelServesAPI(c *gc.C) {
	s.assertJobWithState(c, state.JobManageModel, func(conf agent.Config, agentState *state.State) {
		apiInfo, ok := conf.APIInfo()
		c.Assert(ok, jc.IsTrue)
		st, err := api.Open(apiInfo, fastDialOpts)
		c.Assert(err, jc.ErrorIsNil)
		defer st.Close()
		m, err := apimachiner.NewState(st).Machine(conf.Tag().(names.MachineTag))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(m.Life(), gc.Equals, params.Alive)
	})
}

func (s *MachineSuite) TestManageModelAuditsAPI(c *gc.C) {
	password := "shhh..."
	user := s.Factory.MakeUser(c, &factory.UserParams{
		Password: password,
	})

	err := s.State.UpdateControllerConfig(map[string]interface{}{
		"audit-log-exclude-methods": []interface{}{"Client.FullStatus"},
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.assertJobWithState(c, state.JobManageModel, func(conf agent.Config, agentState *state.State) {
		logPath := filepath.Join(conf.LogDir(), "audit.log")

		makeAPIRequest := func(doRequest func(*api.Client)) {
			apiInfo, ok := conf.APIInfo()
			c.Assert(ok, jc.IsTrue)
			apiInfo.Tag = user.Tag()
			apiInfo.Password = password
			st, err := api.Open(apiInfo, fastDialOpts)
			c.Assert(err, jc.ErrorIsNil)
			defer st.Close()
			doRequest(st.Client())
		}

		// Make requests in separate API connections so they're separate conversations.
		makeAPIRequest(func(client *api.Client) {
			_, err = client.Status(nil)
			c.Assert(err, jc.ErrorIsNil)
		})
		makeAPIRequest(func(client *api.Client) {
			_, err = client.AddMachines([]params.AddMachineParams{{
				Jobs: []multiwatcher.MachineJob{"JobHostUnits"},
			}})
			c.Assert(err, jc.ErrorIsNil)
		})

		// Check that there's a call to Client.AddMachinesV2 in the
		// log, but no call to Client.FullStatus.
		records := readAuditLog(c, logPath)
		c.Assert(records, gc.HasLen, 3)
		c.Assert(records[1].Request, gc.NotNil)
		c.Assert(records[1].Request.Facade, gc.Equals, "Client")
		c.Assert(records[1].Request.Method, gc.Equals, "AddMachinesV2")

		// Now update the controller config to remove the exclusion.
		err := s.State.UpdateControllerConfig(map[string]interface{}{
			"audit-log-exclude-methods": []interface{}{},
		}, nil)
		c.Assert(err, jc.ErrorIsNil)

		prevRecords := len(records)

		// We might need to wait until the controller config change is
		// propagated to the apiserver.
		for a := coretesting.LongAttempt.Start(); a.Next(); {
			makeAPIRequest(func(client *api.Client) {
				_, err = client.Status(nil)
				c.Assert(err, jc.ErrorIsNil)
			})
			// Check to see whether there are more logged requests.
			records = readAuditLog(c, logPath)
			if prevRecords < len(records) {
				break
			}
		}
		// Now there should also be a call to Client.FullStatus (and a response).
		lastRequest := records[len(records)-2]
		c.Assert(lastRequest.Request, gc.NotNil)
		c.Assert(lastRequest.Request.Facade, gc.Equals, "Client")
		c.Assert(lastRequest.Request.Method, gc.Equals, "FullStatus")
	})
}

func readAuditLog(c *gc.C, logPath string) []auditlog.Record {
	file, err := os.Open(logPath)
	c.Assert(err, jc.ErrorIsNil)
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var results []auditlog.Record
	for scanner.Scan() {
		var record auditlog.Record
		err := json.Unmarshal(scanner.Bytes(), &record)
		c.Assert(err, jc.ErrorIsNil)
		results = append(results, record)
	}
	return results
}

func (s *MachineSuite) assertAgentSetsToolsVersion(c *gc.C, job state.MachineJob) {
	vers := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: series.MustHostSeries(),
	}
	vers.Minor++
	m, _, _ := s.primeAgentVersion(c, vers, job)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	timeout := time.After(coretesting.LongWait)
	for done := false; !done; {
		select {
		case <-timeout:
			c.Fatalf("timeout while waiting for agent version to be set")
		case <-time.After(coretesting.ShortWait):
			c.Log("Refreshing")
			err := m.Refresh()
			c.Assert(err, jc.ErrorIsNil)
			c.Log("Fetching agent tools")
			agentTools, err := m.AgentTools()
			c.Assert(err, jc.ErrorIsNil)
			c.Logf("(%v vs. %v)", agentTools.Version, jujuversion.Current)
			if agentTools.Version.Minor != jujuversion.Current.Minor {
				continue
			}
			c.Assert(agentTools.Version.Number, gc.DeepEquals, jujuversion.Current)
			done = true
		}
	}
}

func (s *MachineSuite) TestAgentSetsToolsVersionManageModel(c *gc.C) {
	s.assertAgentSetsToolsVersion(c, state.JobManageModel)
}

func (s *MachineSuite) TestAgentSetsToolsVersionHostUnits(c *gc.C) {
	s.assertAgentSetsToolsVersion(c, state.JobHostUnits)
}

func (s *MachineSuite) TestManageModelRunsCleaner(c *gc.C) {
	s.assertJobWithState(c, state.JobManageModel, func(conf agent.Config, agentState *state.State) {
		// Create an application and unit, and destroy the app.
		app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
		unit, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		err = app.Destroy()
		c.Assert(err, jc.ErrorIsNil)

		// Check the unit was not yet removed.
		err = unit.Refresh()
		c.Assert(err, jc.ErrorIsNil)
		w := unit.Watch()
		defer worker.Stop(w)

		// Trigger a sync on the state used by the agent, and wait
		// for the unit to be removed.
		agentState.StartSync()
		timeout := time.After(coretesting.LongWait)
		for done := false; !done; {
			select {
			case <-timeout:
				c.Fatalf("unit not cleaned up")
			case <-time.After(coretesting.ShortWait):
				s.State.StartSync()
			case <-w.Changes():
				err := unit.Refresh()
				if errors.IsNotFound(err) {
					done = true
				} else {
					c.Assert(err, jc.ErrorIsNil)
				}
			}
		}
	})
}

func (s *MachineSuite) TestJobManageModelRunsMinUnitsWorker(c *gc.C) {
	s.assertJobWithState(c, state.JobManageModel, func(_ agent.Config, agentState *state.State) {
		// Ensure that the MinUnits worker is alive by doing a simple check
		// that it responds to state changes: add an application, set its minimum
		// number of units to one, wait for the worker to add the missing unit.
		app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
		err := app.SetMinUnits(1)
		c.Assert(err, jc.ErrorIsNil)
		w := app.Watch()
		defer worker.Stop(w)

		// Trigger a sync on the state used by the agent, and wait for the unit
		// to be created.
		agentState.StartSync()
		timeout := time.After(coretesting.LongWait)
		for {
			select {
			case <-timeout:
				c.Fatalf("unit not created")
			case <-time.After(coretesting.ShortWait):
				s.State.StartSync()
			case <-w.Changes():
				units, err := app.AllUnits()
				c.Assert(err, jc.ErrorIsNil)
				if len(units) == 1 {
					return
				}
			}
		}
	})
}

func (s *MachineSuite) TestMachineAgentRunsAuthorisedKeysWorker(c *gc.C) {
	//TODO(bogdanteleaga): Fix once we get authentication worker up on windows
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: authentication worker not yet implemented on windows")
	}
	// Start the machine agent.
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	// Update the keys in the environment.
	sshKey := sshtesting.ValidKeyOne.Key + " user@host"
	err := s.Model.UpdateModelConfig(map[string]interface{}{"authorized-keys": sshKey}, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Wait for ssh keys file to be updated.
	s.State.StartSync()
	timeout := time.After(coretesting.LongWait)
	sshKeyWithCommentPrefix := sshtesting.ValidKeyOne.Key + " Juju:user@host"
	for {
		select {
		case <-timeout:
			c.Fatalf("timeout while waiting for authorised ssh keys to change")
		case <-time.After(coretesting.ShortWait):
			s.State.StartSync()
			keys, err := ssh.ListKeys(authenticationworker.SSHUser, ssh.FullKeys)
			c.Assert(err, jc.ErrorIsNil)
			keysStr := strings.Join(keys, "\n")
			if sshKeyWithCommentPrefix != keysStr {
				continue
			}
			return
		}
	}
}

func (s *MachineSuite) TestMachineAgentSymlinks(c *gc.C) {
	stm, _, _ := s.primeAgent(c, state.JobManageModel)
	a := s.newAgent(c, stm)
	defer a.Stop()
	_, done := s.waitForOpenState(c, a)

	// Symlinks should have been created
	for _, link := range jujudSymlinks {
		_, err := os.Stat(utils.EnsureBaseDir(a.rootDir, link))
		c.Assert(err, jc.ErrorIsNil, gc.Commentf(link))
	}

	s.waitStopped(c, state.JobManageModel, a, done)
}

func (s *MachineSuite) TestMachineAgentSymlinkJujuRunExists(c *gc.C) {
	if runtime.GOOS == "windows" {
		// Cannot make symlink to nonexistent file on windows or
		// create a file point a symlink to it then remove it
		c.Skip("Cannot test this on windows")
	}

	stm, _, _ := s.primeAgent(c, state.JobManageModel)
	a := s.newAgent(c, stm)
	defer a.Stop()

	// Pre-create the symlinks, but pointing to the incorrect location.
	a.rootDir = c.MkDir()
	for _, link := range jujudSymlinks {
		fullLink := utils.EnsureBaseDir(a.rootDir, link)
		c.Assert(os.MkdirAll(filepath.Dir(fullLink), os.FileMode(0755)), jc.ErrorIsNil)
		c.Assert(symlink.New("/nowhere/special", fullLink), jc.ErrorIsNil, gc.Commentf(link))
	}

	// Start the agent and wait for it be running.
	_, done := s.waitForOpenState(c, a)

	// juju-run symlink should have been recreated.
	for _, link := range jujudSymlinks {
		fullLink := utils.EnsureBaseDir(a.rootDir, link)
		linkTarget, err := symlink.Read(fullLink)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(linkTarget, gc.Not(gc.Equals), "/nowhere/special", gc.Commentf(link))
	}

	s.waitStopped(c, state.JobManageModel, a, done)
}

func (s *MachineSuite) TestMachineAgentUninstall(c *gc.C) {
	m, ac, _ := s.primeAgent(c, state.JobHostUnits)
	err := m.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	a := s.newAgent(c, m)
	err = runWithTimeout(a)
	c.Assert(err, jc.ErrorIsNil)

	// juju-* symlinks should have been removed on termination.
	for _, link := range []string{jujuRun, jujuDumpLogs, jujuIntrospect} {
		_, err = os.Stat(utils.EnsureBaseDir(a.rootDir, link))
		c.Assert(err, jc.Satisfies, os.IsNotExist)
	}

	// data-dir should have been removed on termination
	_, err = os.Stat(ac.DataDir())
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (s *MachineSuite) TestMachineAgentRunsAPIAddressUpdaterWorker(c *gc.C) {
	// Start the machine agent.
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	// Update the API addresses.
	updatedServers := [][]network.HostPort{
		network.NewHostPorts(1234, "localhost"),
	}
	err := s.BackingState.SetAPIHostPorts(updatedServers)
	c.Assert(err, jc.ErrorIsNil)

	// Wait for config to be updated.
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		s.BackingState.StartSync()
		if !attempt.HasNext() {
			break
		}
		addrs, err := a.CurrentConfig().APIAddresses()
		c.Assert(err, jc.ErrorIsNil)
		if reflect.DeepEqual(addrs, []string{"localhost:1234"}) {
			return
		}
	}
	c.Fatalf("timeout while waiting for agent config to change")
}

func (s *MachineSuite) TestMachineAgentRunsDiskManagerWorker(c *gc.C) {
	// Patch out the worker func before starting the agent.
	started := newSignal()
	newWorker := func(diskmanager.ListBlockDevicesFunc, diskmanager.BlockDeviceSetter) worker.Worker {
		started.trigger()
		return jworker.NewNoOpWorker()
	}
	s.PatchValue(&diskmanager.NewWorker, newWorker)

	// Start the machine agent.
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()
	started.assertTriggered(c, "diskmanager worker to start")
}

func (s *MachineSuite) TestDiskManagerWorkerUpdatesState(c *gc.C) {
	expected := []storage.BlockDevice{{DeviceName: "whatever"}}
	s.PatchValue(&diskmanager.DefaultListBlockDevices, func() ([]storage.BlockDevice, error) {
		return expected, nil
	})

	// Start the machine agent.
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	sb, err := state.NewStorageBackend(s.BackingState)
	c.Assert(err, jc.ErrorIsNil)

	// Wait for state to be updated.
	s.BackingState.StartSync()
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		devices, err := sb.BlockDevices(m.MachineTag())
		c.Assert(err, jc.ErrorIsNil)
		if len(devices) > 0 {
			c.Assert(devices, gc.HasLen, 1)
			c.Assert(devices[0].DeviceName, gc.Equals, expected[0].DeviceName)
			return
		}
	}
	c.Fatalf("timeout while waiting for block devices to be recorded")
}

func (s *MachineSuite) TestMachineAgentRunsMachineStorageWorker(c *gc.C) {
	m, _, _ := s.primeAgent(c, state.JobHostUnits)

	started := newSignal()
	newWorker := func(config storageprovisioner.Config) (worker.Worker, error) {
		c.Check(config.Scope, gc.Equals, m.Tag())
		c.Check(config.Validate(), jc.ErrorIsNil)
		started.trigger()
		return jworker.NewNoOpWorker(), nil
	}
	s.PatchValue(&storageprovisioner.NewStorageProvisioner, newWorker)

	// Start the machine agent.
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()
	started.assertTriggered(c, "storage worker to start")
}

func (s *MachineSuite) TestCertificateDNSUpdated(c *gc.C) {
	m, _, _ := s.primeAgent(c, state.JobManageModel)
	a := s.newAgent(c, m)
	s.testCertificateDNSUpdated(c, a)
}

func (s *MachineSuite) TestCertificateDNSUpdatedInvalidPrivateKey(c *gc.C) {
	m, agentConfig, _ := s.primeAgent(c, state.JobManageModel)

	// Write out config with an invalid private key. This should
	// cause the agent to rewrite the cert and key.
	si, ok := agentConfig.StateServingInfo()
	c.Assert(ok, jc.IsTrue)
	si.PrivateKey = "foo"
	agentConfig.SetStateServingInfo(si)
	err := agentConfig.Write()
	c.Assert(err, jc.ErrorIsNil)

	a := s.newAgent(c, m)
	s.testCertificateDNSUpdated(c, a)
}

func (s *MachineSuite) testCertificateDNSUpdated(c *gc.C, a *MachineAgent) {
	// Set up a channel which fires when State is opened.
	started := make(chan struct{}, 16)
	s.PatchValue(&reportOpenedState, func(*state.State) {
		started <- struct{}{}
	})

	// Start the agent.
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	// Wait for State to be opened. Once this occurs we know that the
	// agent's initial startup has happened.
	s.assertChannelActive(c, started, "agent to start up")

	// Check that certificate was updated when the agent started.
	stateInfo, _ := a.CurrentConfig().StateServingInfo()
	srvCert, _, err := cert.ParseCertAndKey(stateInfo.Cert, stateInfo.PrivateKey)
	c.Assert(err, jc.ErrorIsNil)
	expectedDnsNames := set.NewStrings("localhost", "juju-apiserver", "juju-mongodb")
	certDnsNames := set.NewStrings(srvCert.DNSNames...)
	c.Check(expectedDnsNames.Difference(certDnsNames).IsEmpty(), jc.IsTrue)

	// Check the mongo certificate file too.
	pemContent, err := ioutil.ReadFile(filepath.Join(s.DataDir(), "server.pem"))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(pemContent), gc.Equals, stateInfo.Cert+"\n"+stateInfo.PrivateKey)
}

func (s *MachineSuite) setupIgnoreAddresses(c *gc.C, expectedIgnoreValue bool) chan bool {
	ignoreAddressCh := make(chan bool, 1)
	s.AgentSuite.PatchValue(&machiner.NewMachiner, func(cfg machiner.Config) (worker.Worker, error) {
		select {
		case ignoreAddressCh <- cfg.ClearMachineAddressesOnStart:
		default:
		}

		// The test just cares that NewMachiner is called with the correct
		// value, nothing else is done with the worker.
		return newDummyWorker(), nil
	})

	attrs := coretesting.Attrs{"ignore-machine-addresses": expectedIgnoreValue}
	err := s.Model.UpdateModelConfig(attrs, nil)
	c.Assert(err, jc.ErrorIsNil)
	return ignoreAddressCh
}

func (s *MachineSuite) TestMachineAgentIgnoreAddresses(c *gc.C) {
	for _, expectedIgnoreValue := range []bool{true, false} {
		ignoreAddressCh := s.setupIgnoreAddresses(c, expectedIgnoreValue)

		m, _, _ := s.primeAgent(c, state.JobHostUnits)
		a := s.newAgent(c, m)
		defer a.Stop()
		doneCh := make(chan error)
		go func() {
			doneCh <- a.Run(nil)
		}()

		select {
		case ignoreMachineAddresses := <-ignoreAddressCh:
			if ignoreMachineAddresses != expectedIgnoreValue {
				c.Fatalf("expected ignore-machine-addresses = %v, got = %v", expectedIgnoreValue, ignoreMachineAddresses)
			}
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for the machiner to start")
		}
		s.waitStopped(c, state.JobHostUnits, a, doneCh)
	}
}

func (s *MachineSuite) TestMachineAgentIgnoreAddressesContainer(c *gc.C) {
	ignoreAddressCh := s.setupIgnoreAddresses(c, true)

	parent, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.State.AddMachineInsideMachine(
		state.MachineTemplate{
			Series: "trusty",
			Jobs:   []state.MachineJob{state.JobHostUnits},
		},
		parent.Id(),
		instance.LXD,
	)
	c.Assert(err, jc.ErrorIsNil)

	vers := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: series.MustHostSeries(),
	}
	s.primeAgentWithMachine(c, m, vers)
	a := s.newAgent(c, m)
	defer a.Stop()
	doneCh := make(chan error)
	go func() {
		doneCh <- a.Run(nil)
	}()

	select {
	case ignoreMachineAddresses := <-ignoreAddressCh:
		if ignoreMachineAddresses {
			c.Fatalf("expected ignore-machine-addresses = false, got = true")
		}
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for the machiner to start")
	}
	s.waitStopped(c, state.JobHostUnits, a, doneCh)
}

func (s *MachineSuite) TestMachineWorkers(c *gc.C) {
	tracker := agenttest.NewEngineTracker()
	instrumented := TrackMachines(c, tracker, machineManifolds)
	s.PatchValue(&machineManifolds, instrumented)

	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	// Wait for it to stabilise, running as normal.
	matcher := agenttest.NewWorkerMatcher(c, tracker, a.Tag().String(),
		append(alwaysMachineWorkers, notMigratingMachineWorkers...))
	agenttest.WaitMatch(c, matcher.Check, coretesting.LongWait, s.BackingState.StartSync)
}

func (s *MachineSuite) TestControllerModelWorkers(c *gc.C) {
	uuid := s.BackingState.ModelUUID()

	tracker := agenttest.NewEngineTracker()
	instrumented := TrackModels(c, tracker, iaasModelManifolds)
	s.PatchValue(&iaasModelManifolds, instrumented)

	expectedWorkers := append(alwaysModelWorkers, aliveModelWorkers...)

	matcher := agenttest.NewWorkerMatcher(c, tracker, uuid, expectedWorkers)
	s.assertJobWithState(c, state.JobManageModel, func(agent.Config, *state.State) {
		agenttest.WaitMatch(c, matcher.Check, coretesting.LongWait, s.BackingState.StartSync)
	})
}

func (s *MachineSuite) TestHostedModelWorkers(c *gc.C) {
	// The dummy provider blows up in the face of multi-model
	// scenarios so patch in a minimal environs.Environ that's good
	// enough to allow the model workers to run.
	s.PatchValue(&newEnvirons, func(environs.OpenParams) (environs.Environ, error) {
		return &minModelWorkersEnviron{}, nil
	})

	st, closer := s.setUpNewModel(c)
	defer closer()

	uuid := st.ModelUUID()

	tracker := agenttest.NewEngineTracker()
	instrumented := TrackModels(c, tracker, iaasModelManifolds)
	s.PatchValue(&iaasModelManifolds, instrumented)

	matcher := agenttest.NewWorkerMatcher(c, tracker, uuid,
		append(alwaysModelWorkers, aliveModelWorkers...))
	s.assertJobWithState(c, state.JobManageModel, func(agent.Config, *state.State) {
		agenttest.WaitMatch(c, matcher.Check, ReallyLongWait, st.StartSync)
	})
}

func (s *MachineSuite) TestWorkersForHostedModelWithInvalidCredential(c *gc.C) {
	// The dummy provider blows up in the face of multi-model
	// scenarios so patch in a minimal environs.Environ that's good
	// enough to allow the model workers to run.
	s.PatchValue(&newEnvirons, func(environs.OpenParams) (environs.Environ, error) {
		return &minModelWorkersEnviron{}, nil
	})

	st := s.Factory.MakeModel(c, &factory.ModelParams{
		ConfigAttrs: coretesting.Attrs{
			"max-status-history-age":  "2h",
			"max-status-history-size": "4M",
			"max-action-results-age":  "2h",
			"max-action-results-size": "4M",
		},
		CloudCredential: names.NewCloudCredentialTag("dummy/admin/cred"),
	})
	defer func() {
		err := st.Close()
		c.Check(err, jc.ErrorIsNil)
	}()

	uuid := st.ModelUUID()

	// invalidate cloud credential for this model
	err := st.InvalidateModelCredential("coz i can")
	c.Assert(err, jc.ErrorIsNil)

	tracker := agenttest.NewEngineTracker()
	instrumented := TrackModels(c, tracker, iaasModelManifolds)
	s.PatchValue(&iaasModelManifolds, instrumented)

	expectedWorkers := append(alwaysModelWorkers, aliveModelWorkers...)
	// Since this model's cloud credential is no longer valid,
	// only the workers that don't require a valid credential should remain.
	remainingWorkers := set.NewStrings(expectedWorkers...).Difference(
		set.NewStrings(requireValidCredentialModelWorkers...))

	matcher := agenttest.NewWorkerMatcher(c, tracker, uuid, remainingWorkers.SortedValues())
	s.assertJobWithState(c, state.JobManageModel, func(agent.Config, *state.State) {
		agenttest.WaitMatch(c, matcher.Check, ReallyLongWait, st.StartSync)
	})
}

func (s *MachineSuite) TestMigratingModelWorkers(c *gc.C) {
	st, closer := s.setUpNewModel(c)
	defer closer()
	uuid := st.ModelUUID()

	tracker := agenttest.NewEngineTracker()

	// Replace the real migrationmaster worker with a fake one which
	// does nothing. This is required to make this test be reliable as
	// the environment required for the migrationmaster to operate
	// correctly is too involved to set up from here.
	//
	// TODO(mjs) - an alternative might be to provide a fake Facade
	// and api.Open to the real migrationmaster but this test is
	// awfully far away from the low level details of the worker.
	origModelManifolds := iaasModelManifolds
	modelManifoldsDisablingMigrationMaster := func(config model.ManifoldsConfig) dependency.Manifolds {
		config.NewMigrationMaster = func(config migrationmaster.Config) (worker.Worker, error) {
			return &nullWorker{dead: make(chan struct{})}, nil
		}
		return origModelManifolds(config)
	}
	instrumented := TrackModels(c, tracker, modelManifoldsDisablingMigrationMaster)
	s.PatchValue(&iaasModelManifolds, instrumented)

	targetControllerTag := names.NewControllerTag(utils.MustNewUUID().String())
	_, err := st.CreateMigration(state.MigrationSpec{
		InitiatedBy: names.NewUserTag("admin"),
		TargetInfo: migration.TargetInfo{
			ControllerTag: targetControllerTag,
			Addrs:         []string{"1.2.3.4:5555"},
			CACert:        "cert",
			AuthTag:       names.NewUserTag("user"),
			Password:      "password",
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	matcher := agenttest.NewWorkerMatcher(c, tracker, uuid,
		append(alwaysModelWorkers, migratingModelWorkers...))
	s.assertJobWithState(c, state.JobManageModel, func(agent.Config, *state.State) {
		agenttest.WaitMatch(c, matcher.Check, ReallyLongWait, st.StartSync)
	})
}

func (s *MachineSuite) TestDyingModelCleanedUp(c *gc.C) {
	st, closer := s.setUpNewModel(c)
	defer closer()

	timeout := time.After(ReallyLongWait)
	s.assertJobWithState(c, state.JobManageModel, func(agent.Config, *state.State) {
		model, err := st.Model()
		c.Assert(err, jc.ErrorIsNil)
		watch := model.Watch()
		defer workertest.CleanKill(c, watch)

		err = model.Destroy(state.DestroyModelParams{})
		c.Assert(err, jc.ErrorIsNil)
		for {
			select {
			case <-watch.Changes():
				err := model.Refresh()
				cause := errors.Cause(err)
				if err == nil {
					continue // still there
				} else if errors.IsNotFound(cause) {
					return // successfully removed
				}
				c.Assert(err, jc.ErrorIsNil) // guaranteed fail
			case <-time.After(coretesting.ShortWait):
				st.StartSync()
			case <-timeout:
				c.Fatalf("timed out waiting for workers")
			}
		}
	})
}

func (s *MachineSuite) TestModelWorkersRespectSingularResponsibilityFlag(c *gc.C) {

	// Grab responsibility for the model on behalf of another machine.
	claimer := s.BackingState.SingularClaimer()
	uuid := s.BackingState.ModelUUID()
	err := claimer.Claim(uuid, "machine-999-lxd-99", time.Hour)
	c.Assert(err, jc.ErrorIsNil)

	// Then run a normal model-tracking test, just checking for
	// a different set of workers.
	tracker := agenttest.NewEngineTracker()
	instrumented := TrackModels(c, tracker, iaasModelManifolds)
	s.PatchValue(&iaasModelManifolds, instrumented)

	matcher := agenttest.NewWorkerMatcher(c, tracker, uuid, alwaysModelWorkers)
	s.assertJobWithState(c, state.JobManageModel, func(agent.Config, *state.State) {
		agenttest.WaitMatch(c, matcher.Check, coretesting.LongWait, s.BackingState.StartSync)
	})
}

func (s *MachineSuite) setUpNewModel(c *gc.C) (newSt *state.State, closer func()) {
	// Create a new environment, tests can now watch if workers start for it.
	newSt = s.Factory.MakeModel(c, &factory.ModelParams{
		ConfigAttrs: coretesting.Attrs{
			"max-status-history-age":  "2h",
			"max-status-history-size": "4M",
			"max-action-results-age":  "2h",
			"max-action-results-size": "4M",
		},
	})
	return newSt, func() {
		err := newSt.Close()
		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *MachineSuite) TestReplicasetInitForNewController(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("controllers on windows aren't supported")
	}

	s.fakeEnsureMongo.ServiceInstalled = false

	m, _, _ := s.primeAgent(c, state.JobManageModel)
	a := s.newAgent(c, m)
	agentConfig := a.CurrentConfig()

	err := a.ensureMongoServer(agentConfig)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.fakeEnsureMongo.EnsureCount, gc.Equals, 1)
	c.Assert(s.fakeEnsureMongo.InitiateCount, gc.Equals, 0)
}

type nullWorker struct {
	dead chan struct{}
}

func (w *nullWorker) Kill() {
	close(w.dead)
}

func (w *nullWorker) Wait() error {
	<-w.dead
	return nil
}
