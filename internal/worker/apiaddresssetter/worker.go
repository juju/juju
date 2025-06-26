// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddresssetter

import (
	"context"
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
	"github.com/juju/juju/domain/controllernode"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/internal/errors"
	internalworker "github.com/juju/juju/internal/worker"
)

// ControllerConfigService is an interface for getting the controller config.
type ControllerConfigService interface {
	// ControllerConfig returns the config values for the controller.
	ControllerConfig(ctx context.Context) (controller.Config, error)

	// WatchControllerConfig returns a watcher that returns keys for any changes
	// to controller config.
	WatchControllerConfig(ctx context.Context) (watcher.StringsWatcher, error)
}

// ControllerNodeService provides access to controller nodes.
type ControllerNodeService interface {
	// WatchControllerNodes returns a watcher that observes changes to the
	// controller nodes.
	WatchControllerNodes(ctx context.Context) (watcher.NotifyWatcher, error)

	// GetControllerIDs returns the list of controller IDs from the controller node
	// records.
	GetControllerIDs(ctx context.Context) ([]string, error)

	// SetAPIAddresses sets the provided addresses associated with the provided
	// controller IDs.
	//
	// The following errors can be expected:
	// - [controllernodeerrors.NotFound] if the controller node does not exist.
	SetAPIAddresses(ctx context.Context, args controllernode.SetAPIAddressArgs) error
}

// ApplicationService is an interface for the application domain service.
type ApplicationService interface {
	// WatchUnitAddresses watches for changes to the addresses of the specified
	// unit.
	// This notifies on any changes to the unit addresses and it is up to the
	// caller to determine if the addresses they're interested in have changed.
	WatchUnitAddresses(ctx context.Context, unitName unit.Name) (watcher.NotifyWatcher, error)
}

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// GetUnitAddressesForAPI returns all addresses which can be used for
	// API addresses for the specified unit. local-machine scoped addresses
	// will not be returned.
	//
	// The following errors may be returned:
	// - [uniterrors.UnitNotFound] if the unit does not exist
	// - [network.NoAddressError] if the unit has no api address associated
	GetUnitAddressesForAPI(ctx context.Context, unitName unit.Name) (network.SpaceAddresses, error)
	// SpaceByName returns a space from state that matches the input name. If the
	// space is not found, an error is returned matching
	// [github.com/juju/juju/domain/network/errors.SpaceNotFound].
	SpaceByName(ctx context.Context, name network.SpaceName) (*network.SpaceInfo, error)
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
	ctx := w.catacomb.Context(context.Background())

	controllerNodeChanges, err := w.watchForControllerNodeChanges(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	configChanges, err := w.watchForConfigChanges(ctx)
	if err != nil {
		return errors.Capture(err)
	}

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

// watchForControllerChanges starts a watcher for changes to controller nodes.
// It returns a channel which will receive events if any of the watchers fires.
func (w *apiAddressSetterWorker) watchForControllerNodeChanges(ctx context.Context) (<-chan struct{}, error) {
	watcher, err := w.config.ControllerNodeService.WatchControllerNodes(ctx)
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
func (w *apiAddressSetterWorker) watchForConfigChanges(ctx context.Context) (<-chan []string, error) {
	watcher, err := w.config.ControllerConfigService.WatchControllerConfig(ctx)
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
	controllers := make(map[string]string)
	for _, controllerID := range controllerIDs {
		controllers[controllerID] = controllerID
	}

	w.config.Logger.Debugf(ctx, "controller nodes: %#v", controllerIDs)

	var changed bool
	// Stop controller tracker that no longer correspond to controller nodes.
	workerNames := w.runner.WorkerNames()
	for _, controllerID := range workerNames {
		if _, isRemoved := controllers[controllerID]; !isRemoved {
			if err := w.stopAndRemoveTracker(ctx, controllerID); err != nil {
				return false, errors.Capture(err)
			}
			changed = true
		}
	}

	// Start trackers for new nodes.
	for _, controllerID := range controllerIDs {
		w.config.Logger.Debugf(ctx, "found new controller %q", controllerID)
		changed = true

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
	if err != nil && !errors.Is(err, networkerrors.SpaceNotFound) {
		// If the space is not found, we can ignore it, since that case is
		// handled by the controller node domain in `SetAPIAddresses`.
		return errors.Capture(err)
	}

	args := controllernode.SetAPIAddressArgs{
		MgmtSpace:    mgmtSpace,
		APIAddresses: make(map[string]network.SpaceHostPorts),
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
		addrs, err := w.config.NetworkService.GetUnitAddressesForAPI(ctx, unitName)
		if err != nil {
			return errors.Capture(err)
		}
		hostPorts := network.SpaceAddressesWithPort(addrs, w.config.APIPort)
		if len(hostPorts) == 0 {
			w.config.Logger.Errorf(ctx, "no public address for controller %q", controllerID)
			continue
		}
		args.APIAddresses[controllerID] = hostPorts
	}
	if err := w.config.ControllerNodeService.SetAPIAddresses(ctx, args); err != nil {
		return errors.Capture(err)
	}
	return nil
}
