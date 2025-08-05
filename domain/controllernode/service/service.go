// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"net"
	"net/netip"
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
)

// State describes retrieval and persistence
// methods for controller node concerns.
type State interface {
	// AddDqliteNode adds the Dqlite node ID and bind address for the input
	// controller ID. If the controller ID already exists, it updates the
	// Dqlite node ID and bind address.
	AddDqliteNode(ctx context.Context, controllerID string, nodeID uint64, addr string) error

	// DeleteDqliteNodes removes controller nodes from the controller_node table.
	DeleteDqliteNodes(ctx context.Context, delete []string) error

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

	// NamespaceForWatchControllerAPIAddresses returns the namespace for watching
	// controller api addresses.
	NamespaceForWatchControllerAPIAddresses() string

	// SetAPIAddresses sets the addresses for the provided controller node. It
	// replaces any existing addresses and stores them in the
	// api_controller_address table, with the format "host:port" as a string, as
	// well as the is_agent flag indicating whether the address is available for
	// agents.
	//
	// The following errors can be expected: - [controllernodeerrors.NotFound]
	// if the controller node does not exist.
	SetAPIAddresses(ctx context.Context, addresses map[string]controllernode.APIAddresses) error

	// GetControllerIDs returns the list of controller IDs from the controller
	// node records.
	GetControllerIDs(ctx context.Context) ([]string, error)

	// GetAPIAddressesForAgents returns all APIAddresses available
	// for agents, divided by controller node.
	GetAPIAddressesForAgents(ctx context.Context) (map[string]controllernode.APIAddresses, error)

	// GetAPIAddressesForClients returns all APIAddresses available
	// for clients, divided by controller node.
	GetAPIAddressesForClients(ctx context.Context) (map[string]controllernode.APIAddresses, error)

	// GetAllCloudLocalAPIAddresses returns a string slice of api
	// addresses available for clients. The list only contains cloud
	// local addresses. The returned strings are IP address only without
	// port numbers.
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

// DeleteDqliteNodes deletes the Dqlite node ID and bind address for the input
// controller ID.
func (s *Service) DeleteDqliteNodes(ctx context.Context, controllerIDs []string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.st.DeleteDqliteNodes(ctx, controllerIDs); err != nil {
		return errors.Errorf("deleting Dqlite node details for %q: %w", controllerIDs, err)
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

// SetAPIAddresses sets the provided addresses associated with the provided
// controller IDs.
//
// The following errors can be expected:
// - [controllernodeerrors.NotFound] if the controller node does not exist.
func (s *Service) SetAPIAddresses(ctx context.Context, args controllernode.SetAPIAddressArgs) error {
	addresses := make(map[string]controllernode.APIAddresses, 0)
	for controllerID, addrs := range args.APIAddresses {
		addresses[controllerID] = s.encodeAPIAddresses(ctx, args.MgmtSpace, addrs)
	}
	return s.st.SetAPIAddresses(ctx, addresses)
}

func (s *Service) encodeAPIAddresses(ctx context.Context, mgmtSpace *network.SpaceInfo, addrs network.SpaceHostPorts) controllernode.APIAddresses {
	// We map the SpaceHostPorts addresses to controller api addresses by
	// checking if the address is available for agents (this is the case if the
	// space ID of the address matches the management space ID), and also by
	// joining the address host and port to a string "host:port".
	addresses := make(controllernode.APIAddresses, 0, len(addrs))
	emptyAgentAddresses := true
	for _, spHostPort := range addrs {
		// Check if the address is available for agents. If no management space
		// is set, all addresses are available for agents.
		isAvailableForAgents := mgmtSpace == nil || spHostPort.SpaceID == mgmtSpace.ID
		// Join the address host and port to a string "host:port".
		address := net.JoinHostPort(spHostPort.Host(), strconv.Itoa(spHostPort.Port()))
		addresses = append(addresses, controllernode.APIAddress{
			Address: address,
			IsAgent: isAvailableForAgents,
			Scope:   spHostPort.Scope,
		})
		emptyAgentAddresses = emptyAgentAddresses && !isAvailableForAgents
	}
	// If we have filtered out all addresses, set all to agents to ensure that
	// the API is always reachable for agents.
	if emptyAgentAddresses {
		for i := range addresses {
			addresses[i].IsAgent = true
		}
		s.logger.Warningf(ctx, "all provided API addresses were filtered out with regards to the management space, forcing all addresses to be agents to ensure API connectivity")
	}
	return addresses
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
// controller is not identified.
func (s *Service) GetAPIHostPortsForAgents(ctx context.Context) ([]network.HostPorts, error) {
	agentAddrs, err := s.st.GetAPIAddressesForAgents(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return transformToOrderedHostPorts(agentAddrs)
}

// GetAPIHostPortsForClients returns API HostPorts that are available for
// clients. HostPorts are grouped by controller node, though each specific
// controller is not identified.
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

		address, err := addr.ToHostPortsNoMachineLocal()
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
// addresses for each controller node.
func (s *Service) GetAPIAddressesByControllerIDForAgents(ctx context.Context) (map[string][]string, error) {
	addresses, err := s.st.GetAPIAddressesForAgents(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := make(map[string][]string, len(addresses))
	for controllerID, addrs := range addresses {
		result[controllerID] = addrs.PrioritizedForScope(controllernode.ScopeMatchCloudLocal)
	}

	return result, nil
}

// GetAllAPIAddressesForAgents returns a string slice of api
// addresses available for agents ordered to prefer local-cloud scoped
// addresses and IPv4 over IPv6 for each machine.
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
		orderedAddrs = append(orderedAddrs, addrs.PrioritizedForScope(controllernode.ScopeMatchCloudLocal)...)
	}
	return orderedAddrs, nil
}

// GetAllNoProxyAPIAddressesForAgents returns a sorted, comma separated string
// of agent API addresses suitable for no proxy settings.
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

// GetAllAPIAddressesForClients returns a string slice of api
// addresses available for agents ordered to prefer public scoped
// addresses and IPv4 over IPv6 for each machine.
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
		orderedAddrs = append(orderedAddrs, addrs.PrioritizedForScope(controllernode.ScopeMatchPublic)...)
	}
	return orderedAddrs, nil
}

// GetAllCloudLocalAPIAddresses returns a string slice of api
// addresses available for clients. The list only contains cloud
// local addresses. The returned strings are IP address only without
// port numbers.
func (s *Service) GetAllCloudLocalAPIAddresses(ctx context.Context) ([]string, error) {
	addrs, err := s.st.GetAllCloudLocalAPIAddresses(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	returnAddrs := make([]string, len(addrs))
	for i, addr := range addrs {
		ip, err := netip.ParseAddrPort(addr)
		if err != nil {
			return nil, errors.Capture(err)
		}
		returnAddrs[i] = ip.Addr().String()
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
	_, span := trace.Start(ctx, trace.NameFromFunc())
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

// WatchControllerAPIAddresses returns a watcher that observes changes to the
// controller api addresses.
func (s *WatchableService) WatchControllerAPIAddresses(ctx context.Context) (watcher.NotifyWatcher, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.watcherFactory.NewNotifyWatcher(
		ctx,
		"controller api addresses watcher",
		eventsource.NamespaceFilter(s.st.NamespaceForWatchControllerAPIAddresses(), changestream.All),
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
