// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
)

type portRangeChangeRecorder struct {
	machinePortRanges, appPortRanges map[names.UnitTag]network.GroupedPortRanges

	// The tag of the unit that the following pending open/close ranges apply to.
	unitTag            names.UnitTag
	modelType          model.ModelType
	pendingOpenRanges  network.GroupedPortRanges
	pendingCloseRanges network.GroupedPortRanges
	logger             loggo.Logger
}

func newPortRangeChangeRecorder(
	logger loggo.Logger, unit names.UnitTag,
	modelType model.ModelType,
	machinePortRanges, appPortRanges map[names.UnitTag]network.GroupedPortRanges,
) *portRangeChangeRecorder {
	return &portRangeChangeRecorder{
		logger:            logger,
		unitTag:           unit,
		modelType:         modelType,
		appPortRanges:     appPortRanges,
		machinePortRanges: machinePortRanges,
	}
}

func (r *portRangeChangeRecorder) validatePortRangeForCAAS(portRange network.PortRange) error {
	if r.modelType == model.IAAS || portRange.FromPort == portRange.ToPort {
		return nil
	}
	return errors.NewNotSupported(nil, "port ranges are not supported for k8s applications, please specify a single port")
}

// OpenPortRange registers a request to open the specified port range for the
// provided endpoint name.
func (r *portRangeChangeRecorder) OpenPortRange(endpointName string, portRange network.PortRange) error {
	if err := portRange.Validate(); err != nil {
		return errors.Trace(err)
	}
	if err := r.validatePortRangeForCAAS(portRange); err != nil {
		return errors.Trace(err)
	}

	// If a close request is pending for this port, remove it.
	for i, pr := range r.pendingCloseRanges[endpointName] {
		if pr == portRange {
			r.logger.Tracef("open-port %q and cancel the pending close-port", portRange)
			r.pendingCloseRanges[endpointName] = append(r.pendingCloseRanges[endpointName][:i], r.pendingCloseRanges[endpointName][i+1:]...)
			break
		}
	}

	// Ensure port range does not conflict with the ones already recorded
	// for opening by this unit.
	if err := r.checkForConflict(endpointName, portRange, r.unitTag, r.pendingOpenRanges, true); err != nil {
		if !errors.Is(err, errors.AlreadyExists) {
			return errors.Annotatef(err, "cannot open %v (unit %q)", portRange, r.unitTag.Id())
		}

		// Already exists; this is a no-op.
		return nil
	}

	// Ensure port range does not conflict with existing open port ranges
	// for all units deployed to the machine.
	for otherUnitTag, otherUnitRanges := range r.machinePortRanges {
		if err := r.checkForConflict(endpointName, portRange, otherUnitTag, otherUnitRanges, false); err != nil {
			if !errors.Is(err, errors.AlreadyExists) {
				return errors.Annotatef(err, "cannot open %v (unit %q)", portRange, r.unitTag.Id())
			}
			// Already exists; this is a no-op.
			return nil
		}
	}
	if err := r.checkAppPortRanges(endpointName, portRange); err != nil {
		if !errors.Is(err, errors.AlreadyExists) {
			return errors.Annotatef(err, "cannot open %v (unit %q)", portRange, r.unitTag.Id())
		}
		// Already exists; this is a no-op.
		return nil
	}

	if r.pendingOpenRanges == nil {
		r.pendingOpenRanges = make(network.GroupedPortRanges)
	}
	r.pendingOpenRanges[endpointName] = append(r.pendingOpenRanges[endpointName], portRange)
	return nil
}

func (r *portRangeChangeRecorder) checkAppPortRanges(endpointName string, portRange network.PortRange) error {
	for otherUnitTag, otherUnitRanges := range r.appPortRanges {
		if r.unitTag != otherUnitTag {
			continue
		}
		for existingEndpoint, otherEndpointRanges := range otherUnitRanges {
			if endpointName != existingEndpoint {
				continue
			}
			for _, otherPortRange := range otherEndpointRanges {
				if portRange == otherPortRange {
					// Already exists; this is a no-op.
					return errors.AlreadyExistsf("%v (endpoint %q)", portRange, endpointName)
				}
			}
		}
	}
	return nil
}

// ClosePortRange registers a request to close the specified port range for the
// provided endpoint name. If the machine has no ports open yet, this is a no-op.
func (r *portRangeChangeRecorder) ClosePortRange(endpointName string, portRange network.PortRange) error {
	if err := portRange.Validate(); err != nil {
		return errors.Trace(err)
	}
	if err := r.validatePortRangeForCAAS(portRange); err != nil {
		return errors.Trace(err)
	}

	// If an open request is pending for this port, remove it.
	for i, pr := range r.pendingOpenRanges[endpointName] {
		r.logger.Tracef("closing port %q for endpoint %q, so cancel the pending opening port", portRange, endpointName)
		if pr == portRange {
			r.pendingOpenRanges[endpointName] = append(r.pendingOpenRanges[endpointName][:i], r.pendingOpenRanges[endpointName][i+1:]...)
			break
		}
	}

	// Ensure port range does not conflict with the ones already recorded
	// for closing by this unit.
	if err := r.checkForConflict(endpointName, portRange, r.unitTag, r.pendingCloseRanges, true); err != nil {
		if !errors.IsAlreadyExists(err) {
			return errors.Annotatef(err, "cannot close %v (unit %q)", portRange, r.unitTag.Id())
		}

		// Already exists; this is a no-op.
		return nil
	}

	// The port range should be accepted for closing if:
	// - it exactly matches a an already open port for this unit, or
	// - it doesn't conflict with any open port range; this could be either
	//   because it matches an existing port range for this unit but the endpoints
	//   do not match (e.g. open X for all endpoints, close X for endpoint "foo")
	//   or because the port range is not open in which case this will be a
	//   no-op and filtered out by the controller.
	for otherUnitTag, otherUnitRanges := range r.machinePortRanges {
		if err := r.checkForConflict(endpointName, portRange, otherUnitTag, otherUnitRanges, false); err != nil {
			// Conflicts with an open port range for another unit.
			if !errors.IsAlreadyExists(err) {
				return errors.Annotatef(err, "cannot close %v (unit %q)", portRange, r.unitTag.Id())
			}
		}
	}

	// If it has no open ports then this is a no-op.
	if len(r.machinePortRanges)+len(r.appPortRanges) == 0 {
		return nil
	}

	if r.pendingCloseRanges == nil {
		r.pendingCloseRanges = make(network.GroupedPortRanges)
	}
	r.pendingCloseRanges[endpointName] = append(r.pendingCloseRanges[endpointName], portRange)
	return nil
}

// checkForConflict ensures the opening incomingPortRange for the current unit
// does not conflict with the set of port ranges for another unit. If otherUnit
// matches the current unit and incomingPortRange already exists in the known
// port ranges, the method returns an AlreadyExists error.
func (r *portRangeChangeRecorder) checkForConflict(incomingEndpoint string, incomingPortRange network.PortRange, otherUnitTag names.UnitTag, otherUnitRanges network.GroupedPortRanges, checkingAgainstPending bool) error {
	for existingEndpoint, existingPortRanges := range otherUnitRanges {
		for _, existingPortRange := range existingPortRanges {
			if !incomingPortRange.ConflictsWith(existingPortRange) {
				continue
			}

			// If these are different units then this is definitely a conflict.
			if r.unitTag != otherUnitTag {
				var extraDetails string
				if checkingAgainstPending {
					extraDetails = " requested earlier"
				}
				return errors.Errorf("port range conflicts with %v (unit %q)%s", existingPortRange, otherUnitTag.Id(), extraDetails)
			} else if incomingPortRange == existingPortRange {
				// Same unit and port range. If the endpoints
				// do not match then this is a legal change.
				// (e.g. open X for endpoint foo and then open
				// X for endpoint bar).
				if incomingEndpoint != existingEndpoint {
					continue
				}

				return errors.AlreadyExistsf("%v (endpoint %q)", incomingPortRange, incomingEndpoint)
			}

			var extraDetails string
			if checkingAgainstPending {
				extraDetails = " requested earlier"
			}
			return errors.Errorf("port range conflicts with %v (unit %q)%s", existingPortRange, otherUnitTag.Id(), extraDetails)
		}
	}

	// No conflict
	return nil
}

// OpenedUnitPortRanges returns the set of port ranges currently open by the
// current unit grouped by endpoint.
func (r *portRangeChangeRecorder) OpenedUnitPortRanges() network.GroupedPortRanges {
	if len(r.machinePortRanges[r.unitTag]) > 0 {
		return r.mergeWithPendingChanges(r.machinePortRanges[r.unitTag])
	}
	return r.mergeWithPendingChanges(r.appPortRanges[r.unitTag])
}

// PendingChanges returns the set of recorded open/close port range requests
// (grouped by endpoint name) for the current unit.
func (r *portRangeChangeRecorder) PendingChanges() (network.GroupedPortRanges, network.GroupedPortRanges) {
	return r.pendingOpenRanges, r.pendingCloseRanges
}

// mergeWithPendingChanges takes the input changes and merges them with the
// pending open and close changes.
func (r *portRangeChangeRecorder) mergeWithPendingChanges(portRanges network.GroupedPortRanges) network.GroupedPortRanges {

	resultingChanges := make(network.GroupedPortRanges)
	for group, ranges := range portRanges {
		resultingChanges[group] = append(resultingChanges[group], ranges...)
	}

	// Add the pending open changes
	resultingChanges.MergePendingOpenPortRanges(r.pendingOpenRanges)
	// Remove the pending close changes
	resultingChanges.MergePendingClosePortRanges(r.pendingCloseRanges)

	return resultingChanges
}
