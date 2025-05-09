// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerprovisioner

import (
	"context"
	"fmt"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
)

// NewContainerSetupAndProvisioner returns a ContainerSetupAndProvisioner.
func NewContainerSetupAndProvisioner(ctx context.Context, cs *ContainerSetup, getContainerWatcherFunc GetContainerWatcherFunc) (worker.Worker, error) {
	containerWatcher, err := getContainerWatcherFunc(ctx)
	if err != nil {
		return nil, err
	}
	w := &ContainerSetupAndProvisioner{
		catacomb:         catacomb.Catacomb{},
		containerWatcher: containerWatcher,
		logger:           cs.logger,
		cs:               cs,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "container-provisioner",
		Site: &w.catacomb,
		Work: w.work,
		Init: []worker.Worker{w.containerWatcher},
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// ContainerSetupAndProvisioner is a worker that waits for a container of type
// defined in its config to be found for deployment. Then initializes
// the container system and starts a container provisioner of that type.
type ContainerSetupAndProvisioner struct {
	catacomb catacomb.Catacomb

	cs *ContainerSetup

	containerWatcher watcher.StringsWatcher
	logger           logger.Logger
	provisioner      Provisioner

	// For introspection Report
	mu sync.Mutex
}

func (w *ContainerSetupAndProvisioner) work() error {
	ctx, cancel := w.scopedContext()
	defer cancel()

	// Wait for a container of w.ContainerType type to be
	// found.
	if err := w.waitForContainers(); err != nil {
		return err
	}
	if err := w.checkDying(); err != nil {
		return err
	}

	// The container watcher is no longer needed
	if err := worker.Stop(w.containerWatcher); err != nil {
		return err
	}
	// For introspection Report
	w.mu.Lock()
	w.containerWatcher = nil
	w.mu.Unlock()

	// Set up w.ContainerType provisioning dependencies
	// on this machine.
	if err := w.cs.initialiseContainers(ctx, w.catacomb.Dying()); err != nil {
		return err
	}
	if err := w.checkDying(); err != nil {
		return err
	}

	// Configure and Add the w.ContainerType Provisioner
	provisioner, err := w.cs.initialiseContainerProvisioner(ctx)
	if err != nil {
		return err
	}
	if err := w.checkDying(); err != nil {
		return err
	}
	if err := w.catacomb.Add(provisioner); err != nil {
		return err
	}

	// For introspection Report
	w.mu.Lock()
	w.provisioner = provisioner
	w.mu.Unlock()

	// The container provisioner is now doing all the work, sit and wait
	// to be shutdown.
	select {
	case <-w.catacomb.Dying():
		return w.catacomb.ErrDying()
	}
}

// checkDying, returns an error if this worker's catacomb
// is dying. Needed as the work is not done in a single work.
func (w *ContainerSetupAndProvisioner) checkDying() error {
	select {
	case <-w.catacomb.Dying():
		return w.catacomb.ErrDying()
	default:
		return nil
	}
}

// waitForContainers waits for a container of the type
// configured in this worker to be deployed and returns.
func (w *ContainerSetupAndProvisioner) waitForContainers() error {
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case containerIds, ok := <-w.containerWatcher.Changes():
			if !ok {
				return errors.New("container watcher closed")
			}
			if len(containerIds) == 0 {
				continue
			}
			return nil
		}
	}
}

// Kill is part of the worker.Worker interface.
func (w *ContainerSetupAndProvisioner) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *ContainerSetupAndProvisioner) Wait() error {
	return w.catacomb.Wait()
}

// Report provides information for the engine report.
func (w *ContainerSetupAndProvisioner) Report() map[string]interface{} {
	w.mu.Lock()

	result := make(map[string]interface{}, 0)

	if w.containerWatcher != nil {
		watcherName := fmt.Sprintf("%s-container-watcher", string(w.cs.containerType))
		result[watcherName] = "waiting for containers"
	}
	if w.provisioner != nil {
		provisionerName := fmt.Sprintf("%s-provisioner", string(w.cs.containerType))
		result[provisionerName] = "setup and running"
	}

	w.mu.Unlock()
	return result
}

func (w *ContainerSetupAndProvisioner) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}
