// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"time"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/instance"
	coremachine "github.com/juju/juju/core/machine"
	corerelation "github.com/juju/juju/core/relation"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/constraints"
	domainlife "github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/status"
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

type statusInfoAndUnitName struct {
	UnitName  coreunit.Name `db:"unit_name"`
	StatusID  int           `db:"status_id"`
	Message   string        `db:"message"`
	Data      []byte        `db:"data"`
	UpdatedAt *time.Time    `db:"updated_at"`
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
	UnitName          coreunit.Name `db:"unit_name"`
	WorkloadStatusID  *int          `db:"workload_status_id"`
	WorkloadMessage   string        `db:"workload_message"`
	WorkloadData      []byte        `db:"workload_data"`
	WorkloadUpdatedAt *time.Time    `db:"workload_updated_at"`
	AgentStatusID     *int          `db:"agent_status_id"`
	AgentMessage      string        `db:"agent_message"`
	AgentData         []byte        `db:"agent_data"`
	AgentUpdatedAt    *time.Time    `db:"agent_updated_at"`
	K8sPodStatusID    *int          `db:"k8s_pod_status_id"`
	K8sPodMessage     string        `db:"k8s_pod_message"`
	K8sPodData        []byte        `db:"k8s_pod_data"`
	K8sPodUpdatedAt   *time.Time    `db:"k8s_pod_updated_at"`
	Present           bool          `db:"present"`
}

// relationStatus represents the status of a relation
// from relation_status
type relationStatus struct {
	RelationUUID corerelation.UUID `db:"relation_uuid"`
	StatusID     int               `db:"relation_status_type_id"`
	Reason       string            `db:"suspended_reason"`
	Since        *time.Time        `db:"updated_at"`
}

// Note: this has to be public because it's embedded and sqlair can't see
// the private struct because of reflection.
type CharmLocatorDetails struct {
	CharmReferenceName  string          `db:"charm_reference_name"`
	CharmRevision       int             `db:"charm_revision"`
	CharmSourceID       int             `db:"charm_source_id"`
	CharmArchitectureID sql.Null[int64] `db:"charm_architecture_id"`
}

type applicationStatusDetails struct {
	CharmLocatorDetails
	UUID                   coreapplication.ID `db:"uuid"`
	Name                   string             `db:"name"`
	PlatformOSID           sql.Null[int64]    `db:"platform_os_id"`
	PlatformChannel        string             `db:"platform_channel"`
	PlatformArchitectureID sql.Null[int64]    `db:"platform_architecture_id"`
	ChannelTrack           string             `db:"channel_track"`
	ChannelRisk            sql.Null[string]   `db:"channel_risk"`
	ChannelBranch          string             `db:"channel_branch"`
	LifeID                 domainlife.Life    `db:"life_id"`
	Subordinate            bool               `db:"subordinate"`
	StatusID               int                `db:"status_id"`
	Message                string             `db:"message"`
	Data                   []byte             `db:"data"`
	UpdatedAt              *time.Time         `db:"updated_at"`
	RelationUUID           sql.Null[string]   `db:"relation_uuid"`
	CharmVersion           string             `db:"charm_version"`
	LXDProfile             sql.Null[[]byte]   `db:"lxd_profile"`
	Exposed                bool               `db:"exposed"`
	Scale                  sql.Null[int]      `db:"scale"`
	WorkloadVersion        sql.Null[string]   `db:"workload_version"`
	K8sProviderID          sql.Null[string]   `db:"k8s_provider_id"`
}

type unitStatusDetails struct {
	CharmLocatorDetails
	UUID              coreunit.UUID              `db:"uuid"`
	Name              coreunit.Name              `db:"name"`
	LifeID            domainlife.Life            `db:"life_id"`
	ApplicationName   string                     `db:"application_name"`
	MachineName       sql.Null[coremachine.Name] `db:"machine_name"`
	PrincipalName     sql.Null[coreunit.Name]    `db:"principal_name"`
	Subordinate       bool                       `db:"subordinate"`
	SubordinateName   sql.Null[coreunit.Name]    `db:"subordinate_name"`
	AgentStatusID     int                        `db:"agent_status_id"`
	AgentMessage      string                     `db:"agent_message"`
	AgentData         []byte                     `db:"agent_data"`
	AgentUpdatedAt    *time.Time                 `db:"agent_updated_at"`
	WorkloadStatusID  int                        `db:"workload_status_id"`
	WorkloadMessage   string                     `db:"workload_message"`
	WorkloadData      []byte                     `db:"workload_data"`
	WorkloadUpdatedAt *time.Time                 `db:"workload_updated_at"`
	K8sPodStatusID    int                        `db:"k8s_pod_status_id"`
	K8sPodMessage     string                     `db:"k8s_pod_message"`
	K8sPodData        []byte                     `db:"k8s_pod_data"`
	K8sPodUpdatedAt   *time.Time                 `db:"k8s_pod_updated_at"`
	Present           bool                       `db:"present"`
	AgentVersion      string                     `db:"agent_version"`
	WorkloadVersion   sql.Null[string]           `db:"workload_version"`
	K8sProviderID     sql.Null[string]           `db:"k8s_provider_id"`
}

// relationStatus represents the status of a relation and the relations ID.
type relationStatusAndID struct {
	RelationUUID corerelation.UUID `db:"relation_uuid"`
	RelationID   int               `db:"relation_id"`
	StatusID     int               `db:"relation_status_type_id"`
	Reason       string            `db:"suspended_reason"`
	Since        *time.Time        `db:"updated_at"`
}

type applicationNameUnitCount struct {
	Name      string `db:"name"`
	UnitCount int    `db:"unit_count"`
}

type modelUUID struct {
	UUID string `db:"uuid"`
}

type modelInfo struct {
	Type string `db:"type"`
}

type filesystemUUID struct {
	FilesystemUUID string `db:"uuid"`
}

type filesystemUUIDID struct {
	ID   string `db:"filesystem_id"`
	UUID string `db:"uuid"`
}

type filesystemStatusInfo struct {
	FilesystemUUID string     `db:"filesystem_uuid"`
	StatusID       int        `db:"status_id"`
	Message        string     `db:"message"`
	UpdatedAt      *time.Time `db:"updated_at"`
}

type storageProvisioningStatusInfo struct {
	StatusID            sql.NullInt16  `db:"status_id"`
	StorageInstanceUUID sql.NullString `db:"storage_instance_uuid"`
}

type volumeUUID struct {
	VolumeUUID string `db:"uuid"`
}

type volumeUUIDID struct {
	ID   string `db:"volume_id"`
	UUID string `db:"uuid"`
}

type volumeStatusInfo struct {
	VolumeUUID string     `db:"volume_uuid"`
	StatusID   int        `db:"status_id"`
	Message    string     `db:"message"`
	UpdatedAt  *time.Time `db:"updated_at"`
}

// modelStatusContext represents a single row from the v_model_state view.
// These information are used to determine a model's status.
type modelStatusContext struct {
	Destroying              bool   `db:"destroying"`
	CredentialInvalid       bool   `db:"cloud_credential_invalid"`
	CredentialInvalidReason string `db:"cloud_credential_invalid_reason"`
	Migrating               bool   `db:"migrating"`
}

type machineName struct {
	Name string `db:"name"`
}

type machineUUID struct {
	UUID string `db:"uuid"`
}

type machineStatus struct {
	Status  string              `db:"status"`
	Message string              `db:"message"`
	Data    []byte              `db:"data"`
	Updated sql.Null[time.Time] `db:"updated_at"`
}

type machineNameStatus struct {
	Name    string              `db:"name"`
	Status  string              `db:"status"`
	Message string              `db:"message"`
	Data    []byte              `db:"data"`
	Updated sql.Null[time.Time] `db:"updated_at"`
}

type setMachineStatus struct {
	StatusID    int        `db:"status_id"`
	Message     string     `db:"message"`
	Data        []byte     `db:"data"`
	UpdatedAt   *time.Time `db:"updated_at"`
	MachineUUID string     `db:"machine_uuid"`
}

type machineStatusDetails struct {
	Name                       coremachine.Name          `db:"name"`
	UUID                       coremachine.UUID          `db:"uuid"`
	LifeID                     domainlife.Life           `db:"life_id"`
	Hostname                   sql.Null[string]          `db:"hostname"`
	InstanceID                 sql.Null[string]          `db:"instance_id"`
	DisplayName                sql.Null[string]          `db:"display_name"`
	PlatformOSID               sql.Null[int64]           `db:"platform_os_id"`
	PlatformChannel            string                    `db:"platform_channel"`
	PlatformArchitectureID     sql.Null[int64]           `db:"platform_architecture_id"`
	MachineStatusID            status.MachineStatusType  `db:"machine_status_id"`
	MachineMessage             string                    `db:"machine_message"`
	MachineData                []byte                    `db:"machine_data"`
	MachineUpdatedAt           *time.Time                `db:"machine_updated_at"`
	InstanceStatusID           status.InstanceStatusType `db:"instance_status_id"`
	InstanceMessage            string                    `db:"instance_message"`
	InstanceData               []byte                    `db:"instance_data"`
	InstanceUpdatedAt          *time.Time                `db:"instance_updated_at"`
	InstanceArch               sql.Null[string]          `db:"instance_arch"`
	InstanceCPUCores           sql.Null[uint64]          `db:"instance_cpu_cores"`
	InstanceCPUPower           sql.Null[uint64]          `db:"instance_cpu_power"`
	InstanceMem                sql.Null[uint64]          `db:"instance_mem"`
	InstanceRootDisk           sql.Null[uint64]          `db:"instance_root_disk"`
	InstanceRootDiskSource     sql.Null[string]          `db:"instance_root_disk_source"`
	InstanceVirtType           sql.Null[string]          `db:"instance_virt_type"`
	InstanceAvailabilityZone   sql.Null[string]          `db:"availability_zone_name"`
	ConstraintArch             sql.Null[string]          `db:"constraint_arch"`
	ConstraintCPUCores         sql.Null[uint64]          `db:"constraint_cpu_cores"`
	ConstraintCPUPower         sql.Null[uint64]          `db:"constraint_cpu_power"`
	ConstraintMem              sql.Null[uint64]          `db:"constraint_mem"`
	ConstraintRootDisk         sql.Null[uint64]          `db:"constraint_root_disk"`
	ConstraintRootDiskSource   sql.Null[string]          `db:"constraint_root_disk_source"`
	ConstraintVirtType         sql.Null[string]          `db:"constraint_virt_type"`
	ConstraintInstanceRole     sql.Null[string]          `db:"constraint_instance_role"`
	ConstraintInstanceType     sql.Null[string]          `db:"constraint_instance_type"`
	ConstraintContainerType    sql.Null[string]          `db:"constraint_container_type"`
	ConstraintAllocatePublicIP sql.Null[int]             `db:"constraint_allocate_public_ip"`
	ConstraintImageID          sql.Null[string]          `db:"constraint_image_id"`
}

func decodeHardwareCharacteristics(
	arch sql.Null[string],
	cpuCores sql.Null[uint64],
	cpuPower sql.Null[uint64],
	mem sql.Null[uint64],
	rootDisk sql.Null[uint64],
	rootDiskSource sql.Null[string],
	virtType sql.Null[string],
	availabilityZone sql.Null[string],
) instance.HardwareCharacteristics {
	var hwc instance.HardwareCharacteristics
	if arch.Valid {
		hwc.Arch = ptr(arch.V)
	}
	if cpuCores.Valid {
		hwc.CpuCores = ptr(cpuCores.V)
	}
	if cpuPower.Valid {
		hwc.CpuPower = ptr(cpuPower.V)
	}
	if mem.Valid {
		hwc.Mem = ptr(mem.V)
	}
	if rootDisk.Valid {
		hwc.RootDisk = ptr(rootDisk.V)
	}
	if rootDiskSource.Valid {
		hwc.RootDiskSource = ptr(rootDiskSource.V)
	}
	if virtType.Valid {
		hwc.VirtType = ptr(virtType.V)
	}
	if availabilityZone.Valid {
		hwc.AvailabilityZone = ptr(availabilityZone.V)
	}
	return hwc
}

func decodeConstraints(
	arch sql.Null[string],
	cpuCores sql.Null[uint64],
	cpuPower sql.Null[uint64],
	mem sql.Null[uint64],
	rootDisk sql.Null[uint64],
	rootDiskSource sql.Null[string],
	virtType sql.Null[string],
	instanceRole sql.Null[string],
	instanceType sql.Null[string],
	containerType sql.Null[string],
	allocatePublicIP sql.Null[int],
	imageID sql.Null[string],
) constraints.Constraints {
	var cons constraints.Constraints
	if arch.Valid {
		cons.Arch = ptr(arch.V)
	}
	if cpuCores.Valid {
		cons.CpuCores = ptr(cpuCores.V)
	}
	if cpuPower.Valid {
		cons.CpuPower = ptr(cpuPower.V)
	}
	if mem.Valid {
		cons.Mem = ptr(mem.V)
	}
	if rootDisk.Valid {
		cons.RootDisk = ptr(rootDisk.V)
	}
	if rootDiskSource.Valid {
		cons.RootDiskSource = ptr(rootDiskSource.V)
	}
	if virtType.Valid {
		cons.VirtType = ptr(virtType.V)
	}
	if instanceRole.Valid {
		cons.InstanceRole = ptr(instanceRole.V)
	}
	if instanceType.Valid {
		cons.InstanceType = ptr(instanceType.V)
	}
	if containerType.Valid {
		cons.Container = ptr(instance.ContainerType(containerType.V))
	}
	if allocatePublicIP.Valid {
		cons.AllocatePublicIP = ptr(allocatePublicIP.V == 1)
	}
	if imageID.Valid {
		cons.ImageID = ptr(imageID.V)
	}
	return cons
}
