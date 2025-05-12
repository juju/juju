// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddresssetter

import (
	"context"
	"slices"
	"strconv"
	"time"

	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/errors"
	internalworker "github.com/juju/juju/internal/worker"
)

// ControllerConfigService is an interface for getting the controller config.
type ControllerConfigService interface {
	// ControllerConfig returns the config values for the controller.
	ControllerConfig(ctx context.Context) (controller.Config, error)

	// WatchControllerConfig returns a watcher that returns keys for any changes
	// to controller config.
	WatchControllerConfig() (watcher.StringsWatcher, error)
}

// ControllerNodeService provides access to controller nodes.
type ControllerNodeService interface {
	// WatchControllerNodes returns a watcher that observes changes to the
	// controller nodes.
	WatchControllerNodes() (watcher.NotifyWatcher, error)

	// GetControllerIDs returns the list of controller IDs from the controller node
	// records.
	GetControllerIDs(ctx context.Context) ([]string, error)

	// SetAPIAddresses sets the provided addresses associated with the provided
	// controller ID.
	//
	// The following errors can be expected:
	// - [controllernodeerrors.NotFound] if the controller node does not exist.
	SetAPIAddresses(ctx context.Context, controllerID string, addrs network.SpaceHostPorts, mgmtSpace network.SpaceInfo) error
}

// ApplicationService is an interface for the application domain service.
type ApplicationService interface {
	// WatchNetNodeAddress watches for changes to the specified net nodes
	// addresses.
	// This notifies on any changes to the net nodes addresses. It is up to the
	// caller to determine if the addresses they're interested in has changed.
	WatchNetNodeAddress(ctx context.Context, netNodeUUIDs ...string) (watcher.NotifyWatcher, error)

	// GetUnitNetNodes returns the net node UUIDs associated with the specified
	// unit. The net nodes are selected in the same way as in GetUnitAddresses, i.e.
	// the union of the net nodes of the cloud service (if any) and the net node
	// of the unit.
	//
	// The following errors may be returned:
	// - [uniterrors.UnitNotFound] if the unit does not exist
	GetUnitNetNodes(ctx context.Context, unitName unit.Name) ([]string, error)

	// GetUnitPublicAddresses returns all public addresses for the specified unit.
	//
	// The following errors may be returned:
	// - [uniterrors.UnitNotFound] if the unit does not exist
	// - [network.NoAddressError] if the unit has no public address associated
	GetUnitPublicAddresses(ctx context.Context, unitName unit.Name) (network.SpaceAddresses, error)
}

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// SpaceByName returns a space from state that matches the input name. If the
	// space is not found, an error is returned matching
	// [github.com/juju/juju/domain/network/errors.SpaceNotFound].
	SpaceByName(ctx context.Context, name string) (*network.SpaceInfo, error)
}

// apiAddressSetterWorker is a worker which sets the API addresses for the
// controller, watching for changes both in the controller node's ip addresses
// and the controller config (the juju-mgmt-space key) to filter the addresses
// based on the management space.
type apiAddressSetterWorker struct {
	catacomb catacomb.Catacomb

	config Config

	// controllerNodeChanges is a channel that is used to signal back to the
	// main worker that the controller node addresses have changed.
	controllerNodeChanges chan struct{}
	runner                *worker.Runner
}

// Config holds the configuration for the api address setter worker.
type Config struct {
	ControllerConfigService ControllerConfigService
	ApplicationService      ApplicationService
	ControllerNodeService   ControllerNodeService
	NetworkService          NetworkService
	APIPort                 int
	ControllerAPIPort       int
	Logger                  logger.Logger
}

// Validate validates the worker configuration.
func (config Config) Validate() error {
	if config.ControllerConfigService == nil {
		return errors.New("nil ControllerConfigService not valid").Add(coreerrors.NotValid)
	}
	if config.ApplicationService == nil {
		return errors.New("nil ApplicationService not valid").Add(coreerrors.NotValid)
	}
	if config.ControllerNodeService == nil {
		return errors.New("nil ControllerNodeService not valid").Add(coreerrors.NotValid)
	}
	if config.NetworkService == nil {
		return errors.New("nil NetworkService not valid").Add(coreerrors.NotValid)
	}
	if config.APIPort <= 0 {
		return errors.New("non-positive APIPort not valid").Add(coreerrors.NotValid)
	}
	if config.ControllerAPIPort <= 0 {
		return errors.New("non-positive ControllerAPIPort not valid").Add(coreerrors.NotValid)
	}
	if config.Logger == nil {
		return errors.New("nil Logger not valid").Add(coreerrors.NotValid)
	}
	return nil
}

// New returns a new worker that maintains the api addresses for the controller
// nodes.
func New(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name:    "apiaddresssetter",
		IsFatal: func(error) bool { return false },
		Logger:  internalworker.WrapLogger(config.Logger),
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	w := &apiAddressSetterWorker{
		config:                config,
		controllerNodeChanges: make(chan struct{}),
		runner:                runner,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "apiaddresssetter",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{w.runner},
	}); err != nil {
		return nil, errors.Capture(err)
	}
	return w, nil
}

// Kill is part of the worker.Worker interface.
func (w *apiAddressSetterWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *apiAddressSetterWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *apiAddressSetterWorker) loop() error {
	controllerNodeChanges, err := w.watchForControllerNodeChanges()
	if err != nil {
		return errors.Capture(err)
	}

	configChanges, err := w.watchForConfigChanges()
	if err != nil {
		return errors.Capture(err)
	}

	ctx, cancel := w.scopedContext()
	defer cancel()

	for {
		w.config.Logger.Tracef(ctx, "waiting for controller nodes or addresses changes")
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case <-controllerNodeChanges:
			// A controller was added or removed.
			w.config.Logger.Tracef(ctx, "<-controllerNodeChanges")
			changed, err := w.updateControllerNodes(ctx)
			if err != nil {
				return errors.Capture(err)
			}
			if !changed {
				continue
			}
			w.config.Logger.Tracef(ctx, "controller node added or removed")

		case <-w.controllerNodeChanges:
			// One of the controller nodes changed.
			w.config.Logger.Tracef(ctx, "<-w.controllerNodeChanges")

		case <-configChanges:
			// Controller config has changed.
			w.config.Logger.Tracef(ctx, "<-w.configChanges")

			if len(w.runner.WorkerNames()) == 0 {
				w.config.Logger.Errorf(ctx, "no controller information, ignoring config change")
				continue
			}
		}

		if err := w.updateAPIAddresses(ctx); err != nil {
			w.config.Logger.Errorf(ctx, "cannot update api addresses: %v", err)
		}
	}
}

func (w *apiAddressSetterWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}

// watchForControllerChanges starts a watcher for changes to controller nodes.
// It returns a channel which will receive events if any of the watchers fires.
func (w *apiAddressSetterWorker) watchForControllerNodeChanges() (<-chan struct{}, error) {
	watcher, err := w.config.ControllerNodeService.WatchControllerNodes()
	if err != nil {
		return nil, errors.Capture(err)
	}
	if err := w.catacomb.Add(watcher); err != nil {
		return nil, errors.Capture(err)
	}

	return watcher.Changes(), nil
}

// watchForConfigChanges starts a watcher for changes to controller config.
// It returns a channel which will receive events if the watcher fires.
func (w *apiAddressSetterWorker) watchForConfigChanges() (<-chan []string, error) {
	watcher, err := w.config.ControllerConfigService.WatchControllerConfig()
	if err != nil {
		return nil, errors.Capture(err)
	}
	if err := w.catacomb.Add(watcher); err != nil {
		return nil, errors.Capture(err)
	}

	return watcher.Changes(), nil
}

// updateControllerNodes updates the current list of tracked controller nodes,
// as well as starting and stopping trackers for them as they are added and
// removed.
func (w *apiAddressSetterWorker) updateControllerNodes(ctx context.Context) (bool, error) {
	controllerIDs, err := w.config.ControllerNodeService.GetControllerIDs(ctx)
	if err != nil {
		return false, errors.Errorf("cannot get controller IDs: %w", err)
	}

	w.config.Logger.Debugf(ctx, "controller nodes: %#v", controllerIDs)

	var changed bool

	// Stop controller tracker that no longer correspond to controller nodes.
	workerNames := w.runner.WorkerNames()
	for _, controllerID := range workerNames {
		if !slices.Contains(controllerIDs, controllerID) {
			if err := w.stopAndRemoveTracker(ctx, controllerID); err != nil {
				return false, errors.Capture(err)
			}
			changed = true
		}
	}

	// Start trackers for new nodes.
	for _, controllerID := range controllerIDs {
		w.config.Logger.Debugf(ctx, "found new controller %q", controllerID)

		if err := w.runner.StartWorker(ctx, controllerID, func(ctx context.Context) (worker.Worker, error) {
			id, err := strconv.Atoi(controllerID)
			if err != nil {
				return nil, errors.Errorf("invalid controller ID %q: %w", controllerID, err)
			}
			unitName, err := unit.NewNameFromParts(application.ControllerApplicationName, id)
			if err != nil {
				return nil, errors.Errorf("invalid unit name for controller %q: %w", controllerID, err)
			}
			tracker, err := newControllerTracker(unitName, w.config.ApplicationService, w.controllerNodeChanges, w.config.Logger.Child("controllertracker"))
			if err != nil {
				return nil, errors.Capture(err)
			}
			return tracker, nil
		}); err != nil && !errors.Is(err, coreerrors.AlreadyExists) {
			return false, errors.Errorf("failed to start tracker for controller node %q: %w", controllerID, err)
		}
		changed = true
	}

	return changed, nil
}

func (w *apiAddressSetterWorker) stopAndRemoveTracker(ctx context.Context, controllerID string) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	// Stop tracker since it's no longer required.
	w.config.Logger.Debugf(ctx, "stopping tracker for controller node %q", controllerID)
	if err := w.runner.StopAndRemoveWorker(controllerID, ctx.Done()); errors.Is(err, context.DeadlineExceeded) {
		return errors.Errorf("failed to stop tracker for controller node %q: timed out", controllerID)
	} else if err != nil {
		return errors.Errorf("failed to stop tracker for controller node %q: %w", controllerID, err)
	}

	return nil
}

// updateAPIAddresses updates the API addresses for each tracked controller.
func (w *apiAddressSetterWorker) updateAPIAddresses(ctx context.Context) error {
	cfg, err := w.config.ControllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	mgmtSpace, err := w.config.NetworkService.SpaceByName(ctx, cfg.JujuManagementSpace())
	if err != nil {
		return errors.Capture(err)
	}

	for _, controllerID := range w.runner.WorkerNames() {
		unitNumber, err := strconv.Atoi(controllerID)
		if err != nil {
			return errors.Capture(err)
		}
		unitName, err := unit.NewNameFromParts(application.ControllerApplicationName, unitNumber)
		if err != nil {
			return errors.Capture(err)
		}
		addrs, err := w.config.ApplicationService.GetUnitPublicAddresses(ctx, unitName)
		if err != nil {
			return errors.Capture(err)
		}
		hostPorts := network.SpaceAddressesWithPort(addrs, w.config.APIPort)
		if len(hostPorts) == 0 {
			w.config.Logger.Errorf(ctx, "no public address for controller %q", controllerID)
			continue
		}

		if err := w.config.ControllerNodeService.SetAPIAddresses(ctx, controllerID, hostPorts, *mgmtSpace); err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}
