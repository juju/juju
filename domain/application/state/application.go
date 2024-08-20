// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"reflect"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	domainstorage "github.com/juju/juju/domain/storage"
	storagestate "github.com/juju/juju/domain/storage/state"
	uniterrors "github.com/juju/juju/domain/unit/errors"
	"github.com/juju/juju/internal/uuid"
)

// ApplicationState describes retrieval and persistence methods for storage.
type ApplicationState struct {
	*commonStateBase
	logger logger.Logger
}

// NewApplicationState returns a new state reference.
func NewApplicationState(factory database.TxnRunnerFactory, logger logger.Logger) *ApplicationState {
	return &ApplicationState{
		commonStateBase: &commonStateBase{
			StateBase: domain.NewStateBase(factory),
		},
		logger: logger,
	}
}

// CreateApplication creates an application, whilst inserting a charm into the
// database, returning an error satisfying [applicationerrors.ApplicationAlreadyExists]
// if the application already exists.
func (st *ApplicationState) CreateApplication(ctx context.Context, name string, app application.AddApplicationArg, units ...application.UpsertUnitArg) (coreapplication.ID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	appID, err := coreapplication.NewID()
	if err != nil {
		return "", errors.Trace(err)
	}

	charmID, err := corecharm.NewID()
	if err != nil {
		return "", errors.Trace(err)
	}

	appDetails := applicationDetails{
		ApplicationID: appID.String(),
		Name:          name,
		CharmID:       charmID.String(),
		LifeID:        life.Alive,
	}
	createApplication := `
INSERT INTO application (*) VALUES ($applicationDetails.*)
`
	createApplicationStmt, err := st.Prepare(createApplication, appDetails)
	if err != nil {
		return "", errors.Trace(err)
	}

	platformInfo := applicationPlatform{
		ApplicationID:  appID.String(),
		OSTypeID:       app.Platform.OSTypeID,
		Channel:        app.Platform.Channel,
		ArchitectureID: app.Platform.ArchitectureID,
	}
	createPlatform := `INSERT INTO application_platform (*) VALUES ($applicationPlatform.*)`
	createPlatformStmt, err := st.Prepare(createPlatform, platformInfo)
	if err != nil {
		return "", errors.Trace(err)
	}

	scaleInfo := applicationScale{
		ApplicationID: appID.String(),
		Scale:         len(units),
	}
	createScale := `INSERT INTO application_scale (*) VALUES ($applicationScale.*)`
	createScaleStmt, err := st.Prepare(createScale, scaleInfo)
	if err != nil {
		return "", errors.Trace(err)
	}

	var (
		createChannelStmt *sqlair.Statement
		channelInfo       applicationChannel
	)
	if app.Channel != nil {
		channelInfo = applicationChannel{
			ApplicationID: appID.String(),
			Track:         app.Channel.Track,
			Risk:          string(app.Channel.Risk),
			Branch:        app.Channel.Branch,
		}
		createChannel := `INSERT INTO application_channel (*) VALUES ($applicationChannel.*)`
		if createChannelStmt, err = st.Prepare(createChannel, channelInfo); err != nil {
			return "", errors.Trace(err)
		}
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check if the application already exists.
		if err := st.checkApplicationExists(ctx, tx, name); err != nil {
			return fmt.Errorf("checking if application %q exists: %w", name, err)
		}

		// Insert the charm.
		if err := st.setCharm(ctx, tx, charmID, app.Charm, ""); err != nil {
			return errors.Annotate(err, "setting charm")
		}

		// If the application doesn't exist, create it.
		err := tx.Query(ctx, createApplicationStmt, appDetails).Run()
		if err != nil {
			return errors.Annotatef(err, "creating row for application %q", name)
		}
		if err := tx.Query(ctx, createPlatformStmt, platformInfo).Run(); err != nil {
			return errors.Annotatef(err, "creating platform row for application %q", name)
		}
		if err := tx.Query(ctx, createScaleStmt, scaleInfo).Run(); err != nil {
			return errors.Annotatef(err, "creating scale row for application %q", name)
		}
		if createChannelStmt != nil {
			if err := tx.Query(ctx, createChannelStmt, channelInfo).Run(); err != nil {
				return errors.Annotatef(err, "creating channel row for application %q", name)
			}
		}

		if len(units) == 0 {
			return nil
		}

		for _, u := range units {
			if err := st.insertUnit(ctx, tx, appID, u); err != nil {
				return fmt.Errorf("adding unit for application %q: %w", name, err)
			}
		}

		return nil
	})
	return appID, errors.Annotatef(err, "creating application %q", name)
}

func (st *ApplicationState) checkApplicationExists(ctx context.Context, tx *sqlair.TX, name string) error {
	var appID applicationID
	appName := applicationName{Name: name}
	query := `
SELECT application.uuid AS &applicationID.*
FROM application
WHERE name = $applicationName.name
`
	existsQueryStmt, err := st.Prepare(query, appID, appName)
	if err != nil {
		return errors.Trace(err)
	}

	if err := tx.Query(ctx, existsQueryStmt, appName).Get(&appID); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("checking if application %q exists: %w", name, err)
	}
	return applicationerrors.ApplicationAlreadyExists
}

func (st *ApplicationState) lookupApplication(ctx context.Context, tx *sqlair.TX, name string, deadOk bool) (coreapplication.ID, error) {
	var appID applicationID
	appName := applicationName{Name: name}
	queryApplication := `
SELECT (uuid, life_id) AS (&applicationID.*)
FROM application
WHERE name = $applicationName.name
`
	queryApplicationStmt, err := st.Prepare(queryApplication, appID, appName)
	if err != nil {
		return "", errors.Trace(err)
	}
	err = tx.Query(ctx, queryApplicationStmt, appName).Get(&appID)
	if err != nil {
		if !errors.Is(err, sqlair.ErrNoRows) {
			return "", errors.Annotatef(err, "looking up UUID for application %q", name)
		}
		return "", fmt.Errorf("%w: %s", applicationerrors.ApplicationNotFound, name)
	}
	if !deadOk && appID.LifeID == life.Dead {
		return "", fmt.Errorf("%w: %s", applicationerrors.ApplicationIsDead, name)
	}
	return coreapplication.ID(appID.ID), nil
}

// DeleteApplication deletes the specified application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
// If the application still has units, as error satisfying [applicationerrors.ApplicationHasUnits]
// is returned.
func (st *ApplicationState) DeleteApplication(ctx context.Context, name string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	var appID applicationID
	queryUnits := `SELECT count(*) AS &M.count FROM unit WHERE application_uuid = $applicationID.uuid`
	queryUnitsStmt, err := st.Prepare(queryUnits, sqlair.M{}, appID)
	if err != nil {
		return errors.Trace(err)
	}

	appName := applicationName{Name: name}
	deleteApplication := `DELETE FROM application WHERE name = $applicationName.name`
	deleteApplicationStmt, err := st.Prepare(deleteApplication, appName)
	if err != nil {
		return errors.Trace(err)
	}
	deletePlatform := `DELETE FROM application_platform WHERE application_uuid = $applicationID.uuid`
	deletePlatformStmt, err := st.Prepare(deletePlatform, appID)
	if err != nil {
		return errors.Trace(err)
	}
	deleteScale := `DELETE FROM application_scale WHERE application_uuid = $applicationID.uuid`
	deleteScaleStmt, err := st.Prepare(deleteScale, appID)
	if err != nil {
		return errors.Trace(err)
	}
	deleteChannel := `DELETE FROM application_channel WHERE application_uuid = $applicationID.uuid`
	deleteChannelStmt, err := st.Prepare(deleteChannel, appID)
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		appUUID, err := st.lookupApplication(ctx, tx, name, true)
		if err != nil {
			return errors.Trace(err)
		}
		appID.ID = appUUID.String()

		// Check that there are no units.
		result := sqlair.M{}
		err = tx.Query(ctx, queryUnitsStmt, appID).Get(&result)
		if err != nil {
			if !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Annotatef(err, "querying units for application %q", name)
			}
		}
		numUnits, _ := result["count"].(int64)
		if numUnits > 0 {
			return fmt.Errorf("cannot delete application %q as it still has %d unit(s)%w", name, numUnits, errors.Hide(applicationerrors.ApplicationHasUnits))
		}

		if err := tx.Query(ctx, deletePlatformStmt, appID).Run(); err != nil {
			return errors.Annotatef(err, "deleting platform row for application %q", name)
		}
		if err := tx.Query(ctx, deleteScaleStmt, appID).Run(); err != nil {
			return errors.Annotatef(err, "deleting scale row for application %q", name)
		}
		if err := tx.Query(ctx, deleteChannelStmt, appID).Run(); err != nil {
			return errors.Annotatef(err, "deleting channel row for application %q", name)
		}
		if err := tx.Query(ctx, deleteApplicationStmt, appName).Run(); err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	return errors.Annotatef(err, "deleting application %q", name)
}

// AddUnits adds the specified units to the application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
func (st *ApplicationState) AddUnits(ctx context.Context, applicationName string, args ...application.UpsertUnitArg) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		appID, err := st.lookupApplication(ctx, tx, applicationName, false)
		if err != nil {
			return errors.Trace(err)
		}
		for _, arg := range args {
			if err := st.insertUnit(ctx, tx, appID, arg); err != nil {
				return fmt.Errorf("adding unit for application %q: %w", applicationName, err)
			}
		}
		return nil
	})
	return errors.Annotatef(err, "adding units for application %q", applicationName)
}

func (st *ApplicationState) getUnit(ctx context.Context, tx *sqlair.TX, unitName string) (*unitDetails, error) {
	unit := unitDetails{Name: unitName}
	getUnit := `SELECT (*) AS (&unitDetails.*) FROM unit WHERE name = $unitDetails.name`
	getUnitStmt, err := st.Prepare(getUnit, unit)
	if err != nil {
		return nil, errors.Trace(err)
	}
	err = tx.Query(ctx, getUnitStmt, unit).Get(&unit)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, fmt.Errorf("unit %q not found%w", unitName, errors.Hide(uniterrors.NotFound))
	}
	if err != nil {
		return nil, fmt.Errorf("querying unit %q: %w", unitName, err)
	}
	return &unit, nil
}

func (st *ApplicationState) insertUnit(
	ctx context.Context, tx *sqlair.TX, appID coreapplication.ID, args application.UpsertUnitArg,
) error {
	unitUUID, err := coreunit.NewID()
	if err != nil {
		return errors.Trace(err)
	}
	nodeUUID, err := uuid.NewUUID()
	if err != nil {
		return errors.Trace(err)
	}
	createParams := unitDetails{
		ApplicationID: appID.String(),
		UnitID:        unitUUID.String(),
		NetNodeID:     nodeUUID.String(),
		LifeID:        life.Alive,
	}
	if args.PasswordHash != nil {
		createParams.PasswordHash = *args.PasswordHash
		createParams.PasswordHashAlgorithmID = 0 //currently we only use sha256
	}

	createUnit := `INSERT INTO unit (*) VALUES ($unitDetails.*)`
	createUnitStmt, err := st.Prepare(createUnit, createParams)
	if err != nil {
		return errors.Trace(err)
	}

	createNode := `INSERT INTO net_node (uuid) VALUES ($unitDetails.net_node_uuid)`
	createNodeStmt, err := st.Prepare(createNode, createParams)
	if err != nil {
		return errors.Trace(err)
	}

	// TODO - we are mirroring what's in mongo, hence the unit name is known.
	// In future we'll need to use a sequence to get a new unit id.
	if args.UnitName == nil {
		return fmt.Errorf("must pass unit name when adding a new unit for application %q", appID)
	}
	unitName := *args.UnitName
	createParams.Name = unitName

	if err := tx.Query(ctx, createNodeStmt, createParams).Run(); err != nil {
		return errors.Annotatef(err, "creating net node row for unit %q", unitName)
	}
	if err := tx.Query(ctx, createUnitStmt, createParams).Run(); err != nil {
		return errors.Annotatef(err, "creating unit row for unit %q", unitName)
	}
	if args.CloudContainer != nil {
		if err := st.upsertUnitCloudContainer(ctx, tx, unitName, nodeUUID.String(), args.CloudContainer); err != nil {
			return errors.Annotatef(err, "creating cloud container row for unit %q", unitName)
		}
	}
	return nil
}

func (st *ApplicationState) upsertUnit(
	ctx context.Context, tx *sqlair.TX, toUpdate unitDetails, args application.UpsertUnitArg,
) error {
	if args.PasswordHash != nil {
		toUpdate.PasswordHash = *args.PasswordHash
		toUpdate.PasswordHashAlgorithmID = 0 //currently we only use sha256
	}

	updateUnit := `
UPDATE unit SET
    life_id = $unitDetails.life_id,
    password_hash = $unitDetails.password_hash,
    password_hash_algorithm_id = $unitDetails.password_hash_algorithm_id
WHERE uuid = $unitDetails.uuid
`
	updateUnitStmt, err := st.Prepare(updateUnit, toUpdate)
	if err != nil {
		return errors.Trace(err)
	}

	if err := tx.Query(ctx, updateUnitStmt, toUpdate).Run(); err != nil {
		return errors.Annotatef(err, "updating unit row for unit %q", toUpdate.Name)
	}
	if args.CloudContainer != nil {
		if err := st.upsertUnitCloudContainer(ctx, tx, toUpdate.Name, toUpdate.NetNodeID, args.CloudContainer); err != nil {
			return errors.Annotatef(err, "creating cloud container row for unit %q", toUpdate.Name)
		}
	}
	return nil
}

func (st *ApplicationState) upsertUnitCloudContainer(
	ctx context.Context, tx *sqlair.TX, unitName, netNodeID string, cc *application.CloudContainer,
) error {
	existingContainerInfo := cloudContainer{
		NetNodeID: netNodeID,
	}
	queryCloudContainer := `
SELECT (*) AS (&cloudContainer.*)
FROM cloud_container
WHERE net_node_uuid = $cloudContainer.net_node_uuid
`
	queryStmt, err := st.Prepare(queryCloudContainer, existingContainerInfo)
	if err != nil {
		return errors.Trace(err)
	}

	err = tx.Query(ctx, queryStmt, existingContainerInfo).Get(&existingContainerInfo)
	if err != nil {
		if !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(err, "looking up cloud container %q", unitName)
		}
	}

	var newProviderId string
	if cc.ProviderId != nil {
		newProviderId = *cc.ProviderId
	}
	if existingContainerInfo.ProviderID != "" &&
		newProviderId != "" &&
		existingContainerInfo.ProviderID != newProviderId {
		st.logger.Debugf("unit %q has provider id %q which changed to %q",
			unitName, existingContainerInfo.ProviderID, newProviderId)
	}

	newContainerInfo := cloudContainer{
		NetNodeID: netNodeID,
	}
	if newProviderId != "" {
		newContainerInfo.ProviderID = newProviderId
	}
	if cc.Address != nil {
		// TODO(units) - handle addresses
	}
	if cc.Ports != nil {
		// TODO(units) - handle ports
	}
	// Currently, we only update container attributes but that might change.
	if reflect.DeepEqual(newContainerInfo, existingContainerInfo) {
		return nil
	}

	upsertCloudContainer := `
INSERT INTO cloud_container (*) VALUES ($cloudContainer.*)
ON CONFLICT(net_node_uuid) DO UPDATE
    SET provider_id = excluded.provider_id;
`

	upsertStmt, err := st.Prepare(upsertCloudContainer, newContainerInfo)
	if err != nil {
		return errors.Trace(err)
	}

	err = tx.Query(ctx, upsertStmt, newContainerInfo).Run()
	return errors.Annotatef(err, "updating cloud container for unit %q", unitName)
}

// StorageDefaults returns the default storage sources for a model.
func (st *ApplicationState) StorageDefaults(ctx context.Context) (domainstorage.StorageDefaults, error) {
	rval := domainstorage.StorageDefaults{}

	db, err := st.DB()
	if err != nil {
		return rval, errors.Trace(err)
	}

	attrs := []string{application.StorageDefaultBlockSourceKey, application.StorageDefaultFilesystemSourceKey}
	attrsSlice := sqlair.S(transform.Slice(attrs, func(s string) any { return any(s) }))
	stmt, err := st.Prepare(`
SELECT &KeyValue.* FROM model_config WHERE key IN ($S[:])
`, sqlair.S{}, KeyValue{})
	if err != nil {
		return rval, errors.Trace(err)
	}

	return rval, db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var values []KeyValue
		err := tx.Query(ctx, stmt, attrsSlice).GetAll(&values)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("getting model config attrs for storage defaults: %w", err)
		}

		for _, kv := range values {
			switch k := kv.Key; k {
			case application.StorageDefaultBlockSourceKey:
				v := fmt.Sprint(kv.Value)
				rval.DefaultBlockSource = &v
			case application.StorageDefaultFilesystemSourceKey:
				v := fmt.Sprint(kv.Value)
				rval.DefaultFilesystemSource = &v
			}
		}
		return nil
	})
}

// GetStoragePoolByName returns the storage pool with the specified name, returning an error
// satisfying [storageerrors.PoolNotFoundError] if it doesn't exist.
func (st *ApplicationState) GetStoragePoolByName(ctx context.Context, name string) (domainstorage.StoragePoolDetails, error) {
	db, err := st.DB()
	if err != nil {
		return domainstorage.StoragePoolDetails{}, errors.Trace(err)
	}
	return storagestate.GetStoragePoolByName(ctx, db, name)
}

// GetApplicationID returns the ID for the named application, returning an error
// satisfying [applicationerrors.ApplicationNotFound] if the application is not found.
func (st *ApplicationState) GetApplicationID(ctx domain.AtomicContext, name string) (coreapplication.ID, error) {
	var appID coreapplication.ID
	err := domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		appID, err = st.lookupApplication(ctx, tx, name, false)
		if err != nil {
			return fmt.Errorf("looking up application %q: %w", name, err)
		}
		return nil
	})
	return appID, errors.Annotatef(err, "getting ID for %q", name)
}

// UpsertUnit creates or updates the specified application unit, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
func (st *ApplicationState) UpsertUnit(
	ctx domain.AtomicContext, appID coreapplication.ID, args application.UpsertUnitArg,
) error {
	err := domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		unit, err := st.getUnit(ctx, tx, *args.UnitName)
		if err != nil {
			if errors.Is(err, uniterrors.NotFound) {
				return st.insertUnit(ctx, tx, appID, args)
			}
			return errors.Trace(err)
		}
		return st.upsertUnit(ctx, tx, *unit, args)
	})
	return errors.Annotatef(err, "upserting unit %q for application %q", *args.UnitName, appID)
}

// UnitLife looks up the life of the specified unit, returning an error
// satisfying [uniterrors.NotFound] if the unit is not found.
func (st *ApplicationState) UnitLife(ctx domain.AtomicContext, unitName string) (life.Life, error) {
	unit := unitDetails{Name: unitName}
	queryUnit := `
SELECT unit.life_id AS &unitDetails.*
FROM unit
WHERE name = $unitDetails.name
`
	queryUnitStmt, err := st.Prepare(queryUnit, unit)
	if err != nil {
		return -1, errors.Trace(err)
	}

	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, queryUnitStmt, unit).Get(&unit)
		if err != nil {
			if !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Annotatef(err, "querying unit %q life", unitName)
			}
			return fmt.Errorf("%w: %s", uniterrors.NotFound, unitName)
		}
		return nil
	})
	return unit.LifeID, errors.Annotatef(err, "querying unit %q life", unitName)
}

// ApplicationScaleState looks up the scale state of the specified application, returning an error
// satisfying [applicationerrors.ApplicationNotFound] if the application is not found.
func (st *ApplicationState) ApplicationScaleState(ctx domain.AtomicContext, appID coreapplication.ID) (application.ScaleState, error) {
	appScale := applicationScale{ApplicationID: appID.String()}
	queryScale := `
SELECT (*) AS (&applicationScale.*)
FROM application_scale
WHERE application_uuid = $applicationScale.application_uuid
`
	queryScaleStmt, err := st.Prepare(queryScale, appScale)
	if err != nil {
		return application.ScaleState{}, errors.Trace(err)
	}

	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, queryScaleStmt, appScale).Get(&appScale)
		if err != nil {
			if !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Annotatef(err, "querying application %q scale", appID)
			}
			return fmt.Errorf("%w: %s", applicationerrors.ApplicationNotFound, appID)
		}
		return nil
	})
	return appScale.ToScaleState(), errors.Annotatef(err, "querying application %q scale", appID)
}
