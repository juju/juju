// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"net"
	"sort"
	"strconv"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/controllernode"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// State describes retrieval and persistence
// methods for controller node concerns.
type State interface {
	// AddDqliteNodeID ensures a controller node exists for the supplied ID.
	AddDqliteNodeID(ctx context.Context, controllerID string) error

	// AddDqliteNode adds the Dqlite node ID and bind address for the input
	// controller ID. If the controller ID already exists, it updates the
	// Dqlite node ID and bind address.
	AddDqliteNode(ctx context.Context, controllerID string, nodeID uint64, addr string) error

	// SelectDatabaseNamespace returns the database namespace for the supplied
	// namespace.
	SelectDatabaseNamespace(context.Context, string) (string, error)

	// SetRunningAgentBinaryVersion sets the agent version for the supplied
	// controllerID. Version represents the version of the controller node's
	// agent binary.
	SetRunningAgentBinaryVersion(context.Context, string, coreagentbinary.Version) error

	// NamespaceForWatchControllerNodes returns the namespace for watching
	// controller nodes.
	NamespaceForWatchControllerNodes() string

	// NamespaceForWatchControllerAgentAddresses returns the namespace for
	// watching controller agent addresses.
	NamespaceForWatchControllerAgentAddresses() string

	// NamespaceForWatchControllerClientAddresses returns the namespace for
	// watching controller client addresses.
	NamespaceForWatchControllerClientAddresses() string

	// NamespaceForWatchControllerPeerAddresses returns the namespace for
	// watching controller peer addresses.
	NamespaceForWatchControllerPeerAddresses() string

	// SetAPIAddresses atomically replaces all client, agent and peer address
	// projections.
	//
	// The following errors can be expected:
	// - [controllernodeerrors.StaleControllerMembership] if the projection keys
	// do not exactly match the alive or dying controller nodes.
	SetAPIAddresses(ctx context.Context, addresses controllernode.APIAddressProjections) error

	// GetControllerIDs returns the list of controller IDs from the controller
	// node records.
	GetControllerIDs(ctx context.Context) ([]string, error)

	// GetAPIAddressesForAgents returns all APIAddresses available
	// for agents, divided by controller node. Shared endpoints use the empty
	// controller ID.
	GetAPIAddressesForAgents(ctx context.Context) (map[string]controllernode.APIAddresses, error)

	// GetAPIAddressesForClients returns all APIAddresses available
	// for clients, divided by controller node. Shared endpoints use the empty
	// controller ID.
	GetAPIAddressesForClients(ctx context.Context) (map[string]controllernode.APIAddresses, error)

	// GetAPIAddressesForPeers returns peer API addresses divided by controller
	// node. Peer addresses always have a controller identity.
	GetAPIAddressesForPeers(ctx context.Context) (map[string]controllernode.APIAddresses, error)

	// GetAllCloudLocalAPIAddresses returns client API addresses with cloud-local
	// scope, including shared endpoints. Addresses include port numbers.
	GetAllCloudLocalAPIAddresses(ctx context.Context) ([]string, error)
}

// Service provides the API for working with controller nodes.
type Service struct {
	st     State
	logger logger.Logger
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State, logger logger.Logger) *Service {
	return &Service{
		st:     st,
		logger: logger,
	}
}

// AddDqliteNodeID ensures a controller node exists for the supplied ID.
func (s *Service) AddDqliteNodeID(ctx context.Context, controllerID string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if controllerID == "" {
		return errors.Errorf("controller ID is empty: %w", coreerrors.NotValid)
	}
	if err := s.st.AddDqliteNodeID(ctx, controllerID); err != nil {
		return errors.Errorf("adding controller node %q: %w", controllerID, err)
	}
	return nil
}

// AddDqliteNode adds the Dqlite node ID and bind address for the input
// controller ID. If the controller ID already exists, it updates the
// Dqlite node ID and bind address.
func (s *Service) AddDqliteNode(ctx context.Context, controllerID string, nodeID uint64, addr string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.st.AddDqliteNode(ctx, controllerID, nodeID, addr); err != nil {
		return errors.Errorf("adding Dqlite node details for %q: %w", controllerID, err)
	}
	return nil
}

// IsKnownDatabaseNamespace reports if the namespace is known to the controller.
// If the namespace is not valid an error satisfying [errors.NotValid] is
// returned.
func (s *Service) IsKnownDatabaseNamespace(ctx context.Context, namespace string) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if namespace == "" {
		return false, errors.Errorf("namespace %q is %w, cannot be empty", namespace, coreerrors.NotValid)
	}

	ns, err := s.st.SelectDatabaseNamespace(ctx, namespace)
	if err != nil && !errors.Is(err, controllernodeerrors.NotFound) {
		return false, errors.Errorf("determining namespace existence: %w", err)
	}

	return ns == namespace, nil
}

// SetControllerNodeReportedAgentVersion sets the agent version for the supplied
// controllerID. Version represents the version of the controller node's agent
// binary.
//
// The following errors are possible:
// - [coreerrors.NotValid] if the version is not valid.
// - [coreerrors.NotSupported] if the architecture is not supported.
// - [controllernodeerrors.NotFound] if the controller node does not exist.
func (s *Service) SetControllerNodeReportedAgentVersion(ctx context.Context, controllerID string, version coreagentbinary.Version) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := version.Validate(); err != nil {
		return errors.Errorf("agent version %+v is not valid: %w", version, err)
	}

	if err := s.st.SetRunningAgentBinaryVersion(ctx, controllerID, version); err != nil {
		return errors.Errorf(
			"setting controller node %q agent version (%s): %w",
			controllerID,
			version.Number.String(),
			err,
		)
	}

	return nil
}

// SetAPIAddresses encodes and publishes addresses already selected for each
// routing audience.
//
// The following errors can be expected:
// - [coreerrors.NotValid] if the API port is invalid or a named address set has
// no controller ID.
// - [controllernodeerrors.ControllerAddressNotValid] if an address is empty.
// - [controllernodeerrors.StaleControllerMembership] if controller membership
// changed after address selection.
func (s *Service) SetAPIAddresses(ctx context.Context, args controllernode.SetAPIAddressArgs) error {
	if args.APIPort <= 0 {
		return errors.Errorf("non-positive API port: %w", coreerrors.NotValid)
	}

	projections := make(controllernode.APIAddressProjections, len(args.Addresses)+1)
	for controllerID, selected := range args.Addresses {
		if controllerID == "" {
			return errors.Errorf("controller ID is empty: %w", coreerrors.NotValid)
		}
		clients, err := encodeAPIAddresses(selected.Clients, args.APIPort)
		if err != nil {
			return errors.Errorf("encoding client addresses for controller %q: %w", controllerID, err)
		}
		agents, err := encodeAPIAddresses(selected.Agents, args.APIPort)
		if err != nil {
			return errors.Errorf("encoding agent addresses for controller %q: %w", controllerID, err)
		}
		peers, err := encodeAPIAddresses(selected.Peers, args.APIPort)
		if err != nil {
			return errors.Errorf("encoding peer addresses for controller %q: %w", controllerID, err)
		}
		projections[controllerID] = controllernode.APIAddressProjection{
			Clients: clients,
			Agents:  agents,
			Peers:   peers,
		}
	}
	if len(args.SharedAddresses.Clients) != 0 || len(args.SharedAddresses.Agents) != 0 {
		clients, err := encodeAPIAddresses(args.SharedAddresses.Clients, args.APIPort)
		if err != nil {
			return errors.Errorf("encoding shared client addresses: %w", err)
		}
		agents, err := encodeAPIAddresses(args.SharedAddresses.Agents, args.APIPort)
		if err != nil {
			return errors.Errorf("encoding shared agent addresses: %w", err)
		}
		projections[""] = controllernode.APIAddressProjection{
			Clients: clients,
			Agents:  agents,
		}
	}
	return s.st.SetAPIAddresses(ctx, projections)
}

func encodeAPIAddresses(addrs network.SpaceAddresses, apiPort int) (controllernode.APIAddresses, error) {
	if len(addrs) == 0 {
		return nil, nil
	}

	addresses := make(controllernode.APIAddresses, 0, len(addrs))
	seen := make(map[string]struct{}, len(addrs))
	for _, spaceAddress := range addrs {
		if spaceAddress.Value == "" {
			return nil, errors.Errorf("address is empty: %w", controllernodeerrors.ControllerAddressNotValid)
		}
		address := net.JoinHostPort(spaceAddress.Value, strconv.Itoa(apiPort))
		if _, ok := seen[address]; ok {
			continue
		}
		seen[address] = struct{}{}
		addressUUID, err := uuid.NewUUID()
		if err != nil {
			return nil, errors.Capture(err)
		}
		addresses = append(addresses, controllernode.APIAddress{
			UUID:     addressUUID.String(),
			Address:  address,
			Scope:    spaceAddress.Scope,
			Priority: len(addresses),
		})
	}
	return addresses, nil
}

// GetControllerIDs returns the list of controller IDs from the controller node
// records.
func (s *Service) GetControllerIDs(ctx context.Context) ([]string, error) {
	res, err := s.st.GetControllerIDs(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return res, nil
}

// GetAPIHostPortsForAgents returns API HostPorts that are available for
// agents. HostPorts are grouped by controller node, though each specific
// controller is not identified. Shared endpoints form a separate group.
func (s *Service) GetAPIHostPortsForAgents(ctx context.Context) ([]network.HostPorts, error) {
	agentAddrs, err := s.st.GetAPIAddressesForAgents(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return transformToOrderedHostPorts(agentAddrs)
}

// GetAPIHostPortsForClients returns API HostPorts that are available for
// clients. HostPorts are grouped by controller node, though each specific
// controller is not identified. Shared endpoints form a separate group.
func (s *Service) GetAPIHostPortsForClients(ctx context.Context) ([]network.HostPorts, error) {
	clientAddrs, err := s.st.GetAPIAddressesForClients(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return transformToOrderedHostPorts(clientAddrs)
}

func transformToOrderedHostPorts(input map[string]controllernode.APIAddresses) ([]network.HostPorts, error) {
	ids := mapKeyOrder(input)

	var result []network.HostPorts
	for _, id := range ids {
		addr := input[id]
		if len(addr) == 0 {
			continue
		}

		address, err := addr.ToHostPorts()
		if err != nil {
			return nil, errors.Capture(err)
		}
		result = append(result, address)
	}
	return result, nil
}

// GetAPIAddressesByControllerIDForAgents returns a map of controller IDs to
// their API addresses that are available for agents. The map is keyed by
// controller ID, and the values are slices of strings representing the API
// addresses for each controller node. Shared endpoints are excluded.
func (s *Service) GetAPIAddressesByControllerIDForAgents(ctx context.Context) (map[string][]string, error) {
	addresses, err := s.st.GetAPIAddressesForAgents(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := make(map[string][]string, len(addresses))
	for controllerID, addrs := range addresses {
		if controllerID == "" {
			continue
		}
		result[controllerID] = addrs.Values()
	}

	return result, nil
}

// GetAPIHostPortsForControllerIDForAgents returns the agent-reachable API
// host ports for the given controller node ID in published priority order.
//
// The following errors may be returned:
// - [controllernodeerrors.EmptyAPIAddresses] when no API addresses are found
// for the given controller node ID.
func (s *Service) GetAPIHostPortsForControllerIDForAgents(ctx context.Context, controllerID string) (network.HostPorts, error) {
	addresses, err := s.st.GetAPIAddressesForAgents(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	addrs, ok := addresses[controllerID]
	if controllerID == "" || !ok || len(addrs) == 0 {
		return nil, errors.Errorf(
			"no API addresses found for controller node %q", controllerID,
		).Add(controllernodeerrors.EmptyAPIAddresses)
	}

	return addrs.ToHostPorts()
}

// GetAllAPIAddressesForAgents returns all agent addresses, including shared
// endpoints, in published priority order within each endpoint group.
func (s *Service) GetAllAPIAddressesForAgents(ctx context.Context) ([]string, error) {
	agentAddrs, err := s.st.GetAPIAddressesForAgents(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	ids := mapKeyOrder(agentAddrs)

	var orderedAddrs []string
	for _, id := range ids {
		addrs := agentAddrs[id]
		if len(addrs) == 0 {
			continue
		}
		orderedAddrs = append(orderedAddrs, addrs.Values()...)
	}
	return orderedAddrs, nil
}

// GetAllNoProxyAPIAddressesForAgents returns a sorted, comma separated string
// of all agent API addresses, including shared endpoints, suitable for no proxy
// settings.
func (s *Service) GetAllNoProxyAPIAddressesForAgents(ctx context.Context) (string, error) {
	agentAddrs, err := s.st.GetAPIAddressesForAgents(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	ids := mapKeyOrder(agentAddrs)

	var orderedAddrs controllernode.APIAddresses
	for _, id := range ids {
		addrs := agentAddrs[id]
		if len(addrs) == 0 {
			continue
		}
		orderedAddrs = append(orderedAddrs, addrs...)
	}

	return orderedAddrs.ToNoProxyString(), nil
}

// GetAllAPIAddressesForClients returns all client addresses, including shared
// endpoints, in published priority order within each endpoint group.
func (s *Service) GetAllAPIAddressesForClients(ctx context.Context) ([]string, error) {
	clientAddrs, err := s.st.GetAPIAddressesForClients(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	ids := mapKeyOrder(clientAddrs)

	orderedAddrs := make([]string, 0)
	for _, id := range ids {
		addrs := clientAddrs[id]
		if len(addrs) == 0 {
			continue
		}
		orderedAddrs = append(orderedAddrs, addrs.Values()...)
	}
	return orderedAddrs, nil
}

// GetAPIAddressesByControllerIDForClients returns a map of controller IDs to
// their API addresses that are available for clients. The map is keyed by
// controller ID, and the values are slices of strings representing the API
// addresses for each controller node. Shared endpoints are excluded.
func (s *Service) GetAPIAddressesByControllerIDForClients(ctx context.Context) (map[string][]string, error) {
	addresses, err := s.st.GetAPIAddressesForClients(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := make(map[string][]string, len(addresses))
	for controllerID, addrs := range addresses {
		if controllerID == "" {
			continue
		}
		result[controllerID] = addrs.Values()
	}

	return result, nil
}

// GetAPIAddressesByControllerIDForPeers returns peer API addresses grouped by
// the controller ID they reach. Publication has already selected and ordered
// these addresses.
func (s *Service) GetAPIAddressesByControllerIDForPeers(ctx context.Context) (map[string][]string, error) {
	addresses, err := s.st.GetAPIAddressesForPeers(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := make(map[string][]string, len(addresses))
	for controllerID, addrs := range addresses {
		if controllerID == "" {
			continue
		}
		result[controllerID] = addrs.Values()
	}
	return result, nil
}

// GetAllCloudLocalAPIAddresses returns cloud-local client addresses, including
// shared endpoints. It strips the API ports stored in state, returning bare IP
// addresses or hostnames for consumers such as certificate maintenance.
func (s *Service) GetAllCloudLocalAPIAddresses(ctx context.Context) ([]string, error) {
	addrs, err := s.st.GetAllCloudLocalAPIAddresses(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	returnAddrs := make([]string, len(addrs))
	for i, addr := range addrs {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, errors.Capture(err)
		}
		returnAddrs[i] = host
	}
	return returnAddrs, nil
}

// WatcherFactory instances return watchers for a given namespace and UUID.
type WatcherFactory interface {
	// NewNotifyWatcher returns a new watcher that filters changes from the
	// input base watcher's db/queue. A single filter option is required, though
	// additional filter options can be provided.
	NewNotifyWatcher(
		ctx context.Context,
		summary string,
		filterOption eventsource.FilterOption,
		filterOptions ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)
}

// WatchableService provides the API for working with controller nodes and the
// ability to create watchers.
type WatchableService struct {
	*Service
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new service reference wrapping the input state.
func NewWatchableService(
	st State,
	watcherFactory WatcherFactory,
	logger logger.Logger,
) *WatchableService {
	return &WatchableService{
		Service:        NewService(st, logger),
		watcherFactory: watcherFactory,
	}
}

// WatchControllerNodes returns a watcher that observes changes to the
// controller nodes.
func (s *WatchableService) WatchControllerNodes(ctx context.Context) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.watcherFactory.NewNotifyWatcher(
		ctx,
		"controller nodes watcher",
		eventsource.PredicateFilter(
			s.st.NamespaceForWatchControllerNodes(),
			changestream.All,
			eventsource.AlwaysPredicate,
		),
	)
}

// WatchControllerAgentAddresses returns a watcher that observes changes to the
// controller agent addresses.
func (s *WatchableService) WatchControllerAgentAddresses(ctx context.Context) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.watcherFactory.NewNotifyWatcher(
		ctx,
		"controller agent addresses watcher",
		eventsource.NamespaceFilter(s.st.NamespaceForWatchControllerAgentAddresses(), changestream.All),
	)
}

// WatchControllerClientAddresses returns a watcher that observes changes to the
// controller client addresses.
func (s *WatchableService) WatchControllerClientAddresses(ctx context.Context) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.watcherFactory.NewNotifyWatcher(
		ctx,
		"controller client addresses watcher",
		eventsource.NamespaceFilter(s.st.NamespaceForWatchControllerClientAddresses(), changestream.All),
	)
}

// WatchControllerPeerAddresses returns a watcher that observes changes to the
// controller peer addresses.
func (s *WatchableService) WatchControllerPeerAddresses(ctx context.Context) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.watcherFactory.NewNotifyWatcher(
		ctx,
		"controller peer addresses watcher",
		eventsource.NamespaceFilter(s.st.NamespaceForWatchControllerPeerAddresses(), changestream.All),
	)
}

func mapKeyOrder(m map[string]controllernode.APIAddresses) []string {
	if len(m) == 0 {
		return nil
	}

	ids := make([]string, 0, len(m))
	for controllerID := range m {
		ids = append(ids, controllerID)
	}

	sort.Strings(ids)
	return ids
}
