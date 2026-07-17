// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	coreDB "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/internal/errors"
)

// State represents a type for interacting with the underlying state.
type State struct {
	*domain.StateBase
	logger logger.Logger
}

// NewState returns a new State for interacting with the underlying state.
func NewState(factory coreDB.TxnRunnerFactory, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// AddSpace creates and returns a new space, associating any subnets matching
// the provided CIDRs with it.
//
// All preconditions (the space name is not already in use, and each requested
// CIDR matches at least one subnet) are verified by SELECTs before any writes
// happen, so the rejection path stays as a read transaction. When the
// preconditions hold, the space insert, optional provider_space insert and
// subnet space update all run in the same transaction, keeping the operation
// atomic.
//
// If a space with the same name already exists, an error matching
// [networkerrors.SpaceAlreadyExists] is returned. If any of the given CIDRs
// has no matching subnet, an error matching [networkerrors.SubnetNotFound] is
// returned. In both cases no rows are written.
func (st *State) AddSpace(
	ctx context.Context,
	uuid network.SpaceUUID,
	name network.SpaceName,
	providerID network.Id,
	cidrList []string,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	sp := space{UUID: uuid, Name: name}
	selectSpaceByNameStmt, err := st.Prepare(`
SELECT &space.uuid
FROM   space
WHERE  name = $space.name`, sp)
	if err != nil {
		return errors.Capture(err)
	}
	insertSpaceStmt, err := st.Prepare(`
INSERT INTO space (uuid, name)
VALUES ($space.*)`, sp)
	if err != nil {
		return errors.Capture(err)
	}

	providerSp := providerSpace{ProviderID: providerID, SpaceUUID: uuid}
	insertProviderStmt, err := st.Prepare(`
INSERT INTO provider_space (provider_id, space_uuid)
VALUES ($providerSpace.*)`, providerSp)
	if err != nil {
		return errors.Capture(err)
	}

	type cidrs []string
	var (
		selectCIDRsStmt *sqlair.Statement
		updateCIDRsStmt *sqlair.Statement
		cidrInput       cidrs
	)
	if len(cidrList) > 0 {
		cidrInput = cidrList
		selectCIDRsStmt, err = st.Prepare(`
SELECT DISTINCT &subnet.cidr
FROM   subnet
WHERE  cidr IN ($cidrs[:])`, subnet{}, cidrInput)
		if err != nil {
			return errors.Capture(err)
		}
		updateCIDRsStmt, err = st.Prepare(`
UPDATE subnet
SET    space_uuid = $space.uuid
WHERE  cidr IN ($cidrs[:])`, sp, cidrInput)
		if err != nil {
			return errors.Capture(err)
		}
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var existing space
		err := tx.Query(ctx, selectSpaceByNameStmt, sp).Get(&existing)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("checking space name %q: %w", name, err)
		}
		if err == nil {
			return errors.Errorf("space %q: %w", name, networkerrors.SpaceAlreadyExists)
		}

		if len(cidrList) > 0 {
			// Verify each requested CIDR matches at least one
			// existing subnet. A single CIDR may correspond to
			// multiple subnet rows (e.g. distinct provider networks
			// presenting the same range); we only care that the
			// CIDR is present.
			var found []subnet
			err := tx.Query(ctx, selectCIDRsStmt, cidrInput).GetAll(&found)
			if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("looking up subnets for space %q: %w", uuid, err)
			}
			foundSet := make(map[string]struct{}, len(found))
			for _, s := range found {
				foundSet[s.CIDR] = struct{}{}
			}
			for _, cidr := range cidrList {
				if _, ok := foundSet[cidr]; !ok {
					return errors.Errorf("subnet %q: %w", cidr, networkerrors.SubnetNotFound)
				}
			}
		}

		// Preconditions satisfied: perform the writes.
		if err := tx.Query(ctx, insertSpaceStmt, sp).Run(); err != nil {
			return errors.Errorf("inserting space uuid %q into space table: %w", uuid, err)
		}
		if providerID != "" {
			if err := tx.Query(ctx, insertProviderStmt, providerSp).Run(); err != nil {
				return errors.Errorf("inserting provider id %q into provider_space table: %w", providerID, err)
			}
		}
		if len(cidrList) == 0 {
			return nil
		}
		// Assign all subnets matching any of the requested CIDRs to
		// the new space.
		if err := tx.Query(ctx, updateCIDRsStmt, sp, cidrInput).Run(); err != nil {
			return errors.Errorf("updating subnets for space %q: %w", uuid, err)
		}
		return nil
	})
	return errors.Capture(err)
}

// GetSpace returns the space by UUID. If the space is not found, an error is
// returned matching [networkerrors.SpaceNotFound].
func (st *State) GetSpace(
	ctx context.Context,
	uuid network.SpaceUUID,
) (*network.SpaceInfo, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	sp := space{UUID: uuid}
	spacesStmt, err := st.Prepare(`
SELECT &spaceSubnetRow.*
FROM   v_space_subnet
WHERE  uuid = $space.uuid;`, spaceSubnetRow{}, sp)
	if err != nil {
		return nil, errors.Errorf("preparing select space statement: %w", err)
	}

	var spaceRows SpaceSubnetRows
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, spacesStmt, sp).GetAll(&spaceRows)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("space not found with %s: %w", uuid, networkerrors.SpaceNotFound)
			}
			return errors.Errorf("retrieving space %q: %w", uuid, err)
		}
		return err
	}); err != nil {
		return nil, errors.Errorf("querying spaces: %w", err)
	}

	return &spaceRows.ToSpaceInfos()[0], nil
}

// GetSpaceByName returns the space by name. If the space is not found, an
// error is returned matching [networkerrors.SpaceNotFound].
func (st *State) GetSpaceByName(
	ctx context.Context,
	name network.SpaceName,
) (*network.SpaceInfo, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	sp := space{
		Name: name,
	}

	// Append the space.name condition to the query.
	q := `
SELECT &spaceSubnetRow.*
FROM   v_space_subnet
WHERE  name = $space.name;`

	s, err := st.Prepare(q, spaceSubnetRow{}, sp)
	if err != nil {
		return nil, errors.Errorf("preparing %q: %w", q, err)
	}

	var rows SpaceSubnetRows
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := errors.Capture(tx.Query(ctx, s, sp).GetAll(&rows))
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("space not found with %s: %w", name, networkerrors.SpaceNotFound)
			}
			return errors.Errorf("querying spaces by name: %w", err)
		}
		return nil
	}); err != nil {
		return nil, errors.Capture(err)
	}

	return &rows.ToSpaceInfos()[0], nil
}

// GetAllSpaces returns all spaces for the model.
func (st *State) GetAllSpaces(
	ctx context.Context,
) (network.SpaceInfos, error) {

	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	s, err := st.Prepare(`
SELECT &spaceSubnetRow.*
FROM   v_space_subnet
WHERE  uuid >= ''
`, spaceSubnetRow{})
	if err != nil {
		return nil, errors.Errorf("preparing select all spaces statement: %w", err)
	}

	var rows SpaceSubnetRows
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, s).GetAll(&rows); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return nil
			}
			return errors.Errorf("querying all spaces: %w", err)
		}
		return nil
	}); err != nil {
		return nil, errors.Capture(err)
	}

	return rows.ToSpaceInfos(), nil
}

// UpdateSpace updates the space identified by the passed uuid. If the space is
// not found, an error is returned matching [networkerrors.SpaceNotFound].
func (st *State) UpdateSpace(
	ctx context.Context,
	uuid network.SpaceUUID,
	name network.SpaceName,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	sp := space{
		UUID: uuid,
		Name: name,
	}
	stmt, err := st.Prepare(`
UPDATE space
SET    name = $space.name
WHERE  uuid = $space.uuid;`, sp)
	if err != nil {
		return errors.Errorf("preparing update space statement: %w", err)
	}
	var outcome sqlair.Outcome
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, sp).Get(&outcome)
		if err != nil {
			return errors.Errorf("updating space %q with name %q: %w", uuid, name, err)
		}
		affected, err := outcome.Result().RowsAffected()
		if err != nil {
			return errors.Capture(err)
		}
		if affected == 0 {
			return errors.Errorf("space not found with %s: %w", uuid, networkerrors.SpaceNotFound)
		}
		return nil
	})
	return err
}
