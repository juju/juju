// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/utils/v3/voyeur"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine"
	agenterrors "github.com/juju/juju/agent/errors"
	"github.com/juju/juju/caas"
	caasprovider "github.com/juju/juju/caas/kubernetes/provider"
	caasconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	"github.com/juju/juju/cmd/jujud/agent/modeloperator"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/internal/upgrade"
	jujuversion "github.com/juju/juju/version"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/logsender"
)

// ModelCommand is a cmd.Command responsible for running a model agent.
type ModelCommand struct {
	agentconf.AgentConf
	cmd.CommandBase
	configChangedVal *voyeur.Value
	dead             chan struct{}
	errReason        error
	ModelUUID        string
	runner           *worker.Runner
	upgradeStepsLock gate.Lock
	bufferedLogger   *logsender.BufferedLogWriter
}

// Done signals the model agent is finished
func (m *ModelCommand) Done(err error) {
	m.errReason = err
	close(m.dead)
}

// Info implements Command
func (m *ModelCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "model",
		Purpose: "run a juju model operator",
	})
}

// Init initializers the command for running
func (m *ModelCommand) Init(args []string) error {
	if m.ModelUUID == "" {
		return agenterrors.RequiredError("model-uuid")
	}

	if err := m.AgentConf.CheckArgs(args); err != nil {
		return err
	}

	m.runner = worker.NewRunner(worker.RunnerParams{
		IsFatal:       agenterrors.IsFatal,
		MoreImportant: agenterrors.MoreImportant,
		RestartDelay:  jworker.RestartDelay,
		Logger:        logger,
	})
	return nil
}

// maybeCopyAgentConfig copies the read-only agent config template
// to the writeable agent config file if the file doesn't yet exist.
func (m *ModelCommand) maybeCopyAgentConfig() error {
	err := m.ReadConfig(m.Tag().String())
	if err == nil {
		return nil
	}
	if !os.IsNotExist(errors.Cause(err)) {
		logger.Errorf("reading initial agent config file: %v", err)
		return errors.Trace(err)
	}

	templateFile := filepath.Join(agent.Dir(m.DataDir(), m.Tag()), caasconstants.TemplateFileNameAgentConf)
	if err := copyFile(agent.ConfigPath(m.DataDir(), m.Tag()), templateFile); err != nil {
		logger.Errorf("copying agent config file template: %v", err)
		return errors.Trace(err)
	}
	return m.ReadConfig(m.Tag().String())
}

// NewModelCommand creates a new ModelCommand instance properly initialized
func NewModelCommand(
	bufferedLogger *logsender.BufferedLogWriter,
) *ModelCommand {
	return &ModelCommand{
		AgentConf:        agentconf.NewAgentConf(""),
		configChangedVal: voyeur.NewValue(true),
		dead:             make(chan struct{}),
		bufferedLogger:   bufferedLogger,
	}
}

// Run implements Command
func (m *ModelCommand) Run(ctx *cmd.Context) error {
	logger.Infof("caas model operator start (%s [%s])", jujuversion.Current,
		runtime.Compiler)

	if err := m.maybeCopyAgentConfig(); err != nil {
		return errors.Annotate(err, "creating agent config from template")
	}

	m.upgradeStepsLock = upgrade.NewLock(m.CurrentConfig(), jujuversion.Current)

	_ = m.runner.StartWorker("modeloperator", m.Workers)
	return cmdutil.AgentDone(logger, m.runner.Wait())
}

// SetFlags implements Command
func (m *ModelCommand) SetFlags(f *gnuflag.FlagSet) {
	m.AgentConf.AddFlags(f)
	f.StringVar(&m.ModelUUID, "model-uuid", "", "uuid of the model")
}

// Stop implements worker
func (m *ModelCommand) Stop() error {
	m.runner.Kill()
	return m.Wait()
}

func (m *ModelCommand) Tag() names.Tag {
	return names.NewModelTag(m.ModelUUID)
}

func (m *ModelCommand) Wait() error {
	<-m.dead
	return m.errReason
}

func (m *ModelCommand) Workers() (worker.Worker, error) {
	port := os.Getenv(caasprovider.EnvModelAgentHTTPPort)
	if port == "" {
		return nil, errors.NotValidf("env %s missing", caasprovider.EnvModelAgentHTTPPort)
	}

	svcName := os.Getenv(caasprovider.EnvModelAgentCAASServiceName)
	if svcName == "" {
		return nil, errors.NotValidf("env %s missing", caasprovider.EnvModelAgentCAASServiceName)
	}

	svcNamespace := os.Getenv(caasprovider.EnvModelAgentCAASServiceNamespace)
	if svcNamespace == "" {
		return nil, errors.NotValidf("env %s missing", caasprovider.EnvModelAgentCAASServiceNamespace)
	}

	updateAgentConfLogging := func(loggingConfig string) error {
		return m.AgentConf.ChangeConfig(func(setter agent.ConfigSetter) error {
			setter.SetLoggingConfig(loggingConfig)
			return nil
		})
	}

	manifolds := modeloperator.Manifolds(modeloperator.ManifoldConfig{
		Agent:                  agent.APIHostPortsSetter{Agent: m},
		AgentConfigChanged:     m.configChangedVal,
		NewContainerBrokerFunc: caas.New,
		Port:                   port,
		LogSource:              m.bufferedLogger.Logs(),
		ServiceName:            svcName,
		ServiceNamespace:       svcNamespace,
		UpdateLoggerConfig:     updateAgentConfLogging,
		PreviousAgentVersion:   m.CurrentConfig().UpgradedToVersion(),
		UpgradeStepsLock:       m.upgradeStepsLock,
	})

	// TODO (stickupkid): There is no prometheus registry at this level, we
	// should work out the best way to get it into here.
	engine, err := dependency.NewEngine(engine.DependencyEngineConfig(
		dependency.DefaultMetrics(),
		loggo.GetLogger("juju.worker.dependency"),
	))
	if err != nil {
		return nil, err
	}
	if err := dependency.Install(engine, manifolds); err != nil {
		if err := worker.Stop(engine); err != nil {
			logger.Errorf("while stopping engine with bad manifolds: %v", err)
		}
		return nil, err
	}

	return engine, nil
}
