// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolsversionupdater

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/tools"
	coretools "github.com/juju/juju/internal/tools"
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
	// GetMissingAgentTargetVersions returns missing architectures for the
	// target agent version.
	GetMissingAgentTargetVersions(ctx context.Context) (semversion.Number, []string, error)
}

// WorkerConfig holds parameters for the tools version updater worker.
type WorkerConfig struct {
	DomainServices domainServices
	FindTools      ToolsFinderFunc
	Clock          clock.Clock
	Logger         logger.Logger
}

type updateWorker struct {
	tomb tomb.Tomb

	domainServices domainServices
	findTools      ToolsFinderFunc
	clock          clock.Clock
	logger         logger.Logger
}

// New returns a worker that updates the tools version information for agents.
func New(params WorkerConfig) worker.Worker {
	w := &updateWorker{
		domainServices: params.DomainServices,
		findTools:      params.FindTools,
		clock:          params.Clock,
		logger:         params.Logger,
	}

	w.tomb.Go(w.loop)

	return w
}

// Kill stops the worker.
func (w *updateWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait waits for the worker to stop.
func (w *updateWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *updateWorker) loop() error {
	ctx := w.tomb.Context(context.Background())

	targetVersion, arches, err := w.domainServices.agent.GetMissingAgentTargetVersions(ctx)
	if err != nil {
		return errors.Annotate(err, "getting agent version from service")
	}

	if len(arches) == 0 {
		return nil
	}

	ss := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
	modelCfg, err := w.domainServices.config.ModelConfig(ctx)
	if err != nil {
		return errors.Annotate(err, "cannot get model config")
	}

	env, err := w.domainServices.machine.GetBootstrapEnviron(ctx)
	if err != nil {
		return errors.Annotatef(err, "cannot get cloud provider")
	}

	preferredStreams := tools.PreferredStreams(&targetVersion, modelCfg.Development(), modelCfg.AgentStream())
	for _, arch := range arches {
		versions, err := w.findTools(ctx, ss, env, targetVersion.Major, targetVersion.Minor, preferredStreams, coretools.Filter{
			Number: targetVersion,
			Arch:   arch,
		})
		if err != nil {
			return errors.Annotatef(err, "cannot find available agent binaries")
		} else if versions.Len() == 0 {
			w.logger.Infof(ctx, "no agent binary found for version %s and architecture %s", targetVersion, arch)
			continue
		}

		// Ensure that we store the binaries that match the target version.
		// It doesn't have to match exactly the full version, just the
		// major/minor/patch, build can differ.
	}

	return nil
}
