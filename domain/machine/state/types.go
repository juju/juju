// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"time"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
)

// instanceData represents the struct to be inserted into the instance_data
// table.
type instanceData struct {
	MachineUUID          string           `db:"machine_uuid"`
	LifeID               int64            `db:"life_id"`
	InstanceID           sql.Null[string] `db:"instance_id"`
	DisplayName          sql.Null[string] `db:"display_name"`
	Arch                 *string          `db:"arch"`
	Mem                  *uint64          `db:"mem"`
	RootDisk             *uint64          `db:"root_disk"`
	RootDiskSource       *string          `db:"root_disk_source"`
	CPUCores             *uint64          `db:"cpu_cores"`
	CPUPower             *uint64          `db:"cpu_power"`
	AvailabilityZoneUUID *string          `db:"availability_zone_uuid"`
	VirtType             *string          `db:"virt_type"`
}

// instanceDataResult represents the struct used to retrieve rows when joining
// the machine_cloud_instance table with the availability_zone table.
type instanceDataResult struct {
	MachineUUID      string  `db:"machine_uuid"`
	InstanceID       string  `db:"instance_id"`
	Arch             *string `db:"arch"`
	Mem              *uint64 `db:"mem"`
	RootDisk         *uint64 `db:"root_disk"`
	RootDiskSource   *string `db:"root_disk_source"`
	CPUCores         *uint64 `db:"cpu_cores"`
	CPUPower         *uint64 `db:"cpu_power"`
	AvailabilityZone *string `db:"availability_zone_name"`
	VirtType         *string `db:"virt_type"`
}

// instanceTag represents the struct to be inserted into the instance_tag
// table.
type instanceTag struct {
	MachineUUID string `db:"machine_uuid"`
	Tag         string `db:"tag"`
}

func tagsFromHardwareCharacteristics(machineUUID string, hc *instance.HardwareCharacteristics) []instanceTag {
	if hc == nil || hc.Tags == nil {
		return nil
	}
	res := make([]instanceTag, len(*hc.Tags))
	for i, tag := range *hc.Tags {
		res[i] = instanceTag{
			MachineUUID: machineUUID,
			Tag:         tag,
		}
	}
	return res
}

func (d *instanceDataResult) toHardwareCharacteristics() *instance.HardwareCharacteristics {
	return &instance.HardwareCharacteristics{
		Arch:             d.Arch,
		Mem:              d.Mem,
		RootDisk:         d.RootDisk,
		RootDiskSource:   d.RootDiskSource,
		CpuCores:         d.CPUCores,
		CpuPower:         d.CPUPower,
		AvailabilityZone: d.AvailabilityZone,
		VirtType:         d.VirtType,
	}
}

// entityLife represents the struct to be used for the life_id column within
// the sqlair statements in the machine domain.
type entityLife struct {
	LifeID life.Life `db:"life_id"`
}

// instanceID represents the struct to be used for the instance_id column within
// the sqlair statements in the machine domain.
type instanceID struct {
	ID string `db:"instance_id"`
}

// instanceIDAndDisplayName represents the struct to be used for the display_name and ID
// column within the sqlair statements in the machine domain.
type instanceIDAndDisplayName struct {
	ID   string `db:"instance_id"`
	Name string `db:"display_name"`
}

type setStatusInfo struct {
	StatusID int        `db:"status_id"`
	Message  string     `db:"message"`
	Data     []byte     `db:"data"`
	Updated  *time.Time `db:"updated_at"`
}

type setMachineStatus struct {
	StatusID    int        `db:"status_id"`
	Message     string     `db:"message"`
	Data        []byte     `db:"data"`
	Updated     *time.Time `db:"updated_at"`
	MachineUUID string     `db:"machine_uuid"`
}

type nameAndUUID struct {
	UUID string `db:"uuid"`
	Name string `db:"name"`
}

type machineName struct {
	Name machine.Name `db:"name"`
}

type machineInstanceUUID struct {
	MachineUUID string `db:"machine_uuid"`
	LifeID      int64  `db:"life_id"`
}

type count struct {
	Count int64 `db:"count"`
}

type keepInstance struct {
	KeepInstance bool `db:"keep_instance"`
}

type machineParent struct {
	MachineUUID string `db:"machine_uuid"`
	ParentUUID  string `db:"parent_uuid"`
}

// nameSliceTransform is a function that is used to transform a slice of
// machineName into a slice of machine.Name.
func (s machineName) nameSliceTransform() machine.Name {
	return s.Name
}

// lxdProfile represents the struct to be used for the sqlair statements on the
// machine_lxd_profile table.
type lxdProfile struct {
	MachineUUID string `db:"machine_uuid"`
	Name        string `db:"name"`
	Index       int    `db:"array_index"`
}

// lxdProfileAndName represents data to construct a profile name
// and the profile itself.
type lxdProfileAndName struct {
	AppName    string `db:"name"`
	LXDProfile []byte `db:"lxd_profile"`
	Revision   int    `db:"revision"`
}

type machineNonce struct {
	MachineUUID string `db:"machine_uuid"`
	Nonce       string `db:"nonce"`
}

type machineInstance struct {
	MachineName string `db:"machine_name"`
	InstanceID  string `db:"instance_id"`
	IsContainer int64  `db:"is_container"`
}

type createMachine struct {
	Name        string           `db:"name"`
	NetNodeUUID string           `db:"net_node_uuid"`
	UUID        string           `db:"uuid"`
	Nonce       sql.Null[string] `db:"nonce"`
	LifeID      int64            `db:"life_id"`
}

type archName struct {
	Arch string `db:"architecture"`
}

type machinePlatformUUID struct {
	MachineUUID    string           `db:"machine_uuid"`
	OSID           sql.Null[int64]  `db:"os_id"`
	Channel        sql.Null[string] `db:"channel"`
	ArchitectureID int              `db:"architecture_id"`
}

type machineNameWithNetNodeUUID struct {
	Name        machine.Name `db:"name"`
	NetNodeUUID string       `db:"net_node_uuid"`
}

type machineNameWithMachineUUID struct {
	Name machine.Name `db:"name"`
	UUID machine.UUID `db:"uuid"`
}

type machinePlacement struct {
	MachineUUID string `db:"machine_uuid"`
	ScopeID     int    `db:"scope_id"`
	Directive   string `db:"directive"`
}

type exportMachine struct {
	Name               string `db:"name"`
	UUID               string `db:"uuid"`
	Nonce              string `db:"nonce"`
	PasswordHash       string `db:"password_hash"`
	PlacementDirective string `db:"directive"`
	OSName             string `db:"os_name"`
	Channel            string `db:"channel"`
	InstanceID         string `db:"instance_id"`
}

type machineHostName struct {
	Hostname       sql.Null[string] `db:"hostname"`
	AgentStartedAt time.Time        `db:"agent_started_at"`
}

type containerType struct {
	ContainerType string `db:"container_type"`
}

type machineContainerType struct {
	MachineUUID     string `db:"machine_uuid"`
	ContainerTypeID int    `db:"container_type_id"`
}

type insertMachineAndNetNodeArgs struct {
	machineName string
	machineUUID string
	platform    deployment.Platform
	nonce       *string
	constraints constraints.Constraints
}

type insertChildMachineForContainerPlacementArgs struct {
	machineUUID string
	parentUUID  string
	parentName  string
	scope       string
	platform    deployment.Platform
	nonce       *string
	constraints constraints.Constraints
}

type acquireParentMachineForContainerArgs struct {
	directive   string
	platform    deployment.Platform
	constraints constraints.Constraints
}

type placementDirective struct {
	Directive sql.Null[string] `db:"directive"`
}

// machineConstraint represents a single returned row when joining the
// constraint table with the constraint_space, constraint_tag and
// constraint_zone.
type machineConstraint struct {
	MachineUUID      string          `db:"machine_uuid"`
	Arch             sql.NullString  `db:"arch"`
	CPUCores         sql.Null[int64] `db:"cpu_cores"`
	CPUPower         sql.Null[int64] `db:"cpu_power"`
	Mem              sql.Null[int64] `db:"mem"`
	RootDisk         sql.Null[int64] `db:"root_disk"`
	RootDiskSource   sql.NullString  `db:"root_disk_source"`
	InstanceRole     sql.NullString  `db:"instance_role"`
	InstanceType     sql.NullString  `db:"instance_type"`
	ContainerType    sql.NullString  `db:"container_type"`
	VirtType         sql.NullString  `db:"virt_type"`
	AllocatePublicIP sql.NullBool    `db:"allocate_public_ip"`
	ImageID          sql.NullString  `db:"image_id"`
	SpaceName        sql.NullString  `db:"space_name"`
	SpaceExclude     sql.NullBool    `db:"space_exclude"`
	Tag              sql.NullString  `db:"tag"`
	Zone             sql.NullString  `db:"zone"`
}

type machineConstraints []machineConstraint

type machinePlatform struct {
	OSName       string `db:"os_name"`
	Channel      string `db:"channel"`
	Architecture string `db:"architecture"`
}

type setMachineConstraint struct {
	MachineUUID    string `db:"machine_uuid"`
	ConstraintUUID string `db:"constraint_uuid"`
}

type setConstraint struct {
	UUID             string  `db:"uuid"`
	Arch             *string `db:"arch"`
	CPUCores         *uint64 `db:"cpu_cores"`
	CPUPower         *uint64 `db:"cpu_power"`
	Mem              *uint64 `db:"mem"`
	RootDisk         *uint64 `db:"root_disk"`
	RootDiskSource   *string `db:"root_disk_source"`
	InstanceRole     *string `db:"instance_role"`
	InstanceType     *string `db:"instance_type"`
	ContainerTypeID  *uint64 `db:"container_type_id"`
	VirtType         *string `db:"virt_type"`
	AllocatePublicIP *bool   `db:"allocate_public_ip"`
	ImageID          *string `db:"image_id"`
}

type setConstraintTag struct {
	ConstraintUUID string `db:"constraint_uuid"`
	Tag            string `db:"tag"`
}

type setConstraintSpace struct {
	ConstraintUUID string `db:"constraint_uuid"`
	Space          string `db:"space"`
	Exclude        bool   `db:"exclude"`
}

type setConstraintZone struct {
	ConstraintUUID string `db:"constraint_uuid"`
	Zone           string `db:"zone"`
}

type containerTypeID struct {
	ID uint64 `db:"id"`
}

type containerTypeVal struct {
	Value string `db:"value"`
}

// dbConstraint represents a single row within the v_model_constraint view.
type dbConstraint struct {
	Arch             sql.NullString  `db:"arch"`
	CPUCores         sql.Null[int64] `db:"cpu_cores"`
	CPUPower         sql.Null[int64] `db:"cpu_power"`
	Mem              sql.Null[int64] `db:"mem"`
	RootDisk         sql.Null[int64] `db:"root_disk"`
	RootDiskSource   sql.NullString  `db:"root_disk_source"`
	InstanceRole     sql.NullString  `db:"instance_role"`
	InstanceType     sql.NullString  `db:"instance_type"`
	ContainerType    sql.NullString  `db:"container_type"`
	VirtType         sql.NullString  `db:"virt_type"`
	AllocatePublicIP sql.NullBool    `db:"allocate_public_ip"`
	ImageID          sql.NullString  `db:"image_id"`
}

func (c dbConstraint) toValue(
	tags []dbConstraintTag,
	spaces []dbConstraintSpace,
	zones []dbConstraintZone,
) (constraints.Constraints, error) {
	rval := constraints.Constraints{}
	if c.Arch.Valid {
		rval.Arch = &c.Arch.String
	}
	if c.CPUCores.Valid {
		rval.CpuCores = ptr(uint64(c.CPUCores.V))
	}
	if c.CPUPower.Valid {
		rval.CpuPower = ptr(uint64(c.CPUPower.V))
	}
	if c.Mem.Valid {
		rval.Mem = ptr(uint64(c.Mem.V))
	}
	if c.RootDisk.Valid {
		rval.RootDisk = ptr(uint64(c.RootDisk.V))
	}
	if c.RootDiskSource.Valid {
		rval.RootDiskSource = &c.RootDiskSource.String
	}
	if c.InstanceRole.Valid {
		rval.InstanceRole = &c.InstanceRole.String
	}
	if c.InstanceType.Valid {
		rval.InstanceType = &c.InstanceType.String
	}
	if c.VirtType.Valid {
		rval.VirtType = &c.VirtType.String
	}
	// We only set allocate public ip when it is true and not nil. The reason
	// for this is no matter what the dqlite driver will always return false
	// out of the database even when the value is NULL.
	if c.AllocatePublicIP.Valid {
		rval.AllocatePublicIP = &c.AllocatePublicIP.Bool
	}
	if c.ImageID.Valid {
		rval.ImageID = &c.ImageID.String
	}
	if c.ContainerType.Valid {
		containerType := instance.ContainerType(c.ContainerType.String)
		rval.Container = &containerType
	}

	consTags := make([]string, 0, len(tags))
	for _, tag := range tags {
		consTags = append(consTags, tag.Tag)
	}
	// Only set constraint tags if there are tags in the database value.
	if len(consTags) != 0 {
		rval.Tags = &consTags
	}

	consSpaces := make([]constraints.SpaceConstraint, 0, len(spaces))
	for _, space := range spaces {
		consSpaces = append(consSpaces, constraints.SpaceConstraint{
			SpaceName: space.Space,
			Exclude:   space.Exclude,
		})
	}
	// Only set constraint spaces if there are spaces in the database value.
	if len(consSpaces) != 0 {
		rval.Spaces = &consSpaces
	}

	consZones := make([]string, 0, len(zones))
	for _, zone := range zones {
		consZones = append(consZones, zone.Zone)
	}
	// Only set constraint zones if there are zones in the database value.
	if len(consZones) != 0 {
		rval.Zones = &consZones
	}

	return rval, nil
}

// dbConstraintTag represents a row from either the constraint_tag table or
// v_model_constraint_tag view.
type dbConstraintTag struct {
	ConstraintUUID string `db:"constraint_uuid"`
	Tag            string `db:"tag"`
}

// dbConstraintSpace represents a row from either the constraint_space table or
// v_model_constraint_space view.
type dbConstraintSpace struct {
	ConstraintUUID string `db:"constraint_uuid"`
	Space          string `db:"space"`
	Exclude        bool   `db:"exclude"`
}

// dbConstraintZone represents a row from either the constraint_zone table or
// v_model_constraint_zone view.
type dbConstraintZone struct {
	ConstraintUUID string `db:"constraint_uuid"`
	Zone           string `db:"zone"`
}

type entityUUID struct {
	UUID string `db:"uuid"`
}

type entityName struct {
	Name string `db:"name"`
}

type sshHostKey struct {
	UUID        string `db:"uuid"`
	MachineUUID string `db:"machine_uuid"`
	Key         string `db:"ssh_key"`
}
