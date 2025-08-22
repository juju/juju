// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/ipaddress"
	"github.com/juju/juju/domain/life"
	modelerrors "github.com/juju/juju/domain/model/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/status"
	internalcharm "github.com/juju/juju/internal/charm"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// checkApplicationExists checks that the provided application uuid exists. True
// is returned when the application is found.
func (st *State) checkApplicationExists(
	ctx context.Context,
	tx *sqlair.TX,
	appUUID coreapplication.ID,
) (bool, error) {
	uuidInput := entityUUID{UUID: appUUID.String()}

	checkStmt, err := st.Prepare(`
SELECT &entityUUID.*
FROM   application
WHERE  uuid = $entityUUID.uuid
	`,
		uuidInput,
	)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, checkStmt, uuidInput).Get(&uuidInput)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}
	return true, nil
}

// Deprecated: This method will be removed, as there should be no need to
// determine the model type from the state or service. That's an artifact of
// the caller to call the correct methods.
func (st *State) getModelType(ctx context.Context, tx *sqlair.TX) (coremodel.ModelType, error) {
	var result modelInfo
	stmt, err := st.Prepare("SELECT &modelInfo.type FROM model", result)
	if err != nil {
		return "", errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt).Get(&result); errors.Is(err, sql.ErrNoRows) {
		return "", modelerrors.NotFound
	} else if err != nil {
		return "", errors.Errorf("querying model type: %w", err)
	}

	return coremodel.ModelType(result.ModelType), nil
}

// CreateIAASApplication creates an IAAS application, returning an error
// satisfying [applicationerrors.ApplicationAlreadyExists] if the application
// already exists. It returns as error satisfying
// [applicationerrors.CharmNotFound] if the charm for the application is not
// found.
func (st *State) CreateIAASApplication(
	ctx context.Context,
	name string,
	args application.AddIAASApplicationArg,
	units []application.AddIAASUnitArg,
) (coreapplication.ID, []coremachine.Name, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", nil, errors.Capture(err)
	}

	appUUID, err := coreapplication.NewID()
	if err != nil {
		return "", nil, errors.Capture(err)
	}

	var machineNames []coremachine.Name
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.insertApplication(ctx, tx, name, appUUID, args.BaseAddApplicationArg); err != nil {
			return errors.Errorf("inserting IAAS application %q: %w", name, err)
		}

		if len(units) == 0 {
			return nil
		}
		if machineNames, err = st.insertIAASApplicationUnits(ctx, tx, appUUID, args, units); err != nil {
			return errors.Errorf("inserting IAAS units for application %q: %w", appUUID, err)
		}
		return nil
	})
	if err != nil {
		return "", nil, errors.Errorf("creating IAAS application %q: %w", name, err)
	}
	return appUUID, machineNames, nil
}

// CreateCAASApplication creates an CAAS application, returning an error
// satisfying [applicationerrors.ApplicationAlreadyExists] if the application
// already exists. It returns as error satisfying
// [applicationerrors.CharmNotFound] if the charm for the application is not
// found.
func (st *State) CreateCAASApplication(
	ctx context.Context,
	name string,
	args application.AddCAASApplicationArg,
	units []application.AddCAASUnitArg,
) (coreapplication.ID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	appUUID, err := coreapplication.NewID()
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

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.insertApplication(ctx, tx, name, appUUID, args.BaseAddApplicationArg); err != nil {
			return errors.Errorf("inserting CAAS application %q: %w", name, err)
		}

		if err := tx.Query(ctx, createScaleStmt, scaleInfo).Run(); err != nil {
			return errors.Errorf("inserting scale row for application %q: %w", name, err)
		}

		if len(units) == 0 {
			return nil
		}
		if err = st.insertCAASApplicationUnits(ctx, tx, appUUID, args, units); err != nil {
			return errors.Errorf("inserting CAAS units for application %q: %w", appUUID, err)
		}
		return nil
	})
	if err != nil {
		return "", errors.Errorf("creating CAAS application %q: %w", name, err)
	}
	return appUUID, nil
}

func (st *State) insertApplication(
	ctx context.Context,
	tx *sqlair.TX,
	name string,
	appUUID coreapplication.ID,
	args application.BaseAddApplicationArg,
) error {
	charmID, err := corecharm.NewID()
	if err != nil {
		return errors.Capture(err)
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
		return errors.Capture(err)
	}

	if args.Platform.Architecture == architecture.Unknown {
		return errors.Errorf("cannot insert application with an empty architecture")
	}
	archID, err := encodeArchitecture(args.Platform.Architecture)
	if err != nil {
		return errors.Errorf("encoding architecture: %w", err)
	}
	platformInfo := applicationPlatform{
		ApplicationID:  appUUID,
		OSTypeID:       int(args.Platform.OSType),
		Channel:        args.Platform.Channel,
		ArchitectureID: archID,
	}
	createPlatform := `INSERT INTO application_platform (*) VALUES ($applicationPlatform.*)`
	createPlatformStmt, err := st.Prepare(createPlatform, platformInfo)
	if err != nil {
		return errors.Capture(err)
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
			return errors.Capture(err)
		}
	}

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
		if err := st.setCharm(ctx, tx, charmID, args.Charm, args.CharmDownloadInfo); err != nil {
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
	if err := st.createApplicationResources(
		ctx, tx,
		insertResourcesArgs{
			appID:        appDetails.UUID,
			charmUUID:    appDetails.CharmUUID,
			charmSource:  args.Charm.Source,
			appResources: args.Resources,
		},
		args.PendingResources,
	); err != nil {
		return errors.Errorf("inserting or resolving resources for application %q: %w", name, err)
	}
	if err := st.insertApplicationController(ctx, tx, appDetails, args.IsController); err != nil {
		return errors.Errorf("inserting controller for application %q: %w", name, err)
	}
	if err := st.insertApplicationStorageDirectives(
		ctx, tx, appDetails.UUID, appDetails.CharmUUID, args.StorageDirectives,
	); err != nil {
		return errors.Errorf("inserting storage directives for application %q: %w", name, err)
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
	if err := st.insertApplicationStatus(ctx, tx, appDetails.UUID, args.Status); err != nil {
		return errors.Errorf("inserting status for application %q: %w", name, err)
	}
	if err := st.updateDefaultSpace(ctx, tx, appDetails.UUID.String(), args.EndpointBindings); err != nil {
		return errors.Errorf("updating default space: %w", err)
	}
	if err := st.insertApplicationEndpointBindings(ctx, tx, insertApplicationEndpointsParams{
		appID:    appDetails.UUID,
		bindings: args.EndpointBindings,
	}); err != nil {
		return errors.Errorf("inserting exposed endpoints for application %q: %w", name, err)
	}
	if err := st.insertPeerRelations(ctx, tx, appDetails.UUID); err != nil {
		return errors.Errorf("inserting peer relation for application %q: %w", name, err)
	}
	if err := st.insertDeviceConstraints(ctx, tx, appUUID, args.Devices); err != nil {
		return errors.Errorf("inserting device constraints for application %q: %w", appUUID, err)
	}

	// The channel is optional for local charms. Although, it would be
	// nice to have a channel for local charms, it's not a requirement.
	if createChannelStmt != nil {
		if err := tx.Query(ctx, createChannelStmt, channelInfo).Run(); err != nil {
			return errors.Errorf("inserting channel row for application %q: %w", name, err)
		}
	}
	return nil
}

func (st *State) insertApplicationController(
	ctx context.Context, tx *sqlair.TX,
	appDetails applicationDetails,
	isController bool,
) error {
	if !isController {
		return nil
	}

	stmt, err := st.Prepare(`
INSERT INTO application_controller (application_uuid)
VALUES ($applicationDetails.uuid)
`, appDetails)
	if err != nil {
		return errors.Capture(err)
	}

	return tx.Query(ctx, stmt, appDetails).Run()
}

func (st *State) insertIAASApplicationUnits(
	ctx context.Context, tx *sqlair.TX,
	appUUID coreapplication.ID,
	args application.AddIAASApplicationArg,
	units []application.AddIAASUnitArg,
) ([]coremachine.Name, error) {
	var machineNames []coremachine.Name
	for i, unit := range units {
		_, mNames, err := st.insertIAASUnit(ctx, tx, appUUID, unit)
		if err != nil {
			return nil, errors.Errorf("inserting IAAS unit %d: %w", i, err)
		}
		machineNames = append(machineNames, mNames...)
	}

	return machineNames, nil
}

func (st *State) insertCAASApplicationUnits(
	ctx context.Context, tx *sqlair.TX,
	appUUID coreapplication.ID,
	args application.AddCAASApplicationArg,
	units []application.AddCAASUnitArg,
) error {
	for i, unit := range units {
		if _, err := st.insertCAASUnit(ctx, tx, appUUID, unit); err != nil {
			return errors.Errorf(
				"inserting CAAS unit %d for application: %w", i, err,
			)
		}
	}

	return nil
}

// GetUnitLife looks up the life of the specified unit, returning an error
// satisfying [applicationerrors.UnitNotFound] if the unit is not found.
func (st *State) GetUnitLife(ctx context.Context, unitName coreunit.Name) (life.Life, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return -1, errors.Capture(err)
	}

	var life life.Life
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		life, err = st.getLifeForUnitName(ctx, tx, unitName)
		return errors.Capture(err)
	})
	if err != nil {
		return 0, errors.Errorf("querying unit %q life: %w", unitName, err)
	}
	return life, nil
}

func (st *State) getLifeForUnitName(ctx context.Context, tx *sqlair.TX, unitName coreunit.Name) (life.Life, error) {
	unit := unitNameLife{Name: unitName.String()}
	queryUnit := `
SELECT &unitNameLife.life_id
FROM unit
WHERE name = $unitNameLife.name
`
	queryUnitStmt, err := st.Prepare(queryUnit, unit)
	if err != nil {
		return -1, errors.Capture(err)
	}

	err = tx.Query(ctx, queryUnitStmt, unit).Get(&unit)
	if err != nil {
		if !errors.Is(err, sqlair.ErrNoRows) {
			return -1, errors.Errorf("querying unit %q life: %w", unitName, err)
		}
		return -1, errors.Errorf("%w: %s", applicationerrors.UnitNotFound, unitName)
	}
	return life.Life(unit.LifeID), nil
}

// GetApplicationScaleState looks up the scale state of the specified application, returning an error
// satisfying [applicationerrors.ApplicationNotFound] if the application is not found.
func (st *State) GetApplicationScaleState(ctx context.Context, appUUID coreapplication.ID) (application.ScaleState, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return application.ScaleState{}, errors.Capture(err)
	}

	var appScale application.ScaleState
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		appScale, err = st.getApplicationScaleState(ctx, tx, appUUID)
		return err
	})
	if err != nil {
		return application.ScaleState{}, errors.Errorf("querying application %q scale: %w", appUUID, err)
	}
	return appScale, nil
}

func (st *State) getApplicationScaleState(ctx context.Context, tx *sqlair.TX, appUUID coreapplication.ID) (application.ScaleState, error) {
	appScale := applicationScale{ApplicationID: appUUID}
	queryScale := `
SELECT &applicationScale.*
FROM application_scale
WHERE application_uuid = $applicationScale.application_uuid
`
	queryScaleStmt, err := st.Prepare(queryScale, appScale)
	if err != nil {
		return application.ScaleState{}, errors.Capture(err)
	}

	err = tx.Query(ctx, queryScaleStmt, appScale).Get(&appScale)
	if errors.Is(err, sql.ErrNoRows) {
		return application.ScaleState{}, errors.Errorf("%w: %s", applicationerrors.ApplicationNotFound, appUUID)
	} else if err != nil {
		return application.ScaleState{}, errors.Errorf("querying application %q scale: %w", appUUID, err)
	}
	return application.ScaleState{
		Scaling:     appScale.Scaling,
		Scale:       appScale.Scale,
		ScaleTarget: appScale.ScaleTarget,
	}, nil
}

// GetApplicationLife looks up the life of the specified application, returning
// an error satisfying [applicationerrors.ApplicationNotFoundError] if the
// application is not found.
func (st *State) GetApplicationLife(ctx context.Context, appUUID coreapplication.ID) (life.Life, error) {
	ident := applicationID{ID: appUUID}
	db, err := st.DB(ctx)
	if err != nil {
		return -1, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT &lifeID.* FROM application
WHERE uuid = $applicationID.uuid
`, lifeID{}, ident)
	if err != nil {
		return -1, errors.Capture(err)
	}

	var life lifeID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ident).Get(&life)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("application %s not found", appUUID).Add(applicationerrors.ApplicationNotFound)
		}
		return errors.Capture(err)
	})
	if err != nil {
		return -1, errors.Capture(err)
	}
	return life.LifeID, nil
}

// IsControllerApplication returns true when the application is the controller.
func (st *State) IsControllerApplication(ctx context.Context, appID coreapplication.ID) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	ident := applicationID{ID: appID}
	appExistsQuery := `
SELECT &applicationID.*
FROM application
WHERE uuid = $applicationID.uuid;
`
	appExistsStmt, err := st.Prepare(appExistsQuery, ident)
	if err != nil {
		return false, errors.Errorf("preparing query for application %q: %w", ident.ID, err)
	}

	controllerApp := controllerApplication{
		ApplicationID: appID,
	}
	stmt, err := st.Prepare(`
SELECT TRUE AS &controllerApplication.is_controller
FROM application_controller
WHERE application_uuid = $controllerApplication.application_uuid
`, controllerApp)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, appExistsStmt, ident).Get(&ident)
		if errors.Is(err, sql.ErrNoRows) {
			return applicationerrors.ApplicationNotFound
		} else if err != nil {
			return errors.Errorf("checking application %q exists: %w", ident.ID, err)
		}
		err = tx.Query(ctx, stmt, controllerApp).Get(&controllerApp)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	})
	return controllerApp.IsController, errors.Capture(err)
}

// GetApplicationLifeByName looks up the life of the specified application, returning
// an error satisfying [applicationerrors.ApplicationNotFoundError] if the
// application is not found.
func (st *State) GetApplicationLifeByName(ctx context.Context, appName string) (coreapplication.ID, life.Life, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", -1, errors.Capture(err)
	}

	var app applicationDetails
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		app, err = st.getApplicationDetails(ctx, tx, appName)
		if err != nil {
			return errors.Errorf("querying life for application %q: %w", appName, err)
		}
		return nil
	})
	return app.UUID, app.LifeID, errors.Capture(err)
}

// CheckAllApplicationsAndUnitsAreAlive checks that all applications and units
// in the model are alive, returning an error if any are not.
// The following errors may be returned:
// - [applicationerrors.ApplicationNotAlive] if any applications are not alive.
// - [applicationerrors.UnitNotAlive] if any units are not alive.
func (st *State) CheckAllApplicationsAndUnitsAreAlive(ctx context.Context) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	checkApplicationsStmt, err := st.Prepare(`
SELECT &applicationName.*
FROM application
WHERE life_id != 0
`, applicationName{})
	if err != nil {
		return errors.Capture(err)
	}

	checkUnitsStmt, err := st.Prepare(`
SELECT &unitName.*
FROM unit
WHERE life_id != 0
`, unitName{})
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var deadApps []applicationName
		err := tx.Query(ctx, checkApplicationsStmt).GetAll(&deadApps)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		} else if err == nil {
			names := transform.Slice(deadApps, func(app applicationName) string { return app.Name })
			return errors.Errorf("application(s) %q are not alive", strings.Join(names, ", ")).Add(applicationerrors.ApplicationNotAlive)
		}

		var deadUnits []unitName
		err = tx.Query(ctx, checkUnitsStmt).GetAll(&deadUnits)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		} else if err == nil {
			names := transform.Slice(deadUnits, func(unit unitName) string { return unit.Name.String() })
			return errors.Errorf("unit(s) %q are not alive", strings.Join(names, ", ")).Add(applicationerrors.UnitNotAlive)
		}
		return nil
	})
	if err != nil {
		return errors.Errorf("checking apps and units are alive: %w", err)
	}
	return nil
}

func (st *State) getApplicationDetails(ctx context.Context, tx *sqlair.TX, appName string) (applicationDetails, error) {
	app := applicationDetails{Name: appName}
	query := `
SELECT &applicationDetails.*
FROM application a
WHERE name = $applicationDetails.name
`
	stmt, err := st.Prepare(query, app)
	if err != nil {
		return applicationDetails{}, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, app).Get(&app)
	if err != nil {
		if !errors.Is(err, sqlair.ErrNoRows) {
			return applicationDetails{}, errors.Errorf("querying application details for application %q: %w", appName, err)
		}
		return applicationDetails{}, errors.Errorf("%w: %s", applicationerrors.ApplicationNotFound, appName)
	}
	return app, nil
}

// SetDesiredApplicationScale updates the desired scale of the specified
// application.
func (st *State) SetDesiredApplicationScale(ctx context.Context, appUUID coreapplication.ID, scale int) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	scaleDetails := applicationScale{
		ApplicationID: appUUID,
		Scale:         scale,
	}
	upsertApplicationScale := `
UPDATE application_scale SET scale = $applicationScale.scale
WHERE application_uuid = $applicationScale.application_uuid
`

	upsertStmt, err := st.Prepare(upsertApplicationScale, scaleDetails)
	if err != nil {
		return errors.Capture(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, upsertStmt, scaleDetails).Run()
	})
	return errors.Capture(err)
}

// UpdateApplicationScale updates the desired scale of an application by a
// delta.
// If the resulting scale is less than zero, an error satisfying
// [applicationerrors.ScaleChangeInvalid] is returned.
func (st *State) UpdateApplicationScale(ctx context.Context, appUUID coreapplication.ID, delta int) (int, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return -1, errors.Capture(err)
	}

	upsertApplicationScale := `
UPDATE application_scale SET scale = $applicationScale.scale
WHERE application_uuid = $applicationScale.application_uuid
`
	upsertStmt, err := st.Prepare(upsertApplicationScale, applicationScale{})
	if err != nil {
		return -1, errors.Capture(err)
	}
	var newScale int
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		currentScaleState, err := st.getApplicationScaleState(ctx, tx, appUUID)
		if err != nil {
			return errors.Capture(err)
		}

		newScale = currentScaleState.Scale + delta
		if newScale < 0 {
			return errors.Errorf(
				"%w: cannot remove more units than currently exist", applicationerrors.ScaleChangeInvalid)
		}

		scaleDetails := applicationScale{
			ApplicationID: appUUID,
			Scale:         newScale,
		}
		return tx.Query(ctx, upsertStmt, scaleDetails).Run()
	})
	return newScale, errors.Capture(err)
}

// SetApplicationScalingState sets the scaling details for the given caas
// application Scale is optional and is only set if not nil.
func (st *State) SetApplicationScalingState(ctx context.Context, appName string, targetScale int, scaling bool) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	scaleDetails := applicationScale{
		Scaling:     scaling,
		ScaleTarget: targetScale,
	}

	upsertApplicationScale := `
UPDATE application_scale SET
    scale = $applicationScale.scale,
    scaling = $applicationScale.scaling,
    scale_target = $applicationScale.scale_target
WHERE application_uuid = $applicationScale.application_uuid
`

	upsertStmt, err := st.Prepare(upsertApplicationScale, scaleDetails)
	if err != nil {
		return errors.Capture(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		appDetails, err := st.getApplicationDetails(ctx, tx, appName)
		if err != nil {
			return errors.Capture(err)
		}
		scaleDetails.ApplicationID = appDetails.UUID

		currentScaleState, err := st.getApplicationScaleState(ctx, tx, appDetails.UUID)
		if err != nil {
			return errors.Capture(err)
		}

		if scaling {
			switch appDetails.LifeID {
			case life.Alive:
				// if starting a scale, ensure we are scaling to the same target.
				if !currentScaleState.Scaling && currentScaleState.Scale != targetScale {
					return applicationerrors.ScalingStateInconsistent
				}
				// Make sure to leave the scale value unchanged.
				scaleDetails.Scale = currentScaleState.Scale
			case life.Dying, life.Dead:
				// force scale to the scale target when dying/dead.
				scaleDetails.Scale = targetScale
			}
		} else {
			// Make sure to leave the scale value unchanged.
			scaleDetails.Scale = currentScaleState.Scale
		}

		return tx.Query(ctx, upsertStmt, scaleDetails).Run()
	})
	return errors.Capture(err)
}

// UpsertCloudService updates the cloud service for the specified application.
// The following errors may be returned:
// - [applicationerrors.ApplicationNotFound] if the application doesn't exist
func (st *State) UpsertCloudService(ctx context.Context, applicationName, providerID string, sAddrs network.ProviderAddresses) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	serviceInfo := cloudService{
		ProviderID: providerID,
	}

	// Query any existing records for application and provider id.
	queryExistingStmt, err := st.Prepare(`
SELECT &cloudService.* FROM k8s_service
WHERE  application_uuid = $cloudService.application_uuid
AND    provider_id = $cloudService.provider_id`, serviceInfo)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		appUUID, err := st.lookupApplication(ctx, tx, applicationName)
		if err != nil {
			return errors.Capture(err)
		}
		serviceInfo.ApplicationUUID = appUUID

		// First see if the cloud service for the app and provider id already exists.
		// If so, it's a no-op.
		err = tx.Query(ctx, queryExistingStmt, serviceInfo).Get(&serviceInfo)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"querying cloud service for application %q and provider id %q: %w", applicationName, providerID, err)
		} else if errors.Is(err, sqlair.ErrNoRows) {
			// Nothing already exists so create a new net node and the cloud
			// service.
			netNodeUUID, cloudServiceUUID, err := st.createCloudService(ctx, tx, serviceInfo)
			if err != nil {
				return errors.Errorf("creating cloud service for application %q: %w", applicationName, err)
			}
			serviceInfo.NetNodeUUID = netNodeUUID.String()
			serviceInfo.UUID = cloudServiceUUID.String()
		}

		if len(sAddrs) > 0 {
			// If we have addresses to insert, then first create the link layer
			// device (if needed) and then insert the addresses.
			if err := st.upsertCloudServiceAddresses(ctx, tx, serviceInfo, applicationName, sAddrs); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	if err != nil {
		return errors.Errorf("updating cloud service for application %q: %w", applicationName, err)
	}
	return nil
}

// createCloudService creates a cloud service for the specified application and
// its associated net node. It returns the net node UUID, the cloud service UUID
// and an error if any.
func (st *State) createCloudService(
	ctx context.Context,
	tx *sqlair.TX,
	serviceInfo cloudService,
) (domainnetwork.NetNodeUUID, uuid.UUID, error) {
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	if err != nil {
		return "", uuid.UUID{}, errors.Capture(err)
	}
	nodeDBUUID := dbUUID{UUID: netNodeUUID.String()}

	insertNetNodeStmt, err := st.Prepare(`
INSERT INTO net_node (uuid) VALUES ($dbUUID.uuid)
`, nodeDBUUID)
	if err != nil {
		return "", uuid.UUID{}, errors.Capture(err)
	}
	serviceInfo.NetNodeUUID = netNodeUUID.String()

	if err := tx.Query(ctx, insertNetNodeStmt, nodeDBUUID).Run(); err != nil {
		return "", uuid.UUID{}, errors.Errorf("inserting net node for cloud service application %q: %w", serviceInfo.ApplicationUUID, err)
	}

	insertCloudServiceStmt, err := st.Prepare(`
INSERT INTO k8s_service (*) VALUES ($cloudService.*)
`, serviceInfo)
	if err != nil {
		return "", uuid.UUID{}, errors.Capture(err)
	}

	cloudServiceUUID, err := uuid.NewUUID()
	if err != nil {
		return "", uuid.UUID{}, errors.Capture(err)
	}
	serviceInfo.UUID = cloudServiceUUID.String()
	if err := tx.Query(ctx, insertCloudServiceStmt, serviceInfo).Run(); err != nil {
		return "", uuid.UUID{}, errors.Errorf("inserting cloud service for application %q: %w", serviceInfo.ApplicationUUID, err)
	}
	return netNodeUUID, cloudServiceUUID, nil
}

func (st *State) upsertCloudServiceAddresses(
	ctx context.Context,
	tx *sqlair.TX,
	serviceInfo cloudService,
	applicationName string,
	addresses network.ProviderAddresses,
) error {
	var linkLayerDeviceUUID dbUUID
	queryLinkLayerDeviceFromServiceStmt, err := st.Prepare(`
SELECT lld.uuid AS &dbUUID.uuid
FROM   link_layer_device AS lld
JOIN   net_node AS nn ON nn.uuid = lld.net_node_uuid
WHERE  nn.uuid = $cloudService.net_node_uuid
		`, linkLayerDeviceUUID, serviceInfo)
	if err != nil {
		return errors.Capture(err)
	}

	// Retrieve the link layer device UUID for the service.
	var lldUUIDStr string
	err = tx.Query(ctx, queryLinkLayerDeviceFromServiceStmt, serviceInfo).Get(&linkLayerDeviceUUID)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("querying cloud service link layer device for application %q: %w", serviceInfo.ApplicationUUID, err)
	} else if errors.Is(err, sqlair.ErrNoRows) {
		// Ensure the address link layer device is inserted.
		lldUUID, err := st.insertCloudServiceDevice(ctx, tx, applicationName, serviceInfo.NetNodeUUID)
		if err != nil {
			return errors.Errorf("inserting cloud service link layer device for application %q: %w", serviceInfo.ApplicationUUID, err)
		}
		lldUUIDStr = lldUUID.String()
	} else {
		lldUUIDStr = linkLayerDeviceUUID.UUID
	}

	// Before inserting the new addresses, we need to remove any existing
	// ones for the given application and provider id.
	if err := st.deleteCloudServiceAddresses(ctx, tx, serviceInfo.ApplicationUUID, serviceInfo.ProviderID); err != nil {
		return errors.Capture(err)
	}
	if err := st.insertCloudServiceAddresses(ctx, tx, lldUUIDStr, serviceInfo.NetNodeUUID, addresses); err != nil {
		return errors.Errorf("inserting cloud service addresses for application %q: %w", applicationName, err)
	}
	return nil
}

func (st *State) insertCloudServiceDevice(
	ctx context.Context, tx *sqlair.TX, applicationName, netNodeUUID string,
) (uuid.UUID, error) {
	// For cloud services, the device is a placeholder without
	// a MAC address and once inserted, not updated. It just exists
	// to tie the address to the net node corresponding to the
	// cloud service.
	devUUID, err := uuid.NewUUID()
	if err != nil {
		return uuid.UUID{}, errors.Capture(err)
	}
	cloudServiceDeviceInfo := cloudServiceDevice{
		UUID:              devUUID.String(),
		Name:              fmt.Sprintf("placeholder for %q cloud service", applicationName),
		DeviceTypeID:      int(domainnetwork.DeviceTypeUnknown),
		VirtualPortTypeID: int(domainnetwork.NonVirtualPortType),
		NetNodeID:         netNodeUUID,
	}
	insertCloudServiceDeviceStmt, err := st.Prepare(`
INSERT INTO link_layer_device (*) VALUES ($cloudServiceDevice.*)
`, cloudServiceDeviceInfo)
	if err != nil {
		return uuid.UUID{}, errors.Capture(err)
	}

	if err := tx.Query(ctx, insertCloudServiceDeviceStmt, cloudServiceDeviceInfo).Run(); err != nil {
		return uuid.UUID{}, errors.Capture(err)
	}
	return devUUID, nil
}

func (st *State) deleteCloudServiceAddresses(ctx context.Context, tx *sqlair.TX, appUUID coreapplication.ID, providerID string) error {
	cloudService := cloudService{
		ApplicationUUID: appUUID,
		ProviderID:      providerID,
	}
	deleteAddressStmt, err := st.Prepare(`
DELETE FROM ip_address
WHERE device_uuid IN (
    SELECT device_uuid
    FROM   link_layer_device AS lld
    JOIN   net_node AS nn ON nn.uuid = lld.net_node_uuid
    JOIN   k8s_service AS ks ON ks.net_node_uuid = nn.uuid
    WHERE  ks.application_uuid = $cloudService.application_uuid
    AND    ks.provider_id = $cloudService.provider_id
);
`, cloudService)
	if err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, deleteAddressStmt, cloudService).Run(); err != nil {
		return errors.Errorf("removing cloud service addresses for application %q and providerID %q: %w", appUUID, providerID, err)
	}
	return nil
}

// addressTypeForUnspecifiedCIDR returns the address type based on the provided
// unspecified CIDR. These unspecified CIDRs are in the placeholder subnets
// for kubernetes until they can be filled in with actual data. When that happens,
// this approach will need to be revisited.
func addressTypeForUnspecifiedCIDR(cidr string) network.AddressType {
	switch cidr {
	case "0.0.0.0/0":
		return network.IPv4Address
	case "::/0":
		return network.IPv6Address
	default:
		return ""
	}
}

func (st *State) k8sSubnetUUIDsByAddressType(ctx context.Context, tx *sqlair.TX) (map[network.AddressType]string, error) {
	result := make(map[network.AddressType]string)
	subnetStmt, err := st.Prepare(`
 SELECT &subnet.*
 FROM subnet
 `, subnet{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var subnets []subnet
	if err = tx.Query(ctx, subnetStmt).GetAll(&subnets); err != nil {
		return nil, errors.Errorf("getting subnet uuid: %w", err)
	}
	// Note: Today there are only two k8s subnets, which are a placeholders.
	// Finding the subnet for the ip address will be more complex
	// in the future.
	if len(subnets) != 2 {
		return nil, errors.Errorf("expected 2 subnet uuid, got %d", len(subnets))
	}

	for _, subnet := range subnets {
		addrType := addressTypeForUnspecifiedCIDR(subnet.CIDR)
		result[addrType] = subnet.UUID
	}
	return result, nil
}

func (st *State) insertCloudServiceAddresses(
	ctx context.Context, tx *sqlair.TX, linkLayerDeviceUUID string, netNodeUUID string, addresses network.ProviderAddresses) error {
	if len(addresses) == 0 {
		return nil
	}

	subnetUUIDs, err := st.k8sSubnetUUIDsByAddressType(ctx, tx)
	if err != nil {
		return errors.Capture(err)
	}

	ipAddresses := make([]ipAddress, len(addresses))
	for i, address := range addresses {
		// Create a UUID for new addresses.
		addrUUID, err := uuid.NewUUID()
		if err != nil {
			return errors.Capture(err)
		}
		subnetUUID, ok := subnetUUIDs[address.AddressType()]
		if !ok {
			// Note: This is a programming error. Today the K8S subnets are
			// placeholders which should always be created when a model is
			// added.
			return errors.Errorf("subnet for address type %q not found", address.AddressType())
		}
		ipAddresses[i] = ipAddress{
			AddressUUID:  addrUUID.String(),
			Value:        address.Value,
			NetNodeUUID:  netNodeUUID,
			SubnetUUID:   subnetUUID,
			ConfigTypeID: int(ipaddress.MarshallConfigType(address.ConfigType)),
			TypeID:       int(ipaddress.MarshallAddressType(address.AddressType())),
			OriginID:     int(ipaddress.MarshallOrigin(network.OriginProvider)),
			ScopeID:      int(ipaddress.MarshallScope(address.AddressScope())),
			DeviceID:     linkLayerDeviceUUID,
		}
	}

	insertAddressStmt, err := st.Prepare(`
INSERT INTO ip_address (*)
VALUES ($ipAddress.*);
`, ipAddress{})
	if err != nil {
		return errors.Capture(err)
	}

	for _, ipAddress := range ipAddresses {
		if err = tx.Query(ctx, insertAddressStmt, ipAddress).Run(); err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}

// InitialWatchStatementApplicationsWithPendingCharms returns the initial
// namespace query for the applications with pending charms watcher.
func (st *State) InitialWatchStatementApplicationsWithPendingCharms() (string, eventsource.NamespaceQuery) {
	queryFunc := func(ctx context.Context, runner database.TxnRunner) ([]string, error) {
		stmt, err := st.Prepare(`
SELECT a.uuid AS &applicationID.uuid
FROM application a
JOIN charm c ON a.charm_uuid = c.uuid
WHERE c.available = FALSE
`, applicationID{})
		if err != nil {
			return nil, errors.Capture(err)
		}

		var results []applicationID
		err = runner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			err := tx.Query(ctx, stmt).GetAll(&results)
			if errors.Is(err, sqlair.ErrNoRows) {
				return nil
			}
			return errors.Capture(err)
		})
		if err != nil {
			return nil, errors.Errorf("querying requested applications that have pending charms: %w", err)
		}

		return transform.Slice(results, func(r applicationID) string {
			return r.ID.String()
		}), nil
	}
	return "application", queryFunc
}

// InitialWatchStatementApplicationConfigHash returns the initial namespace
// query for the application config hash watcher.
func (st *State) InitialWatchStatementApplicationConfigHash(appName string) (string, eventsource.NamespaceQuery) {
	queryFunc := func(ctx context.Context, runner database.TxnRunner) ([]string, error) {
		app := applicationName{Name: appName}
		stmt, err := st.Prepare(`
SELECT &applicationConfigHash.*
FROM application_config_hash ach
JOIN application a ON a.uuid = ach.application_uuid
WHERE a.name = $applicationName.name
`, app, applicationConfigHash{})
		if err != nil {
			return nil, errors.Capture(err)
		}
		var result []applicationConfigHash
		err = runner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			err := tx.Query(ctx, stmt, app).GetAll(&result)
			if errors.Is(err, sqlair.ErrNoRows) {
				return nil
			}
			return errors.Capture(err)
		})
		if err != nil {
			return nil, errors.Errorf("querying unit IDs for %q: %w", appName, err)
		}
		hashes := make([]string, len(result))
		for i, r := range result {
			hashes[i] = r.SHA256
		}
		return hashes, nil
	}
	return "application_config_hash", queryFunc
}

// InitialWatchStatementApplications returns the initial namespace
// query for applications events, as well as the watcher namespace to watch.
func (st *State) InitialWatchStatementApplications() (string, eventsource.NamespaceQuery) {
	queryFunc := func(ctx context.Context, runner database.TxnRunner) ([]string, error) {
		stmt, err := st.Prepare(`
SELECT &applicationID.* FROM application
`, applicationID{})
		if err != nil {
			return nil, errors.Capture(err)
		}
		var result []applicationID
		err = runner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			return tx.Query(ctx, stmt).GetAll(&result)
		})
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return nil, errors.Errorf("querying for initial watch statement: %w", err)
		}
		return transform.Slice(result, func(a applicationID) string { return a.ID.String() }), nil
	}
	return "application", queryFunc
}

// GetNetNodeUUIDByUnitName returns the net node UUID for the named unit or the
// cloud service associated with the unit's application. This method is meant
// to be used in the WatchUnitAddressesHash watcher as a filter for ip address
// changes.
//
// It first checks if a cloud service exists for the application, if there is
// then it returns the net node UUID for the cloud service without checking for
// the unit's net node since it corresponds to the cloud container address
// instead.
//
// If the unit does not exist an error satisfying
// [applicationerrors.UnitNotFound] will be returned.
func (st *State) GetNetNodeUUIDByUnitName(ctx context.Context, name coreunit.Name) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	unitName := unitName{Name: name}
	k8sServiceNetNodeStmt, err := st.Prepare(`
SELECT k.net_node_uuid AS &netNodeUUID.uuid
FROM   k8s_service k
JOIN   application a ON a.uuid = k.application_uuid
JOIN   unit u ON u.application_uuid = a.uuid
WHERE  u.name = $unitName.name
`, unitName, netNodeUUID{})
	if err != nil {
		return "", errors.Capture(err)
	}
	unitNetNodeStmt, err := st.Prepare(`
SELECT net_node_uuid AS &netNodeUUID.uuid
FROM   unit
WHERE  name = $unitName.name
`, unitName, netNodeUUID{})
	if err != nil {
		return "", errors.Capture(err)
	}

	var nodeUUID netNodeUUID
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// First try to get the net node UUID from the k8s service.
		err := tx.Query(ctx, k8sServiceNetNodeStmt, unitName).Get(&nodeUUID)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		} else if err == nil {
			return nil
		}

		// If nothing found, try to get the net node UUID from the unit.
		err = tx.Query(ctx, unitNetNodeStmt, unitName).Get(&nodeUUID)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		} else if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("%w: %s", applicationerrors.UnitNotFound, name)
		}

		return nil
	}); err != nil {
		return "", errors.Capture(err)
	}

	return nodeUUID.NetNodeUUID, nil
}

// GetAddressesHash returns the sha256 hash of the application unit and cloud
// service (if any) addresses along with the associated endpoint bindings.
//
// NOTE(nvinuesa): This method is used in the `WatchUnitAddressesHash` watcher
// to validate if a change has indeed occurred. The issue with this behavior is
// that it will get fired very often and the probability of a change that is
// of interest for the unit is low.
// A possible future improvement would be to accumulate the change events and
// check whether the unit of interest has been affaceted, before hitting the db.
func (st *State) GetAddressesHash(ctx context.Context, appUUID coreapplication.ID, netNodeUUID string) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var (
		spaceAddresses   []spaceAddress
		endpointBindings map[string]network.SpaceUUID
	)
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		spaceAddresses, err = st.getNetNodeSpaceAddresses(ctx, tx, netNodeUUID)
		if err != nil {
			return errors.Capture(err)
		}
		endpointBindings, err = st.getEndpointBindings(ctx, tx, appUUID)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	}); err != nil {
		return "", errors.Capture(err)
	}

	return st.hashAddressesAndEndpoints(spaceAddresses, endpointBindings)
}

func (st *State) getNetNodeSpaceAddresses(ctx context.Context, tx *sqlair.TX, netNode string) ([]spaceAddress, error) {
	var result []spaceAddress

	netNodeUUID := netNodeUUID{NetNodeUUID: netNode}
	stmt, err := st.Prepare(`
SELECT
    ip.address_value AS &spaceAddress.address_value,
    ip.type_id AS &spaceAddress.type_id,
    ip.scope_id AS &spaceAddress.scope_id,
    sn.space_uuid AS &spaceAddress.space_uuid
FROM      net_node nn
JOIN      link_layer_device lld ON lld.net_node_uuid = nn.uuid
JOIN      ip_address ip ON ip.device_uuid = lld.uuid
LEFT JOIN subnet sn ON sn.uuid = ip.subnet_uuid
WHERE     nn.uuid = $netNodeUUID.uuid;
`, netNodeUUID, spaceAddress{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, netNodeUUID).GetAll(&result)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("querying net node %q space addresses: %w", netNode, err)
	}
	return result, nil
}

// hashAddressesAndEndpoints returns a hash of the addresses and endpoint
// bindings.
//
// NOTE(nvinuesa): This type of methods should usually be located in the
// service layer, since it contains pure business logic. However, this method
// is used in the initial query of the `WatchUnitAddressesHash` watcher, and
// since the initial query must return a []string, we cannot call this method
// from the service layer. Another solution would have been passing this as a
// closure to the watcher, but this would have required to create new service
// layer structs that match exactly the ones in state, which is not a desirable
// pattern.
func (st *State) hashAddressesAndEndpoints(addresses []spaceAddress, endpointBindings map[string]network.SpaceUUID) (string, error) {
	if len(addresses) == 0 && len(endpointBindings) == 0 {
		return "", nil
	}

	hash := sha256.New()
	// Sort addresses by value, which is needed for the hash to be consistent.
	sort.Slice(addresses, func(i, j int) bool {
		return addresses[i].Value < addresses[j].Value
	})

	// Add the hash parts for each address.
	for _, spaceAddress := range addresses {
		if err := st.hashAddress(hash, spaceAddress); err != nil {
			return "", errors.Capture(err)
		}
	}

	// Sort the endpoint bindings by key (endpoint name) and add each binding to
	// the hash.
	endpointNames := make([]string, 0, len(endpointBindings))
	for name := range endpointBindings {
		endpointNames = append(endpointNames, name)
	}
	sort.Strings(endpointNames)
	for _, endpointName := range endpointNames {
		if _, err := hash.Write(fmt.Appendf(nil, "%s:%s", endpointName, endpointBindings[endpointName])); err != nil {
			return "", errors.Capture(err)
		}
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (st *State) hashAddress(writer io.Writer, address spaceAddress) error {
	if _, err := writer.Write([]byte(address.Value)); err != nil {
		return errors.Errorf("hashing address %q: %w", address.Value, err)
	}
	addressType := strconv.Itoa(address.TypeID)
	if _, err := writer.Write([]byte(addressType)); err != nil {
		return errors.Errorf("hashing address type %q: %w", addressType, err)
	}
	addressScope := strconv.Itoa(address.ScopeID)
	if _, err := writer.Write([]byte(addressScope)); err != nil {
		return errors.Errorf("hashing address scope %q: %w", addressScope, err)
	}
	spaceUUID := network.AlphaSpaceId
	if address.SpaceUUID.Valid {
		spaceUUID = address.SpaceUUID.V
	}
	if _, err := writer.Write([]byte(spaceUUID)); err != nil {
		return errors.Errorf("hashing space uuid %q: %w", spaceUUID, err)
	}
	return nil
}

// GetApplicationsWithPendingCharmsFromUUIDs returns the application IDs for the
// applications with pending charms from the specified UUIDs.
func (st *State) GetApplicationsWithPendingCharmsFromUUIDs(ctx context.Context, uuids []coreapplication.ID) ([]coreapplication.ID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	type applicationIDs []coreapplication.ID

	stmt, err := st.Prepare(`
SELECT a.uuid AS &applicationID.uuid
FROM application AS a
JOIN charm AS c ON a.charm_uuid = c.uuid
WHERE a.uuid IN ($applicationIDs[:]) AND c.available = FALSE
`, applicationID{}, applicationIDs{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var results []applicationID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, applicationIDs(uuids)).GetAll(&results)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return errors.Capture(err)
	})
	if err != nil {
		return nil, errors.Errorf("querying requested applications that have pending charms: %w", err)
	}

	return transform.Slice(results, func(r applicationID) coreapplication.ID {
		return r.ID
	}), nil
}

// GetCharmIDByApplicationName returns a charm ID by application name. It
// returns an error if the charm can not be found by the name. This can also be
// used as a cheap way to see if a charm exists without needing to load the
// charm metadata.
//
// Returns [applicationerrors.ApplicationNameNotValid] if the name is not valid,
// [applicationerrors.ApplicationNotFound] if the application is not found, and
// [applicationerrors.CharmNotFound] if the charm is not found.
func (st *State) GetCharmIDByApplicationName(ctx context.Context, name string) (corecharm.ID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var result corecharm.ID
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		appUUID, err := st.lookupApplication(ctx, tx, name)
		if err != nil {
			return errors.Errorf("looking up application %q: %w", name, err)
		}

		result, err = st.getCharmIDByApplicationID(ctx, tx, appUUID)
		if err != nil {
			return errors.Errorf("getting charm for application %q: %w", name, err)
		}

		return nil
	}); err != nil {
		return "", errors.Capture(err)
	}

	return result, nil
}

// GetCharmByApplicationID returns the charm for the specified application
// ID.
// This method should be used sparingly, as it is not efficient. It should
// be only used when you need the whole charm, otherwise use the more specific
// methods.
//
// If the application does not exist, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (st *State) GetCharmByApplicationID(ctx context.Context, appUUID coreapplication.ID) (charm.Charm, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return charm.Charm{}, errors.Capture(err)
	}

	var ch charm.Charm
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		charmUUID, err := st.getCharmIDByApplicationID(ctx, tx, appUUID)
		if err != nil {
			return errors.Errorf("getting charm ID from application ID %q: %w", appUUID, err)
		}

		// Now get the charm by the UUID, but if it doesn't exist, return an
		// error.
		chIdent := charmID{UUID: charmUUID}
		ch, _, err = st.getCharm(ctx, tx, chIdent)
		if err != nil {
			return errors.Errorf("getting charm for application %q: %w", appUUID, err)
		}
		return nil
	}); err != nil {
		return ch, errors.Capture(err)
	}

	return ch, nil
}

// SetApplicationCharm sets a new charm for the specified application using
// the provided parameters and validates changes.
// Some validation needs to be transactional:
// - relation compatibility needs to be transactional, since a new or removed
// relation can change the validation result.
func (st *State) SetApplicationCharm(ctx context.Context, id coreapplication.ID,
	params application.UpdateCharmParams) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		meta := params.Charm.Meta()
		if meta == nil {
			return errors.Errorf("charm doesn't have any valid metadata")
		}

		relations, err := st.getAllNonPeerRelationInfo(ctx, tx, id)
		if err != nil {
			return errors.Errorf("fetching all relation for application %q: %w", id, err)
		}
		if err := precheckUpgradeRelation(meta, relations); err != nil {
			return err
		}

		//TODO(storage) - update charm and storage directive for app

		bindings := transform.Map(params.EndpointBindings, func(k string, v network.SpaceName) (string, string) {
			return k, v.String()
		})
		return st.mergeApplicationEndpointBindings(ctx, tx, id.String(), bindings, false)
	}); err != nil {
		return errors.Capture(err)
	}

	return nil
}

// GetApplicationIDByUnitName returns the application ID for the named unit.
//
// Returns an error satisfying [applicationerrors.UnitNotFound] if the unit
// doesn't exist.
func (st *State) GetApplicationIDByUnitName(
	ctx context.Context,
	name coreunit.Name,
) (coreapplication.ID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	unit := unitName{Name: name}
	queryUnit := `
SELECT application_uuid AS &applicationID.uuid
FROM unit
WHERE name = $unitName.name;
`
	query, err := st.Prepare(queryUnit, applicationID{}, unit)
	if err != nil {
		return "", errors.Errorf("preparing query for unit %q: %w", name, err)
	}

	var app applicationID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query, unit).Get(&app)
		if errors.Is(err, sqlair.ErrNoRows) {
			return applicationerrors.UnitNotFound
		}
		return err
	})
	if err != nil {
		return "", errors.Errorf("querying unit %q application id: %w", name, err)
	}
	return app.ID, nil
}

// GetApplicationIDAndNameByUnitName returns the application ID and name for the
// named unit.
//
// Returns an error satisfying [applicationerrors.UnitNotFound] if the unit
// doesn't exist.
func (st *State) GetApplicationIDAndNameByUnitName(
	ctx context.Context,
	name coreunit.Name,
) (coreapplication.ID, string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", "", errors.Capture(err)
	}

	unit := unitName{Name: name}
	queryUnit := `
SELECT a.uuid AS &applicationIDAndName.uuid,
       a.name AS &applicationIDAndName.name
FROM unit u
JOIN application a ON a.uuid = u.application_uuid
WHERE u.name = $unitName.name;
`
	query, err := st.Prepare(queryUnit, applicationIDAndName{}, unit)
	if err != nil {
		return "", "", errors.Errorf("preparing query for unit %q: %w", name, err)
	}

	var app applicationIDAndName
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query, unit).Get(&app)
		if errors.Is(err, sqlair.ErrNoRows) {
			return applicationerrors.UnitNotFound
		}
		return err
	})
	if err != nil {
		return "", "", errors.Errorf("querying unit %q application id: %w", name, err)
	}
	return app.ID, app.Name, nil
}

// GetCharmModifiedVersion looks up the charm modified version of the given
// application.
//
// Returns [applicationerrors.ApplicationNotFound] if the
// application is not found.
func (st *State) GetCharmModifiedVersion(ctx context.Context, id coreapplication.ID) (int, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return -1, errors.Capture(err)
	}

	type cmv struct {
		CharmModifiedVersion int `db:"charm_modified_version"`
	}

	appUUID := applicationID{ID: id}
	queryApp := `
SELECT &cmv.*
FROM application
WHERE uuid = $applicationID.uuid
`
	query, err := st.Prepare(queryApp, cmv{}, appUUID)
	if err != nil {
		return -1, errors.Errorf("preparing query for application %q: %w", id, err)
	}

	var version cmv
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query, appUUID).Get(&version)
		if errors.Is(err, sqlair.ErrNoRows) {
			return applicationerrors.ApplicationNotFound
		}
		return err
	})
	if err != nil {
		return -1, errors.Errorf("querying charm modified version: %w", err)
	}
	return version.CharmModifiedVersion, err
}

// GetAsyncCharmDownloadInfo gets the charm download for the specified
// application, returning an error satisfying
// [applicationerrors.CharmAlreadyAvailable] if the application is already
// downloading a charm, or [applicationerrors.ApplicationNotFound] if the
// application is not found.
func (st *State) GetAsyncCharmDownloadInfo(ctx context.Context, appID coreapplication.ID) (application.CharmDownloadInfo, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return application.CharmDownloadInfo{}, errors.Capture(err)
	}

	appIdent := applicationID{ID: appID}

	query, err := st.Prepare(`
SELECT &applicationCharmDownloadInfo.*
FROM v_application_charm_download_info
WHERE application_uuid = $applicationID.uuid
`, applicationCharmDownloadInfo{}, appIdent)
	if err != nil {
		return application.CharmDownloadInfo{}, errors.Errorf("preparing query for application %q: %w", appID, err)
	}

	var info applicationCharmDownloadInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query, appIdent).Get(&info)
		if errors.Is(err, sqlair.ErrNoRows) {
			return applicationerrors.ApplicationNotFound
		} else if err != nil {
			return err
		}
		if info.Available {
			return applicationerrors.CharmAlreadyAvailable
		}
		return nil
	})
	if err != nil {
		return application.CharmDownloadInfo{}, errors.Errorf("reserving charm download for application %q: %w", appID, err)
	}

	// We can only reserve charms from CharmHub charms.
	if source, err := decodeCharmSource(info.SourceID); err != nil {
		return application.CharmDownloadInfo{}, errors.Errorf("decoding charm source for %q: %w", appID, err)
	} else if source != charm.CharmHubSource {
		return application.CharmDownloadInfo{}, errors.Errorf("unexpected charm source for %q: %w", appID, applicationerrors.CharmProvenanceNotValid)
	}

	charmUUID, err := corecharm.ParseID(info.CharmUUID)
	if err != nil {
		return application.CharmDownloadInfo{}, errors.Errorf("encoding charm uuid for %q: %w", appID, err)
	}

	provenance, err := decodeProvenance(info.Provenance)
	if err != nil {
		return application.CharmDownloadInfo{}, errors.Errorf("decoding charm provenance: %w", err)
	}

	return application.CharmDownloadInfo{
		CharmUUID: charmUUID,
		Name:      info.Name,
		SHA256:    info.Hash,
		DownloadInfo: charm.DownloadInfo{
			Provenance:         provenance,
			DownloadURL:        info.DownloadURL,
			CharmhubIdentifier: info.CharmhubIdentifier,
			DownloadSize:       info.DownloadSize,
		},
	}, nil
}

// ResolveCharmDownload resolves the charm download for the specified
// application, updating the charm with the specified charm information.
// This will only set mutable charm fields. Currently this will also set
// actions.yaml, although that will be removed once the charmhub store
// provides this information.
// Returns an error satisfying [applicationerrors.CharmNotFound] if the charm
// is not found, and [applicationerrors.CharmAlreadyResolved] if the charm is
// already resolved.
func (st *State) ResolveCharmDownload(ctx context.Context, id corecharm.ID, info application.ResolvedCharmDownload) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	charmUUID := charmID{UUID: id}

	resolvedQuery := `
SELECT &charmAvailable.*
FROM charm
WHERE uuid = $charmID.uuid
`
	resolvedStmt, err := st.Prepare(resolvedQuery, charmUUID, charmAvailable{})
	if err != nil {
		return errors.Errorf("preparing query for charm %q: %w", id, err)
	}

	chState := resolveCharmState{
		ArchivePath:     info.ArchivePath,
		ObjectStoreUUID: info.ObjectStoreUUID.String(),
		LXDProfile:      info.LXDProfile,
	}

	charmQuery := `
UPDATE charm
SET
	archive_path = $resolveCharmState.archive_path,
	object_store_uuid = $resolveCharmState.object_store_uuid,
	lxd_profile = $resolveCharmState.lxd_profile,
	available = TRUE
WHERE uuid = $charmID.uuid;`
	charmStmt, err := st.Prepare(charmQuery, charmUUID, chState)
	if err != nil {
		return errors.Errorf("preparing query: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var available charmAvailable
		err := tx.Query(ctx, resolvedStmt, charmUUID).Get(&available)
		if errors.Is(err, sqlair.ErrNoRows) {
			return applicationerrors.CharmNotFound
		} else if err != nil {
			return errors.Capture(err)
		} else if available.Available {
			// If the charm is already resolved, we don't need to provide
			// any additional information.
			return applicationerrors.CharmAlreadyResolved
		}

		// Write the charm actions.yaml, this will actually disappear once the
		// charmhub store provides this information.
		if err = st.setCharmActions(ctx, tx, id, info.Actions); err != nil {
			return errors.Errorf("setting charm actions for %q: %w", id, err)
		}

		if err := tx.Query(ctx, charmStmt, charmUUID, chState).Run(); err != nil {
			return errors.Errorf("updating charm state: %w", err)
		}

		return nil
	})
	if err != nil {
		return errors.Errorf("resolving charm download for %q: %w", id, err)
	}
	return nil
}

// GetApplicationsForRevisionUpdater returns all the applications for the
// revision updater. This will only return charmhub charms, for applications
// that are alive.
// This will return an empty slice if there are no applications.
func (st *State) GetApplicationsForRevisionUpdater(ctx context.Context) ([]application.RevisionUpdaterApplication, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	revUpdaterAppQuery := `
SELECT &revisionUpdaterApplication.*
FROM v_revision_updater_application
`

	revUpdaterAppStmt, err := st.Prepare(revUpdaterAppQuery, revisionUpdaterApplication{})
	if err != nil {
		return nil, errors.Errorf("preparing query for revision updater applications: %w", err)
	}

	numUnitsQuery := `
SELECT &revisionUpdaterApplicationNumUnits.*
FROM v_revision_updater_application_unit
`

	numUnitsStmt, err := st.Prepare(numUnitsQuery, revisionUpdaterApplicationNumUnits{})
	if err != nil {
		return nil, errors.Errorf("preparing query for number of units: %w", err)
	}

	var apps []revisionUpdaterApplication
	var appUnitCounts []revisionUpdaterApplicationNumUnits
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, revUpdaterAppStmt).GetAll(&apps)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}

		err = tx.Query(ctx, numUnitsStmt).GetAll(&appUnitCounts)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}

		return err
	})
	if err != nil {
		return nil, errors.Errorf("querying revision updater applications: %w", err)
	}

	unitCounts := make(map[string]int)
	for _, app := range appUnitCounts {
		unitCounts[app.UUID] = app.NumUnits
	}

	return transform.SliceOrErr(apps, func(r revisionUpdaterApplication) (application.RevisionUpdaterApplication, error) {
		// The following architecture IDs should never diverge, as we only
		// support homogenous architectures. Yet we have two sources of truth.
		charmArch, err := decodeArchitecture(r.CharmArchitectureID)
		if err != nil {
			return application.RevisionUpdaterApplication{}, errors.Errorf("decoding architecture: %w", err)
		}

		appArch, err := decodeArchitecture(r.PlatformArchitectureID)
		if err != nil {
			return application.RevisionUpdaterApplication{}, errors.Errorf("decoding architecture: %w", err)
		}

		risk, err := decodeRisk(r.ChannelRisk)
		if err != nil {
			return application.RevisionUpdaterApplication{}, errors.Errorf("decoding risk: %w", err)
		}

		osType, err := decodeOSType(r.PlatformOSID)
		if err != nil {
			return application.RevisionUpdaterApplication{}, errors.Errorf("decoding os type: %w", err)
		}

		return application.RevisionUpdaterApplication{
			Name: r.Name,
			CharmLocator: charm.CharmLocator{
				Name:         r.ReferenceName,
				Revision:     r.Revision,
				Source:       charm.CharmHubSource,
				Architecture: charmArch,
			},
			Origin: application.Origin{
				ID:       r.CharmhubIdentifier,
				Revision: r.Revision,
				Channel: deployment.Channel{
					Track:  r.ChannelTrack,
					Risk:   risk,
					Branch: r.ChannelBranch,
				},
				Platform: deployment.Platform{
					Channel:      r.PlatformChannel,
					OSType:       osType,
					Architecture: appArch,
				},
			},
			NumUnits: unitCounts[r.UUID],
		}, nil
	})
}

// GetApplicationConfigAndSettings returns the application config and settings
// attributes for the application ID.
//
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (st *State) GetApplicationConfigAndSettings(ctx context.Context, appID coreapplication.ID) (map[string]application.ApplicationConfig, application.ApplicationSettings, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, application.ApplicationSettings{}, errors.Capture(err)
	}

	// We don't currently check for life in the old code, it might though be
	// worth checking if the application is not dead.
	ident := applicationID{ID: appID}

	var configs []applicationConfig
	var settings applicationSettings
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.checkApplicationNotDead(ctx, tx, appID)
		if err != nil {
			return errors.Capture(err)
		}

		configs, err = st.getApplicationConfig(ctx, tx, ident)
		if err != nil {
			return errors.Errorf("querying application config: %w", err)
		}

		settings, err = st.getApplicationSettings(ctx, tx, ident)
		if err != nil {
			return errors.Errorf("querying application settings: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, application.ApplicationSettings{}, errors.Errorf("querying application config: %w", err)
	}

	result := make(map[string]application.ApplicationConfig)
	for _, c := range configs {
		typ, err := decodeConfigType(c.Type)
		if err != nil {
			return nil, application.ApplicationSettings{}, errors.Errorf("decoding config type: %w", err)
		}

		result[c.Key] = application.ApplicationConfig{
			Type:  typ,
			Value: c.Value,
		}
	}
	return result, application.ApplicationSettings{
		Trust: settings.Trust,
	}, nil
}

// GetApplicationConfigWithDefaults returns the application config attributes
// for the configuration, or the charm default value if the config attribute is
// not set.
//
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (st *State) GetApplicationConfigWithDefaults(ctx context.Context, appID coreapplication.ID) (map[string]application.ApplicationConfig, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	ident := applicationID{ID: appID}
	var configs []applicationConfig
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.checkApplicationNotDead(ctx, tx, appID)
		if err != nil {
			return errors.Capture(err)
		}

		configs, err = st.getApplicationConfigWithDefaults(ctx, tx, ident)
		if err != nil {
			return errors.Errorf("querying application config: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, errors.Errorf("querying application config: %w", err)
	}

	result := make(map[string]application.ApplicationConfig)
	for _, c := range configs {
		typ, err := decodeConfigType(c.Type)
		if err != nil {
			return nil, errors.Errorf("decoding config type: %w", err)
		}

		result[c.Key] = application.ApplicationConfig{
			Type:  typ,
			Value: c.Value,
		}
	}

	return result, nil
}

// GetApplicationTrustSetting returns the application trust setting.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (st *State) GetApplicationTrustSetting(ctx context.Context, appID coreapplication.ID) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	// We don't currently check for life in the old code, it might though be
	// worth checking if the application is not dead.
	ident := applicationID{ID: appID}

	settingsQuery := `
SELECT trust AS &applicationSettings.trust
FROM application_setting
WHERE application_uuid = $applicationID.uuid;`

	settingsStmt, err := st.Prepare(settingsQuery, applicationSettings{}, ident)
	if err != nil {
		return false, errors.Errorf("preparing query for application trust setting: %w", err)
	}

	var settings applicationSettings
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkApplicationNotDead(ctx, tx, appID); err != nil {
			return errors.Capture(err)
		}

		if err := tx.Query(ctx, settingsStmt, ident).Get(&settings); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying application settings: %w", err)
		}

		return nil
	})
	if err != nil {
		return false, errors.Errorf("querying application config: %w", err)
	}

	return settings.Trust, nil
}

// UpdateApplicationConfigAndSettings updates the application config attributes
// using the configuration.
func (st *State) UpdateApplicationConfigAndSettings(
	ctx context.Context,
	appID coreapplication.ID,
	config map[string]application.ApplicationConfig,
	settings application.UpdateApplicationSettingsArg,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	ident := applicationID{ID: appID}

	upsertQuery := `
INSERT INTO application_config (*)
VALUES ($setApplicationConfig.*)
ON CONFLICT(application_uuid, "key") DO UPDATE SET
	value = excluded.value
`
	upsertSettingsQuery := `
INSERT INTO application_setting (*)
VALUES ($setApplicationSettings.*)
ON CONFLICT(application_uuid) DO UPDATE SET
	trust = excluded.trust;
	`

	upsertStmt, err := st.Prepare(upsertQuery, setApplicationConfig{})
	if err != nil {
		return errors.Errorf("preparing upsert query: %w", err)
	}
	upsertSettingsStmt, err := st.Prepare(upsertSettingsQuery, setApplicationSettings{})
	if err != nil {
		return errors.Errorf("preparing upsert settings query: %w", err)
	}

	upserts := make([]setApplicationConfig, 0, len(config))
	for k, cfgVal := range config {
		typeID, err := encodeConfigType(cfgVal.Type)
		if err != nil {
			return errors.Errorf("encoding config type: %w", err)
		}
		upserts = append(upserts, setApplicationConfig{
			ApplicationUUID: ident.ID,
			Key:             k,
			Value:           cfgVal.Value,
			TypeID:          typeID,
		})
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkApplicationNotDead(ctx, tx, appID); err != nil {
			return errors.Capture(err)
		}

		if len(upserts) > 0 {
			if err := tx.Query(ctx, upsertStmt, upserts).Run(); err != nil {
				return errors.Errorf("upserting config: %w", err)
			}
		}

		if settings.Trust != nil {
			if err := tx.Query(ctx, upsertSettingsStmt, setApplicationSettings{
				ApplicationUUID: appID,
				Trust:           *settings.Trust,
			}).Run(); err != nil {
				return errors.Errorf("upserting settings: %w", err)
			}
		}

		if err := st.updateConfigHash(ctx, tx, ident); err != nil {
			return errors.Errorf("refreshing config hash: %w", err)
		}
		return nil
	})
	if err != nil {
		return errors.Errorf("updating application config: %w", err)
	}
	return nil
}

// UnsetApplicationConfigKeys removes the specified keys from the application
// config. If the key does not exist, it is ignored.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (st *State) UnsetApplicationConfigKeys(ctx context.Context, appID coreapplication.ID, keys []string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	ident := applicationID{ID: appID}

	// This isn't ideal, as we could request this in one query, but we need to
	// perform multiple queries to get the data. First is to get the application
	// availability, second to just get the application overlay config for the
	// charm config and the application settings for the trust config.
	appQuery := `
SELECT &applicationID.*
FROM application
WHERE uuid = $applicationID.uuid;
`
	deleteQuery := `
DELETE FROM application_config
WHERE application_uuid = $applicationID.uuid
AND key IN ($S[:]);
`
	settingsQuery := `
INSERT INTO application_setting (*)
VALUES ($setApplicationSettings.*)
ON CONFLICT(application_uuid) DO UPDATE SET
    trust = excluded.trust;
`

	appStmt, err := st.Prepare(appQuery, ident)
	if err != nil {
		return errors.Errorf("preparing query for application config: %w", err)
	}
	deleteStmt, err := st.Prepare(deleteQuery, ident, sqlair.S{})
	if err != nil {
		return errors.Errorf("preparing query for application config: %w", err)
	}
	settingsStmt, err := st.Prepare(settingsQuery, setApplicationSettings{})
	if err != nil {
		return errors.Errorf("preparing query for application config: %w", err)
	}

	removals := make(sqlair.S, len(keys))
	for i, k := range keys {
		removals[i] = k
	}
	removeTrust := slices.Contains(keys, "trust")

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, appStmt, ident).Get(&ident); errors.Is(err, sqlair.ErrNoRows) {
			return applicationerrors.ApplicationNotFound
		} else if err != nil {
			return errors.Errorf("querying application: %w", err)
		}

		if err := tx.Query(ctx, deleteStmt, removals, ident).Run(); internaldatabase.IsErrConstraintForeignKey(err) {
			return applicationerrors.ApplicationNotFound
		} else if err != nil {
			return errors.Errorf("deleting config: %w", err)
		}

		if !removeTrust {
			return nil
		}

		if err := tx.Query(ctx, settingsStmt, setApplicationSettings{
			ApplicationUUID: ident.ID,
			Trust:           false,
		}).Run(); err != nil {
			return errors.Errorf("deleting setting: %w", err)
		}

		return nil
	})
	if err != nil {
		return errors.Errorf("removing application config: %w", err)
	}
	return nil
}

// GetCharmConfigByApplicationID returns the charm config for the specified
// application ID.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
// If the charm for the application does not exist.
func (st *State) GetCharmConfigByApplicationID(ctx context.Context, appID coreapplication.ID) (corecharm.ID, charm.Config, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", charm.Config{}, errors.Capture(err)
	}

	appIdent := applicationID{ID: appID}

	appQuery := `
SELECT &charmUUID.*
FROM application
WHERE uuid = $applicationID.uuid;
`
	appStmt, err := st.Prepare(appQuery, appIdent, charmUUID{})
	if err != nil {
		return "", charm.Config{}, errors.Errorf("preparing query for charm config: %w", err)
	}

	var (
		ident       charmUUID
		charmConfig charm.Config
	)
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, appStmt, appIdent).Get(&ident); errors.Is(err, sqlair.ErrNoRows) {
			return applicationerrors.ApplicationNotFound
		} else if err != nil {
			return errors.Capture(err)
		}

		// In theory, this can return a CharmNotFound error. However, we retrieved
		// our identifier from a field with referential integrity, so in practise
		// this is impossible.
		// TODO(jack-w-shaw): Retrieve the charm config directly using the application
		// ID, instead of force-fitting the getCharmConfig method.
		charmUUID := ident.UUID
		charmConfig, err = st.getCharmConfig(ctx, tx, charmID{UUID: charmUUID})
		return errors.Capture(err)
	}); err != nil {
		return "", charm.Config{}, errors.Capture(err)
	}

	return ident.UUID, charmConfig, nil
}

// GetApplicationName returns the name of the specified application.
// The following errors may be returned:
// - [applicationerrors.ApplicationNotFound] if the application does not exist
func (st *State) GetApplicationName(ctx context.Context, appID coreapplication.ID) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var name string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		name, err = st.getApplicationName(ctx, tx, appID)
		return err
	})
	if err != nil {
		return "", errors.Capture(err)
	}
	return name, nil
}

// GetApplicationIDByName returns the application ID for the named application.
// The following errors may be returned:
// - [applicationerrors.ApplicationNotFound] if the application does not exist
func (st *State) GetApplicationIDByName(ctx context.Context, name string) (coreapplication.ID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var id coreapplication.ID
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		id, err = st.lookupApplication(ctx, tx, name)
		return err
	}); err != nil {
		return "", errors.Capture(err)
	}
	return id, nil
}

// ShouldAllowCharmUpgradeOnError indicates if the units of an application
// should upgrade to the latest version of the application charm even if they
// are in error state.
//
// An error satisfying [applicationerrors.ApplicationNotFoundError]
// is returned if the application doesn't exist.
func (st *State) ShouldAllowCharmUpgradeOnError(ctx context.Context, appName string) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	arg := getCharmUpgradeOnError{
		Name: appName,
	}

	stmt, err := st.Prepare(`
SELECT &getCharmUpgradeOnError.*
FROM   application
WHERE  name = $getCharmUpgradeOnError.name
`, arg)
	if err != nil {
		return false, errors.Capture(err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, arg).Get(&arg)
		if errors.Is(err, sqlair.ErrNoRows) {
			return applicationerrors.ApplicationNotFound
		}
		return errors.Capture(err)
	}); err != nil {
		return false, errors.Capture(err)
	}

	return arg.CharmUpgradeOnError, nil
}

// getApplicationName returns the application name. If no application is found,
// an error satisfying [applicationerrors.ApplicationNotFound] is returned.
func (st *State) getApplicationName(
	ctx context.Context,
	tx *sqlair.TX,
	id coreapplication.ID) (string, error) {
	arg := applicationIDAndName{
		ID: id,
	}
	stmt, err := st.Prepare(`
SELECT &applicationIDAndName.*
FROM   application
WHERE  uuid = $applicationIDAndName.uuid
`, arg)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, arg).Get(&arg)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", applicationerrors.ApplicationNotFound
	} else if err != nil {
		return "", errors.Capture(err)
	}

	return arg.Name, nil
}

// GetApplicationConfigHash returns the SHA256 hash of the application config
// for the specified application ID.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (st *State) GetApplicationConfigHash(ctx context.Context, appID coreapplication.ID) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	ident := applicationID{ID: appID}

	query := `
SELECT sha256 AS &applicationConfigHash.sha256
FROM application_config_hash
WHERE application_uuid = $applicationID.uuid;
`

	stmt, err := st.Prepare(query, applicationConfigHash{}, ident)
	if err != nil {
		return "", errors.Errorf("preparing query for application config hash: %w", err)
	}

	var hash applicationConfigHash
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkApplicationNotDead(ctx, tx, appID); err != nil {
			return errors.Capture(err)
		}

		if err := tx.Query(ctx, stmt, ident).Get(&hash); errors.Is(err, sqlair.ErrNoRows) {
			return applicationerrors.ApplicationNotFound
		} else if err != nil {
			return errors.Capture(err)
		}

		return nil
	}); err != nil {
		return "", errors.Capture(err)
	}

	return hash.SHA256, nil
}

// GetApplicationPlatformAndChannel returns the platform and channel for the
// specified application ID.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (st *State) GetApplicationCharmOrigin(ctx context.Context, appID coreapplication.ID) (application.CharmOrigin, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return application.CharmOrigin{}, errors.Capture(err)
	}

	ident := applicationID{ID: appID}

	queryOrigin := `
SELECT &applicationOrigin.*
FROM v_application_origin
WHERE uuid = $applicationID.uuid;`

	stmtOrigin, err := st.Prepare(queryOrigin, applicationOrigin{}, ident)
	if err != nil {
		return application.CharmOrigin{}, errors.Errorf("preparing query for application platform and channel: %w", err)
	}

	queryPlatformChannel := `
SELECT &applicationPlatformAndChannel.*
FROM v_application_platform_channel
WHERE application_uuid = $applicationID.uuid;
`
	stmtPlatformChannel, err := st.Prepare(queryPlatformChannel, applicationPlatformAndChannel{}, ident)
	if err != nil {
		return application.CharmOrigin{}, errors.Errorf("preparing query for application platform and channel: %w", err)
	}

	var (
		appOrigin       applicationOrigin
		appPlatformChan applicationPlatformAndChannel
	)
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkApplicationNotDead(ctx, tx, appID); err != nil {
			return errors.Errorf("getting application life: %w", err)
		}

		err := tx.Query(ctx, stmtOrigin, ident).Get(&appOrigin)
		if err != nil {
			return errors.Errorf("querying origin: %w", err)
		}

		err = tx.Query(ctx, stmtPlatformChannel, ident).Get(&appPlatformChan)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying platform and channel: %w", err)
		}

		return nil
	}); err != nil {
		return application.CharmOrigin{}, errors.Errorf("querying application %q: %w", appID, err)
	}

	source, err := decodeCharmSource(appOrigin.SourceID)
	if err != nil {
		return application.CharmOrigin{}, errors.Errorf("decoding charm source: %w", err)
	}

	platform, err := decodePlatform(appPlatformChan.PlatformChannel, appPlatformChan.PlatformOSID, appPlatformChan.PlatformArchitectureID)
	if err != nil {
		return application.CharmOrigin{}, errors.Errorf("decoding platform: %w", err)
	}

	channel, err := decodeChannel(appPlatformChan.ChannelTrack, appPlatformChan.ChannelRisk, appPlatformChan.ChannelBranch)
	if err != nil {
		return application.CharmOrigin{}, errors.Errorf("decoding channel: %w", err)
	}

	var revision = -1
	if appOrigin.Revision.Valid {
		revision = int(appOrigin.Revision.V)
	}

	var hash string
	if appOrigin.Hash.Valid {
		hash = appOrigin.Hash.String
	}

	var charmhubIdentifier string
	if appOrigin.CharmhubIdentifier.Valid {
		charmhubIdentifier = appOrigin.CharmhubIdentifier.String
	}

	return application.CharmOrigin{
		Name:               appOrigin.ReferenceName,
		Source:             source,
		Platform:           platform,
		Channel:            channel,
		Revision:           revision,
		Hash:               hash,
		CharmhubIdentifier: charmhubIdentifier,
	}, nil
}

// GetApplicationConstraints returns the application constraints for the
// specified application ID.
// Empty constraints are returned if no constraints exist for the given
// application ID.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (st *State) GetApplicationConstraints(ctx context.Context, appID coreapplication.ID) (constraints.Constraints, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return constraints.Constraints{}, errors.Capture(err)
	}

	ident := applicationID{ID: appID}

	query := `
SELECT &applicationConstraint.*
FROM v_application_constraint
WHERE application_uuid = $applicationID.uuid;
`

	stmt, err := st.Prepare(query, applicationConstraint{}, ident)
	if err != nil {
		return constraints.Constraints{}, errors.Errorf("preparing query for application constraints: %w", err)
	}

	var result applicationConstraints
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkApplicationNotDead(ctx, tx, appID); err != nil {
			return errors.Capture(err)
		}

		err := tx.Query(ctx, stmt, ident).GetAll(&result)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}

		return nil
	}); err != nil {
		return constraints.Constraints{}, errors.Errorf("querying application constraints for application %q: %w", appID, err)
	}

	return decodeConstraints(result), nil
}

// SetApplicationConstraints sets the application constraints for the
// specified application ID.
// This method overwrites the full constraints on every call.
// If invalid constraints are provided (e.g. invalid container type or
// non-existing space), a [applicationerrors.InvalidApplicationConstraints]
// error is returned.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (st *State) SetApplicationConstraints(ctx context.Context, appID coreapplication.ID, cons constraints.Constraints) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	cUUID, err := uuid.NewUUID()
	if err != nil {
		return errors.Capture(err)
	}
	cUUIDStr := cUUID.String()

	selectConstraintUUIDQuery := `
SELECT &constraintUUID.*
FROM application_constraint
WHERE application_uuid = $applicationUUID.application_uuid
`
	selectConstraintUUIDStmt, err := st.Prepare(selectConstraintUUIDQuery, constraintUUID{}, applicationUUID{})
	if err != nil {
		return errors.Errorf("preparing select application constraint uuid query: %w", err)
	}

	// Check that spaces provided as constraints do exist in the space table.
	selectSpaceQuery := `SELECT &spaceUUID.uuid FROM space WHERE name = $spaceName.name`
	selectSpaceStmt, err := st.Prepare(selectSpaceQuery, spaceUUID{}, spaceName{})
	if err != nil {
		return errors.Errorf("preparing select space query: %w", err)
	}

	// Cleanup all previous tags, spaces and zones from their join tables.
	deleteConstraintTagsQuery := `DELETE FROM constraint_tag WHERE constraint_uuid = $constraintUUID.constraint_uuid`
	deleteConstraintTagsStmt, err := st.Prepare(deleteConstraintTagsQuery, constraintUUID{})
	if err != nil {
		return errors.Errorf("preparing delete constraint tags query: %w", err)
	}
	deleteConstraintSpacesQuery := `DELETE FROM constraint_space WHERE constraint_uuid = $constraintUUID.constraint_uuid`
	deleteConstraintSpacesStmt, err := st.Prepare(deleteConstraintSpacesQuery, constraintUUID{})
	if err != nil {
		return errors.Errorf("preparing delete constraint spaces query: %w", err)
	}
	deleteConstraintZonesQuery := `DELETE FROM constraint_zone WHERE constraint_uuid = $constraintUUID.constraint_uuid`
	deleteConstraintZonesStmt, err := st.Prepare(deleteConstraintZonesQuery, constraintUUID{})
	if err != nil {
		return errors.Errorf("preparing delete constraint zones query: %w", err)
	}

	selectContainerTypeIDQuery := `SELECT &containerTypeID.id FROM container_type WHERE value = $containerTypeVal.value`
	selectContainerTypeIDStmt, err := st.Prepare(selectContainerTypeIDQuery, containerTypeID{}, containerTypeVal{})
	if err != nil {
		return errors.Errorf("preparing select container type id query: %w", err)
	}

	insertConstraintsQuery := `
INSERT INTO "constraint"(*)
VALUES ($setConstraint.*)
ON CONFLICT (uuid) DO UPDATE SET
    arch = excluded.arch,
    cpu_cores = excluded.cpu_cores,
    cpu_power = excluded.cpu_power,
    mem = excluded.mem,
    root_disk= excluded.root_disk,
    root_disk_source = excluded.root_disk_source,
    instance_role = excluded.instance_role,
    instance_type = excluded.instance_type,
    container_type_id = excluded.container_type_id,
    virt_type = excluded.virt_type,
    allocate_public_ip = excluded.allocate_public_ip,
    image_id = excluded.image_id
`
	insertConstraintsStmt, err := st.Prepare(insertConstraintsQuery, setConstraint{})
	if err != nil {
		return errors.Errorf("preparing insert constraints query: %w", err)
	}

	insertConstraintTagsQuery := `INSERT INTO constraint_tag(*) VALUES ($setConstraintTag.*)`
	insertConstraintTagsStmt, err := st.Prepare(insertConstraintTagsQuery, setConstraintTag{})
	if err != nil {
		return errors.Errorf("preparing insert constraint tags query: %w", err)
	}

	insertConstraintSpacesQuery := `INSERT INTO constraint_space(*) VALUES ($setConstraintSpace.*)`
	insertConstraintSpacesStmt, err := st.Prepare(insertConstraintSpacesQuery, setConstraintSpace{})
	if err != nil {
		return errors.Capture(err)
	}

	insertConstraintZonesQuery := `INSERT INTO constraint_zone(*) VALUES ($setConstraintZone.*)`
	insertConstraintZonesStmt, err := st.Prepare(insertConstraintZonesQuery, setConstraintZone{})
	if err != nil {
		return errors.Capture(err)
	}

	insertAppConstraintsQuery := `
INSERT INTO application_constraint(*)
VALUES ($setApplicationConstraint.*)
ON CONFLICT (application_uuid) DO NOTHING
`
	insertAppConstraintsStmt, err := st.Prepare(insertAppConstraintsQuery, setApplicationConstraint{})
	if err != nil {
		return errors.Errorf("preparing insert application constraints query: %w", err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkApplicationNotDead(ctx, tx, appID); err != nil {
			return errors.Capture(err)
		}

		var containerTypeID containerTypeID
		if cons.Container != nil {
			err = tx.Query(ctx, selectContainerTypeIDStmt, containerTypeVal{Value: string(*cons.Container)}).Get(&containerTypeID)
			if errors.Is(err, sqlair.ErrNoRows) {
				st.logger.Warningf(ctx, "cannot set constraints, container type %q does not exist", *cons.Container)
				return applicationerrors.InvalidApplicationConstraints
			}
			if err != nil {
				return errors.Capture(err)
			}
		}

		// First check if the constraint already exists, in that case
		// we need to update it, unsetting the nil values.
		var retrievedConstraintUUID constraintUUID
		err := tx.Query(ctx, selectConstraintUUIDStmt, applicationUUID{ApplicationUUID: appID.String()}).Get(&retrievedConstraintUUID)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		} else if err == nil {
			cUUIDStr = retrievedConstraintUUID.ConstraintUUID
		}

		// Cleanup tags, spaces and zones from their join tables.
		if err := tx.Query(ctx, deleteConstraintTagsStmt, constraintUUID{ConstraintUUID: cUUIDStr}).Run(); err != nil {
			return errors.Capture(err)
		}
		if err := tx.Query(ctx, deleteConstraintSpacesStmt, constraintUUID{ConstraintUUID: cUUIDStr}).Run(); err != nil {
			return errors.Capture(err)
		}
		if err := tx.Query(ctx, deleteConstraintZonesStmt, constraintUUID{ConstraintUUID: cUUIDStr}).Run(); err != nil {
			return errors.Capture(err)
		}

		constraints := encodeConstraints(cUUIDStr, cons, containerTypeID.ID)

		if err := tx.Query(ctx, insertConstraintsStmt, constraints).Run(); err != nil {
			return errors.Capture(err)
		}

		if cons.Tags != nil {
			for _, tag := range *cons.Tags {
				constraintTag := setConstraintTag{ConstraintUUID: cUUIDStr, Tag: tag}
				if err := tx.Query(ctx, insertConstraintTagsStmt, constraintTag).Run(); err != nil {
					return errors.Capture(err)
				}
			}
		}

		if cons.Spaces != nil {
			for _, space := range *cons.Spaces {
				// Make sure the space actually exists.
				var spaceUUID spaceUUID
				err := tx.Query(ctx, selectSpaceStmt, spaceName{Name: space.SpaceName}).Get(&spaceUUID)
				if errors.Is(err, sqlair.ErrNoRows) {
					st.logger.Warningf(ctx, "cannot set constraints, space %q does not exist", space)
					return applicationerrors.InvalidApplicationConstraints
				}
				if err != nil {
					return errors.Capture(err)
				}

				constraintSpace := setConstraintSpace{ConstraintUUID: cUUIDStr, Space: space.SpaceName, Exclude: space.Exclude}
				if err := tx.Query(ctx, insertConstraintSpacesStmt, constraintSpace).Run(); err != nil {
					return errors.Capture(err)
				}
			}
		}

		if cons.Zones != nil {
			for _, zone := range *cons.Zones {
				constraintZone := setConstraintZone{ConstraintUUID: cUUIDStr, Zone: zone}
				if err := tx.Query(ctx, insertConstraintZonesStmt, constraintZone).Run(); err != nil {
					return errors.Capture(err)
				}
			}
		}

		return errors.Capture(
			tx.Query(ctx, insertAppConstraintsStmt, setApplicationConstraint{
				ApplicationUUID: appID.String(),
				ConstraintUUID:  cUUIDStr,
			}).Run(),
		)
	})
}

// GetDeviceConstraints returns the device constraints for an application.
//
// If the application is dead, [applicationerrors.ApplicationIsDead] is returned.
// If the application is not found, [applicationerrors.ApplicationNotFound]
// is returned.
func (st *State) GetDeviceConstraints(ctx context.Context, appID coreapplication.ID) (map[string]devices.Constraints, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	ident := applicationID{ID: appID}

	query := `
SELECT &deviceConstraint.*
FROM device_constraint AS dc
LEFT JOIN device_constraint_attribute AS dca ON dca.device_constraint_uuid = dc.uuid
WHERE dc.application_uuid = $applicationID.uuid;
`

	stmt, err := st.Prepare(query, deviceConstraint{}, ident)
	if err != nil {
		return nil, errors.Errorf("preparing query for application constraints: %w", err)
	}

	var result []deviceConstraint
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkApplicationNotDead(ctx, tx, appID); err != nil {
			return errors.Capture(err)
		}

		err := tx.Query(ctx, stmt, ident).GetAll(&result)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}

		return nil
	}); err != nil {
		return nil, errors.Capture(err)
	}

	return st.decodeDeviceConstraints(result), nil
}

func (st *State) decodeDeviceConstraints(cons []deviceConstraint) map[string]devices.Constraints {
	res := make(map[string]devices.Constraints)
	if len(cons) == 0 {
		return res
	}
	for _, row := range cons {
		if _, ok := res[row.Name]; !ok {
			res[row.Name] = devices.Constraints{
				Type:       devices.DeviceType(row.Type),
				Count:      row.Count,
				Attributes: make(map[string]string),
			}
		}
		if row.AttributeKey.Valid {
			res[row.Name].Attributes[row.AttributeKey.String] = row.AttributeValue.String
		}
	}
	return res
}

func (st *State) insertDeviceConstraints(ctx context.Context, tx *sqlair.TX, appID coreapplication.ID, cons map[string]devices.Constraints) error {
	if len(cons) == 0 {
		return nil
	}
	setDeviceConstraints := make([]setDeviceConstraint, 0, len(cons))
	setDeviceConstraintAttributes := make([]setDeviceConstraintAttribute, 0)
	for name, deviceCons := range cons {
		uuid, err := uuid.NewUUID()
		if err != nil {
			return errors.Capture(err)
		}
		setDeviceConstraints = append(setDeviceConstraints, setDeviceConstraint{
			UUID:            uuid.String(),
			ApplicationUUID: appID.String(),
			Name:            name,
			Count:           deviceCons.Count,
			Type:            string(deviceCons.Type),
		})
		for k, v := range deviceCons.Attributes {
			setDeviceConstraintAttributes = append(setDeviceConstraintAttributes, setDeviceConstraintAttribute{
				DeviceConstraintUUID: uuid.String(),
				AttributeKey:         k,
				AttributeValue:       v,
			})
		}
	}

	insertDeviceConstraintQuery := `
INSERT INTO device_constraint (*)
VALUES ($setDeviceConstraint.*)
`
	insertDeviceConstraintStmt, err := st.Prepare(insertDeviceConstraintQuery, setDeviceConstraint{})
	if err != nil {
		return errors.Errorf("preparing insert device constraints query: %w", err)
	}
	err = tx.Query(ctx, insertDeviceConstraintStmt, setDeviceConstraints).Run()
	if err != nil {
		return errors.Errorf("inserting device constraints: %w", err)
	}

	insertDeviceConstraintAttributesQuery := `
INSERT INTO device_constraint_attribute (*)
VALUES ($setDeviceConstraintAttribute.*)
`
	insertDeviceConstraintAttributesStmt, err := st.Prepare(insertDeviceConstraintAttributesQuery, setDeviceConstraintAttribute{})
	if err != nil {
		return errors.Errorf("preparing insert device constraint attributes query: %w", err)
	}

	err = tx.Query(ctx, insertDeviceConstraintAttributesStmt, setDeviceConstraintAttributes).Run()
	if err != nil {
		return errors.Errorf("inserting device constraint attributes: %w", err)
	}
	return nil
}

// NamespaceForWatchApplication returns the namespace identifier
// for application change watchers.
func (*State) NamespaceForWatchApplication() string {
	return "application"
}

// NamespaceForWatchApplicationConfig returns a namespace string identifier
// for application configuration changes.
func (*State) NamespaceForWatchApplicationConfig() string {
	return "application_config_hash"
}

// NamesapceForWatchApplicationSetting returns the namespace string identifier
// for application setting changes.
func (*State) NamespaceForWatchApplicationSetting() string {
	return "application_setting"
}

// NamespaceForWatchApplicationScale returns the namespace identifier
// for application scale change watchers.
func (*State) NamespaceForWatchApplicationScale() string {
	return "application_scale"
}

// NamespaceForWatchApplicationExposed returns the namespace identifier
// for application exposed endpoints changes. The first return value is the
// namespace for the application exposed endpoints to spaces table, and the
// second is the namespace for the application exposed endpoints to CIDRs
// table.
func (*State) NamespaceForWatchApplicationExposed() (string, string) {
	return "application_exposed_endpoint_space", "application_exposed_endpoint_cidr"
}

// NamespaceForWatchUnitForLegacyUniter returns the namespace identifiers
// for unit changes needed for the uniter. The first return value is the
// namespace for the unit's inherent properties, the second is the namespace
// of unit principals (used to watch for changes in subordinates), and the
// third is the namespace for the unit's resolved mode.
func (*State) NamespaceForWatchUnitForLegacyUniter() (string, string, string) {
	return "unit", "unit_principal", "unit_resolved"
}

// NamespaceForWatchNetNodeAddress returns the namespace identifier for
// net node address changes, which is the ip_address table.
func (*State) NamespaceForWatchNetNodeAddress() string {
	return "ip_address"
}

// decodeConstraints flattens and maps the list of rows of applicatioConstraint
// to get a single constraints.Constraints. The flattening is needed because of the
// spaces, tags and zones constraints which are slices. We can safely assume
// that the non-slice values are repeated on every row so we can safely
// overwrite the previous value on each iteration.
func decodeConstraints(cons applicationConstraints) constraints.Constraints {
	var res constraints.Constraints

	// Empty constraints is not an error case, so return early the empty
	// result.
	if len(cons) == 0 {
		return res
	}

	// Unique spaces, tags and zones:
	spaces := make(map[string]constraints.SpaceConstraint)
	tags := set.NewStrings()
	zones := set.NewStrings()

	for _, row := range cons {
		if row.Arch.Valid {
			res.Arch = &row.Arch.String
		}
		if row.CPUCores.Valid {
			cpuCores := uint64(row.CPUCores.V)
			res.CpuCores = &cpuCores
		}
		if row.CPUPower.Valid {
			cpuPower := uint64(row.CPUPower.V)
			res.CpuPower = &cpuPower
		}
		if row.Mem.Valid {
			mem := uint64(row.Mem.V)
			res.Mem = &mem
		}
		if row.RootDisk.Valid {
			rootDisk := uint64(row.RootDisk.V)
			res.RootDisk = &rootDisk
		}
		if row.RootDiskSource.Valid {
			res.RootDiskSource = &row.RootDiskSource.String
		}
		if row.InstanceRole.Valid {
			res.InstanceRole = &row.InstanceRole.String
		}
		if row.InstanceType.Valid {
			res.InstanceType = &row.InstanceType.String
		}
		if row.ContainerType.Valid {
			containerType := instance.ContainerType(row.ContainerType.String)
			res.Container = &containerType
		}
		if row.VirtType.Valid {
			res.VirtType = &row.VirtType.String
		}
		if row.AllocatePublicIP.Valid {
			res.AllocatePublicIP = &row.AllocatePublicIP.Bool
		}
		if row.ImageID.Valid {
			res.ImageID = &row.ImageID.String
		}
		if row.SpaceName.Valid {
			var exclude bool
			if row.SpaceExclude.Valid {
				exclude = row.SpaceExclude.Bool
			}
			spaces[row.SpaceName.String] = constraints.SpaceConstraint{
				SpaceName: row.SpaceName.String,
				Exclude:   exclude,
			}
		}
		if row.Tag.Valid {
			tags.Add(row.Tag.String)
		}
		if row.Zone.Valid {
			zones.Add(row.Zone.String)
		}
	}

	// Add the unique spaces, tags and zones to the result:
	if len(spaces) > 0 {
		res.Spaces = ptr(slices.Collect(maps.Values(spaces)))
	}
	if len(tags) > 0 {
		tagsSlice := tags.SortedValues()
		res.Tags = &tagsSlice
	}
	if len(zones) > 0 {
		zonesSlice := zones.SortedValues()
		res.Zones = &zonesSlice
	}

	return res
}

// encodeConstraints maps the constraints.Constraints to a constraint struct, which
// does not contain the spaces, tags and zones constraints.
func encodeConstraints(constraintUUID string, cons constraints.Constraints, containerTypeID uint64) setConstraint {
	res := setConstraint{
		UUID:             constraintUUID,
		Arch:             cons.Arch,
		CPUCores:         cons.CpuCores,
		CPUPower:         cons.CpuPower,
		Mem:              cons.Mem,
		RootDisk:         cons.RootDisk,
		RootDiskSource:   cons.RootDiskSource,
		InstanceRole:     cons.InstanceRole,
		InstanceType:     cons.InstanceType,
		VirtType:         cons.VirtType,
		ImageID:          cons.ImageID,
		AllocatePublicIP: cons.AllocatePublicIP,
	}
	if cons.Container != nil {
		res.ContainerTypeID = &containerTypeID
	}
	return res
}

// lookupApplication looks up the application by name and returns the
// application.ID.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (st *State) lookupApplication(ctx context.Context, tx *sqlair.TX, name string) (coreapplication.ID, error) {
	app := applicationIDAndName{Name: name}
	queryApplicationStmt, err := st.Prepare(`
SELECT uuid AS &applicationIDAndName.uuid
FROM application
WHERE name = $applicationIDAndName.name
`, app)
	if err != nil {
		return "", errors.Capture(err)
	}
	err = tx.Query(ctx, queryApplicationStmt, app).Get(&app)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("%w: %s", applicationerrors.ApplicationNotFound, name)
	} else if err != nil {
		return "", errors.Errorf("looking up UUID for application %q: %w", name, err)
	}
	return app.ID, nil
}

func (st *State) checkApplicationCharm(ctx context.Context, tx *sqlair.TX, ident applicationID, charmID charmID) error {
	query := `
SELECT COUNT(*) AS &countResult.count
FROM application
WHERE uuid = $applicationID.uuid
AND charm_uuid = $charmID.uuid;
	`
	stmt, err := st.Prepare(query, countResult{}, ident, charmID)
	if err != nil {
		return errors.Errorf("preparing verification query: %w", err)
	}

	// Ensure that the charm is the same as the one we're trying to set.
	var count countResult
	if err := tx.Query(ctx, stmt, ident, charmID).Get(&count); err != nil {
		return errors.Errorf("verifying charm: %w", err)
	}
	if count.Count == 0 {
		return applicationerrors.ApplicationHasDifferentCharm
	}
	return nil
}

func (st *State) getApplicationConfigWithDefaults(ctx context.Context, tx *sqlair.TX, appID applicationID) ([]applicationConfig, error) {
	configStmt, err := st.Prepare(`
SELECT
	cc.key AS &applicationConfig.key,
	COALESCE(ac.value, cc.default_value) AS &applicationConfig.value,
	cct.name AS &applicationConfig.type
FROM application AS a
JOIN charm_config AS cc ON a.charm_uuid = cc.charm_uuid
JOIN charm_config_type AS cct ON cc.type_id = cct.id
LEFT JOIN application_config AS ac ON cc.key = ac.key AND cc.type_id = ac.type_id
WHERE a.uuid = $applicationID.uuid;
`, applicationConfig{}, appID)
	if err != nil {
		return nil, errors.Errorf("preparing query for application config: %w", err)
	}

	var results []applicationConfig
	if err := tx.Query(ctx, configStmt, appID).GetAll(&results); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("querying application config: %w", err)
	}
	return results, nil
}

func (st *State) getApplicationConfig(ctx context.Context, tx *sqlair.TX, appID applicationID) ([]applicationConfig, error) {
	configQuery := `
SELECT &applicationConfig.*
FROM v_application_config
WHERE uuid = $applicationID.uuid;
`
	configStmt, err := st.Prepare(configQuery, applicationConfig{}, appID)
	if err != nil {
		return nil, errors.Errorf("preparing query for application config: %w", err)
	}

	results := []applicationConfig{}
	if err := tx.Query(ctx, configStmt, appID).GetAll(&results); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("querying application config: %w", err)
	}
	return results, nil
}

func (st *State) getApplicationSettings(ctx context.Context, tx *sqlair.TX, appID applicationID) (applicationSettings, error) {
	settingsQuery := `
SELECT &applicationSettings.*
FROM application_setting
WHERE application_uuid = $applicationID.uuid;
`
	settingsStmt, err := st.Prepare(settingsQuery, applicationSettings{}, appID)
	if err != nil {
		return applicationSettings{}, errors.Errorf("preparing query for application config: %w", err)
	}

	var result applicationSettings
	if err := tx.Query(ctx, settingsStmt, appID).Get(&result); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return applicationSettings{}, errors.Errorf("querying application settings: %w", err)
	}
	return result, nil
}

func (st *State) insertApplicationConfig(
	ctx context.Context,
	tx *sqlair.TX,
	appID coreapplication.ID,
	config map[string]application.ApplicationConfig,
) error {
	if len(config) == 0 {
		return nil
	}

	insertQuery := `
INSERT INTO application_config (*)
VALUES ($setApplicationConfig.*);
`
	insertStmt, err := st.Prepare(insertQuery, setApplicationConfig{})
	if err != nil {
		return errors.Errorf("preparing insert query: %w", err)
	}

	inserts := make([]setApplicationConfig, 0, len(config))
	for k, v := range config {
		typeID, err := encodeConfigType(v.Type)
		if err != nil {
			return errors.Errorf("encoding config type: %w", err)
		}

		inserts = append(inserts, setApplicationConfig{
			ApplicationUUID: appID,
			Key:             k,
			Value:           v.Value,
			TypeID:          typeID,
		})
	}

	if err := tx.Query(ctx, insertStmt, inserts).Run(); err != nil {
		return errors.Errorf("inserting config: %w", err)
	}

	return nil
}

func (st *State) insertApplicationSettings(
	ctx context.Context,
	tx *sqlair.TX,
	appID coreapplication.ID,
	settings application.ApplicationSettings,
) error {
	insertQuery := `
INSERT INTO application_setting (*)
VALUES ($setApplicationSettings.*);
`
	insertStmt, err := st.Prepare(insertQuery, setApplicationSettings{})
	if err != nil {
		return errors.Errorf("preparing insert query: %w", err)
	}

	if err := tx.Query(ctx, insertStmt, setApplicationSettings{
		ApplicationUUID: appID,
		Trust:           settings.Trust,
	}).Run(); err != nil {
		return errors.Errorf("inserting settings: %w", err)
	}

	return nil
}

func (st *State) insertApplicationStatus(
	ctx context.Context,
	tx *sqlair.TX,
	appID coreapplication.ID,
	sts *status.StatusInfo[status.WorkloadStatusType],
) error {
	if sts == nil {
		return nil
	}

	insertQuery := `
INSERT INTO application_status (*) VALUES ($applicationStatus.*);
`

	insertStmt, err := st.Prepare(insertQuery, applicationStatus{})
	if err != nil {
		return errors.Errorf("preparing insert query: %w", err)
	}

	statusID, err := status.EncodeWorkloadStatus(sts.Status)
	if err != nil {
		return errors.Errorf("encoding status: %w", err)
	}

	if err := tx.Query(ctx, insertStmt, applicationStatus{
		ApplicationUUID: appID.String(),
		StatusID:        statusID,
		Message:         sts.Message,
		Data:            sts.Data,
		UpdatedAt:       sts.Since,
	}).Run(); err != nil {
		return errors.Errorf("inserting status: %w", err)
	}
	return nil
}

func (st *State) updateConfigHash(ctx context.Context, tx *sqlair.TX, appID applicationID) error {
	setHashQuery := `
INSERT INTO application_config_hash (*)
VALUES ($applicationConfigHash.*)
ON CONFLICT (application_uuid) DO UPDATE SET
	sha256 = excluded.sha256
`
	setHashStmt, err := st.Prepare(setHashQuery, applicationConfigHash{})
	if err != nil {
		return errors.Errorf("preparing set hash query: %w", err)
	}

	config, err := st.getApplicationConfig(ctx, tx, appID)
	if err != nil {
		return errors.Errorf("getting application config: %w", err)
	}
	settings, err := st.getApplicationSettings(ctx, tx, appID)
	if err != nil {
		return errors.Errorf("getting application settings: %w", err)
	}

	hash, err := hashConfigAndSettings(config, settings)
	if err != nil {
		return errors.Errorf("hashing config and settings: %w", err)
	}

	if err := tx.Query(ctx, setHashStmt, applicationConfigHash{
		ApplicationUUID: appID.ID,
		SHA256:          hash,
	}).Run(); err != nil {
		return errors.Errorf("setting hash: %w", err)
	}

	return nil
}

func decodeRisk(risk string) (deployment.ChannelRisk, error) {
	switch risk {
	case "stable":
		return deployment.RiskStable, nil
	case "candidate":
		return deployment.RiskCandidate, nil
	case "beta":
		return deployment.RiskBeta, nil
	case "edge":
		return deployment.RiskEdge, nil
	default:
		return "", errors.Errorf("unknown risk %q", risk)
	}
}

func decodeOSType(osType sql.Null[int64]) (deployment.OSType, error) {
	if !osType.Valid {
		return 0, errors.Errorf("os type is null")
	}

	switch osType.V {
	case 0:
		return deployment.Ubuntu, nil
	default:
		return -1, errors.Errorf("unknown os type %v", osType)
	}
}

func hashConfigAndSettings(config []applicationConfig, settings applicationSettings) (string, error) {
	h := sha256.New()

	// Ensure we have a stable order for the keys.
	sort.Slice(config, func(i, j int) bool {
		return config[i].Key < config[j].Key
	})

	for _, c := range config {
		if _, err := h.Write([]byte(c.Key)); err != nil {
			return "", errors.Errorf("writing config key: %w", err)
		}
		if _, err := h.Write([]byte(fmt.Sprintf("%v", c.Value))); err != nil {
			return "", errors.Errorf("writing config value: %w", err)
		}
	}
	if _, err := h.Write([]byte(strconv.FormatBool(settings.Trust))); err != nil {
		return "", errors.Errorf("writing settings: %w", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func decodePlatform(channel string, os, arch sql.Null[int64]) (deployment.Platform, error) {
	osType, err := decodeOSType(os)
	if err != nil {
		return deployment.Platform{}, errors.Errorf("decoding os type: %w", err)
	}

	archType, err := decodeArchitecture(arch)
	if err != nil {
		return deployment.Platform{}, errors.Errorf("decoding architecture: %w", err)
	}

	return deployment.Platform{
		Channel:      channel,
		OSType:       osType,
		Architecture: archType,
	}, nil
}

func decodeChannel(track string, risk sql.NullString, branch string) (*deployment.Channel, error) {
	if !risk.Valid {
		return nil, nil
	}

	riskType, err := decodeRisk(risk.String)
	if err != nil {
		return nil, errors.Errorf("decoding risk: %w", err)
	}

	return &deployment.Channel{
		Track:  track,
		Risk:   riskType,
		Branch: branch,
	}, nil
}

// getAllNonPeerRelationInfo retrieves metadata for all non-peer relations of a
// given application.
// It returns a slice of relationInfo containing all relevant information from
// charm_relation and the count of applications linked through this relation to
// the current one.
func (st *State) getAllNonPeerRelationInfo(ctx context.Context, tx *sqlair.TX, id coreapplication.ID) ([]relationInfo, error) {
	type application dbUUID
	app := application{UUID: id.String()}

	stmt, err := st.Prepare(`
SELECT
    a.name AS &relationInfo.application_name,
    vcr.charm_uuid AS &relationInfo.charm_uuid,
    vcr.name AS &relationInfo.name,
    vcr.role AS &relationInfo.role,
    vcr.interface AS &relationInfo.interface,
    vcr.optional AS &relationInfo.optional,
    vcr.capacity AS &relationInfo.capacity,
    vcr.scope AS &relationInfo.scope,
    COUNT(DISTINCT re.relation_uuid) AS &relationInfo.count
FROM   v_charm_relation AS vcr
JOIN   application_endpoint AS ae ON vcr.uuid = ae.charm_relation_uuid
JOIN   relation_endpoint AS re ON ae.uuid = re.endpoint_uuid
JOIN   application AS a ON ae.application_uuid = a.uuid
WHERE  ae.application_uuid = $application.uuid
AND    vcr.role != 'peer'
GROUP BY a.name, vcr.charm_uuid, vcr.name, vcr.role, vcr.interface, vcr.optional, vcr.capacity, vcr.scope -- for count
`, app, relationInfo{})
	if err != nil {
		return nil, errors.Errorf("preparing query: %w", err)
	}

	var result []relationInfo
	if err := tx.Query(ctx, stmt, app).GetAll(&result); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return result, nil
		}
		return nil, errors.Errorf("failed to select non-peer relation info: %w", err)
	}

	return result, nil
}

// precheckUpgradeRelation validates if upgrading a charm won't break existing
// relations or exceed relation limits.
// It ensures that:
// - the new charm implements existing relation given as a argument,
// - the current count of established relations does not exceed
// the new charm limit for each specified relation.
func precheckUpgradeRelation(meta *internalcharm.Meta, relations []relationInfo) error {
	relSpec := meta.CombinedRelations()
	for _, rel := range relations {
		charmRel := decodeRelation(rel)
		if !charmRel.ImplementedBy(meta) {
			return errors.Errorf("would break relation %s:%s", rel.ApplicationName, rel.Name)
		}
		// The relation will always be found. If not, it would have caused
		// the previous check to fail.
		if spec := relSpec[rel.Name]; rel.Count > spec.Limit {
			return errors.Errorf("new charm version imposes a maximum relation limit of %d for %s:%s which cannot be"+
				" satisfied by the number of already established relations (%d)", spec.Limit, rel.ApplicationName,
				rel.Name, rel.Count)
		}
	}
	return nil
}

// decodeRelation transforms the relationInfo data into an [internalcharm.Relation]
// structure.
func decodeRelation(ri relationInfo) internalcharm.Relation {
	return internalcharm.Relation{
		Name:      ri.Name,
		Role:      internalcharm.RelationRole(ri.Role),
		Interface: ri.Interface,
		Optional:  ri.Optional,
		Limit:     ri.Capacity,
		Scope:     internalcharm.RelationScope(ri.Scope),
	}
}
