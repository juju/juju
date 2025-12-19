// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolsversionchecker

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/semversion"
	modelagenterrors "github.com/juju/juju/domain/modelagent/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/tools"
	coretools "github.com/juju/juju/internal/tools"
	jworker "github.com/juju/juju/internal/worker"
)

// MachineService provides access to an environ for finding agent binaries.
type MachineService interface {
	// GetBootstrapEnviron returns the bootstrap environ.
	GetBootstrapEnviron(ctx context.Context) (environs.BootstrapEnviron, error)
}

// ModelConfigService provides access to the model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
}

// ModelAgentService provides access to the Juju agent version for the model.
type ModelAgentService interface {
	// GetModelTargetAgentVersion returns the target agent version for the
	// entire model.
	GetModelTargetAgentVersion(ctx context.Context) (semversion.Number, error)

	// UpdateLatestAgentVersion persists the latest available agent version.
	UpdateLatestAgentVersion(context.Context, semversion.Number) error
}

// WorkerConfig holds params for the version checker worker..
type WorkerConfig struct {
	checkInterval time.Duration
	logger        logger.Logger

	domainServices domainServices

	findTools ToolsFinderFunc
}

// New returns a worker that periodically wakes up to try to find out and
// record the latest version of the tools so the update possibility can be
// displayed to the users on status.
func New(params WorkerConfig) worker.Worker {
	w := &toolsVersionWorker{
		logger:         params.logger,
		domainServices: params.domainServices,
		findTools:      params.findTools,
	}

	return jworker.NewPeriodicWorker(w.performCheck, params.checkInterval, jworker.NewTimer)
}

type toolsVersionWorker struct {
	logger         logger.Logger
	domainServices domainServices

	findTools ToolsFinderFunc
}

func (w *toolsVersionWorker) performCheck(ctx context.Context) error {
	ver, err := w.checkToolsAvailability(ctx)
	if errors.Is(err, errors.NotFound) {
		// No newer tools, so exit silently.
		return nil
	} else if err != nil {
		return errors.Annotate(err, "cannot get latest version")
	} else if ver.IsZero() {
		w.logger.Debugf(ctx, "The lookup of agent binaries returned version Zero. This should only happen during bootstrap.")
		return nil
	}

	err = w.domainServices.agent.UpdateLatestAgentVersion(ctx, ver)
	if errors.Is(err, modelagenterrors.LatestVersionDowngradeNotSupported) {
		w.logger.Warningf(ctx, err.Error())
	} else if err != nil {
		return errors.Annotate(err, "updating latest agent version")
	}
	return nil
}

func (w *toolsVersionWorker) checkToolsAvailability(ctx context.Context) (semversion.Number, error) {
	currentVersion, err := w.domainServices.agent.GetModelTargetAgentVersion(ctx)
	if err != nil {
		return semversion.Zero, errors.Annotate(err, "getting agent version from service")
	}

	ss := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
	modelCfg, err := w.domainServices.config.ModelConfig(ctx)
	if err != nil {
		return semversion.Zero, errors.Annotate(err, "cannot get model config")
	}

	env, err := w.domainServices.machine.GetBootstrapEnviron(ctx)
	if err != nil {
		return semversion.Zero, errors.Annotatef(err, "cannot get cloud provider")
	}

	preferredStreams := tools.PreferredStreams(&currentVersion, modelCfg.Development(), modelCfg.AgentStream())
	vers, err := w.findTools(ctx, ss, env, currentVersion.Major, currentVersion.Minor, preferredStreams, coretools.Filter{})
	if err != nil {
		return semversion.Zero, errors.Annotatef(err, "cannot find available agent binaries")
	}
	// Newest also returns a list of the items in this list matching with the
	// newest version.
	newest, _ := vers.Newest()
	return newest, nil
}
