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
	"github.com/juju/loggo/v3"
	"github.com/juju/names/v6"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/agent"
	agentconfig "github.com/juju/juju/agent/config"
	agentengine "github.com/juju/juju/agent/engine"
	agenterrors "github.com/juju/juju/agent/errors"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	"github.com/juju/juju/cmd/jujud/agent/safemode"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/core/semversion"
	internaldependency "github.com/juju/juju/internal/dependency"
	internallogger "github.com/juju/juju/internal/logger"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/dbaccessor"
	jujunames "github.com/juju/juju/juju/names"
)

// NewSafeModeAgentCommand creates a Command that handles parsing
// command-line arguments and instantiating and running a
// SafeModeControllerAgent.
func NewSafeModeAgentCommand(
	ctx *cmd.Context,
	safeModeControllerAgentFactory safeModeControllerAgentFactoryFnType,
	agentInitializer AgentInitializer,
	configFetcher agentconfig.AgentConfigWriter,
) cmd.Command {
	return &safeModeAgentCommand{
		ctx:                            ctx,
		safeModeControllerAgentFactory: safeModeControllerAgentFactory,
		agentInitializer:               agentInitializer,
		currentConfig:                  configFetcher,
	}
}

type safeModeAgentCommand struct {
	cmd.CommandBase

	// This group of arguments is required.
	agentInitializer               AgentInitializer
	currentConfig                  agentconfig.AgentConfigWriter
	safeModeControllerAgentFactory safeModeControllerAgentFactoryFnType
	ctx                            *cmd.Context

	isCaas   bool
	agentTag names.Tag

	// The following are set via command-line flags.
	controllerId string
}

// Init is called by the cmd system to initialize the structure for
// running.
func (a *safeModeAgentCommand) Init(args []string) error {
	if a.controllerId == "" {
		return errors.New("--controller-id must be set")
	}
	if !names.IsValidControllerAgent(a.controllerId) {
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

	a.agentTag = names.NewControllerAgentTag(a.controllerId)
	if err := agentconfig.ReadAgentConfig(a.currentConfig, a.agentTag.Id()); err != nil {
		return errors.Errorf("cannot read agent configuration: %v", err)
	}
	config := a.currentConfig.CurrentConfig()
	if err := os.MkdirAll(config.LogDir(), 0o644); err != nil {
		logger.Warningf(context.TODO(), "cannot create log dir: %v", err)
	}
	a.isCaas = config.Value(agent.ProviderType) == k8sconstants.CAASProviderType

	return nil
}

// Run instantiates a SafeModeControllerAgent and runs it.
func (a *safeModeAgentCommand) Run(c *cmd.Context) error {
	if err := ensuringControllerNotRunning(a.agentTag); err != nil {
		if errors.Is(err, errors.AlreadyExists) {
			fmt.Fprint(os.Stderr, safeModeControllerWarning)
			return nil
		}
		return err
	}

	// Force the writing of the safe-mode header to os.Stderr so it
	// cannot be suppressed (this is not a security measure, but it is
	// better than nothing).
	fmt.Fprint(os.Stderr, safeModeWarningHeader)

	controllerAgent, err := a.safeModeControllerAgentFactory(a.agentTag, a.isCaas)
	if err != nil {
		return errors.Trace(err)
	}
	return controllerAgent.Run(c)
}

// SetFlags adds the requisite flags to run this command.
func (a *safeModeAgentCommand) SetFlags(f *gnuflag.FlagSet) {
	a.agentInitializer.AddFlags(f)
	f.StringVar(&a.controllerId, "controller-id", "", "id of the controller to run")
}

// Info returns usage information for the command.
func (a *safeModeAgentCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "safe-mode",
		Purpose: "run a juju controller in safe mode",
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
	safeModeControllerWarning = `
Running in safe mode while the controller is running is dangerous. Please
stop the controller service before running in safe mode.
`
)

type safeModeControllerAgentFactoryFnType func(names.Tag, bool) (*SafeModeControllerAgent, error)

// SafeModeControllerAgentFactoryFn returns a function which instantiates a
// SafeModeControllerAgent given a controller agent tag.
func SafeModeControllerAgentFactoryFn(
	agentConfWriter agentconfig.AgentConfigWriter,
	newDBWorkerFunc dbaccessor.NewDBWorkerFunc,
) safeModeControllerAgentFactoryFnType {
	return func(agentTag names.Tag, isCaasAgent bool) (*SafeModeControllerAgent, error) {
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
		return NewSafeModeControllerAgent(
			agentTag,
			agentConfWriter,
			runner,
			newDBWorkerFunc,
			isCaasAgent,
		)
	}
}

// NewSafeModeControllerAgent instantiates a new SafeModeControllerAgent.
func NewSafeModeControllerAgent(
	agentTag names.Tag,
	agentConfWriter agentconfig.AgentConfigWriter,
	runner *worker.Runner,
	newDBWorkerFunc dbaccessor.NewDBWorkerFunc,
	isCaasAgent bool,
) (*SafeModeControllerAgent, error) {
	a := &SafeModeControllerAgent{
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

// SafeModeControllerAgent is a stripped-down agent that runs only the
// manifolds required to bring the Dqlite database online for recovery.
// It does not participate in the controller's normal dependency engine.
type SafeModeControllerAgent struct {
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

// Wait waits for the safe-mode controller agent to finish.
func (a *SafeModeControllerAgent) Wait() error {
	<-a.dead
	return a.errReason
}

// Stop stops the safe-mode controller agent.
func (a *SafeModeControllerAgent) Stop() error {
	a.runner.Kill()
	return a.Wait()
}

// Done signals the safe-mode controller agent is finished.
func (a *SafeModeControllerAgent) Done(err error) {
	a.errReason = err
	close(a.dead)
}

// WorkersStarted returns a channel that is closed once all top-level
// workers have been started.  Provided for testing purposes.
func (a *SafeModeControllerAgent) WorkersStarted() <-chan struct{} {
	return a.workersStarted
}

// Tag returns the controller agent's tag.
func (a *SafeModeControllerAgent) Tag() names.Tag {
	return a.agentTag
}

// ChangeConfig updates the agent's configuration and notifies
// listeners of the change.
func (a *SafeModeControllerAgent) ChangeConfig(mutate agent.ConfigMutator) error {
	err := a.AgentConfigWriter.ChangeConfig(mutate)
	a.configChangedVal.Set(true)
	return errors.Trace(err)
}

// Run runs a safe-mode controller agent.
func (a *SafeModeControllerAgent) Run(ctx *cmd.Context) (err error) {
	defer a.Done(err)

	if err := a.ReadConfig(a.Tag().String()); err != nil {
		return errors.Errorf("cannot read agent configuration: %v", err)
	}

	agentConfig := a.CurrentConfig()
	agentName := a.Tag().String()

	agentconf.SetupAgentLogging(internallogger.DefaultContext(), agentConfig)

	createEngine := a.makeEngineCreator(agentName, agentConfig.UpgradedToVersion())
	_ = a.runner.StartWorker(ctx, "engine", createEngine)

	// At this point, all workers will have been configured to start.
	close(a.workersStarted)
	err = a.runner.Wait()
	return cmdutil.AgentDone(logger, err)
}

func (a *SafeModeControllerAgent) makeEngineCreator(
	agentName string,
	previousAgentVersion semversion.Number,
) func(ctx context.Context) (worker.Worker, error) {
	return func(ctx context.Context) (worker.Worker, error) {
		eng, err := dependency.NewEngine(agentengine.DependencyEngineConfig(
			dependency.DefaultMetrics(),
			internaldependency.WrapLogger(
				internallogger.GetLogger("juju.worker.dependency"),
			),
		))
		if err != nil {
			return nil, err
		}

		manifoldsCfg := safemode.ManifoldsConfig{
			Agent:              agent.APIHostPortsSetter{Agent: a},
			AgentConfigChanged: a.configChangedVal,
			NewDBWorkerFunc:    a.newDBWorkerFunc,
			Clock:              clock.WallClock,
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
		return eng, nil
	}
}

// ensuringControllerNotRunning checks that the normal controller
// service is not active.  If it is, safe-mode must not start, because
// the two processes would compete for the Dqlite database.
func ensuringControllerNotRunning(tag names.Tag) error {
	svcName := fmt.Sprintf(
		"%s-controller-%s.service",
		jujunames.JujuController,
		tag.Id(),
	)
	cmd := exec.Command("systemctl", "check", svcName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Exit code 3 is ESRCH — no such process / unit not active.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 3 {
			return nil
		}
		return errors.Annotatef(err, "systemctl check failed")
	}
	if strings.TrimSpace(string(output)) != "active" {
		return nil
	}
	return errors.AlreadyExistsf("controller is running")
}
