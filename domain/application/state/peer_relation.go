// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"time"

	"github.com/canonical/sqlair"

	coreapplication "github.com/juju/juju/core/application"
	corerelation "github.com/juju/juju/core/relation"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/relation"
	sequencestate "github.com/juju/juju/domain/sequence/state"
	"github.com/juju/juju/internal/errors"
)

// getPeerEndpoints retrieves a list of peer endpoint for the given
// application UUID from the database.
func (st *State) getPeerEndpoints(ctx context.Context, tx *sqlair.TX, uuid coreapplication.ID) ([]peerEndpoint, error) {
	type application dbUUID
	app := application{UUID: uuid.String()}

	stmt, err := st.Prepare(`
SELECT 
    ae.uuid AS &peerEndpoint.uuid,
    cr.name AS &peerEndpoint.name
FROM   application_endpoint ae
JOIN   charm_relation cr ON cr.uuid = ae.charm_relation_uuid
JOIN   charm_relation_role crr ON crr.id = cr.role_id
WHERE  ae.application_uuid = $application.uuid
AND    crr.name = 'peer'
ORDER BY cr.name -- ensure that peer endpoints relation id are always generated in alphabetical order
`, app,
		peerEndpoint{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var endpoints []peerEndpoint
	if err := tx.Query(ctx, stmt, app).GetAll(&endpoints); errors.Is(err, sqlair.ErrNoRows) {
		return nil, nil
	}

	return endpoints, err
}

// insertPeerRelations inserts peer relations for the specified application UUID
// within a transactional context.
// It retrieves peer endpoints, creates new relations for them,
// and inserts their statuses and endpoints. Returns an error if any step fails.
func (st *State) insertPeerRelations(ctx context.Context, tx *sqlair.TX, appUUID coreapplication.ID) error {
	peerEndpoints, err := st.getPeerEndpoints(ctx, tx, appUUID)
	if err != nil {
		return errors.Errorf("getting peer endpoints: %w", err)
	}

	for _, peer := range peerEndpoints {
		relID, err := sequencestate.NextValue(ctx, st, tx, relation.SequenceNamespace)
		if err != nil {
			return errors.Errorf("getting next relation id: %w", err)
		}

		// Insert a new relation with a new relation ID and UUID.
		if err := st.insertPeerRelation(ctx, tx, peer, relID); err != nil {
			return errors.Errorf("inserting peer relation for peer %q: %w", peer.Name, err)
		}
	}
	return nil
}

// insertMigratingPeerRelations inserts peer relations for the specified application UUID
// within a transactional context, using the relation ID provided during migration.
// It retrieves peer endpoints, creates new relations for them,
// and inserts their statuses and endpoints. Returns an error if any step fails.
func (st *State) insertMigratingPeerRelations(ctx context.Context, tx *sqlair.TX, appUUID coreapplication.ID, relations map[string]int) error {
	peerEndpoints, err := st.getPeerEndpoints(ctx, tx, appUUID)
	if err != nil {
		return errors.Errorf("getting peer endpoints: %w", err)
	}

	for _, peer := range peerEndpoints {
		// Find the relation ID of this endpoint.
		id, ok := relations[peer.Name]
		if !ok {
			return errors.Errorf("relation id not found for peer relation: %q", peer.Name)
		}

		// Insert a new relation with a migrated relation ID and new relation UUID.
		if err := st.insertPeerRelation(ctx, tx, peer, uint64(id)); err != nil {
			return errors.Errorf("inserting peer relation for peer %q: %w", peer.Name, err)
		}
	}
	return nil
}

// insertPeerRelation inserts a single peer relation.
func (st *State) insertPeerRelation(ctx context.Context, tx *sqlair.TX, peer peerEndpoint, relID uint64) error {
	relUUID, err := st.insertNewRelation(ctx, tx, relID)
	if err != nil {
		return errors.Errorf("inserting new relation for peer endpoint %q: %w", peer.Name, err)
	}

	// Insert relation status.
	if err := st.insertNewRelationStatus(ctx, tx, relUUID); err != nil {
		return errors.Errorf("inserting new relation status for peer endpoint %q: %w", peer.Name, err)
	}

	// Insert the relation endpoint
	if err := st.insertNewRelationEndpoint(ctx, tx, relUUID, peer.UUID); err != nil {
		return errors.Errorf("inserting new relation endpoint for %q: %w", peer.Name, err)
	}

	return nil
}

// insertNewRelation creates a new relation entry in the database and returns its UUID or an error if the operation fails.
func (st *State) insertNewRelation(ctx context.Context, tx *sqlair.TX, relID uint64) (corerelation.UUID, error) {
	relUUID, err := corerelation.NewUUID()
	if err != nil {
		return relUUID, errors.Errorf("generating new relation UUID: %w", err)
	}

	type relationIDAndUUID struct {
		UUID corerelation.UUID `db:"uuid"`
		ID   uint64            `db:"relation_id"`
	}

	stmtInsert, err := st.Prepare(`
INSERT INTO relation (uuid, life_id, relation_id)
VALUES ($relationIDAndUUID.uuid, 0, $relationIDAndUUID.relation_id)
`, relationIDAndUUID{})
	if err != nil {
		return relUUID, errors.Capture(err)
	}

	relUUIDArg := relationIDAndUUID{
		UUID: relUUID,
		ID:   relID,
	}

	if err := tx.Query(ctx, stmtInsert, relUUIDArg).Run(); err != nil {
		return relUUID, errors.Capture(err)
	}

	return relUUID, nil
}

// insertNewRelationEndpoint inserts a new relation endpoint into the database
// using the provided context and transaction.
func (st *State) insertNewRelationEndpoint(ctx context.Context, tx *sqlair.TX, relUUID corerelation.UUID,
	endpointUUID corerelation.EndpointUUID) error {
	uuid, err := corerelation.NewEndpointUUID()
	if err != nil {
		return errors.Errorf("generating new relation endpoint UUID: %w", err)
	}

	type setRelationEndpoint struct {
		UUID         corerelation.EndpointUUID `db:"uuid"`
		RelationUUID corerelation.UUID         `db:"relation_uuid"`
		EndpointUUID corerelation.EndpointUUID `db:"endpoint_uuid"`
	}

	endpoint := setRelationEndpoint{
		UUID:         uuid,
		RelationUUID: relUUID,
		EndpointUUID: endpointUUID,
	}
	stmt, err := st.Prepare(`
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES ($setRelationEndpoint.*)`, endpoint)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt, endpoint).Run(); err != nil {
		return errors.Capture(err)
	}

	return nil
}

// insertNewRelationStatus inserts a new relation status into the
// relation_status table in the database.
// It uses the provided context, transaction, and relation UUID to create a
// record with a status of 'joining'.
func (st *State) insertNewRelationStatus(ctx context.Context, tx *sqlair.TX, uuid corerelation.UUID) error {

	type setRelationStatus struct {
		RelationUUID corerelation.UUID `db:"relation_uuid"`
		Status       corestatus.Status `db:"status"`
		UpdatedAt    time.Time         `db:"updated_at"`
	}

	status := setRelationStatus{
		RelationUUID: uuid,
		Status:       corestatus.Joining,
		UpdatedAt:    st.clock.Now(),
	}

	stmt, err := st.Prepare(`
INSERT INTO relation_status (relation_uuid, relation_status_type_id, updated_at)
SELECT $setRelationStatus.relation_uuid, status.id, $setRelationStatus.updated_at
FROM   relation_status_type status
WHERE  status.name = $setRelationStatus.status`, status)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt, status).Run(); err != nil {
		return errors.Capture(err)
	}
	return nil
}
