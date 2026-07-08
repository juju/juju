// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo/v3"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	agentengine "github.com/juju/juju/agent/engine"
	agenterrors "github.com/juju/juju/agent/errors"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/jujud/agent/safemode"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/internal/controllerruntimeconfig"
	internaldependency "github.com/juju/juju/internal/dependency"
	internallogger "github.com/juju/juju/internal/logger"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/dbaccessor"
	jujunames "github.com/juju/juju/juju/names"
)

// NewSafeModeApplicationCommand creates a Command that handles parsing
// command-line arguments and instantiating and running a
// SafeModeControllerApplication.
func NewSafeModeApplicationCommand(
	ctx *cmd.Context,
	safeModeControllerAgentFactory SafeModeControllerApplicationFactoryFn,
	agentInitializer AgentInitializer,
) cmd.Command {
	return &safeModeApplicationCommand{
		ctx:                            ctx,
		safeModeControllerAgentFactory: safeModeControllerAgentFactory,
		agentInitializer:               agentInitializer,
	}
}

type safeModeApplicationCommand struct {
	cmd.CommandBase

	// This group of arguments is required.
	agentInitializer               AgentInitializer
	safeModeControllerAgentFactory SafeModeControllerApplicationFactoryFn
	ctx                            *cmd.Context

	runtimeConfig controllerruntimeconfig.ControllerRuntimeConfig
	agentTag      names.Tag

	// The following are set via command-line flags.
	controllerId string
}

// Init is called by the cmd system to initialize the structure for
// running.
func (a *safeModeApplicationCommand) Init(args []string) error {
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

	runtimeConfigPath := controllerruntimeconfig.ConfigPath(filepath.Join(
		a.agentInitializer.DataDir(), "agents", "controller-"+a.agentTag.Id(),
	))
	runtimeConfig, err := controllerruntimeconfig.ReadControllerRuntimeConfig(runtimeConfigPath)
	if err != nil {
		return errors.Errorf("cannot read controller runtime config: %v", err)
	}
	a.runtimeConfig = runtimeConfig

	if err := os.MkdirAll(runtimeConfig.LogDir, 0o644); err != nil {
		logger.Warningf(context.TODO(), "cannot create log dir: %v", err)
	}

	return nil
}

// Run instantiates a SafeModeControllerApplication and runs it.
func (a *safeModeApplicationCommand) Run(c *cmd.Context) error {
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

	controllerAgent, err := a.safeModeControllerAgentFactory(a.agentTag, a.runtimeConfig)
	if err != nil {
		return errors.Trace(err)
	}
	return controllerAgent.Run(c)
}

// SetFlags adds the requisite flags to run this command.
func (a *safeModeApplicationCommand) SetFlags(f *gnuflag.FlagSet) {
	a.agentInitializer.AddFlags(f)
	f.StringVar(&a.controllerId, "controller-id", "", "id of the controller to run")
}

// Info returns usage information for the command.
func (a *safeModeApplicationCommand) Info() *cmd.Info {
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

type SafeModeControllerApplicationFactoryFn func(names.Tag, controllerruntimeconfig.ControllerRuntimeConfig) (*SafeModeControllerApplication, error)

// SafeModeControllerAppliationFactory returns a function which instantiates a
// SafeModeControllerApplication given a controller agent tag.
func SafeModeControllerAppliationFactory(
	newDBWorkerFunc dbaccessor.NewDBWorkerFunc,
) SafeModeControllerApplicationFactoryFn {
	return func(agentTag names.Tag, runtimeConfig controllerruntimeconfig.ControllerRuntimeConfig) (*SafeModeControllerApplication, error) {
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
		return NewSafeModeControllerApplication(
			agentTag,
			runner,
			newDBWorkerFunc,
			runtimeConfig,
		)
	}
}

// NewSafeModeControllerApplication instantiates a new SafeModeControllerApplication.
func NewSafeModeControllerApplication(
	agentTag names.Tag,
	runner *worker.Runner,
	newDBWorkerFunc dbaccessor.NewDBWorkerFunc,
	runtimeConfig controllerruntimeconfig.ControllerRuntimeConfig,
) (*SafeModeControllerApplication, error) {
	a := &SafeModeControllerApplication{
		agentTag:        agentTag,
		workersStarted:  make(chan struct{}),
		dead:            make(chan struct{}),
		runner:          runner,
		newDBWorkerFunc: newDBWorkerFunc,
		runtimeConfig:   runtimeConfig,
	}
	return a, nil
}

// SafeModeControllerApplication is a stripped-down agent that runs only the
// manifolds required to bring the Dqlite database online for recovery. It does
// not participate in the controller's normal dependency engine.
type SafeModeControllerApplication struct {
	dead      chan struct{}
	errReason error
	agentTag  names.Tag
	runner    *worker.Runner

	workersStarted chan struct{}

	newDBWorkerFunc dbaccessor.NewDBWorkerFunc

	runtimeConfig controllerruntimeconfig.ControllerRuntimeConfig
}

type safeModeControllerStartupValueProvider struct {
	agent *SafeModeControllerApplication
}

func (p safeModeControllerStartupValueProvider) ControllerStartupValues() (dbaccessor.ControllerStartupValues, error) {
	runtimeCfg, err := controllerruntimeconfig.ReadControllerRuntimeConfig(
		controllerruntimeconfig.ConfigPath(
			filepath.Join(p.agent.runtimeConfig.DataDir, "agents", "controller-"+p.agent.Tag().Id()),
		),
	)
	if err != nil {
		return dbaccessor.ControllerStartupValues{}, errors.Trace(err)
	}
	return dbaccessor.ControllerStartupValues{
		ControllerID:          runtimeCfg.ControllerID,
		DataDir:               runtimeCfg.DataDir,
		DqlitePort:            runtimeCfg.DqlitePort,
		QueryTracingEnabled:   runtimeCfg.QueryTracingEnabled,
		QueryTracingThreshold: runtimeCfg.QueryTracingThreshold,
		DqliteBusyTimeout:     runtimeCfg.DqliteBusyTimeout,
		CACert:                runtimeCfg.CACert,
		ControllerCert:        runtimeCfg.ControllerCert,
		ControllerPrivateKey:  runtimeCfg.ControllerPrivateKey,
	}, nil
}

// Wait waits for the safe-mode controller agent to finish.
func (a *SafeModeControllerApplication) Wait() error {
	<-a.dead
	return a.errReason
}

// Stop stops the safe-mode controller agent.
func (a *SafeModeControllerApplication) Stop() error {
	a.runner.Kill()
	return a.Wait()
}

// Done signals the safe-mode controller agent is finished.
func (a *SafeModeControllerApplication) Done(err error) {
	a.errReason = err
	close(a.dead)
}

// WorkersStarted returns a channel that is closed once all top-level
// workers have been started.  Provided for testing purposes.
func (a *SafeModeControllerApplication) WorkersStarted() <-chan struct{} {
	return a.workersStarted
}

// Tag returns the controller agent's tag.
func (a *SafeModeControllerApplication) Tag() names.Tag {
	return a.agentTag
}

// Run runs a safe-mode controller agent.
func (a *SafeModeControllerApplication) Run(ctx *cmd.Context) (err error) {
	defer a.Done(err)

	setupLoggingFromStrings(
		internallogger.DefaultContext(),
		a.runtimeConfig.LoggingOverride,
		a.runtimeConfig.LoggingConfig,
	)

	createEngine := a.makeEngineCreator()
	_ = a.runner.StartWorker(ctx, "engine", createEngine)

	// At this point, all workers will have been configured to start.
	close(a.workersStarted)
	err = a.runner.Wait()
	return cmdutil.AgentDone(logger, err)
}

func (a *SafeModeControllerApplication) makeEngineCreator() func(ctx context.Context) (worker.Worker, error) {
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

		safeModeStartupValueProvider := safeModeControllerStartupValueProvider{
			agent: a,
		}
		manifoldsCfg := safemode.ManifoldsConfig{
			NewDBWorkerFunc:         a.newDBWorkerFunc,
			ControllerStartupValues: safeModeStartupValueProvider,
			ControllerID:            a.Tag().Id(),
			LogDir:                  a.runtimeConfig.LogDir,
			ConfigChangeSocketPath:  path.Join(a.runtimeConfig.DataDir, "configchange.socket"),
			Clock:                   clock.WallClock,
		}

		var manifolds dependency.Manifolds
		if a.runtimeConfig.LoopbackPreferred {
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
	c := exec.Command("systemctl", "check", svcName)
	output, err := c.CombinedOutput()
	if err != nil {
		// Exit code 3 is ESRCH — no such process / unit not active.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 3 {
			return nil
		}
		return errors.Annotatef(err, "systemctl check failed")
	}
	if strings.TrimSpace(string(output)) != "active" {
		return nil
	}
	return errors.AlreadyExistsf("controller is running")
}
