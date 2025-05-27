// Copyright 2012-2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"
	"github.com/juju/pubsub/v2"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/addons"
	"github.com/juju/juju/agent/engine"
	agenterrors "github.com/juju/juju/agent/errors"
	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/api/base"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/constants"
	"github.com/juju/juju/cmd/containeragent/utils"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/paths"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/cmd"
	internaldependency "github.com/juju/juju/internal/dependency"
	"github.com/juju/juju/internal/featureflag"
	internallogger "github.com/juju/juju/internal/logger"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
	internalpubsub "github.com/juju/juju/internal/pubsub"
	"github.com/juju/juju/internal/upgrade"
	"github.com/juju/juju/internal/upgrades"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/introspection"
	"github.com/juju/juju/internal/worker/logsender"
	uniterworker "github.com/juju/juju/internal/worker/uniter"
	jnames "github.com/juju/juju/juju/names"
)

var (
	logger = internallogger.GetLogger("juju.cmd.containeragent.unit")

	jujuRun        = paths.JujuExec(paths.CurrentOS())
	jujuIntrospect = paths.JujuIntrospect(paths.CurrentOS())
)

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

	charmModifiedVersion    int
	envVars                 []string
	containerNames          []string
	colocatedWithController bool
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
	f.BoolVar(&c.colocatedWithController, "controller", false, "should be specified if this unit agent is running on the same machine as a controller")
}

func (c *containerUnitAgent) CharmModifiedVersion() int {
	return c.charmModifiedVersion
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

	var err error
	if c.runner, err = worker.NewRunner(worker.RunnerParams{
		Name:          "containeragent",
		IsFatal:       agenterrors.IsFatal,
		MoreImportant: agenterrors.MoreImportant,
		RestartDelay:  internalworker.RestartDelay,
		Logger:        internalworker.WrapLogger(logger),
	}); err != nil {
		return errors.Trace(err)
	}

	if err := ensureAgentConf(c.AgentConf); err != nil {
		return errors.Annotate(err, "ensuring agent conf file")
	}

	unitTag, ok := c.CurrentConfig().Tag().(names.UnitTag)
	if !ok {
		return errors.NotValidf("expected a unit tag; got %q", unitTag)
	}

	srcDir := path.Dir(os.Args[0])
	if err := c.ensureToolSymlinks(srcDir, c.DataDir(), unitTag); err != nil {
		return errors.Annotate(err, "ensuring agent tool symlinks")
	}
	containerNames := c.environment.Getenv(k8sconstants.EnvJujuContainerNames)
	if len(containerNames) > 0 {
		c.containerNames = strings.Split(containerNames, ",")
	}

	if err := introspection.WriteProfileFunctions(introspection.ProfileDir); err != nil {
		// This isn't fatal, just annoying.
		logger.Errorf(context.TODO(), "failed to write profile funcs: %v", err)
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
		path.Join(toolsDir, jnames.ContainerAgent),
		jujuRun, jujuIntrospect,
	} {
		if err = c.fileReaderWriter.Symlink(path.Join(srcPath, jnames.ContainerAgent), link); err != nil && !errors.Is(err, os.ErrExist) {
			return errors.Annotatef(err, "ensuring symlink %q", link)
		}
	}

	if err = c.fileReaderWriter.Symlink(path.Join(srcPath, jnames.Jujuc), path.Join(toolsDir, jnames.Jujuc)); err != nil && !errors.Is(err, os.ErrExist) {
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
func (c *containerUnitAgent) workers(sigTermCh chan os.Signal) (worker.Worker, error) {
	probePort := os.Getenv(constants.EnvHTTPProbePort)
	if probePort == "" {
		probePort = constants.DefaultHTTPProbePort
	}

	updateAgentConfLogging := func(loggingConfig string) error {
		return c.AgentConf.ChangeConfig(func(setter agent.ConfigSetter) error {
			setter.SetLoggingConfig(loggingConfig)
			return nil
		})
	}
	localHub := pubsub.NewSimpleHub(&pubsub.SimpleHubConfig{
		Logger: internalpubsub.WrapLogger(internallogger.GetLogger("juju.localhub")),
	})
	agentConfig := c.AgentConf.CurrentConfig()
	cfg := manifoldsConfig{
		Agent:                   agent.APIHostPortsSetter{Agent: c},
		LogSource:               c.bufferedLogger.Logs(),
		LeadershipGuarantee:     30 * time.Second,
		UpgradeStepsLock:        upgrade.NewLock(agentConfig, jujuversion.Current),
		PreUpgradeSteps:         upgrades.PreUpgradeSteps,
		UpgradeSteps:            upgrades.PerformUpgradeSteps,
		AgentConfigChanged:      c.configChangedVal,
		ValidateMigration:       c.validateMigration,
		PrometheusRegisterer:    c.prometheusRegistry,
		UpdateLoggerConfig:      updateAgentConfLogging,
		PreviousAgentVersion:    agentConfig.UpgradedToVersion(),
		ProbeAddress:            "localhost",
		ProbePort:               probePort,
		MachineLock:             c.machineLock,
		Clock:                   c.clk,
		CharmModifiedVersion:    c.CharmModifiedVersion(),
		ContainerNames:          c.containerNames,
		LocalHub:                localHub,
		ColocatedWithController: c.colocatedWithController,
		SignalCh:                sigTermCh,
	}
	manifolds := Manifolds(cfg)

	metrics := engine.NewMetrics()
	workerMetricsSink := metrics.ForModel(agentConfig.Model())
	eng, err := dependency.NewEngine(engine.DependencyEngineConfig(
		workerMetricsSink,
		internaldependency.WrapLogger(internallogger.GetLogger("juju.worker.dependency")),
	))
	if err != nil {
		return nil, err
	}
	if err := dependency.Install(eng, manifolds); err != nil {
		if err := worker.Stop(eng); err != nil {
			logger.Errorf(context.TODO(), "while stopping engine with bad manifolds: %v", err)
		}
		return nil, err
	}
	if err := addons.StartIntrospection(addons.IntrospectionConfig{
		AgentDir:           agentConfig.Dir(),
		Engine:             eng,
		MachineLock:        c.machineLock,
		PrometheusGatherer: c.prometheusRegistry,
		WorkerFunc:         introspection.NewWorker,
		Clock:              c.clk,
		Logger:             logger.Child("introspection"),
	}); err != nil {
		// If the introspection worker failed to start, we just log error
		// but continue. It is very unlikely to happen in the real world
		// as the only issue is connecting to the abstract domain socket
		// and the agent is controlled by the OS to only have one.
		logger.Errorf(context.TODO(), "failed to start introspection worker: %v", err)
	}
	if err := addons.RegisterEngineMetrics(c.prometheusRegistry, metrics, eng, workerMetricsSink); err != nil {
		// If the dependency engine metrics fail, continue on. This is unlikely
		// to happen in the real world, but should't stop or bring down an
		// agent.
		logger.Errorf(context.TODO(), "failed to start the dependency engine metrics %v", err)
	}

	return eng, nil
}

func (c *containerUnitAgent) Run(ctx *cmd.Context) (err error) {
	defer c.Done(err)
	ctx.Infof("starting containeragent unit command")

	agentConfig := c.CurrentConfig()
	machineLock, err := machinelock.New(machinelock.Config{
		AgentName:   c.Tag().String(),
		Clock:       c.clk,
		Logger:      internallogger.GetLogger("juju.machinelock"),
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

	sigTermCh := make(chan os.Signal, 1)
	signal.Notify(sigTermCh, syscall.SIGTERM)

	err = c.runner.StartWorker(ctx, "unit", func(ctx context.Context) (worker.Worker, error) {
		return c.workers(sigTermCh)
	})
	if err != nil {
		return errors.Annotate(err, "starting worker")
	}

	return AgentDone(logger, c.runner.Wait())
}

// validateMigration is called by the migrationminion to help check
// that the agent will be ok when connected to a new controller.
func (c *containerUnitAgent) validateMigration(ctx context.Context, apiCaller base.APICaller) error {
	// TODO(mjs) - more extensive checks to come.
	tag := c.CurrentConfig().Tag()
	unitTag, ok := tag.(names.UnitTag)
	if !ok {
		return errors.NotValidf("expected a unit tag; got %q", tag)
	}
	facade := uniter.NewClient(apiCaller, unitTag)
	_, err := facade.Unit(ctx, unitTag)
	if err != nil {
		return errors.Trace(err)
	}
	model, err := facade.Model(ctx)
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
func AgentDone(logger corelogger.Logger, err error) error {
	err = errors.Cause(err)
	switch err {
	case internalworker.ErrTerminateAgent:
		// These errors are swallowed here because we want to exit
		// the agent process without error, to avoid the init system
		// restarting us.
		logger.Infof(context.TODO(), "agent terminating")
		err = nil
	}
	if err == internalworker.ErrRestartAgent {
		// This does not seem to happen for k8s units.
		logger.Infof(context.TODO(), "agent restarting")
	}
	return err
}

func ensureAgentConf(ac agentconf.AgentConf) error {
	templateConfigPath := path.Join(ac.DataDir(), k8sconstants.TemplateFileNameAgentConf)
	logger.Debugf(context.TODO(), "template config path %s", templateConfigPath)
	config, err := agent.ReadConfig(templateConfigPath)
	if err != nil {
		return errors.Annotate(err, "reading template agent config file")
	}

	unitTag := config.Tag()
	configPath := agent.ConfigPath(ac.DataDir(), unitTag)
	logger.Debugf(context.TODO(), "config path %s", configPath)
	// if the rendered configuration already exists, use that copy
	// as it likely has updated api addresses or could have a newer password,
	// otherwise we need to copy the template.
	if _, err := os.Stat(configPath); err == nil {
		return ac.ReadConfig(unitTag.String())
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("cannot stat current config %s: %w", configPath, err)
	}

	configDir := path.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return errors.Annotatef(err, "making agent directory %q", configDir)
	}
	configBytes, err := config.Render()
	if err != nil {
		return errors.Trace(err)
	}
	if err := os.WriteFile(configPath, configBytes, 0644); err != nil {
		return errors.Annotate(err, "writing agent config file")
	}

	if err := ac.ReadConfig(unitTag.String()); err != nil {
		return errors.Annotate(err, "reading agent config file")
	}
	return nil
}
