// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisionertask

import (
	"github.com/juju/juju/core/life"
)

// MachineEventType represents the type of event sent from the main loop to workers.
type MachineEventType string

const (
	// EventLifeChanged indicates the machine's life value changed.
	EventLifeChanged MachineEventType = "LifeChanged"

	// EventZoneAssigned indicates the AZ Coordinator assigned a zone.
	EventZoneAssigned MachineEventType = "ZoneAssigned"

	// EventZoneRequestFailed indicates the zone request could not be fulfilled.
	EventZoneRequestFailed MachineEventType = "ZoneRequestFailed"
)

// String returns a human-readable representation of the event type.
func (t MachineEventType) String() string {
	return string(t)
}

// MachineEvent represents all events that can be sent to a machine worker.
type MachineEvent struct {
	Type      MachineEventType
	Life      life.Value // For LifeChanged events.
	Zone      string     // For ZoneAssigned events (zone name).
	ZoneError error      // For ZoneRequestFailed events.
}

// WorkerRequestType represents the type of request sent from workers to the main loop.
type WorkerRequestType string

const (
	// RequestZone asks the AZ Coordinator for a zone assignment.
	RequestZone WorkerRequestType = "RequestZone"

	// RequestProvisionComplete notifies that provisioning finished.
	RequestProvisionComplete WorkerRequestType = "RequestProvisionComplete"

	// RequestCancelZone cancels a pending zone request.
	RequestCancelZone WorkerRequestType = "RequestCancelZone"
)

// String returns a human-readable representation of the request type.
func (t WorkerRequestType) String() string {
	return string(t)
}

// ZoneRequestPayload contains parameters for requesting a zone assignment.
type ZoneRequestPayload struct {
	MachineID         string
	DistributionGroup []string // Machine IDs in same distribution group.
	Constraints       []string // Zone constraints from provisioning info.
}

// ProvisionResultPayload reports the outcome of a provisioning attempt.
type ProvisionResultPayload struct {
	MachineID  string
	InstanceID string
	ZoneName   string
	Success    bool
	Error      error // Only set if Success is false.
}

// WorkerRequest represents requests from machine workers to the main loop.
type WorkerRequest struct {
	Type      WorkerRequestType
	MachineID string
	// Payload contains the request-specific data.
	// For RequestZone: ZoneRequestPayload.
	// For RequestProvisionComplete: ProvisionResultPayload.
	// For RequestCancelZone: nil (MachineID is sufficient).
	Payload any
}

// NewZoneRequest creates a new WorkerRequest for requesting a zone.
func NewZoneRequest(machineID string, distGroup []string, constraints []string) WorkerRequest {
	return WorkerRequest{
		Type:      RequestZone,
		MachineID: machineID,
		Payload: ZoneRequestPayload{
			MachineID:         machineID,
			DistributionGroup: distGroup,
			Constraints:       constraints,
		},
	}
}

// NewProvisionCompleteRequest creates a new WorkerRequest for reporting provision completion.
func NewProvisionCompleteRequest(machineID, instanceID, zoneName string, success bool, err error) WorkerRequest {
	return WorkerRequest{
		Type:      RequestProvisionComplete,
		MachineID: machineID,
		Payload: ProvisionResultPayload{
			MachineID:  machineID,
			InstanceID: instanceID,
			ZoneName:   zoneName,
			Success:    success,
			Error:      err,
		},
	}
}

// NewCancelZoneRequest creates a new WorkerRequest for canceling a zone request.
func NewCancelZoneRequest(machineID string) WorkerRequest {
	return WorkerRequest{
		Type:      RequestCancelZone,
		MachineID: machineID,
	}
}
