// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	"os"
	"path"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/dependency"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/addons"
	agentconfig "github.com/juju/juju/agent/config"
	"github.com/juju/juju/agent/engine"
	corelogger "github.com/juju/juju/core/logger"
	jv "github.com/juju/juju/core/version"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	jt "github.com/juju/juju/internal/testing"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/deployer"
)

const veryShortWait = 5 * time.Millisecond

type NestedContextSuite struct {
	BaseSuite

	config  deployer.ContextConfig
	agent   agentconfig.AgentConf
	workers *unitWorkersStub
}

var _ = gc.Suite(&NestedContextSuite{})

func (s *NestedContextSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	logger := loggertesting.WrapCheckLog(c).Child("nested-context")

	datadir := c.MkDir()
	machine := names.NewMachineTag("42")
	config, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths: agent.Paths{
				DataDir:         datadir,
				LogDir:          c.MkDir(),
				MetricsSpoolDir: c.MkDir(),
			},
			Tag:                    machine,
			Password:               "sekrit",
			Nonce:                  "unused",
			Controller:             jt.ControllerTag,
			Model:                  jt.ModelTag,
			APIAddresses:           []string{"a1:123", "a2:123"},
			CACert:                 "fake CACert",
			UpgradedToVersion:      jv.Current,
			AgentLogfileMaxBackups: 7,
			AgentLogfileMaxSizeMB:  123,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(config.Write(), jc.ErrorIsNil)

	s.agent = agentconfig.NewAgentConfig(datadir)
	err = s.agent.ReadConfig(machine.String())
	c.Assert(err, jc.ErrorIsNil)

	s.workers = &unitWorkersStub{
		started: make(chan string, 10), // eval size later
		stopped: make(chan string, 10), // eval size later
		logger:  logger,
	}
	s.config = deployer.ContextConfig{
		Agent:  s.agent,
		Clock:  clock.WallClock,
		Logger: logger,
		UnitEngineConfig: func() dependency.EngineConfig {
			return engine.DependencyEngineConfig(
				dependency.DefaultMetrics(),
				loggo.GetLogger("juju.worker.dependency"),
			)
		},
		SetupLogging:  func(corelogger.LoggerContext, agent.Config) {},
		UnitManifolds: s.workers.Manifolds,
	}
}

func (s *NestedContextSuite) TestConfigMissingAgentConfig(c *gc.C) {
	s.config.Agent = nil
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), gc.Equals, "missing Agent not valid")
}

func (s *NestedContextSuite) TestConfigMissingClock(c *gc.C) {
	s.config.Clock = nil
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), gc.Equals, "missing Clock not valid")
}

func (s *NestedContextSuite) TestConfigMissingLogger(c *gc.C) {
	s.config.Logger = nil
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), gc.Equals, "missing Logger not valid")
}

func (s *NestedContextSuite) TestConfigMissingSetupLogging(c *gc.C) {
	s.config.SetupLogging = nil
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), gc.Equals, "missing SetupLogging not valid")
}

func (s *NestedContextSuite) TestConfigMissingUnitEngineConfig(c *gc.C) {
	s.config.UnitEngineConfig = nil
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), gc.Equals, "missing UnitEngineConfig not valid")
}

func (s *NestedContextSuite) TestConfigMissingUnitManifolds(c *gc.C) {
	s.config.UnitManifolds = nil
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), gc.Equals, "missing UnitManifolds not valid")
}

func (s *NestedContextSuite) newContext(c *gc.C) deployer.Context {
	context, err := deployer.NewNestedContext(s.config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, context) })
	s.InitializeCurrentToolsDir(c, s.agent.DataDir())
	return context
}

func (s *NestedContextSuite) TestContextStops(c *gc.C) {
	// Create a context and make sure the clean kill is good.
	ctx := s.newContext(c)
	report := ctx.Report()
	c.Assert(report, jc.DeepEquals, map[string]interface{}{
		"deployed": []string{},
		"units": map[string]interface{}{
			"workers": map[string]interface{}{},
		},
	})
}

func (s *NestedContextSuite) TestDeployUnit(c *gc.C) {
	ctx := s.newContext(c)
	unitName := "something/0"
	err := ctx.DeployUnit(unitName, "password")
	c.Assert(err, jc.ErrorIsNil)

	// Wait for unit to start.
	s.workers.waitForStart(c, unitName)

	// Unit agent dir exists.
	unitConfig := agent.ConfigPath(s.agent.DataDir(), names.NewUnitTag(unitName))
	c.Assert(unitConfig, jc.IsNonEmptyFile)

	// Unit written into the config value as deployed units.
	c.Assert(s.agent.CurrentConfig().Value("deployed-units"), gc.Equals, unitName)
	c.Assert(s.agent.CurrentConfig().AgentLogfileMaxBackups(), gc.Equals, 7)
	c.Assert(s.agent.CurrentConfig().AgentLogfileMaxSizeMB(), gc.Equals, 123)
}

func (s *NestedContextSuite) TestRecallUnit(c *gc.C) {
	unitName := "something/0"
	tag := names.NewUnitTag(unitName)
	s.config.RebootMonitorStatePurger = &fakeRebootMonitor{c: c, tag: tag}
	ctx := s.newContext(c)
	err := ctx.DeployUnit(unitName, "password")
	c.Assert(err, jc.ErrorIsNil)

	// Wait for unit to start.
	s.workers.waitForStart(c, unitName)

	// Waiting for the unit to be indicated as started (above) is not sufficient
	// for this test.
	// The unitWorkersStub that represents the nested config for the unit
	// dependency engine indicates that the unit is started as soon as it is
	// created, but the introspection socket is created subsequently, which can
	// inhibit removal of the directory during the subsequent call to RecallUnit.
	// Waiting for the socket file to be present on disk is more robust.
	socketPath := path.Join(agent.Dir(s.agent.DataDir(), tag), addons.IntrospectionSocketName)
	err = waitForFile(socketPath)
	c.Assert(err, jc.ErrorIsNil)

	err = ctx.RecallUnit(unitName)
	c.Assert(err, jc.ErrorIsNil)

	// Unit agent dir no longer exists.
	c.Assert(agent.Dir(s.agent.DataDir(), tag), jc.DoesNotExist)

	// Unit written into the config value as deployed units.
	c.Assert(s.agent.CurrentConfig().Value("deployed-units"), gc.HasLen, 0)

	// Recall is idempotent.
	err = ctx.RecallUnit(unitName)
	c.Assert(err, jc.ErrorIsNil)
}

func waitForFile(filePath string) error {
	maxAttempts := 10
	pollInterval := 50 * time.Millisecond

	for i := 0; i < maxAttempts; i++ {
		if _, err := os.Stat(filePath); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}

		time.Sleep(pollInterval)
	}

	return errors.New("file not found after 10 attempts")
}

func (s *NestedContextSuite) TestErrTerminateAgentFromAgentWorker(c *gc.C) {
	_ = s.errTerminateAgentFromAgentWorker(c)
}

func (s *NestedContextSuite) errTerminateAgentFromAgentWorker(c *gc.C) deployer.Context {
	s.workers.workerError = jworker.ErrTerminateAgent
	ctx := s.newContext(c)
	unitName := "something/0"
	err := ctx.DeployUnit(unitName, "password")
	c.Assert(err, jc.ErrorIsNil)

	// Wait for unit to start.
	s.workers.waitForStart(c, unitName)

	// Unit is marked as stopped. There is a potential race due to the
	// number of goroutines that need to fire to get the information back
	// to the nested context.
	report := s.waitForStoppedCount(c, ctx, 1)

	c.Assert(report, jc.DeepEquals, map[string]interface{}{
		"deployed": []string{"something/0"},
		"stopped":  []string{"something/0"},
		"units": map[string]interface{}{
			"workers": map[string]interface{}{},
		},
	})
	return ctx
}

func (s *NestedContextSuite) waitForStoppedCount(c *gc.C, ctx deployer.Context, length int) map[string]interface{} {
	report := ctx.Report()
	maxTime := time.After(testing.LongWait)
	for {
		stopped := report["stopped"]
		if stopped != nil && len(stopped.([]string)) == length {
			break
		}
		select {
		case <-time.After(veryShortWait):
			report = ctx.Report()
		case <-maxTime:
			c.Fatal("unit not stopped")
		}
	}
	return report
}

func (s *NestedContextSuite) deployThreeUnits(c *gc.C, ctx deployer.Context) {
	// Units are conveniently in alphabetical order.
	for _, unitName := range []string{"first/0", "second/0", "third/0"} {
		err := ctx.DeployUnit(unitName, "password")
		c.Assert(err, jc.ErrorIsNil)
		// Wait for unit to start.
		s.workers.waitForStart(c, unitName)
	}

	report := ctx.Report()
	// There is a race condition here between the worker, which says the
	// start function was called, and the engine report itself having recorded
	// that the worker has started, and updated the engine report. In manual
	// testing locally it passed 30 odd times before failing, but on slower
	// machines it may well be more frequent, so have a loop here to test.
	maxTime := time.After(testing.LongWait)
	for {
		units := report["units"].(map[string]interface{})
		workers := units["workers"].(map[string]interface{})

		first := workers["first/0"].(map[string]interface{})
		second := workers["second/0"].(map[string]interface{})
		third := workers["third/0"].(map[string]interface{})

		if first["state"] == "started" && second["state"] == "started" && third["state"] == "started" {
			break
		}
		select {
		case <-time.After(veryShortWait):
			report = ctx.Report()
		case <-maxTime:
			c.Fatal("third unit worker did not start")
		}
	}
}

func (s *NestedContextSuite) TestReport(c *gc.C) {
	ctx := s.newContext(c)
	s.deployThreeUnits(c, ctx)

	check := jc.NewMultiChecker()
	check.AddExpr(`_["units"][_][_][_][_][_]["started"]`, jc.Ignore)
	check.AddExpr(`_["units"][_][_]["started"]`, jc.Ignore)
	// Dates are shown here as an example, but are ignored by the checker.
	c.Assert(ctx.Report(), check, map[string]interface{}{
		"deployed": []string{"first/0", "second/0", "third/0"},
		"units": map[string]interface{}{
			"workers": map[string]interface{}{
				"first/0": map[string]interface{}{
					"report": map[string]interface{}{
						"manifolds": map[string]interface{}{
							"worker": map[string]interface{}{
								"inputs":      []string{},
								"start-count": 1,
								"started":     "2020-07-24 03:01:20",
								"state":       "started",
							},
						},
						"state": "started",
					},
					"started": "2020-07-24 03:01:20",
					"state":   "started",
				},
				"second/0": map[string]interface{}{
					"report": map[string]interface{}{
						"manifolds": map[string]interface{}{
							"worker": map[string]interface{}{
								"inputs":      []string{},
								"start-count": 1,
								"started":     "2020-07-24 03:01:20",
								"state":       "started",
							},
						},
						"state": "started",
					},
					"started": "2020-07-24 03:01:20",
					"state":   "started",
				},
				"third/0": map[string]interface{}{
					"report": map[string]interface{}{
						"manifolds": map[string]interface{}{
							"worker": map[string]interface{}{
								"inputs":      []string{},
								"start-count": 1,
								"started":     "2020-07-24 03:01:20",
								"state":       "started",
							},
						},
						"state": "started",
					},
					"started": "2020-07-24 03:01:20",
					"state":   "started",
				},
			},
		},
	})

}

type fakeRebootMonitor struct {
	c   *gc.C
	tag names.UnitTag
}

func (m *fakeRebootMonitor) PurgeState(tag names.Tag) error {
	m.c.Assert(tag.String(), gc.Equals, m.tag.String())
	return nil
}
