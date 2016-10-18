// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"runtime"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/utils/featureflag"
	"github.com/juju/utils/voyeur"
	"gopkg.in/juju/names.v2"
	"gopkg.in/natefinch/lumberjack.v2"
	"gopkg.in/tomb.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/cmd/jujud/agent/unit"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/introspection"
	"github.com/juju/juju/worker/logsender"
)

var (
	agentLogger = loggo.GetLogger("juju.jujud")

	// should be an explicit dependency, can't do it cleanly yet
	unitManifolds = unit.Manifolds
)

// UnitAgent is a cmd.Command responsible for running a unit agent.
type UnitAgent struct {
	cmd.CommandBase
	tomb tomb.Tomb
	AgentConf
	configChangedVal *voyeur.Value
	UnitName         string
	runner           worker.Runner
	bufferedLogs     logsender.LogRecordCh
	setupLogging     func(agent.Config) error
	logToStdErr      bool
	ctx              *cmd.Context

	// Used to signal that the upgrade worker will not
	// reboot the agent on startup because there are no
	// longer any immediately pending agent upgrades.
	// Channel used as a selectable bool (closed means true).
	initialUpgradeCheckComplete chan struct{}
}

// NewUnitAgent creates a new UnitAgent value properly initialized.
func NewUnitAgent(ctx *cmd.Context, bufferedLogs logsender.LogRecordCh) *UnitAgent {
	return &UnitAgent{
		AgentConf:        NewAgentConf(""),
		configChangedVal: voyeur.NewValue(true),
		ctx:              ctx,
		initialUpgradeCheckComplete: make(chan struct{}),
		bufferedLogs:                bufferedLogs,
	}
}

// Info returns usage information for the command.
func (a *UnitAgent) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "unit",
		Purpose: "run a juju unit agent",
	}
}

func (a *UnitAgent) SetFlags(f *gnuflag.FlagSet) {
	a.AgentConf.AddFlags(f)
	f.StringVar(&a.UnitName, "unit-name", "", "name of the unit to run")
	f.BoolVar(&a.logToStdErr, "log-to-stderr", false, "whether to log to standard error instead of log files")
}

// Init initializes the command for running.
func (a *UnitAgent) Init(args []string) error {
	if a.UnitName == "" {
		return cmdutil.RequiredError("unit-name")
	}
	if !names.IsValidUnit(a.UnitName) {
		return errors.Errorf(`--unit-name option expects "<service>/<n>" argument`)
	}
	if err := a.AgentConf.CheckArgs(args); err != nil {
		return err
	}
	a.runner = worker.NewRunner(cmdutil.IsFatal, cmdutil.MoreImportant, worker.RestartDelay)

	if !a.logToStdErr {
		if err := a.ReadConfig(a.Tag().String()); err != nil {
			return err
		}
		agentConfig := a.CurrentConfig()

		// the writer in ctx.stderr gets set as the loggo writer in github.com/juju/cmd/logging.go
		a.ctx.Stderr = &lumberjack.Logger{
			Filename:   agent.LogFilename(agentConfig),
			MaxSize:    300, // megabytes
			MaxBackups: 2,
		}

	}

	return nil
}

// Stop stops the unit agent.
func (a *UnitAgent) Stop() error {
	a.runner.Kill()
	return a.tomb.Wait()
}

// Run runs a unit agent.
func (a *UnitAgent) Run(ctx *cmd.Context) error {
	defer a.tomb.Done()
	if err := a.ReadConfig(a.Tag().String()); err != nil {
		return err
	}
	agentLogger.Infof("unit agent %v start (%s [%s])", a.Tag().String(), jujuversion.Current, runtime.Compiler)
	if flags := featureflag.String(); flags != "" {
		logger.Warningf("developer feature flags enabled: %s", flags)
	}

	a.runner.StartWorker("api", a.APIWorkers)
	err := cmdutil.AgentDone(logger, a.runner.Wait())
	a.tomb.Kill(err)
	return err
}

// APIWorkers returns a dependency.Engine running the unit agent's responsibilities.
func (a *UnitAgent) APIWorkers() (worker.Worker, error) {
	manifolds := unitManifolds(unit.ManifoldsConfig{
		Agent:               agent.APIHostPortsSetter{a},
		LogSource:           a.bufferedLogs,
		LeadershipGuarantee: 30 * time.Second,
		AgentConfigChanged:  a.configChangedVal,
		ValidateMigration:   a.validateMigration,
	})

	config := dependency.EngineConfig{
		IsFatal:     cmdutil.IsFatal,
		WorstError:  cmdutil.MoreImportantError,
		ErrorDelay:  3 * time.Second,
		BounceDelay: 10 * time.Millisecond,
	}
	engine, err := dependency.NewEngine(config)
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
		Agent:      a,
		Engine:     engine,
		WorkerFunc: introspection.NewWorker,
	}); err != nil {
		// If the introspection worker failed to start, we just log error
		// but continue. It is very unlikely to happen in the real world
		// as the only issue is connecting to the abstract domain socket
		// and the agent is controlled by by the OS to only have one.
		logger.Errorf("failed to start introspection worker: %v", err)
	}
	return engine, nil
}

func (a *UnitAgent) Tag() names.Tag {
	return names.NewUnitTag(a.UnitName)
}

func (a *UnitAgent) ChangeConfig(mutate agent.ConfigMutator) error {
	err := a.AgentConf.ChangeConfig(mutate)
	a.configChangedVal.Set(true)
	return errors.Trace(err)
}

// validateMigration is called by the migrationminion to help check
// that the agent will be ok when connected to a new controller.
func (a *UnitAgent) validateMigration(apiCaller base.APICaller) error {
	// TODO(mjs) - more extensive checks to come.
	unitTag := names.NewUnitTag(a.UnitName)
	facade := uniter.NewState(apiCaller, unitTag)
	_, err := facade.Unit(unitTag)
	if err != nil {
		return errors.Trace(err)
	}
	model, err := facade.Model()
	if err != nil {
		return errors.Trace(err)
	}
	curModelUUID := a.CurrentConfig().Model().Id()
	newModelUUID := model.UUID()
	if newModelUUID != curModelUUID {
		return errors.Errorf("model mismatch when validating: got %q, expected %q",
			newModelUUID, curModelUUID)
	}
	return nil
}
