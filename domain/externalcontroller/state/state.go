// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/crossmodel"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/internal/uuid"
)

// State implements the domain external controller state.
type State struct {
	*domain.StateBase
}

// NewState returns a new State instance.
func NewState(factory coredatabase.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// Controller returns the external controller with the given UUID.
func (st *State) Controller(
	ctx context.Context,
	controllerUUID string,
) (*crossmodel.ControllerInfo, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	q := `
SELECT (ctrl.uuid,
       alias,
       ca_cert,
       address) as (&Controller.*),
       model.uuid as &Controller.model
FROM   external_controller AS ctrl
       LEFT JOIN external_model AS model
       ON        ctrl.uuid = model.controller_uuid
       LEFT JOIN external_controller_address AS addrs
       ON        ctrl.uuid = addrs.controller_uuid
WHERE  ctrl.uuid = $M.id`
	s, err := sqlair.Prepare(q, Controller{}, sqlair.M{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing %q", q)
	}

	var rows Controllers
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(tx.Query(ctx, s, sqlair.M{"id": controllerUUID}).GetAll(&rows))
	}); err != nil {
		return nil, errors.Annotate(domain.CoerceError(err), "querying external controller")
	}

	if len(rows) == 0 {
		return nil, errors.NotFoundf("external controller %q", controllerUUID)
	}

	return &rows.ToControllerInfo()[0], nil
}

// ControllersForModels returns the external controllers for the given model
// UUIDs. If no model UUIDs are provided, then no controllers are returned.
func (st *State) ControllersForModels(ctx context.Context, modelUUIDs ...string) ([]crossmodel.ControllerInfo, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// TODO(nvinuesa): We should use an `IN` statement query here, instead
	// of looping over the list of models and performing N queries, but
	// they are not yet supported on sqlair:
	// https://github.com/canonical/sqlair/pull/76
	q := `
SELECT (ctrl.uuid,  
       ctrl.alias,
       ctrl.ca_cert,
       addrs.address) as (&Controller.*),
       model.uuid as &Controller.model
FROM   external_controller AS ctrl	
       LEFT JOIN external_model AS model
       ON        ctrl.uuid = model.controller_uuid
       LEFT JOIN external_controller_address AS addrs
       ON        ctrl.uuid = addrs.controller_uuid
WHERE  ctrl.uuid = (
    SELECT controller_uuid
    FROM   external_model AS model
    WHERE  model.uuid = $M.model
)`

	s, err := sqlair.Prepare(q, Controller{}, sqlair.M{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing %q", q)
	}

	var resultControllerInfos Controllers
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for _, modelUUID := range modelUUIDs {
			var rows Controllers
			err := tx.Query(ctx, s, sqlair.M{"model": modelUUID}).GetAll(&rows)
			if err != nil {
				return errors.Trace(domain.CoerceError(err))
			}
			resultControllerInfos = append(resultControllerInfos, rows...)
		}

		return nil
	}); err != nil {
		return nil, errors.Annotate(err, "querying external controller")
	}

	return resultControllerInfos.ToControllerInfo(), nil
}

// UpdateExternalController updates the external controller information.
func (st *State) UpdateExternalController(
	ctx context.Context,
	ci crossmodel.ControllerInfo,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	stmts, err := NewUpdateStatements()
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.updateExternalControllerTx(ctx, tx, stmts, ci)
	})

	return errors.Trace(err)
}

// ImportExternalControllers imports the list of ControllerInfo
// external controllers on one single transaction.
func (st *State) ImportExternalControllers(ctx context.Context, infos []crossmodel.ControllerInfo) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	stmts, err := NewUpdateStatements()
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for _, ci := range infos {
			err := st.updateExternalControllerTx(
				ctx,
				tx,
				stmts,
				ci,
			)
			if err != nil {
				return errors.Trace(err)
			}
		}
		return nil
	})

	return errors.Trace(err)
}

func (st *State) updateExternalControllerTx(
	ctx context.Context,
	tx *sqlair.TX,
	stmts *updateStatements,
	ci crossmodel.ControllerInfo,
) error {
	cID := ci.ControllerTag.Id()
	externalController := Controller{
		ID:     cID,
		Alias:  sql.NullString{String: ci.Alias, Valid: true},
		CACert: ci.CACert,
	}
	if err := tx.Query(ctx, stmts.upsertController, externalController).Run(); err != nil {
		return errors.Trace(domain.CoerceError(err))
	}

	cIDs := uuids(ci.Addrs)
	if len(cIDs) == 0 {
		cIDs = uuids{}
	}
	if err := tx.Query(ctx, stmts.deleteUnusedAddresses, externalController, cIDs).Run(); err != nil {
		return errors.Trace(domain.CoerceError(err))
	}

	for _, addr := range ci.Addrs {
		uuid, err := uuid.NewUUID()
		if err != nil {
			return errors.Trace(err)
		}

		externContAddr := Address{ID: uuid.String(), ControllerUUID: cID, Addr: addr}
		if err := tx.Query(ctx, stmts.insertNewAddresses, externContAddr).Run(); err != nil {
			return errors.Trace(domain.CoerceError(err))
		}
	}

	for _, modelUUID := range ci.ModelUUIDs {
		externalModel := Model{ID: modelUUID, ControllerUUID: cID}
		if err := tx.Query(ctx, stmts.upsertModel, externalModel).Run(); err != nil {
			return errors.Trace(domain.CoerceError(err))
		}
	}

	return nil
}

// ModelsForController returns the model UUIDs associated with the given
// controller UUID.
func (st *State) ModelsForController(
	ctx context.Context,
	controllerUUID string,
) ([]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	q := `
SELECT uuid 
FROM   external_model 
WHERE  controller_uuid = ?`

	var modelUUIDs []string
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, q, controllerUUID)
		if err != nil {
			return errors.Trace(domain.CoerceError(err))
		}
		defer rows.Close()

		for rows.Next() {
			var modelUUID string
			if err := rows.Scan(&modelUUID); err != nil {
				return errors.Trace(err)
			}
			modelUUIDs = append(modelUUIDs, modelUUID)
		}

		return nil
	})

	return modelUUIDs, err
}
