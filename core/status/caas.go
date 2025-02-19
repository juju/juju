// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

// UnitDisplayStatus is used for CAAS units where the status of the unit
// could be overridden by the status of the container.
func UnitDisplayStatus(unitStatus, containerStatus StatusInfo) StatusInfo {
	if unitStatus.Status == Terminated {
		return unitStatus
	}
	if containerStatus.Status == Terminated {
		return containerStatus
	}
	if containerStatus.Status == "" {
		// No container update received from k8s yet.
		// Unit may have set sttaus, in which case use it.
		if isStatusModified(unitStatus) {
			return unitStatus
		}
		// If no unit status set, assume still allocating.
		return StatusInfo{
			Status:  Waiting,
			Message: unitStatus.Message,
			Since:   containerStatus.Since,
		}
	}
	if unitStatus.Status != Active && unitStatus.Status != Waiting && unitStatus.Status != Blocked {
		// Charm has said that there's a problem (error) or
		// it's doing something (maintenance) so we'll stick with that.
		return unitStatus
	}

	// Charm may think it's active, but as yet there's no way for it to
	// query the workload state, so we'll ensure that we only say that
	// it's active if the pod is reported as running. If not, we'll report
	// any pod error.
	switch containerStatus.Status {
	case Error, Blocked, Allocating:
		return containerStatus
	case Waiting:
		if unitStatus.Status == Active {
			return containerStatus
		}
	case Running:
		// Unit hasn't moved from initial state.
		// thumper: I find this questionable, at best it is Unknown.
		if !isStatusModified(unitStatus) {
			return containerStatus
		}
	}
	return unitStatus
}

func isStatusModified(unitStatus StatusInfo) bool {
	return (unitStatus.Status != "" && unitStatus.Status != Waiting) ||
		(unitStatus.Message != MessageWaitForContainer &&
			unitStatus.Message != MessageInitializingAgent &&
			unitStatus.Message != MessageInstallingAgent)
}
