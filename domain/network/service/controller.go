// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"net/netip"
	"sort"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
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

	// GetModelType returns the type of the current model.
	GetModelType(context.Context) (model.ModelType, error)

	// NamespacesForWatchControllerNetwork returns all the namespaces that we
	// need to watch in order to re-trigger controller network detection.
	NamespacesForWatchControllerNetwork() []string
}

// GetControllerPeerAddresses returns IP/DNS addresses for communication between
// controllers. Dying units remain eligible while draining; Dead, missing and
// non-controller units are rejected. An existing unit with no addresses returns
// an empty result, which is distinct from a failed read.
//
// A management space restricts machine-controller candidates when it contains
// eligible addresses; otherwise all eligible candidates remain as a fallback.
// Kubernetes pod scopes are preserved, including legacy machine-local scopes.
func (s *Service) GetControllerPeerAddresses(ctx context.Context, name unit.Name, managementSpace network.SpaceName) (network.SpaceAddresses, error) {
	controllerAddresses, err := s.controllerNetwork(ctx, name)
	if err != nil {
		return nil, errors.Capture(err)
	}
	modelType, err := s.st.GetModelType(ctx)
	if err != nil {
		return nil, errors.Errorf("getting controller model type: %w", err)
	}
	addresses, space, err := s.selectControllerAddresses(ctx, controllerAddresses, modelType, managementSpace)
	if err != nil {
		return nil, errors.Capture(err)
	}
	if space != nil {
		var matched network.SpaceAddresses
		for _, address := range addresses {
			if address.SpaceID == space.ID {
				matched = append(matched, address)
			}
		}
		if len(matched) > 0 {
			addresses = matched
		}
	}
	return orderControllerAddresses(addresses), nil
}

// GetControllerTargetAddresses returns addresses for external operations that
// must reach this particular controller. Alive and Dying units are eligible.
// Selection is independent of management-space configuration; addresses in that
// space remain eligible under the ordinary client address-selection rules.
// Kubernetes returns [coreerrors.NotSupported]: pod reachability inside the
// cluster is insufficient to establish an external node-specific transport.
func (s *Service) GetControllerTargetAddresses(ctx context.Context, name unit.Name) (network.SpaceAddresses, error) {
	controllerAddresses, err := s.controllerNetwork(ctx, name)
	if err != nil {
		return nil, errors.Capture(err)
	}
	modelType, err := s.st.GetModelType(ctx)
	if err != nil {
		return nil, errors.Errorf("getting controller model type: %w", err)
	}
	if modelType == model.CAAS {
		return nil, errors.Errorf("external targeting of Kubernetes controllers: %w", coreerrors.NotSupported)
	}
	addresses := selectControllerAPIAddresses(controllerAddressCandidates(controllerAddresses, modelType), nil)
	return orderControllerAddresses(addresses), nil
}

func (s *Service) selectControllerAddresses(ctx context.Context, addresses domainnetwork.ControllerAPIAddresses, modelType model.ModelType, managementSpace network.SpaceName) (network.SpaceAddresses, *network.SpaceInfo, error) {
	var space *network.SpaceInfo
	if managementSpace != "" && modelType == model.IAAS {
		var err error
		space, err = s.st.GetSpaceByName(ctx, managementSpace)
		if err != nil {
			return nil, nil, errors.Errorf("getting management space %q: %w", managementSpace, err)
		}
	}
	return selectControllerAPIAddresses(controllerAddressCandidates(addresses, modelType), space), space, nil
}

func (s *Service) controllerNetwork(ctx context.Context, name unit.Name) (domainnetwork.ControllerAPIAddresses, error) {
	if err := name.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	if name.Application() != application.ControllerApplicationName {
		return nil, applicationerrors.UnitNotFound
	}
	addresses, err := s.st.GetControllerUnitNetwork(ctx, name.String())
	if err != nil {
		return nil, errors.Errorf("getting network for controller unit %q: %w", name, err)
	}
	return addresses, nil
}

func controllerAddressCandidates(addresses domainnetwork.ControllerAPIAddresses, modelType model.ModelType) domainnetwork.ControllerAPIAddresses {
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
			if modelType != model.CAAS || candidate.Type == network.HostName {
				continue
			}
		default:
			continue
		}
		result = append(result, candidate)
	}
	return result
}

func orderControllerAddresses(addresses network.SpaceAddresses) network.SpaceAddresses {
	// Stable ordering prevents unchanged facts from causing connection churn.
	sort.Slice(addresses, func(i, j int) bool {
		a, b := addresses[i], addresses[j]
		if a.Scope != b.Scope {
			return a.Scope < b.Scope
		}
		if a.Value != b.Value {
			return a.Value < b.Value
		}
		return a.SpaceID < b.SpaceID
	})
	return addresses
}

// WatchControllerNetwork observes unit identity/lifecycle, IP/DNS, device and
// space changes. Consume the initial event before the first address query.
// Watching source namespaces rather than a snapshot of net-node IDs ensures
// reassociation and changes to a replacement node remain observable.
func (s *WatchableService) WatchControllerNetwork(ctx context.Context) (watcher.NotifyWatcher, error) {
	var filters []eventsource.FilterOption
	for _, namespace := range s.st.NamespacesForWatchControllerNetwork() {
		filters = append(filters, eventsource.NamespaceFilter(namespace, changestream.All))
	}
	return s.watcherFactory.NewNotifyWatcher(ctx, "controller network", filters[0], filters[1:]...)
}
