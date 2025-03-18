// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"
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

	controller := Controller{ID: controllerUUID}

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
WHERE  ctrl.uuid = $Controller.uuid`
	s, err := st.Prepare(q, controller)
	if err != nil {
		return nil, errors.Annotatef(err, "preparing %q", q)
	}

	var rows Controllers
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(tx.Query(ctx, s, controller).GetAll(&rows))
	}); errors.Is(err, sqlair.ErrNoRows) || len(rows) == 0 {
		return nil, errors.NotFoundf("external controller %q", controllerUUID)
	} else if err != nil {
		return nil, errors.Annotate(err, "querying external controller")
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

	dbModelUUIDs := ModelUUIDs(modelUUIDs)
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
WHERE  ctrl.uuid IN (
    SELECT controller_uuid
    FROM   external_model AS model
    WHERE  model.uuid IN ($ModelUUIDs[:])
)`

	stmt, err := st.Prepare(q, Controller{}, dbModelUUIDs)
	if err != nil {
		return nil, errors.Annotatef(err, "preparing %q", q)
	}

	var resultControllerInfos Controllers
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, dbModelUUIDs).GetAll(&resultControllerInfos)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Trace(err)
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

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.updateExternalControllerTx(ctx, tx, ci)
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

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for _, ci := range infos {
			err := st.updateExternalControllerTx(
				ctx,
				tx,
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

// NamespaceForWatchExternalController returns the namespace identifier
// used by watchers for external controller updates.
func (*State) NamespaceForWatchExternalController() string {
	return "external_controller"
}

func (st *State) updateExternalControllerTx(
	ctx context.Context,
	tx *sqlair.TX,
	ci crossmodel.ControllerInfo,
) error {
	cID := ci.ControllerUUID
	externalController := Controller{
		ID:     cID,
		Alias:  sql.NullString{String: ci.Alias, Valid: true},
		CACert: ci.CACert,
	}

	upsertControllerQuery := `
INSERT INTO external_controller (uuid, alias, ca_cert)
VALUES ($Controller.*)
  ON CONFLICT(uuid) DO UPDATE SET alias=excluded.alias, ca_cert=excluded.ca_cert
`
	upsertControllerStmt, err := st.Prepare(upsertControllerQuery, externalController)
	if err != nil {
		return errors.Annotatef(err, "preparing %q:", upsertControllerQuery)
	}

	if err := tx.Query(ctx, upsertControllerStmt, externalController).Run(); err != nil {
		return errors.Trace(err)
	}

	cIDs := ControllerUUIDs(ci.Addrs)

	deleteUnusedAddressesQuery := `
DELETE FROM external_controller_address
WHERE  controller_uuid = $Controller.uuid
AND    address NOT IN ($ControllerUUIDs[:])
`
	deleteUnusedAddressesStmt, err := st.Prepare(deleteUnusedAddressesQuery, externalController, cIDs)
	if err != nil {
		return errors.Annotatef(err, "preparing %q:", deleteUnusedAddressesQuery)
	}

	if err := tx.Query(ctx, deleteUnusedAddressesStmt, externalController, cIDs).Run(); err != nil {
		return errors.Trace(err)
	}

	if len(ci.Addrs) > 0 {
		var externContAddrs []Address
		for _, addr := range ci.Addrs {
			uuid, err := uuid.NewUUID()
			if err != nil {
				return errors.Trace(err)
			}
			externContAddrs = append(externContAddrs, Address{
				ID:             uuid.String(),
				ControllerUUID: cID,
				Addr:           addr,
			})
		}

		insertNewAddressesQuery := `
INSERT INTO external_controller_address (uuid, controller_uuid, address)
VALUES ($Address.*)
  ON CONFLICT(controller_uuid, address) DO NOTHING
`
		insertNewAddressesStmt, err := st.Prepare(insertNewAddressesQuery, Address{})
		if err != nil {
			return errors.Annotatef(err, "preparing %q:", insertNewAddressesQuery)
		}

		if err := tx.Query(ctx, insertNewAddressesStmt, externContAddrs).Run(); err != nil {
			return errors.Trace(err)
		}
	}

	if len(ci.ModelUUIDs) > 0 {
		var externModels []Model
		for _, modelUUID := range ci.ModelUUIDs {
			externModels = append(externModels, Model{
				ID:             modelUUID,
				ControllerUUID: cID,
			})
		}

		// TODO (manadart 2023-05-13): Check current implementation and see if
		// we need to delete models as we do for addresses, or whether this
		// (additive) approach is what we have now.
		upsertModelQuery := `
INSERT INTO external_model (uuid, controller_uuid)
VALUES ($Model.*)
  ON CONFLICT(uuid) DO UPDATE SET controller_uuid=excluded.controller_uuid
`
		upsertModelStmt, err := st.Prepare(upsertModelQuery, Model{})
		if err != nil {
			return errors.Annotatef(err, "preparing %q:", upsertModelQuery)
		}

		if err := tx.Query(ctx, upsertModelStmt, externModels).Run(); err != nil {
			return errors.Trace(err)
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

	controller := Controller{
		ID: controllerUUID,
	}

	stmt, err := st.Prepare(`
SELECT &Model.uuid 
FROM   external_model 
WHERE  controller_uuid = $Controller.uuid`, controller, Model{})
	if err != nil {
		return nil, errors.Annotate(err, "preparing select models for controller")
	}

	var models []Model
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, controller).GetAll(&models)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Trace(err)
		}

		return nil
	})

	modelUUIDs := transform.Slice[Model, string](
		models,
		func(m Model) string { return m.ID },
	)
	return modelUUIDs, err
}
