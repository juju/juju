// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v6"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	agentconfig "github.com/juju/juju/agent/config"
	agentengine "github.com/juju/juju/agent/engine"
	agenterrors "github.com/juju/juju/agent/errors"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	"github.com/juju/juju/cmd/jujud-controller/agent/safemode"
	cmdutil "github.com/juju/juju/cmd/jujud-controller/util"
	"github.com/juju/juju/cmd/jujud/reboot"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/cmd"
	internaldependency "github.com/juju/juju/internal/dependency"
	internallogger "github.com/juju/juju/internal/logger"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/dbaccessor"
	"github.com/juju/juju/rpc/params"
)

// NewSafeModeAgentCommand creates a Command that handles parsing
// command-line arguments and instantiating and running a
// MachineAgent.
func NewSafeModeAgentCommand(
	ctx *cmd.Context,
	safeModeMachineAgentFactory safeModeMachineAgentFactoryFnType,
	agentInitializer AgentInitializer,
	configFetcher agentconfig.AgentConfigWriter,
) cmd.Command {
	return &safeModeAgentCommand{
		ctx:                         ctx,
		safeModeMachineAgentFactory: safeModeMachineAgentFactory,
		agentInitializer:            agentInitializer,
		currentConfig:               configFetcher,
	}
}

type safeModeAgentCommand struct {
	cmd.CommandBase

	// This group of arguments is required.
	agentInitializer            AgentInitializer
	currentConfig               agentconfig.AgentConfigWriter
	safeModeMachineAgentFactory safeModeMachineAgentFactoryFnType
	ctx                         *cmd.Context

	isCaas   bool
	agentTag names.Tag

	// The following are set via command-line flags.
	machineId string
	// TODO(controlleragent) - this will be in a new controller agent command
	controllerId string
}

// Init is called by the cmd system to initialize the structure for
// running.
func (a *safeModeAgentCommand) Init(args []string) error {
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
		logger.Warningf(context.TODO(), "cannot create log dir: %v", err)
	}
	a.isCaas = config.Value(agent.ProviderType) == k8sconstants.CAASProviderType

	return nil
}

// Run instantiates a MachineAgent and runs it.
func (a *safeModeAgentCommand) Run(c *cmd.Context) error {
	if err := ensuringJujudNotRunning(a.agentTag); err != nil {
		if errors.Is(err, errors.AlreadyExists) {
			fmt.Fprint(os.Stderr, safeModeJujudWarning)
			return nil
		}
		return err
	}

	// Force the writing of the safemode header to os.Stderr, so it can't be
	// bypassed with a flag (obviously, this is not a security measure, but
	// it's better than nothing).
	fmt.Fprint(os.Stderr, safeModeWarningHeader)

	machineAgent, err := a.safeModeMachineAgentFactory(a.agentTag, a.isCaas)
	if err != nil {
		return errors.Trace(err)
	}
	return machineAgent.Run(c)
}

// SetFlags adds the requisite flags to run this command.
func (a *safeModeAgentCommand) SetFlags(f *gnuflag.FlagSet) {
	a.agentInitializer.AddFlags(f)
	f.StringVar(&a.machineId, "machine-id", "", "id of the machine to run")
	f.StringVar(&a.controllerId, "controller-id", "", "id of the controller to run")
}

// Info returns usage information for the command.
func (a *safeModeAgentCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "safe-mode",
		Purpose: "run a juju in safe mode",
	})
}

const (
	safeModeWarningHeader = `
Running in safe mode. 
---------------------

This is a special mode of operation that allows you to have single node
only access to the database. This is useful if you have a database that is
corrupt and you want to recover data from it.

This will only stand up the minimum set of services required to allow
you to connect to the database and perform recovery operations.

Use at your own risk.
`
	safeModeJujudWarning = `
Running in safe mode while jujud is running is dangerous. Please stop jujud
before running in safe mode.
`
)

type safeModeMachineAgentFactoryFnType func(names.Tag, bool) (*SafeModeMachineAgent, error)

// SafeModeMachineAgentFactoryFn returns a function which instantiates a
// SafeModeMachineAgent given a machineId.
func SafeModeMachineAgentFactoryFn(
	agentConfWriter agentconfig.AgentConfigWriter,
	newDBWorkerFunc dbaccessor.NewDBWorkerFunc,
) safeModeMachineAgentFactoryFnType {
	return func(agentTag names.Tag, isCaasAgent bool) (*SafeModeMachineAgent, error) {
		runner, err := worker.NewRunner(worker.RunnerParams{
			Name:          "safemode",
			IsFatal:       agenterrors.IsFatal,
			MoreImportant: agenterrors.MoreImportant,
			RestartDelay:  internalworker.RestartDelay,
			Logger:        internalworker.WrapLogger(logger),
		})
		if err != nil {
			return nil, errors.Trace(err)
		}
		return NewSafeModeMachineAgent(
			agentTag,
			agentConfWriter,
			runner,
			newDBWorkerFunc,
			isCaasAgent,
		)
	}
}

// NewSafeModeMachineAgent instantiates a new SafeModeMachineAgent.
func NewSafeModeMachineAgent(
	agentTag names.Tag,
	agentConfWriter agentconfig.AgentConfigWriter,
	runner *worker.Runner,
	newDBWorkerFunc dbaccessor.NewDBWorkerFunc,
	isCaasAgent bool,
) (*SafeModeMachineAgent, error) {
	a := &SafeModeMachineAgent{
		agentTag:          agentTag,
		AgentConfigWriter: agentConfWriter,
		configChangedVal:  voyeur.NewValue(true),
		workersStarted:    make(chan struct{}),
		dead:              make(chan struct{}),
		runner:            runner,
		newDBWorkerFunc:   newDBWorkerFunc,
		isCaasAgent:       isCaasAgent,
	}
	return a, nil
}

// SafeModeMachineAgent is responsible for tying together all functionality
// needed to orchestrate a Jujud instance which controls a machine.
type SafeModeMachineAgent struct {
	agentconfig.AgentConfigWriter

	dead             chan struct{}
	errReason        error
	agentTag         names.Tag
	runner           *worker.Runner
	configChangedVal *voyeur.Value

	workersStarted chan struct{}

	newDBWorkerFunc dbaccessor.NewDBWorkerFunc

	isCaasAgent bool
}

// Wait waits for the safe mode machine agent to finish.
func (a *SafeModeMachineAgent) Wait() error {
	<-a.dead
	return a.errReason
}

// Stop stops the safe mode machine agent.
func (a *SafeModeMachineAgent) Stop() error {
	a.runner.Kill()
	return a.Wait()
}

// Done signals the safe mode machine agent is finished
func (a *SafeModeMachineAgent) Done(err error) {
	a.errReason = err
	close(a.dead)
}

// Run runs a safe mode machine agent.
func (a *SafeModeMachineAgent) Run(ctx *cmd.Context) (err error) {
	defer a.Done(err)

	if err := a.ReadConfig(a.Tag().String()); err != nil {
		return errors.Errorf("cannot read agent configuration: %v", err)
	}

	agentConfig := a.CurrentConfig()
	agentName := a.Tag().String()

	agentconf.SetupAgentLogging(internallogger.DefaultContext(), agentConfig)

	createEngine := a.makeEngineCreator(agentName, agentConfig.UpgradedToVersion())
	_ = a.runner.StartWorker(ctx, "engine", createEngine)

	// At this point, all workers will have been configured to start
	close(a.workersStarted)
	err = a.runner.Wait()
	switch errors.Cause(err) {
	case internalworker.ErrRebootMachine:
		logger.Infof(context.TODO(), "Caught reboot error")
		err = a.executeRebootOrShutdown(params.ShouldReboot)
	case internalworker.ErrShutdownMachine:
		logger.Infof(context.TODO(), "Caught shutdown error")
		err = a.executeRebootOrShutdown(params.ShouldShutdown)
	}
	return cmdutil.AgentDone(logger, err)
}

func (a *SafeModeMachineAgent) Tag() names.Tag {
	return a.agentTag
}

func (a *SafeModeMachineAgent) ChangeConfig(mutate agent.ConfigMutator) error {
	err := a.AgentConfigWriter.ChangeConfig(mutate)
	a.configChangedVal.Set(true)
	return errors.Trace(err)
}

func (a *SafeModeMachineAgent) makeEngineCreator(
	agentName string, previousAgentVersion semversion.Number,
) func(ctx context.Context) (worker.Worker, error) {
	return func(ctx context.Context) (worker.Worker, error) {
		eng, err := dependency.NewEngine(agentengine.DependencyEngineConfig(
			dependency.DefaultMetrics(),
			internaldependency.WrapLogger(internallogger.GetLogger("juju.worker.dependency")),
		))
		if err != nil {
			return nil, err
		}

		manifoldsCfg := safemode.ManifoldsConfig{
			Agent:              agent.APIHostPortsSetter{Agent: a},
			AgentConfigChanged: a.configChangedVal,
			NewDBWorkerFunc:    a.newDBWorkerFunc,
			Clock:              clock.WallClock,
			IsCaasConfig:       a.isCaasAgent,
		}

		var manifolds dependency.Manifolds
		if a.isCaasAgent {
			manifolds = safemode.CAASManifolds(manifoldsCfg)
		} else {
			manifolds = safemode.IAASManifolds(manifoldsCfg)
		}

		if err := dependency.Install(eng, manifolds); err != nil {
			if err := worker.Stop(eng); err != nil {
				logger.Errorf(context.TODO(), "while stopping engine with bad manifolds: %v", err)
			}
			return nil, err
		}
		return eng, err
	}
}

func (a *SafeModeMachineAgent) executeRebootOrShutdown(action params.RebootAction) error {
	// block until all units/containers are ready, and reboot/shutdown
	finalize, err := reboot.NewRebootWaiter(a.CurrentConfig())
	if err != nil {
		return errors.Trace(err)
	}

	logger.Infof(context.TODO(), "Reboot: Executing reboot")
	err = finalize.ExecuteReboot(action)
	if err != nil {
		logger.Infof(context.TODO(), "Reboot: Error executing reboot: %v", err)
		return errors.Trace(err)
	}
	// We return ErrRebootMachine so the agent will simply exit without error
	// pending reboot/shutdown.
	return internalworker.ErrRebootMachine
}

func ensuringJujudNotRunning(tag names.Tag) error {
	cmd := exec.Command("systemctl", "check", fmt.Sprintf("jujud-machine-%s.service", tag.Id()))
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Exit code of 3 is ESRCH, which means no such process.
		// See: https://man7.org/linux/man-pages/man3/errno.3.html
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 3 {
			return nil
		}
		return errors.Annotatef(err, "systemctrl check failed")
	}
	if strings.TrimSpace(string(output)) != "active" {
		return nil
	}
	return errors.AlreadyExistsf("jujud is running")
}
