// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/statushistory"
	statushistoryerrors "github.com/juju/juju/domain/statushistory/errors"
)

// ModelState represents a type for interacting with the underlying model
// database state.
type ModelState struct {
	*domain.StateBase
}

// NewState returns a new State for interacting with the underlying model
// database state.
func NewState(
	factory database.TxnRunnerFactory,
) *ModelState {
	return &ModelState{
		StateBase: domain.NewStateBase(factory),
	}
}

// Model returns the read-only model for status history.
func (s *ModelState) Model(ctx context.Context) (statushistory.ReadOnlyModel, error) {
	db, err := s.DB()
	if err != nil {
		return statushistory.ReadOnlyModel{}, errors.Trace(err)
	}

	stmt, err := s.Prepare(`SELECT &readOnlyModel.* FROM model;`, readOnlyModel{})
	if err != nil {
		return statushistory.ReadOnlyModel{}, errors.Trace(err)
	}

	var model readOnlyModel
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt).Get(&model)
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return statushistory.ReadOnlyModel{}, fmt.Errorf("model %w", statushistoryerrors.NotFound)
		}
		return statushistory.ReadOnlyModel{}, errors.Trace(err)
	}
	return statushistory.ReadOnlyModel{
		UUID:  model.UUID,
		Name:  model.Name,
		Owner: model.Owner,
	}, nil
}
