// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/internal/errors"
)

// ExportApplications returns all the applications in the model.
func (st *State) GetApplicationsForExport(ctx context.Context) ([]application.ExportApplication, error) {
	db, err := st.DB()
	if err != nil {
		return nil, err
	}

	var app exportApplication
	query := `SELECT &exportApplication.* FROM v_application_export`
	stmt, err := st.Prepare(query, app)
	if err != nil {
		return nil, err
	}

	var apps []exportApplication
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&apps)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("failed to get applications for export: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	exportApps := make([]application.ExportApplication, len(apps))
	for i, app := range apps {
		exportApps[i] = application.ExportApplication{
			UUID:         app.UUID,
			Name:         app.Name,
			CharmUUID:    app.CharmUUID,
			Life:         app.Life,
			PasswordHash: app.PasswordHash,
			Exposed:      app.Exposed,
			Subordinate:  app.Subordinate,
		}
	}
	return exportApps, nil
}
