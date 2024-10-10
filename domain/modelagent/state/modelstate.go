// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/errors"
)

type ModelState struct {
	*domain.StateBase
}

// NewModelState returns a new [ModelState] object.
func NewModelState(factory database.TxnRunnerFactory) *ModelState {
	return &ModelState{
		StateBase: domain.NewStateBase(factory),
	}
}

// CheckApplicationExists check to see if the given machine exists in the
// model. If the machine does not exist an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (m *ModelState) CheckApplicationExists(
	ctx context.Context,
	name string,
) error {
	db, err := m.DB()
	if err != nil {
		return errors.Errorf(
			"getting database to check application %q exists: %w",
			name, err,
		)
	}

	applicationName := applicationName{name}
	stmt, err := m.Prepare(`
SELECT &applicationName.*
FROM application
WHERE name = $applicationName.name
`, applicationName)

	if err != nil {
		return errors.Errorf(
			"preparing application %q selection statement: %w", name, err,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, applicationName).Get(&applicationName)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"application %q does not exist", name,
			).Add(applicationerrors.ApplicationNotFound)
		} else if err != nil {
			return errors.Errorf(
				"checking if application %q exists: %w", name, err,
			)
		}

		return nil
	})

	return err
}

// CheckMachineExists check to see if the given machine exists in the model. If
// the machine does not exist an error satisfying
// [machineerrors.MachineNotFound] is returned.
func (m *ModelState) CheckMachineExists(
	ctx context.Context,
	name machine.Name,
) error {
	db, err := m.DB()
	if err != nil {
		return errors.Errorf(
			"getting database to check machine %q exists: %w",
			name, err,
		)
	}

	machineNameVal := machineName{name.String()}
	stmt, err := m.Prepare(`
SELECT &machineName.*
FROM machine
WHERE name = $machineName.name
`, machineNameVal)

	if err != nil {
		return errors.Errorf(
			"preparing machine %q selection statement: %w", name, err,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, machineNameVal).Get(&machineNameVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"machine %q does not exist", name,
			).Add(machineerrors.MachineNotFound)
		} else if err != nil {
			return errors.Errorf(
				"checking if machine %q exists: %w", name, err,
			)
		}

		return nil
	})

	return err
}

// GetModelUUID returns the uuid for the model represented by this state.
func (m *ModelState) GetModelUUID(ctx context.Context) (model.UUID, error) {
	db, err := m.DB()
	if err != nil {
		return model.UUID(""), errors.Errorf(
			"getting database to get the model uuid: %w", err,
		)
	}

	modelUUIDVal := modelUUIDValue{}

	stmt, err := m.Prepare(`
SELECT (uuid) AS (&modelUUIDValue.model_uuid)
FROM model
`, modelUUIDVal)
	if err != nil {
		return model.UUID(""), errors.Errorf(
			"preparing model uuid selection statement: %w", err,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&modelUUIDVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.New(
				"getting model uuid from database, read only model records don't exist",
			)
		} else if err != nil {
			return errors.Errorf(
				"getting model uuid from database: %w", err,
			)
		}
		return nil
	})

	if err != nil {
		return model.UUID(""), err
	}

	return model.UUID(modelUUIDVal.UUID), nil
}

// CheckUnitExists checks to see if the given unit exists in the model. If
// the unit does not exist an error satisfying
// [applicationerrors.UnitNotFound] is returned.
func (m *ModelState) CheckUnitExists(
	ctx context.Context,
	name string,
) error {
	db, err := m.DB()
	if err != nil {
		return errors.Errorf(
			"getting database to check unit %q exists: %w",
			name, err,
		)
	}

	unitNameVal := unitName{name}
	stmt, err := m.Prepare(`
SELECT &unitName.*
FROM unit
WHERE name = $unitName.name
`, unitNameVal)

	if err != nil {
		return errors.Errorf(
			"preparing unit %q selection statement: %w", name, err,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, unitNameVal).Get(&unitNameVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"unit %q does not exist", name,
			).Add(machineerrors.MachineNotFound)
		} else if err != nil {
			return errors.Errorf(
				"checking if unit %q exists: %w", name, err,
			)
		}

		return nil
	})

	return err
}
