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
	modelUUIDs []string,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	cID := ci.ControllerTag.Id()

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
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

		for _, modelUUID := range modelUUIDs {
			q := `
INSERT INTO external_model (uuid, controller_uuid)
VALUES (?, ?)
  ON CONFLICT(uuid) DO UPDATE SET controller_uuid=excluded.controller_uuid`[1:]

			if _, err := tx.ExecContext(ctx, q, modelUUID, cID); err != nil {
				return errors.Trace(err)
			}
		}

		return nil
	})

	return errors.Trace(err)
}
