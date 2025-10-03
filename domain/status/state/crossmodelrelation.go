// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"

	coreremoteapplication "github.com/juju/juju/core/remoteapplication"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	domainlife "github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/status"
	statuserrors "github.com/juju/juju/domain/status/errors"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// GetRemoteApplicationOffererUUIDByName returns the UUID for the named for the remote
// application wrapping the named application
func (st *ModelState) GetRemoteApplicationOffererUUIDByName(
	ctx context.Context,
	name string,
) (coreremoteapplication.UUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var uuid coreremoteapplication.UUID
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		uuid, err = st.lookupRemoteApplicationOfferer(ctx, tx, name)
		return err
	}); err != nil {
		return "", errors.Capture(err)
	}

	return uuid, nil
}

// GetRemoteApplicationOffererStatus returns the status of the specified remote
// application in the local model.
func (st *ModelState) GetRemoteApplicationOffererStatus(
	ctx context.Context,
	uuid string,
) (status.StatusInfo[status.WorkloadStatusType], error) {
	db, err := st.DB(ctx)
	if err != nil {
		return status.StatusInfo[status.WorkloadStatusType]{}, errors.Capture(err)
	}

	ident := remoteApplicationUUID{RemoteApplicationUUID: uuid}

	query, err := st.Prepare(`
SELECT &statusInfo.*
FROM application_remote_offerer_status
WHERE application_remote_offerer_uuid = $remoteApplicationUUID.uuid
`, statusInfo{}, ident)
	if err != nil {
		return status.StatusInfo[status.WorkloadStatusType]{}, errors.Capture(err)
	}

	var statusInfo statusInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkRemoteApplicationNotDead(ctx, tx, ident); err != nil {
			return errors.Capture(err)
		}
		err := tx.Query(ctx, query, ident).Get(&statusInfo)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("remote application %q status not set", uuid).
				Add(statuserrors.RemoteApplicationStatusNotFound)
		} else if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return status.StatusInfo[status.WorkloadStatusType]{}, errors.Capture(err)
	}

	statusType, err := status.DecodeWorkloadStatus(statusInfo.StatusID)
	if err != nil {
		return status.StatusInfo[status.WorkloadStatusType]{}, errors.Capture(err)
	}

	return status.StatusInfo[status.WorkloadStatusType]{
		Status:  statusType,
		Message: statusInfo.Message,
		Data:    statusInfo.Data,
		Since:   statusInfo.UpdatedAt,
	}, nil
}

// SetRemoteApplicationOffererStatus sets the status of the specified remote
// application in the local model.
func (st *ModelState) SetRemoteApplicationOffererStatus(
	ctx context.Context,
	remoteApplicationUUID string,
	sts status.StatusInfo[status.WorkloadStatusType],
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	statusID, err := status.EncodeWorkloadStatus(sts.Status)
	if err != nil {
		return errors.Capture(err)
	}

	statusInfo := remoteApplicationStatus{
		RemoteApplicationUUID: remoteApplicationUUID,
		StatusID:              statusID,
		Message:               sts.Message,
		Data:                  sts.Data,
		UpdatedAt:             sts.Since,
	}

	stmt, err := st.Prepare(`
INSERT INTO application_remote_offerer_status (*) VALUES ($remoteApplicationStatus.*)
ON CONFLICT(application_remote_offerer_uuid) DO UPDATE SET
    status_id = excluded.status_id,
    message = excluded.message,
    updated_at = excluded.updated_at,
    data = excluded.data;
`, statusInfo)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, statusInfo).Run(); internaldatabase.IsErrConstraintForeignKey(err) {
			return errors.Errorf("%w: %q", crossmodelrelationerrors.RemoteApplicationNotFound, remoteApplicationUUID)
		} else if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (st *ModelState) lookupRemoteApplicationOfferer(ctx context.Context, tx *sqlair.TX, name string) (coreremoteapplication.UUID, error) {
	type remoteApplicationUUID struct {
		RemoteApplicationUUID sql.Null[string] `db:"uuid"`
	}
	appIdent := applicationName{Name: name}

	queryRemoteAppStmt, err := st.Prepare(`
SELECT    aro.uuid AS &remoteApplicationUUID.uuid
FROM      application AS a
-- left join ensures we can distinguish between an application that doesn't
-- exist and one that is not remote
LEFT JOIN application_remote_offerer AS aro
WHERE     a.name = $applicationName.name `,
		remoteApplicationUUID{}, appIdent)
	if err != nil {
		return "", errors.Capture(err)
	}

	var result remoteApplicationUUID

	err = tx.Query(ctx, queryRemoteAppStmt, appIdent).Get(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("application %q not found", name).Add(applicationerrors.ApplicationNotFound)
	} else if err != nil {
		return "", errors.Errorf("looking up UUID for remote application %q: %w", name, err)
	} else if !result.RemoteApplicationUUID.Valid {
		return "", errors.Errorf("application %q is not remote", name).Add(crossmodelrelationerrors.ApplicationNotRemote)
	}

	return coreremoteapplication.UUID(result.RemoteApplicationUUID.V), nil
}

func (st *ModelState) checkRemoteApplicationNotDead(ctx context.Context, tx *sqlair.TX, ident remoteApplicationUUID) error {
	type life struct {
		LifeID domainlife.Life `db:"life_id"`
	}

	query := `
SELECT &life.*
FROM application_remote_offerer
WHERE uuid = $remoteApplicationUUID.uuid;
`
	stmt, err := st.Prepare(query, ident, life{})
	if err != nil {
		return errors.Errorf("preparing query for remote application %q: %w", ident.RemoteApplicationUUID, err)
	}

	var result life
	err = tx.Query(ctx, stmt, ident).Get(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return crossmodelrelationerrors.RemoteApplicationNotFound
	} else if err != nil {
		return errors.Errorf("checking if remote application %q exists: %w", ident.RemoteApplicationUUID, err)
	}

	switch result.LifeID {
	case domainlife.Dead:
		return crossmodelrelationerrors.RemoteApplicationIsDead
	default:
		return nil
	}
}
