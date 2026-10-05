// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddresssetter

import (
	"context"
	"maps"
	"slices"
	"strconv"

	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"

	"github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/controllernode"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/errors"
)

// ControllerConfigService is an interface for getting the controller config.
type ControllerConfigService interface {
	// GetManagementSpaceAndAPIPort returns the management space and API port.
	GetManagementSpaceAndAPIPort(ctx context.Context) (network.SpaceName, int, error)

	// WatchControllerConfig returns a watcher that returns keys for any changes
	// to controller config.
	WatchControllerConfig(ctx context.Context) (watcher.StringsWatcher, error)
}

// ControllerNodeService provides access to controller nodes.
type ControllerNodeService interface {
	// WatchControllerNodes returns a watcher that observes changes to the
	// controller nodes.
	WatchControllerNodes(ctx context.Context) (watcher.NotifyWatcher, error)

	// GetControllerIDs returns the IDs of alive or dying controller nodes.
	GetControllerIDs(ctx context.Context) ([]string, error)

	// SetAPIAddresses publishes the selected API addresses.
	SetAPIAddresses(ctx context.Context, args controllernode.SetAPIAddressArgs) error
}

// NetworkService provides controller network address selections.
type NetworkService interface {
	// GetControllerClientAddresses returns addresses selected for ordinary
	// client discovery. IAAS addresses are associated with controller units;
	// CAAS Service addresses are returned as shared endpoints.
	GetControllerClientAddresses(ctx context.Context, names []unit.Name) (domainnetwork.ControllerAddressSelection, error)

	// GetControllerAgentAddresses returns addresses selected for ordinary
	// agent discovery. IAAS selection honours the management space, while CAAS
	// returns shared Service endpoints and ignores management-space policy.
	GetControllerAgentAddresses(ctx context.Context, names []unit.Name, managementSpace network.SpaceName) (domainnetwork.ControllerAddressSelection, error)

	// GetControllerPeerAddresses returns addresses grouped by controller unit.
	// Peer addresses are never shared. IAAS selection honours the management
	// space; CAAS selection uses controller pod addresses.
	GetControllerPeerAddresses(ctx context.Context, names []unit.Name, managementSpace network.SpaceName) (domainnetwork.ControllerAddressSelection, error)

	// WatchControllerNetwork watches all controller-model network facts that
	// can change client, agent or peer selections. Its initial event must be
	// consumed before reading addresses.
	WatchControllerNetwork(ctx context.Context) (watcher.NotifyWatcher, error)
}

// apiAddressSetterWorker publishes controller API addresses from authoritative
// controller, configuration and network sources.
type apiAddressSetterWorker struct {
	catacomb catacomb.Catacomb
	config   Config
}

// Config holds the configuration for the api address setter worker.
type Config struct {
	ControllerConfigService ControllerConfigService
	ControllerNodeService   ControllerNodeService
	NetworkService          NetworkService
	Logger                  logger.Logger
}

// Validate validates the worker configuration.
func (config Config) Validate() error {
	if config.ControllerConfigService == nil {
		return errors.New("nil ControllerConfigService not valid").Add(coreerrors.NotValid)
	}
	if config.ControllerNodeService == nil {
		return errors.New("nil ControllerNodeService not valid").Add(coreerrors.NotValid)
	}
	if config.NetworkService == nil {
		return errors.New("nil NetworkService not valid").Add(coreerrors.NotValid)
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

	w := &apiAddressSetterWorker{config: config}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "apiaddresssetter",
		Site: &w.catacomb,
		Work: w.loop,
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

	nodeWatcher, err := w.config.ControllerNodeService.WatchControllerNodes(ctx)
	if err != nil {
		return errors.Errorf("watching controller nodes: %w", err)
	}
	if err := w.catacomb.Add(nodeWatcher); err != nil {
		return errors.Capture(err)
	}
	configWatcher, err := w.config.ControllerConfigService.WatchControllerConfig(ctx)
	if err != nil {
		return errors.Errorf("watching controller config: %w", err)
	}
	if err := w.catacomb.Add(configWatcher); err != nil {
		return errors.Capture(err)
	}
	networkWatcher, err := w.config.NetworkService.WatchControllerNetwork(ctx)
	if err != nil {
		return errors.Errorf("watching controller network: %w", err)
	}
	if err := w.catacomb.Add(networkWatcher); err != nil {
		return errors.Capture(err)
	}

	if err := w.consumeInitialNotify(nodeWatcher.Changes(), "controller node"); err != nil {
		return err
	}
	if err := w.consumeInitialStrings(configWatcher.Changes(), "controller config"); err != nil {
		return err
	}
	if err := w.consumeInitialNotify(networkWatcher.Changes(), "controller network"); err != nil {
		return err
	}

	if err := w.reconcile(ctx); err != nil {
		return errors.Capture(err)
	}
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case _, ok := <-nodeWatcher.Changes():
			if !ok {
				return errors.New("controller node watcher closed")
			}
		case _, ok := <-configWatcher.Changes():
			if !ok {
				return errors.New("controller config watcher closed")
			}
		case _, ok := <-networkWatcher.Changes():
			if !ok {
				return errors.New("controller network watcher closed")
			}
		}

		if err := w.reconcile(ctx); err != nil {
			return errors.Capture(err)
		}
	}
}

func (w *apiAddressSetterWorker) consumeInitialNotify(ch <-chan struct{}, name string) error {
	select {
	case <-w.catacomb.Dying():
		return w.catacomb.ErrDying()
	case _, ok := <-ch:
		if !ok {
			return errors.Errorf("%s watcher closed", name)
		}
		return nil
	}
}

func (w *apiAddressSetterWorker) consumeInitialStrings(ch <-chan []string, name string) error {
	select {
	case <-w.catacomb.Dying():
		return w.catacomb.ErrDying()
	case _, ok := <-ch:
		if !ok {
			return errors.Errorf("%s watcher closed", name)
		}
		return nil
	}
}

func (w *apiAddressSetterWorker) reconcile(ctx context.Context) error {
	controllerIDs, err := w.config.ControllerNodeService.GetControllerIDs(ctx)
	if errors.Is(err, controllernodeerrors.EmptyControllerIDs) {
		// No controller nodes are alive or dying, so clear the published
		// addresses. Also, it's possible to have shared addresses, and we must
		// be able to set those addresses without any controller nodes.
		controllerIDs = nil
	} else if err != nil {
		return errors.Errorf("getting controller IDs: %w", err)
	}

	controllerUnitNames := make(map[string]unit.Name, len(controllerIDs))

	// Convert controller IDs to unit names, which are used to look up network
	// selections.
	for _, controllerID := range controllerIDs {
		unitNumber, err := strconv.Atoi(controllerID)
		if err != nil {
			return errors.Errorf("invalid controller ID %q: %w", controllerID, err)
		}
		name, err := unit.NewNameFromParts(application.ControllerApplicationName, unitNumber)
		if err != nil {
			return errors.Errorf("creating unit name for controller %q: %w", controllerID, err)
		}
		controllerUnitNames[controllerID] = name
	}

	// Collect the unit names in a slice for use with the network service.
	names := slices.Collect(maps.Values(controllerUnitNames))

	managementSpace, apiPort, err := w.config.ControllerConfigService.GetManagementSpaceAndAPIPort(ctx)
	if err != nil {
		return errors.Errorf("getting management space and API port: %w", err)
	}

	// Get the client addresses, which are used for ordinary client discovery.
	// The management space is used for IAAS selection, but ignored for CAAS
	// selection.
	clients, err := w.config.NetworkService.GetControllerClientAddresses(ctx, names)
	if err != nil {
		return errors.Errorf("getting controller client addresses: %w", err)
	}

	// Get the agent addresses, which are used for ordinary agent discovery. The
	// management space is used for IAAS selection, but ignored for CAAS
	// selection.
	agents, err := w.config.NetworkService.GetControllerAgentAddresses(ctx, names, managementSpace)
	if err != nil {
		return errors.Errorf("getting controller agent addresses: %w", err)
	}

	// Get the peer addresses, which are used for controller-to-controller
	// communication. The management space is used for IAAS selection, but
	// ignored for CAAS selection.
	peers, err := w.config.NetworkService.GetControllerPeerAddresses(ctx, names, managementSpace)
	if err != nil {
		return errors.Errorf("getting controller peer addresses: %w", err)
	}

	// Build the API address set for each controller node, which includes the
	// client, agent and peer addresses. The client and agent addresses are
	// shared for CAAS selection, but not for IAAS selection.
	addresses := make(map[string]controllernode.APIAddressSet, len(controllerIDs))
	for _, controllerID := range controllerIDs {
		controllerUnitName, ok := controllerUnitNames[controllerID]
		if !ok {
			return errors.Errorf("controller ID %q has no unit name", controllerID)
		}
		name := controllerUnitName
		addresses[controllerID] = controllernode.APIAddressSet{
			Clients: slices.Clone(clients.ByUnit[name]),
			Agents:  slices.Clone(agents.ByUnit[name]),
			Peers:   slices.Clone(peers.ByUnit[name]),
		}
	}

	if err := w.config.ControllerNodeService.SetAPIAddresses(ctx, controllernode.SetAPIAddressArgs{
		APIPort:   apiPort,
		Addresses: addresses,
		SharedAddresses: controllernode.SharedAPIAddressSet{
			Clients: slices.Clone(clients.Shared),
			Agents:  slices.Clone(agents.Shared),
		},
	}); err != nil {
		return errors.Errorf("publishing API addresses: %w", err)
	}
	return nil
}
