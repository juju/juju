// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
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
	id := applicationID{
		ID: appID,
	}
	query := `
SELECT &exportUnit.* FROM v_unit_export
WHERE application_uuid = $applicationID.uuid
`
	stmt, err := st.Prepare(query, unit, id)
	if err != nil {
		return nil, errors.Errorf("preparing statement: %w", err)
	}

	var units []exportUnit
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, id).GetAll(&units)
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

func (st *State) InsertMigratingApplication(ctx context.Context, name string, args application.InsertApplicationArgs) (coreapplication.ID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	appUUID, err := coreapplication.NewID()
	if err != nil {
		return "", errors.Capture(err)
	}

	charmID, err := corecharm.NewID()
	if err != nil {
		return "", errors.Capture(err)
	}

	appDetails := applicationDetails{
		UUID:      appUUID,
		Name:      name,
		CharmUUID: charmID,
		LifeID:    life.Alive,

		// The space is defaulted to Alpha, which is guaranteed to exist.
		// However, if there is default space defined in endpoints bindings
		// (through a binding with an empty endpoint), the application space
		// will be updated later in the transaction, during the insertion
		// of application_endpoints.
		// The space defined here will be used as default space when creating
		// relation where application_endpoint doesn't have a defined space.
		SpaceUUID: network.AlphaSpaceId,
	}

	createApplication := `INSERT INTO application (*) VALUES ($applicationDetails.*)`
	createApplicationStmt, err := st.Prepare(createApplication, appDetails)
	if err != nil {
		return "", errors.Capture(err)
	}

	scaleInfo := applicationScale{
		ApplicationID: appUUID,
		Scale:         args.Scale,
	}
	createScale := `INSERT INTO application_scale (*) VALUES ($applicationScale.*)`
	createScaleStmt, err := st.Prepare(createScale, scaleInfo)
	if err != nil {
		return "", errors.Capture(err)
	}

	platformInfo := applicationPlatform{
		ApplicationID:  appUUID,
		OSTypeID:       int(args.Platform.OSType),
		Channel:        args.Platform.Channel,
		ArchitectureID: int(args.Platform.Architecture),
	}
	createPlatform := `INSERT INTO application_platform (*) VALUES ($applicationPlatform.*)`
	createPlatformStmt, err := st.Prepare(createPlatform, platformInfo)
	if err != nil {
		return "", errors.Capture(err)
	}

	var (
		referenceName = args.Charm.ReferenceName
		revision      = args.Charm.Revision
		charmName     = args.Charm.Metadata.Name
	)

	var (
		createChannelStmt *sqlair.Statement
		channelInfo       applicationChannel
	)
	if ch := args.Channel; ch != nil {
		channelInfo = applicationChannel{
			ApplicationID: appUUID,
			Track:         ch.Track,
			Risk:          string(ch.Risk),
			Branch:        ch.Branch,
		}
		createChannel := `INSERT INTO application_channel (*) VALUES ($applicationChannel.*)`
		if createChannelStmt, err = st.Prepare(createChannel, channelInfo); err != nil {
			return "", errors.Capture(err)
		}
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check if the application already exists.
		if err := st.checkApplicationNameAvailable(ctx, tx, name); err != nil {
			return errors.Errorf("checking if application %q exists: %w", name, err)
		}

		shouldInsertCharm := true

		// Check if the charm already exists.
		existingCharmID, err := st.checkCharmReferenceExists(ctx, tx, referenceName, revision)
		if err != nil && !errors.Is(err, applicationerrors.CharmAlreadyExists) {
			return errors.Errorf("checking if charm %q exists: %w", charmName, err)
		} else if errors.Is(err, applicationerrors.CharmAlreadyExists) {
			// We already have an existing charm, in this case we just want
			// to point the application to the existing charm.
			appDetails.CharmUUID = existingCharmID

			shouldInsertCharm = false
		}

		if shouldInsertCharm {
			// When importing a charm, we don't have all the information
			// about the charm. We could call charmhub store directly, but
			// that has the potential to block a migration if the charmhub
			// store is down. If we require that information, then it's
			// possible to fill this missing information in the charmhub
			// store using the charmhub identifier.
			// If the controllers do not have the same charmhub url, then
			// all bets are off.
			downloadInfo := &charm.DownloadInfo{Provenance: charm.ProvenanceMigration}
			if err := st.setCharm(ctx, tx, charmID, args.Charm, downloadInfo); err != nil {
				return errors.Errorf("setting charm: %w", err)
			}
		}

		// If the application doesn't exist, create it.
		if err := tx.Query(ctx, createApplicationStmt, appDetails).Run(); err != nil {
			return errors.Errorf("inserting row for application %q: %w", name, err)
		}
		if err := tx.Query(ctx, createPlatformStmt, platformInfo).Run(); err != nil {
			return errors.Errorf("inserting platform row for application %q: %w", name, err)
		}
		if err := tx.Query(ctx, createScaleStmt, scaleInfo).Run(); err != nil {
			return errors.Errorf("inserting scale row for application %q: %w", name, err)
		}
		if err := st.createApplicationResources(
			ctx, tx,
			insertResourcesArgs{
				appID:        appDetails.UUID,
				charmUUID:    appDetails.CharmUUID,
				charmSource:  args.Charm.Source,
				appResources: args.Resources,
			},
			nil,
		); err != nil {
			return errors.Errorf("inserting or resolving resources for application %q: %w", name, err)
		}
		if err := st.insertApplicationConfig(ctx, tx, appDetails.UUID, args.Config); err != nil {
			return errors.Errorf("inserting config for application %q: %w", name, err)
		}
		if err := st.insertApplicationSettings(ctx, tx, appDetails.UUID, args.Settings); err != nil {
			return errors.Errorf("inserting settings for application %q: %w", name, err)
		}
		if err := st.updateConfigHash(ctx, tx, applicationID{ID: appUUID}); err != nil {
			return errors.Errorf("refreshing config hash for application %q: %w", name, err)
		}
		if err := st.insertApplicationEndpoints(ctx, tx, insertApplicationEndpointsParams{
			appID:     appDetails.UUID,
			charmUUID: appDetails.CharmUUID,
			bindings:  args.EndpointBindings,
		}); err != nil {
			return errors.Errorf("inserting exposed endpoints for application %q: %w", name, err)
		}
		if err := st.insertMigratingPeerRelations(ctx, tx, appDetails.UUID, args.PeerRelations); err != nil {
			return errors.Errorf("inserting peer relation for application %q: %w", name, err)
		}

		// The channel is optional for local charms. Although, it would be
		// nice to have a channel for local charms, it's not a requirement.
		if createChannelStmt != nil {
			if err := tx.Query(ctx, createChannelStmt, channelInfo).Run(); err != nil {
				return errors.Errorf("inserting channel row for application %q: %w", name, err)
			}
		}
		return nil
	})
	if err != nil {
		return "", errors.Errorf("creating application %q: %w", name, err)
	}
	return appUUID, nil
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
