// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/credential"
	corelife "github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
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
	AgentVersion string `db:"target_version"`

	// CloudName is the name of the cloud to associate with the model.
	CloudName string `db:"cloud_name"`

	// CloudType is the type of the underlying cloud (e.g. lxd, azure, ...)
	CloudType string `db:"cloud_type"`

	// CloudRegion is the region that the model will use in the cloud.
	CloudRegion string `db:"cloud_region_name"`

	// CredentialName is the name of the model cloud credential.
	CredentialName string `db:"cloud_credential_name"`

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
	return coremodel.Model{
		Name:         m.Name,
		Life:         corelife.Value(m.Life),
		UUID:         coremodel.UUID(m.UUID),
		ModelType:    coremodel.ModelType(m.ModelType),
		AgentVersion: agentVersion,
		Cloud:        m.CloudName,
		CloudType:    m.CloudType,
		CloudRegion:  m.CloudRegion,
		Credential: credential.Key{
			Name:  m.CredentialName,
			Cloud: m.CredentialCloudName,
			Owner: credOwnerName,
		},
		Owner:     user.UUID(m.OwnerUUID),
		OwnerName: ownerName,
	}, nil
}

// dbModelUUID represents the controller model uuid from the controller table.
type dbModelUUID struct {
	ModelUUID string `db:"model_uuid"`
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
