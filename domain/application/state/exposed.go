// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/internal/errors"
)

// IsApplicationExposed returns whether the provided application is exposed or
// not.
func (st *State) IsApplicationExposed(ctx context.Context, appID coreapplication.ID) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Capture(err)
	}

	ident := applicationID{ID: appID}
	query := `
SELECT COUNT(*) AS &countResult.count
FROM v_application_exposed_endpoint
WHERE application_uuid = $applicationID.uuid;
	`
	stmt, err := st.Prepare(query, countResult{}, ident)
	if err != nil {
		return false, errors.Errorf("preparing application exposed query: %w", err)
	}

	var count countResult
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, ident).Get(&count); err != nil {
			return errors.Errorf("checking if application %q is exposed: %w", appID, err)
		}
		return nil
	})

	if err != nil {
		return false, errors.Capture(err)
	}
	return count.Count > 0, nil
}

// GetExposedEndpoints returns a map where keys are endpoint names (or the ""
// value which represents all endpoints) and values are ExposedEndpoint
// instances that specify which sources (spaces or CIDRs) can access the
// opened ports for each endpoint once the application is exposed.
func (st *State) GetExposedEndpoints(ctx context.Context, appID coreapplication.ID) (map[string]application.ExposedEndpoint, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	ident := applicationID{ID: appID}
	query := `
SELECT 
    cr.name AS &endpointCIDRsSpaces.name,
    cidr AS &endpointCIDRsSpaces.cidr,
    e.space_uuid AS &endpointCIDRsSpaces.space_uuid
FROM v_application_exposed_endpoint e
LEFT JOIN application_endpoint ae ON e.application_endpoint_uuid = ae.uuid
LEFT JOIN charm_relation cr ON ae.charm_relation_uuid = cr.uuid
WHERE e.application_uuid = $applicationID.uuid;
	`
	stmt, err := st.Prepare(query, endpointCIDRsSpaces{}, ident)
	if err != nil {
		return nil, errors.Errorf("preparing exposed endpoints query: %w", err)
	}

	var (
		endpoints []endpointCIDRsSpaces
	)

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, ident).GetAll(&endpoints); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf("retrieving exposed endpoints for application %q: %w", appID, err)
		}
		return nil
	})

	if err != nil {
		return nil, errors.Capture(err)
	}
	return encodeExposedEndpoints(endpoints), nil
}

func encodeExposedEndpoints(endpoints []endpointCIDRsSpaces) map[string]application.ExposedEndpoint {
	if len(endpoints) == 0 {
		return nil
	}

	exposed := make(map[string]application.ExposedEndpoint)
	for _, endpoint := range endpoints {
		endpointName := network.WildcardEndpoint
		if endpoint.Name.Valid {
			endpointName = endpoint.Name.String
		}

		entry, ok := exposed[endpointName]
		if endpoint.CIDR != "" {
			// ExposeToCIDRs string set is not initialized.
			if !ok || entry.ExposeToCIDRs.Size() == 0 {
				// Since we cannot assign to a struct field in map, we need to
				// copy the full struct with the spaces set as well.
				exposed[endpointName] = application.ExposedEndpoint{
					ExposeToCIDRs:    set.NewStrings(endpoint.CIDR),
					ExposeToSpaceIDs: exposed[endpointName].ExposeToSpaceIDs,
				}
			} else {
				exposed[endpointName].ExposeToCIDRs.Add(endpoint.CIDR)
			}
		}
		if endpoint.SpaceUUID != "" {
			// ExposeToSpaceIDs string set is not initialized.
			if !ok || entry.ExposeToSpaceIDs.Size() == 0 {
				// Since we cannot assign to a struct field in map, we need to
				// copy the full struct with the CIDRs set as well.
				exposed[endpointName] = application.ExposedEndpoint{
					ExposeToCIDRs:    exposed[endpointName].ExposeToCIDRs,
					ExposeToSpaceIDs: set.NewStrings(endpoint.SpaceUUID),
				}
			} else {
				exposed[endpointName].ExposeToSpaceIDs.Add(endpoint.SpaceUUID)
			}
		}
	}

	return exposed
}

// UnsetExposeSettings removes the expose settings for the provided list of
// endpoint names. If the resulting exposed endpoints map for the application
// becomes empty after the settings are removed, the application will be
// automatically unexposed.
// If the provided set of endpoints is empty, all exposed endpoints of the
// application will be removed.
func (st *State) UnsetExposeSettings(ctx context.Context, appID coreapplication.ID, exposedEndpoints set.Strings) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if exposedEndpoints.IsEmpty() {
			return st.unsetAllExposedEndpoints(ctx, tx, appID)
		} else {
			for _, endpoint := range exposedEndpoints.Values() {
				if err := st.unsetExposedEndpoint(ctx, tx, appID, endpoint); err != nil {
					return errors.Capture(err)
				}
			}
		}
		return nil
	})

	return errors.Capture(err)
}

// MergeExposeSettings marks the application as exposed and merges the provided
// ExposedEndpoint details into the current set of expose settings. The merge
// operation will overwrite expose settings for each existing endpoint name.
func (st *State) MergeExposeSettings(ctx context.Context, appID coreapplication.ID, exposedEndpoints map[string]application.ExposedEndpoint) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for endpoint, exposedEndpoint := range exposedEndpoints {
			if err := st.unsetExposedEndpoint(ctx, tx, appID, endpoint); err != nil {
				return errors.Capture(err)
			}
			if err := st.upsertExposedCIDRs(ctx, tx, appID, endpoint, exposedEndpoint.ExposeToCIDRs); err != nil {
				return errors.Capture(err)
			}
			if err := st.upsertExposedSpaces(ctx, tx, appID, endpoint, exposedEndpoint.ExposeToSpaceIDs); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})

	return errors.Capture(err)
}

func (st *State) unsetAllExposedEndpoints(ctx context.Context, tx *sqlair.TX, appID coreapplication.ID) error {
	applicationID := applicationID{ID: appID}

	unsetExposedCIDRQuery := `
DELETE FROM application_exposed_endpoint_cidr
WHERE application_uuid = $applicationID.uuid;
`
	unsetExposedCIDRStmt, err := st.Prepare(unsetExposedCIDRQuery, applicationID)
	if err != nil {
		return errors.Errorf("preparing unset exposed cidr query: %w", err)
	}

	unsetExposedSpaceQuery := `
DELETE FROM application_exposed_endpoint_space
WHERE application_uuid = $applicationID.uuid;
`
	unsetExposedSpaceStmt, err := st.Prepare(unsetExposedSpaceQuery, applicationID)
	if err != nil {
		return errors.Errorf("preparing unset exposed space query: %w", err)
	}

	if err := tx.Query(ctx, unsetExposedCIDRStmt, applicationID).Run(); err != nil {
		return errors.Errorf("unsetting all exposed endpoints to CIDRs of application %q: %w", appID, err)
	}
	if err := tx.Query(ctx, unsetExposedSpaceStmt, applicationID).Run(); err != nil {
		return errors.Errorf("unsetting all exposed endpoints to spaces of application %q: %w", appID, err)
	}

	return nil
}

func (st *State) unsetExposedEndpoint(ctx context.Context, tx *sqlair.TX, appID coreapplication.ID, endpoint string) error {
	if err := st.unsetExposedEndpointCIDRs(ctx, tx, appID, endpoint); err != nil {
		return errors.Capture(err)
	}
	if err := st.unsetExposedEndpointSpaces(ctx, tx, appID, endpoint); err != nil {
		return errors.Capture(err)
	}
	return nil
}

func (st *State) unsetExposedEndpointCIDRs(ctx context.Context, tx *sqlair.TX, appID coreapplication.ID, endpoint string) error {
	applicationID := applicationID{ID: appID}
	endpointName := setEndpointName{Name: endpoint}

	// Since we need to keep referential integrity with respect to the endpoint
	// as stored in charm_relation, we first check if the provided endpoint is
	// the wildcard and in that case we simply remove the CIDRs where the
	// application_endpoint_uuid is NULL.
	var (
		unsetExposedCIDRQuery string
		unsetExposedCIDRStmt  *sqlair.Statement
		err                   error
	)
	if endpoint == network.WildcardEndpoint {
		unsetExposedCIDRQuery = `
DELETE FROM application_exposed_endpoint_cidr
WHERE application_uuid = $applicationID.uuid
AND application_endpoint_uuid IS NULL;
`
		unsetExposedCIDRStmt, err = st.Prepare(unsetExposedCIDRQuery, applicationID)
		if err != nil {
			return errors.Errorf("preparing unset exposed cidr endpoint %q on application %q query: %w", endpoint, appID, err)
		}
		if err := tx.Query(ctx, unsetExposedCIDRStmt, applicationID).Run(); err != nil {
			return errors.Errorf("unsetting exposed cidr endpoint %q on application %q: %w", endpoint, appID, err)
		}
	} else {
		unsetExposedCIDRQuery = `
DELETE FROM application_exposed_endpoint_cidr
WHERE application_uuid = $applicationID.uuid 
AND application_endpoint_uuid IN (
    SELECT uuid
    FROM v_application_endpoint_uuid
    WHERE name = $setEndpointName.name
    AND application_uuid = $applicationID.uuid
);
`
		unsetExposedCIDRStmt, err = st.Prepare(unsetExposedCIDRQuery, applicationID, endpointName)
		if err != nil {
			return errors.Errorf("preparing unset exposed cidr endpoint %q on application %q query: %w", endpoint, appID, err)
		}
		if err := tx.Query(ctx, unsetExposedCIDRStmt, applicationID, endpointName).Run(); err != nil {
			return errors.Errorf("unsetting exposed cidr endpoint %q on application %q: %w", endpoint, appID, err)
		}
	}
	return nil
}

func (st *State) unsetExposedEndpointSpaces(ctx context.Context, tx *sqlair.TX, appID coreapplication.ID, endpoint string) error {
	applicationID := applicationID{ID: appID}
	endpointName := setEndpointName{Name: endpoint}

	// Since we need to keep referential integrity with respect to the endpoint
	// as stored in charm_relation, we first check if the provided endpoint is
	// the wildcard and in that case we simply remove the spaces where the
	// application_endpoint_uuid is NULL.
	var (
		unsetExposedSpaceQuery string
		unsetExposedSpaceStmt  *sqlair.Statement
		err                    error
	)
	if endpoint == network.WildcardEndpoint {
		unsetExposedSpaceQuery = `
DELETE FROM application_exposed_endpoint_space
WHERE application_uuid = $applicationID.uuid
AND application_endpoint_uuid IS NULL;
`
		unsetExposedSpaceStmt, err = st.Prepare(unsetExposedSpaceQuery, applicationID)
		if err != nil {
			return errors.Errorf("preparing unset exposed space endpoint %q on application %q query: %w", endpoint, appID, err)
		}
		if err := tx.Query(ctx, unsetExposedSpaceStmt, applicationID).Run(); err != nil {
			return errors.Errorf("unsetting exposed space endpoint %q on application %q: %w", endpoint, appID, err)
		}
	} else {
		unsetExposedSpaceQuery = `
DELETE FROM application_exposed_endpoint_space
WHERE application_uuid = $applicationID.uuid 
AND application_endpoint_uuid IN (
	SELECT uuid
	FROM v_application_endpoint_uuid
	WHERE name = $setEndpointName.name
	AND application_uuid = $applicationID.uuid
);
`
		unsetExposedSpaceStmt, err := st.Prepare(unsetExposedSpaceQuery, applicationID, endpointName)
		if err != nil {
			return errors.Errorf("preparing unset exposed space endpoint %q on application %q query: %w", endpoint, appID, err)
		}
		if err := tx.Query(ctx, unsetExposedSpaceStmt, applicationID, endpointName).Run(); err != nil {
			return errors.Errorf("unsetting exposed space endpoint %q on application %q: %w", endpoint, appID, err)
		}
	}

	return nil
}

func (st *State) upsertExposedSpaces(ctx context.Context, tx *sqlair.TX, appID coreapplication.ID, endpoint string, exposeToSpaceIDs set.Strings) error {
	if exposeToSpaceIDs.Size() == 0 {
		return nil
	}

	var upsertExposedSpaceQuery string

	if endpoint == network.WildcardEndpoint {
		upsertExposedSpaceQuery = `
INSERT INTO application_exposed_endpoint_space(application_uuid, space_uuid)
VALUES ($setExposedSpace.application_uuid, $setExposedSpace.space_uuid)
`
	} else {
		upsertExposedSpaceQuery = `
INSERT INTO application_exposed_endpoint_space(application_uuid, application_endpoint_uuid, space_uuid)
    SELECT $setExposedSpace.application_uuid, uuid, $setExposedSpace.space_uuid
    FROM v_application_endpoint_uuid
    WHERE name = $setExposedSpace.endpoint
    AND application_uuid = $setExposedSpace.application_uuid;
`
	}

	upsertExposedSpaceStmt, err := st.Prepare(upsertExposedSpaceQuery, setExposedSpace{})
	if err != nil {
		return errors.Errorf("preparing insert exposed endpoints to spaces query: %w", err)
	}

	for _, exposeToSpaceID := range exposeToSpaceIDs.Values() {
		setExposedSpace := setExposedSpace{
			ApplicationUUID: appID.String(),
			EndpointName:    endpoint,
			SpaceUUID:       exposeToSpaceID,
		}
		if err := tx.Query(ctx, upsertExposedSpaceStmt, setExposedSpace).Run(); err != nil {
			return errors.Errorf("inserting exposed endpoints to spaces: %w", err)
		}
	}

	return nil
}

func (st *State) upsertExposedCIDRs(ctx context.Context, tx *sqlair.TX, appID coreapplication.ID, endpoint string, exposeToCIDRs set.Strings) error {
	if exposeToCIDRs.Size() == 0 {
		return nil
	}

	var upsertExposedCIDRQuery string

	// Since we need to keep referential integrity with respect to the endpoint
	// as stored in charm_relation, we first check if the provided endpoint is
	// the wildcard and in that case we simply insert the CIDRs and let the
	// endpoint NULL.
	if endpoint == network.WildcardEndpoint {
		upsertExposedCIDRQuery = `
INSERT INTO application_exposed_endpoint_cidr(application_uuid, cidr)
VALUES ($setExposedCIDR.application_uuid, $setExposedCIDR.cidr)
`
	} else {
		upsertExposedCIDRQuery = `
INSERT INTO application_exposed_endpoint_cidr(application_uuid, application_endpoint_uuid, cidr)
    SELECT $setExposedCIDR.application_uuid, uuid, $setExposedCIDR.cidr
    FROM v_application_endpoint_uuid
    WHERE name = $setExposedCIDR.endpoint
    AND application_uuid = $setExposedCIDR.application_uuid;
`
	}

	setExposedCIDR := setExposedCIDR{
		ApplicationUUID: appID.String(),
		EndpointName:    endpoint,
	}
	upsertExposedCIDRStmt, err := st.Prepare(upsertExposedCIDRQuery, setExposedCIDR)
	if err != nil {
		return errors.Errorf("preparing insert exposed endpoints to CIDRs query: %w", err)
	}

	for _, exposeToCIDR := range exposeToCIDRs.Values() {
		setExposedCIDR.CIDR = exposeToCIDR
		if err := tx.Query(ctx, upsertExposedCIDRStmt, setExposedCIDR).Run(); err != nil {
			return errors.Errorf("inserting exposed endpoints to CIDRs: %w", err)
		}
	}

	return nil
}

// EndpointsExist returns an error satisfying
// [applicationerrors.EndpointNotFound] if any of the provided endpoints do not
// exist.
func (st *State) EndpointsExist(ctx context.Context, appID coreapplication.ID, endpoints set.Strings) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	eps := endpointNames(endpoints.Values())

	query := `
SELECT COUNT(*) AS &countResult.count
FROM application_endpoint
LEFT JOIN charm_relation ON application_endpoint.charm_relation_uuid = charm_relation.uuid
WHERE application_endpoint.application_uuid = $applicationID.uuid 
AND charm_relation.name IN ($endpointNames[:]);
	`
	applicationID := applicationID{ID: appID}
	stmt, err := st.Prepare(query, countResult{}, applicationID, eps)
	if err != nil {
		return errors.Errorf("preparing endpoint exists query: %w", err)
	}

	var count countResult
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, applicationID, eps).Get(&count); err != nil {
			return errors.Errorf("checking if endpoints %+v exist: %w", endpoints.Values(), err)
		}
		return nil
	})

	if err != nil {
		return errors.Capture(err)
	}
	if count.Count != endpoints.Size() {
		return errors.Errorf("one or more of the provided endpoints %+v do not exist", endpoints.Values()).
			Add(applicationerrors.EndpointNotFound)
	}
	return nil
}

// SpacesExist returns an error satisfying [networkerrors.SpaceNotFound] if any
// of the provided spaces do not exist.
func (st *State) SpacesExist(ctx context.Context, spaceUUIDs set.Strings) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	sps := spaces(spaceUUIDs.Values())

	query := `
SELECT COUNT(*) AS &countResult.count
FROM space
WHERE uuid IN ($spaces[:]);
	`
	stmt, err := st.Prepare(query, countResult{}, sps)
	if err != nil {
		return errors.Errorf("preparing space exists query: %w", err)
	}

	var count countResult
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, sps).Get(&count); err != nil {
			return errors.Errorf("checking if spaces %+v exist: %w", spaceUUIDs.Values(), err)
		}
		return nil
	})

	if err != nil {
		return errors.Capture(err)
	}
	if count.Count != spaceUUIDs.Size() {
		return errors.Errorf("one or more of the provided spaces %+v do not exist", spaceUUIDs.Values()).
			Add(networkerrors.SpaceNotFound)
	}
	return nil
}
