// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"strings"

	"github.com/canonical/sqlair"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/errors"
)

// GetModelRedirection returns redirection information for a model that has
// been migrated away from this controller, read from the redirect snapshot
// written by source-side migration REAP. A staged-but-incomplete redirect
// (completed_at IS NULL) is not active and is treated as not redirected.
// Returns an error satisfying [modelerrors.ModelNotRedirected] when no
// completed redirect exists.
func (s *State) GetModelRedirection(
	ctx context.Context,
	modelUUID coremodel.UUID,
) (model.ModelRedirection, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return model.ModelRedirection{}, errors.Capture(err)
	}

	arg := dbRedirectModelUUID{ModelUUID: modelUUID.String()}
	stmt, err := s.Prepare(`
SELECT &dbModelRedirect.*
FROM   model_migration_redirect
WHERE  model_uuid = $dbRedirectModelUUID.model_uuid
AND    completed_at IS NOT NULL
`, arg, dbModelRedirect{})
	if err != nil {
		return model.ModelRedirection{}, errors.Capture(err)
	}

	var row dbModelRedirect
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		row = dbModelRedirect{}

		err := tx.Query(ctx, stmt, arg).Get(&row)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"no completed redirect for model %q: %w",
				modelUUID, modelerrors.ModelNotRedirected,
			)
		}
		return err
	})
	if err != nil {
		return model.ModelRedirection{}, errors.Capture(err)
	}

	var alias string
	if row.TargetControllerAlias != nil {
		alias = *row.TargetControllerAlias
	}
	// Addresses are host:port strings, so the comma-separated encoding is
	// unambiguous: neither hosts (including bracketed IPv6 literals) nor
	// ports can contain a comma.
	var addresses []string
	if row.TargetAddresses != "" {
		addresses = strings.Split(row.TargetAddresses, ",")
	}
	return model.ModelRedirection{
		Addresses:       addresses,
		CACert:          row.TargetCACert,
		ControllerUUID:  row.TargetControllerUUID,
		ControllerAlias: alias,
	}, nil
}

// GetModelRedirectUsers returns the users captured with access to a migrated
// model in the redirect snapshot.
func (s *State) GetModelRedirectUsers(
	ctx context.Context,
	modelUUID coremodel.UUID,
) ([]model.RedirectUser, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	arg := dbRedirectModelUUID{ModelUUID: modelUUID.String()}
	stmt, err := s.Prepare(`
SELECT &dbRedirectUser.*
FROM   model_migration_redirect_user
WHERE  model_uuid = $dbRedirectModelUUID.model_uuid
`, arg, dbRedirectUser{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var rows []dbRedirectUser
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		rows = nil

		err := tx.Query(ctx, stmt, arg).GetAll(&rows)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	users := make([]model.RedirectUser, len(rows))
	for i, r := range rows {
		users[i] = model.RedirectUser{
			UserName: r.UserName,
			Access:   r.Access,
		}
	}
	return users, nil
}
