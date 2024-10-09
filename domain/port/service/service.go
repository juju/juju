// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/port"
	"github.com/juju/juju/internal/errors"
)

// WildcardEndpoint is a special endpoint that represents all endpoints.
const WildcardEndpoint = ""

// AtomicState describes the subset of methods on state that run within an atomic
// context.
type AtomicState interface {
	domain.AtomicStateBase

	// GetColocatedOpenedPorts returns all the open ports for all units co-located
	// with the given unit. Units are considered co-located if they share the same
	// net-node.
	GetColocatedOpenedPorts(ctx domain.AtomicContext, unitUUID coreunit.UUID) ([]network.PortRange, error)

	// GetEndpointOpenedPorts returns the opened ports for a given endpoint of a
	// given unit.
	GetEndpointOpenedPorts(ctx domain.AtomicContext, unitUUID coreunit.UUID, endpoint string) ([]network.PortRange, error)

	// GetEndpoints returns all endpoints for a given unit.
	GetEndpoints(ctx domain.AtomicContext, unitUUID coreunit.UUID) ([]string, error)

	// UpdateUnitPorts opens and closes ports for the endpoints of a given unit.
	// The opened and closed ports for the same endpoints must not conflict.
	UpdateUnitPorts(ctx domain.AtomicContext, unitUUID coreunit.UUID, openPorts, closePorts network.GroupedPortRanges) error
}

// State describes the methods that a state implementation must provide to
// manage opened ports for units.
type State interface {
	WatcherState
	AtomicState

	// GetUnitOpenedPorts returns the opened ports for a given unit uuid,
	// grouped by endpoint.
	GetUnitOpenedPorts(ctx context.Context, unitUUID coreunit.UUID) (network.GroupedPortRanges, error)

	// GetMachineOpenedPorts returns the opened ports for all the units on the
	// given machine. Opened ports are grouped first by unit and then by endpoint.
	GetMachineOpenedPorts(ctx context.Context, machineUUID string) (map[coreunit.UUID]network.GroupedPortRanges, error)

	// GetApplicationOpenedPorts returns the opened ports for all the units of the
	// given application. We return opened ports paired with the unit UUIDs, grouped
	// by endpoint.
	GetApplicationOpenedPorts(ctx context.Context, applicationUUID coreapplication.ID) (port.UnitEndpointPortRanges, error)

	// SetUnitPorts sets open ports for the endpoints of a given unit.
	SetUnitPorts(ctx context.Context, unitName string, openPorts network.GroupedPortRanges) error
}

// Service provides the API for managing the opened ports for units.
type Service struct {
	st State
}

// NewService returns a new Service for managing opened ports for units.
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}

// GetUnitOpenedPorts returns the opened ports for a given unit uuid, grouped by
// endpoint.
func (s *Service) GetUnitOpenedPorts(ctx context.Context, unitUUID coreunit.UUID) (network.GroupedPortRanges, error) {
	return s.st.GetUnitOpenedPorts(ctx, unitUUID)
}

// GetMachineOpenedPorts returns the opened ports for all the units on the machine.
// Opened ports are grouped first by unit and then by endpoint.
//
// TODO: Once we have a core static machine uuid type, use it here.
func (s *Service) GetMachineOpenedPorts(ctx context.Context, machineUUID string) (map[coreunit.UUID]network.GroupedPortRanges, error) {
	return s.st.GetMachineOpenedPorts(ctx, machineUUID)
}

// GetApplicationOpenedPorts returns the opened ports for all the units of the
// application. Opened ports are grouped first by unit and then by endpoint.
func (s *Service) GetApplicationOpenedPorts(ctx context.Context, applicationUUID coreapplication.ID) (map[coreunit.UUID]network.GroupedPortRanges, error) {
	openedPorts, err := s.st.GetApplicationOpenedPorts(ctx, applicationUUID)
	if err != nil {
		return nil, errors.Errorf("failed to get opened ports for application %s: %w", applicationUUID, err)
	}
	return openedPorts.ByUnitByEndpoint(), nil
}

// GetApplicationOpenedPortsByEndpoint returns all the opened ports for the given
// application, across all units, grouped by endpoint.
//
// NOTE: The returned port ranges are atomised, meaning that each port range
// we guarantee that each port range is of unit length. This is useful for
// down-stream consumers such as k8s, which can only reason with unit-length
// port ranges.
func (s *Service) GetApplicationOpenedPortsByEndpoint(ctx context.Context, applicationUUID coreapplication.ID) (network.GroupedPortRanges, error) {
	openedPorts, err := s.st.GetApplicationOpenedPorts(ctx, applicationUUID)
	if err != nil {
		return nil, errors.Errorf("failed to get opened ports for application %s: %w", applicationUUID, err)
	}
	ret := network.GroupedPortRanges{}

	// group port ranges by endpoint across all units and atomise them.
	for _, openedPort := range openedPorts {
		endpoint := openedPort.Endpoint
		ret[endpoint] = append(ret[endpoint], atomisePortRange(openedPort.PortRange)...)
	}

	// de-dupe our port ranges
	for endpoint, portRanges := range ret {
		ret[endpoint] = network.UniquePortRanges(portRanges)
	}

	return ret, nil
}

// atomisePortRange breaks down the input port range into a slice of unit-length
// port ranges.
func atomisePortRange(portRange network.PortRange) []network.PortRange {
	ret := make([]network.PortRange, portRange.Length())
	for i := 0; i < portRange.Length(); i++ {
		ret[i] = network.PortRange{
			Protocol: portRange.Protocol,
			FromPort: portRange.FromPort + i,
			ToPort:   portRange.FromPort + i,
		}
	}
	return ret
}

func (s *Service) SetUnitPorts(ctx context.Context, unitName string, openPorts network.GroupedPortRanges) error {
	return s.st.SetUnitPorts(ctx, unitName, openPorts)
}

// UpdateUnitPorts opens and closes ports for the endpoints of a given unit.
//
// NOTE: There is a special wildcard endpoint "" that represents all endpoints.
// Any operations applied to the wildcard endpoint will logically applied to all
// endpoints.
//
// That is, if we open a port range on the wildcard endpoint, we will open it as
// usual but as a side effect we close that port range on all other endpoints.
//
// On the other hand, if we close a specific endpoint's port range that is open
// on the wildcard endpoint, we will close it on the wildcard endpoint and open
// it on all other endpoints except the targeted endpoint.
func (s *Service) UpdateUnitPorts(ctx context.Context, unitUUID coreunit.UUID, openPorts, closePorts network.GroupedPortRanges) error {
	if len(openPorts.UniquePortRanges())+len(closePorts.UniquePortRanges()) == 0 {
		return nil
	}

	allInputPortRanges := append(openPorts.UniquePortRanges(), closePorts.UniquePortRanges()...)
	//  verify input port ranges do not conflict with each other.
	err := verifyNoPortRangeConflicts(allInputPortRanges, allInputPortRanges)
	if err != nil {
		return errors.Errorf("cannot update unit ports with conflict(s): %w", err)
	}

	// construct a map of all port ranges being closed to the endpoint they're
	// being closed on. Except the wildcard endpoint.
	closePortsToEndpointMap := make(map[network.PortRange]string)
	for endpoint, endpointClosePorts := range closePorts {
		for _, portRange := range endpointClosePorts {
			closePortsToEndpointMap[portRange] = endpoint
		}
	}
	// Ensure endpoints closed on the wildcard endpoint are not in the map.
	for _, wildcardClosePorts := range closePorts[WildcardEndpoint] {
		delete(closePortsToEndpointMap, wildcardClosePorts)
	}

	err = s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		// Verify input port ranges do not conflict with any port ranges
		// co-located with the unit.
		colocatedOpened, err := s.st.GetColocatedOpenedPorts(ctx, unitUUID)
		if err != nil {
			return errors.Errorf("failed to get opened ports co-located with unit %s: %w", unitUUID, err)
		}
		err = verifyNoPortRangeConflicts(allInputPortRanges, colocatedOpened)
		if err != nil {
			return errors.Errorf("cannot update unit ports with conflict(s) on co-located units: %w", err)
		}

		wildcardOpen, _ := openPorts[WildcardEndpoint]
		wildcardClose, _ := closePorts[WildcardEndpoint]

		wildcardOpened, err := s.st.GetEndpointOpenedPorts(ctx, unitUUID, WildcardEndpoint)
		if err != nil {
			return errors.Errorf("failed to get opened ports for wildcard endpoint: %w", err)
		}
		wildcardOpenedSet := map[network.PortRange]bool{}
		for _, portRange := range wildcardOpened {
			wildcardOpenedSet[portRange] = true
		}

		// Remove openPorts ranges that are already open on the wildcard endpoint.
		for endpoint, endpointOpenPorts := range openPorts {
			if endpoint == WildcardEndpoint {
				continue
			}
			for i, portRange := range endpointOpenPorts {
				if _, ok := wildcardOpenedSet[portRange]; ok {
					openPorts[endpoint] = append(openPorts[endpoint][:i], openPorts[endpoint][i+1:]...)
				}
			}
		}

		// cache for endpoints. We may need to list the existing endpoints 0, 1,
		// or n times. Cache the result and only fill it when we need it, to avoid
		// unnecessary calls.
		var endpoints []string

		// If we're opening a port range on the wildcard endpoint, we need to
		// close it on all other endpoints.
		//
		// NOTE: This ensures that is a port range is open on the wildcard
		// endpoint, it is closed on all other endpoints.
		for _, openPortRange := range wildcardOpen {
			if endpoints == nil {
				endpoints, err = s.st.GetEndpoints(ctx, unitUUID)
				if err != nil {
					return errors.Errorf("failed to get unit endpoints: %w", err)
				}
			}

			for _, endpoint := range endpoints {
				if endpoint == WildcardEndpoint {
					continue
				}
				delete(openPorts, endpoint)
				closePorts[endpoint] = append(closePorts[endpoint], openPortRange)
			}
		}

		// Close port ranges closed on the wildcard endpoint on all other endpoints.
		for _, closePortRange := range wildcardClose {
			if endpoints == nil {
				endpoints, err = s.st.GetEndpoints(ctx, unitUUID)
				if err != nil {
					return errors.Errorf("failed to get unit endpoints: %w", err)
				}
			}

			for _, endpoint := range endpoints {
				if endpoint == WildcardEndpoint {
					continue
				}
				closePorts[endpoint] = append(closePorts[endpoint], closePortRange)
			}
		}

		// If we're closing a port range for a specific endpoint which is open
		// on the wildcard endpoint, we need to close it on the wildcard endpoint
		// and open it on all other endpoints except the targeted endpoint.
		for _, portRange := range wildcardOpened {
			if endpoint, ok := closePortsToEndpointMap[portRange]; ok {
				if endpoints == nil {
					endpoints, err = s.st.GetEndpoints(ctx, unitUUID)
					if err != nil {
						return errors.Errorf("failed to get unit endpoints: %w", err)
					}
				}

				// This port range, open on the wildcard endpoint, is being closed
				// on some endpoint. We need to close it on the wildcard, and open
				// it on all endpoints other than the wildcard & targeted endpoint.
				closePorts[WildcardEndpoint] = append(closePorts[WildcardEndpoint], portRange)

				for _, otherEndpoint := range endpoints {
					if otherEndpoint == WildcardEndpoint || otherEndpoint == endpoint {
						continue
					}
					openPorts[otherEndpoint] = append(openPorts[otherEndpoint], portRange)
				}

				// Remove the port range from openPorts for the targeted endpoint.
				for i, otherPortRange := range openPorts[endpoint] {
					if otherPortRange == portRange {
						openPorts[endpoint] = append(openPorts[endpoint][:i], openPorts[endpoint][i+1:]...)
						break
					}
				}
			}
		}

		return s.st.UpdateUnitPorts(ctx, unitUUID, openPorts, closePorts)
	})
	if err != nil {
		return errors.Errorf("failed to update unit ports: %w", err)
	}
	return nil
}

// verifyNoPortRangeConflicts verifies the provided port ranges do not conflict
// with each other.
//
// A conflict occurs when two (or more) port ranges across all endpoints overlap,
// but are not equal.
func verifyNoPortRangeConflicts(rangesA, rangesB []network.PortRange) error {
	var conflicts []string
	for _, portRange := range rangesA {
		for _, otherPortRange := range rangesB {
			if portRange.ConflictsWith(otherPortRange) && portRange != otherPortRange {
				conflicts = append(conflicts, fmt.Sprintf("[%s, %s]", portRange, otherPortRange))
			}
		}
	}
	if len(conflicts) == 0 {
		return nil
	}
	return errors.Errorf("%w: %s", port.ErrPortRangeConflict, conflicts)
}
