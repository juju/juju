// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	coreapplication "github.com/juju/juju/core/application"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/errors"
)

type applicationID struct {
	ID coreapplication.ID `db:"uuid"`
}

type applicationIDAndName struct {
	ID   coreapplication.ID `db:"uuid"`
	Name string             `db:"name"`
}

type unitUUID struct {
	UnitUUID coreunit.UUID `db:"uuid"`
}

type unitName struct {
	Name coreunit.Name `db:"name"`
}

type unitPresence struct {
	UnitUUID coreunit.UUID `db:"unit_uuid"`
	LastSeen time.Time     `db:"last_seen"`
}

type statusInfo struct {
	StatusID  int        `db:"status_id"`
	Message   string     `db:"message"`
	Data      []byte     `db:"data"`
	UpdatedAt *time.Time `db:"updated_at"`
}

type applicationStatusInfo struct {
	ApplicationID coreapplication.ID `db:"application_uuid"`
	StatusID      int                `db:"status_id"`
	Message       string             `db:"message"`
	Data          []byte             `db:"data"`
	UpdatedAt     *time.Time         `db:"updated_at"`
}

type applicationNameStatusInfo struct {
	ApplicationName string     `db:"name"`
	StatusID        int        `db:"status_id"`
	Message         string     `db:"message"`
	Data            []byte     `db:"data"`
	UpdatedAt       *time.Time `db:"updated_at"`
}

type unitStatusInfo struct {
	UnitUUID  coreunit.UUID `db:"unit_uuid"`
	StatusID  int           `db:"status_id"`
	Message   string        `db:"message"`
	Data      []byte        `db:"data"`
	UpdatedAt *time.Time    `db:"updated_at"`
}

type unitPresentStatusInfo struct {
	StatusID  int        `db:"status_id"`
	Message   string     `db:"message"`
	Data      []byte     `db:"data"`
	UpdatedAt *time.Time `db:"updated_at"`
	Present   bool       `db:"present"`
}

type statusInfoAndUnitName struct {
	UnitName  coreunit.Name `db:"name"`
	StatusID  int           `db:"status_id"`
	Message   string        `db:"message"`
	Data      []byte        `db:"data"`
	UpdatedAt *time.Time    `db:"updated_at"`
}

type statusInfoAndUnitNameAndPresence struct {
	UnitName  coreunit.Name `db:"name"`
	StatusID  int           `db:"status_id"`
	Message   string        `db:"message"`
	Data      []byte        `db:"data"`
	UpdatedAt *time.Time    `db:"updated_at"`
	Present   bool          `db:"present"`
}

type fullUnitStatus struct {
	UnitName          coreunit.Name `db:"unit_name"`
	WorkloadStatusID  *int          `db:"workload_status_id"`
	WorkloadMessage   *string       `db:"workload_message"`
	WorkloadData      []byte        `db:"workload_data"`
	WorkloadUpdatedAt *time.Time    `db:"workload_updated_at"`
	AgentStatusID     *int          `db:"agent_status_id"`
	AgentMessage      *string       `db:"agent_message"`
	AgentData         []byte        `db:"agent_data"`
	AgentUpdatedAt    *time.Time    `db:"agent_updated_at"`
	Present           bool          `db:"present"`
}

func encodeCloudContainerStatus(s status.CloudContainerStatusType) (int, error) {
	switch s {
	case status.CloudContainerStatusUnset:
		return 0, nil
	case status.CloudContainerStatusWaiting:
		return 1, nil
	case status.CloudContainerStatusBlocked:
		return 2, nil
	case status.CloudContainerStatusRunning:
		return 3, nil
	default:
		return -1, errors.Errorf("unknown status %q", s)
	}
}

func decodeCloudContainerStatus(s int) (status.CloudContainerStatusType, error) {
	switch s {
	case 0:
		return status.CloudContainerStatusUnset, nil
	case 1:
		return status.CloudContainerStatusWaiting, nil
	case 2:
		return status.CloudContainerStatusBlocked, nil
	case 3:
		return status.CloudContainerStatusRunning, nil
	default:
		return -1, errors.Errorf("unknown status %d", s)
	}
}

func encodeAgentStatus(s status.UnitAgentStatusType) (int, error) {
	switch s {
	case status.UnitAgentStatusAllocating:
		return 0, nil
	case status.UnitAgentStatusExecuting:
		return 1, nil
	case status.UnitAgentStatusIdle:
		return 2, nil
	case status.UnitAgentStatusError:
		return 3, nil
	case status.UnitAgentStatusFailed:
		return 4, nil
	case status.UnitAgentStatusLost:
		return 5, nil
	case status.UnitAgentStatusRebooting:
		return 6, nil
	default:
		return -1, errors.Errorf("unknown status %q", s)
	}
}

func decodeAgentStatus(s int) (status.UnitAgentStatusType, error) {
	switch s {
	case 0:
		return status.UnitAgentStatusAllocating, nil
	case 1:
		return status.UnitAgentStatusExecuting, nil
	case 2:
		return status.UnitAgentStatusIdle, nil
	case 3:
		return status.UnitAgentStatusError, nil
	case 4:
		return status.UnitAgentStatusFailed, nil
	case 5:
		return status.UnitAgentStatusLost, nil
	case 6:
		return status.UnitAgentStatusRebooting, nil
	default:
		return -1, errors.Errorf("unknown status %d", s)
	}
}

func encodeWorkloadStatus(s status.WorkloadStatusType) (int, error) {
	switch s {
	case status.WorkloadStatusUnset:
		return 0, nil
	case status.WorkloadStatusUnknown:
		return 1, nil
	case status.WorkloadStatusMaintenance:
		return 2, nil
	case status.WorkloadStatusWaiting:
		return 3, nil
	case status.WorkloadStatusBlocked:
		return 4, nil
	case status.WorkloadStatusActive:
		return 5, nil
	case status.WorkloadStatusTerminated:
		return 6, nil
	default:
		return -1, errors.Errorf("unknown status %q", s)
	}
}

func decodeWorkloadStatus(s int) (status.WorkloadStatusType, error) {
	switch s {
	case 0:
		return status.WorkloadStatusUnset, nil
	case 1:
		return status.WorkloadStatusUnknown, nil
	case 2:
		return status.WorkloadStatusMaintenance, nil
	case 3:
		return status.WorkloadStatusWaiting, nil
	case 4:
		return status.WorkloadStatusBlocked, nil
	case 5:
		return status.WorkloadStatusActive, nil
	case 6:
		return status.WorkloadStatusTerminated, nil
	default:
		return -1, errors.Errorf("unknown status %d", s)
	}
}
