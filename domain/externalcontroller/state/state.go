// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/juju/collections/transform"
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

func NewState(factory domain.DBFactory) *State {
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

	var (
		controllerInfo        crossmodel.ControllerInfo
		controllerRow         *sql.Row
		controllerAddressRows *sql.Rows
	)
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		controllerQuery := `
SELECT alias, ca_cert FROM external_controller WHERE uuid=?`[1:]
		controllerAdressQuery := `
SELECT address FROM external_controller_address WHERE controller_uuid=? ORDER BY uuid`[1:]

		controllerRow = tx.QueryRowContext(ctx, controllerQuery, controllerUUID)
		controllerAddressRows, err = tx.QueryContext(ctx, controllerAdressQuery, controllerUUID)

		controllerInfo, err = controllerInfoFromRows(controllerUUID, controllerRow, controllerAddressRows)
		return err
	})

	return &controllerInfo, err
}

func controllerInfoFromRows(
	controllerUUID string,
	controllerRow *sql.Row,
	controllerAddressRows *sql.Rows,
) (crossmodel.ControllerInfo, error) {
	defer controllerAddressRows.Close()

	var (
		controllerInfo      crossmodel.ControllerInfo
		controllerAddresses []string
		// Only Alias can be NULL in the external_controller table as
		// defined in schema.
		controllerAlias sql.NullString
	)

	controllerInfo.ControllerTag = names.NewControllerTag(controllerUUID)

	for controllerAddressRows.Next() {
		var controllerAddress string
		if err := controllerAddressRows.Scan(&controllerAddress); err != nil {
			return crossmodel.ControllerInfo{}, errors.Trace(err)
		}
		controllerAddresses = append(controllerAddresses, controllerAddress)
	}
	controllerInfo.Addrs = controllerAddresses

	if err := controllerRow.Scan(&controllerAlias, &controllerInfo.CACert); err != nil {
		if database.IsErrNotFound(err) {
			return crossmodel.ControllerInfo{}, errors.NotFoundf("external controller with UUID %v", controllerUUID)
		}
		return crossmodel.ControllerInfo{}, errors.Trace(err)
	}
	if controllerAlias.Valid {
		controllerInfo.Alias = controllerAlias.String
	}

	return controllerInfo, nil
}

func (st *State) UpdateExternalController(
	ctx context.Context,
	ci crossmodel.ControllerInfo,
	modelUUIDs []string,
) error {
	ec := ExternalController{
		ID:     ci.ControllerTag.Id(),
		Alias:  ci.Alias,
		Addrs:  ci.Addrs,
		CACert: ci.CACert,
		Models: modelUUIDs,
	}

	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := `
INSERT INTO external_controller (uuid, alias, ca_cert)
VALUES (?, ?, ?)
  ON CONFLICT(uuid) DO UPDATE SET alias=?, ca_cert=?`[1:]

		if _, err := tx.ExecContext(ctx, q, ec.ID, ec.Alias, ec.CACert, ec.Alias, ec.CACert); err != nil {
			return errors.Trace(err)
		}

		q = fmt.Sprintf(`
DELETE FROM external_controller_address
WHERE  controller_uuid = ?
AND    address NOT IN (%s)`[1:], database.SliceToPlaceholder(ec.Addrs))

		args := append([]any{ec.ID}, transform.Slice(ec.Addrs, func(s string) any { return s })...)
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return errors.Trace(err)
		}

		for _, addr := range ec.Addrs {
			q := `
INSERT INTO external_controller_address (uuid, controller_uuid, address)
VALUES (?, ?, ?)
  ON CONFLICT(controller_uuid, address) DO NOTHING`[1:]

			if _, err := tx.ExecContext(ctx, q, utils.MustNewUUID().String(), ec.ID, addr); err != nil {
				return errors.Trace(err)
			}
		}

		// TODO (manadart 2023-05-13): Check current implementation and see if
		// we need to delete models as we do for addresses, or whether this
		// (additive) approach is what we have now.

		for _, modelUUID := range ec.Models {
			q := `
INSERT INTO external_model (uuid, controller_uuid)
VALUES (?, ?)
  ON CONFLICT(uuid) DO UPDATE SET controller_uuid=?`[1:]

			if _, err := tx.ExecContext(ctx, q, modelUUID, ec.ID, ec.ID); err != nil {
				return errors.Trace(err)
			}
		}

		return nil
	})

	return errors.Trace(err)
}
