// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

// IsMachineViable returns true if the machine is started.
func IsMachineViable(status StatusInfo) bool {
	// This traps the known machine status codes, but if the status isn't
	// recognised, we assume the machine is not viable.
	switch status.Status {
	case Started:
		return true
	case Pending, Down, Stopped, Error, Unknown:
		return false
	default:
		return false
	}
}

// IsInstanceViable returns true if the instance is running.
func IsInstanceViable(status StatusInfo) bool {
	// This traps the known instance status codes, but if the status isn't
	// recognised, we assume the instance is not viable.
	switch status.Status {
	case Running:
		return true
	case Empty, Allocating, Error, ProvisioningError, Unknown:
		return false
	default:
		return false
	}
}

// IsAgentViable returns true if the agent is idle or executing.
func IsAgentViable(status StatusInfo) bool {
	// This traps the known agent status codes, but if the status isn't
	// recognised, we assume the agent is not viable.
	switch status.Status {
	case Idle, Executing:
		return true
	case Allocating, Error, Failed, Rebooting:
		return false
	default:
		return false
	}
}

// IsUnitWorkloadViable returns true if the unit workload is active, or is
// in a state where it is expected to become active.
func IsUnitWorkloadViable(status StatusInfo) bool {
	// This traps the known workload status codes, but if the status isn't
	// recognised, we assume the workload is not viable.
	switch status.Status {
	case Active:
		return true
	case Maintenance:
		switch status.Message {
		case MessageInstallingCharm:
			return false
		}
		return true
	case Waiting:
		switch status.Message {
		case MessageWaitForMachine,
			MessageInstallingAgent,
			MessageInitializingAgent:
			return false
		}
		return true
	case Blocked, Error, Terminated, Unknown:
		return false
	default:
		return false
	}
}
