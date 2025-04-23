// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"time"

	coreapplication "github.com/juju/juju/core/application"
	corerelation "github.com/juju/juju/core/relation"
	coreunit "github.com/juju/juju/core/unit"
	domainlife "github.com/juju/juju/domain/life"
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
// from relation_status
type relationStatus struct {
	RelationUUID corerelation.UUID `db:"relation_uuid"`
	StatusID     int               `db:"relation_status_type_id"`
	Reason       string            `db:"suspended_reason"`
	Since        *time.Time        `db:"updated_at"`
}

type applicationStatusDetails struct {
	UUID                   coreapplication.ID `db:"uuid"`
	Name                   string             `db:"name"`
	PlatformOSID           sql.NullInt64      `db:"platform_os_id"`
	PlatformChannel        string             `db:"platform_channel"`
	PlatformArchitectureID sql.NullInt64      `db:"platform_architecture_id"`
	ChannelTrack           string             `db:"channel_track"`
	ChannelRisk            sql.NullString     `db:"channel_risk"`
	ChannelBranch          string             `db:"channel_branch"`
	LifeID                 domainlife.Life    `db:"life_id"`
	Subordinate            bool               `db:"subordinate"`
	StatusID               int                `db:"status_id"`
	Message                string             `db:"message"`
	Data                   []byte             `db:"data"`
	UpdatedAt              *time.Time         `db:"updated_at"`
	RelationUUID           sql.NullString     `db:"relation_uuid"`
	CharmReferenceName     string             `db:"charm_reference_name"`
	CharmRevision          int                `db:"charm_revision"`
	CharmSourceID          int                `db:"charm_source_id"`
	CharmArchitectureID    sql.NullInt64      `db:"charm_architecture_id"`
	CharmVersion           string             `db:"charm_version"`
	LXDProfile             sql.Null[[]byte]   `db:"lxd_profile"`
	Exposed                bool               `db:"exposed"`
	Scale                  sql.Null[int]      `db:"scale"`
	K8sProviderID          sql.NullString     `db:"k8s_provider_id"`
}
