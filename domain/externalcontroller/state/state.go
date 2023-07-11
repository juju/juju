// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/database"
	"github.com/juju/juju/domain"
)

type State struct {
	*domain.StateBase
}

func NewState(factory domain.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

func (st *State) Controller(
	ctx context.Context,
	controllerUUID string,
) (*crossmodel.ControllerInfo, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	q := `
SELECT 	(alias, ca_cert, address) as &ExternalController.* 
FROM   	external_controller AS ctrl
       	LEFT JOIN external_controller_address AS addrs
       	ON ctrl.uuid = addrs.controller_uuid
WHERE  	ctrl.uuid = $M.id`
	s, err := sqlair.Prepare(q, ExternalController{}, sqlair.M{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing %q", q)
	}

	var rows []ExternalController
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(tx.Query(ctx, s, sqlair.M{"id": controllerUUID}).GetAll(&rows))
	}); err != nil {
		return nil, errors.Annotate(err, "querying external controller")
	}

	if len(rows) == 0 {
		return nil, errors.NotFoundf("external controller %q", controllerUUID)
	}

	return controllerInfoFromRows(controllerUUID, rows), nil
}

func (st *State) ControllerForModel(
	ctx context.Context,
	modelUUID string,
) (*crossmodel.ControllerInfo, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	q := `
SELECT 	(ctrl.uuid, alias, ca_cert, address) as &ExternalController.* 
FROM   	external_controller AS ctrl	
	JOIN external_model AS model
	ON ctrl.uuid = model.controller_uuid
       	LEFT JOIN external_controller_address AS addrs
       	ON ctrl.uuid = addrs.controller_uuid
WHERE  	model.uuid = $M.id`
	s, err := sqlair.Prepare(q, ExternalController{}, sqlair.M{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing %q", q)
	}

	var rows []ExternalController
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(tx.Query(ctx, s, sqlair.M{"id": modelUUID}).GetAll(&rows))
	}); err != nil {
		return nil, errors.Annotate(err, "querying external controller for model")
	}

	if len(rows) == 0 {
		return nil, errors.NotFoundf("external controller for model %q", modelUUID)
	}

	return controllerInfoFromRows(rows[0].ID, rows), nil
}

func controllerInfoFromRows(uuid string, rows []ExternalController) *crossmodel.ControllerInfo {
	// We know that we queried for a single ID, so the first instance
	// of the controller info fields will be repeated.
	ci := crossmodel.ControllerInfo{
		ControllerTag: names.NewControllerTag(uuid),
		Alias:         rows[0].Alias.String,
		CACert:        rows[0].CACert,
	}

	if rows[0].Addr.Valid {
		ci.Addrs = make([]string, len(rows))
		for i, row := range rows {
			ci.Addrs[i] = row.Addr.String
		}
	}

	return &ci
}

func (st *State) UpdateExternalController(
	ctx context.Context,
	ci crossmodel.ControllerInfo,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
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

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
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

func (st *State) updateExternalControllerTx(
	ctx context.Context,
	tx *sql.Tx,
	ci crossmodel.ControllerInfo,
) error {
	cID := ci.ControllerTag.Id()
	q := `
INSERT INTO external_controller (uuid, alias, ca_cert)
VALUES (?, ?, ?)
  ON CONFLICT(uuid) DO UPDATE SET alias=excluded.alias, ca_cert=excluded.ca_cert`[1:]

	if _, err := tx.ExecContext(ctx, q, cID, ci.Alias, ci.CACert); err != nil {
		return errors.Trace(err)
	}

	addrsBinds, addrsAnyVals := database.SliceToPlaceholder(ci.Addrs)
	q = fmt.Sprintf(`
DELETE FROM external_controller_address
WHERE  controller_uuid = ?
AND    address NOT IN (%s)`[1:], addrsBinds)

	args := append([]any{cID}, addrsAnyVals...)
	if _, err := tx.ExecContext(ctx, q, args...); err != nil {
		return errors.Trace(err)
	}

	for _, addr := range ci.Addrs {
		q := `
INSERT INTO external_controller_address (uuid, controller_uuid, address)
VALUES (?, ?, ?)
  ON CONFLICT(controller_uuid, address) DO NOTHING`[1:]

		if _, err := tx.ExecContext(ctx, q, utils.MustNewUUID().String(), cID, addr); err != nil {
			return errors.Trace(err)
		}
	}

	// TODO (manadart 2023-05-13): Check current implementation and see if
	// we need to delete models as we do for addresses, or whether this
	// (additive) approach is what we have now.
	for _, modelUUID := range ci.ModelUUIDs {
		q := `
INSERT INTO external_model (uuid, controller_uuid)
VALUES (?, ?)
  ON CONFLICT(uuid) DO UPDATE SET controller_uuid=excluded.controller_uuid`[1:]

		if _, err := tx.ExecContext(ctx, q, modelUUID, cID); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

func (st *State) ModelsForController(
	ctx context.Context,
	controllerUUID string,
) ([]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	q := `
SELECT 	uuid 
FROM   	external_model 
WHERE  	controller_uuid = ?`

	var modelUUIDs []string
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, q, controllerUUID)
		if err != nil {
			return errors.Trace(err)
		}

		for rows.Next() {
			var modelUUID string
			if err := rows.Scan(&modelUUID); err != nil {
				_ = rows.Close()
				return errors.Trace(err)
			}
			modelUUIDs = append(modelUUIDs, modelUUID)
		}

		return nil
	})

	return modelUUIDs, err
}

func (st *State) ControllersForModels(ctx context.Context, modelUUIDs []string) ([]crossmodel.ControllerInfo, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelBinds, modelVals := database.SliceToPlaceholder(modelUUIDs)

	// TODO(nvinuesa): We should use SQLair here, but for the moment it's
	// not possible for two reasons:
	// 1) Queries with `IN` statements are not yet supported:
	// https://github.com/canonical/sqlair/pull/76
	// 2) The `AS` statement is not supported for SELECT statement
	// column names (in our case we need the `model.uuid AS model`).
	q := fmt.Sprintf(`
SELECT  ctrl.uuid,  
	ctrl.alias,
	ctrl.ca_cert,
	model.uuid AS model,
	addrs.address
FROM external_controller AS ctrl	
JOIN 	external_model AS model
ON ctrl.uuid = model.controller_uuid
LEFT JOIN external_controller_address AS addrs
ON ctrl.uuid = addrs.controller_uuid
WHERE model.uuid IN (%s)`, modelBinds)

	var resultControllers []crossmodel.ControllerInfo
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, q, modelVals...)
		if err != nil {
			return errors.Trace(err)
		}

		// Prepare structs for unique models and addresses for each
		// controller.
		uniqueModelUUIDs := make(map[string]map[string]string)
		uniqueAddresses := make(map[string]map[string]string)
		uniqueControllers := make(map[string]crossmodel.ControllerInfo)
		for rows.Next() {
			var controller ExternalController
			if err := rows.Scan(&controller.ID, &controller.Alias, &controller.CACert, &controller.ModelUUID, &controller.Addr); err != nil {
				_ = rows.Close()
				return errors.Trace(err)
			}
			uniqueControllers[controller.ID] = crossmodel.ControllerInfo{
				ControllerTag: names.NewControllerTag(controller.ID),
				CACert:        controller.CACert,
				Alias:         controller.Alias.String,
			}

			// Each row contains only one address, so it's safe
			// to access the only possible (nullable) value.
			if controller.Addr.Valid {
				if _, ok := uniqueAddresses[controller.ID]; !ok {
					uniqueAddresses[controller.ID] = make(map[string]string)
				}
				uniqueAddresses[controller.ID][controller.Addr.String] = controller.Addr.String
			}
			// Each row contains only one model, so it's safe
			// to access the only possible (nullable) value.
			if controller.ModelUUID.Valid {
				if _, ok := uniqueModelUUIDs[controller.ID]; !ok {
					uniqueModelUUIDs[controller.ID] = make(map[string]string)
				}
				uniqueModelUUIDs[controller.ID][controller.ModelUUID.String] = controller.ModelUUID.String
			}
		}

		// Iterate through every controller and flatten its models and
		// addresses.
		for controllerID, controller := range uniqueControllers {
			var modelUUIDs []string
			for _, modelUUID := range uniqueModelUUIDs[controllerID] {
				modelUUIDs = append(modelUUIDs, modelUUID)
			}
			controller.ModelUUIDs = modelUUIDs

			var addresses []string
			for _, modelUUID := range uniqueAddresses[controllerID] {
				addresses = append(addresses, modelUUID)
			}
			controller.Addrs = addresses

			resultControllers = append(resultControllers, controller)
		}

		return nil
	})

	return resultControllers, err
}
