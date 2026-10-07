// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"net/netip"
	"sort"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/errors"
)

// ControllerState reads controller-model network facts without a provider.
type ControllerState interface {
	// GetControllerUnitNetwork returns addresses possible for use in contacting
	// a controller.
	GetControllerUnitNetwork(context.Context, string) (domainnetwork.ControllerAPIAddresses, error)

	// GetControllerServiceAddresses returns the shared controller Service's
	// addresses independently of the controller units.
	GetControllerServiceAddresses(context.Context) (domainnetwork.ControllerAPIAddresses, error)

	// GetModelType returns the type of the current model.
	GetModelType(context.Context) (model.ModelType, error)

	// NamespacesForWatchControllerNetwork returns all the namespaces that we
	// need to watch in order to re-trigger controller network detection.
	NamespacesForWatchControllerNetwork() []string
}

// GetControllerPeerAddresses returns IP/DNS addresses for communication between
// controllers, grouped by controller unit. Dying units remain eligible while
// draining; Dead, missing and non-controller units are rejected. An existing
// unit with no addresses has an empty result, which is distinct from a failed
// read. Invalid names and failed reads return errors, never a partial selection.
//
// A management space restricts machine-controller candidates when it contains
// eligible addresses; otherwise all eligible candidates remain as a fallback.
// Management-space configuration is ignored for Kubernetes models.
// Kubernetes pod scopes are preserved, including legacy machine-local scopes.
func (s *Service) GetControllerPeerAddresses(ctx context.Context, names []unit.Name, managementSpace network.SpaceName) (domainnetwork.ControllerAddressSelection, error) {
	for _, name := range names {
		if err := validateControllerUnitName(name); err != nil {
			return domainnetwork.ControllerAddressSelection{}, errors.Capture(err)
		}
	}
	modelType, err := s.st.GetModelType(ctx)
	if err != nil {
		return domainnetwork.ControllerAddressSelection{}, errors.Errorf("getting controller model type: %w", err)
	}
	var space *network.SpaceInfo
	if managementSpace != "" && modelType == model.IAAS {
		space, err = s.st.GetSpaceByName(ctx, managementSpace)
		if err != nil {
			return domainnetwork.ControllerAddressSelection{}, errors.Errorf("getting management space %q: %w", managementSpace, err)
		}
	}

	result := domainnetwork.ControllerAddressSelection{
		ByUnit: make(map[unit.Name]network.SpaceAddresses, len(names)),
	}
	for _, name := range names {
		controllerAddresses, err := s.controllerNetwork(ctx, name)
		if err != nil {
			return domainnetwork.ControllerAddressSelection{}, errors.Capture(err)
		}
		addresses := selectControllerAddresses(controllerAddresses, modelType == model.CAAS, space)
		result.ByUnit[name] = orderControllerAddresses(addresses, network.ScopeCloudLocal)
	}
	return result, nil
}

// GetControllerClientAddresses selects addresses for ordinary client discovery.
// Machine addresses retain their unit identity and are independent of management
// space configuration. Kubernetes uses shared Service addresses, preferring
// public addresses with cloud-local fallback; pod addresses are excluded.
// The caller supplies controller membership as unit names. Kubernetes Service
// discovery is independent of that membership, including an empty list.
// Invalid names and failed reads return errors, never a partial selection.
func (s *Service) GetControllerClientAddresses(ctx context.Context, names []unit.Name) (domainnetwork.ControllerAddressSelection, error) {
	return s.getControllerDiscoveryAddresses(ctx, names, "", network.ScopePublic)
}

// GetControllerAgentAddresses selects addresses for ordinary agent discovery.
// Machine addresses retain their unit identity and honour the management space,
// falling back to eligible addresses when that space has no candidates.
// Kubernetes uses shared Service addresses, preferring cloud-local addresses
// with public fallback; pod addresses and management-space policy do not apply.
// Shared addresses remain available even when names is empty. Invalid names
// and failed reads return errors, never a partial selection.
func (s *Service) GetControllerAgentAddresses(ctx context.Context, names []unit.Name, managementSpace network.SpaceName) (domainnetwork.ControllerAddressSelection, error) {
	return s.getControllerDiscoveryAddresses(ctx, names, managementSpace, network.ScopeCloudLocal)
}

func (s *Service) getControllerDiscoveryAddresses(ctx context.Context, names []unit.Name, managementSpace network.SpaceName, preferredScope network.Scope) (domainnetwork.ControllerAddressSelection, error) {
	for _, name := range names {
		if err := validateControllerUnitName(name); err != nil {
			return domainnetwork.ControllerAddressSelection{}, errors.Capture(err)
		}
	}
	modelType, err := s.st.GetModelType(ctx)
	if err != nil {
		return domainnetwork.ControllerAddressSelection{}, errors.Errorf("getting controller model type: %w", err)
	}
	if modelType == model.CAAS {
		candidates, err := s.st.GetControllerServiceAddresses(ctx)
		if err != nil {
			return domainnetwork.ControllerAddressSelection{}, errors.Errorf("getting controller Service addresses: %w", err)
		}
		addresses := selectControllerAPIAddresses(controllerAddressCandidates(candidates, false), nil)
		return domainnetwork.ControllerAddressSelection{
			Shared: orderControllerAddresses(addresses, preferredScope),
		}, nil
	}

	result := domainnetwork.ControllerAddressSelection{
		ByUnit: make(map[unit.Name]network.SpaceAddresses, len(names)),
	}

	var space *network.SpaceInfo
	if managementSpace != "" {
		space, err = s.st.GetSpaceByName(ctx, managementSpace)
		if err != nil {
			return domainnetwork.ControllerAddressSelection{}, errors.Errorf("getting management space %q: %w", managementSpace, err)
		}
	}

	for _, name := range names {
		candidates, err := s.controllerNetwork(ctx, name)
		if err != nil {
			return domainnetwork.ControllerAddressSelection{}, errors.Capture(err)
		}
		addresses := selectControllerAddresses(candidates, false, space)
		result.ByUnit[name] = orderControllerAddresses(addresses, preferredScope)
	}
	return result, nil
}

func selectControllerAddresses(candidates domainnetwork.ControllerAPIAddresses, includeMachineLocal bool, space *network.SpaceInfo) network.SpaceAddresses {
	addresses := selectControllerAPIAddresses(controllerAddressCandidates(candidates, includeMachineLocal), space)
	if space == nil {
		return addresses
	}
	var matched network.SpaceAddresses
	for _, address := range addresses {
		if address.SpaceID == space.ID {
			matched = append(matched, address)
		}
	}
	if len(matched) > 0 {
		return matched
	}
	return addresses
}

func (s *Service) controllerNetwork(ctx context.Context, name unit.Name) (domainnetwork.ControllerAPIAddresses, error) {
	if err := validateControllerUnitName(name); err != nil {
		return nil, errors.Capture(err)
	}
	addresses, err := s.st.GetControllerUnitNetwork(ctx, name.String())
	if err != nil {
		return nil, errors.Errorf("getting network for controller unit %q: %w", name, err)
	}
	return addresses, nil
}

func validateControllerUnitName(name unit.Name) error {
	if err := name.Validate(); err != nil {
		return errors.Capture(err)
	}
	if name.Application() != application.ControllerApplicationName {
		return applicationerrors.UnitNotFound
	}
	return nil
}

func controllerAddressCandidates(addresses domainnetwork.ControllerAPIAddresses, includeMachineLocal bool) domainnetwork.ControllerAPIAddresses {
	var result domainnetwork.ControllerAPIAddresses
	for _, candidate := range addresses {
		if candidate.DeviceType == domainnetwork.DeviceTypeLoopback {
			continue
		}
		if candidate.Type != network.HostName {
			ip, err := netip.ParseAddr(candidate.Value)
			if err != nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
				continue
			}
		}
		switch candidate.Scope {
		case network.ScopeCloudLocal, network.ScopePublic:
		case network.ScopeMachineLocal:
			if !includeMachineLocal || candidate.Type == network.HostName {
				continue
			}
		default:
			continue
		}
		result = append(result, candidate)
	}
	return result
}

func orderControllerAddresses(addresses network.SpaceAddresses, preferredScope network.Scope) network.SpaceAddresses {
	// Stable ordering prevents unchanged facts from causing connection churn.
	sort.SliceStable(addresses, func(i, j int) bool {
		a, b := addresses[i], addresses[j]
		if a.Scope != b.Scope {
			if a.Scope == preferredScope || b.Scope == preferredScope {
				return a.Scope == preferredScope
			}
			return a.Scope < b.Scope
		}
		if a.Value != b.Value {
			return a.Value < b.Value
		}
		return a.SpaceID < b.SpaceID
	})
	return addresses
}

// WatchControllerNetwork observes unit identity/lifecycle, Service associations,
// IP/DNS, device and space changes. Consume its initial event before querying.
// Watching source namespaces rather than a snapshot of net-node IDs ensures
// reassociation and changes to a replacement node remain observable.
func (s *WatchableService) WatchControllerNetwork(ctx context.Context) (watcher.NotifyWatcher, error) {
	var filters []eventsource.FilterOption
	for _, namespace := range s.st.NamespacesForWatchControllerNetwork() {
		filters = append(filters, eventsource.NamespaceFilter(namespace, changestream.All))
	}
	return s.watcherFactory.NewNotifyWatcher(ctx, "controller network", filters[0], filters[1:]...)
}
