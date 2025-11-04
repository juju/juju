// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainlife "github.com/juju/juju/domain/life"
	"github.com/juju/juju/internal/errors"
)

// State represents a type for interacting with the underlying state for
// cross model relations.
type State struct {
	*domain.StateBase
	modelUUID string
	clock     clock.Clock
	logger    logger.Logger
}

// NewState returns a new state reference.
func NewState(factory database.TxnRunnerFactory, modelUUID model.UUID, clock clock.Clock, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		modelUUID: modelUUID.String(),
		clock:     clock,
		logger:    logger,
	}
}

func ptr[T any](v T) *T {
	return &v
}

func (st *State) checkApplicationNotDead(ctx context.Context, tx *sqlair.TX, appUUID string) error {
	stmt, err := st.Prepare(`
SELECT &lifeID.*
FROM   application
WHERE  uuid = $uuid.uuid
`, lifeID{}, uuid{})
	if err != nil {
		return errors.Capture(err)
	}

	var life lifeID
	if err := tx.Query(ctx, stmt, uuid{UUID: appUUID}).Get(&life); errors.Is(err, sqlair.ErrNoRows) {
		return applicationerrors.ApplicationNotFound
	} else if err != nil {
		return errors.Capture(err)
	}

	if life.Life == int(domainlife.Dead) {
		return applicationerrors.ApplicationIsDead
	}
	return nil
}
