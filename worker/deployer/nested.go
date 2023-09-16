// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	agenterrors "github.com/juju/juju/agent/errors"
	message "github.com/juju/juju/internal/pubsub/agent"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/common/reboot"
)

const (
	deployedUnitsKey = "deployed-units"
	stoppedUnitsKey  = "stopped-units"
)

// Logger represents a logger used by the context.
type Logger interface {
	Errorf(string, ...interface{})
	Warningf(string, ...interface{})
	Infof(string, ...interface{})
	Debugf(string, ...interface{})
	Tracef(string, ...interface{})
}

// RebootMonitorStatePurger is implemented by types that can clean up the
// internal reboot-tracking state for a particular entity.
type RebootMonitorStatePurger interface {
	PurgeState(tag names.Tag) error
}

// The nested deployer context is responsible for creating dependency engine
// manifolds to run workers for each unit and add them to the worker.
// The context itself is a worker where that worker is the dependency engine
// for the unit workers.

var _ Context = (*nestedContext)(nil)

type nestedContext struct {
	logger Logger
	agent  agent.Agent
	// agentConfig is a snapshot of the current configuration.
	agentConfig    agent.Config
	baseUnitConfig UnitAgentConfig

	mu     sync.Mutex
	units  map[string]*UnitAgent
	errors map[string]error
	runner *worker.Runner
	hub    Hub
	unsub  func()

	// rebootMonitorStatePurger allows the deployer to clean up the
	// internal reboot tracking state when a unit gets removed.
	rebootMonitorStatePurger RebootMonitorStatePurger
}

// ContextConfig contains all the information that the nested context
// needs to run.
type ContextConfig struct {
	Agent                    agent.Agent
	Clock                    clock.Clock
	Hub                      Hub
	Logger                   Logger
	UnitEngineConfig         func() dependency.EngineConfig
	SetupLogging             func(*loggo.Context, agent.Config)
	UnitManifolds            func(config UnitManifoldsConfig) dependency.Manifolds
	RebootMonitorStatePurger RebootMonitorStatePurger
}

// Validate ensures all the required values are set.
func (c *ContextConfig) Validate() error {
	if c.Agent == nil {
		return errors.NotValidf("missing Agent")
	}
	if c.Clock == nil {
		return errors.NotValidf("missing Clock")
	}
	if c.Hub == nil {
		return errors.NotValidf("missing Hub")
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
	if c.UnitManifolds == nil {
		return errors.NotValidf("missing UnitManifolds")
	}
	return nil
}

// NewNestedContext creates a new deployer context that is responsible for
// running the workers for units as individual dependency engines in a runner it
// owns.
func NewNestedContext(config ContextConfig) (Context, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	agentConfig := config.Agent.CurrentConfig()
	context := &nestedContext{
		logger:      config.Logger,
		agent:       config.Agent,
		agentConfig: agentConfig,
		baseUnitConfig: UnitAgentConfig{
			DataDir:          agentConfig.DataDir(),
			Clock:            config.Clock,
			Logger:           config.Logger,
			UnitEngineConfig: config.UnitEngineConfig,
			UnitManifolds:    config.UnitManifolds,
			SetupLogging:     config.SetupLogging,
		},

		units:  make(map[string]*UnitAgent),
		errors: make(map[string]error),
		runner: worker.NewRunner(worker.RunnerParams{
			Logger:        config.Logger,
			IsFatal:       agenterrors.IsFatal,
			MoreImportant: agenterrors.MoreImportant,
			RestartDelay:  jworker.RestartDelay,
		}),
		hub:                      config.Hub,
		rebootMonitorStatePurger: config.RebootMonitorStatePurger,
	}

	if context.rebootMonitorStatePurger == nil {
		context.rebootMonitorStatePurger = reboot.NewMonitor(agentConfig.TransientDataDir())
	}

	unsubStop := context.hub.Subscribe(message.StopUnitTopic, context.stopUnitRequest)
	unsubStart := context.hub.Subscribe(message.StartUnitTopic, context.startUnitRequest)
	unsubStatus := context.hub.Subscribe(message.UnitStatusTopic, context.unitStatusRequest)
	context.unsub = func() {
		unsubStop()
		unsubStart()
		unsubStatus()
	}
	// Stat all the units that context should have deployed and started.
	units := context.deployedUnits()
	stopped := context.stoppedUnits()
	config.Logger.Infof("new context: units %q, stopped %q", strings.Join(units.Values(), ", "), strings.Join(stopped.Values(), ", "))
	for _, u := range units.SortedValues() {
		if u == "" {
			config.Logger.Warningf("empty unit")
			continue
		}
		agent, err := context.newUnitAgent(u)
		if err != nil {
			config.Logger.Errorf("unable to start unit %q: %v", u, err)
			context.errors[u] = err
			continue
		}
		context.units[u] = agent
		if !stopped.Contains(u) {
			if err := context.startUnitWorkers(u); err != nil {
				config.Logger.Errorf("unable to start workers for unit %q: %v", u, err)
				context.errors[u] = err
			}
		}
	}

	return context, nil
}

func (c *nestedContext) stopUnitRequest(topic string, data interface{}) {
	units, ok := data.(message.Units)
	if !ok {
		c.logger.Errorf("data should be a Units structure")
	}
	response := message.StartStopResponse{}
	for _, unitName := range units.Names {
		if err := c.stopUnit(unitName); err != nil {
			response[unitName] = err.Error()
		} else {
			response[unitName] = "stopped"
		}
	}
	c.hub.Publish(message.StopUnitResponseTopic, response)
}

func (c *nestedContext) startUnitRequest(topic string, data interface{}) {
	units, ok := data.(message.Units)
	if !ok {
		c.logger.Errorf("data should be a Units structure")
	}
	response := message.StartStopResponse{}
	for _, unitName := range units.Names {
		if unitAgent, ok := c.units[unitName]; ok && !unitAgent.running() {
			// Start over with a new agent, we do not know how long it's been lost.
			newAgent, err := c.newUnitAgent(unitName)
			if err != nil {
				response[unitName] = fmt.Sprintf("unable to get new unit agent: %v", err)
				c.errors[unitName] = err
				continue
			}
			c.units[unitName] = newAgent
		}

		if err := c.startUnit(unitName); err != nil {
			response[unitName] = err.Error()
		} else {
			response[unitName] = "started"
		}
	}
	c.hub.Publish(message.StartUnitResponseTopic, response)
}

func (c *nestedContext) unitStatusRequest(topic string, _ interface{}) {
	c.mu.Lock()
	agentName := c.agentConfig.Tag()
	deployed := c.deployedUnits()
	stopped := c.stoppedUnits()
	c.mu.Unlock()

	units := make(map[string]string)
	for _, unitName := range deployed.Values() {
		status := "running"
		if stopped.Contains(unitName) {
			status = "stopped"
		}
		units[unitName] = status
	}

	response := message.Status{
		"agent": agentName.String(),
		"units": units,
	}
	c.hub.Publish(message.UnitStatusResponseTopic, response)
}

func (c *nestedContext) newUnitAgent(unitName string) (*UnitAgent, error) {
	unitConfig := c.baseUnitConfig
	unitConfig.Name = unitName
	// Add a Filter function to the engine config with a method that has the
	// unitName bound in.
	engineConfig := unitConfig.UnitEngineConfig()
	engineConfig.Filter = func(err error) error {
		err = errors.Cause(err)
		switch err {
		case jworker.ErrTerminateAgent:
			// Here we just return nil to have the worker Wait function
			// return nil, so that the start function isn't called again.
			// We also try to record the unit as "stopped".
			c.hub.Publish(message.StopUnitTopic,
				message.Units{Names: []string{unitName}})
			return nil
		case jworker.ErrRestartAgent:
			// Return a different error that the Runner will not identify
			// as fatal to get the workers restarted.
			return errors.New("restart unit agent workers")
		}
		// Otherwise just return the error
		return err
	}
	// Replace the unit engine conf function with one that returns
	// the engineConfig above from the closure.
	unitConfig.UnitEngineConfig = func() dependency.EngineConfig {
		return engineConfig
	}
	return NewUnitAgent(unitConfig)
}

// Kill the embedded running.
func (c *nestedContext) Kill() {
	c.unsub()
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

	deployed := c.deployedUnits()
	result := map[string]interface{}{
		"deployed": deployed.SortedValues(),
	}
	if c.runner != nil {
		result["units"] = c.runner.Report()
	}
	if len(c.errors) > 0 {
		errors := make(map[string]string)
		for unitName, err := range c.errors {
			errors[unitName] = err.Error()
		}
		result["errors"] = errors
	}
	stopped := c.stoppedUnits()
	if len(stopped) > 0 {
		result["stopped"] = stopped.SortedValues()
	}
	return result
}

// DeployUnit is called when there is a new unit found on the machine.
// The unit's agent.conf is still being used by the unit workers, so that
// needs to be created, along with a link to the tools directory for the
// unit.
func (c *nestedContext) DeployUnit(unitName, initialPassword string) error {
	// Create unit agent config file.
	tag := names.NewUnitTag(unitName)
	_, err := c.createUnitAgentConfig(tag, initialPassword)
	if err != nil {
		// Any error here is indicative of a disk issue, potentially out of
		// space or inodes. Either way, bouncing the deployer and having the
		// exponential backoff enter play is the right decision.
		return errors.Trace(err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.logger.Tracef("starting the unit workers for %q", unitName)

	agent, err := c.newUnitAgent(unitName)
	c.units[unitName] = agent
	if err != nil {
		c.logger.Errorf("unable to create unit agent %q: %v", unitName, err)
		c.errors[unitName] = err
	} else {
		if err := c.startUnitWorkers(unitName); err != nil {
			c.logger.Errorf("unable to start workers for unit %q: %v", unitName, err)
			c.errors[unitName] = err
		}
	}

	c.logger.Tracef("updating the deployed units to add %q", unitName)
	// Add to deployed-units stored in the machine agent config.
	units := c.deployedUnits()
	units.Add(unitName)
	allUnits := strings.Join(units.SortedValues(), ",")
	if err := c.updateConfigValue(deployedUnitsKey, allUnits); err != nil {
		// It isn't really fatal to the deployer if the deployed-units can't
		// be updated, but it is indicative of a disk error.
		c.logger.Warningf("couldn't update stopped deployed units to add %q, %s", unitName, err.Error())
	}

	return nil
}

func (c *nestedContext) startUnitWorkers(unitName string) error {
	// Assumes lock is held.
	c.logger.Infof("starting workers for %q", unitName)
	agent, ok := c.units[unitName]
	if !ok {
		return errors.NotFoundf("unit %q", unitName)
	}
	if agent.running() {
		c.logger.Infof("unit workers for %q are already running", unitName)
		return nil
	}

	err := c.runner.StartWorker(unitName, agent.start)
	// Ensure starting a unit worker is idempotent.
	if err == nil || errors.Is(err, errors.AlreadyExists) {
		return nil
	}
	return errors.Trace(err)
}

func (c *nestedContext) stopUnitWorkers(unitName string) error {
	// Assumes lock is held.
	agent, ok := c.units[unitName]
	if !ok {
		return errors.NotFoundf("unit %q", unitName)
	}
	if !agent.running() {
		c.logger.Infof("unit workers for %q not running", unitName)
		return nil
	}
	if err := c.runner.StopAndRemoveWorker(unitName, nil); err != nil {
		if errors.Is(err, errors.NotFound) {
			// NotFound, assume it's already stopped.
			return nil
		}
		// StopWorker only ever returns an error when the runner is dead.
		// In that case, it is fine to return errors back to the deployer worker.
		return errors.Annotatef(err, "unable to stop workers for %q", unitName)
	}
	return nil
}

// stopUnit will stop the workers for the unit specified, and record the
// unit as one of the stopped ones so it won't be started when the deployer
// is restarted.
func (c *nestedContext) stopUnit(unitName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.stopUnitWorkers(unitName); err != nil {
		return errors.Trace(err)
	}

	units := c.stoppedUnits()
	// If the unit is already stopped, no need to update it.
	if units.Contains(unitName) {
		return nil
	}

	units.Add(unitName)
	allUnits := strings.Join(units.SortedValues(), ",")
	if err := c.updateConfigValue(stoppedUnitsKey, allUnits); err != nil {
		// It isn't really fatal to the deployer if the stopped units can't
		// be updated, but it is indicative of a disk error.
		c.logger.Warningf("couldn't update stopped units to add %q, %s", unitName, err.Error())
	}

	return nil
}

// startUnit will start the workers for a stopped unit specified.
func (c *nestedContext) startUnit(unitName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.startUnitWorkers(unitName); err != nil {
		return errors.Trace(err)
	}

	// If we get to here, we know the unit existed and was stopped,
	// so we don't bother checking that it is in the set as it should be.
	units := c.stoppedUnits()
	units.Remove(unitName)
	allUnits := strings.Join(units.SortedValues(), ",")
	if err := c.updateConfigValue(stoppedUnitsKey, allUnits); err != nil {
		// It isn't really fatal to the deployer if the stopped units can't
		// be updated, but it is indicative of a disk error.
		c.logger.Warningf("couldn't update stopped units to add %q, %s", unitName, err.Error())
	}

	return nil
}

func (c *nestedContext) updateConfigValue(key, value string) error {
	writeErr := c.agent.ChangeConfig(func(setter agent.ConfigSetter) error {
		setter.SetValue(key, value)
		return nil
	})
	c.agentConfig = c.agent.CurrentConfig()
	return writeErr
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

			AgentLogfileMaxBackups: c.agentConfig.AgentLogfileMaxBackups(),
			AgentLogfileMaxSizeMB:  c.agentConfig.AgentLogfileMaxSizeMB(),
		})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return conf, errors.Trace(conf.Write())
}

// RecallUnit is called when a unit is being removed from the machine.
// If the model removes a unit, or the model is being torn down in an
// orderly manner, this function is called.
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
	tag := names.NewUnitTag(unitName)
	agentDir := agent.Dir(c.agentConfig.DataDir(), tag)
	if err := os.RemoveAll(agentDir); err != nil {
		return errors.Annotate(err, "unable to remove agent dir")
	}

	// Ensure that the reboot monitor flag files for the unit are also
	// cleaned up. This not really important if the machine is about to
	// be recycled but it must be done for manual machines as the flag files
	// will linger around until a reboot occurs.
	if err := c.rebootMonitorStatePurger.PurgeState(tag); err != nil {
		return errors.Trace(err)
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

func removeOnErr(err *error, logger Logger, path string) {
	if *err != nil {
		if err := os.RemoveAll(path); err != nil {
			logger.Errorf("installer: cannot remove %q: %v", path, err)
		}
	}
}
