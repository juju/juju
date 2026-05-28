// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"
	"fmt"
	"io"
	"os"

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
	"github.com/juju/juju/cmd/jujud/agent/dbrepl"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	internaldependency "github.com/juju/juju/internal/dependency"
	internallogger "github.com/juju/juju/internal/logger"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/dbreplaccessor"
)

// NewDBReplAgentCommand creates a Command that handles parsing
// command-line arguments and instantiating and running a
// replControllerAgent.
func NewDBReplAgentCommand(
	ctx *cmd.Context,
	replControllerAgentFactory dbReplControllerAgentFactoryFnType,
	agentInitializer AgentInitializer,
	configFetcher agentconfig.AgentConfigWriter,
) cmd.Command {
	return &dbReplAgentCommand{
		ctx:                        ctx,
		replControllerAgentFactory: replControllerAgentFactory,
		agentInitializer:           agentInitializer,
		currentConfig:              configFetcher,
	}
}

type dbReplAgentCommand struct {
	cmd.CommandBase

	// This group of arguments is required.
	agentInitializer           AgentInitializer
	currentConfig              agentconfig.AgentConfigWriter
	replControllerAgentFactory dbReplControllerAgentFactoryFnType
	ctx                        *cmd.Context

	isCaas   bool
	agentTag names.Tag

	// The following are set via command-line flags.
	controllerId string
}

// Init is called by the cmd system to initialize the structure for
// running.
func (a *dbReplAgentCommand) Init(args []string) error {
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
	if err := os.MkdirAll(config.LogDir(), 0644); err != nil {
		logger.Warningf(context.TODO(), "cannot create log dir: %v", err)
	}
	a.isCaas = config.Value(agent.ProviderType) == k8sconstants.CAASProviderType

	return nil
}

// Run instantiates a replControllerAgent and runs it.
func (a *dbReplAgentCommand) Run(c *cmd.Context) error {
	// Force the writing of the repl header to os.Stderr.
	if !c.Quiet() {
		fmt.Fprint(os.Stderr, replWarningHeader)
	}

	controllerAgent, err := a.replControllerAgentFactory(a.agentTag, a.isCaas)
	if err != nil {
		return errors.Trace(err)
	}
	return controllerAgent.Run(c)
}

// SetFlags adds the requisite flags to run this command.
func (a *dbReplAgentCommand) SetFlags(f *gnuflag.FlagSet) {
	a.agentInitializer.AddFlags(f)
	f.StringVar(&a.controllerId, "controller-id", "", "id of the controller to run")
}

// Info returns usage information for the command.
func (a *dbReplAgentCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "db-repl",
		Purpose: "run a juju controller db repl",
	})
}

const replWarningHeader = `
Running DB REPL.
----------------

This is a DB REPL (Read-Eval-Print Loop) environment.

You can run arbitrary code here, including code that can modify the
state of the system. Be careful!

Type '.help' for help.
`

type dbReplControllerAgentFactoryFnType func(names.Tag, bool) (*replControllerAgent, error)

// DBReplControllerAgentFactoryFn returns a function which instantiates a
// replControllerAgent given a controller agent tag.
func DBReplControllerAgentFactoryFn(
	agentConfWriter agentconfig.AgentConfigWriter,
	newDBReplWorkerFunc dbreplaccessor.NewDBReplWorkerFunc,
) dbReplControllerAgentFactoryFnType {
	return func(agentTag names.Tag, isCaasAgent bool) (*replControllerAgent, error) {
		runner, err := worker.NewRunner(worker.RunnerParams{
			Name:          "repl",
			IsFatal:       agenterrors.IsFatal,
			MoreImportant: agenterrors.MoreImportant,
			RestartDelay:  internalworker.RestartDelay,
			Logger:        internalworker.WrapLogger(logger),
		})
		if err != nil {
			return nil, errors.Trace(err)
		}
		return NewREPLControllerAgent(
			agentTag,
			agentConfWriter,
			runner,
			newDBReplWorkerFunc,
			isCaasAgent,
		)
	}
}

// NewREPLControllerAgent instantiates a new replControllerAgent.
func NewREPLControllerAgent(
	agentTag names.Tag,
	agentConfWriter agentconfig.AgentConfigWriter,
	runner *worker.Runner,
	newDBReplWorkerFunc dbreplaccessor.NewDBReplWorkerFunc,
	isCaasAgent bool,
) (*replControllerAgent, error) {
	a := &replControllerAgent{
		agentTag:            agentTag,
		AgentConfigWriter:   agentConfWriter,
		configChangedVal:    voyeur.NewValue(true),
		dead:                make(chan struct{}),
		runner:              runner,
		newDBReplWorkerFunc: newDBReplWorkerFunc,
		isCaasAgent:         isCaasAgent,
	}
	return a, nil
}

// replControllerAgent is a stripped-down agent that runs only the
// manifolds required to provide an interactive Dqlite REPL.  It does
// not participate in the controller's normal dependency engine.
type replControllerAgent struct {
	agentconfig.AgentConfigWriter

	dead             chan struct{}
	errReason        error
	agentTag         names.Tag
	runner           *worker.Runner
	configChangedVal *voyeur.Value

	newDBReplWorkerFunc dbreplaccessor.NewDBReplWorkerFunc

	isCaasAgent bool
}

// Wait waits for the repl controller agent to finish.
func (a *replControllerAgent) Wait() error {
	<-a.dead
	return a.errReason
}

// Stop stops the repl controller agent.
func (a *replControllerAgent) Stop() error {
	a.runner.Kill()
	return a.Wait()
}

// Done signals the repl controller agent is finished.
func (a *replControllerAgent) Done(err error) {
	a.errReason = err
	close(a.dead)
}

// Tag returns the controller agent's tag.
func (a *replControllerAgent) Tag() names.Tag {
	return a.agentTag
}

// ChangeConfig updates the agent's configuration and notifies
// listeners of the change.
func (a *replControllerAgent) ChangeConfig(mutate agent.ConfigMutator) error {
	err := a.AgentConfigWriter.ChangeConfig(mutate)
	a.configChangedVal.Set(true)
	return errors.Trace(err)
}

// Run runs a repl controller agent.
func (a *replControllerAgent) Run(ctx *cmd.Context) (err error) {
	defer a.Done(err)

	if err := a.ReadConfig(a.Tag().String()); err != nil {
		return errors.Errorf("cannot read agent configuration: %v", err)
	}

	agentConfig := a.CurrentConfig()
	agentconf.SetupAgentLogging(internallogger.DefaultContext(), agentConfig)

	createEngine := a.makeEngineCreator(ctx.Stdout, ctx.Stderr, ctx.Stdin)
	_ = a.runner.StartWorker(ctx, "engine", createEngine)

	// At this point, all workers will have been configured to start.
	err = a.runner.Wait()
	return cmdutil.AgentDone(logger, err)
}

func (a *replControllerAgent) makeEngineCreator(
	stdout, stderr io.Writer,
	stdin io.Reader,
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

		manifoldsCfg := dbrepl.ManifoldsConfig{
			Agent:               agent.APIHostPortsSetter{Agent: a},
			AgentConfigChanged:  a.configChangedVal,
			NewDBReplWorkerFunc: a.newDBReplWorkerFunc,
			ControllerID:        a.Tag().Id(),
			Clock:               clock.WallClock,
			Stdout:              stdout,
			Stderr:              stderr,
			Stdin:               stdin,
		}

		var manifolds dependency.Manifolds
		if a.isCaasAgent {
			manifolds = dbrepl.CAASManifolds(manifoldsCfg)
		} else {
			manifolds = dbrepl.IAASManifolds(manifoldsCfg)
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
