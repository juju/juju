// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"os"
	"strings"
	"sync"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/os/series"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	"github.com/kr/pretty"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/tools"
	agenterrors "github.com/juju/juju/cmd/jujud/agent/errors"
	jujuversion "github.com/juju/juju/version"
	jworker "github.com/juju/juju/worker"
)

const (
	deployedUnitsKey = "deployed-units"
	stoppedUnitsKey  = "stopped-units"
)

// Logger represents a logger used by the context.
type Logger interface {
	Criticalf(string, ...interface{})
	Errorf(string, ...interface{})
	Warningf(string, ...interface{})
	Infof(string, ...interface{})
	Debugf(string, ...interface{})
	Tracef(string, ...interface{})
}

// The nested deployer context is responsible for creating dependency engine
// manifolds to run workers for each unit and add them to the worker.
// The context itself is a worker where that worker is the dependency engine
// for the unit workers.

var _ Context = (*nestedContext)(nil)

type nestedContext struct {
	logger            Logger
	agentConfig       agent.Config
	baseUnitConfig    UnitAgentConfig
	updateConfigValue func(string, string) error

	mu     sync.Mutex
	units  map[string]*UnitAgent
	runner *worker.Runner
}

// ContextConfig contains all the information that the nested context
// needs to run.
type ContextConfig struct {
	AgentConfig       agent.Config
	Clock             clock.Clock
	Logger            Logger
	UnitEngineConfig  func() dependency.EngineConfig
	SetupLogging      func(*loggo.Context, agent.Config)
	UpdateConfigValue func(string, string) error
	UnitManifolds     func(config UnitManifoldsConfig) dependency.Manifolds
}

// Validate ensures all the required values are set.
func (c *ContextConfig) Validate() error {
	if c.AgentConfig == nil {
		return errors.NotValidf("missing AgentConfig")
	}
	if c.Clock == nil {
		return errors.NotValidf("missing Clock")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	if c.SetupLogging == nil {
		return errors.NotValidf("missing SetupLogging")
	}
	if c.UnitEngineConfig == nil {
		return errors.NotValidf("missing UnitEngineConfig")
	}
	if c.UpdateConfigValue == nil {
		return errors.NotValidf("missing UpdateConfigValue")
	}
	if c.UnitManifolds == nil {
		return errors.NotValidf("missing UnitManifolds")
	}
	return nil
}

// NewNestedContext creates a new deployer context that is responsible for
// running the workers for units as individual dependency engines in a runner it
// owns.
func NewNestedContext(config ContextConfig) (*nestedContext, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	context := &nestedContext{
		logger:      config.Logger,
		agentConfig: config.AgentConfig,
		baseUnitConfig: UnitAgentConfig{
			DataDir:          config.AgentConfig.DataDir(),
			Clock:            config.Clock,
			Logger:           config.Logger,
			UnitEngineConfig: config.UnitEngineConfig,
			UnitManifolds:    config.UnitManifolds,
			SetupLogging:     config.SetupLogging,
		},
		updateConfigValue: config.UpdateConfigValue,

		units: make(map[string]*UnitAgent),
		runner: worker.NewRunner(worker.RunnerParams{
			IsFatal:       agenterrors.IsFatal,
			MoreImportant: agenterrors.MoreImportant,
			RestartDelay:  jworker.RestartDelay,
		}),
	}

	// Stat all the units that context should have deployed and started.
	units := context.deployedUnits()
	stopped := context.stoppedUnits()
	config.Logger.Infof("new context: units %q, stopped %q", pretty.Sprint(units), pretty.Sprint(stopped))
	hasError := false
	for _, u := range units.SortedValues() {
		if u == "" {
			config.Logger.Warningf("empty unit")
			continue
		}
		unitConfig := context.baseUnitConfig
		unitConfig.Name = u
		agent, err := NewUnitAgent(unitConfig)
		if err != nil {
			hasError = true
			config.Logger.Errorf("unable to start unit %q: %v", u, err)
			continue
		}
		context.units[u] = agent
		if !stopped.Contains(u) {
			if err := context.startUnit(u); err != nil {
				return nil, errors.Annotatef(err, "issues starting workers for unit %q", u)
			}
		}
	}
	// if has errors, stop things///
	if hasError {
		context.runner.Kill()
		context.runner.Wait()
		return nil, errors.Errorf("unable to start units")
	}

	return context, nil
}

// Kill the embedded running.
func (c *nestedContext) Kill() {
	c.runner.Kill()
}

// Wait for the embedded runner to finish.
func (c *nestedContext) Wait() error {
	return c.runner.Wait()
}

// Report shows both the expected units and the status of the
// engine reports for those units.
func (c *nestedContext) Report() map[string]interface{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	running := c.runner.Report()

	deployed := c.deployedUnits()
	stopped := c.stoppedUnits()

	result := map[string]interface{}{
		"deployed": deployed.SortedValues(),
		"units":    running,
	}
	if len(stopped) > 0 {
		result["stopped"] = stopped.SortedValues()
	}
	return result
}

func (c *nestedContext) DeployUnit(unitName, initialPassword string) error {
	// Create unit agent config file.
	tag := names.NewUnitTag(unitName)
	_, err := c.createUnitAgentConfig(tag, initialPassword)
	if err != nil {
		return errors.Trace(err)
	}

	// Create a symlink for the unit "agent" binaries.
	// This is used because the uniter is still using the tools directory
	// for the unit agent for creating the jujuc symlinks.
	c.logger.Tracef("creating symlink for %q to tools directory for jujuc", unitName)
	dataDir := c.agentConfig.DataDir()
	hostSeries, err := series.HostSeries()
	if err != nil {
		return errors.Trace(err)
	}
	current := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: hostSeries,
	}
	toolsDir := tools.ToolsDir(dataDir, tag.String())
	defer removeOnErr(&err, c.logger, toolsDir)
	_, err = tools.ChangeAgentTools(dataDir, tag.String(), current)
	if err != nil {
		return errors.Trace(err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.logger.Tracef("starting the unit workers for %q", unitName)
	if err := c.startUnit(unitName); err != nil {
		return errors.Trace(err)
	}

	c.logger.Tracef("updating the deployed units to add %q", unitName)
	// Add to deployed-units stored in the machine agent config.
	units := c.deployedUnits()
	units.Add(unitName)
	allUnits := strings.Join(units.SortedValues(), ",")
	if err := c.updateConfigValue(deployedUnitsKey, allUnits); err != nil {
		return errors.Annotatef(err, "couldn't update deployed units to add %q", unitName)
	}

	return nil
}

func (c *nestedContext) startUnit(unitName string) error {
	// Assumes lock is held.
	c.logger.Infof("starting workers for %q", unitName)
	agent, found := c.units[unitName]
	if !found {
		unitConfig := c.baseUnitConfig
		unitConfig.Name = unitName
		var err error
		agent, err = NewUnitAgent(unitConfig)
		if err != nil {
			return errors.Trace(err)
		}
		c.units[unitName] = agent
	}
	return errors.Trace(c.runner.StartWorker(unitName, agent.start))
}

func (c *nestedContext) stopUnitWorkers(unitName string) error {
	// Assumes lock is held.
	agent := c.units[unitName]
	if !agent.running {
		c.logger.Infof("unit workers for %q not running", unitName)
		return nil
	}
	if err := c.runner.StopWorker(unitName); err != nil {
		return errors.Annotatef(err, "unable to stop workers for %q", unitName)
	}
	return nil
}

func (c *nestedContext) StopUnit(unitName string) error {
	// TODO: add a StartUnit for the stop/start behaviour.
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.stopUnitWorkers(unitName); err != nil {
		return errors.Trace(err)
	}

	units := c.stoppedUnits()
	units.Add(unitName)
	allUnits := strings.Join(units.SortedValues(), ",")
	if err := c.updateConfigValue(stoppedUnitsKey, allUnits); err != nil {
		return errors.Annotatef(err, "couldn't update stopped units to add %q", unitName)
	}

	return nil
}

func (c *nestedContext) createUnitAgentConfig(tag names.UnitTag, initialPassword string) (agent.Config, error) {
	c.logger.Tracef("create unit agent config for %q", tag)
	dataDir := c.agentConfig.DataDir()
	logDir := c.agentConfig.LogDir()
	apiAddresses, err := c.agentConfig.APIAddresses()
	if err != nil {
		return nil, errors.Trace(err)
	}

	conf, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths: agent.Paths{
				DataDir:         dataDir,
				LogDir:          logDir,
				MetricsSpoolDir: agent.DefaultPaths.MetricsSpoolDir,
			},
			Tag:               tag,
			Password:          initialPassword,
			Nonce:             "unused",
			Controller:        c.agentConfig.Controller(),
			Model:             c.agentConfig.Model(),
			APIAddresses:      apiAddresses,
			CACert:            c.agentConfig.CACert(),
			UpgradedToVersion: c.agentConfig.UpgradedToVersion(),
		})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return conf, errors.Trace(conf.Write())
}

func (c *nestedContext) RecallUnit(unitName string) error {
	// Stop runner for unit
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.stopUnitWorkers(unitName); err != nil {
		return errors.Trace(err)
	}

	// Remove unit from deployed-units
	units := c.deployedUnits()
	units.Remove(unitName)
	allUnits := strings.Join(units.SortedValues(), ",")
	if err := c.updateConfigValue(deployedUnitsKey, allUnits); err != nil {
		return errors.Annotatef(err, "couldn't update deployed units to remove %q", unitName)
	}
	stoppedUnits := c.stoppedUnits()
	if stoppedUnits.Contains(unitName) {
		stoppedUnits.Remove(unitName)
		allUnits := strings.Join(stoppedUnits.SortedValues(), ",")
		if err := c.updateConfigValue(stoppedUnitsKey, allUnits); err != nil {
			return errors.Annotatef(err, "couldn't update stopped units to remove %q", unitName)
		}
	}

	// Remove agent directory.
	agentDir := agent.Dir(c.agentConfig.DataDir(), names.NewUnitTag(unitName))
	if err := os.RemoveAll(agentDir); err != nil {
		return errors.Annotate(err, "unable to remove agent dir")
	}

	return nil
}

func (c *nestedContext) stoppedUnits() set.Strings {
	return set.NewStrings(c.getUnits(stoppedUnitsKey)...)
}

func (c *nestedContext) deployedUnits() set.Strings {
	return set.NewStrings(c.getUnits(deployedUnitsKey)...)
}

func (c *nestedContext) getUnits(key string) []string {
	value := c.agentConfig.Value(key)
	if value == "" {
		return nil
	}
	units := strings.Split(value, ",")
	return units
}

func (c *nestedContext) DeployedUnits() ([]string, error) {
	return c.getUnits(deployedUnitsKey), nil
}

func (c *nestedContext) AgentConfig() agent.Config {
	return c.agentConfig
}
