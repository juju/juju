// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"os"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	agentconfig "github.com/juju/juju/agent/config"
	agentengine "github.com/juju/juju/agent/engine"
	agenterrors "github.com/juju/juju/agent/errors"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	"github.com/juju/juju/cmd/jujud-controller/agent/safemode"
	cmdutil "github.com/juju/juju/cmd/jujud-controller/util"
	"github.com/juju/juju/cmd/jujud/reboot"
	"github.com/juju/juju/internal/cmd"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/storage/looputil"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/dbaccessor"
	"github.com/juju/juju/internal/worker/logsender"
	"github.com/juju/juju/rpc/params"
)

// NewDBReplAgentCommand creates a Command that handles parsing
// command-line arguments and instantiating and running a
// MachineAgent.
func NewDBReplAgentCommand(
	ctx *cmd.Context,
	replMachineAgentFactory dbReplMachineAgentFactoryFnType,
	agentInitializer AgentInitializer,
	configFetcher agentconfig.AgentConfigWriter,
) cmd.Command {
	return &dbReplAgentCommand{
		ctx:                     ctx,
		replMachineAgentFactory: replMachineAgentFactory,
		agentInitializer:        agentInitializer,
		currentConfig:           configFetcher,
	}
}

type dbReplAgentCommand struct {
	cmd.CommandBase

	// This group of arguments is required.
	agentInitializer        AgentInitializer
	currentConfig           agentconfig.AgentConfigWriter
	replMachineAgentFactory dbReplMachineAgentFactoryFnType
	ctx                     *cmd.Context

	isCaas   bool
	agentTag names.Tag

	// The following are set via command-line flags.
	machineId string
	// TODO(controlleragent) - this will be in a new controller agent command
	controllerId string
}

// Init is called by the cmd system to initialize the structure for
// running.
func (a *dbReplAgentCommand) Init(args []string) error {
	if a.machineId == "" && a.controllerId == "" {
		return errors.New("either machine-id or controller-id must be set")
	}
	if a.machineId != "" && !names.IsValidMachine(a.machineId) {
		return errors.Errorf("--machine-id option must be a non-negative integer")
	}
	if a.controllerId != "" && !names.IsValidControllerAgent(a.controllerId) {
		return errors.Errorf("--controller-id option must be a non-negative integer")
	}
	if err := a.agentInitializer.CheckArgs(args); err != nil {
		return err
	}

	// Due to changes in the logging, and needing to care about old
	// models that have been upgraded, we need to explicitly remove the
	// file writer if one has been added, otherwise we will get duplicate
	// lines of all logging in the log file.
	_, _ = loggo.RemoveWriter("logfile")

	if a.machineId != "" {
		a.agentTag = names.NewMachineTag(a.machineId)
	} else {
		a.agentTag = names.NewControllerAgentTag(a.controllerId)
	}
	if err := agentconfig.ReadAgentConfig(a.currentConfig, a.agentTag.Id()); err != nil {
		return errors.Errorf("cannot read agent configuration: %v", err)
	}
	config := a.currentConfig.CurrentConfig()
	if err := os.MkdirAll(config.LogDir(), 0644); err != nil {
		logger.Warningf("cannot create log dir: %v", err)
	}
	a.isCaas = config.Value(agent.ProviderType) == k8sconstants.CAASProviderType

	return nil
}

// Run instantiates a MachineAgent and runs it.
func (a *dbReplAgentCommand) Run(c *cmd.Context) error {
	// Force the writing of the repl header to os.Stderr.
	if !c.Quiet() {
		fmt.Fprint(os.Stderr, replWarningHeader)
	}

	machineAgent, err := a.replMachineAgentFactory(a.agentTag, a.isCaas)
	if err != nil {
		return errors.Trace(err)
	}
	return machineAgent.Run(c)
}

// SetFlags adds the requisite flags to run this command.
func (a *dbReplAgentCommand) SetFlags(f *gnuflag.FlagSet) {
	a.agentInitializer.AddFlags(f)
	f.StringVar(&a.machineId, "machine-id", "", "id of the machine to run")
	f.StringVar(&a.controllerId, "controller-id", "", "id of the controller to run")
}

// Info returns usage information for the command.
func (a *dbReplAgentCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "db-repl",
		Purpose: "run a juju in db repl",
	})
}

const (
	replWarningHeader = `
Running DB REPL.
----------------

This is a DB REPL (Read-Eval-Print Loop) environment.

You can run arbitrary code here, including code that can modify the
state of the system. Be careful!
`
)

type dbReplMachineAgentFactoryFnType func(names.Tag, bool) (*replMachineAgent, error)

// DBReplMachineAgentFactoryFn returns a function which instantiates a
// replMachineAgent given a machineId.
func DBReplMachineAgentFactoryFn(
	agentConfWriter agentconfig.AgentConfigWriter,
	bufferedLogger *logsender.BufferedLogWriter,
	newDBWorkerFunc dbaccessor.NewDBWorkerFunc,
	rootDir string,
) dbReplMachineAgentFactoryFnType {
	return func(agentTag names.Tag, isCaasAgent bool) (*replMachineAgent, error) {
		return NewREPLMachineAgent(
			agentTag,
			agentConfWriter,
			bufferedLogger,
			worker.NewRunner(worker.RunnerParams{
				IsFatal:       agenterrors.IsFatal,
				MoreImportant: agenterrors.MoreImportant,
				RestartDelay:  jworker.RestartDelay,
				Logger:        logger,
			}),
			looputil.NewLoopDeviceManager(),
			newDBWorkerFunc,
			rootDir,
			isCaasAgent,
		)
	}
}

// NewREPLMachineAgent instantiates a new replMachineAgent.
func NewREPLMachineAgent(
	agentTag names.Tag,
	agentConfWriter agentconfig.AgentConfigWriter,
	bufferedLogger *logsender.BufferedLogWriter,
	runner *worker.Runner,
	loopDeviceManager looputil.LoopDeviceManager,
	newDBWorkerFunc dbaccessor.NewDBWorkerFunc,
	rootDir string,
	isCaasAgent bool,
) (*replMachineAgent, error) {
	a := &replMachineAgent{
		agentTag:          agentTag,
		AgentConfigWriter: agentConfWriter,
		configChangedVal:  voyeur.NewValue(true),
		bufferedLogger:    bufferedLogger,
		workersStarted:    make(chan struct{}),
		dead:              make(chan struct{}),
		runner:            runner,
		rootDir:           rootDir,
		newDBWorkerFunc:   newDBWorkerFunc,
		loopDeviceManager: loopDeviceManager,
		isCaasAgent:       isCaasAgent,
	}
	return a, nil
}

// replMachineAgent is responsible for tying together all functionality
// needed to orchestrate a Jujud instance which controls a machine.
type replMachineAgent struct {
	agentconfig.AgentConfigWriter

	dead             chan struct{}
	errReason        error
	agentTag         names.Tag
	runner           *worker.Runner
	rootDir          string
	bufferedLogger   *logsender.BufferedLogWriter
	configChangedVal *voyeur.Value

	workersStarted chan struct{}

	newDBWorkerFunc dbaccessor.NewDBWorkerFunc

	loopDeviceManager looputil.LoopDeviceManager

	isCaasAgent bool
}

// Wait waits for the repl machine agent to finish.
func (a *replMachineAgent) Wait() error {
	<-a.dead
	return a.errReason
}

// Stop stops the repl machine agent.
func (a *replMachineAgent) Stop() error {
	a.runner.Kill()
	return a.Wait()
}

// Done signals the repl machine agent is finished
func (a *replMachineAgent) Done(err error) {
	a.errReason = err
	close(a.dead)
}

// Run runs a repl machine agent.
func (a *replMachineAgent) Run(ctx *cmd.Context) (err error) {
	defer a.Done(err)

	if err := a.ReadConfig(a.Tag().String()); err != nil {
		return errors.Errorf("cannot read agent configuration: %v", err)
	}

	agentConfig := a.CurrentConfig()
	agentName := a.Tag().String()

	agentconf.SetupAgentLogging(internallogger.DefaultContext(), agentConfig)

	createEngine := a.makeEngineCreator(agentName, agentConfig.UpgradedToVersion())
	_ = a.runner.StartWorker("engine", createEngine)

	// At this point, all workers will have been configured to start
	close(a.workersStarted)
	err = a.runner.Wait()
	switch errors.Cause(err) {
	case jworker.ErrRebootMachine:
		logger.Infof("Caught reboot error")
		err = a.executeRebootOrShutdown(params.ShouldReboot)
	case jworker.ErrShutdownMachine:
		logger.Infof("Caught shutdown error")
		err = a.executeRebootOrShutdown(params.ShouldShutdown)
	}
	return cmdutil.AgentDone(logger, err)
}

func (a *replMachineAgent) Tag() names.Tag {
	return a.agentTag
}

func (a *replMachineAgent) ChangeConfig(mutate agent.ConfigMutator) error {
	err := a.AgentConfigWriter.ChangeConfig(mutate)
	a.configChangedVal.Set(true)
	return errors.Trace(err)
}

func (a *replMachineAgent) makeEngineCreator(
	agentName string, previousAgentVersion version.Number,
) func() (worker.Worker, error) {
	return func() (worker.Worker, error) {
		eng, err := dependency.NewEngine(agentengine.DependencyEngineConfig(
			dependency.DefaultMetrics(),
			internallogger.GetLogger("juju.worker.dependency"),
		))
		if err != nil {
			return nil, err
		}

		manifoldsCfg := safemode.ManifoldsConfig{
			PreviousAgentVersion: previousAgentVersion,
			AgentName:            agentName,
			Agent:                agent.APIHostPortsSetter{Agent: a},
			RootDir:              a.rootDir,
			AgentConfigChanged:   a.configChangedVal,
			NewDBWorkerFunc:      a.newDBWorkerFunc,
			LogSource:            a.bufferedLogger.Logs(),
			Clock:                clock.WallClock,
			IsCaasConfig:         a.isCaasAgent,

			SetupLogging: agentconf.SetupAgentLogging,
		}

		var manifolds dependency.Manifolds
		if a.isCaasAgent {
			manifolds = safemode.CAASManifolds(manifoldsCfg)
		} else {
			manifolds = safemode.IAASManifolds(manifoldsCfg)
		}

		if err := dependency.Install(eng, manifolds); err != nil {
			if err := worker.Stop(eng); err != nil {
				logger.Errorf("while stopping engine with bad manifolds: %v", err)
			}
			return nil, err
		}
		return eng, err
	}
}

func (a *replMachineAgent) executeRebootOrShutdown(action params.RebootAction) error {
	// block until all units/containers are ready, and reboot/shutdown
	finalize, err := reboot.NewRebootWaiter(a.CurrentConfig())
	if err != nil {
		return errors.Trace(err)
	}

	logger.Infof("Reboot: Executing reboot")
	err = finalize.ExecuteReboot(action)
	if err != nil {
		logger.Infof("Reboot: Error executing reboot: %v", err)
		return errors.Trace(err)
	}
	// We return ErrRebootMachine so the agent will simply exit without error
	// pending reboot/shutdown.
	return jworker.ErrRebootMachine
}
