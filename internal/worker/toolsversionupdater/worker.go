// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolsversionupdater

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/agentbinary/service"
)

// ModelAgentService provides access to the Juju agent version for the model.
type ModelAgentService interface {
	// GetMissingAgentTargetVersions returns missing architectures for the
	// target agent version.
	GetMissingAgentTargetVersions(ctx context.Context) (semversion.Number, []string, error)
}

// AgentBinaryService provides access to agent binaries stored in external
// stores.
type AgentBinaryService interface {
	// RetrieveExternalAgentBinary attempts to retrieve the specified agent binary
	// from one or more configured external stores. It validates the integrity of
	// the fetched binary via SHA256 and SHA384 comparison, then caches and persists
	// it into the local store for subsequent faster retrieval. If the binary cannot
	// be found in any external store or fails hash verification, an appropriate
	// error is returned. The returned reader provides the verified binary content
	// along with its size and SHA384 checksum.
	RetrieveExternalAgentBinary(ctx context.Context, ver coreagentbinary.Version) (*service.ComputedHashes, error)
}

// WorkerConfig holds parameters for the tools version updater worker.
type WorkerConfig struct {
	ModelAgentService  ModelAgentService
	AgentBinaryService AgentBinaryService
	Logger             logger.Logger
}

type updateWorker struct {
	tomb tomb.Tomb

	modelAgentService  ModelAgentService
	agentBinaryService AgentBinaryService

	logger logger.Logger
}

// New returns a worker that updates the tools version information for agents.
func New(params WorkerConfig) worker.Worker {
	w := &updateWorker{
		modelAgentService:  params.ModelAgentService,
		agentBinaryService: params.AgentBinaryService,

		logger: params.Logger,
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

	targetVersion, arches, err := w.modelAgentService.GetMissingAgentTargetVersions(ctx)
	if err != nil {
		return errors.Annotate(err, "getting agent version from service")
	}

	if len(arches) == 0 {
		return nil
	}

	for _, arch := range arches {
		w.logger.Debugf(ctx, "fetching agent binary for version %q and architecture %q", targetVersion, arch)

		_, err := w.agentBinaryService.RetrieveExternalAgentBinary(ctx, coreagentbinary.Version{
			Number: targetVersion,
			Arch:   arch,
		})
		if err != nil {
			return errors.Annotatef(err, "retrieving external agent binary for version %q and architecture %q", targetVersion, arch)
		}
	}

	return nil
}
