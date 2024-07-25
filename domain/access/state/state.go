// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/domain/access"
	accesserrors "github.com/juju/juju/domain/access/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
)

// State represents a type for interacting with the underlying state.
// Composes both user and permission state, so we can interact with both
// from the single state, whilst also keeping the concerns separate.
type State struct {
	*UserState
	*PermissionState
}

// NewState returns a new State for interacting with the underlying state.
func NewState(factory database.TxnRunnerFactory, logger logger.Logger) *State {
	return &State{
		UserState:       NewUserState(factory),
		PermissionState: NewPermissionState(factory, logger),
	}
}

// GetModelUsers will retrieve basic information about all users with
// permissions on the given model UUID.
// If the model cannot be found it will return modelerrors.NotFound.
// If no permissions can be found on the model it will return
// accesserrors.PermissionNotValid.
func (st *State) GetModelUsers(ctx context.Context, apiUser string, modelUUID coremodel.UUID) ([]access.ModelUserInfo, error) {
	db, err := st.UserState.DB()
	if err != nil {
		return nil, errors.Annotate(err, "getting DB access")
	}
	q := `
	SELECT    (u.name, u.display_name, mll.time, p.access_type) AS (&dbModelUserInfo.*)
	FROM      v_user_auth u
	JOIN      v_permission p ON u.uuid = p.grant_to AND p.grant_on = $dbModelUUID.uuid
	LEFT JOIN model_last_login mll ON mll.user_uuid = u.uuid AND mll.model_uuid = p.grant_on
	WHERE     u.disabled = false
	AND       u.removed = false
	`

	var userInfo []access.ModelUserInfo
	uuid := dbModelUUID{UUID: modelUUID.String()}
	args := []any{uuid}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		_, err := st.authorizedOnTarget(ctx, tx, apiUser, modelUUID.String())
		if errors.Is(err, accesserrors.PermissionNotValid) {
			args = append(args, userName{Name: apiUser})
			q += `AND u.name = $userName.name`
		} else if err != nil {
			return errors.Annotate(err, "getting query results")
		}

		stmt, err := st.UserState.Prepare(q, append(args, dbModelUserInfo{})...)
		if err != nil {
			return errors.Annotatef(err, "preparing select model user info statement")
		}
		var modelUsers []dbModelUserInfo
		err = tx.Query(ctx, stmt, args...).GetAll(&modelUsers)
		if errors.Is(err, sqlair.ErrNoRows) {
			exists, err := st.UserState.checkModelExists(ctx, tx, modelUUID)
			if err != nil {
				return errors.Trace(err)
			} else if !exists {
				return modelerrors.NotFound
			}
			return accesserrors.PermissionNotValid
		} else if err != nil {
			return errors.Trace(err)
		}

		for _, modelUser := range modelUsers {
			if modelUser.Name == permission.EveryoneTagName {
				externalModelUsers, err := st.getExternalModelUsers(ctx, tx, uuid, modelUser.AccessType)
				if err != nil {
					return errors.Annotate(err, "getting external users")
				}
				userInfo = append(userInfo, externalModelUsers...)
			} else {
				userInfo = append(userInfo, modelUser.toModelUserInfo())
			}
		}

		return nil
	})
	if err != nil {
		return nil, errors.Annotatef(err, "getting users for model %q", modelUUID)
	}

	return userInfo, nil
}

func (st *State) getExternalModelUsers(ctx context.Context, tx *sqlair.TX, modelUUID dbModelUUID, everyoneAccess string) ([]access.ModelUserInfo, error) {
	q := `
SELECT    (u.name, u.display_name, mll.time) AS (&dbModelUserInfo.*)
FROM      v_user_auth u
LEFT JOIN model_last_login mll ON mll.user_uuid = u.uuid AND mll.model_uuid = $dbModelUUID.uuid
WHERE     u.disabled = false
AND       u.removed = false
AND       u.external = true
`
	stmt, err := st.UserState.Prepare(q, dbModelUserInfo{}, modelUUID)
	if err != nil {
		return nil, errors.Annotatef(err, "preparing select model user info statement")
	}

	var modelUsers []dbModelUserInfo
	err = tx.Query(ctx, stmt, modelUUID).GetAll(&modelUsers)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	var userInfo []access.ModelUserInfo
	for _, modelUser := range modelUsers {
		if modelUser.Name != permission.EveryoneTagName {
			modelUser.AccessType = everyoneAccess
			userInfo = append(userInfo, modelUser.toModelUserInfo())
		}
	}

	return userInfo, nil
}
