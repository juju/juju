// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/internal/errors"
)

// GetApplicationsForExport returns all the applications in the model.
// If the model does not exist, an error satisfying
// [modelerrors.NotFound] is returned.
// If the application does not exist, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (st *State) GetApplicationsForExport(ctx context.Context) ([]application.ExportApplication, error) {
	db, err := st.DB()
	if err != nil {
		return nil, err
	}

	var app exportApplication
	query := `SELECT &exportApplication.* FROM v_application_export`
	stmt, err := st.Prepare(query, app)
	if err != nil {
		return nil, errors.Errorf("preparing statement: %w", err)
	}

	var (
		modelType model.ModelType
		apps      []exportApplication
	)
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		modelType, err = st.getModelType(ctx, tx)
		if err != nil {
			return err
		}

		err := tx.Query(ctx, stmt).GetAll(&apps)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("getting applications for export: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	exportApps := make([]application.ExportApplication, len(apps))
	for i, app := range apps {
		locator, err := decodeCharmLocator(charmLocator{
			ReferenceName:  app.CharmReferenceName,
			Revision:       app.CharmRevision,
			SourceID:       app.CharmSourceID,
			ArchitectureID: app.CharmArchitectureID,
		})
		if err != nil {
			return nil, err
		}

		var providerID *string
		if app.K8sServiceProviderID.Valid {
			providerID = ptr(app.K8sServiceProviderID.String)
		}

		exportApps[i] = application.ExportApplication{
			UUID:                 app.UUID,
			Name:                 app.Name,
			ModelType:            modelType,
			CharmUUID:            app.CharmUUID,
			Life:                 app.Life,
			Placement:            app.Placement,
			Exposed:              app.Exposed,
			Subordinate:          app.Subordinate,
			CharmModifiedVersion: app.CharmModifiedVersion,
			CharmUpgradeOnError:  app.CharmUpgradeOnError,
			CharmLocator: charm.CharmLocator{
				Name:         locator.Name,
				Revision:     locator.Revision,
				Source:       locator.Source,
				Architecture: locator.Architecture,
			},
			K8sServiceProviderID: providerID,
		}
	}
	return exportApps, nil
}

// GetApplicationUnitsForExport returns all the units for a given
// application in the model.
// If the application does not exist, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (st *State) GetApplicationUnitsForExport(ctx context.Context, appID coreapplication.ID) ([]application.ExportUnit, error) {
	db, err := st.DB()
	if err != nil {
		return nil, err
	}

	var unit exportUnit
	query := `SELECT &exportUnit.* FROM v_unit_export`
	stmt, err := st.Prepare(query, unit)
	if err != nil {
		return nil, errors.Errorf("preparing statement: %w", err)
	}

	var units []exportUnit
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&units)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("getting units for application export %q: %w", appID, err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	exportUnits := make([]application.ExportUnit, len(units))
	for i, unit := range units {
		exportUnits[i] = application.ExportUnit{
			UUID:         unit.UUID,
			Name:         unit.Name,
			PasswordHash: unit.PasswordHash,
		}
	}
	return exportUnits, nil
}
