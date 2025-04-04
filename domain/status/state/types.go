// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	coreapplication "github.com/juju/juju/core/application"
	corerelation "github.com/juju/juju/core/relation"
	coreunit "github.com/juju/juju/core/unit"
)

type applicationID struct {
	ID coreapplication.ID `db:"uuid"`
}

type applicationIDAndName struct {
	ID   coreapplication.ID `db:"uuid"`
	Name string             `db:"name"`
}

type relationUUID struct {
	RelationUUID corerelation.UUID `db:"relation_uuid"`
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

type statusInfoAndUnitNameAndPresence struct {
	UnitName  coreunit.Name `db:"unit_name"`
	StatusID  int           `db:"status_id"`
	Message   string        `db:"message"`
	Data      []byte        `db:"data"`
	UpdatedAt *time.Time    `db:"updated_at"`
	Present   bool          `db:"present"`
}

type workloadAgentStatus struct {
	UnitName          coreunit.Name `db:"unit_name"`
	WorkloadStatusID  *int          `db:"workload_status_id"`
	WorkloadMessage   string        `db:"workload_message"`
	WorkloadData      []byte        `db:"workload_data"`
	WorkloadUpdatedAt *time.Time    `db:"workload_updated_at"`
	AgentStatusID     *int          `db:"agent_status_id"`
	AgentMessage      string        `db:"agent_message"`
	AgentData         []byte        `db:"agent_data"`
	AgentUpdatedAt    *time.Time    `db:"agent_updated_at"`
	Present           bool          `db:"present"`
}

type fullUnitStatus struct {
	UnitName           coreunit.Name `db:"unit_name"`
	WorkloadStatusID   *int          `db:"workload_status_id"`
	WorkloadMessage    string        `db:"workload_message"`
	WorkloadData       []byte        `db:"workload_data"`
	WorkloadUpdatedAt  *time.Time    `db:"workload_updated_at"`
	AgentStatusID      *int          `db:"agent_status_id"`
	AgentMessage       string        `db:"agent_message"`
	AgentData          []byte        `db:"agent_data"`
	AgentUpdatedAt     *time.Time    `db:"agent_updated_at"`
	ContainerStatusID  *int          `db:"container_status_id"`
	ContainerMessage   string        `db:"container_message"`
	ContainerData      []byte        `db:"container_data"`
	ContainerUpdatedAt *time.Time    `db:"container_updated_at"`
	Present            bool          `db:"present"`
}

// relationStatus represents the status of a relation
// from v_relation_status
type relationStatus struct {
	RelationUUID corerelation.UUID `db:"relation_uuid"`
	StatusID     int               `db:"relation_status_type_id"`
	Reason       string            `db:"suspended_reason"`
	Since        *time.Time        `db:"updated_at"`
}
