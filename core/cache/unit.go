// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"time"

	"github.com/juju/pubsub"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
)

// Unit represents a unit in a cached model.
type Unit struct {
	// Resident identifies the unit as a type-agnostic cached entity
	// and tracks resources that it is responsible for cleaning up.
	*Resident

	metrics *ControllerGauges
	hub     *pubsub.SimpleHub

	details    UnitChange
	configHash string
}

func newUnit(metrics *ControllerGauges, hub *pubsub.SimpleHub, res *Resident) *Unit {
	u := &Unit{
		Resident: res,
		metrics:  metrics,
		hub:      hub,
	}
	return u
}

// Note that these property accessors are not lock-protected.
// They are intended for calling from external packages that have retrieved a
// deep copy from the cache.

// Name returns the name of this unit.
func (u *Unit) Name() string {
	return u.details.Name
}

// Application returns the application name of this unit.
func (u *Unit) Application() string {
	return u.details.Application
}

// MachineId returns the ID of the machine hosting this unit.
func (u *Unit) MachineId() string {
	return u.details.MachineId
}

// Subordinate returns a bool indicating whether this unit is a subordinate.
func (u *Unit) Subordinate() bool {
	return u.details.Subordinate
}

// Principal returns the name of the principal unit for the same application.
func (u *Unit) Principal() string {
	return u.details.Principal
}

// CharmURL returns the charm URL for this unit's application.
func (u *Unit) CharmURL() string {
	return u.details.CharmURL
}

// Ports returns the exposed ports for the unit.
func (u *Unit) Ports() []network.Port {
	return u.details.Ports
}

func (u *Unit) setDetails(details UnitChange) {
	// If this is the first receipt of details, set the removal message.
	if u.removalMessage == nil {
		u.removalMessage = RemoveUnit{
			ModelUUID: details.ModelUUID,
			Name:      details.Name,
		}
	}

	u.setStale(false)

	machineChange := u.details.MachineId != details.MachineId
	u.details = details
	if machineChange || u.details.Subordinate {
		u.hub.Publish(u.modelTopic(modelUnitAdd), u)
	}
}

// copy returns a copy of the unit, ensuring appropriate deep copying.
func (u *Unit) copy() Unit {
	var cPorts []network.Port
	uPorts := u.details.Ports
	if uPorts != nil {
		cPorts = make([]network.Port, len(uPorts))
		for i, p := range uPorts {
			cPorts[i] = p
		}
	}

	var cPortRanges []network.PortRange
	uPortRanges := u.details.PortRanges
	if uPortRanges != nil {
		cPortRanges = make([]network.PortRange, len(uPortRanges))
		for i, p := range uPortRanges {
			cPortRanges[i] = p
		}
	}

	cu := *u
	cu.details.Ports = cPorts
	cu.details.PortRanges = cPortRanges
	cu.details.WorkloadStatus = copyStatusInfo(u.details.WorkloadStatus)
	cu.details.AgentStatus = copyStatusInfo(u.details.AgentStatus)
	return cu
}

func (u *Unit) modelTopic(suffix string) string {
	return modelTopic(u.details.ModelUUID, suffix)
}

func copyStatusInfo(info status.StatusInfo) status.StatusInfo {
	var cData map[string]interface{}
	iData := info.Data
	if iData != nil {
		cData = make(map[string]interface{}, len(iData))
		for i, d := range iData {
			cData[i] = d
		}
	}

	var cSince *time.Time
	if info.Since != nil {
		s := *info.Since
		cSince = &s
	}

	return status.StatusInfo{
		Status:  info.Status,
		Message: info.Message,
		Data:    cData,
		Since:   cSince,
	}
}
