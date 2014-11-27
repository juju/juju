// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
)

// PortRangeInfo contains information about a pending open- or
// close-port operation for a port range. This is only exported for
// testing.
type PortRangeInfo struct {
	ShouldOpen  bool
	RelationTag names.RelationTag
}

// PortRange contains a port range and a relation id. Used as key to
// pendingRelations and is only exported for testing.
type PortRange struct {
	Ports      network.PortRange
	RelationId int
}

func validatePortRange(protocol string, fromPort, toPort int) (network.PortRange, error) {
	// Validate the given range.
	newRange := network.PortRange{
		Protocol: strings.ToLower(protocol),
		FromPort: fromPort,
		ToPort:   toPort,
	}
	if err := newRange.Validate(); err != nil {
		return network.PortRange{}, err
	}
	return newRange, nil
}

func tryOpenPorts(
	protocol string,
	fromPort, toPort int,
	unitTag names.UnitTag,
	machinePorts map[network.PortRange]params.RelationUnit,
	pendingPorts map[PortRange]PortRangeInfo,
) error {
	// TODO(dimitern) Once port ranges are linked to relations in
	// addition to networks, refactor this functions and test it
	// better to ensure it handles relations properly.
	relationId := -1

	//Validate the given range.
	newRange, err := validatePortRange(protocol, fromPort, toPort)
	if err != nil {
		return err
	}
	rangeKey := PortRange{
		Ports:      newRange,
		RelationId: relationId,
	}

	rangeInfo, isKnown := pendingPorts[rangeKey]
	if isKnown {
		if !rangeInfo.ShouldOpen {
			// If the same range is already pending to be closed, just
			// mark is pending to be opened.
			rangeInfo.ShouldOpen = true
			pendingPorts[rangeKey] = rangeInfo
		}
		return nil
	}

	// Ensure there are no conflicts with existing ports on the
	// machine.
	for portRange, relUnit := range machinePorts {
		relUnitTag, err := names.ParseUnitTag(relUnit.Unit)
		if err != nil {
			return errors.Annotatef(
				err,
				"machine ports %v contain invalid unit tag",
				portRange,
			)
		}
		if newRange.ConflictsWith(portRange) {
			if portRange == newRange && relUnitTag == unitTag {
				// The same unit trying to open the same range is just
				// ignored.
				return nil
			}
			return errors.Errorf(
				"cannot open %v (unit %q): conflicts with existing %v (unit %q)",
				newRange, unitTag.Id(), portRange, relUnitTag.Id(),
			)
		}
	}
	// Ensure other pending port ranges do not conflict with this one.
	for rangeKey, rangeInfo := range pendingPorts {
		if newRange.ConflictsWith(rangeKey.Ports) && rangeInfo.ShouldOpen {
			return errors.Errorf(
				"cannot open %v (unit %q): conflicts with %v requested earlier",
				newRange, unitTag.Id(), rangeKey.Ports,
			)
		}
	}

	rangeInfo = pendingPorts[rangeKey]
	rangeInfo.ShouldOpen = true
	pendingPorts[rangeKey] = rangeInfo
	return nil
}

func tryClosePorts(
	protocol string,
	fromPort, toPort int,
	unitTag names.UnitTag,
	machinePorts map[network.PortRange]params.RelationUnit,
	pendingPorts map[PortRange]PortRangeInfo,
) error {
	// TODO(dimitern) Once port ranges are linked to relations in
	// addition to networks, refactor this functions and test it
	// better to ensure it handles relations properly.
	relationId := -1

	// Validate the given range.
	newRange, err := validatePortRange(protocol, fromPort, toPort)
	if err != nil {
		return err
	}
	rangeKey := PortRange{
		Ports:      newRange,
		RelationId: relationId,
	}

	rangeInfo, isKnown := pendingPorts[rangeKey]
	if isKnown {
		if rangeInfo.ShouldOpen {
			// If the same range is already pending to be opened, just
			// remove it from pending.
			delete(pendingPorts, rangeKey)
		}
		return nil
	}

	// Ensure the range we're trying to close is opened on the
	// machine.
	relUnit, found := machinePorts[newRange]
	if !found {
		// Trying to close a range which is not open is ignored.
		return nil
	} else if relUnit.Unit != unitTag.String() {
		relUnitTag, err := names.ParseUnitTag(relUnit.Unit)
		if err != nil {
			return errors.Annotatef(
				err,
				"machine ports %v contain invalid unit tag",
				newRange,
			)
		}
		return errors.Errorf(
			"cannot close %v (opened by %q) from %q",
			newRange, relUnitTag.Id(), unitTag.Id(),
		)
	}

	rangeInfo = pendingPorts[rangeKey]
	rangeInfo.ShouldOpen = false
	pendingPorts[rangeKey] = rangeInfo
	return nil
}
