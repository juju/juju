// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	"github.com/juju/version/v2"

	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/life"
	corelife "github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/user"
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

	// CloudID is the ID of the model's cloud from the cloud table.
	CloudID string `db:"cloud_uuid"`

	// CloudType is the type of the underlying cloud (e.g. lxd, azure, ...)
	CloudType string `db:"cloud_type"`

	// CloudRegion is the region that the model will use in the cloud.
	CloudRegion string `db:"cloud_region_name"`

	// CredentialName is the name of the model cloud credential.
	CredentialName string `db:"cloud_credential_name"`

	// CredentialID is the ID of the model's credential from the
	// cloud_credential table.
	CredentialID string `db:"cloud_credential_uuid"`

	// CredentialCloudName is the cloud name that the model cloud credential applies to.
	CredentialCloudName string `db:"cloud_credential_cloud_name"`

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
	return coremodel.Model{
		Name:         m.Name,
		Life:         life.Value(m.Life),
		UUID:         coremodel.UUID(m.UUID),
		ModelType:    coremodel.ModelType(m.ModelType),
		AgentVersion: agentVersion,
		Cloud:        m.CloudName,
		CloudType:    m.CloudType,
		CloudRegion:  m.CloudRegion,
		Credential: credential.Key{
			Name:  m.CredentialName,
			Cloud: m.CredentialCloudName,
			Owner: m.CredentialOwnerName,
		},
		Owner:     user.UUID(m.OwnerUUID),
		OwnerName: m.OwnerName,
	}, nil
}

// dbModelUUID represents the controller model uuid from the controller table.
type dbModelUUID struct {
	ModelUUID string `db:"model_uuid"`
}

// lifeList represents a list of life values, to be used to filter returned
// models.
type lifeList []corelife.Value

// modelIDList represents a list of model IDs, to be used to filter returned
// models.
type modelIDList []coremodel.UUID

// userModelLastLogin represents a row from the model_last_login table.
type userModelLastLogin struct {
	UserID    string     `db:"user_uuid"`
	LastLogin *time.Time `db:"time"`
}
