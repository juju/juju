// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"

	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/errors"
)

// GetModelCloudType returns the type of the cloud that is in use by this model.
func (st *State) GetModelCloudType(ctx context.Context) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	type cloud struct {
		Type string `db:"cloud_type"`
	}

	m := cloud{}
	stmt, err := st.Prepare(`SELECT &cloud.cloud_type FROM model`, m)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&m)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("model does not exist").Add(modelerrors.NotFound)
		}
		return err
	})

	if err != nil {
		return "", errors.Capture(err)
	}

	return m.Type, nil
}
