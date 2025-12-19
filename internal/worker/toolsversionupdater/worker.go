// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolsversionupdater

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
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

// WorkerConfig holds parameters for the tools version updater worker.
type WorkerConfig struct {
	DomainServices domainServices
	Clock          clock.Clock
	Logger         logger.Logger
}

type updateWorker struct {
	tomb tomb.Tomb

	domainServices domainServices
	clock          clock.Clock
	logger         logger.Logger
}

// New returns a worker that updates the tools version information for agents.
func New(params WorkerConfig) worker.Worker {
	w := &updateWorker{}

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
