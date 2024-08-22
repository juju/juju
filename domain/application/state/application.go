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
	"github.com/juju/names/v5"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	domainstorage "github.com/juju/juju/domain/storage"
	storagestate "github.com/juju/juju/domain/storage/state"
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
	createApplication := `INSERT INTO application (*) VALUES ($applicationDetails.*)`
	createApplicationStmt, err := st.Prepare(createApplication, appDetails)
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

	platformInfo := applicationPlatform{
		ApplicationID:  appID.String(),
		OSTypeID:       int(app.Platform.OSType),
		Channel:        app.Platform.Channel,
		ArchitectureID: int(app.Platform.Architecture),
	}
	createPlatform := `INSERT INTO application_platform (*) VALUES ($applicationPlatform.*)`
	createPlatformStmt, err := st.Prepare(createPlatform, platformInfo)
	if err != nil {
		return "", errors.Trace(err)
	}

	originInfo := setCharmOrigin{
		CharmID: charmID.String(),
		// Set the Name on charm origin to the application name. This is
		// because the charm metadata.yaml can differ from the charm name
		// that was used to create the application.
		// This can happen if the charmhub charm name is different from the
		// charm name in the metadata.yaml. This isn't supposed to happen, but
		// it can.
		ReferenceName: app.Origin.ReferenceName,
		SourceID:      encodeCharmOriginSource(app.Origin.Source),
		Revision:      app.Origin.Revision,
	}
	createOrigin := `INSERT INTO charm_origin (*) VALUES ($setCharmOrigin.*)`
	createOriginStmt, err := st.Prepare(createOrigin, originInfo)
	if err != nil {
		return "", errors.Trace(err)
	}

	charmPlatformInfo := charmPlatform{
		CharmID:        charmID.String(),
		OSTypeID:       int(app.Platform.OSType),
		Channel:        app.Platform.Channel,
		ArchitectureID: int(app.Platform.Architecture),
	}
	createCharmPlatform := `INSERT INTO charm_platform (*) VALUES ($charmPlatform.*)`
	createCharmPlatformStmt, err := st.Prepare(createCharmPlatform, charmPlatformInfo)
	if err != nil {
		return "", errors.Trace(err)
	}

	var (
		createChannelStmt *sqlair.Statement
		channelInfo       applicationChannel
	)
	if ch := app.Channel; ch != nil {
		channelInfo = applicationChannel{
			ApplicationID: appID.String(),
			Track:         ch.Track,
			Risk:          string(ch.Risk),
			Branch:        ch.Branch,
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
		if err := tx.Query(ctx, createOriginStmt, originInfo).Run(); err != nil {
			return errors.Annotatef(err, "creating origin row for application %q", name)
		}
		if err := tx.Query(ctx, createCharmPlatformStmt, charmPlatformInfo).Run(); err != nil {
			return errors.Annotatef(err, "creating charm platform row for application %q", name)
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

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.deleteApplication(ctx, tx, name)
	})
	return errors.Annotatef(err, "deleting application %q", name)
}

func (st *ApplicationState) deleteApplication(ctx context.Context, tx *sqlair.TX, name string) error {

	var appID applicationID
	queryUnits := `SELECT count(*) AS &countResult.count FROM unit WHERE application_uuid = $applicationID.uuid`
	queryUnitsStmt, err := st.Prepare(queryUnits, countResult{}, appID)
	if err != nil {
		return errors.Trace(err)
	}

	appName := applicationName{Name: name}
	deleteApplication := `DELETE FROM application WHERE name = $applicationName.name`
	deleteApplicationStmt, err := st.Prepare(deleteApplication, appName)
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

	appUUID, err := st.lookupApplication(ctx, tx, name, true)
	if err != nil {
		return errors.Trace(err)
	}
	appID.ID = appUUID.String()

	// Check that there are no units.
	result := sqlair.M{}
	err = tx.Query(ctx, queryUnitsStmt, appID).Get(&result)
	if err != nil {
		return errors.Annotatef(err, "querying units for application %q", name)
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
		return nil, fmt.Errorf("unit %q not found%w", unitName, errors.Hide(applicationerrors.UnitNotFound))
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
	// if cc.Address != nil {
	//   TODO(units) - handle addresses
	// }
	// if cc.Ports != nil {
	//   TODO(units) - handle ports
	// }
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

// DeleteUnit deletes the specified unit.
func (st *ApplicationState) DeleteUnit(ctx context.Context, unitName string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.deleteUnit(ctx, tx, unitName)
	})
	return errors.Annotatef(err, "deleting unit %q", unitName)
}

func (st *ApplicationState) deleteUnit(ctx context.Context, tx *sqlair.TX, unitName string) error {

	unit := coreUnit{Name: unitName}

	queryUnit := `SELECT uuid as &coreUnit.uuid FROM unit WHERE name = $coreUnit.name`
	queryUnitStmt, err := st.Prepare(queryUnit, unit)
	if err != nil {
		return errors.Trace(err)
	}

	deleteUnit := `DELETE FROM unit WHERE name = $coreUnit.name`
	deleteUnitStmt, err := st.Prepare(deleteUnit, unit)
	if err != nil {
		return errors.Trace(err)
	}

	deleteNode := `
DELETE FROM net_node WHERE uuid IN
(SELECT net_node_uuid FROM unit WHERE name = $coreUnit.name)
`
	deleteNodeStmt, err := st.Prepare(deleteNode, unit)
	if err != nil {
		return errors.Trace(err)
	}

	err = tx.Query(ctx, queryUnitStmt, unit).Get(&unit)
	if errors.Is(err, sqlair.ErrNoRows) {
		// Unit already deleted is a no op.
		return nil
	}
	if err != nil {
		return errors.Annotatef(err, "looking up UUID for unit %q", unitName)
	}

	if err := tx.Query(ctx, deleteUnitStmt, unit).Run(); err != nil {
		return errors.Annotatef(err, "deleting unit %q", unitName)
	}
	if err := tx.Query(ctx, deleteNodeStmt, unit).Run(); err != nil {
		return errors.Annotatef(err, "deleting net node for unit  %q", unitName)
	}
	return nil
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
			if errors.Is(err, applicationerrors.UnitNotFound) {
				return st.insertUnit(ctx, tx, appID, args)
			}
			return errors.Trace(err)
		}
		return st.upsertUnit(ctx, tx, *unit, args)
	})
	return errors.Annotatef(err, "upserting unit %q for application %q", *args.UnitName, appID)
}

// GetUnitLife looks up the life of the specified unit, returning an error
// satisfying [applicationerrors.UnitNotFound] if the unit is not found.
func (st *ApplicationState) GetUnitLife(ctx domain.AtomicContext, unitName string) (life.Life, error) {
	unit := coreUnit{Name: unitName}
	queryUnit := `
SELECT unit.life_id AS &coreUnit.*
FROM unit
WHERE name = $coreUnit.name
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
			return fmt.Errorf("%w: %s", applicationerrors.UnitNotFound, unitName)
		}
		return nil
	})
	return unit.LifeID, errors.Annotatef(err, "querying unit %q life", unitName)
}

// SetUnitLife sets the life of the specified unit, returning an error
// satisfying [applicationerrors.UnitNotFound] if the unit is not found.
func (st *ApplicationState) SetUnitLife(ctx domain.AtomicContext, unitName string, l life.Life) error {
	unit := coreUnit{Name: unitName, LifeID: l}
	query := `
SELECT uuid AS &coreUnit.uuid
FROM unit
WHERE name = $coreUnit.name
`
	stmt, err := st.Prepare(query, unit)
	if err != nil {
		return errors.Trace(err)
	}

	updateLifeQuery := `
UPDATE unit
SET life_id = $coreUnit.life_id
WHERE name = $coreUnit.name
-- we ensure the life can never go backwards.
AND life_id < $coreUnit.life_id
`

	updateLifeStmt, err := st.Prepare(updateLifeQuery, unit)
	if err != nil {
		return errors.Trace(err)
	}

	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, unit).Get(&unit)
		if errors.Is(err, sqlair.ErrNoRows) {
			return fmt.Errorf("unit %q not found%w", unitName, errors.Hide(applicationerrors.UnitNotFound))
		} else if err != nil {
			return errors.Annotatef(err, "querying unit %q", unitName)
		}
		return tx.Query(ctx, updateLifeStmt, unit).Run()
	})
	return errors.Annotatef(err, "updating unit life for %q", unitName)
}

// GetApplicationScaleState looks up the scale state of the specified application, returning an error
// satisfying [applicationerrors.ApplicationNotFound] if the application is not found.
func (st *ApplicationState) GetApplicationScaleState(ctx domain.AtomicContext, appID coreapplication.ID) (application.ScaleState, error) {
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
	return appScale.toScaleState(), errors.Annotatef(err, "querying application %q scale", appID)
}

// GetApplicationLife looks up the life of the specified application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application is not found.
func (st *ApplicationState) GetApplicationLife(ctx domain.AtomicContext, appName string) (coreapplication.ID, life.Life, error) {
	app := applicationName{Name: appName}
	query := `
SELECT (*) AS (&applicationID.*)
FROM application a
WHERE name = $applicationName.name
`
	stmt, err := st.Prepare(query, app, applicationID{})
	if err != nil {
		return "", -1, errors.Trace(err)
	}

	var appInfo applicationID
	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		err = tx.Query(ctx, stmt, app).Get(&appInfo)
		if err != nil {
			if !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Annotatef(err, "querying life for application %q", appName)
			}
			return fmt.Errorf("%w: %s", applicationerrors.ApplicationNotFound, appName)
		}
		return nil
	})
	return coreapplication.ID(appInfo.ID), appInfo.LifeID, errors.Trace(err)
}

// SetApplicationLife sets the life of the specified application.
func (st *ApplicationState) SetApplicationLife(ctx domain.AtomicContext, appID coreapplication.ID, l life.Life) error {
	lifeQuery := `
UPDATE application
SET life_id = $applicationID.life_id
WHERE uuid = $applicationID.uuid
-- we ensure the life can never go backwards.
AND life_id <= $applicationID.life_id
`
	app := applicationID{ID: appID.String(), LifeID: l}
	lifeStmt, err := st.Prepare(lifeQuery, app)
	if err != nil {
		return errors.Trace(err)
	}

	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, lifeStmt, app).Run()
		return errors.Trace(err)
	})
	return errors.Annotatef(err, "updating application life for %q", appID)
}

// SetDesiredApplicationScale updates the desired scale of the specified application.
func (st *ApplicationState) SetDesiredApplicationScale(ctx domain.AtomicContext, appID coreapplication.ID, scale int) error {
	scaleDetails := applicationScale{
		ApplicationID: appID.String(),
		Scale:         scale,
	}
	upsertApplicationScale := `
UPDATE application_scale SET scale = $applicationScale.scale
WHERE application_uuid = $applicationScale.application_uuid
`

	upsertStmt, err := st.Prepare(upsertApplicationScale, scaleDetails)
	if err != nil {
		return errors.Trace(err)
	}
	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, upsertStmt, scaleDetails).Run()
	})
	return errors.Trace(err)
}

// SetApplicationScalingState sets the scaling details for the given caas application
// Scale is optional and is only set if not nil.
func (st *ApplicationState) SetApplicationScalingState(ctx domain.AtomicContext, appID coreapplication.ID, scale *int, targetScale int, scaling bool) error {
	scaleDetails := applicationScale{
		ApplicationID: appID.String(),
		Scaling:       scaling,
		ScaleTarget:   targetScale,
	}
	var setScaleTerm string
	if scale != nil {
		scaleDetails.Scale = *scale
		setScaleTerm = "scale = $applicationScale.scale,"
	}

	upsertApplicationScale := fmt.Sprintf(`
UPDATE application_scale SET
    %s
    scaling = $applicationScale.scaling,
    scale_target = $applicationScale.scale_target
WHERE application_uuid = $applicationScale.application_uuid
`, setScaleTerm)

	upsertStmt, err := st.Prepare(upsertApplicationScale, scaleDetails)
	if err != nil {
		return errors.Trace(err)
	}
	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, upsertStmt, scaleDetails).Run()
	})
	return errors.Trace(err)
}

// UpsertCloudService updates the cloud service for the specified application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
func (st *ApplicationState) UpsertCloudService(ctx context.Context, name, providerID string, sAddrs network.SpaceAddresses) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	// TODO(units) - handle addresses

	upsertCloudService := `
INSERT INTO cloud_service (*) VALUES ($cloudService.*)
ON CONFLICT(application_uuid) DO UPDATE
    SET provider_id = excluded.provider_id;
`

	upsertStmt, err := st.Prepare(upsertCloudService, cloudService{})
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		appID, err := st.lookupApplication(ctx, tx, name, false)
		if err != nil {
			return errors.Trace(err)
		}
		serviceInfo := cloudService{
			ApplicationID: appID.String(),
			ProviderID:    providerID,
		}
		return tx.Query(ctx, upsertStmt, serviceInfo).Run()
	})
	return errors.Annotatef(err, "updating cloud service for application %q", name)
}

// InitialWatchStatementUnitLife returns the initial namespace query for the application unit life watcher.
func (st *ApplicationState) InitialWatchStatementUnitLife(appName string) (string, eventsource.NamespaceQuery) {
	queryFunc := func(ctx context.Context, runner database.TxnRunner) ([]string, error) {
		app := applicationName{Name: appName}
		stmt, err := st.Prepare(`
SELECT u.uuid AS &unitDetails.uuid
FROM unit u
JOIN application a ON a.uuid = u.application_uuid
WHERE a.name = $applicationName.name
`, app, unitDetails{})
		if err != nil {
			return nil, errors.Trace(err)
		}
		var result []unitDetails
		err = runner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			err := tx.Query(ctx, stmt, app).GetAll(&result)
			if errors.Is(err, sqlair.ErrNoRows) {
				return nil
			}
			return errors.Trace(err)
		})
		if err != nil {
			return nil, errors.Annotatef(err, "querying unit IDs for %q", appName)
		}
		uuids := make([]string, len(result))
		for i, r := range result {
			uuids[i] = r.UnitID
		}
		return uuids, nil
	}
	return "unit", queryFunc
}

type unitIDs []string

// GetApplicationUnitLife returns the life values for the specified units of the given application.
// The supplied ids may belong to a different application; the application name is used to filter.
func (st *ApplicationState) GetApplicationUnitLife(ctx context.Context, appName string, ids ...string) (map[string]life.Life, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	lifeQuery := `
SELECT (u.uuid, u.life_id) AS (&unitDetails.*)
FROM unit u
JOIN application a ON a.uuid = u.application_uuid
WHERE u.uuid IN ($unitIDs[:])
AND a.name = $applicationName.name
`

	app := applicationName{Name: appName}
	lifeStmt, err := st.Prepare(lifeQuery, app, unitDetails{}, unitIDs{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var lifes []unitDetails
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, lifeStmt, unitIDs(ids), app).GetAll(&lifes)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return errors.Trace(err)
	})
	if err != nil {
		return nil, errors.Annotatef(err, "querying unit life for %q", appName)
	}
	result := make(map[string]life.Life)
	for _, u := range lifes {
		result[u.UnitID] = u.LifeID
	}
	return result, nil
}

// RemoveUnitMaybeApplication removes the unit from state, and may remove
// its application as well, if the application is Dying and no other references
// to it exist. It will fail if the unit is not Dead.
// An error satisfying [applicationerrors.UnitNotFound] is returned
// if the unit is not found.
func (st *ApplicationState) RemoveUnitMaybeApplication(ctx domain.AtomicContext, unitName string) error {
	unit := coreUnit{Name: unitName}
	peerCountQuery := `
SELECT a.life_id as &unitCount.app_life_id, u.life_id AS &unitCount.unit_life_id, count(peer.uuid) AS &unitCount.count
FROM unit u
JOIN application a ON a.uuid = u.application_uuid
LEFT JOIN unit peer ON u.application_uuid = peer.application_uuid AND peer.uuid != u.uuid
WHERE u.name = $coreUnit.name
`
	peerCountStmt, err := st.Prepare(peerCountQuery, unit, unitCount{})
	if err != nil {
		return errors.Trace(err)
	}
	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Count the number of units besides this one
		// belonging to the same application.
		var count unitCount
		err = tx.Query(ctx, peerCountStmt, unit).Get(&count)
		if errors.Is(err, sqlair.ErrNoRows) {
			return fmt.Errorf("unit %q not found%w", unitName, errors.Hide(applicationerrors.UnitNotFound))
		}
		if err != nil {
			return errors.Annotatef(err, "querying peer count for unit %q", unitName)
		}
		// This should never happen since this method is called by the service
		// after setting the unit to Dead. But we check anyway.
		// There's no need for a typed error.
		if count.UnitLifeID != life.Dead {
			return fmt.Errorf("unit %q is not dead, life is %v", unitName, count.UnitLifeID)
		}

		err = st.deleteUnit(ctx, tx, unitName)
		if err != nil {
			return errors.Trace(err)
		}
		if count.Count > 0 || count.ApplicationLifeID == life.Alive {
			return nil
		}
		// This is the last unit, and the application is
		// not alive, so we delete the application.
		appName, _ := names.UnitApplication(unitName)
		err = st.deleteApplication(ctx, tx, appName)
		return errors.Trace(err)
	})
	return errors.Annotatef(err, "removing unit %q", unitName)
}

// GetCharmByApplicationName returns the charm for the specified application
// name.
// This method should be used sparingly, as it is not efficient. It should
// be only used when you need the whole charm, otherwise use the more specific
// methods.
//
// If the application does not exist, an error satisfying
// [applicationerrors.ApplicationNotFoundError] is returned.
// If the charm for the application does not exist, an error satisfying
// [applicationerrors.CharmNotFoundError] is returned.
func (st *ApplicationState) GetCharmByApplicationName(ctx context.Context, name string) (charm.Charm, charm.CharmOrigin, application.Platform, error) {
	db, err := st.DB()
	if err != nil {
		return charm.Charm{}, charm.CharmOrigin{}, application.Platform{}, errors.Trace(err)
	}

	query, err := st.Prepare(`
SELECT &applicationCharmUUID.*
FROM application
WHERE uuid = $applicationID.uuid
`, applicationCharmUUID{}, applicationID{})
	if err != nil {
		return charm.Charm{}, charm.CharmOrigin{}, application.Platform{}, fmt.Errorf("preparing query for application %q: %w", name, err)
	}

	var (
		ch          charm.Charm
		chOrigin    charm.CharmOrigin
		appPlatform application.Platform
	)
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		appID, err := st.lookupApplication(ctx, tx, name, false)
		if err != nil {
			return fmt.Errorf("looking up application %q: %w", name, err)
		}

		appIdent := applicationID{ID: appID.String()}

		var charmIdent applicationCharmUUID
		if err := tx.Query(ctx, query, appIdent).Get(&charmIdent); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return fmt.Errorf("application %s: %w", name, applicationerrors.ApplicationNotFound)
			}
			return fmt.Errorf("getting charm for application %q: %w", name, err)
		}

		// If the charmUUID is empty, then something went wrong with adding an
		// application.
		if charmIdent.CharmUUID == "" {
			// Do not return a CharmNotFound error here. The application is in
			// a broken state. There isn't anything we can do to fix it here.
			// This will require manual intervention.
			return fmt.Errorf("application is missing charm")
		}

		// Now get the charm by the UUID, but if it doesn't exist, return an
		// error.
		chIdent := charmID{UUID: charmIdent.CharmUUID}
		ch, err = st.getCharm(ctx, tx, chIdent)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return fmt.Errorf("application %s: %w", name, applicationerrors.CharmNotFound)
			}
			return fmt.Errorf("getting charm for application %q: %w", name, err)
		}

		chOrigin, err = st.getCharmOrigin(ctx, tx, chIdent)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return fmt.Errorf("application %s: %w", name, applicationerrors.CharmNotFound)
			}
			return fmt.Errorf("getting charm origin for application %q: %w", name, err)
		}

		appPlatform, err = st.getPlatform(ctx, tx, appIdent)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return fmt.Errorf("application %s: %w", name, applicationerrors.InvalidApplicationState)
			}
			return fmt.Errorf("getting charm platform for application %q: %w", name, err)
		}
		return nil
	}); err != nil {
		return ch, chOrigin, appPlatform, errors.Trace(err)
	}

	return ch, chOrigin, appPlatform, nil
}

// getPlatform returns the application platform for the given charm ID.
func (s *ApplicationState) getPlatform(ctx context.Context, tx *sqlair.TX, ident applicationID) (application.Platform, error) {
	query := `
SELECT &applicationPlatform.*
FROM application_platform
WHERE application_uuid = $applicationID.uuid;
`

	stmt, err := s.Prepare(query, applicationPlatform{}, ident)
	if err != nil {
		return application.Platform{}, fmt.Errorf("failed to prepare query: %w", err)
	}

	var platform applicationPlatform
	if err := tx.Query(ctx, stmt, ident).Get(&platform); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return application.Platform{}, applicationerrors.CharmNotFound
		}
		return application.Platform{}, fmt.Errorf("failed to get application platform: %w", err)
	}
	return decodePlatform(platform)
}

func decodePlatform(platform applicationPlatform) (application.Platform, error) {
	osType, err := decodeOSType(platform.OSTypeID)
	if err != nil {
		return application.Platform{}, fmt.Errorf("failed to decode os type: %w", err)
	}

	arch, err := decodeArchitecture(platform.ArchitectureID)
	if err != nil {
		return application.Platform{}, fmt.Errorf("failed to decode architecture: %w", err)
	}

	return application.Platform{
		Channel:      platform.Channel,
		OSType:       osType,
		Architecture: arch,
	}, nil
}

func decodeOSType(osType int) (application.OSType, error) {
	switch osType {
	case 0:
		return charm.Ubuntu, nil
	default:
		return -1, fmt.Errorf("unsupported os type: %d", osType)
	}
}

func decodeArchitecture(arch int) (application.Architecture, error) {
	switch arch {
	case 0:
		return charm.AMD64, nil
	case 1:
		return charm.ARM64, nil
	case 2:
		return charm.PPC64EL, nil
	case 3:
		return charm.S390X, nil
	case 4:
		return charm.RISV64, nil
	default:
		return -1, fmt.Errorf("unsupported architecture: %d", arch)
	}
}
