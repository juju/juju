// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"sort"

	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/port"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
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
	GetColocatedOpenedPorts(ctx domain.AtomicContext, unitUUID unit.UUID) ([]network.PortRange, error)

	// GetUnitOpenedPortsWithUUIDs returns the opened ports for the given unit with the
	// UUID of the port range. The opened ports are grouped by endpoint.
	GetUnitOpenedPortsWithUUIDs(ctx domain.AtomicContext, unitUUID unit.UUID) (map[string][]port.PortRangeWithUUID, error)

	// GetEndpoints returns all endpoints for a given unit.
	GetEndpoints(ctx domain.AtomicContext, unitUUID unit.UUID) ([]port.Endpoint, error)

	// AddEndpoints adds the endpoints to a given unit. Return the added endpoints
	// with their corresponding UUIDs.
	AddEndpoints(ctx domain.AtomicContext, unitUUID unit.UUID, endpoints []string) ([]port.Endpoint, error)

	// AddOpenedPorts adds the given port ranges to the database. Port ranges must
	// be grouped by endpoint UUID.
	AddOpenedPorts(ctx domain.AtomicContext, portRangesByEndpointUUID map[port.UUID][]network.PortRange) error

	// RemoveOpenedPorts removes the given port ranges from the database by uuid.
	RemoveOpenedPorts(ctx domain.AtomicContext, portRangeUUIDs []port.UUID) error
}

// State describes the methods that a state implementation must provide to
// manage opened ports for units.
type State interface {
	AtomicState

	// GetUnitOpenedPorts returns the opened ports for a given unit uuid,
	// grouped by endpoint.
	GetUnitOpenedPorts(ctx context.Context, unitUUID unit.UUID) (network.GroupedPortRanges, error)

	// GetMachineOpenedPorts returns the opened ports for all the units on the
	// given machine. Opened ports are grouped first by unit and then by endpoint.
	GetMachineOpenedPorts(ctx context.Context, machineUUID string) (map[unit.UUID]network.GroupedPortRanges, error)

	// GetApplicationOpenedPorts returns the opened ports for all the units of the
	// given application. We return opened ports paired with the unit UUIDs, grouped
	// by endpoint.
	GetApplicationOpenedPorts(ctx context.Context, applicationUUID application.ID) (port.UnitEndpointPortRanges, error)
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
func (s *Service) GetUnitOpenedPorts(ctx context.Context, unitUUID unit.UUID) (network.GroupedPortRanges, error) {
	if err := unitUUID.Validate(); err != nil {
		return nil, err
	}
	return s.st.GetUnitOpenedPorts(ctx, unitUUID)
}

// GetMachineOpenedPorts returns the opened ports for all the units on the machine.
// Opened ports are grouped first by unit and then by endpoint.
//
// TODO: Use a machine.UUID type when one exists.
func (s *Service) GetMachineOpenedPorts(ctx context.Context, machineUUID string) (map[unit.UUID]network.GroupedPortRanges, error) {
	if !uuid.IsValidUUIDString(machineUUID) {
		return nil, errors.Errorf("uuid %q not valid", machineUUID)
	}
	return s.st.GetMachineOpenedPorts(ctx, machineUUID)
}

// GetApplicationOpenedPorts returns the opened ports for all the units of the
// application. Opened ports are grouped first by unit and then by endpoint.
func (s *Service) GetApplicationOpenedPorts(ctx context.Context, applicationUUID application.ID) (map[unit.UUID]network.GroupedPortRanges, error) {
	if err := applicationUUID.Validate(); err != nil {
		return nil, err
	}
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
func (s *Service) GetApplicationOpenedPortsByEndpoint(ctx context.Context, applicationUUID application.ID) (network.GroupedPortRanges, error) {
	if err := applicationUUID.Validate(); err != nil {
		return nil, err
	}
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
func (s *Service) UpdateUnitPorts(ctx context.Context, unitUUID unit.UUID, openPorts, closePorts network.GroupedPortRanges) error {
	if err := unitUUID.Validate(); err != nil {
		return err
	}
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

		// Ensure that all required endpoints are present for the unit and get
		// their names with uuids.
		existingEndpoints, err := s.st.GetEndpoints(ctx, unitUUID)
		if err != nil {
			return errors.Errorf("failed to get unit endpoints: %w", err)
		}
		newEndpoints, err := s.addMissingEndpoints(ctx, unitUUID, existingEndpoints, openPorts, closePorts)
		if err != nil {
			return errors.Errorf("failed to ensure required endpoints for unit %s: %w", unitUUID, err)
		}
		endpoints := append(existingEndpoints, newEndpoints...)

		currentOpenedPorts, err := s.st.GetUnitOpenedPortsWithUUIDs(ctx, unitUUID)
		if err != nil {
			return errors.Errorf("failed to get opened ports for unit %s: %w", unitUUID, err)
		}

		openedOnWildcard := transform.Slice(currentOpenedPorts[WildcardEndpoint], func(pr port.PortRangeWithUUID) network.PortRange {
			return pr.PortRange
		})
		openPorts, closePorts, err := s.reconcileWildcard(
			endpoints, openedOnWildcard, openPorts, closePorts,
		)
		if err != nil {
			return errors.Errorf("failed to reconcile the wildcard endpoint: %w", err)
		}

		currentPortRangesIndex := indexPortRanges(currentOpenedPorts)

		portRangesToAdd := filterOutAlreadyOpenRanges(currentPortRangesIndex, openPorts)
		if len(portRangesToAdd) > 0 {
			groupedPortRangesToAdd := groupPortRangesByEndpointUUID(endpoints, portRangesToAdd)
			err = s.st.AddOpenedPorts(ctx, groupedPortRangesToAdd)
			if err != nil {
				return errors.Errorf("failed to open ports for unit %s: %w", unitUUID, err)
			}
		}

		portRangeUUIDsToRemove := findPortRangeUUIDsToClose(currentPortRangesIndex, closePorts)
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
	ctx domain.AtomicContext, unitUUID unit.UUID, portRanges []network.PortRange,
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
func (s *Service) addMissingEndpoints(
	ctx domain.AtomicContext, unitUUID unit.UUID, currentEndpoints []port.Endpoint, openPorts, closePorts network.GroupedPortRanges,
) ([]port.Endpoint, error) {

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

	newEndpoints := []port.Endpoint{}
	if len(newEndpointNames) > 0 {
		var err error
		newEndpoints, err = s.st.AddEndpoints(ctx, unitUUID, newEndpointNames)
		if err != nil {
			return nil, errors.Errorf("failed to add endpoints for unit %s: %w", unitUUID, err)
		}
	}
	return newEndpoints, nil
}

// reconcileWildcard reconciles the open and close port ranges for the wildcard
// endpoint with the open and close port ranges for all other endpoints, returning
// a new collection of openPorts and closePorts.
//
// There is a special wildcard endpoint "" that represents all endpoints.
// Any operations applied to the wildcard endpoint will logically applied to all
// endpoints.
//
// That is, if we open a port range on the wildcard endpoint, we will open it as
// usual. But as a side effect, we ensure that endpoint is not open on any other
// endpoint.
//
// On the other hand, if we close a port range on the wildcard endpoint, we will
// close it on all other endpoints.
//
// Also, if we close a specific endpoint's port range that is open
// on the wildcard endpoint, we will close it on the wildcard endpoint and open
// it on all other endpoints except the targeted endpoint.
//
// This means:
//   - Dropping ranges already open on the wildcard endpoint from openPorts for
//     any endpoint it is present.
//   - Dropping ranges we are opening on the wildcard endpoint from openPorts for
//     all other endpoints.
//   - Adding that range to closePorts for all endpoints (later we we will search
//     current state to find range uuids to be removed).
func (s *Service) reconcileWildcard(
	endpoints []port.Endpoint, openedOnWildcard []network.PortRange, openPortsIn, closePortsIn network.GroupedPortRanges,
) (network.GroupedPortRanges, network.GroupedPortRanges, error) {
	openedOnWildcardSet := map[network.PortRange]bool{}
	for _, portRange := range openedOnWildcard {
		openedOnWildcardSet[portRange] = true
	}
	wildcardOpeningSet := map[network.PortRange]bool{}
	for _, portRange := range openPortsIn[WildcardEndpoint] {
		wildcardOpeningSet[portRange] = true
	}

	// Construct our openPortsOut return value. And filter out port ranges:
	//  - Already open on the wildcard endpoint.
	//  - Being opened on the wildcard endpoint.
	openPortsOut := network.GroupedPortRanges{}
	for endpoint, endpointOpenPorts := range openPortsIn {
		// Leave the wildcard endpoint as is.
		if endpoint == WildcardEndpoint {
			openPortsOut[endpoint] = endpointOpenPorts
			continue
		}
		for _, portRange := range endpointOpenPorts {
			if _, ok := openedOnWildcardSet[portRange]; ok {
				continue
			}
			if _, ok := wildcardOpeningSet[portRange]; ok {
				continue
			}
			openPortsOut[endpoint] = append(openPortsOut[endpoint], portRange)
		}
	}

	closePortsOut := closePortsIn.Clone()

	// If we're opening a port range on the wildcard endpoint, we need to
	// close it on all other endpoints.
	for wildcardOpeningPortRange := range wildcardOpeningSet {
		for _, endpoint := range endpoints {
			// Leave the wildcard endpoint as is.
			if endpoint.Endpoint == WildcardEndpoint {
				continue
			}
			closePortsOut[endpoint.Endpoint] = append(closePortsOut[endpoint.Endpoint], wildcardOpeningPortRange)
		}
	}

	// Close port ranges closed on the wildcard endpoint on all other endpoints.
	wildcardClosing, _ := closePortsIn[WildcardEndpoint]
	for _, wildcardClosingPortRange := range wildcardClosing {
		// Leave the wildcard endpoint as is.
		for _, endpoint := range endpoints {
			if endpoint.Endpoint == WildcardEndpoint {
				continue
			}
			closePortsOut[endpoint.Endpoint] = append(closePortsOut[endpoint.Endpoint], wildcardClosingPortRange)
		}
	}

	// construct a map of all port ranges being closed to the endpoint they're
	// being closed on. Except the wildcard endpoint.
	closePortsToEndpointMap := make(map[network.PortRange]string)
	for endpoint, endpointClosePorts := range closePortsOut {
		for _, portRange := range endpointClosePorts {
			closePortsToEndpointMap[portRange] = endpoint
		}
	}
	// Ensure endpoints closed on the wildcard endpoint are not in the map.
	for _, wildcardClosePorts := range closePortsOut[WildcardEndpoint] {
		delete(closePortsToEndpointMap, wildcardClosePorts)
	}

	// If we're closing a port range for a specific endpoint which is open
	// on the wildcard endpoint, we need to close it on the wildcard endpoint
	// and open it on all other endpoints except the targeted endpoint.
	for _, portRange := range openedOnWildcard {
		if endpoint, ok := closePortsToEndpointMap[portRange]; ok {

			// This port range, open on the wildcard endpoint, is being closed
			// on some endpoint. We need to close it on the wildcard, and open
			// it on all endpoints other than the wildcard & targeted endpoint.
			closePortsOut[WildcardEndpoint] = append(closePortsOut[WildcardEndpoint], portRange)

			for _, otherEndpoint := range endpoints {
				if otherEndpoint.Endpoint == WildcardEndpoint || otherEndpoint.Endpoint == endpoint {
					continue
				}
				openPortsOut[otherEndpoint.Endpoint] = append(openPortsOut[otherEndpoint.Endpoint], portRange)
			}

			// Remove the port range from openPorts for the targeted endpoint.
			for i, otherPortRange := range openPortsOut[endpoint] {
				if otherPortRange == portRange {
					openPortsOut[endpoint] = append(openPortsOut[endpoint][:i], openPortsOut[endpoint][i+1:]...)
					break
				}
			}
		}
	}

	return openPortsOut, closePortsOut, nil
}

func filterOutAlreadyOpenRanges(current portRangeUUIDIndex, toAdd network.GroupedPortRanges) network.GroupedPortRanges {
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

func groupPortRangesByEndpointUUID(endpoints []port.Endpoint, gpr network.GroupedPortRanges) map[port.UUID][]network.PortRange {
	epUUIDIndex := map[string]port.UUID{}
	for _, endpoint := range endpoints {
		epUUIDIndex[endpoint.Endpoint] = endpoint.UUID
	}

	byEndpointUUID := map[port.UUID][]network.PortRange{}
	for endpoint, portRanges := range gpr {
		byEndpointUUID[epUUIDIndex[endpoint]] = portRanges
	}

	return byEndpointUUID
}

func findPortRangeUUIDsToClose(current portRangeUUIDIndex, toRemove network.GroupedPortRanges) []port.UUID {
	var uuidsToRemove []port.UUID
	for endpoint, portRanges := range toRemove {
		for _, portRange := range portRanges {
			if uuid, ok := current[endpoint][portRange]; ok {
				uuidsToRemove = append(uuidsToRemove, uuid)
			}
		}
	}
	sort.Slice(uuidsToRemove, func(i, j int) bool {
		return uuidsToRemove[i] < uuidsToRemove[j]
	})
	return uuidsToRemove
}
