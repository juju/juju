package state

import (
	"time"

	corelife "github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
)

// stateModel represents a model pulled from the controller database model
// table.
type stateModel struct {
	Name            string `db:"name"`
	Life            string `db:"life"`
	ID              string `db:"uuid"`
	ModelType       string `db:"model_type"`
	AgentVersion    string `db:"target_agent_version"`
	Cloud           string `db:"cloud_name"`
	CloudID         string `db:"cloud_uuid"`
	CloudType       string `db:"cloud_type"`
	CloudRegion     string `db:"cloud_region_name"`
	CredentialName  string `db:"cloud_credential_name"`
	CredentialID    string `db:"cloud_credential_uuid"`
	CredentialCloud string `db:"cloud_credential_cloud_name"`
	CredentialOwner string `db:"cloud_credential_owner_name"`
	OwnerID         string `db:"owner_uuid"`
	OwnerName       string `db:"owner_name"`
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
