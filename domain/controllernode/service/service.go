// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"net"
	"net/netip"
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
	// CurateNodes adds and removes controller node records according to the
	// input slices.
	CurateNodes(context.Context, []string, []string) error

	// UpdateDqliteNode sets the Dqlite node ID and bind address
	// for the input controller ID.
	// The controller ID must be a valid controller node.
	UpdateDqliteNode(context.Context, string, uint64, string) error

	// IsControllerNode returns true if the supplied nodeID is a controller
	// node.
	IsControllerNode(context.Context, string) (bool, error)

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

	// GetAPIAddresses returns the list of API addresses for the provided
	// controller node.
	GetAPIAddresses(ctx context.Context, ctrlID string) ([]string, error)

	// GetAPIAddressesByControllerIDForAgents returns a map of controller IDs to
	// their API addresses that are available for agents. The map is keyed by
	// controller ID, and the values are slices of strings representing the API
	// addresses for each controller node.
	GetAPIAddressesByControllerIDForAgents(ctx context.Context) (map[string][]string, error)

	// GetAPIAddressesForAgents returns the list of API address strings including
	// port for the provided controller node that are available for agents.
	GetAPIAddressesForAgents(ctx context.Context, ctrlID string) ([]string, error)

	// GetAllAPIAddressesWithScopeForAgents returns all APIAddresses available for
	// agents, divided by controller node.
	GetAllAPIAddressesWithScopeForAgents(ctx context.Context) ([]controllernode.APIAddresses, error)

	// GetAllAPIAddressesWithScopeForClients returns all APIAddresses available for
	// clients, divided by controller node.
	GetAllAPIAddressesWithScopeForClients(ctx context.Context) ([]controllernode.APIAddresses, error)

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

// CurateNodes modifies the known control plane by adding and removing
// controller node records according to the input slices.
func (s *Service) CurateNodes(ctx context.Context, toAdd, toRemove []string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.st.CurateNodes(ctx, toAdd, toRemove); err != nil {
		return errors.Errorf("curating controller codes; adding %v, removing %v: %w", toAdd, toRemove, err)
	}
	return nil
}

// UpdateDqliteNode sets the Dqlite node ID and bind address for the input
// controller ID.
func (s *Service) UpdateDqliteNode(ctx context.Context, controllerID string, nodeID uint64, addr string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.st.UpdateDqliteNode(ctx, controllerID, nodeID, addr); err != nil {
		return errors.Errorf("updating Dqlite node details for %q: %w", controllerID, err)
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

// IsControllerNode returns true if the supplied nodeID is a controller node.
func (s *Service) IsControllerNode(ctx context.Context, nodeID string) (bool, error) {
	if nodeID == "" {
		return false, errors.Errorf("node ID %q is %w, cannot be empty", nodeID, coreerrors.NotValid)
	}

	isController, err := s.st.IsControllerNode(ctx, nodeID)
	if err != nil {
		return false, errors.Errorf("checking is controller node: %w", err)
	}
	return isController, nil
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

// GetAPIAddresses returns the list of API addresses for the provided controller
// node.
func (s *Service) GetAPIAddresses(ctx context.Context, nodeID string) ([]string, error) {
	if nodeID == "" {
		return nil, errors.Errorf("node ID %q is %w, cannot be empty", nodeID, coreerrors.NotValid)
	}
	return s.st.GetAPIAddresses(ctx, nodeID)
}

// GetAPIHostPortsForAgents returns API HostPorts that are available for
// agents. HostPorts are grouped by controller node, though each specific
// controller is not identified.
func (s *Service) GetAPIHostPortsForAgents(ctx context.Context) ([]network.HostPorts, error) {
	agentAddrs, err := s.st.GetAllAPIAddressesWithScopeForAgents(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := make([]network.HostPorts, len(agentAddrs))
	for i, addr := range agentAddrs {
		result[i], err = addr.ToHostPortsNoMachineLocal()
		if err != nil {
			return nil, errors.Capture(err)
		}
	}
	return result, nil
}

// GetAPIHostPortsForClients returns API HostPorts that are available for
// clients. HostPorts are grouped by controller node, though each specific
// controller is not identified.
func (s *Service) GetAPIHostPortsForClients(ctx context.Context) ([]network.HostPorts, error) {
	clientAddrs, err := s.st.GetAllAPIAddressesWithScopeForClients(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := make([]network.HostPorts, len(clientAddrs))
	for i, addr := range clientAddrs {
		// todo - skip machine local
		result[i], err = addr.ToHostPortsNoMachineLocal()
		if err != nil {
			return nil, errors.Capture(err)
		}
	}
	return result, nil
}

// GetAPIAddressesByControllerIDForAgents returns a map of controller IDs to
// their API addresses that are available for agents. The map is keyed by
// controller ID, and the values are slices of strings representing the API
// addresses for each controller node.
func (s *Service) GetAPIAddressesByControllerIDForAgents(ctx context.Context) (map[string][]string, error) {
	return s.st.GetAPIAddressesByControllerIDForAgents(ctx)
}

// GetAllAPIAddressesForAgentsInPreferredOrder returns a string slice of api
// addresses available for agents ordered to prefer local-cloud scoped
// addresses and IPv4 over IPv6 for each machine.
func (s *Service) GetAllAPIAddressesForAgentsInPreferredOrder(ctx context.Context) ([]string, error) {
	agentAddrs, err := s.st.GetAllAPIAddressesWithScopeForAgents(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	orderedAddrs := make([]string, 0)
	for _, addrs := range agentAddrs {
		orderedAddrs = append(orderedAddrs, addrs.PrioritizedForScope(controllernode.ScopeMatchCloudLocal)...)
	}
	return orderedAddrs, nil
}

// GetAllNoProxyAPIAddressesForAgents returns a sorted, comma separated string
// of agent API addresses suitable for no proxy settings.
func (s *Service) GetAllNoProxyAPIAddressesForAgents(ctx context.Context) (string, error) {
	agentAddrs, err := s.st.GetAllAPIAddressesWithScopeForAgents(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}
	orderedAddrs := make(controllernode.APIAddresses, 0)
	for _, addrs := range agentAddrs {
		orderedAddrs = append(orderedAddrs, addrs...)
	}
	return orderedAddrs.ToNoProxyString(), nil
}

// GetAllAPIAddressesForClients returns a string slice of api
// addresses available for agents ordered to prefer public scoped
// addresses and IPv4 over IPv6 for each machine.
func (s *Service) GetAllAPIAddressesForClients(ctx context.Context) ([]string, error) {
	clientAddrs, err := s.st.GetAllAPIAddressesWithScopeForClients(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	orderedAddrs := make([]string, 0)
	for _, addrs := range clientAddrs {
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

// GetAPIAddressesForAgents returns the list of API address strings including
// port for the provided controller node that are available for agents.
func (s *Service) GetAPIAddressesForAgents(ctx context.Context, nodeID string) ([]string, error) {
	if nodeID == "" {
		return nil, errors.Errorf("node ID %q is %w, cannot be empty", nodeID, coreerrors.NotValid)
	}
	return s.st.GetAPIAddressesForAgents(ctx, nodeID)
}

// WatcherFactory instances return watchers for a given namespace and UUID.
type WatcherFactory interface {
	// NewNotifyWatcher returns a new watcher that filters changes from the
	// input base watcher's db/queue. A single filter option is required, though
	// additional filter options can be provided.
	NewNotifyWatcher(
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
		eventsource.NamespaceFilter(s.st.NamespaceForWatchControllerAPIAddresses(), changestream.All),
	)
}
