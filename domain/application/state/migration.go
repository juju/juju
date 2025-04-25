// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/model"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
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

		for i := range apps {
			bindings, err := st.getEndpointBindings(ctx, tx, apps[i].UUID)
			if err != nil {
				return errors.Errorf("getting endpoing bindings")
			}
			apps[i].EndpointBindings = bindings
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
			EndpointBindings:     app.EndpointBindings,
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
			UUID:      unit.UUID,
			Name:      unit.Name,
			Machine:   unit.Machine,
			Principal: unit.Principal,
		}
	}
	return exportUnits, nil
}

// InsertIAASUnits imports the fully formed units for the specified IAAS
// application. This is only used when importing units during model migration.
func (st *State) InsertMigratingIAASUnits(ctx context.Context, appUUID coreapplication.ID, units ...application.ImportUnitArg) error {
	if len(units) == 0 {
		return nil
	}
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for _, arg := range units {
			if err := st.importIAASUnit(ctx, tx, appUUID, arg); err != nil {
				return errors.Errorf("importing IAAS unit %q: %w", arg.UnitName, err)
			}
		}
		return nil
	})
}

// InsertCAASUnits imports the fully formed units for the specified CAAS
// application. This is only used when importing units during model migration.
func (st *State) InsertMigratingCAASUnits(ctx context.Context, appUUID coreapplication.ID, units ...application.ImportUnitArg) error {
	if len(units) == 0 {
		return nil
	}
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for _, arg := range units {
			if err := st.importCAASUnit(ctx, tx, appUUID, arg); err != nil {
				return errors.Errorf("importing CAAS unit %q: %w", arg.UnitName, err)
			}
		}
		return nil
	})
}

func (st *State) importCAASUnit(
	ctx context.Context,
	tx *sqlair.TX,
	appUUID coreapplication.ID,
	args application.ImportUnitArg,
) error {
	_, err := st.getUnitDetails(ctx, tx, args.UnitName)
	if err == nil {
		return errors.Errorf("unit %q already exists", args.UnitName).Add(applicationerrors.UnitAlreadyExists)
	} else if !errors.Is(err, applicationerrors.UnitNotFound) {
		return errors.Errorf("looking up unit %q: %w", args.UnitName, err)
	}

	unitUUID, err := coreunit.NewUUID()
	if err != nil {
		return errors.Capture(err)
	}

	netNodeUUID, err := st.insertNetNode(ctx, tx)
	if err != nil {
		return errors.Capture(err)
	}

	if err := st.insertUnit(ctx, tx, appUUID, unitUUID, netNodeUUID, insertUnitArg{
		UnitName:       args.UnitName,
		CloudContainer: args.CloudContainer,
		Password:       args.Password,
		Constraints:    args.Constraints,
		UnitStatusArg:  args.UnitStatusArg,
	}); err != nil {
		return errors.Errorf("importing unit for CAAS application %q: %w", appUUID, err)
	}

	if args.Principal != "" {
		if err = st.recordUnitPrincipal(ctx, tx, args.Principal, args.UnitName); err != nil {
			return errors.Errorf("importing subordinate info for unit %q: %w", args.UnitName, err)
		}
	}

	// If there is no storage, return early.
	if len(args.Storage) == 0 {
		return nil
	}

	attachArgs, err := st.insertUnitStorage(ctx, tx, appUUID, unitUUID, args.Storage, args.StoragePoolKind)
	if err != nil {
		return errors.Errorf("importing storage for unit %q: %w", args.UnitName, err)
	}
	err = st.attachUnitStorage(ctx, tx, args.StorageParentDir, args.StoragePoolKind, unitUUID, netNodeUUID, attachArgs)
	if err != nil {
		return errors.Errorf("importing storage for unit %q: %w", args.UnitName, err)
	}
	return nil
}

func (st *State) importIAASUnit(
	ctx context.Context,
	tx *sqlair.TX,
	appUUID coreapplication.ID,
	args application.ImportUnitArg,
) error {
	_, err := st.getUnitDetails(ctx, tx, args.UnitName)
	if err == nil {
		return errors.Errorf("unit %q already exists", args.UnitName).Add(applicationerrors.UnitAlreadyExists)
	} else if !errors.Is(err, applicationerrors.UnitNotFound) {
		return errors.Errorf("looking up unit %q: %w", args.UnitName, err)
	}

	unitUUID, err := coreunit.NewUUID()
	if err != nil {
		return errors.Capture(err)
	}

	netNodeUUID, err := st.getMachineNetNodeUUIDFromName(ctx, tx, args.Machine)
	if err != nil {
		return errors.Capture(err)
	}

	if err := st.insertUnit(ctx, tx, appUUID, unitUUID, netNodeUUID, insertUnitArg{
		UnitName:       args.UnitName,
		CloudContainer: args.CloudContainer,
		Password:       args.Password,
		Constraints:    args.Constraints,
		UnitStatusArg:  args.UnitStatusArg,
	}); err != nil {
		return errors.Errorf("importing unit for application %q: %w", appUUID, err)
	}

	if args.Principal != "" {
		if err = st.recordUnitPrincipal(ctx, tx, args.Principal, args.UnitName); err != nil {
			return errors.Errorf("importing subordinate info for unit %q: %w", args.UnitName, err)
		}
	}

	if _, err := st.insertUnitStorage(ctx, tx, appUUID, unitUUID, args.Storage, args.StoragePoolKind); err != nil {
		return errors.Errorf("importing storage for unit %q: %w", args.UnitName, err)
	}
	return nil
}
