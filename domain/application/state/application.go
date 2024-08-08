// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain/application"
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
func NewApplicationState(base *commonStateBase, logger logger.Logger) *ApplicationState {
	return &ApplicationState{
		commonStateBase: base,
		logger:          logger,
	}
}

// CreateApplication creates an application, whilst inserting a charm into the
// database, returning an error satisfying [applicationerrors.ApplicationAle\readyExists]
// if the application already exists.
func (st *ApplicationState) CreateApplication(ctx context.Context, name string, app application.AddApplicationArg, units ...application.AddUnitArg) (coreapplication.ID, error) {
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
		if createChannelStmt != nil {
			if err := tx.Query(ctx, createChannelStmt, channelInfo).Run(); err != nil {
				return errors.Annotatef(err, "creating channel row for application %q", name)
			}
		}

		if len(units) == 0 {
			return nil
		}

		for _, u := range units {
			if err := st.upsertUnit(ctx, tx, name, appID, u); err != nil {
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

func (st *ApplicationState) lookupApplication(ctx context.Context, tx *sqlair.TX, name string) (coreapplication.ID, error) {
	var appID applicationID
	appName := applicationName{Name: name}
	queryApplication := `
SELECT application.uuid AS &applicationID.*
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
	return coreapplication.ID(appID.ID), nil
}

// UpsertApplicationUnit creates or updates the specified application unit, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
func (st *ApplicationState) UpsertApplicationUnit(ctx context.Context, name string, unit application.AddUnitArg) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		appID, err := st.lookupApplication(ctx, tx, name)
		if err != nil {
			return fmt.Errorf("cannot add unit %q: %w", name, err)
		}
		if err := st.upsertUnit(ctx, tx, name, appID, unit); err != nil {
			return fmt.Errorf("adding unit for application %q: %w", name, err)
		}
		return nil
	})
	return errors.Annotatef(err, "upserting application %q", name)
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
	deleteChannel := `DELETE FROM application_channel WHERE application_uuid = $applicationID.uuid`
	deleteChannelStmt, err := st.Prepare(deleteChannel, appID)
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		appUUID, err := st.lookupApplication(ctx, tx, name)
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
func (st *ApplicationState) AddUnits(ctx context.Context, applicationName string, args ...application.AddUnitArg) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		appID, err := st.lookupApplication(ctx, tx, applicationName)
		if err != nil {
			return errors.Trace(err)
		}
		for _, arg := range args {
			if err := st.upsertUnit(ctx, tx, applicationName, appID, arg); err != nil {
				return fmt.Errorf("adding unit for application %q: %w", applicationName, err)
			}
		}
		return nil
	})
	return errors.Annotatef(err, "adding units for application %q", applicationName)
}

// upsertUnit inserts of updates the specified unit..
func (st *ApplicationState) upsertUnit(
	ctx context.Context, tx *sqlair.TX, appName string, appID coreapplication.ID, args application.AddUnitArg,
) error {
	unitUUID, err := uuid.NewUUID()
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
	// TODO(units) - handle the cloud container attributes

	createUnit := `
INSERT INTO unit (*) VALUES ($unitDetails.*)
ON CONFLICT DO NOTHING
`
	createUnitStmt, err := st.Prepare(createUnit, createParams)
	if err != nil {
		return errors.Trace(err)
	}

	createNode := `
INSERT INTO net_node (uuid) VALUES ($unitDetails.net_node_uuid)
ON CONFLICT DO NOTHING
`
	createNodeStmt, err := st.Prepare(createNode, createParams)
	if err != nil {
		return errors.Trace(err)
	}

	// TODO - we are mirroring what's in mongo, hence the unit name is known.
	// In future we'll need to use a sequence to get a new unit id.
	if args.UnitName == nil {
		return fmt.Errorf("must pass unit name when adding a new unit for application %q", appName)
	}
	unitName := *args.UnitName
	createParams.Name = unitName

	if err := tx.Query(ctx, createNodeStmt, createParams).Run(); err != nil {
		return errors.Annotatef(err, "creating net node row for unit %q", unitName)
	}
	if err := tx.Query(ctx, createUnitStmt, createParams).Run(); err != nil {
		return errors.Annotatef(err, "creating unit row for unit %q", unitName)
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
