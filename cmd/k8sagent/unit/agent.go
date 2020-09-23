// Copyright 2012-2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit

import (
	"runtime"
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/utils/v2/voyeur"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/uniter"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/jujud/agent/addons"
	"github.com/juju/juju/cmd/jujud/agent/agentconf"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	agenterrors "github.com/juju/juju/cmd/jujud/agent/errors"
	"github.com/juju/juju/core/machinelock"
	jujuversion "github.com/juju/juju/version"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/introspection"
	"github.com/juju/juju/worker/logsender"
)

var logger = loggo.GetLogger("juju.cmd.k8sagent.unit")

type k8sUnitAgent struct {
	cmd.CommandBase
	agentconf.AgentConf
	configChangedVal *voyeur.Value
	// Unit command of k8sagent only knows application name but not unit name.
	// It will configure out the unit name from agent.conf file by itself.
	applicationName string
	clk             clock.Clock
	runner          *worker.Runner
	bufferedLogger  *logsender.BufferedLogWriter
	setupLogging    func(agent.Config) error
	ctx             *cmd.Context
	dead            chan struct{}
	errReason       error
	machineLock     machinelock.Lock

	prometheusRegistry *prometheus.Registry
}

func New(ctx *cmd.Context, bufferedLogger *logsender.BufferedLogWriter) (cmd.Command, error) {
	prometheusRegistry, err := addons.NewPrometheusRegistry()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &k8sUnitAgent{
		AgentConf:          agentconf.NewAgentConf(""),
		configChangedVal:   voyeur.NewValue(true),
		ctx:                ctx,
		clk:                clock.WallClock,
		dead:               make(chan struct{}),
		bufferedLogger:     bufferedLogger,
		prometheusRegistry: prometheusRegistry,
	}, nil
}

// Info returns a description of the command.
func (c *k8sUnitAgent) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "unit",
		Purpose: "starting a k8s agent",
	})
}

// SetFlags implements Command.
func (c *k8sUnitAgent) SetFlags(f *gnuflag.FlagSet) {
	c.AgentConf.AddFlags(f)
	f.StringVar(&c.applicationName, "application-name", "", "name of the application")
}

// Init initializes the command for running.
func (c *k8sUnitAgent) Init(args []string) error {
	if c.applicationName == "" {
		return agenterrors.RequiredError("application-name")
	}
	if !names.IsValidApplication(c.applicationName) {
		return errors.Errorf(`--application-name option expects "<application>" argument`)
	}
	if err := c.AgentConf.CheckArgs(args); err != nil {
		return err
	}
	c.runner = worker.NewRunner(worker.RunnerParams{
		IsFatal:       agenterrors.IsFatal,
		MoreImportant: agenterrors.MoreImportant,
		RestartDelay:  jworker.RestartDelay,
	})

	// Note: agent.conf file is at /var/lib/juju/agents/<application-tag>/agent.conf
	// Tag in agent.conf is unit tag.
	if err := c.ReadConfig(c.applicationTag().String()); err != nil {
		return errors.Trace(err)
	}

	tag := c.CurrentConfig().Tag()
	if _, ok := tag.(names.UnitTag); !ok {
		return errors.NotValidf("expected a unit tag; got %q", tag)
	}
	return nil
}

// Wait waits for the k8s unit agent to finish.
func (c *k8sUnitAgent) Wait() error {
	<-c.dead
	return c.errReason
}

// Stop implements Worker.
func (c *k8sUnitAgent) Stop() error {
	c.runner.Kill()
	return c.Wait()
}

// Done signals the machine agent is finished
func (c *k8sUnitAgent) Done(err error) {
	c.errReason = err
	close(c.dead)
}

// Tag implements Agent.
func (c *k8sUnitAgent) Tag() names.UnitTag {
	// TODO(ycliuhw): Currently CAAS operator logs in using application tag and no password setup for units now.
	// Because k8sagent unit command logs in as an unit,  we need initialize password for CAAS unit.
	return c.CurrentConfig().Tag().(names.UnitTag)
}

func (c *k8sUnitAgent) applicationTag() names.Tag {
	return names.NewApplicationTag(c.applicationName)
}

// ChangeConfig implements Agent.
func (c *k8sUnitAgent) ChangeConfig(mutate agent.ConfigMutator) error {
	err := c.AgentConf.ChangeConfig(mutate)
	c.configChangedVal.Set(true)
	return errors.Trace(err)
}

// Workers returns a dependency.Engine running the k8s unit agent's responsibilities.
func (c *k8sUnitAgent) workers() (worker.Worker, error) {
	updateAgentConfLogging := func(loggingConfig string) error {
		return c.AgentConf.ChangeConfig(func(setter agent.ConfigSetter) error {
			setter.SetLoggingConfig(loggingConfig)
			return nil
		})
	}

	agentConfig := c.AgentConf.CurrentConfig()
	cfg := manifoldsConfig{
		Agent:                agent.APIHostPortsSetter{Agent: c},
		LogSource:            c.bufferedLogger.Logs(),
		LeadershipGuarantee:  30 * time.Second,
		AgentConfigChanged:   c.configChangedVal,
		ValidateMigration:    c.validateMigration,
		PrometheusRegisterer: c.prometheusRegistry,
		UpdateLoggerConfig:   updateAgentConfLogging,
		PreviousAgentVersion: agentConfig.UpgradedToVersion(),
		MachineLock:          c.machineLock,
		Clock:                c.clk,
	}
	manifolds := Manifolds(cfg)

	engine, err := dependency.NewEngine(engine.DependencyEngineConfig())
	if err != nil {
		return nil, err
	}
	if err := dependency.Install(engine, manifolds); err != nil {
		if err := worker.Stop(engine); err != nil {
			logger.Errorf("while stopping engine with bad manifolds: %v", err)
		}
		return nil, err
	}
	if err := addons.StartIntrospection(addons.IntrospectionConfig{
		Agent:              c,
		Engine:             engine,
		MachineLock:        c.machineLock,
		NewSocketName:      addons.DefaultIntrospectionSocketName,
		PrometheusGatherer: c.prometheusRegistry,
		WorkerFunc:         introspection.NewWorker,
		// If the k8sagent gains the ability to interact with the introspection
		// worker, the introspection worker should be configured with a clock
		// and hub. See the machine agent.
	}); err != nil {
		// If the introspection worker failed to start, we just log error
		// but continue. It is very unlikely to happen in the real world
		// as the only issue is connecting to the abstract domain socket
		// and the agent is controlled by by the OS to only have one.
		logger.Errorf("failed to start introspection worker: %v", err)
	}
	return engine, nil
}

func (c *k8sUnitAgent) Run(ctx *cmd.Context) (err error) {
	defer c.Done(err)
	ctx.Infof("starting k8sagent unit command")

	agentConfig := c.CurrentConfig()
	machineLock, err := machinelock.New(machinelock.Config{
		AgentName:   c.Tag().String(),
		Clock:       c.clk,
		Logger:      loggo.GetLogger("juju.machinelock"),
		LogFilename: agent.MachineLockLogFilename(agentConfig),
	})
	// There will only be an error if the required configuration
	// values are not passed in.
	if err != nil {
		return errors.Trace(err)
	}
	c.machineLock = machineLock

	ctx.Infof("k8sagent unit %q start (%s [%s])", c.Tag().String(), jujuversion.Current, runtime.Compiler)
	if flags := featureflag.String(); flags != "" {
		ctx.Warningf("developer feature flags enabled: %s", flags)
	}

	c.runner.StartWorker("unit", c.workers)
	return AgentDone(logger, c.runner.Wait())
}

// validateMigration is called by the migrationminion to help check
// that the agent will be ok when connected to a new controller.
func (c *k8sUnitAgent) validateMigration(apiCaller base.APICaller) error {
	// TODO(mjs) - more extensive checks to come.
	tag := c.CurrentConfig().Tag()
	unitTag, ok := tag.(names.UnitTag)
	if !ok {
		return errors.NotValidf("expected a unit tag; got %q", tag)
	}
	facade := uniter.NewState(apiCaller, unitTag)
	_, err := facade.Unit(unitTag)
	if err != nil {
		return errors.Trace(err)
	}
	model, err := facade.Model()
	if err != nil {
		return errors.Trace(err)
	}
	curModelUUID := c.CurrentConfig().Model().Id()
	newModelUUID := model.UUID
	if newModelUUID != curModelUUID {
		return errors.Errorf("model mismatch when validating: got %q, expected %q",
			newModelUUID, curModelUUID)
	}
	return nil
}

// AgentDone processes the error returned by an exiting agent.
func AgentDone(logger loggo.Logger, err error) error {
	err = errors.Cause(err)
	switch err {
	case jworker.ErrTerminateAgent:
		// These errors are swallowed here because we want to exit
		// the agent process without error, to avoid the init system
		// restarting us.
		err = nil
	}
	if err == jworker.ErrRestartAgent {
		// This does not seems to happen for k8s units.
		logger.Warningf("agent restarting")
	}
	return err
}
