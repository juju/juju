// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"

	coreremoteapplication "github.com/juju/juju/core/remoteapplication"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	domainlife "github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/status"
	statuserrors "github.com/juju/juju/domain/status/errors"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// GetApplicationUUIDForOffer returns the UUID of the application that the
// specified offer belongs to.
func (st *ModelState) GetApplicationUUIDForOffer(ctx context.Context, oUUID string) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	ident := offerUUID{OfferUUID: oUUID}

	// NOTE: In theory, this could return multiple results. However, we have
	// a trigger in place that ensures offer_endpoints for an offer are always
	// bound to the same application. So we can just return the first result.
	stmt, err := st.Prepare(`
SELECT ae.application_uuid AS &applicationUUID.uuid
FROM   offer AS o
JOIN   offer_endpoint AS oe ON o.uuid = oe.offer_uuid
JOIN   application_endpoint AS ae ON oe.endpoint_uuid = ae.uuid
WHERE  o.uuid = $offerUUID.uuid
	`, applicationUUID{}, ident)

	if err != nil {
		return "", errors.Capture(err)
	}

	var result applicationUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ident).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("offer %q not found", oUUID).Add(crossmodelrelationerrors.OfferNotFound)
		} else if err != nil {
			return errors.Capture(err)
		}
		return nil
	})

	if err != nil {
		return "", errors.Capture(err)
	}
	return result.UUID, nil
}

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

// GetRemoteApplicationOffererStatuses returns the statuses of all remote
// application offerers in the model, indexed by application name.
func (st *ModelState) GetRemoteApplicationOffererStatuses(ctx context.Context) (map[string]status.RemoteApplicationOfferer, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT
  a.name           AS &fullRemoteApplicationStatus.name,
  aro.life_id      AS &fullRemoteApplicationStatus.life_id,
  aros.status_id   AS &fullRemoteApplicationStatus.status_id,
  aros.message     AS &fullRemoteApplicationStatus.message,
  aros.data        AS &fullRemoteApplicationStatus.data,
  aros.updated_at  AS &fullRemoteApplicationStatus.updated_at,
  aro.offer_url    AS &fullRemoteApplicationStatus.offer_url,
  cr."name"        AS &fullRemoteApplicationStatus.endpoint_name,
  cr.interface     AS &fullRemoteApplicationStatus.endpoint_interface,
  crr."name"       AS &fullRemoteApplicationStatus.endpoint_role,
  cr.capacity      AS &fullRemoteApplicationStatus.endpoint_limit,
  re.relation_uuid AS &fullRemoteApplicationStatus.relation_uuid
FROM      application AS a
JOIN      application_remote_offerer        AS aro  ON a.uuid = aro.application_uuid
JOIN      application_remote_offerer_status AS aros ON aro.uuid = aros.application_remote_offerer_uuid
-- No need to left join, since a remote app needs at least one endpoint to make sense.
JOIN      application_endpoint              AS ae   ON a.uuid = ae.application_uuid
JOIN      charm_relation                    AS cr   ON ae.charm_relation_uuid = cr.uuid
JOIN      charm_relation_role               AS crr  ON cr.role_id = crr.id
-- Left join, since we don't know if any relations exist
LEFT JOIN v_relation_endpoint               AS re   ON re.application_uuid = a.uuid
`, fullRemoteApplicationStatus{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var statuses []fullRemoteApplicationStatus
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&statuses)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return decodeFullRemoteApplicationStatuses(statuses)
}

func decodeFullRemoteApplicationStatuses(statuses []fullRemoteApplicationStatus) (map[string]status.RemoteApplicationOfferer, error) {
	result := make(map[string]status.RemoteApplicationOfferer)
	seenRelationsByApp := make(map[string]set.Strings)
	seenEndpointsByApp := make(map[string]set.Strings)

	for _, s := range statuses {
		if app, ok := result[s.Name]; !ok {
			statusType, err := status.DecodeWorkloadStatus(s.StatusID)
			if err != nil {
				return nil, errors.Errorf("decoding workload status ID for application %q: %w", s.Name, err)
			}

			var relations []string
			if s.RelationUUID.Valid {
				relations = append(relations, s.RelationUUID.V)
				seenRelationsByApp[s.Name] = set.NewStrings(s.RelationUUID.V)
			}
			seenEndpointsByApp[s.Name] = set.NewStrings(s.EndpointName)

			result[s.Name] = status.RemoteApplicationOfferer{
				Status: status.StatusInfo[status.WorkloadStatusType]{
					Status:  statusType,
					Message: s.Message,
					Data:    s.Data,
					Since:   s.UpdatedAt,
				},
				OfferURL: s.OfferURL,
				Life:     s.LifeID,
				Endpoints: []status.Endpoint{{
					Name:      s.EndpointName,
					Role:      s.EndpointRole,
					Interface: s.EndpointInterface,
					Limit:     s.EndpointLimit,
				}},
				Relations: relations,
			}
		} else {
			if s.RelationUUID.Valid {
				seenRelations := seenRelationsByApp[s.Name]
				if !seenRelations.Contains(s.RelationUUID.V) {
					app.Relations = append(app.Relations, s.RelationUUID.V)
					seenRelations.Add(s.RelationUUID.V)
				}
			}
			seenEndpoints := seenEndpointsByApp[s.Name]
			if !seenEndpoints.Contains(s.EndpointName) {
				app.Endpoints = append(app.Endpoints, status.Endpoint{
					Name:      s.EndpointName,
					Role:      s.EndpointRole,
					Interface: s.EndpointInterface,
					Limit:     s.EndpointLimit,
				})
				seenEndpoints.Add(s.EndpointName)
			}
			result[s.Name] = app
		}
	}

	return result, nil
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
