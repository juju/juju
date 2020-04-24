// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

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
	"github.com/juju/juju/api/base"
	apicaasoperator "github.com/juju/juju/api/caasoperator"
	caasprovider "github.com/juju/juju/caas/kubernetes/provider"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/jujud/agent/caasoperator"
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

var (
	// Should be an explicit dependency, can't do it cleanly yet.
	// Exported for testing.
	CaasOperatorManifolds = caasoperator.Manifolds
)

// CaasOperatorAgent is a cmd.Command responsible for running a CAAS operator agent.
type CaasOperatorAgent struct {
	cmd.CommandBase
	AgentConf
	configChangedVal *voyeur.Value
	ApplicationName  string
	runner           *worker.Runner
	bufferedLogger   *logsender.BufferedLogWriter
	setupLogging     func(agent.Config) error
	ctx              *cmd.Context
	dead             chan struct{}
	errReason        error
	machineLock      machinelock.Lock

	preUpgradeSteps upgrades.PreUpgradeStepsFunc
	upgradeComplete gate.Lock

	prometheusRegistry *prometheus.Registry

	configure func(*caasoperator.ManifoldsConfig) error
}

// NewCaasOperatorAgent creates a new CAASOperatorAgent instance properly initialized.
func NewCaasOperatorAgent(
	ctx *cmd.Context,
	bufferedLogger *logsender.BufferedLogWriter,
	configure func(*caasoperator.ManifoldsConfig) error,
) (*CaasOperatorAgent, error) {
	prometheusRegistry, err := newPrometheusRegistry()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &CaasOperatorAgent{
		AgentConf:          NewAgentConf(""),
		configChangedVal:   voyeur.NewValue(true),
		ctx:                ctx,
		dead:               make(chan struct{}),
		bufferedLogger:     bufferedLogger,
		prometheusRegistry: prometheusRegistry,
		preUpgradeSteps:    upgrades.PreUpgradeSteps,
		configure:          configure,
	}, nil
}

// Info implements Command.
func (op *CaasOperatorAgent) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "caasoperator",
		Purpose: "run a juju CAAS Operator",
	})
}

// SetFlags implements Command.
func (op *CaasOperatorAgent) SetFlags(f *gnuflag.FlagSet) {
	op.AgentConf.AddFlags(f)
	f.StringVar(&op.ApplicationName, "application-name", "", "name of the application")
}

// Init initializes the command for running.
func (op *CaasOperatorAgent) Init(args []string) error {
	if op.ApplicationName == "" {
		return cmdutil.RequiredError("application-name")
	}
	if !names.IsValidApplication(op.ApplicationName) {
		return errors.Errorf(`--application-name option expects "<application>" argument`)
	}
	if err := op.AgentConf.CheckArgs(args); err != nil {
		return err
	}
	op.runner = worker.NewRunner(worker.RunnerParams{
		IsFatal:       cmdutil.IsFatal,
		MoreImportant: cmdutil.MoreImportant,
		RestartDelay:  jworker.RestartDelay,
	})
	return nil
}

// Wait waits for the CaasOperator agent to finish.
func (op *CaasOperatorAgent) Wait() error {
	<-op.dead
	return op.errReason
}

// Stop implements Worker.
func (op *CaasOperatorAgent) Stop() error {
	op.runner.Kill()
	return op.Wait()
}

// Done signals the machine agent is finished
func (op *CaasOperatorAgent) Done(err error) {
	op.errReason = err
	close(op.dead)
}

// maybeCopyAgentConfig copies the read-only agent config template
// to the writeable agent config file if the file doesn't yet exist.
func (op *CaasOperatorAgent) maybeCopyAgentConfig() error {
	err := op.ReadConfig(op.Tag().String())
	if err == nil {
		return nil
	}
	if !os.IsNotExist(errors.Cause(err)) {
		logger.Errorf("reading initial agent config file: %v", err)
		return errors.Trace(err)
	}
	templateFile := filepath.Join(agent.Dir(op.DataDir(), op.Tag()), caasprovider.TemplateFileNameAgentConf)
	if err := copyFile(agent.ConfigPath(op.DataDir(), op.Tag()), templateFile); err != nil {
		logger.Errorf("copying agent config file template: %v", err)
		return errors.Trace(err)
	}
	return op.ReadConfig(op.Tag().String())
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

// Run implements Command.
func (op *CaasOperatorAgent) Run(ctx *cmd.Context) (err error) {
	defer op.Done(err)
	if err := op.maybeCopyAgentConfig(); err != nil {
		return errors.Annotate(err, "creating agent config from template")
	}
	agentConfig := op.CurrentConfig()
	machineLock, err := machinelock.New(machinelock.Config{
		AgentName:   op.Tag().String(),
		Clock:       clock.WallClock,
		Logger:      loggo.GetLogger("juju.machinelock"),
		LogFilename: agent.MachineLockLogFilename(agentConfig),
	})
	// There will only be an error if the required configuration
	// values are not passed in.
	if err != nil {
		return errors.Trace(err)
	}
	op.machineLock = machineLock
	op.upgradeComplete = upgradesteps.NewLock(agentConfig)

	logger.Infof("caas operator %v start (%s [%s])", op.Tag().String(), jujuversion.Current, runtime.Compiler)
	if flags := featureflag.String(); flags != "" {
		logger.Warningf("developer feature flags enabled: %s", flags)
	}

	op.runner.StartWorker("api", op.Workers)
	return cmdutil.AgentDone(logger, op.runner.Wait())
}

// Workers returns a dependency.Engine running the operator's responsibilities.
func (op *CaasOperatorAgent) Workers() (worker.Worker, error) {
	updateAgentConfLogging := func(loggingConfig string) error {
		return op.AgentConf.ChangeConfig(func(setter agent.ConfigSetter) error {
			setter.SetLoggingConfig(loggingConfig)
			return nil
		})
	}

	agentConfig := op.AgentConf.CurrentConfig()
	manifoldConfig := caasoperator.ManifoldsConfig{
		Agent:                agent.APIHostPortsSetter{op},
		AgentConfigChanged:   op.configChangedVal,
		Clock:                clock.WallClock,
		LogSource:            op.bufferedLogger.Logs(),
		UpdateLoggerConfig:   updateAgentConfLogging,
		PrometheusRegisterer: op.prometheusRegistry,
		LeadershipGuarantee:  15 * time.Second,
		PreUpgradeSteps:      op.preUpgradeSteps,
		UpgradeStepsLock:     op.upgradeComplete,
		ValidateMigration:    op.validateMigration,
		MachineLock:          op.machineLock,
		PreviousAgentVersion: agentConfig.UpgradedToVersion(),
	}
	if op.configure != nil {
		if err := op.configure(&manifoldConfig); err != nil {
			return nil, errors.Trace(err)
		}
	}
	manifolds := CaasOperatorManifolds(manifoldConfig)

	engine, err := dependency.NewEngine(dependencyEngineConfig())
	if err != nil {
		return nil, err
	}
	if err := dependency.Install(engine, manifolds); err != nil {
		if err := worker.Stop(engine); err != nil {
			logger.Errorf("while stopping engine with bad manifolds: %v", err)
		}
		return nil, err
	}
	if err := startIntrospection(introspectionConfig{
		Agent:              op,
		Engine:             engine,
		MachineLock:        op.machineLock,
		NewSocketName:      DefaultIntrospectionSocketName,
		PrometheusGatherer: op.prometheusRegistry,
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

// Tag implements Agent.
func (op *CaasOperatorAgent) Tag() names.Tag {
	return names.NewApplicationTag(op.ApplicationName)
}

// ChangeConfig implements Agent.
func (op *CaasOperatorAgent) ChangeConfig(mutate agent.ConfigMutator) error {
	err := op.AgentConf.ChangeConfig(mutate)
	op.configChangedVal.Set(true)
	return errors.Trace(err)
}

// validateMigration is called by the migrationminion to help check
// that the agent will be ok when connected to a new controller.
func (op *CaasOperatorAgent) validateMigration(apiCaller base.APICaller) error {
	// TODO(wallyworld) - more extensive checks to come.
	facade := apicaasoperator.NewClient(apiCaller)
	_, err := facade.Life(op.ApplicationName)
	if err != nil {
		return errors.Trace(err)
	}
	model, err := facade.Model()
	if err != nil {
		return errors.Trace(err)
	}
	curModelUUID := op.CurrentConfig().Model().Id()
	newModelUUID := model.UUID
	if newModelUUID != curModelUUID {
		return errors.Errorf("model mismatch when validating: got %q, expected %q",
			newModelUUID, curModelUUID)
	}
	return nil
}
