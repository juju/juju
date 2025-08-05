// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/cloud"
	coredatabase "github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	cloudstate "github.com/juju/juju/domain/cloud/state"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/modelprovider"
	"github.com/juju/juju/internal/errors"
)

// State is used to access the database.
type State struct {
	*domain.StateBase
}

// NewState creates a state to access the database.
func NewState(factory coredatabase.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// GetModelCloudAndCredential returns the cloud, cloud region
// and credential for the given model.
// The following errors are possible:
// - [modelerrors.NotFound] when the model does not exist.
func (st *State) GetModelCloudAndCredential(ctx context.Context, uuid coremodel.UUID) (*cloud.Cloud, string, *modelprovider.CloudCredentialInfo, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, "", nil, errors.Capture(err)
	}

	modelUUID := modelUUID{
		UUID: uuid,
	}

	credStmt, err := st.Prepare(`
SELECT    ca.auth_type AS &cloudCredentialWithAttribute.auth_type,
          ca.attribute_key AS &cloudCredentialWithAttribute.attribute_key,
          ca.attribute_value AS &cloudCredentialWithAttribute.attribute_value,
          m.cloud_uuid AS &cloudCredentialWithAttribute.cloud_uuid,
          m.cloud_region_name AS &cloudCredentialWithAttribute.cloud_region_name
FROM      v_model m
LEFT JOIN v_cloud_credential_attribute ca ON ca.uuid = m.cloud_credential_uuid
WHERE     m.uuid = $modelUUID.uuid
`, cloudCredentialWithAttribute{}, modelUUID)
	if err != nil {
		return nil, "", nil, errors.Capture(err)
	}

	var cld cloud.Cloud
	rows := []cloudCredentialWithAttribute{}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, credStmt, modelUUID).GetAll(&rows)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf("credential for model %q not found", uuid).Add(modelerrors.NotFound)
		} else if err != nil {
			return errors.Errorf("getting cloud credential for model %q: %w", uuid, err)
		}
		cld, err = cloudstate.GetCloudForUUID(ctx, st, tx, rows[0].CloudUUID)
		if err != nil {
			return errors.Errorf("getting cloud for model %q: %w", uuid, err)
		}
		return nil
	})
	if err != nil {
		return nil, "", nil, errors.Capture(err)
	}
	cred := &modelprovider.CloudCredentialInfo{
		AuthType:   cloud.AuthType(rows[0].AuthType),
		Attributes: make(map[string]string, len(rows)),
	}
	for _, row := range rows {
		cred.Attributes[row.AttributeKey] = row.AttributeValue
	}
	return &cld, rows[0].CloudRegionName, cred, nil
}
