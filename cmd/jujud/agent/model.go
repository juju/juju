// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

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
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/cmd"
	internaldependency "github.com/juju/juju/internal/dependency"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/upgrade"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/gate"
	"github.com/juju/juju/internal/worker/logsender"
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

	var err error
	m.runner, err = worker.NewRunner(worker.RunnerParams{
		Name:          "model",
		IsFatal:       agenterrors.IsFatal,
		MoreImportant: agenterrors.MoreImportant,
		RestartDelay:  internalworker.RestartDelay,
		Logger:        internalworker.WrapLogger(logger),
	})
	if err != nil {
		return errors.Trace(err)
	}

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
		logger.Errorf(context.TODO(), "reading initial agent config file: %v", err)
		return errors.Trace(err)
	}

	templateFile := filepath.Join(agent.Dir(m.DataDir(), m.Tag()), caasconstants.TemplateFileNameAgentConf)
	if err := copyFile(agent.ConfigPath(m.DataDir(), m.Tag()), templateFile); err != nil {
		logger.Errorf(context.TODO(), "copying agent config file template: %v", err)
		return errors.Trace(err)
	}
	return m.ReadConfig(m.Tag().String())
}

func copyFile(dest, source string) error {
	df, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0600)
	if err != nil {
		return errors.Trace(err)
	}
	defer df.Close()

	f, err := os.Open(source)
	if err != nil {
		return errors.Trace(err)
	}
	defer f.Close()

	_, err = io.Copy(df, f)
	return errors.Trace(err)
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
	logger.Infof(ctx, "caas model operator start (%s [%s])", jujuversion.Current,
		runtime.Compiler)

	if err := m.maybeCopyAgentConfig(); err != nil {
		return errors.Annotate(err, "creating agent config from template")
	}

	m.upgradeStepsLock = upgrade.NewLock(m.CurrentConfig(), jujuversion.Current)

	_ = m.runner.StartWorker(ctx, "modeloperator", m.Workers)
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

func (m *ModelCommand) Workers(ctx context.Context) (worker.Worker, error) {
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
		internaldependency.WrapLogger(internallogger.GetLogger("juju.worker.dependency")),
	))
	if err != nil {
		return nil, err
	}
	if err := dependency.Install(engine, manifolds); err != nil {
		if err := worker.Stop(engine); err != nil {
			logger.Errorf(context.TODO(), "while stopping engine with bad manifolds: %v", err)
		}
		return nil, err
	}

	return engine, nil
}
