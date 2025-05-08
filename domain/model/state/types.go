// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"time"

	corecloud "github.com/juju/juju/core/cloud"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/instance"
	corelife "github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// dbModel represents the state of a model.
type dbModel struct {
	// Name is the human friendly name of the model.
	Name string `db:"name"`

	// Life is the current state of the model.
	// Options are alive, dying, dead. Every model starts as alive, only
	// during the destruction of the model it transitions to dying and then
	// dead.
	Life string `db:"life"`

	// UUID is the universally unique identifier of the model.
	UUID string `db:"uuid"`

	// ModelType is the type of model.
	ModelType string `db:"model_type"`

	// CloudName is the name of the cloud to associate with the model.
	CloudName string `db:"cloud_name"`

	// CloudType is the type of the underlying cloud (e.g. lxd, azure, ...)
	CloudType string `db:"cloud_type"`

	// CloudRegion is the region that the model will use in the cloud.
	CloudRegion sql.NullString `db:"cloud_region_name"`

	// CredentialName is the name of the model cloud credential.
	CredentialName sql.NullString `db:"cloud_credential_name"`

	// CredentialCloudName is the cloud name that the model cloud credential applies to.
	CredentialCloudName sql.NullString `db:"cloud_credential_cloud_name"`

	// CredentialOwnerName is the owner of the model cloud credential.
	CredentialOwnerName string `db:"cloud_credential_owner_name"`

	// OwnerUUID is the uuid of the user that owns this model in the Juju controller.
	OwnerUUID string `db:"owner_uuid"`

	// OwnerName is the name of the model owner in the Juju controller.
	OwnerName string `db:"owner_name"`

	// ControllerUUID is the uuid of the controller that the model is in.
	ControllerUUID string `db:"controller_uuid"`
}

func (m *dbModel) toCoreModel() (coremodel.Model, error) {
	ownerName, err := user.NewName(m.OwnerName)
	if err != nil {
		return coremodel.Model{}, errors.Capture(err)
	}
	var credOwnerName user.Name
	if m.CredentialOwnerName != "" {
		credOwnerName, err = user.NewName(m.CredentialOwnerName)
		if err != nil {
			return coremodel.Model{}, errors.Capture(err)
		}
	}

	var cloudRegion string
	if m.CloudRegion.Valid {
		cloudRegion = m.CloudRegion.String
	}

	var credentialName string
	if m.CredentialName.Valid {
		credentialName = m.CredentialName.String
	}

	var credentialCloudName string
	if m.CredentialCloudName.Valid {
		credentialCloudName = m.CredentialCloudName.String
	}

	return coremodel.Model{
		Name:        m.Name,
		Life:        corelife.Value(m.Life),
		UUID:        coremodel.UUID(m.UUID),
		ModelType:   coremodel.ModelType(m.ModelType),
		Cloud:       m.CloudName,
		CloudType:   m.CloudType,
		CloudRegion: cloudRegion,
		Credential: credential.Key{
			Name:  credentialName,
			Cloud: credentialCloudName,
			Owner: credOwnerName,
		},
		Owner:     user.UUID(m.OwnerUUID),
		OwnerName: ownerName,
	}, nil
}

// dbInitialModel represents initial the model state written to the model table on
// model creation.
type dbInitialModel struct {
	// UUID is the universally unique identifier of the model.
	UUID string `db:"uuid"`

	// CloudUUID is the unique identifier for the cloud the model is on.
	CloudUUID string `db:"cloud_uuid"`

	// ModelType is the type of model.
	ModelType string `db:"model_type"`

	// LifeID the ID of the current state of the model.
	LifeID int `db:"life_id"`

	// Name is the human friendly name of the model.
	Name string `db:"name"`

	// OwnerUUID is the uuid of the user that owns this model in the Juju controller.
	OwnerUUID string `db:"owner_uuid"`
}

type dbNames struct {
	ModelName string `db:"name"`
	OwnerName string `db:"owner_name"`
}

// dbUUID represents a UUID.
type dbUUID struct {
	UUID string `db:"uuid"`
}

// dbControllerUUID represents the controller uuid value on a model record.
type dbControllerUUID struct {
	UUID string `db:"controller_uuid"`
}

// dbModelUUIDRef represents the model uuid in tables that have a foreign key on
// the model uuid.
type dbModelUUIDRef struct {
	ModelUUID string `db:"model_uuid"`
}

type dbModelUUID struct {
	UUID string `db:"uuid"`
}

type dbModelNameAndOwner struct {
	// Name is the model name.
	Name string `db:"name"`
	// OwnerUUID is the UUID of the model owner.
	OwnerUUID string `db:"owner_uuid"`
}

// dbModelType represents the model type from the model table.
type dbModelType struct {
	Type string `db:"type"`
}

// dbModelSummary stores the information from the model table for a model
// summary.
type dbModelSummary struct {
	// Name is the model name.
	Name string `db:"name"`
	// UUID is the model unique identifier.
	UUID string `db:"uuid"`
	// Type is the model type (e.g. IAAS or CAAS).
	Type string `db:"model_type"`
	// OwnerName is the tag of the user that owns the model.
	OwnerName string `db:"owner_name"`
	// Life is the current lifecycle state of the model.
	Life string `db:"life"`

	// CloudName is the name of the model cloud.
	CloudName string `db:"cloud_name"`
	// Cloud type is the models cloud type.
	CloudType string `db:"cloud_type"`
	// CloudRegion is the region of the model cloud.
	CloudRegion string `db:"cloud_region_name"`

	// CloudCredentialName is the name of the cloud credential.
	CloudCredentialName string `db:"cloud_credential_name"`
	// CloudCredentialCloudName is the name of the cloud the credential is for.
	CloudCredentialCloudName string `db:"cloud_credential_cloud_name"`
	// CloudCredentialOwnerName is the name of the cloud credential owner.
	CloudCredentialOwnerName string `db:"cloud_credential_owner_name"`

	// Access is the access level the supplied user has on this model
	Access permission.Access `db:"access_type"`

	// UserLastConnection is the last time this user has accessed this model
	UserLastConnection *time.Time `db:"time"`

	// IsControllerModel provides a boolean indication if we consider this
	// model to be the one that hosts this Juju controller.
	IsControllerModel bool `db:"is_controller_model"`

	// ControllerUUID is the UUID of the controller that the model is in. Don't
	// rely on this value always being set.
	ControllerUUID sql.NullString `db:"controller_uuid"`
}

// decodeModelSummary transforms a dbModelSummary into a coremodel.ModelSummary.
func (m dbModelSummary) decodeUserModelSummary() (coremodel.UserModelSummary, error) {
	ms, err := m.decodeModelSummary()
	if err != nil {
		return coremodel.UserModelSummary{}, errors.Capture(err)
	}
	return coremodel.UserModelSummary{
		ModelSummary:       ms,
		UserAccess:         m.Access,
		UserLastConnection: m.UserLastConnection,
	}, nil
}

// decodeModelSummary transforms a dbModelSummary into a coremodel.ModelSummary.
func (m dbModelSummary) decodeModelSummary() (coremodel.ModelSummary, error) {
	ownerName, err := user.NewName(m.OwnerName)
	if err != nil {
		return coremodel.ModelSummary{}, errors.Capture(err)
	}
	var credOwnerName user.Name
	if m.CloudCredentialOwnerName != "" {
		credOwnerName, err = user.NewName(m.CloudCredentialOwnerName)
		if err != nil {
			return coremodel.ModelSummary{}, errors.Capture(err)
		}
	}
	return coremodel.ModelSummary{
		Name:        m.Name,
		UUID:        coremodel.UUID(m.UUID),
		ModelType:   coremodel.ModelType(m.Type),
		CloudType:   m.CloudType,
		CloudName:   m.CloudName,
		CloudRegion: m.CloudRegion,
		CloudCredentialKey: credential.Key{
			Cloud: m.CloudCredentialCloudName,
			Owner: credOwnerName,
			Name:  m.CloudCredentialName,
		},
		ControllerUUID: m.ControllerUUID.String,
		IsController:   m.IsControllerModel,
		OwnerName:      ownerName,
		Life:           corelife.Value(m.Life),
	}, nil
}

// dbUserName represents a user name.
type dbUserName struct {
	Name string `db:"name"`
}

// dbUserUUID represents a user uuid.
type dbUserUUID struct {
	UUID string `db:"uuid"`
}

// dbModelUserInfo represents information about a user on a particular model.
type dbModelUserInfo struct {
	// Name is the username of the user.
	Name string `db:"name"`

	// DisplayName is a user-friendly name represent the user as.
	DisplayName string `db:"display_name"`

	// LastModelLogin is the last time the user logged in to the model.
	LastModelLogin time.Time `db:"time"`

	// AccessType is the access level the user has on the model.
	AccessType string `db:"access_type"`
}

// toModelUserInfo converts the result from the DB to the type
// access.ModelUserInfo.
func (info *dbModelUserInfo) toModelUserInfo() (coremodel.ModelUserInfo, error) {
	name, err := user.NewName(info.Name)
	if err != nil {
		return coremodel.ModelUserInfo{}, errors.Capture(err)
	}

	return coremodel.ModelUserInfo{
		Name:           name,
		DisplayName:    info.DisplayName,
		LastModelLogin: info.LastModelLogin,
		Access:         permission.Access(info.AccessType),
	}, nil
}

type dbReadOnlyModel struct {
	UUID              string `db:"uuid"`
	ControllerUUID    string `db:"controller_uuid"`
	Name              string `db:"name"`
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

type dbCloudType struct {
	Type string `db:"type"`
}

type dbName struct {
	Name string `db:"name"`
}

type dbModelActivated struct {
	Activated bool `db:"activated"`
}

type dbModelNamespace struct {
	UUID      string         `db:"model_uuid"`
	Namespace sql.NullString `db:"namespace"`
}

type dbCloudCredential struct {
	Name                string         `db:"cloud_name"`
	CloudRegionName     string         `db:"cloud_region_name"`
	CredentialName      sql.NullString `db:"cloud_credential_name"`
	CredentialOwnerName sql.NullString `db:"cloud_credential_owner_name"`
	CredentialCloudName string         `db:"cloud_credential_cloud_name"`
}

type dbModelCloudRegionCredential struct {
	CloudName           string         `db:"cloud"`
	CloudRegionName     string         `db:"cloud_region"`
	CredentialName      sql.NullString `db:"credential_name"`
	CredentialOwnerName sql.NullString `db:"credential_owner"`
}

type dbCloudRegionUUID struct {
	CloudRegionUUID string `db:"uuid"`
}

type dbUpdateCredentialResult struct {
	CloudUUID           string `db:"cloud_uuid"`
	CloudCredentialUUID string `db:"cloud_credential_uuid"`
}

type dbCredKey struct {
	CloudName           string `db:"cloud_name"`
	OwnerName           string `db:"owner_name"`
	CloudCredentialName string `db:"cloud_credential_name"`
}

type dbUpdateCredential struct {
	// UUID is the model uuid.
	UUID string `db:"uuid"`
	// CloudUUID is the cloud uuid.
	CloudUUID string `db:"cloud_uuid"`
	// CloudCredentialUUID is the cloud credential uuid.
	CloudCredentialUUID string `db:"cloud_credential_uuid"`
}

// dbPermission represents a permission in the system.
type dbPermission struct {
	// UUID is the unique identifier for the permission.
	UUID string `db:"uuid"`

	// GrantOn is the unique identifier of the permission target.
	// A name or UUID depending on the ObjectType.
	GrantOn string `db:"grant_on"`

	// GrantTo is the unique identifier of the user the permission
	// is granted to.
	GrantTo string `db:"grant_to"`

	// AccessType is a string version of core permission AccessType.
	AccessType string `db:"access_type"`

	// ObjectType is a string version of core permission ObjectType.
	ObjectType string `db:"object_type"`
}

// dbModelAgent represents a row from the controller model_agent table.
type dbModelAgent struct {
	// StreamID is the unique identifier for the agent stream that is being used
	// for model agents.
	StreamID int `db:"stream_id"`

	// TargetVersion describes the desired agent version that should be
	// being run in this model. It should not be considered "the" version that
	// is being run for every agent as each agent needs to upgrade to this
	// version.
	TargetVersion string `db:"target_version"`
}

type dbModelSecretBackend struct {
	// ModelUUID is the models unique identifier.
	ModelUUID string `db:"model_uuid"`
	// SecretBackendUUID is the secret backends unique identifier.
	SecretBackendUUID string `db:"secret_backend_uuid"`
}

// dbModelState is used to represent a single row from the
// v_model_status_state view. This information is used to feed a model's status.
type dbModelState struct {
	Destroying              bool   `db:"destroying"`
	CredentialInvalid       bool   `db:"cloud_credential_invalid"`
	CredentialInvalidReason string `db:"cloud_credential_invalid_reason"`
	Migrating               bool   `db:"migrating"`
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

type dbModelCloudCredentialUUID struct {
	ModelUUID      coremodel.UUID  `db:"uuid"`
	CloudUUID      corecloud.UUID  `db:"cloud_uuid"`
	CredentialUUID credential.UUID `db:"cloud_credential_uuid"`
}
