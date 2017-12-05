// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"fmt"
	"os"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/clock"
	"github.com/kr/pretty"
	"gopkg.in/juju/charm.v6/hooks"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/status"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/caasoperator/commands"
	"github.com/juju/juju/worker/caasoperator/hook"
	"github.com/juju/juju/worker/caasoperator/operation"
	"github.com/juju/juju/worker/caasoperator/runner"
	"github.com/juju/juju/worker/caasoperator/runner/context"
	"github.com/juju/juju/worker/catacomb"
)

var logger = loggo.GetLogger("juju.worker.caasoperator")

// caasOperator implements the capabilities of the caasoperator agent. It is not intended to
// implement the actual *behaviour* of the caasoperator agent; that responsibility is
// delegated to Mode values, which are expected to react to events and direct
// the caasoperator's responses to them.
type caasOperator struct {
	catacomb catacomb.Catacomb
	config   Config
	paths    context.Paths

	// Cache the last reported status information
	// so we don't make unnecessary api calls.
	setStatusMutex      sync.Mutex
	lastReportedStatus  status.Status
	lastReportedMessage string

	operationFactory  operation.Factory
	operationExecutor operation.Executor
}

// Config hold the configuration for a caasoperator worker.
type Config struct {
	// ModelUUID is the UUID of the model.
	ModelUUID string

	// ModelName is the name of the model.
	ModelName string

	// NewRunnerFactoryFunc returns a hook/cmd/action runner factory.
	NewRunnerFactoryFunc runner.NewRunnerFactoryFunc

	// Application holds the name of the application that
	// this CAAS operator manages.
	Application string

	// ApplicationConfigGetter is an interface used for
	// watching and getting the application's config settings.
	ApplicationConfigGetter ApplicationConfigGetter

	// CharmGetter is an interface used for getting the
	// application's charm URL and SHA256 hash.
	CharmGetter CharmGetter

	// Clock holds the clock to be used by the CAAS operator
	// for time-related operations.
	Clock clock.Clock

	// ContainerSpecSetter provides an interface for
	// setting the container spec for the application
	// or unit thereof.
	ContainerSpecSetter ContainerSpecSetter

	// DataDir holds the path to the Juju "data directory",
	// i.e. "/var/lib/juju" (by default). The CAAS operator
	// expects to find the jujud binary at <data-dir>/tools/jujud.
	DataDir string

	// Downloader is an interface used for downloading the
	// application charm.
	Downloader Downloader

	// StatusSetter is an interface used for setting the
	// application status.
	StatusSetter StatusSetter

	// APIAddressGetter is an interface for getting the
	// controller API addresses.
	APIAddressGetter APIAddressGetter

	// ProxySettingsGetter is an interface for getting the
	// model proxy settings.
	ProxySettingsGetter ProxySettingsGetter
}

func (config Config) Validate() error {
	if !names.IsValidApplication(config.Application) {
		return errors.NotValidf("application name %q", config.Application)
	}
	if config.ApplicationConfigGetter == nil {
		return errors.NotValidf("missing ApplicationConfigGetter")
	}
	if config.CharmGetter == nil {
		return errors.NotValidf("missing CharmGetter")
	}
	if config.Clock == nil {
		return errors.NotValidf("missing Clock")
	}
	if config.ContainerSpecSetter == nil {
		return errors.NotValidf("missing ContainerSpecSetter")
	}
	if config.DataDir == "" {
		return errors.NotValidf("missing DataDir")
	}
	if config.Downloader == nil {
		return errors.NotValidf("missing Downloader")
	}
	if config.StatusSetter == nil {
		return errors.NotValidf("missing StatusSetter")
	}
	if config.APIAddressGetter == nil {
		return errors.NotValidf("missing APIAddressGetter")
	}
	if config.ProxySettingsGetter == nil {
		return errors.NotValidf("missing ProxySettingsGetter")
	}
	return nil
}

// NewWorker creates a new worker which will install and operate a
// CaaS-based application, by executing hooks and operations in
// response to application state changes.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	op := &caasOperator{
		config: config,
		paths:  NewPaths(config.DataDir, names.NewApplicationTag(config.Application)),
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Site: &op.catacomb,
		Work: op.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return op, nil
}

func (op *caasOperator) loop() (err error) {
	if err := op.init(); err != nil {
		if err == jworker.ErrTerminateAgent {
			return err
		}
		return errors.Annotatef(err,
			"failed to initialize caasoperator for %q",
			op.config.Application,
		)
	}

	configGetter := op.config.ApplicationConfigGetter
	configWatcher, err := configGetter.WatchApplicationConfig(op.config.Application)
	if err != nil {
		return errors.Annotate(err, "starting an application config watcher")
	}
	op.catacomb.Add(configWatcher)

	for {
		select {
		case <-op.catacomb.Dying():
			return op.catacomb.ErrDying()
		case <-configWatcher.Changes():
			settings, err := configGetter.ApplicationConfig(op.config.Application)
			if err != nil {
				return errors.Annotate(err, "getting application config")
			}
			logger.Debugf("application config changed: %s", pretty.Sprint(settings))

			hookOp, err := op.operationFactory.NewRunHook(hook.Info{Kind: hooks.ConfigChanged})
			if err != nil {
				return errors.Trace(err)
			}
			if err := op.operationExecutor.Run(hookOp); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

func (op *caasOperator) init() (err error) {
	agentBinaryDir := op.paths.GetToolsDir()
	logger.Debugf("creating caas operator symlinks in %v", agentBinaryDir)
	if err := agenttools.EnsureSymlinks(
		agentBinaryDir,
		agentBinaryDir,
		commands.CommandNames(),
	); err != nil {
		return err
	}
	if err := op.ensureCharm(); err != nil {
		return errors.Trace(err)
	}

	contextFactory, err := context.NewContextFactory(context.FactoryConfig{
		ContextFactoryAPI: &contextFactoryAPIAdaptor{
			APIAddressGetter:    op.config.APIAddressGetter,
			ProxySettingsGetter: op.config.ProxySettingsGetter,
		},
		HookAPI: &hookAPIAdaptor{
			appName:                 op.config.Application,
			StatusSetter:            op.config.StatusSetter,
			ApplicationConfigGetter: op.config.ApplicationConfigGetter,
			ContainerSpecSetter:     op.config.ContainerSpecSetter,
		},
		ModelUUID:        op.config.ModelUUID,
		ModelName:        op.config.ModelName,
		ApplicationTag:   names.NewApplicationTag(op.config.Application),
		GetRelationInfos: nil, // TODO(caas)
		Paths:            op.paths,
		Clock:            op.config.Clock,
	})
	if err != nil {
		return err
	}
	runnerFactory, err := op.config.NewRunnerFactoryFunc(
		op.paths, contextFactory,
	)
	if err != nil {
		return errors.Trace(err)
	}

	op.operationFactory = operation.NewFactory(operation.FactoryParams{
		RunnerFactory: runnerFactory,
		Abort:         op.catacomb.Dying(),
		Callbacks:     &operationCallbacks{op},
	})

	operationExecutor, err := operation.NewExecutor()
	if err != nil {
		return errors.Trace(err)
	}
	op.operationExecutor = operationExecutor

	return nil
}

func (op *caasOperator) ensureCharm() error {
	charmDir := op.paths.GetCharmDir()
	if _, err := os.Stat(charmDir); !os.IsNotExist(err) {
		return errors.Trace(err)
	}
	curl, sha256, err := op.config.CharmGetter.Charm(op.config.Application)
	if err != nil {
		return errors.Trace(err)
	}
	if op.setStatus(status.Maintenance, "downloading charm (%s)", curl); err != nil {
		return errors.Trace(err)
	}
	if err := downloadCharm(
		op.config.Downloader,
		curl, sha256, charmDir,
		op.catacomb.Dying(),
	); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (op *caasOperator) setStatus(status status.Status, message string, args ...interface{}) error {
	err := op.config.StatusSetter.SetStatus(
		op.config.Application,
		status,
		fmt.Sprintf(message, args...),
		nil,
	)
	return errors.Annotate(err, "setting status")
}

// Kill is part of the worker.Worker interface.
func (op *caasOperator) Kill() {
	op.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (op *caasOperator) Wait() error {
	return op.catacomb.Wait()
}
