// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"database/sql"
	"time"

	corecloud "github.com/juju/juju/core/cloud"
	"github.com/juju/juju/core/credential"
	corelife "github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/internal/errors"
)

// dbModel represents the state of a model.
type dbModel struct {
	// Name is the human friendly name of the model.
	Name string `db:"name"`

	// Qualifier disambiguates the model name.
	Qualifier string `db:"qualifier"`

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

	// ControllerUUID is the uuid of the controller that the model is in.
	ControllerUUID string `db:"controller_uuid"`
}

func (m *dbModel) toCoreModel() (coremodel.Model, error) {
	var credOwnerName user.Name
	if m.CredentialOwnerName != "" {
		var err error
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
		Qualifier:   coremodel.Qualifier(m.Qualifier),
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

	// Qualifier is used to disambiguate the model name.
	Qualifier string `db:"qualifier"`
}

type dbNames struct {
	ModelName      string `db:"name"`
	ModelQualifier string `db:"qualifier"`
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

type dbModelNameAndQualifier struct {
	// Name is the model name.
	Name string `db:"name"`
	// Qualifier is the model qualifier.
	Qualifier string `db:"qualifier"`
}

// dbModelType represents the model type from the model table.
type dbModelType struct {
	Type string `db:"type"`
}

// dbUserModelSummary is summary of the information for a model from the
// perspective of a user that has access to the model.
type dbUserModelSummary struct {
	// Access is the access level the supplied user has on this model.
	Access permission.Access `db:"access_type"`

	// UserLastConnection is the last time this user has accessed this model.
	UserLastConnection *time.Time `db:"time"`

	// Life is the current model's life value.
	Life string `db:"life"`

	// Model state related members

	// Destroying indicates if the model is in the process of being destroyed.
	Destroying bool `db:"destroying"`

	// CredentialInvalid indicates if the model is using a credential that is
	// invalid.
	CredentialInvalid bool `db:"cloud_credential_invalid"`

	// CredentialInvalidReason is the reason the credential is invalid.
	CredentialInvalidReason string `db:"cloud_credential_invalid_reason"`

	// Migrating indicates if the model is in the process of being migrated.
	Migrating bool `db:"migrating"`
}

// dbModelSummary represents the information about a model for a summary that
// isn't available from the model's own database.
type dbModelSummary struct {
	// -- Model info related members --

	// Life is the current model's life value.
	Life string `db:"life"`

	// -- Model state related members --

	// Destroying indicates if the model is in the process of being destroyed.
	Destroying bool `db:"destroying"`

	// CredentialInvalid indicates if the model is using a credential that is
	// invalid.
	CredentialInvalid bool `db:"cloud_credential_invalid"`

	// CredentialInvalidReason is the reason the credential is invalid.
	CredentialInvalidReason string `db:"cloud_credential_invalid_reason"`

	// Migrating indicates if the model is in the process of being migrated.
	Migrating bool `db:"migrating"`
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

type dbModelCredentialInvalidStatus struct {
	CredentialInvalid bool `db:"cloud_credential_invalid"`
}
