// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	uniterrors "github.com/juju/juju/domain/unitstate/errors"
)

type State struct {
	*domain.StateBase
}

// NewState returns a new state reference.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// GetUnitUUIDForName returns the UUID corresponding to the input unit name.
// If no unit with the name exists, a [errors.UnitNotFound] error is returned.
func (st *State) GetUnitUUIDForName(ctx domain.AtomicContext, name string) (string, error) {
	uName := unitName{Name: name}
	uuid := unitUUID{}

	q := "SELECT &unitUUID.uuid FROM unit WHERE name = $unitName.name"
	stmt, err := st.Prepare(q, uName, uuid)
	if err != nil {
		return "", fmt.Errorf("failed to prepare query: %w", err)
	}

	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, uName).Get(&uuid)
		if errors.Is(err, sqlair.ErrNoRows) {
			return uniterrors.UnitNotFound
		}
		return err
	})

	return uuid.UUID, err
}
