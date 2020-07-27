// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/os/series"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	jt "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	jv "github.com/juju/juju/version"
	"github.com/juju/juju/worker/deployer"
)

type NestedContextSuite struct {
	testing.IsolationSuite

	config  deployer.ContextConfig
	agent   *fakeAgent
	workers *unitWorkersStub
}

var _ = gc.Suite(&NestedContextSuite{})

func (s *NestedContextSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	logger := loggo.GetLogger("test.nestedcontext")
	logger.SetLogLevel(loggo.TRACE)

	s.agent = &fakeAgent{
		c:      c,
		values: make(map[string]string),
	}
	s.workers = &unitWorkersStub{
		started: make(chan string, 10), // eval size later
		stopped: make(chan string, 10), // eval size later
		logger:  logger,
	}
	s.config = deployer.ContextConfig{
		AgentConfig:      s.agent,
		Clock:            clock.WallClock,
		Logger:           logger,
		UnitEngineConfig: engine.DependencyEngineConfig,
		SetupLogging: func(c *loggo.Context, _ agent.Config) {
			c.GetLogger("").SetLogLevel(loggo.DEBUG)
		},
		UpdateConfigValue: s.agent.setValue,
		UnitManifolds:     s.workers.Manifolds,
	}
}

func (s *NestedContextSuite) TestConfigMissingAgentConfig(c *gc.C) {
	s.config.AgentConfig = nil
	err := s.config.Validate()
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
	c.Assert(err.Error(), gc.Equals, "missing AgentConfig not valid")
}

func (s *NestedContextSuite) TestConfigMissingClock(c *gc.C) {
	s.config.Clock = nil
	err := s.config.Validate()
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
	c.Assert(err.Error(), gc.Equals, "missing Clock not valid")
}

func (s *NestedContextSuite) TestConfigMissingLogger(c *gc.C) {
	s.config.Logger = nil
	err := s.config.Validate()
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
	c.Assert(err.Error(), gc.Equals, "missing Logger not valid")
}

func (s *NestedContextSuite) TestConfigMissingSetupLogging(c *gc.C) {
	s.config.SetupLogging = nil
	err := s.config.Validate()
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
	c.Assert(err.Error(), gc.Equals, "missing SetupLogging not valid")
}

func (s *NestedContextSuite) TestConfigMissingUnitEngineConfig(c *gc.C) {
	s.config.UnitEngineConfig = nil
	err := s.config.Validate()
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
	c.Assert(err.Error(), gc.Equals, "missing UnitEngineConfig not valid")
}

func (s *NestedContextSuite) TestConfigMissingUpdateConfigValue(c *gc.C) {
	s.config.UpdateConfigValue = nil
	err := s.config.Validate()
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
	c.Assert(err.Error(), gc.Equals, "missing UpdateConfigValue not valid")
}

func (s *NestedContextSuite) TestConfigMissingUnitManifolds(c *gc.C) {
	s.config.UnitManifolds = nil
	err := s.config.Validate()
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
	c.Assert(err.Error(), gc.Equals, "missing UnitManifolds not valid")
}

func (s *NestedContextSuite) newContext(c *gc.C) deployer.Context {
	context, err := deployer.NewNestedContext(s.config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, context) })
	// Initialize the tools directory for the agent.
	// This should be <DataDir>/tools/<version>-<series>-<arch>.
	current := version.Binary{
		Number: jv.Current,
		Arch:   arch.HostArch(),
		Series: series.MustHostSeries(),
	}
	toolsDir := tools.SharedToolsDir(s.agent.DataDir(), current)
	// Make that directory.
	err = os.MkdirAll(toolsDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	toolsPath := filepath.Join(toolsDir, "downloaded-tools.txt")
	testTools := coretools.Tools{Version: current, URL: "http://testing.invalid/tools"}
	data, err := json.Marshal(testTools)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(toolsPath, data, 0644)
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(s.agent.Value("deployed-units"), gc.Equals, unitName)
}

func (s *NestedContextSuite) TestRecallUnit(c *gc.C) {
	ctx := s.newContext(c)
	unitName := "something/0"
	err := ctx.DeployUnit(unitName, "password")
	c.Assert(err, jc.ErrorIsNil)

	// Wait for unit to start.
	s.workers.waitForStart(c, unitName)

	err = ctx.RecallUnit(unitName)

	// Unit agent dir no longer exists.
	unitAgentDir := agent.Dir(s.agent.DataDir(), names.NewUnitTag(unitName))
	c.Assert(unitAgentDir, jc.DoesNotExist)

	// Unit written into the config value as deployed units.
	c.Assert(s.agent.Value("deployed-units"), gc.HasLen, 0)
}

func (s *NestedContextSuite) TestReport(c *gc.C) {
	ctx := s.newContext(c)

	// Units are conveniently in alphabetical order.
	for _, unitName := range []string{"first/0", "second/0", "third/0"} {
		err := ctx.DeployUnit(unitName, "password")
		c.Assert(err, jc.ErrorIsNil)
		// Wait for unit to start.
		s.workers.waitForStart(c, unitName)
	}

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

type fakeClock struct {
	clock.Clock
}

type fakeAgent struct {
	agent.Agent
	agent.Config

	c       *gc.C
	dataDir string
	logDir  string
	values  map[string]string
}

func (f *fakeAgent) Model() names.ModelTag {
	return jt.ModelTag
}

func (f *fakeAgent) Controller() names.ControllerTag {
	return jt.ControllerTag
}

func (f *fakeAgent) UpgradedToVersion() version.Number {
	return jv.Current
}

func (f *fakeAgent) CACert() string {
	return "fake CACert"
}

func (f *fakeAgent) APIAddresses() ([]string, error) {
	return []string{"a1:123", "a2:123"}, nil
}

func (f *fakeAgent) DataDir() string {
	if f.dataDir == "" {
		f.dataDir = f.c.MkDir()
	}
	return f.dataDir
}

func (f *fakeAgent) LogDir() string {
	if f.logDir == "" {
		f.logDir = f.c.MkDir()
	}
	return f.logDir
}

func (f *fakeAgent) setValue(key, value string) error {
	f.values[key] = value
	return nil
}

func (f *fakeAgent) Value(key string) string {
	return f.values[key]
}
