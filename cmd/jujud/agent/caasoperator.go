// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"runtime"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/featureflag"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	apicaasoperator "github.com/juju/juju/api/caasoperator"
	"github.com/juju/juju/cmd/jujud/agent/caasoperator"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/core/machinelock"
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
	ApplicationName string
	runner          *worker.Runner
	bufferedLogger  *logsender.BufferedLogWriter
	setupLogging    func(agent.Config) error
	ctx             *cmd.Context
	dead            chan struct{}
	errReason       error
	machineLock     machinelock.Lock

	upgradeComplete gate.Lock

	prometheusRegistry *prometheus.Registry
}

// NewCaasOperatorAgent creates a new CAASOperatorAgent instance properly initialized.
func NewCaasOperatorAgent(ctx *cmd.Context, bufferedLogger *logsender.BufferedLogWriter) (*CaasOperatorAgent, error) {
	prometheusRegistry, err := newPrometheusRegistry()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &CaasOperatorAgent{
		AgentConf:          NewAgentConf(""),
		ctx:                ctx,
		dead:               make(chan struct{}),
		bufferedLogger:     bufferedLogger,
		prometheusRegistry: prometheusRegistry,
	}, nil
}

// Info implements Command.
func (op *CaasOperatorAgent) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "caasoperator",
		Purpose: "run a juju CAAS Operator",
	}
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

// Run implements Command.
func (op *CaasOperatorAgent) Run(ctx *cmd.Context) (err error) {
	defer op.Done(err)
	if err := op.ReadConfig(op.Tag().String()); err != nil {
		return err
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
	manifolds := CaasOperatorManifolds(caasoperator.ManifoldsConfig{
		Agent:                op,
		Clock:                clock.WallClock,
		LogSource:            op.bufferedLogger.Logs(),
		PrometheusRegisterer: op.prometheusRegistry,
		LeadershipGuarantee:  30 * time.Second,
		UpgradeStepsLock:     op.upgradeComplete,
		ValidateMigration:    op.validateMigration,
		MachineLock:          op.machineLock,
	})

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
