// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	jujuerrors "github.com/juju/errors"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/instance"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/life"
	modelerrors "github.com/juju/juju/domain/model/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	storagestate "github.com/juju/juju/domain/storage/state"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// GetModelType returns the model type for the underlying model. If the model
// does not exist then an error satisfying [modelerrors.NotFound] will be returned.
func (st *State) GetModelType(ctx context.Context) (coremodel.ModelType, error) {
	db, err := st.DB()
	if err != nil {
		return "", jujuerrors.Trace(err)
	}

	var result modelInfo
	stmt, err := st.Prepare("SELECT &modelInfo.type FROM model", result)
	if err != nil {
		return "", jujuerrors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&result)
		if errors.Is(err, sql.ErrNoRows) {
			return modelerrors.NotFound
		}
		return err
	})
	if err != nil {
		return "", errors.Errorf("querying model type: %w", err)
	}
	return coremodel.ModelType(result.ModelType), nil
}

// CreateApplication creates an application, returning an error satisfying
// [applicationerrors.ApplicationAlreadyExists] if the application already exists.
// It returns as error satisfying [applicationerrors.CharmNotFound] if the charm
// for the application is not found.
func (st *State) CreateApplication(
	ctx context.Context,
	name string,
	args application.AddApplicationArg,
	units []application.AddUnitArg,
) (coreapplication.ID, error) {
	db, err := st.DB()
	if err != nil {
		return "", jujuerrors.Trace(err)
	}

	appUUID, err := coreapplication.NewID()
	if err != nil {
		return "", jujuerrors.Trace(err)
	}

	charmID, err := corecharm.NewID()
	if err != nil {
		return "", jujuerrors.Trace(err)
	}

	appDetails := applicationDetails{
		UUID:    appUUID,
		Name:    name,
		CharmID: charmID.String(),
		LifeID:  life.Alive,
	}

	createApplication := `INSERT INTO application (*) VALUES ($applicationDetails.*)`
	createApplicationStmt, err := st.Prepare(createApplication, appDetails)
	if err != nil {
		return "", jujuerrors.Trace(err)
	}

	scaleInfo := applicationScale{
		ApplicationID: appUUID,
		Scale:         args.Scale,
	}
	createScale := `INSERT INTO application_scale (*) VALUES ($applicationScale.*)`
	createScaleStmt, err := st.Prepare(createScale, scaleInfo)
	if err != nil {
		return "", jujuerrors.Trace(err)
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
		return "", jujuerrors.Trace(err)
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
			return "", jujuerrors.Trace(err)
		}
	}

	configHash, err := hashConfigAndSettings(args.Config, args.Settings)
	if err != nil {
		return "", jujuerrors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check if the application already exists.
		if err := st.checkApplicationNameAvailable(ctx, tx, name); err != nil {
			return fmt.Errorf("checking if application %q exists: %w", name, err)
		}

		shouldInsertCharm := true

		// Check if the charm already exists.
		existingCharmID, err := st.checkCharmReferenceExists(ctx, tx, referenceName, revision)
		if err != nil && !errors.Is(err, applicationerrors.CharmAlreadyExists) {
			return fmt.Errorf("checking if charm %q exists: %w", charmName, err)
		} else if errors.Is(err, applicationerrors.CharmAlreadyExists) {
			// We already have an existing charm, in this case we just want
			// to point the application to the existing charm.
			appDetails.CharmID = existingCharmID.String()

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
		if err := tx.Query(ctx, createScaleStmt, scaleInfo).Run(); err != nil {
			return errors.Errorf("inserting scale row for application %q: %w", name, err)
		}
		if err := st.createApplicationResources(
			ctx, tx,
			insertResourcesArgs{
				appID:        appDetails.UUID,
				charmUUID:    appDetails.CharmID,
				charmSource:  args.Charm.Source,
				appResources: args.Resources,
			},
			args.PendingResources,
		); err != nil {
			return errors.Errorf("inserting or resolving resources for application %q: %w", name, err)
		}
		if err := st.insertApplicationStorage(ctx, tx, appDetails, args.Storage); err != nil {
			return errors.Errorf("inserting storage for application %q: %w", name, err)
		}
		if err := st.insertApplicationConfig(ctx, tx, appDetails.UUID, args.Config); err != nil {
			return errors.Errorf("inserting config for application %q: %w", name, err)
		}
		if err := st.insertApplicationSettings(ctx, tx, appDetails.UUID, args.Settings); err != nil {
			return errors.Errorf("inserting settings for application %q: %w", name, err)
		}
		if err := st.insertApplicationConfigHash(ctx, tx, appDetails.UUID, configHash); err != nil {
			return errors.Errorf("inserting config hash for application %q: %w", name, err)
		}
		if err := st.insertApplicationStatus(ctx, tx, appDetails.UUID, args.Status); err != nil {
			return errors.Errorf("inserting status for application %q: %w", name, err)
		}

		// The channel is optional for local charms. Although, it would be
		// nice to have a channel for local charms, it's not a requirement.
		if createChannelStmt != nil {
			if err := tx.Query(ctx, createChannelStmt, channelInfo).Run(); err != nil {
				return errors.Errorf("inserting channel row for application %q: %w", name, err)
			}
		}

		if len(units) == 0 {
			return nil
		}

		if err = st.insertApplicationUnits(ctx, tx, appUUID, args, units); err != nil {
			return errors.Errorf("inserting units for application %q: %w", appUUID, err)
		}
		return nil
	})
	if err != nil {
		return "", errors.Errorf("creating application %q: %w", name, err)
	}
	return appUUID, nil
}

func (st *State) insertApplicationUnits(
	ctx context.Context, tx *sqlair.TX,
	appUUID coreapplication.ID, args application.AddApplicationArg, units []application.AddUnitArg,
) error {
	insertUnits := make([]application.InsertUnitArg, len(units))
	for i, unit := range units {
		insertUnits[i] = application.InsertUnitArg{
			UnitName:         unit.UnitName,
			Constraints:      unit.Constraints,
			Storage:          args.Storage,
			StoragePoolKind:  args.StoragePoolKind,
			StorageParentDir: args.StorageParentDir,
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus:    unit.UnitStatusArg.AgentStatus,
				WorkloadStatus: unit.UnitStatusArg.WorkloadStatus,
			},
		}
	}

	modelType, err := st.GetModelType(ctx)
	if err != nil {
		return errors.Errorf("getting model type: %w", err)
	}
	if modelType == coremodel.IAAS {
		for _, arg := range insertUnits {
			if err := st.insertIAASUnit(ctx, tx, appUUID, arg); err != nil {
				return errors.Errorf("inserting IAAS unit %q: %w", arg.UnitName, err)
			}
		}
	} else {
		for _, arg := range insertUnits {
			if err := st.insertCAASUnit(ctx, tx, appUUID, arg); err != nil {
				return errors.Errorf("inserting CAAS unit %q: %w", arg.UnitName, err)
			}
		}
	}
	return nil
}

// DeleteApplication deletes the specified application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
// If the application still has units, as error satisfying [applicationerrors.ApplicationHasUnits]
// is returned.
func (st *State) DeleteApplication(ctx context.Context, name string) error {
	db, err := st.DB()
	if err != nil {
		return jujuerrors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.deleteApplication(ctx, tx, name)
	})
	if err != nil {
		return errors.Errorf("deleting application %q: %w", name, err)
	}
	return nil
}

func (st *State) deleteApplication(ctx context.Context, tx *sqlair.TX, name string) error {
	app := applicationDetails{Name: name}
	queryUnitsStmt, err := st.Prepare(`
SELECT count(*) AS &countResult.count
FROM unit
WHERE application_uuid = $applicationDetails.uuid
`, countResult{}, app)
	if err != nil {
		return jujuerrors.Trace(err)
	}

	// NOTE: This is a work around because teardown is not implemented yet. Ideally,
	// our workflow will mean that by the time the application is dead and we are
	// ready to delete it, a worker will have already cleaned up all dependencies.
	// However, this is not the case yet. Remove the secret owner for the unit,
	// leaving the secret orphaned, to ensure we don't get a foreign key violation.
	deleteSecretOwner := `
DELETE FROM secret_application_owner
WHERE application_uuid = $applicationDetails.uuid
`
	deleteSecretOwnerStmt, err := st.Prepare(deleteSecretOwner, app)
	if err != nil {
		return jujuerrors.Trace(err)
	}

	deleteApplicationStmt, err := st.Prepare(`DELETE FROM application WHERE name = $applicationDetails.name`, app)
	if err != nil {
		return jujuerrors.Trace(err)
	}

	appUUID, err := st.lookupApplication(ctx, tx, name)
	if err != nil {
		return jujuerrors.Trace(err)
	}
	app.UUID = appUUID

	// Check that there are no units.
	var result countResult
	err = tx.Query(ctx, queryUnitsStmt, app).Get(&result)
	if err != nil {
		return errors.Errorf("querying units for application %q: %w", name, err)
	}
	if numUnits := result.Count; numUnits > 0 {
		return errors.Errorf("cannot delete application %q as it still has %d unit(s)%w", name, numUnits, jujuerrors.Hide(applicationerrors.ApplicationHasUnits))
	}

	if err := tx.Query(ctx, deleteSecretOwnerStmt, app).Run(); err != nil {
		return errors.Errorf("deleting secret owner for application %q: %w", name, err)
	}

	if err := st.deleteCloudServices(ctx, tx, appUUID); err != nil {
		return errors.Errorf("deleting cloud service for application %q: %w", name, err)
	}

	// TODO(units) - fix these tables to allow deletion of rows
	// Deleting resource row results in FK mismatch error,
	// foreign key mismatch - "resource" referencing "resource_meta"
	// even for empty tables and even though there's no FK
	// from resource_meta to resource.
	//
	// resource
	// resource_meta

	if err := st.deleteSimpleApplicationReferences(ctx, tx, app.UUID); err != nil {
		return errors.Errorf("deleting associated records for application %q: %w", name, err)
	}
	if err := tx.Query(ctx, deleteApplicationStmt, app).Run(); err != nil {
		return errors.Errorf("deleting application %q: %w", name, err)
	}
	return nil
}

func (st *State) deleteCloudServices(ctx context.Context, tx *sqlair.TX, appUUID coreapplication.ID) error {
	app := applicationDetails{UUID: appUUID}

	deleteNodeStmt, err := st.Prepare(`
DELETE FROM net_node WHERE uuid IN (
    SELECT net_node_uuid
    FROM k8s_service
    WHERE application_uuid = $applicationDetails.uuid
)`, app)
	if err != nil {
		return jujuerrors.Trace(err)
	}

	deleteCloudServiceStmt, err := st.Prepare(`
DELETE FROM k8s_service
WHERE application_uuid = $applicationDetails.uuid
`, app)
	if err != nil {
		return jujuerrors.Trace(err)
	}

	if err := tx.Query(ctx, deleteCloudServiceStmt, app).Run(); err != nil {
		return jujuerrors.Trace(err)
	}
	if err := tx.Query(ctx, deleteNodeStmt, app).Run(); err != nil {
		return errors.Errorf("deleting net node for cloud service application %q: %w", appUUID, err)
	}
	return nil
}

func (st *State) deleteSimpleApplicationReferences(ctx context.Context, tx *sqlair.TX, appUUID coreapplication.ID) error {
	app := applicationID{ID: appUUID}

	for _, table := range []string{
		"application_channel",
		"application_platform",
		"application_scale",
		"application_config",
		"application_config_hash",
		"application_constraint",
		"application_setting",
		"application_exposed_endpoint_space",
		"application_exposed_endpoint_cidr",
		"application_storage_directive",
	} {
		deleteApplicationReference := fmt.Sprintf(`DELETE FROM %s WHERE application_uuid = $applicationID.uuid`, table)
		deleteApplicationReferenceStmt, err := st.Prepare(deleteApplicationReference, app)
		if err != nil {
			return jujuerrors.Trace(err)
		}

		if err := tx.Query(ctx, deleteApplicationReferenceStmt, app).Run(); err != nil {
			return errors.Errorf("deleting reference to application in table %q: %w", table, err)
		}
	}
	return nil
}

// StorageDefaults returns the default storage sources for a model.
func (st *State) StorageDefaults(ctx context.Context) (domainstorage.StorageDefaults, error) {
	rval := domainstorage.StorageDefaults{}

	db, err := st.DB()
	if err != nil {
		return rval, jujuerrors.Trace(err)
	}

	attrs := []string{application.StorageDefaultBlockSourceKey, application.StorageDefaultFilesystemSourceKey}
	attrsSlice := sqlair.S(transform.Slice(attrs, func(s string) any { return any(s) }))
	stmt, err := st.Prepare(`
SELECT &KeyValue.* FROM model_config WHERE key IN ($S[:])
`, sqlair.S{}, KeyValue{})
	if err != nil {
		return rval, jujuerrors.Trace(err)
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
func (st *State) GetStoragePoolByName(ctx context.Context, name string) (domainstorage.StoragePoolDetails, error) {
	db, err := st.DB()
	if err != nil {
		return domainstorage.StoragePoolDetails{}, jujuerrors.Trace(err)
	}
	return storagestate.GetStoragePoolByName(ctx, db, name)
}

// GetUnitLife looks up the life of the specified unit, returning an error
// satisfying [applicationerrors.UnitNotFound] if the unit is not found.
func (st *State) GetUnitLife(ctx context.Context, unitName coreunit.Name) (life.Life, error) {
	db, err := st.DB()
	if err != nil {
		return -1, jujuerrors.Trace(err)
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
	unit := minimalUnit{Name: unitName}
	queryUnit := `
SELECT &minimalUnit.life_id
FROM unit
WHERE name = $minimalUnit.name
`
	queryUnitStmt, err := st.Prepare(queryUnit, unit)
	if err != nil {
		return -1, jujuerrors.Trace(err)
	}

	err = tx.Query(ctx, queryUnitStmt, unit).Get(&unit)
	if err != nil {
		if !errors.Is(err, sqlair.ErrNoRows) {
			return -1, errors.Errorf("querying unit %q life: %w", unitName, err)
		}
		return -1, errors.Errorf("%w: %s", applicationerrors.UnitNotFound, unitName)
	}
	return unit.LifeID, nil
}

// GetApplicationScaleState looks up the scale state of the specified application, returning an error
// satisfying [applicationerrors.ApplicationNotFound] if the application is not found.
func (st *State) GetApplicationScaleState(ctx context.Context, appUUID coreapplication.ID) (application.ScaleState, error) {
	db, err := st.DB()
	if err != nil {
		return application.ScaleState{}, jujuerrors.Trace(err)
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
		return application.ScaleState{}, jujuerrors.Trace(err)
	}

	err = tx.Query(ctx, queryScaleStmt, appScale).Get(&appScale)
	if err != nil {
		if !errors.Is(err, sqlair.ErrNoRows) {
			return application.ScaleState{}, errors.Errorf("querying application %q scale: %w", appUUID, err)
		}
		return application.ScaleState{}, errors.Errorf("%w: %s", applicationerrors.ApplicationNotFound, appUUID)
	}
	return appScale.toScaleState(), nil
}

// GetApplicationLife looks up the life of the specified application, returning
// an error satisfying [applicationerrors.ApplicationNotFoundError] if the
// application is not found.
func (st *State) GetApplicationLife(ctx context.Context, appName string) (coreapplication.ID, life.Life, error) {
	db, err := st.DB()
	if err != nil {
		return "", -1, jujuerrors.Trace(err)
	}

	var app applicationDetails
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		app, err = st.getApplicationDetails(ctx, tx, appName)
		if err != nil {
			return errors.Errorf("querying life for application %q: %w", appName, err)
		}
		return nil
	})
	return app.UUID, app.LifeID, jujuerrors.Trace(err)
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
		return applicationDetails{}, jujuerrors.Trace(err)
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

// SetApplicationLife sets the life of the specified application.
func (st *State) SetApplicationLife(ctx context.Context, appUUID coreapplication.ID, l life.Life) error {
	db, err := st.DB()
	if err != nil {
		return jujuerrors.Trace(err)
	}

	lifeQuery := `
UPDATE application
SET life_id = $applicationIDAndLife.life_id
WHERE uuid = $applicationIDAndLife.uuid
-- we ensure the life can never go backwards.
AND life_id <= $applicationIDAndLife.life_id
`
	app := applicationIDAndLife{ID: appUUID, LifeID: l}
	lifeStmt, err := st.Prepare(lifeQuery, app)
	if err != nil {
		return jujuerrors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, lifeStmt, app).Run()
		if err != nil {
			return errors.Errorf("updating application life for %q: %w", appUUID, err)
		}
		return nil
	})
	return jujuerrors.Trace(err)
}

// SetDesiredApplicationScale updates the desired scale of the specified
// application.
func (st *State) SetDesiredApplicationScale(ctx context.Context, appUUID coreapplication.ID, scale int) error {
	db, err := st.DB()
	if err != nil {
		return jujuerrors.Trace(err)
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
		return jujuerrors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, upsertStmt, scaleDetails).Run()
	})
	return jujuerrors.Trace(err)
}

// UpdateApplicationScale updates the desired scale of an application by a
// delta.
// If the resulting scale is less than zero, an error satisfying
// [applicationerrors.ScaleChangeInvalid] is returned.
func (st *State) UpdateApplicationScale(ctx context.Context, appUUID coreapplication.ID, delta int) (int, error) {
	db, err := st.DB()
	if err != nil {
		return -1, jujuerrors.Trace(err)
	}

	upsertApplicationScale := `
UPDATE application_scale SET scale = $applicationScale.scale
WHERE application_uuid = $applicationScale.application_uuid
`
	upsertStmt, err := st.Prepare(upsertApplicationScale, applicationScale{})
	if err != nil {
		return -1, jujuerrors.Trace(err)
	}
	var newScale int
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		currentScaleState, err := st.getApplicationScaleState(ctx, tx, appUUID)
		if err != nil {
			return jujuerrors.Trace(err)
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
	return newScale, jujuerrors.Trace(err)
}

// SetApplicationScalingState sets the scaling details for the given caas
// application Scale is optional and is only set if not nil.
func (st *State) SetApplicationScalingState(ctx context.Context, appName string, targetScale int, scaling bool) error {
	db, err := st.DB()
	if err != nil {
		return jujuerrors.Trace(err)
	}

	scaleDetails := applicationScale{
		Scaling:     scaling,
		ScaleTarget: targetScale,
	}

	upsertApplicationScale := fmt.Sprintf(`
UPDATE application_scale SET
    scale = $applicationScale.scale,
    scaling = $applicationScale.scaling,
    scale_target = $applicationScale.scale_target
WHERE application_uuid = $applicationScale.application_uuid
`)

	upsertStmt, err := st.Prepare(upsertApplicationScale, scaleDetails)
	if err != nil {
		return jujuerrors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		appDetails, err := st.getApplicationDetails(ctx, tx, appName)
		if err != nil {
			return jujuerrors.Trace(err)
		}
		scaleDetails.ApplicationID = appDetails.UUID

		currentScaleState, err := st.getApplicationScaleState(ctx, tx, appDetails.UUID)
		if err != nil {
			return jujuerrors.Trace(err)
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
	return jujuerrors.Trace(err)
}

// UpsertCloudService updates the cloud service for the specified application,
// returning an error satisfying [applicationerrors.ApplicationNotFoundError] if
// the application doesn't exist.
func (st *State) UpsertCloudService(ctx context.Context, applicationName, providerID string, sAddrs network.SpaceAddresses) error {
	db, err := st.DB()
	if err != nil {
		return jujuerrors.Trace(err)
	}

	// TODO(units) - handle addresses

	serviceInfo := cloudService{ProviderID: providerID}

	// Query any existing records for application and provider id.
	queryExistingStmt, err := st.Prepare(`
SELECT &cloudService.* FROM k8s_service
WHERE application_uuid = $cloudService.application_uuid
AND provider_id = $cloudService.provider_id`, serviceInfo)
	if err != nil {
		return jujuerrors.Trace(err)
	}

	createNodeStmt, err := st.Prepare(`
INSERT INTO net_node (uuid) VALUES ($cloudService.net_node_uuid)
`, serviceInfo)
	if err != nil {
		return jujuerrors.Trace(err)
	}

	insertStmt, err := st.Prepare(`
INSERT INTO k8s_service (*) VALUES ($cloudService.*)
`, serviceInfo)
	if err != nil {
		return jujuerrors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		appUUID, err := st.lookupApplication(ctx, tx, applicationName)
		if err != nil {
			return jujuerrors.Trace(err)
		}
		serviceInfo.ApplicationUUID = appUUID

		// First see if the cloud service for the app and provider id already exists.
		// If so, it's a no-op.
		err = tx.Query(ctx, queryExistingStmt, serviceInfo).Get(&serviceInfo)
		if err != nil && !jujuerrors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"querying cloud service for application %q and provider id %q: %w", applicationName, providerID, err)
		}
		if err == nil {
			return nil
		}

		// Nothing already exists so create a new net node for the cloud service.
		nodeUUID, err := uuid.NewUUID()
		if err != nil {
			return jujuerrors.Trace(err)
		}
		serviceInfo.NetNodeUUID = nodeUUID.String()
		if err := tx.Query(ctx, createNodeStmt, serviceInfo).Run(); err != nil {
			return errors.Errorf("creating cloud service net node for application %q: %w", applicationName, err)
		}
		serviceInfo.ProviderID = providerID
		uuid, err := uuid.NewUUID()
		if err != nil {
			return jujuerrors.Trace(err)
		}
		serviceInfo.UUID = uuid.String()
		return tx.Query(ctx, insertStmt, serviceInfo).Run()
	})
	if err != nil {
		return errors.Errorf("updating cloud service for application %q: %w", applicationName, err)
	}
	return nil
}

// GetApplicationStatus looks up the status of the specified application,
// returning an error satisfying [applicationerrors.ApplicationNotFound] if the
// application is not found.
func (st *State) GetApplicationStatus(ctx context.Context, appID coreapplication.ID) (*application.StatusInfo[application.WorkloadStatusType], error) {
	db, err := st.DB()
	if err != nil {
		return nil, jujuerrors.Trace(err)
	}

	identID := applicationID{ID: appID}
	query, err := st.Prepare(`
SELECT &applicationStatusInfo.*
FROM application_status
WHERE application_uuid = $applicationID.uuid;
`, identID, applicationStatusInfo{})
	if err != nil {
		return nil, jujuerrors.Trace(err)
	}
	var status applicationStatusInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkApplicationNotDead(ctx, tx, appID); err != nil {
			return jujuerrors.Trace(err)
		}
		if err := tx.Query(ctx, query, identID).Get(&status); errors.Is(err, sqlair.ErrNoRows) {
			// If the application status is not set, then it's up to the
			// the caller to either return that information or use derive
			// the status from the units.
			return nil
		} else if err != nil {
			return jujuerrors.Trace(err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	statusType, err := decodeWorkloadStatus(status.StatusID)
	if err != nil {
		return nil, jujuerrors.Trace(err)
	}

	return &application.StatusInfo[application.WorkloadStatusType]{
		Status:  statusType,
		Message: status.Message,
		Data:    status.Data,
		Since:   status.UpdatedAt,
	}, nil
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
			return nil, jujuerrors.Trace(err)
		}

		var results []applicationID
		err = runner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			err := tx.Query(ctx, stmt).GetAll(&results)
			if errors.Is(err, sqlair.ErrNoRows) {
				return nil
			}
			return jujuerrors.Trace(err)
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
			return nil, jujuerrors.Trace(err)
		}
		var result []applicationConfigHash
		err = runner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			err := tx.Query(ctx, stmt, app).GetAll(&result)
			if errors.Is(err, sqlair.ErrNoRows) {
				return nil
			}
			return jujuerrors.Trace(err)
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

// GetApplicationsWithPendingCharmsFromUUIDs returns the application IDs for the
// applications with pending charms from the specified UUIDs.
func (st *State) GetApplicationsWithPendingCharmsFromUUIDs(ctx context.Context, uuids []coreapplication.ID) ([]coreapplication.ID, error) {
	db, err := st.DB()
	if err != nil {
		return nil, jujuerrors.Trace(err)
	}

	type applicationIDs []coreapplication.ID

	stmt, err := st.Prepare(`
SELECT a.uuid AS &applicationID.uuid
FROM application AS a
JOIN charm AS c ON a.charm_uuid = c.uuid
WHERE a.uuid IN ($applicationIDs[:]) AND c.available = FALSE
`, applicationID{}, applicationIDs{})
	if err != nil {
		return nil, jujuerrors.Trace(err)
	}

	var results []applicationID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, applicationIDs(uuids)).GetAll(&results)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return jujuerrors.Trace(err)
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
	db, err := st.DB()
	if err != nil {
		return "", jujuerrors.Trace(err)
	}

	query, err := st.Prepare(`
SELECT &applicationCharmUUID.*
FROM application
WHERE uuid = $applicationID.uuid
	`, applicationCharmUUID{}, applicationID{})
	if err != nil {
		return "", errors.Errorf("preparing query for application %q: %w", name, err)
	}

	var result charmID
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		appUUID, err := st.lookupApplication(ctx, tx, name)
		if err != nil {
			return errors.Errorf("looking up application %q: %w", name, err)
		}

		appIdent := applicationID{ID: appUUID}

		var charmIdent applicationCharmUUID
		if err := tx.Query(ctx, query, appIdent).Get(&charmIdent); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("application %s: %w", name, applicationerrors.ApplicationNotFound)
			}
			return errors.Errorf("getting charm for application %q: %w", name, err)
		}

		// If the charmUUID is empty, then something went wrong with adding an
		// application.
		if charmIdent.CharmUUID == "" {
			// Do not return a CharmNotFound error here. The application is in
			// a broken state. There isn't anything we can do to fix it here.
			// This will require manual intervention.
			return errors.Errorf("application is missing charm")
		}

		// Now get the charm by the UUID, but if it doesn't exist, return an
		// error.
		chIdent := charmID{UUID: charmIdent.CharmUUID}
		err = st.checkCharmExists(ctx, tx, chIdent)
		if err != nil {
			return errors.Errorf("getting charm for application %q: %w", name, err)
		}

		result = chIdent

		return nil
	}); err != nil {
		return "", jujuerrors.Trace(err)
	}

	return corecharm.ParseID(result.UUID)
}

// GetCharmByApplicationID returns the charm for the specified application
// ID.
// This method should be used sparingly, as it is not efficient. It should
// be only used when you need the whole charm, otherwise use the more specific
// methods.
//
// If the application does not exist, an error satisfying
// [applicationerrors.ApplicationNotFoundError] is returned.
// If the charm for the application does not exist, an error satisfying
// [applicationerrors.CharmNotFoundError] is returned.
func (st *State) GetCharmByApplicationID(ctx context.Context, appUUID coreapplication.ID) (charm.Charm, error) {
	db, err := st.DB()
	if err != nil {
		return charm.Charm{}, jujuerrors.Trace(err)
	}

	query, err := st.Prepare(`
SELECT &applicationCharmUUID.*
FROM application
WHERE uuid = $applicationID.uuid
`, applicationCharmUUID{}, applicationID{})
	if err != nil {
		return charm.Charm{}, errors.Errorf("preparing query for application %q: %w", appUUID, err)
	}

	var ch charm.Charm
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		appIdent := applicationID{ID: appUUID}

		var charmIdent applicationCharmUUID
		if err := tx.Query(ctx, query, appIdent).Get(&charmIdent); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("application %s: %w", appUUID, applicationerrors.ApplicationNotFound)
			}
			return errors.Errorf("getting charm for application %q: %w", appUUID, err)
		}

		// If the charmUUID is empty, then something went wrong with adding an
		// application.
		if charmIdent.CharmUUID == "" {
			// Do not return a CharmNotFound error here. The application is in
			// a broken state. There isn't anything we can do to fix it here.
			// This will require manual intervention.
			return errors.Errorf("application is missing charm")
		}

		// Now get the charm by the UUID, but if it doesn't exist, return an
		// error.
		chIdent := charmID{UUID: charmIdent.CharmUUID}
		ch, _, err = st.getCharm(ctx, tx, chIdent)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("application %s: %w", appUUID, applicationerrors.CharmNotFound)
			}
			return errors.Errorf("getting charm for application %q: %w", appUUID, err)
		}
		return nil
	}); err != nil {
		return ch, jujuerrors.Trace(err)
	}

	return ch, nil
}

// GetApplicationIDByUnitName returns the application ID for the named unit.
//
// Returns an error satisfying [applicationerrors.UnitNotFound] if the unit
// doesn't exist.
func (st *State) GetApplicationIDByUnitName(
	ctx context.Context,
	name coreunit.Name,
) (coreapplication.ID, error) {
	db, err := st.DB()
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
	db, err := st.DB()
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
	db, err := st.DB()
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
	db, err := st.DB()
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
		return application.CharmDownloadInfo{}, fmt.Errorf("decoding charm provenance: %w", err)
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
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	charmUUID := charmID{UUID: id.String()}

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
		return fmt.Errorf("preparing query: %w", err)
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
			return fmt.Errorf("updating charm state: %w", err)
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
	db, err := st.DB()
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
				Channel: application.Channel{
					Track:  r.ChannelTrack,
					Risk:   risk,
					Branch: r.ChannelBranch,
				},
				Platform: application.Platform{
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
	db, err := st.DB()
	if err != nil {
		return nil, application.ApplicationSettings{}, errors.Capture(err)
	}

	// We don't currently check for life in the old code, it might though be
	// worth checking if the application is not dead.
	ident := applicationID{ID: appID}

	configQuery := `
SELECT &applicationConfig.*
FROM v_application_config
WHERE uuid = $applicationID.uuid;
`
	settingsQuery := `
SELECT &applicationSettings.*
FROM application_setting
WHERE application_uuid = $applicationID.uuid;`

	configStmt, err := st.Prepare(configQuery, applicationConfig{}, ident)
	if err != nil {
		return nil, application.ApplicationSettings{}, errors.Errorf("preparing query for application config: %w", err)
	}
	settingsStmt, err := st.Prepare(settingsQuery, applicationSettings{}, ident)
	if err != nil {
		return nil, application.ApplicationSettings{}, errors.Errorf("preparing query for application config: %w", err)
	}

	var configs []applicationConfig
	var settings applicationSettings
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkApplicationNotDead(ctx, tx, appID); err != nil {
			return errors.Capture(err)
		}

		if err := tx.Query(ctx, configStmt, ident).GetAll(&configs); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying application config: %w", err)
		}

		if err := tx.Query(ctx, settingsStmt, ident).Get(&settings); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
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

// GetApplicationTrustSetting returns the application trust setting.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (st *State) GetApplicationTrustSetting(ctx context.Context, appID coreapplication.ID) (bool, error) {
	db, err := st.DB()
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

// SetApplicationConfig sets the application config attributes using the
// configuration.
func (st *State) SetApplicationConfigAndSettings(
	ctx context.Context,
	appID coreapplication.ID,
	cID corecharm.ID,
	config map[string]application.ApplicationConfig,
	settings application.ApplicationSettings,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	ident := applicationID{ID: appID}
	charmIdent := charmID{UUID: cID.String()}

	getQuery := `
SELECT &applicationConfig.*
FROM v_application_config
WHERE uuid = $applicationID.uuid;
`
	deleteQuery := `
DELETE FROM application_config
WHERE application_uuid = $applicationID.uuid
AND key IN ($S[:]);
`
	insertQuery := `
INSERT INTO application_config (*)
VALUES ($setApplicationConfig.*);
`
	updateQuery := `
UPDATE application_config
SET value = $setApplicationConfig.value,
	type_id = $setApplicationConfig.type_id
WHERE application_uuid = $setApplicationConfig.application_uuid;
`
	settingsQuery := `
INSERT INTO application_setting (*)
VALUES ($setApplicationSettings.*)
ON CONFLICT(application_uuid) DO UPDATE SET
	trust = excluded.trust;
`
	setHashQuery := `
UPDATE application_config_hash
SET sha256 = $applicationConfigHash.sha256
WHERE application_uuid = $applicationConfigHash.application_uuid;
`

	getStmt, err := st.Prepare(getQuery, applicationConfig{}, ident)
	if err != nil {
		return errors.Errorf("preparing get query: %w", err)
	}
	deleteStmt, err := st.Prepare(deleteQuery, ident, sqlair.S{})
	if err != nil {
		return errors.Errorf("preparing delete query: %w", err)
	}
	insertStmt, err := st.Prepare(insertQuery, setApplicationConfig{})
	if err != nil {
		return errors.Errorf("preparing insert query: %w", err)
	}
	updateStmt, err := st.Prepare(updateQuery, setApplicationConfig{})
	if err != nil {
		return errors.Errorf("preparing update query: %w", err)
	}
	settingsStmt, err := st.Prepare(settingsQuery, setApplicationSettings{})
	if err != nil {
		return errors.Errorf("preparing settings query: %w", err)
	}
	setHashStmt, err := st.Prepare(setHashQuery, applicationConfigHash{})
	if err != nil {
		return errors.Errorf("preparing set hash query: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkApplicationNotDead(ctx, tx, appID); err != nil {
			return errors.Capture(err)
		}
		if err := st.checkApplicationCharm(ctx, tx, ident, charmIdent); err != nil {
			return errors.Capture(err)
		}

		var current []applicationConfig
		if err := tx.Query(ctx, getStmt, ident).GetAll(&current); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying application config: %w", err)
		}

		currentM := make(map[string]applicationConfig)
		for _, c := range current {
			currentM[c.Key] = c
		}

		// Work out what we need to do, based on what we have, vs what we
		// need.
		var removals sqlair.S
		var updates []setApplicationConfig
		for k, currentCfg := range currentM {
			cfg, ok := config[k]
			if !ok {
				removals = append(removals, k)
				continue
			}

			// If the value and type are the same, we don't need to update. It
			// should be safe to compare the types, even if we're casting a
			// string to the type. This is because the type will either match or
			// not.
			if cfg.Value == currentCfg.Value && cfg.Type == charm.OptionType(currentCfg.Type) {
				continue
			}

			typeID, err := encodeConfigType(cfg.Type)
			if err != nil {
				return errors.Errorf("encoding config type: %w", err)
			}

			updates = append(updates, setApplicationConfig{
				ApplicationUUID: ident.ID.String(),
				Key:             k,
				Value:           cfg.Value,
				TypeID:          typeID,
			})

		}
		var inserts []setApplicationConfig
		for k, v := range config {
			if _, ok := currentM[k]; ok {
				continue
			}

			typeID, err := encodeConfigType(v.Type)
			if err != nil {
				return errors.Errorf("encoding config type: %w", err)
			}

			inserts = append(inserts, setApplicationConfig{
				ApplicationUUID: ident.ID.String(),
				Key:             k,
				Value:           v.Value,
				TypeID:          typeID,
			})
		}

		// We have to check the foreign key constraint on each request, as
		// each one is optional, bar the last query.

		if len(removals) > 0 {
			if err := tx.Query(ctx, deleteStmt, removals, ident).Run(); internaldatabase.IsErrConstraintForeignKey(err) {
				return applicationerrors.ApplicationNotFound
			} else if err != nil {
				return errors.Errorf("deleting config: %w", err)
			}
		}
		if len(inserts) > 0 {
			if err := tx.Query(ctx, insertStmt, inserts).Run(); internaldatabase.IsErrConstraintForeignKey(err) {
				return applicationerrors.ApplicationNotFound
			} else if err != nil {
				return errors.Errorf("inserting config: %w", err)
			}
		}
		for _, update := range updates {
			if err := tx.Query(ctx, updateStmt, update).Run(); internaldatabase.IsErrConstraintForeignKey(err) {
				return applicationerrors.ApplicationNotFound
			} else if err != nil {
				return errors.Errorf("updating config: %w", err)
			}
		}

		if err := tx.Query(ctx, settingsStmt, setApplicationSettings{
			ApplicationUUID: ident.ID.String(),
			Trust:           settings.Trust,
		}).Run(); internaldatabase.IsErrConstraintForeignKey(err) {
			return applicationerrors.ApplicationNotFound
		} else if err != nil {
			return errors.Errorf("updating settings: %w", err)
		}

		configHash, err := hashConfigAndSettings(config, settings)
		if err != nil {
			return errors.Errorf("hashing config and settings: %w", err)
		}

		if err := tx.Query(ctx, setHashStmt, applicationConfigHash{
			ApplicationUUID: ident.ID.String(),
			SHA256:          configHash,
		}).Run(); err != nil {
			return errors.Errorf("setting hash: %w", err)
		}

		return nil
	})
	if err != nil {
		return errors.Errorf("setting application config: %w", err)
	}
	return nil
}

// UnsetApplicationConfigKeys removes the specified keys from the application
// config. If the key does not exist, it is ignored.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (st *State) UnsetApplicationConfigKeys(ctx context.Context, appID coreapplication.ID, keys []string) error {
	db, err := st.DB()
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
			ApplicationUUID: ident.ID.String(),
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
// If the charm for the application does not exist, an error satisfying
// [applicationerrors.CharmNotFoundError] is returned.
func (st *State) GetCharmConfigByApplicationID(ctx context.Context, appID coreapplication.ID) (corecharm.ID, charm.Config, error) {
	db, err := st.DB()
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

		charmConfig, err = st.getCharmConfig(ctx, tx, charmID{UUID: ident.UUID})
		return errors.Capture(err)
	}); err != nil {
		return "", charm.Config{}, errors.Capture(err)
	}

	charmID, err := corecharm.ParseID(ident.UUID)
	if err != nil {
		return "", charm.Config{}, errors.Errorf("parsing charm id: %w", err)
	}

	return charmID, charmConfig, nil
}

// GetApplicationIDByName returns the application ID for the named application.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (st *State) GetApplicationIDByName(ctx context.Context, name string) (coreapplication.ID, error) {
	db, err := st.DB()
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

// GetApplicationConfigHash returns the SHA256 hash of the application config
// for the specified application ID.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (st *State) GetApplicationConfigHash(ctx context.Context, appID coreapplication.ID) (string, error) {
	db, err := st.DB()
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
	db, err := st.DB()
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
			return errors.Capture(err)
		}

		err := tx.Query(ctx, stmtOrigin, ident).Get(&appOrigin)
		if err != nil {
			return errors.Capture(err)
		}

		err = tx.Query(ctx, stmtPlatformChannel, ident).Get(&appPlatformChan)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}

		return nil
	}); err != nil {
		return application.CharmOrigin{}, errors.Errorf("querying application platform and channel for application %q: %w", appID, err)
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

	return application.CharmOrigin{
		Name:     appOrigin.ReferenceName,
		Source:   source,
		Platform: platform,
		Channel:  channel,
	}, nil
}

// GetApplicationConstraints returns the application constraints for the
// specified application ID.
// Empty constraints are returned if no constraints exist for the given
// application ID.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (st *State) GetApplicationConstraints(ctx context.Context, appID coreapplication.ID) (constraints.Constraints, error) {
	db, err := st.DB()
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
	db, err := st.DB()
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
			if errors.Is(err, sql.ErrNoRows) {
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
				if errors.Is(err, sql.ErrNoRows) {
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

// NamespaceForWatchApplicationScale returns the namespace identifier
// for application scale change watchers.
func (*State) NamespaceForWatchApplicationScale() string {
	return "application_scale"
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
			cpuCores := uint64(row.CPUCores.Int64)
			res.CpuCores = &cpuCores
		}
		if row.CPUPower.Valid {
			cpuPower := uint64(row.CPUPower.Int64)
			res.CpuPower = &cpuPower
		}
		if row.Mem.Valid {
			mem := uint64(row.Mem.Int64)
			res.Mem = &mem
		}
		if row.RootDisk.Valid {
			rootDisk := uint64(row.RootDisk.Int64)
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
		return "", jujuerrors.Trace(err)
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
			ApplicationUUID: appID.String(),
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
		ApplicationUUID: appID.String(),
		Trust:           settings.Trust,
	}).Run(); err != nil {
		return errors.Errorf("inserting settings: %w", err)
	}

	return nil
}

func (st *State) insertApplicationConfigHash(
	ctx context.Context,
	tx *sqlair.TX,
	appID coreapplication.ID,
	sha256 string,
) error {
	insertQuery := `
INSERT INTO application_config_hash (*) VALUES ($applicationConfigHash.*);
`
	insertStmt, err := st.Prepare(insertQuery, applicationConfigHash{})
	if err != nil {
		return errors.Errorf("preparing insert query: %w", err)
	}

	if err := tx.Query(ctx, insertStmt, applicationConfigHash{
		ApplicationUUID: appID.String(),
		SHA256:          sha256,
	}).Run(); err != nil {
		return errors.Errorf("inserting hash: %w", err)
	}
	return nil
}

func (st *State) insertApplicationStatus(
	ctx context.Context,
	tx *sqlair.TX,
	appID coreapplication.ID,
	status *application.StatusInfo[application.WorkloadStatusType],
) error {
	if status == nil {
		return nil
	}

	insertQuery := `
INSERT INTO application_status (*) VALUES ($applicationStatus.*);
`

	insertStmt, err := st.Prepare(insertQuery, applicationStatus{})
	if err != nil {
		return errors.Errorf("preparing insert query: %w", err)
	}

	statusID, err := encodeWorkloadStatus(status.Status)
	if err != nil {
		return errors.Errorf("encoding status: %w", err)
	}

	if err := tx.Query(ctx, insertStmt, applicationStatus{
		ApplicationUUID: appID.String(),
		StatusID:        statusID,
		Message:         status.Message,
		Data:            status.Data,
		UpdatedAt:       status.Since,
	}).Run(); err != nil {
		return errors.Errorf("inserting status: %w", err)
	}
	return nil
}

func decodeRisk(risk string) (application.ChannelRisk, error) {
	switch risk {
	case "stable":
		return application.RiskStable, nil
	case "candidate":
		return application.RiskCandidate, nil
	case "beta":
		return application.RiskBeta, nil
	case "edge":
		return application.RiskEdge, nil
	default:
		return "", errors.Errorf("unknown risk %q", risk)
	}
}

func decodeOSType(osType sql.NullInt64) (application.OSType, error) {
	if !osType.Valid {
		return 0, errors.Errorf("os type is null")
	}

	switch osType.Int64 {
	case 0:
		return application.Ubuntu, nil
	default:
		return -1, errors.Errorf("unknown os type %v", osType)
	}
}

func hashConfigAndSettings(config map[string]application.ApplicationConfig, settings application.ApplicationSettings) (string, error) {
	h := sha256.New()

	// Ensure we have a stable order for the keys.
	keys := slices.Collect(maps.Keys(config))
	sort.Strings(keys)

	for _, key := range keys {
		if _, err := h.Write([]byte(key)); err != nil {
			return "", errors.Errorf("writing key %q: %w", key, err)
		}

		v, ok := config[key]
		if !ok {
			return "", errors.Errorf("missing value for key %q", key)
		}

		val, err := encodeConfigValue(v)
		if err != nil {
			return "", errors.Errorf("encoding value for key %q: %w", key, err)
		}
		if _, err := h.Write([]byte(val)); err != nil {
			return "", errors.Errorf("writing value for key %q: %w", key, err)
		}
	}
	if _, err := h.Write([]byte(fmt.Sprintf("%t", settings.Trust))); err != nil {
		return "", errors.Errorf("writing trust setting: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func encodeConfigValue(v application.ApplicationConfig) (string, error) {
	switch v.Type {
	case charm.OptionBool:
		b, ok := v.Value.(bool)
		if !ok {
			return "", errors.Errorf("value is not a bool")
		}
		return strconv.FormatBool(b), nil
	case charm.OptionInt:
		switch t := v.Value.(type) {
		case int:
			return strconv.Itoa(t), nil
		case int64:
			return strconv.FormatInt(t, 10), nil
		default:
			return "", errors.Errorf("value is not an int")
		}
	case charm.OptionFloat:
		f, ok := v.Value.(float64)
		if !ok {
			return "", errors.Errorf("value is not a float")
		}
		return fmt.Sprintf("%f", f), nil
	case charm.OptionString, charm.OptionSecret:
		s, ok := v.Value.(string)
		if !ok {
			return "", errors.Errorf("value is not a string")
		}
		return s, nil
	default:
		return "", errors.Errorf("unknown config type %v", v.Type)

	}
}

func decodePlatform(channel string, os, arch sql.NullInt64) (application.Platform, error) {
	osType, err := decodeOSType(os)
	if err != nil {
		return application.Platform{}, errors.Errorf("decoding os type: %w", err)
	}

	archType, err := decodeArchitecture(arch)
	if err != nil {
		return application.Platform{}, errors.Errorf("decoding architecture: %w", err)
	}

	return application.Platform{
		Channel:      channel,
		OSType:       osType,
		Architecture: archType,
	}, nil
}

func decodeChannel(track string, risk sql.NullString, branch string) (*application.Channel, error) {
	if !risk.Valid {
		return nil, nil
	}

	riskType, err := decodeRisk(risk.String)
	if err != nil {
		return nil, errors.Errorf("decoding risk: %w", err)
	}

	return &application.Channel{
		Track:  track,
		Risk:   riskType,
		Branch: branch,
	}, nil
}
