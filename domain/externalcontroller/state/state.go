// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/database"
	"github.com/juju/juju/domain"
	"github.com/juju/utils/v3"
)

type State struct {
	*domain.StateBase
}

func NewState(factory domain.DBFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

func (st *State) UpdateExternalController(ctx context.Context, ci crossmodel.ControllerInfo, modelUUIDs []string) error {
	ec := ExternalController{
		ID:     ci.ControllerTag.Id(),
		Alias:  ci.Alias,
		Addrs:  ci.Addrs,
		CACert: ci.CACert,
		Models: nil,
	}

	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := `
INSERT INTO external_controller (uuid, alias)
VALUES (?, ?)
  ON CONFLICT(uuid) DO UPDATE SET alias=?`[1:]

		if _, err := tx.ExecContext(ctx, q, ec.ID, ec.Alias, ec.Alias); err != nil {
			return errors.Trace(err)
		}

		q = fmt.Sprintf(`
DELETE FROM external_controller_address
WHERE  controller_uuid = ?
AND    address NOT IN (%s)`[1:], database.SliceToPlaceholder(ec.Addrs))

		if _, err := tx.ExecContext(ctx, q, ec.Addrs); err != nil {
			return errors.Trace(err)
		}

		// TODO (manadart 2024-04-28): Update CA Cert.

		for _, addr := range ec.Addrs {
			q := `
INSERT INTO external_controller_address (uuid, controller_uuid, address)
VALUES (?, ?, ?)
  ON CONFLICT(controller_uuid, address) DO NOTHING`[1:]

			if _, err := tx.ExecContext(ctx, q, utils.MustNewUUID().String(), ec.ID, addr); err != nil {
				return errors.Trace(err)
			}
		}

		for _, modelUUID := range modelUUIDs {
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
