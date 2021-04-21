// Copyright 2012-2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit

import (
	"io/ioutil"
	"os"
	"path"
	"runtime"
	"strings"
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
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/containeragent/utils"
	"github.com/juju/juju/cmd/jujud/agent/addons"
	"github.com/juju/juju/cmd/jujud/agent/agentconf"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	agenterrors "github.com/juju/juju/cmd/jujud/agent/errors"
	"github.com/juju/juju/core/machinelock"
	jnames "github.com/juju/juju/juju/names"
	jujuversion "github.com/juju/juju/version"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/introspection"
	"github.com/juju/juju/worker/logsender"
	uniterworker "github.com/juju/juju/worker/uniter"
)

var logger = loggo.GetLogger("juju.cmd.containeragent.unit")

type containerUnitAgent struct {
	cmd.CommandBase
	agentconf.AgentConf

	configChangedVal *voyeur.Value
	clk              clock.Clock
	runner           *worker.Runner
	bufferedLogger   *logsender.BufferedLogWriter
	ctx              *cmd.Context
	dead             chan struct{}
	errReason        error
	machineLock      machinelock.Lock

	prometheusRegistry *prometheus.Registry

	fileReaderWriter utils.FileReaderWriter
	environment      utils.Environment

	charmModifiedVersion int
	envVars              []string
	containerNames       []string
}

// New creates containeragent unit command.
func New(ctx *cmd.Context, bufferedLogger *logsender.BufferedLogWriter) (cmd.Command, error) {
	prometheusRegistry, err := addons.NewPrometheusRegistry()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &containerUnitAgent{
		AgentConf:          agentconf.NewAgentConf(""),
		configChangedVal:   voyeur.NewValue(true),
		ctx:                ctx,
		clk:                clock.WallClock,
		dead:               make(chan struct{}),
		bufferedLogger:     bufferedLogger,
		prometheusRegistry: prometheusRegistry,
		fileReaderWriter:   utils.NewFileReaderWriter(),
		environment:        utils.NewEnvironment(),
	}, nil
}

// Info returns a description of the command.
func (c *containerUnitAgent) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "unit",
		Purpose: "Start containeragent.",
	})
}

// SetFlags implements Command.
func (c *containerUnitAgent) SetFlags(f *gnuflag.FlagSet) {
	c.AgentConf.AddFlags(f)
	f.IntVar(&c.charmModifiedVersion, "charm-modified-version", -1, "charm modified version to validate downloaded charm is for the provided infrastructure")
	f.Var(cmd.NewAppendStringsValue(&c.envVars), "append-env", "can be specified multiple times and with the form ENV_VAR=VALUE where VALUE can be empty or contain unexpanded variables using $OTHER_ENV")
}

func (c *containerUnitAgent) CharmModifiedVersion() int {
	return c.charmModifiedVersion
}

func (c *containerUnitAgent) ensureAgentConf(dataDir string) error {
	templateConfigPath := path.Join(dataDir, k8sconstants.TemplateFileNameAgentConf)
	logger.Debugf("template config path %s", templateConfigPath)
	config, err := agent.ReadConfig(templateConfigPath)
	if err != nil {
		return errors.Annotate(err, "reading template agent config file")
	}
	unitTag := config.Tag()
	configPath := agent.ConfigPath(dataDir, unitTag)
	logger.Debugf("config path %s", configPath)
	configDir := path.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return errors.Annotatef(err, "making agent directory %q", configDir)
	}
	configBytes, err := config.Render()
	if err != nil {
		return errors.Trace(err)
	}
	if err := ioutil.WriteFile(configPath, configBytes, 0644); err != nil {
		return errors.Annotate(err, "writing agent config file")
	}

	if err := c.ReadConfig(unitTag.String()); err != nil {
		return errors.Annotate(err, "reading agent config file")
	}
	return nil
}

// Init initializes the command for running.
func (c *containerUnitAgent) Init(args []string) error {
	if err := c.AgentConf.CheckArgs(args); err != nil {
		return err
	}

	// Append environment with passed in values.
	for _, e := range c.envVars {
		kv := strings.SplitN(e, "=", 2)
		k, v := "", ""
		switch len(kv) {
		case 1:
			k = kv[0]
		case 2:
			k = kv[0]
			v = kv[1]
		default:
			return errors.NotValidf("invalid K=V pair for --append-env")
		}
		if v == "" {
			err := c.environment.Unsetenv(k)
			if err != nil {
				return errors.Trace(err)
			}
		} else {
			err := c.environment.Setenv(k, c.environment.ExpandEnv(v))
			if err != nil {
				return errors.Trace(err)
			}
		}
	}

	c.runner = worker.NewRunner(worker.RunnerParams{
		IsFatal:       agenterrors.IsFatal,
		MoreImportant: agenterrors.MoreImportant,
		RestartDelay:  jworker.RestartDelay,
	})

	dataDir := c.DataDir()

	if err := c.ensureAgentConf(dataDir); err != nil {
		return errors.Annotate(err, "ensuring agent conf file")
	}

	unitTag, ok := c.CurrentConfig().Tag().(names.UnitTag)
	if !ok {
		return errors.NotValidf("expected a unit tag; got %q", unitTag)
	}

	srcDir := path.Dir(os.Args[0])
	if err := c.ensureToolSymlinks(srcDir, dataDir, unitTag); err != nil {
		return errors.Annotate(err, "ensuring agent tool symlinks")
	}
	containerNames := c.environment.Getenv("JUJU_CONTAINER_NAMES")
	if len(containerNames) > 0 {
		c.containerNames = strings.Split(containerNames, ",")
	}
	return nil
}

func (c *containerUnitAgent) ensureToolSymlinks(srcPath, dataDir string, unitTag names.UnitTag) error {
	// Setup tool symlinks
	uniterPaths := uniterworker.NewPaths(dataDir, unitTag, nil)
	toolsDir := uniterPaths.GetToolsDir()
	err := c.fileReaderWriter.RemoveAll(toolsDir)
	if err != nil {
		return errors.Annotatef(err, "removing old tools dir")
	}
	err = c.fileReaderWriter.MkdirAll(toolsDir, 0755)
	if err != nil {
		return errors.Annotate(err, "creating tools dir")
	}

	for _, link := range []string{
		jnames.ContainerAgent,
		jnames.JujuExec,
		jnames.JujuIntrospect,
	} {
		newName := path.Join(toolsDir, link)
		if err = c.fileReaderWriter.Symlink(path.Join(srcPath, jnames.ContainerAgent), newName); err != nil {
			return errors.Annotatef(err, "ensuring symlink %q", link)
		}
	}

	if err = c.fileReaderWriter.Symlink(path.Join(srcPath, jnames.Jujuc), path.Join(toolsDir, jnames.Jujuc)); err != nil {
		return errors.Annotatef(err, "ensuring symlink %q", jnames.Jujuc)
	}
	return nil
}

// Wait waits for the k8s unit agent to finish.
func (c *containerUnitAgent) Wait() error {
	<-c.dead
	return c.errReason
}

// Stop implements Worker.
func (c *containerUnitAgent) Stop() error {
	c.runner.Kill()
	return c.Wait()
}

// Done signals the machine agent is finished
func (c *containerUnitAgent) Done(err error) {
	c.errReason = err
	close(c.dead)
}

// Tag implements Agent.
func (c *containerUnitAgent) Tag() names.UnitTag {
	return c.CurrentConfig().Tag().(names.UnitTag)
}

// ChangeConfig implements Agent.
func (c *containerUnitAgent) ChangeConfig(mutate agent.ConfigMutator) error {
	err := c.AgentConf.ChangeConfig(mutate)
	c.configChangedVal.Set(true)
	return errors.Trace(err)
}

// Workers returns a dependency.Engine running the k8s unit agent's responsibilities.
func (c *containerUnitAgent) workers() (worker.Worker, error) {
	probePort := os.Getenv(k8sconstants.EnvAgentHTTPProbePort)
	if probePort == "" {
		return nil, errors.NotValidf("env %s missing", k8sconstants.EnvAgentHTTPProbePort)
	}

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
		ProbePort:            probePort,
		MachineLock:          c.machineLock,
		Clock:                c.clk,
		CharmModifiedVersion: c.CharmModifiedVersion(),
		ContainerNames:       c.containerNames,
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
	}); err != nil {
		// If the introspection worker failed to start, we just log error
		// but continue. It is very unlikely to happen in the real world
		// as the only issue is connecting to the abstract domain socket
		// and the agent is controlled by by the OS to only have one.
		logger.Errorf("failed to start introspection worker: %v", err)
	}
	return engine, nil
}

func (c *containerUnitAgent) Run(ctx *cmd.Context) (err error) {
	defer c.Done(err)
	ctx.Infof("starting containeragent unit command")

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

	ctx.Infof("containeragent unit %q start (%s [%s])", c.Tag().String(), jujuversion.Current, runtime.Compiler)
	if flags := featureflag.String(); flags != "" {
		ctx.Warningf("developer feature flags enabled: %s", flags)
	}

	_ = c.runner.StartWorker("unit", c.workers)
	return AgentDone(logger, c.runner.Wait())
}

// validateMigration is called by the migrationminion to help check
// that the agent will be ok when connected to a new controller.
func (c *containerUnitAgent) validateMigration(apiCaller base.APICaller) error {
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
