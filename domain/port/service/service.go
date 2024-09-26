// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"sort"

	"github.com/juju/collections/set"

	"github.com/juju/juju/core/network"
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
	GetColocatedOpenedPorts(ctx domain.AtomicContext, unitUUID string) ([]network.PortRange, error)

	// GetUnitOpenedPortsUUID returns the opened ports for the given unit with the
	// UUID of the port range. The opened ports are grouped by endpoint.
	GetUnitOpenedPortsUUID(ctx domain.AtomicContext, unitUUID string) (map[string][]port.PortRangeUUID, error)

	// GetEndpoints returns all endpoints for a given unit.
	GetEndpoints(ctx domain.AtomicContext, unitUUID string) ([]port.Endpoint, error)

	// AddEndpoints adds the endpoints to a given unit. Return the added endpoints
	// with their corresponding UUIDs.
	AddEndpoints(ctx domain.AtomicContext, unitUUID string, endpoints []string) ([]port.Endpoint, error)

	// AddOpenedPorts adds the given port ranges to the database. Port ranges must
	// be grouped by endpoint UUID.
	AddOpenedPorts(ctx domain.AtomicContext, portRangesByEndpointUUID network.GroupedPortRanges) error

	// RemoveOpenedPorts removes the given port ranges from the database by uuid.
	RemoveOpenedPorts(ctx domain.AtomicContext, portRangeUUIDs []string) error
}

// State describes the methods that a state implementation must provide to
// manage opened ports for units.
type State interface {
	AtomicState

	// GetUnitOpenedPorts returns the opened ports for a given unit uuid,
	// grouped by endpoint.
	GetUnitOpenedPorts(ctx context.Context, unitUUID string) (network.GroupedPortRanges, error)

	// GetMachineOpenedPorts returns the opened ports for all the units on the
	// given machine. Opened ports are grouped first by unit and then by endpoint.
	GetMachineOpenedPorts(ctx context.Context, machineUUID string) (map[string]network.GroupedPortRanges, error)

	// GetApplicationOpenedPorts returns the opened ports for all the units of the
	// given application. We return opened ports paired with the unit UUIDs, grouped
	// by endpoint.
	GetApplicationOpenedPorts(ctx context.Context, applicationUUID string) (port.UnitEndpointPortRanges, error)
}

// Service provides the API for managing the opened ports for units.
type Service struct {
	st State
}

// NewService returns a new Service providing an API to manage the opened ports
// for units.
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}

// GetUnitOpenedPorts returns the opened ports for a given unit uuid, grouped by
// endpoint.
func (s *Service) GetUnitOpenedPorts(ctx context.Context, unitUUID string) (network.GroupedPortRanges, error) {
	return s.st.GetUnitOpenedPorts(ctx, unitUUID)
}

// GetMachineOpenedPorts returns the opened ports for all the units on the machine.
// Opened ports are grouped first by unit and then by endpoint.
func (s *Service) GetMachineOpenedPorts(ctx context.Context, machineUUID string) (map[string]network.GroupedPortRanges, error) {
	return s.st.GetMachineOpenedPorts(ctx, machineUUID)
}

// GetApplicationOpenedPorts returns the opened ports for all the units of the
// application. Opened ports are grouped first by unit and then by endpoint.
func (s *Service) GetApplicationOpenedPorts(ctx context.Context, applicationUUID string) (map[string]network.GroupedPortRanges, error) {
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
func (s *Service) GetApplicationOpenedPortsByEndpoint(ctx context.Context, applicationUUID string) (network.GroupedPortRanges, error) {
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
func (s *Service) UpdateUnitPorts(ctx context.Context, unitUUID string, openPorts, closePorts network.GroupedPortRanges) error {
	if len(openPorts.UniquePortRanges())+len(closePorts.UniquePortRanges()) == 0 {
		return nil
	}

	allInputPortRanges := append(openPorts.UniquePortRanges(), closePorts.UniquePortRanges()...)
	//  verify input port ranges do not conflict with each other.
	err := verifyNoPortRangeConflicts(allInputPortRanges, allInputPortRanges)
	if err != nil {
		return errors.Errorf("cannot update unit ports with conflict(s): %w", err)
	}

	err = s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		err := s.verifyNoColocatedPortRangeConflicts(ctx, unitUUID, allInputPortRanges)
		if err != nil {
			return errors.Errorf("cannot update unit ports with conflict(s) on co-located units: %w", err)
		}

		endpoints, err := s.ensureEndpoints(ctx, unitUUID, openPorts, closePorts)
		if err != nil {
			return errors.Errorf("failed to ensure required endpoints for unit %s: %w", unitUUID, err)
		}

		currentOpenedPorts, err := s.st.GetUnitOpenedPortsUUID(ctx, unitUUID)
		if err != nil {
			return errors.Errorf("failed to get opened ports for unit %s: %w", unitUUID, err)
		}

		openPorts, closePorts, err := s.reconcileWildcard(
			ctx, unitUUID, endpoints, currentOpenedPorts[WildcardEndpoint], openPorts, closePorts,
		)
		if err != nil {
			return errors.Errorf("failed to reconcile the wildcard endpoint: %w", err)
		}

		currentPortRangesIndex := indexPortRanges(currentOpenedPorts)

		portRangesToAdd := filterPortRangesToAdd(currentPortRangesIndex, openPorts)
		if len(portRangesToAdd) > 0 {
			groupedPortRangesToAdd := groupPortRangesByEndpointUUID(endpoints, portRangesToAdd)
			err = s.st.AddOpenedPorts(ctx, groupedPortRangesToAdd)
			if err != nil {
				return errors.Errorf("failed to open ports for unit %s: %w", unitUUID, err)
			}
		}

		portRangeUUIDsToRemove := filterPortRangesToRemove(currentPortRangesIndex, closePorts)
		if len(portRangeUUIDsToRemove) > 0 {
			err = s.st.RemoveOpenedPorts(ctx, portRangeUUIDsToRemove)
			if err != nil {
				return errors.Errorf("failed to close ports for unit %s: %w", unitUUID, err)
			}
		}

		return nil
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

// verifyNoColocatedPortRangeConflicts verifies the provided port ranges do not
// conflict with the port ranges opened on units co-located with the given unit.
func (s *Service) verifyNoColocatedPortRangeConflicts(
	ctx domain.AtomicContext, unitUUID string, portRanges []network.PortRange,
) error {
	colocatedOpened, err := s.st.GetColocatedOpenedPorts(ctx, unitUUID)
	if err != nil {
		return errors.Errorf("failed to get opened ports co-located with unit %s: %w", unitUUID, err)
	}
	return verifyNoPortRangeConflicts(portRanges, colocatedOpened)
}

// ensureEndpoints ensures the given endpoints required to open and close a given
// set of ports are present for the given unit. Returns all endpoints for the unit
// after ensuring after adding any.
func (s *Service) ensureEndpoints(
	ctx domain.AtomicContext, unitUUID string, openPorts, closePorts network.GroupedPortRanges,
) ([]port.Endpoint, error) {
	currentEndpoints, err := s.st.GetEndpoints(ctx, unitUUID)
	if err != nil {
		return nil, errors.Errorf("failed to get unit endpoints: %w", err)
	}

	newEndpointNamesSet := set.NewStrings()
	for endpoint := range openPorts {
		newEndpointNamesSet.Add(endpoint)
	}
	for endpoint := range closePorts {
		newEndpointNamesSet.Add(endpoint)
	}
	for _, endpoint := range currentEndpoints {
		newEndpointNamesSet.Remove(endpoint.Endpoint)
	}
	newEndpointNames := newEndpointNamesSet.SortedValues()

	var newEndpoints []port.Endpoint
	if len(newEndpointNames) > 0 {
		var err error
		newEndpoints, err = s.st.AddEndpoints(ctx, unitUUID, newEndpointNames)
		if err != nil {
			return nil, errors.Errorf("failed to add endpoints for unit %s: %w", unitUUID, err)
		}
	}
	return append(currentEndpoints, newEndpoints...), nil
}

func (s *Service) reconcileWildcard(
	ctx domain.AtomicContext, unitUUID string, endpoints []port.Endpoint, wildcardOpened []port.PortRangeUUID, openPorts, closePorts network.GroupedPortRanges,
) (network.GroupedPortRanges, network.GroupedPortRanges, error) {
	wildcardOpen, _ := openPorts[WildcardEndpoint]
	wildcardClose, _ := closePorts[WildcardEndpoint]

	wildcardOpenedSet := map[network.PortRange]bool{}
	for _, portRange := range wildcardOpened {
		wildcardOpenedSet[portRange.PortRange] = true
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

	// If we're opening a port range on the wildcard endpoint, we need to
	// close it on all other endpoints.
	//
	// NOTE: This ensures that is a port range is open on the wildcard
	// endpoint, it is closed on all other endpoints.
	for _, openPortRange := range wildcardOpen {

		for _, endpoint := range endpoints {
			if endpoint.Endpoint == WildcardEndpoint {
				continue
			}
			delete(openPorts, endpoint.Endpoint)
			closePorts[endpoint.Endpoint] = append(closePorts[endpoint.Endpoint], openPortRange)
		}
	}

	// Close port ranges closed on the wildcard endpoint on all other endpoints.
	for _, closePortRange := range wildcardClose {

		for _, endpoint := range endpoints {
			if endpoint.Endpoint == WildcardEndpoint {
				continue
			}
			closePorts[endpoint.Endpoint] = append(closePorts[endpoint.Endpoint], closePortRange)
		}
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

	// If we're closing a port range for a specific endpoint which is open
	// on the wildcard endpoint, we need to close it on the wildcard endpoint
	// and open it on all other endpoints except the targeted endpoint.
	for _, portRange := range wildcardOpened {
		if endpoint, ok := closePortsToEndpointMap[portRange.PortRange]; ok {

			// This port range, open on the wildcard endpoint, is being closed
			// on some endpoint. We need to close it on the wildcard, and open
			// it on all endpoints other than the wildcard & targeted endpoint.
			closePorts[WildcardEndpoint] = append(closePorts[WildcardEndpoint], portRange.PortRange)

			for _, otherEndpoint := range endpoints {
				if otherEndpoint.Endpoint == WildcardEndpoint || otherEndpoint.Endpoint == endpoint {
					continue
				}
				openPorts[otherEndpoint.Endpoint] = append(openPorts[otherEndpoint.Endpoint], portRange.PortRange)
			}

			// Remove the port range from openPorts for the targeted endpoint.
			for i, otherPortRange := range openPorts[endpoint] {
				if otherPortRange == portRange.PortRange {
					openPorts[endpoint] = append(openPorts[endpoint][:i], openPorts[endpoint][i+1:]...)
					break
				}
			}
		}
	}

	return openPorts, closePorts, nil
}

func filterPortRangesToAdd(current portRangeUUIDIndex, toAdd network.GroupedPortRanges) network.GroupedPortRanges {
	filtered := network.GroupedPortRanges{}
	for endpoint, portRanges := range toAdd {
		for _, portRange := range portRanges {
			if _, ok := current[endpoint][portRange]; !ok {
				filtered[endpoint] = append(filtered[endpoint], portRange)
			}
		}
	}
	return filtered
}

func groupPortRangesByEndpointUUID(endpoints []port.Endpoint, gpr network.GroupedPortRanges) network.GroupedPortRanges {
	epUUIDIndex := map[string]string{}
	for _, endpoint := range endpoints {
		epUUIDIndex[endpoint.Endpoint] = endpoint.UUID
	}

	byEndpointUUID := network.GroupedPortRanges{}
	for endpoint, portRanges := range gpr {
		byEndpointUUID[epUUIDIndex[endpoint]] = portRanges
	}

	return byEndpointUUID
}

func filterPortRangesToRemove(current portRangeUUIDIndex, toRemove network.GroupedPortRanges) []string {
	var uuidsToRemove []string
	for endpoint, portRanges := range toRemove {
		for _, portRange := range portRanges {
			if uuid, ok := current[endpoint][portRange]; ok {
				uuidsToRemove = append(uuidsToRemove, uuid)
			}
		}
	}
	sort.Strings(uuidsToRemove)
	return uuidsToRemove
}
