// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"io"
	"runtime"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/featureflag"
	"gopkg.in/natefinch/lumberjack.v2"
	"launchpad.net/gnuflag"
	"launchpad.net/tomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/leadership"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/network"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/apiaddressupdater"
	workerlogger "github.com/juju/juju/worker/logger"
	"github.com/juju/juju/worker/proxyupdater"
	"github.com/juju/juju/worker/rsyslog"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/upgrader"
)

var (
	agentLogger         = loggo.GetLogger("juju.jujud")
	reportClosedUnitAPI = func(io.Closer) {}
)

// UnitAgent is a cmd.Command responsible for running a unit agent.
type UnitAgent struct {
	cmd.CommandBase
	tomb tomb.Tomb
	AgentConf
	UnitName         string
	runner           worker.Runner
	setupLogging     func(agent.Config) error
	logToStdErr      bool
	ctx              *cmd.Context
	apiStateUpgrader APIStateUpgrader

	// Used to signal that the upgrade worker will not
	// reboot the agent on startup because there are no
	// longer any immediately pending agent upgrades.
	// Channel used as a selectable bool (closed means true).
	initialAgentUpgradeCheckComplete chan struct{}
}

// NewUnitAgent creates a new UnitAgent value properly initialized.
func NewUnitAgent(ctx *cmd.Context) *UnitAgent {
	return &UnitAgent{
		AgentConf: NewAgentConf(""),
		ctx:       ctx,
		initialAgentUpgradeCheckComplete: make(chan struct{}),
	}
}

func (a *UnitAgent) getUpgrader(st *api.State) APIStateUpgrader {
	if a.apiStateUpgrader != nil {
		return a.apiStateUpgrader
	}
	return st.Upgrader()
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
		return fmt.Errorf(`--unit-name option expects "<service>/<n>" argument`)
	}
	if err := a.AgentConf.CheckArgs(args); err != nil {
		return err
	}
	a.runner = worker.NewRunner(cmdutil.IsFatal, cmdutil.MoreImportant)

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
	agentConfig := a.CurrentConfig()

	agentLogger.Infof("unit agent %v start (%s [%s])", a.Tag().String(), version.Current, runtime.Compiler)
	if flags := featureflag.String(); flags != "" {
		logger.Warningf("developer feature flags enabled: %s", flags)
	}

	network.InitializeFromConfig(agentConfig)
	a.runner.StartWorker("api", a.APIWorkers)
	err := cmdutil.AgentDone(logger, a.runner.Wait())
	a.tomb.Kill(err)
	return err
}

func (a *UnitAgent) APIWorkers() (_ worker.Worker, err error) {
	agentConfig := a.CurrentConfig()
	dataDir := agentConfig.DataDir()
	hookLock, err := cmdutil.HookExecutionLock(dataDir)
	if err != nil {
		return nil, err
	}
	st, entity, err := OpenAPIState(agentConfig, a)
	if err != nil {
		return nil, err
	}
	unitTag, err := names.ParseUnitTag(entity.Tag())
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Ensure that the environment uuid is stored in the agent config.
	// Luckily the API has it recorded for us after we connect.
	if agentConfig.Environment().Id() == "" {
		err := a.ChangeConfig(func(setter agent.ConfigSetter) error {
			environTag, err := st.EnvironTag()
			if err != nil {
				return errors.Annotate(err, "no environment uuid set on api")
			}

			return setter.Migrate(agent.MigrateParams{
				Environment: environTag,
			})
		})
		if err != nil {
			logger.Warningf("unable to save environment uuid: %v", err)
			// Not really fatal, just annoying.
		}
	}

	defer func() {
		if err != nil {
			st.Close()
			reportClosedUnitAPI(st)
		}
	}()

	// Before starting any workers, ensure we record the Juju version this unit
	// agent is running.
	currentTools := &tools.Tools{Version: version.Current}
	apiStateUpgrader := a.getUpgrader(st)
	if err := apiStateUpgrader.SetVersion(agentConfig.Tag().String(), currentTools.Version); err != nil {
		return nil, errors.Annotate(err, "cannot set unit agent version")
	}

	runner := worker.NewRunner(cmdutil.ConnectionIsFatal(logger, st), cmdutil.MoreImportant)
	// start proxyupdater first to ensure proxy settings are correct
	runner.StartWorker("proxyupdater", func() (worker.Worker, error) {
		return proxyupdater.New(st.Environment(), false), nil
	})
	runner.StartWorker("upgrader", func() (worker.Worker, error) {
		return upgrader.NewAgentUpgrader(
			st.Upgrader(),
			agentConfig,
			agentConfig.UpgradedToVersion(),
			func() bool { return false },
			a.initialAgentUpgradeCheckComplete,
		), nil
	})
	runner.StartWorker("logger", func() (worker.Worker, error) {
		return workerlogger.NewLogger(st.Logger(), agentConfig), nil
	})
	runner.StartWorker("uniter", func() (worker.Worker, error) {
		uniterFacade, err := st.Uniter()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return uniter.NewUniter(uniterFacade, unitTag, leadership.NewClient(st), dataDir, hookLock), nil
	})

	runner.StartWorker("apiaddressupdater", func() (worker.Worker, error) {
		uniterFacade, err := st.Uniter()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return apiaddressupdater.NewAPIAddressUpdater(uniterFacade, a), nil
	})
	runner.StartWorker("rsyslog", func() (worker.Worker, error) {
		return cmdutil.NewRsyslogConfigWorker(st.Rsyslog(), agentConfig, rsyslog.RsyslogModeForwarding)
	})
	return cmdutil.NewCloseWorker(logger, runner, st), nil
}

func (a *UnitAgent) Tag() names.Tag {
	return names.NewUnitTag(a.UnitName)
}
