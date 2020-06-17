// Copyright 2012-2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit

import (
	"runtime"

	"github.com/juju/clock"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/utils/voyeur"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/agent"
	jujucmd "github.com/juju/juju/cmd"
	jujudagent "github.com/juju/juju/cmd/jujud/agent"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/upgrades"
	jujuversion "github.com/juju/juju/version"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/introspection"
	"github.com/juju/juju/worker/logsender"
	"github.com/juju/juju/worker/upgradesteps"
)

var logger = loggo.GetLogger("juju.cmd.k8sagent.unit")

type unitCommand struct {
	cmd.CommandBase
	jujudagent.AgentConf
	configChangedVal *voyeur.Value
	// Unit command of k8sagent only knows application name but not unit name.
	// It will configure out the unit name from agent.conf file by itself.
	ApplicationName string
	clk             clock.Clock
	runner          *worker.Runner
	bufferedLogger  *logsender.BufferedLogWriter
	setupLogging    func(agent.Config) error
	ctx             *cmd.Context
	dead            chan struct{}
	errReason       error
	machineLock     machinelock.Lock

	preUpgradeSteps upgrades.PreUpgradeStepsFunc
	upgradeComplete gate.Lock

	prometheusRegistry *prometheus.Registry
}

func New(ctx *cmd.Context, bufferedLogger *logsender.BufferedLogWriter) (cmd.Command, error) {
	prometheusRegistry, err := jujudagent.NewPrometheusRegistry()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &unitCommand{
		AgentConf:          jujudagent.NewAgentConf(""),
		configChangedVal:   voyeur.NewValue(true),
		ctx:                ctx,
		clk:                clock.WallClock,
		dead:               make(chan struct{}),
		bufferedLogger:     bufferedLogger,
		prometheusRegistry: prometheusRegistry,
		preUpgradeSteps:    upgrades.PreUpgradeSteps,
	}, nil
}

// Info returns a description of the command.
func (c *unitCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "unit",
		Purpose: "starting a k8s agent",
	})
}

// SetFlags implements Command.
func (c *unitCommand) SetFlags(f *gnuflag.FlagSet) {
	// --data-dir /juju  ?
	c.AgentConf.AddFlags(f)
	f.StringVar(&c.ApplicationName, "application-name", "", "name of the application")
}

// Init initializes the command for running.
func (c *unitCommand) Init(args []string) error {
	if c.ApplicationName == "" {
		return cmdutil.RequiredError("application-name")
	}
	if !names.IsValidApplication(c.ApplicationName) {
		return errors.Errorf(`--application-name option expects "<application>" argument`)
	}
	if err := c.AgentConf.CheckArgs(args); err != nil {
		return err
	}
	c.runner = worker.NewRunner(worker.RunnerParams{
		IsFatal:       cmdutil.IsFatal,
		MoreImportant: cmdutil.MoreImportant,
		RestartDelay:  jworker.RestartDelay,
	})

	// Note: c.Tag() == applicationTag && c.CurrentConfig().Tag() == unitTag !!
	if err := c.ReadConfig(c.Tag().String()); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Wait waits for the k8s unit agent to finish.
func (c *unitCommand) Wait() error {
	<-c.dead
	return c.errReason
}

// Stop implements Worker.
func (c *unitCommand) Stop() error {
	c.runner.Kill()
	return c.Wait()
}

// Done signals the machine agent is finished
func (c *unitCommand) Done(err error) {
	c.errReason = err
	close(c.dead)
}

// Tag implements Agent.
func (c *unitCommand) Tag() names.Tag {
	return names.NewApplicationTag(c.ApplicationName)
}

// ChangeConfig implements Agent.
func (c *unitCommand) ChangeConfig(mutate agent.ConfigMutator) error {
	err := c.AgentConf.ChangeConfig(mutate)
	c.configChangedVal.Set(true)
	return errors.Trace(err)
}

// Workers returns a dependency.Engine running the k8s unit agent's responsibilities.
func (c *unitCommand) workers() (worker.Worker, error) {
	updateAgentConfLogging := func(loggingConfig string) error {
		return c.AgentConf.ChangeConfig(func(setter agent.ConfigSetter) error {
			setter.SetLoggingConfig(loggingConfig)
			return nil
		})
	}

	agentConfig := c.AgentConf.CurrentConfig()
	cfg := manifoldsConfig{
		// TODO
		UpdateLoggerConfig:   updateAgentConfLogging,
		PreviousAgentVersion: agentConfig.UpgradedToVersion(),
	}
	manifolds := Manifolds(cfg)

	engine, err := dependency.NewEngine(jujudagent.DependencyEngineConfig())
	if err != nil {
		return nil, err
	}
	if err := dependency.Install(engine, manifolds); err != nil {
		if err := worker.Stop(engine); err != nil {
			logger.Errorf("while stopping engine with bad manifolds: %v", err)
		}
		return nil, err
	}
	if err := jujudagent.StartIntrospection(jujudagent.IntrospectionConfig{
		Agent:              c,
		Engine:             engine,
		MachineLock:        c.machineLock,
		NewSocketName:      jujudagent.DefaultIntrospectionSocketName,
		PrometheusGatherer: c.prometheusRegistry,
		WorkerFunc:         introspection.NewWorker,
	}); err != nil {
		// If the introspection worker failed to start, we just log error
		// but continue. It is very unlikely to happen in the real world
		// as the only issue is connecting to the abstract domain socket
		// and the agent is controlled by by the OS to only have one.
		logger.Errorf("failed to start introspection worker: %v", err)
	}
	return engine, nil
}

func (c *unitCommand) Run(_ *cmd.Context) (err error) {
	defer c.Done(err)
	logger.Infof("starting k8sagent unit command")

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
	c.upgradeComplete = upgradesteps.NewLock(agentConfig)

	logger.Infof("k8sagent unit %q start (%s [%s])", c.Tag().String(), jujuversion.Current, runtime.Compiler)
	if flags := featureflag.String(); flags != "" {
		logger.Warningf("developer feature flags enabled: %s", flags)
	}

	c.runner.StartWorker("api", c.workers)
	return cmdutil.AgentDone(logger, c.runner.Wait())
}
