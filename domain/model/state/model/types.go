// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"database/sql"

	"github.com/juju/juju/core/instance"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/internal/uuid"
)

// dbUUID represents a UUID.
type dbUUID struct {
	UUID string `db:"uuid"`
}

// dbControllerUUID represents the controller uuid value on a model record.
type dbControllerUUID struct {
	UUID string `db:"controller_uuid"`
}

type dbModelUUID struct {
	UUID string `db:"uuid"`
}

// dbModelType represents the model type from the model table.
type dbModelType struct {
	Type string `db:"type"`
}

// dbModelInfoSummary represents a summary of the information located with in
// the model database. Specifically the model information.
type dbModelInfoSummary struct {
	UUID               string         `db:"uuid"`
	Name               string         `db:"name"`
	Qualifier          string         `db:"qualifier"`
	Type               string         `db:"type"`
	ControllerUUID     string         `db:"controller_uuid"`
	Cloud              string         `db:"cloud"`
	CloudType          string         `db:"cloud_type"`
	CloudRegion        sql.NullString `db:"cloud_region"`
	IsControllerModel  bool           `db:"is_controller_model"`
	TargetAgentVersion string         `db:"target_version"`
}

// dbModelCountSummary is a count summary of the things in the database that we
// care about.
type dbModelCountSummary struct {
	CoreCount    int64 `db:"core_count"`
	MachineCount int64 `db:"machine_count"`
	UnitCount    int64 `db:"unit_count"`
}

type dbReadOnlyModel struct {
	UUID              string `db:"uuid"`
	ControllerUUID    string `db:"controller_uuid"`
	Name              string `db:"name"`
	Qualifier         string `db:"qualifier"`
	Type              string `db:"type"`
	Cloud             string `db:"cloud"`
	CloudType         string `db:"cloud_type"`
	CloudRegion       string `db:"cloud_region"`
	CredentialOwner   string `db:"credential_owner"`
	CredentialName    string `db:"credential_name"`
	IsControllerModel bool   `db:"is_controller_model"`
}

type dbModelMetrics struct {
	ApplicationCount int `db:"application_count"`
	MachineCount     int `db:"machine_count"`
	UnitCount        int `db:"unit_count"`
}

type dbModelCloudRegionCredential struct {
	CloudName           string         `db:"cloud"`
	CloudRegionName     string         `db:"cloud_region"`
	CredentialName      sql.NullString `db:"credential_name"`
	CredentialOwnerName sql.NullString `db:"credential_owner"`
}

// dbModelAgent represents a row from the model_agent table
// with nullable values for the purpose of defensive programming.
type dbModelAgent struct {
	// StreamID is the unique identifier for the agent stream that is being used
	// for model agents.
	StreamID int `db:"stream_id"`

	// TargetVersion describes the desired agent version that should be
	// being run in this model. It should not be considered "the" version that
	// is being run for every agent as each agent needs to upgrade to this
	// version.
	TargetVersion sql.Null[string] `db:"target_version"`

	// LatestVersion describes the latest known agent version for the model.
	LatestVersion sql.Null[string] `db:"latest_version"`
}

// dbModelConstraint represents a single row from the model_constraint table.
type dbModelConstraint struct {
	ModelUUID      string `db:"model_uuid"`
	ConstraintUUID string `db:"constraint_uuid"`
}

// dbConstraint represents a single row within the v_model_constraint view.
type dbConstraint struct {
	Arch             sql.NullString `db:"arch"`
	CPUCores         sql.NullInt64  `db:"cpu_cores"`
	CPUPower         sql.NullInt64  `db:"cpu_power"`
	Mem              sql.NullInt64  `db:"mem"`
	RootDisk         sql.NullInt64  `db:"root_disk"`
	RootDiskSource   sql.NullString `db:"root_disk_source"`
	InstanceRole     sql.NullString `db:"instance_role"`
	InstanceType     sql.NullString `db:"instance_type"`
	ContainerType    sql.NullString `db:"container_type"`
	VirtType         sql.NullString `db:"virt_type"`
	AllocatePublicIP sql.NullBool   `db:"allocate_public_ip"`
	ImageID          sql.NullString `db:"image_id"`
}

// dbConstraintInsert is used to supply insert values into the constraint table.
type dbConstraintInsert struct {
	UUID             string         `db:"uuid"`
	Arch             sql.NullString `db:"arch"`
	CPUCores         sql.NullInt64  `db:"cpu_cores"`
	CPUPower         sql.NullInt64  `db:"cpu_power"`
	Mem              sql.NullInt64  `db:"mem"`
	RootDisk         sql.NullInt64  `db:"root_disk"`
	RootDiskSource   sql.NullString `db:"root_disk_source"`
	InstanceRole     sql.NullString `db:"instance_role"`
	InstanceType     sql.NullString `db:"instance_type"`
	ContainerTypeId  sql.NullInt64  `db:"container_type_id"`
	VirtType         sql.NullString `db:"virt_type"`
	AllocatePublicIP sql.NullBool   `db:"allocate_public_ip"`
	ImageID          sql.NullString `db:"image_id"`
}

// constraintsToDBInsert is responsible for taking a constraints value and
// transforming the values into a [dbConstraintInsert] object.
func constraintsToDBInsert(
	uuid uuid.UUID,
	constraints constraints.Constraints,
) dbConstraintInsert {
	return dbConstraintInsert{
		UUID: uuid.String(),
		Arch: sql.NullString{
			String: deref(constraints.Arch),
			Valid:  constraints.Arch != nil,
		},
		CPUCores: sql.NullInt64{
			Int64: int64(deref(constraints.CpuCores)),
			Valid: constraints.CpuCores != nil,
		},
		CPUPower: sql.NullInt64{
			Int64: int64(deref(constraints.CpuPower)),
			Valid: constraints.CpuPower != nil,
		},
		Mem: sql.NullInt64{
			Int64: int64(deref(constraints.Mem)),
			Valid: constraints.Mem != nil,
		},
		RootDisk: sql.NullInt64{
			Int64: int64(deref(constraints.RootDisk)),
			Valid: constraints.RootDisk != nil,
		},
		RootDiskSource: sql.NullString{
			String: deref(constraints.RootDiskSource),
			Valid:  constraints.RootDiskSource != nil,
		},
		InstanceRole: sql.NullString{
			String: deref(constraints.InstanceRole),
			Valid:  constraints.InstanceRole != nil,
		},
		InstanceType: sql.NullString{
			String: deref(constraints.InstanceType),
			Valid:  constraints.InstanceType != nil,
		},
		VirtType: sql.NullString{
			String: deref(constraints.VirtType),
			Valid:  constraints.VirtType != nil,
		},
		AllocatePublicIP: sql.NullBool{
			Bool:  deref(constraints.AllocatePublicIP),
			Valid: constraints.AllocatePublicIP != nil,
		},
		ImageID: sql.NullString{
			String: deref(constraints.ImageID),
			Valid:  constraints.ImageID != nil,
		},
	}
}

func ptr[T any](i T) *T {
	return &i
}

// deref returns the dereferenced value of T if T is not nil. Otherwise the zero
// value of T is returned.
func deref[T any](i *T) T {
	if i == nil {
		var v T
		return v
	}
	return *i
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
		rval.CpuCores = ptr(uint64(c.CPUCores.Int64))
	}
	if c.CPUPower.Valid {
		rval.CpuPower = ptr(uint64(c.CPUPower.Int64))
	}
	if c.Mem.Valid {
		rval.Mem = ptr(uint64(c.Mem.Int64))
	}
	if c.RootDisk.Valid {
		rval.RootDisk = ptr(uint64(c.RootDisk.Int64))
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

// dbAggregateCount is a type to store the result for counting the number of
// rows returned by a select query.
type dbAggregateCount struct {
	Count int `db:"count"`
}

// dbContainerTypeId represents the id of a container type as found in the
// container_type table.
type dbContainerTypeId struct {
	Id int64 `db:"id"`
}

// dbContainerTypeValue represents a container type value from the
// container_type table.
type dbContainerTypeValue struct {
	Value string `db:"value"`
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

// dbConstraintUUID represents a constraint uuid within the database.
type dbConstraintUUID struct {
	UUID string `db:"uuid"`
}

type dbModelLife struct {
	UUID      coremodel.UUID `db:"uuid"`
	Life      life.Life      `db:"life_id"`
	Activated bool           `db:"activated"`
}

// dbInsertStoragePool represents the information required for inserting a new
// storage pool into the model.
type dbInsertStoragePool struct {
	UUID     string `db:"uuid"`
	Name     string `db:"name"`
	Type     string `db:"type"`
	OriginID int    `db:"origin_id"`
}

// dbInsertStoragePoolAttribute represents a single attribute for a storage pool
// that is to be inserted.
type dbInsertStoragePoolAttribute struct {
	StoragePoolUUID string `db:"storage_pool_uuid"`
	Key             string `db:"key"`
	Value           string `db:"value"`
}
