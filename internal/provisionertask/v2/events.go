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
	EventLifeChanged MachineEventType = "life changed"

	// EventZoneAssigned indicates the AZ Coordinator assigned a zone.
	EventZoneAssigned MachineEventType = "zone assigned"

	// EventZoneRequestFailed indicates the zone request could not be fulfilled.
	EventZoneRequestFailed MachineEventType = "zone request failed"
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

// WorkerMessageType represents the type of message sent from workers to the main loop.
type WorkerMessageType string

const (
	// MessageRequestZone asks the AZ Coordinator for a zone assignment.
	MessageRequestZone WorkerMessageType = "request zone"

	// MessageProvisionComplete notifies that provisioning finished.
	MessageProvisionComplete WorkerMessageType = "provision complete"
)

// String returns a human-readable representation of the message type.
func (t WorkerMessageType) String() string {
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
	Error      error
}

// WorkerMessage represents messages from machine workers to the main loop.
type WorkerMessage struct {
	Type      WorkerMessageType
	MachineID string
	// Payload contains the message-specific data.
	// For MessageRequestZone: ZoneRequestPayload.
	// For MessageProvisionComplete: ProvisionResultPayload.
	Payload any
}

// NewZoneRequestMessage creates a new WorkerMessage for requesting a zone.
func NewZoneRequestMessage(machineID string, distGroup []string, constraints []string) WorkerMessage {
	return WorkerMessage{
		Type:      MessageRequestZone,
		MachineID: machineID,
		Payload: ZoneRequestPayload{
			MachineID:         machineID,
			DistributionGroup: distGroup,
			Constraints:       constraints,
		},
	}
}

// NewProvisionCompleteMessage creates a new WorkerMessage for reporting provision completion.
func NewProvisionCompleteMessage(machineID, instanceID, zoneName string, err error) WorkerMessage {
	return WorkerMessage{
		Type:      MessageProvisionComplete,
		MachineID: machineID,
		Payload: ProvisionResultPayload{
			MachineID:  machineID,
			InstanceID: instanceID,
			ZoneName:   zoneName,
			Error:      err,
		},
	}
}
