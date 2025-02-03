// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/instance"
	corelife "github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
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

	// AgentVersion is the target version for agents running under this model.
	AgentVersion string `db:"target_agent_version"`

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
}

func (m *dbModel) toCoreModel() (coremodel.Model, error) {
	agentVersion, err := version.Parse(m.AgentVersion)
	if err != nil {
		return coremodel.Model{}, fmt.Errorf("parsing model %q agent version %q: %w", m.UUID, agentVersion, err)
	}
	ownerName, err := user.NewName(m.OwnerName)
	if err != nil {
		return coremodel.Model{}, errors.Trace(err)
	}
	var credOwnerName user.Name
	if m.CredentialOwnerName != "" {
		credOwnerName, err = user.NewName(m.CredentialOwnerName)
		if err != nil {
			return coremodel.Model{}, errors.Trace(err)
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
		Name:         m.Name,
		Life:         corelife.Value(m.Life),
		UUID:         coremodel.UUID(m.UUID),
		ModelType:    coremodel.ModelType(m.ModelType),
		AgentVersion: agentVersion,
		Cloud:        m.CloudName,
		CloudType:    m.CloudType,
		CloudRegion:  cloudRegion,
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
	Type string `db:"model_type"`
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

	// AgentVersion is the agent version for this model.
	AgentVersion string `db:"target_agent_version"`
}

// decodeModelSummary transforms a dbModelSummary into a coremodel.ModelSummary.
func (m dbModelSummary) decodeUserModelSummary(controllerInfo dbController) (coremodel.UserModelSummary, error) {
	ms, err := m.decodeModelSummary(controllerInfo)
	if err != nil {
		return coremodel.UserModelSummary{}, errors.Trace(err)
	}
	return coremodel.UserModelSummary{
		ModelSummary:       ms,
		UserAccess:         m.Access,
		UserLastConnection: m.UserLastConnection,
	}, nil
}

// decodeModelSummary transforms a dbModelSummary into a coremodel.ModelSummary.
func (m dbModelSummary) decodeModelSummary(controllerInfo dbController) (coremodel.ModelSummary, error) {
	var agentVersion version.Number
	if m.AgentVersion != "" {
		var err error
		agentVersion, err = version.Parse(m.AgentVersion)
		if err != nil {
			return coremodel.ModelSummary{}, errors.Annotatef(
				err, "parsing model %q agent version %q", m.Name, agentVersion,
			)
		}
	}
	ownerName, err := user.NewName(m.OwnerName)
	if err != nil {
		return coremodel.ModelSummary{}, errors.Trace(err)
	}
	var credOwnerName user.Name
	if m.CloudCredentialOwnerName != "" {
		credOwnerName, err = user.NewName(m.CloudCredentialOwnerName)
		if err != nil {
			return coremodel.ModelSummary{}, errors.Trace(err)
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
		ControllerUUID: controllerInfo.UUID,
		IsController:   m.UUID == controllerInfo.ModelUUID,
		OwnerName:      ownerName,
		Life:           corelife.Value(m.Life),
		AgentVersion:   agentVersion,
	}, nil
}

// dbController represents a row from the controller table.
type dbController struct {
	ModelUUID string `db:"model_uuid"`
	UUID      string `db:"uuid"`
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
		return coremodel.ModelUserInfo{}, errors.Trace(err)
	}

	return coremodel.ModelUserInfo{
		Name:           name,
		DisplayName:    info.DisplayName,
		LastModelLogin: info.LastModelLogin,
		Access:         permission.Access(info.AccessType),
	}, nil
}

type dbReadOnlyModel struct {
	UUID               string         `db:"uuid"`
	ControllerUUID     string         `db:"controller_uuid"`
	Name               string         `db:"name"`
	Type               string         `db:"type"`
	TargetAgentVersion sql.NullString `db:"target_agent_version"`
	Cloud              string         `db:"cloud"`
	CloudType          string         `db:"cloud_type"`
	CloudRegion        string         `db:"cloud_region"`
	CredentialOwner    string         `db:"credential_owner"`
	CredentialName     string         `db:"credential_name"`
	IsControllerModel  bool           `db:"is_controller_model"`
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
	CredentialName      sql.NullString `db:"cloud_credential_name"`
	CredentialOwnerName sql.NullString `db:"cloud_credential_owner_name"`
	CredentialCloudName string         `db:"cloud_credential_cloud_name"`
}

type dbCloudOwner struct {
	Name      string `db:"name"`
	OwnerName string `db:"owner_name"`
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
	// UUID is the models unique identifier.
	UUID string `db:"model_uuid"`
	// PreviousVersion describes the agent version that was in use before the
	// current TargetVersion.
	PreviousVersion string `db:"previous_version"`
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

type dbModelConstraint struct {
	ModelUUID      string `db:"model_uuid"`
	ConstraintUUID string `db:"constraint_uuid"`
}

// dbConstraint represents a single row within the v_constraint view.
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
	consValue constraints.Value) dbConstraintInsert {
	return dbConstraintInsert{
		UUID: uuid.String(),
		Arch: sql.NullString{
			String: deref(consValue.Arch),
			Valid:  consValue.Arch != nil,
		},
		CPUCores: sql.NullInt64{
			Int64: int64(deref(consValue.CpuCores)),
			Valid: consValue.CpuCores != nil,
		},
		CPUPower: sql.NullInt64{
			Int64: int64(deref(consValue.CpuPower)),
			Valid: consValue.CpuPower != nil,
		},
		Mem: sql.NullInt64{
			Int64: int64(deref(consValue.Mem)),
			Valid: consValue.Mem != nil,
		},
		RootDisk: sql.NullInt64{
			Int64: int64(deref(consValue.RootDisk)),
			Valid: consValue.RootDisk != nil,
		},
		RootDiskSource: sql.NullString{
			String: deref(consValue.RootDiskSource),
			Valid:  consValue.RootDiskSource != nil,
		},
		InstanceRole: sql.NullString{
			String: deref(consValue.InstanceRole),
			Valid:  consValue.InstanceRole != nil,
		},
		InstanceType: sql.NullString{
			String: deref(consValue.InstanceType),
			Valid:  consValue.InstanceType != nil,
		},
		VirtType: sql.NullString{
			String: deref(consValue.VirtType),
			Valid:  consValue.VirtType != nil,
		},
		AllocatePublicIP: sql.NullBool{
			Bool:  deref(consValue.AllocatePublicIP),
			Valid: consValue.AllocatePublicIP != nil,
		},
		ImageID: sql.NullString{
			String: deref(consValue.ImageID),
			Valid:  consValue.ImageID != nil,
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
		return *new(T)
	}
	return *i
}

func (c dbConstraint) toValue(tags []dbConstraintTag, spaces []dbConstraintSpace, zones []dbConstraintZone) (constraints.Value, error) {
	consVal := constraints.Value{}
	if c.Arch.Valid {
		consVal.Arch = &c.Arch.String
	}
	if c.CPUCores.Valid {
		consVal.CpuCores = ptr(uint64(c.CPUCores.Int64))
	}
	if c.CPUPower.Valid {
		consVal.CpuPower = ptr(uint64(c.CPUPower.Int64))
	}
	if c.Mem.Valid {
		consVal.Mem = ptr(uint64(c.Mem.Int64))
	}
	if c.RootDisk.Valid {
		consVal.RootDisk = ptr(uint64(c.RootDisk.Int64))
	}
	if c.RootDiskSource.Valid {
		consVal.RootDiskSource = &c.RootDiskSource.String
	}
	if c.InstanceRole.Valid {
		consVal.InstanceRole = &c.InstanceRole.String
	}
	if c.InstanceType.Valid {
		consVal.InstanceType = &c.InstanceType.String
	}
	if c.VirtType.Valid {
		consVal.VirtType = &c.VirtType.String
	}
	if c.AllocatePublicIP.Valid {
		consVal.AllocatePublicIP = &c.AllocatePublicIP.Bool
	}
	if c.ImageID.Valid {
		consVal.ImageID = &c.ImageID.String
	}
	if c.ContainerType.Valid {
		containerType := instance.ContainerType(c.ContainerType.String)
		consVal.Container = &containerType
	}

	consTags := make([]string, 0, len(tags))
	for _, tag := range tags {
		consTags = append(consTags, tag.Tag)
	}
	// Only set constraint tags if there are tags in the database value.
	if len(consTags) != 0 {
		consVal.Tags = &consTags
	}

	consSpaces := make([]string, 0, len(spaces))
	for _, space := range spaces {
		consSpaces = append(consSpaces, space.Space)
	}
	// Only set constraint spaces if there are spaces in the database value.
	if len(consSpaces) != 0 {
		consVal.Spaces = &consSpaces
	}

	consZones := make([]string, 0, len(zones))
	for _, zone := range zones {
		consZones = append(consZones, zone.Zone)
	}
	// Only set constraint zones if there are zones in the database value.
	if len(consZones) != 0 {
		consVal.Zones = &consZones
	}

	return consVal, nil
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

type dbConstraintTag struct {
	ConstraintUUID string `db:"constraint_uuid"`
	Tag            string `db:"tag"`
}

type dbConstraintSpace struct {
	ConstraintUUID string `db:"constraint_uuid"`
	Space          string `db:"space"`
}

type dbConstraintZone struct {
	ConstraintUUID string `db:"constraint_uuid"`
	Zone           string `db:"zone"`
}

// dbConstraintUUID represents a constraint uuid within the database.
type dbConstraintUUID struct {
	UUID string `db:"uuid"`
}
