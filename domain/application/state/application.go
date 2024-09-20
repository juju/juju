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
	jujudb "github.com/juju/juju/internal/database"
	internalerrors "github.com/juju/juju/internal/errors"
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

	var (
		referenceName = app.Origin.ReferenceName
		revision      = app.Origin.Revision
		charmName     = app.Charm.Metadata.Name
	)

	originInfo := setCharmOrigin{
		CharmUUID: charmID.String(),
		// Set the Name on charm origin to the application name. This is
		// because the charm metadata.yaml can differ from the charm name
		// that was used to create the application.
		// This can happen if the charmhub charm name is different from the
		// charm name in the metadata.yaml. This isn't supposed to happen, but
		// it can.
		ReferenceName: referenceName,
		SourceID:      encodeCharmOriginSource(app.Origin.Source),
		Revision:      revision,
	}
	createOrigin := `INSERT INTO charm_origin (*) VALUES ($setCharmOrigin.*)`
	createOriginStmt, err := st.Prepare(createOrigin, originInfo)
	if err != nil {
		return "", errors.Trace(err)
	}

	charmPlatformInfo := charmPlatform{
		CharmUUID:      charmID.String(),
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
			// Only insert the charm if it doesn't already exist.
			// This includes the origin and platform.
			//
			// TODO (stickupkid): What happens to the charm_platform if the
			// architecture is different? We might want to record multiple
			// platforms for a charm, or just remove the charm_platform table
			// altogether. We can do a reverse lookup from the application
			// to the installed charm architecture.
			if err := st.setCharm(ctx, tx, charmID, app.Charm, ""); err != nil {
				return errors.Annotate(err, "setting charm")
			}
			if err := tx.Query(ctx, createOriginStmt, originInfo).Run(); err != nil {
				return errors.Annotatef(err, "creating origin row for application %q", name)
			}
			if err := tx.Query(ctx, createCharmPlatformStmt, charmPlatformInfo).Run(); err != nil {
				return errors.Annotatef(err, "creating charm platform row for application %q", name)
			}
		}

		// If the application doesn't exist, create it.
		if err := tx.Query(ctx, createApplicationStmt, appDetails).Run(); err != nil {
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
SELECT &applicationID.uuid
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
SELECT &applicationID.*
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

// DeleteApplication deletes the specified application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
// If the application still has units, as error satisfying [applicationerrors.ApplicationHasUnits]
// is returned.
func (st *ApplicationState) DeleteApplication(ctx domain.AtomicContext, name string) error {
	err := domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
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

	appUUID, err := st.lookupApplication(ctx, tx, name)
	if err != nil {
		return errors.Trace(err)
	}
	appID.ID = appUUID.String()

	// Check that there are no units.
	var result countResult
	err = tx.Query(ctx, queryUnitsStmt, appID).Get(&result)
	if err != nil {
		return errors.Annotatef(err, "querying units for application %q", name)
	}
	if numUnits := result.Count; numUnits > 0 {
		return fmt.Errorf("cannot delete application %q as it still has %d unit(s)%w", name, numUnits, errors.Hide(applicationerrors.ApplicationHasUnits))
	}

	// TODO(units) - fix these tables to allow deletion of rows
	// Deleting resource row results in FK mismatch error,
	// foreign key mismatch - "resource" referencing "resource_meta"
	// even for empty tables and even though there's no FK
	// from resource_meta to resource.
	//
	// resource
	// resource_meta

	if err := st.deleteSimpleApplicationReferences(ctx, tx, appID.ID); err != nil {
		return errors.Annotatef(err, "deleting associated records for application %q", appName)
	}
	if err := tx.Query(ctx, deleteApplicationStmt, appName).Run(); err != nil {
		return errors.Trace(err)
	}
	return nil
}
func (st *ApplicationState) deleteSimpleApplicationReferences(ctx context.Context, tx *sqlair.TX, appID string) error {
	app := applicationID{ID: appID}

	for _, table := range []string{
		"cloud_service",
		"application_channel",
		"application_platform",
		"application_scale",
		"application_config",
		"application_constraint",
		"application_setting",
		"application_endpoint_space",
		"application_endpoint_cidr",
		"application_storage_directive",
	} {
		deleteApplicationReference := fmt.Sprintf(`DELETE FROM %s WHERE application_uuid = $applicationID.uuid`, table)
		deleteApplicationReferenceStmt, err := st.Prepare(deleteApplicationReference, app)
		if err != nil {
			return errors.Trace(err)
		}

		if err := tx.Query(ctx, deleteApplicationReferenceStmt, app).Run(); err != nil {
			return errors.Annotatef(err, "deleting reference to application in table %q", table)
		}
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
		appID, err := st.lookupApplication(ctx, tx, applicationName)
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

// GetUnitUUIDs returns the UUIDs for the named units in bulk, returning an error
// satisfying [applicationerrors.UnitNotFound] if any of the units don't exist.
func (st *ApplicationState) GetUnitUUIDs(ctx context.Context, names []string) ([]coreunit.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}
	unitNames := unitNames(names)

	query, err := st.Prepare(`
SELECT &unitNameAndUUID.*
FROM unit
WHERE name IN ($unitNames[:])
`, unitNameAndUUID{}, unitNames)
	if err != nil {
		return nil, internalerrors.Errorf("preparing query: %w", err)
	}

	uuidsAndNames := []unitNameAndUUID{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, query, unitNames).GetAll(&uuidsAndNames)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, internalerrors.Errorf("querying unit names: %w", err)
	}

	nameToUUID := make(map[string]coreunit.UUID, len(uuidsAndNames))
	for _, u := range uuidsAndNames {
		nameToUUID[u.Name] = coreunit.UUID(u.UnitUUID)
	}

	return transform.SliceOrErr(names, func(name string) (coreunit.UUID, error) {
		uuid, ok := nameToUUID[name]
		if !ok {
			return "", internalerrors.Errorf("unit %q not found%w", name, errors.Hide(applicationerrors.UnitNotFound))
		}
		return uuid, nil
	})
}

// GetUnitNames gets in bulk the names for the specified unit UUIDs, returning an error
// satisfying [applicationerrors.UnitNotFound] if any units are not found.
func (st *ApplicationState) GetUnitNames(ctx context.Context, uuids []coreunit.UUID) ([]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}
	unitUUIDs := unitUUIDs(transform.Slice(uuids, func(uuid coreunit.UUID) string { return uuid.String() }))

	query, err := st.Prepare(`
SELECT &unitNameAndUUID.*
FROM unit
WHERE uuid IN ($unitUUIDs[:])
`, unitNameAndUUID{}, unitUUIDs)
	if err != nil {
		return nil, internalerrors.Errorf("preparing query: %w", err)
	}

	uuidsAndNames := []unitNameAndUUID{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query, unitUUIDs).GetAll(&uuidsAndNames)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, internalerrors.Errorf("querying unit names: %w", err)
	}

	uuidToName := make(map[string]string, len(uuidsAndNames))
	for _, u := range uuidsAndNames {
		uuidToName[u.UnitUUID] = u.Name
	}

	return transform.SliceOrErr(uuids, func(uuid coreunit.UUID) (string, error) {
		name, ok := uuidToName[uuid.String()]
		if !ok {
			return "", internalerrors.Errorf("unit %q not found%w", uuid, errors.Hide(applicationerrors.UnitNotFound))
		}
		return name, nil
	})
}

func (st *ApplicationState) getUnit(ctx context.Context, tx *sqlair.TX, name string) (*unitDetails, error) {
	unitName := unitName{Name: name}
	getUnit := `SELECT &unitDetails.* FROM unit WHERE name = $unitName.name`
	getUnitStmt, err := st.Prepare(getUnit, unitDetails{}, unitName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var unit unitDetails
	err = tx.Query(ctx, getUnitStmt, unitName).Get(&unit)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, internalerrors.Errorf("unit %q not found%w", unitName, errors.Hide(applicationerrors.UnitNotFound))
	}
	if err != nil {
		return nil, internalerrors.Errorf("querying unit %q: %w", unitName, err)
	}
	return &unit, nil
}

// InsertUnit insert the specified application unit, returning an error
// satisfying [applicationerrors.UnitAlreadyExists]
// if the unit exists.
func (st *ApplicationState) InsertUnit(
	ctx domain.AtomicContext, appID coreapplication.ID, args application.UpsertUnitArg,
) error {
	err := domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		_, err := st.getUnit(ctx, tx, args.UnitName)
		if err == nil {
			return fmt.Errorf("unit %q already exists%w", args.UnitName, errors.Hide(applicationerrors.UnitAlreadyExists))
		}
		if err != nil && !errors.Is(err, applicationerrors.UnitNotFound) {
			return errors.Annotatef(err, "looking up unit %q", args.UnitName)
		}
		return st.insertUnit(ctx, tx, appID, args)
	})
	return errors.Annotatef(err, "inserting unit for application %q", appID)
}

func (st *ApplicationState) insertUnit(
	ctx context.Context, tx *sqlair.TX, appID coreapplication.ID, args application.UpsertUnitArg,
) error {
	unitUUID, err := coreunit.NewUUID()
	if err != nil {
		return errors.Trace(err)
	}
	nodeUUID, err := uuid.NewUUID()
	if err != nil {
		return errors.Trace(err)
	}
	createParams := unitDetails{
		ApplicationID: appID.String(),
		UnitUUID:      unitUUID.String(),
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

	createParams.Name = args.UnitName

	if err := tx.Query(ctx, createNodeStmt, createParams).Run(); err != nil {
		return errors.Annotatef(err, "creating net node row for unit %q", args.UnitName)
	}
	if err := tx.Query(ctx, createUnitStmt, createParams).Run(); err != nil {
		return errors.Annotatef(err, "creating unit row for unit %q", args.UnitName)
	}
	if args.CloudContainer != nil {
		if err := st.upsertUnitCloudContainer(ctx, tx, args.UnitName, nodeUUID.String(), args.CloudContainer); err != nil {
			return errors.Annotatef(err, "creating cloud container row for unit %q", args.UnitName)
		}
	}
	return nil
}

// UpdateUnit updates the specified application unit, returning an error
// satisfying [applicationerrors.UnitNotFoundError] if the unit doesn't exist.
func (st *ApplicationState) UpdateUnit(
	ctx domain.AtomicContext, appID coreapplication.ID, args application.UpsertUnitArg,
) error {
	err := domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		toUpdate, err := st.getUnit(ctx, tx, args.UnitName)
		if err != nil {
			return errors.Trace(err)
		}

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
		updateUnitStmt, err := st.Prepare(updateUnit, *toUpdate)
		if err != nil {
			return errors.Trace(err)
		}

		if err := tx.Query(ctx, updateUnitStmt, toUpdate).Run(); err != nil {
			return errors.Annotatef(err, "updating unit row for unit %q", toUpdate.Name)
		}
		if args.CloudContainer != nil {
			if err := st.upsertUnitCloudContainer(ctx, tx, toUpdate.Name, toUpdate.NetNodeID, args.CloudContainer); err != nil {
				return errors.Annotatef(err, "updating cloud container row for unit %q", toUpdate.Name)
			}
		}
		return nil
	})
	return errors.Annotatef(err, "updating unit %q for application %q", args.UnitName, appID)
}

func (st *ApplicationState) upsertUnitCloudContainer(
	ctx context.Context, tx *sqlair.TX, unitName, netNodeID string, cc *application.CloudContainer,
) error {
	existingContainerInfo := cloudContainer{
		NetNodeID: netNodeID,
	}
	queryCloudContainer := `
SELECT &cloudContainer.*
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

	upsertCloudContainer := `
INSERT INTO cloud_container (*) VALUES ($cloudContainer.*)
ON CONFLICT(net_node_uuid) DO UPDATE
    SET provider_id = excluded.provider_id;
`

	upsertStmt, err := st.Prepare(upsertCloudContainer, newContainerInfo)
	if err != nil {
		return errors.Trace(err)
	}

	if err := tx.Query(ctx, upsertStmt, newContainerInfo).Run(); err != nil {
		return errors.Annotatef(err, "updating cloud container for unit %q", unitName)
	}

	if cc.Address != nil {
		if err := st.upsertCloudContainerAddress(ctx, tx, unitName, netNodeID, *cc.Address); err != nil {
			return errors.Annotatef(err, "updating cloud container address for unit %q", unitName)
		}
	}
	if cc.Ports != nil {
		if err := st.upsertCloudContainerPorts(ctx, tx, netNodeID, *cc.Ports); err != nil {
			return errors.Annotatef(err, "updating cloud container ports for unit %q", unitName)
		}
	}
	return nil
}

func (st *ApplicationState) upsertCloudContainerAddress(
	ctx context.Context, tx *sqlair.TX, unitName, netNodeID string, address application.ContainerAddress,
) error {
	// First ensure the address link layer device is upserted.
	// For cloud containers, the device is a placeholder without
	// a MAC address. It just exits to tie the address to the
	// net node corresponding to the cloud container.
	cloudContainerDeviceInfo := cloudContainerDevice{
		Name:              address.Device.Name,
		NetNodeID:         netNodeID,
		DeviceTypeID:      int(address.Device.DeviceTypeID),
		VirtualPortTypeID: int(address.Device.VirtualPortTypeID),
	}

	selectCloudContainerDeviceStmt, err := st.Prepare(`
SELECT &cloudContainerDevice.uuid
FROM link_layer_device
WHERE net_node_uuid = $cloudContainerDevice.net_node_uuid
`, cloudContainerDeviceInfo)
	if err != nil {
		return errors.Trace(err)
	}

	insertCloudContainerDeviceStmt, err := st.Prepare(`
INSERT INTO link_layer_device (*) VALUES ($cloudContainerDevice.*)
`, cloudContainerDeviceInfo)
	if err != nil {
		return errors.Trace(err)
	}

	// See if the link layer device exists, if not insert it.
	err = tx.Query(ctx, selectCloudContainerDeviceStmt, cloudContainerDeviceInfo).Get(&cloudContainerDeviceInfo)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Annotatef(err, "querying cloud container link layer device for unit %q", unitName)
	}
	if errors.Is(err, sqlair.ErrNoRows) {
		deviceUUID, err := uuid.NewUUID()
		if err != nil {
			return errors.Trace(err)
		}
		cloudContainerDeviceInfo.UUID = deviceUUID.String()
		if err := tx.Query(ctx, insertCloudContainerDeviceStmt, cloudContainerDeviceInfo).Run(); err != nil {
			return errors.Annotatef(err, "inserting cloud container device for unit %q", unitName)
		}
	}
	deviceUUID := cloudContainerDeviceInfo.UUID

	// Now process the address details.
	ipAddr := ipAddress{
		Value:        address.Value,
		ConfigTypeID: int(address.ConfigType),
		TypeID:       int(address.AddressType),
		OriginID:     int(address.Origin),
		ScopeID:      int(address.Scope),
		DeviceID:     deviceUUID,
	}

	selectAddressUUIDStmt, err := st.Prepare(`
SELECT &ipAddress.uuid
FROM ip_address
WHERE device_uuid = $ipAddress.device_uuid;
`, ipAddr)
	if err != nil {
		return errors.Trace(err)
	}

	upsertAddressStmt, err := sqlair.Prepare(`
INSERT INTO ip_address (*)
VALUES ($ipAddress.*)
ON CONFLICT(uuid) DO UPDATE SET
    address_value = excluded.address_value,
    type_id = excluded.type_id,
    scope_id = excluded.scope_id,
    origin_id = excluded.origin_id,
    config_type_id = excluded.config_type_id
`, ipAddr)
	if err != nil {
		return errors.Trace(err)
	}

	// Container addresses are never deleted unless the container itself is deleted.
	// First see if there's an existing address recorded.
	err = tx.Query(ctx, selectAddressUUIDStmt, ipAddr).Get(&ipAddr)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return fmt.Errorf("querying existing cloud container address for device %q: %w", deviceUUID, err)
	}

	// Create a UUID for new addresses.
	if errors.Is(err, sqlair.ErrNoRows) {
		addrUUID, err := uuid.NewUUID()
		if err != nil {
			return errors.Trace(err)
		}
		ipAddr.AddressUUID = addrUUID.String()
	}

	// Update the address values.
	if err = tx.Query(ctx, upsertAddressStmt, ipAddr).Run(); err != nil {
		return fmt.Errorf("updating cloud container address attributes for device %q: %w", deviceUUID, err)
	}
	return nil
}

type ports []string

func (st *ApplicationState) upsertCloudContainerPorts(ctx context.Context, tx *sqlair.TX, cloudContainerID string, portValues []string) error {
	ccPort := cloudContainerPort{
		CloudContainerUUID: cloudContainerID,
	}
	deleteStmt, err := st.Prepare(`
DELETE FROM cloud_container_port
WHERE port NOT IN ($ports[:])
AND cloud_container_uuid = $cloudContainerPort.cloud_container_uuid;
`, ports{}, ccPort)
	if err != nil {
		return errors.Trace(err)
	}

	upsertStmt, err := sqlair.Prepare(`
INSERT INTO cloud_container_port (*)
VALUES ($cloudContainerPort.*)
ON CONFLICT(cloud_container_uuid, port)
DO NOTHING
`, ccPort)
	if err != nil {
		return errors.Trace(err)
	}

	if err := tx.Query(ctx, deleteStmt, ports(portValues), ccPort).Run(); err != nil {
		return fmt.Errorf("removing cloud container ports for %q: %w", cloudContainerID, err)
	}

	for _, port := range portValues {
		ccPort.Port = port
		if err := tx.Query(ctx, upsertStmt, ccPort).Run(); err != nil {
			return fmt.Errorf("updating cloud container ports for %q: %w", cloudContainerID, err)
		}
	}

	return nil
}

// DeleteUnit deletes the specified unit.
// If the unit's application is Dying and no
// other references to it exist, true is returned to
// indicate the application could be safely deleted.
// It will fail if the unit is not Dead.
func (st *ApplicationState) DeleteUnit(ctx domain.AtomicContext, unitName string) (bool, error) {
	unit := minimalUnit{Name: unitName}
	peerCountQuery := `
SELECT a.life_id as &unitCount.app_life_id, u.life_id AS &unitCount.unit_life_id, count(peer.uuid) AS &unitCount.count
FROM unit u
JOIN application a ON a.uuid = u.application_uuid
LEFT JOIN unit peer ON u.application_uuid = peer.application_uuid AND peer.uuid != u.uuid
WHERE u.name = $minimalUnit.name
`
	peerCountStmt, err := st.Prepare(peerCountQuery, unit, unitCount{})
	if err != nil {
		return false, errors.Trace(err)
	}
	canRemoveApplication := false
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
			return errors.Annotate(err, "deleting dead unit")
		}
		canRemoveApplication = count.Count == 0 && count.ApplicationLifeID != life.Alive
		return nil
	})
	return canRemoveApplication, errors.Annotatef(err, "removing unit %q", unitName)
}

func (st *ApplicationState) deleteUnit(ctx context.Context, tx *sqlair.TX, unitName string) error {

	unit := minimalUnit{Name: unitName}

	queryUnit := `SELECT &minimalUnit.* FROM unit WHERE name = $minimalUnit.name`
	queryUnitStmt, err := st.Prepare(queryUnit, unit)
	if err != nil {
		return errors.Trace(err)
	}

	deleteUnit := `DELETE FROM unit WHERE name = $minimalUnit.name`
	deleteUnitStmt, err := st.Prepare(deleteUnit, unit)
	if err != nil {
		return errors.Trace(err)
	}

	deleteNode := `
DELETE FROM net_node WHERE uuid = (
    SELECT net_node_uuid FROM unit WHERE name = $minimalUnit.name
)
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

	if err := st.deleteCloudContainer(ctx, tx, unit.NetNodeID); err != nil {
		return errors.Annotatef(err, "deleting cloud container for unit %q", unitName)
	}

	if err := st.deletePorts(ctx, tx, unit.ID); err != nil {
		return errors.Annotatef(err, "deleting port ranges for unit %q", unitName)
	}

	// TODO(units) - delete storage, annotations

	if err := st.deleteSimpleUnitReferences(ctx, tx, unit.ID); err != nil {
		return errors.Annotatef(err, "deleting associated records for unit %q", unitName)
	}

	if err := tx.Query(ctx, deleteUnitStmt, unit).Run(); err != nil {
		return errors.Annotatef(err, "deleting unit %q", unitName)
	}
	if err := tx.Query(ctx, deleteNodeStmt, unit).Run(); err != nil {
		return errors.Annotatef(err, "deleting net node for unit  %q", unitName)
	}
	return nil
}

func (st *ApplicationState) deleteCloudContainer(ctx context.Context, tx *sqlair.TX, netNodeID string) error {
	cloudContainer := cloudContainer{NetNodeID: netNodeID}

	if err := st.deleteCloudContainerPorts(ctx, tx, netNodeID); err != nil {
		return errors.Trace(err)
	}

	if err := st.deleteCloudContainerAddresses(ctx, tx, netNodeID); err != nil {
		return errors.Trace(err)
	}

	deleteCloudContainerStmt, err := st.Prepare(`
DELETE FROM cloud_container
WHERE net_node_uuid = $cloudContainer.net_node_uuid`, cloudContainer)
	if err != nil {
		return errors.Trace(err)
	}

	if err := tx.Query(ctx, deleteCloudContainerStmt, cloudContainer).Run(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (st *ApplicationState) deleteCloudContainerAddresses(ctx context.Context, tx *sqlair.TX, netNodeID string) error {
	unit := minimalUnit{
		NetNodeID: netNodeID,
	}
	deleteAddressStmt, err := st.Prepare(`
DELETE FROM ip_address
WHERE device_uuid IN (
    SELECT device_uuid FROM link_layer_device lld
    WHERE lld.net_node_uuid = $minimalUnit.net_node_uuid
)
`, unit)
	if err != nil {
		return errors.Trace(err)
	}
	deleteDeviceStmt, err := st.Prepare(`
DELETE FROM link_layer_device
WHERE net_node_uuid = $minimalUnit.net_node_uuid`, unit)
	if err != nil {
		return errors.Trace(err)
	}
	if err := tx.Query(ctx, deleteAddressStmt, unit).Run(); err != nil {
		return fmt.Errorf("removing cloud container addresses for %q: %w", netNodeID, err)
	}
	if err := tx.Query(ctx, deleteDeviceStmt, unit).Run(); err != nil {
		return fmt.Errorf("removing cloud container link layer devices for %q: %w", netNodeID, err)
	}
	return nil
}

func (st *ApplicationState) deleteCloudContainerPorts(ctx context.Context, tx *sqlair.TX, netNodeID string) error {
	unit := minimalUnit{
		NetNodeID: netNodeID,
	}
	deleteStmt, err := st.Prepare(`
DELETE FROM cloud_container_port
WHERE cloud_container_uuid = $minimalUnit.net_node_uuid
`, unit)
	if err != nil {
		return errors.Trace(err)
	}
	if err := tx.Query(ctx, deleteStmt, unit).Run(); err != nil {
		return fmt.Errorf("removing cloud container ports for %q: %w", netNodeID, err)
	}
	return nil
}

func (st *ApplicationState) deletePorts(ctx context.Context, tx *sqlair.TX, unitID string) error {
	unit := minimalUnit{ID: unitID}

	deletePortRange := `
DELETE FROM port_range
WHERE unit_endpoint_uuid IN (
    SELECT uuid FROM unit_endpoint ue
    WHERE ue.unit_uuid = $minimalUnit.uuid
)
`
	deletePortRangeStmt, err := st.Prepare(deletePortRange, unit)
	if err != nil {
		return errors.Trace(err)
	}

	if err := tx.Query(ctx, deletePortRangeStmt, unit).Run(); err != nil {
		return errors.Annotate(err, "cannot delete port range records")
	}

	deleteEndpoint := `DELETE FROM unit_endpoint WHERE unit_uuid = $minimalUnit.uuid`
	deleteEndpointStmt, err := st.Prepare(deleteEndpoint, unit)
	if err != nil {
		return errors.Annotate(err, "cannot delete endpoint records")
	}

	if err := tx.Query(ctx, deleteEndpointStmt, unit).Run(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (st *ApplicationState) deleteSimpleUnitReferences(ctx context.Context, tx *sqlair.TX, unitID string) error {
	unit := minimalUnit{ID: unitID}

	for _, table := range []string{
		"unit_agent",
		"unit_state",
		"unit_state_charm",
		"unit_state_relation",
		"unit_agent_status_data",
		"unit_agent_status",
		"unit_workload_status_data",
		"unit_workload_status",
		"cloud_container_status_data",
		"cloud_container_status",
	} {
		deleteUnitReference := fmt.Sprintf(`DELETE FROM %s WHERE unit_uuid = $minimalUnit.uuid`, table)
		deleteUnitReferenceStmt, err := st.Prepare(deleteUnitReference, unit)
		if err != nil {
			return errors.Trace(err)
		}

		if err := tx.Query(ctx, deleteUnitReferenceStmt, unit).Run(); err != nil {
			return errors.Annotatef(err, "deleting reference to unit in table %q", table)
		}
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
		appID, err = st.lookupApplication(ctx, tx, name)
		if err != nil {
			return fmt.Errorf("looking up application %q: %w", name, err)
		}
		return nil
	})
	return appID, errors.Annotatef(err, "getting ID for %q", name)
}

// GetUnitLife looks up the life of the specified unit, returning an error
// satisfying [applicationerrors.UnitNotFound] if the unit is not found.
func (st *ApplicationState) GetUnitLife(ctx domain.AtomicContext, unitName string) (life.Life, error) {
	unit := minimalUnit{Name: unitName}
	queryUnit := `
SELECT &minimalUnit.life_id
FROM unit
WHERE name = $minimalUnit.name
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
	unit := minimalUnit{Name: unitName, LifeID: l}
	query := `
SELECT &minimalUnit.uuid
FROM unit
WHERE name = $minimalUnit.name
`
	stmt, err := st.Prepare(query, unit)
	if err != nil {
		return errors.Trace(err)
	}

	updateLifeQuery := `
UPDATE unit
SET life_id = $minimalUnit.life_id
WHERE name = $minimalUnit.name
-- we ensure the life can never go backwards.
AND life_id < $minimalUnit.life_id
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
SELECT &applicationScale.*
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
SELECT &applicationID.*
FROM application a
WHERE name = $applicationName.name
`
	stmt, err := st.Prepare(query, app, applicationID{})
	if err != nil {
		return "", -1, errors.Trace(err)
	}

	var appInfo applicationID
	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, app).Get(&appInfo); err != nil {
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
		appID, err := st.lookupApplication(ctx, tx, name)
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

type statusKeys []string

// saveStatusData saves the status key value data for the specified unit in the specified table.
// It's called from each different SaveStatus method which previously has confirmed the unit UUID exists.
func (st *ApplicationState) saveStatusData(ctx context.Context, tx *sqlair.TX, table string, unitUUID coreunit.UUID, data map[string]string) error {
	unit := minimalUnit{ID: unitUUID.String()}
	var keys statusKeys
	for k := range data {
		keys = append(keys, k)
	}

	deleteStmt, err := st.Prepare(fmt.Sprintf(`
DELETE FROM %s
WHERE key NOT IN ($statusKeys[:])
AND unit_uuid = $minimalUnit.uuid;
`, table), keys, unit)
	if err != nil {
		return errors.Trace(err)
	}

	statusData := unitStatusData{UnitUUID: unitUUID.String()}
	upsertStmt, err := sqlair.Prepare(fmt.Sprintf(`
INSERT INTO %s (*)
VALUES ($unitStatusData.*)
ON CONFLICT(unit_uuid, key) DO UPDATE SET
    data = excluded.data;
`, table), statusData)
	if err != nil {
		return errors.Trace(err)
	}

	if err := tx.Query(ctx, deleteStmt, keys, minimalUnit{}).Run(); err != nil {
		return fmt.Errorf("removing %q status data for %q: %w", table, unitUUID, err)
	}

	for k, v := range data {
		statusData.Key = k
		statusData.Data = v
		if err := tx.Query(ctx, upsertStmt, statusData).Run(); err != nil {
			return fmt.Errorf("updating %q status data for %q: %w", table, unitUUID, err)
		}
	}
	return nil
}

// SaveCloudContainerStatus saves the given cloud container status, overwriting any current status data.
// If returns an error satisfying [applicationerrors.UnitNotFound] if the unit doesn't exist.
func (st *ApplicationState) SaveCloudContainerStatus(ctx domain.AtomicContext, unitUUID coreunit.UUID, status application.CloudContainerStatusStatusInfo) error {
	statusInfo := unitStatusInfo{
		UnitUUID:  unitUUID.String(),
		StatusID:  int(status.StatusID),
		Message:   status.Message,
		UpdatedAt: status.Since,
	}
	stmt, err := st.Prepare(`
INSERT INTO cloud_container_status (*) VALUES ($unitStatusInfo.*)
ON CONFLICT(unit_uuid) DO UPDATE SET
    status_id = excluded.status_id,
    message = excluded.message,
    updated_at = excluded.updated_at;
`, statusInfo)
	if err != nil {
		return errors.Trace(err)
	}
	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, statusInfo).Run()
		// This is purely defensive and is not expected in practice - the unitUUID
		// is expected to be validated earlier in the atomic txn workflow.
		if jujudb.IsErrConstraintForeignKey(err) {
			return fmt.Errorf("%w: %q", applicationerrors.UnitNotFound, unitUUID)
		}
		err = st.saveStatusData(ctx, tx, "cloud_container_status_data", unitUUID, status.Data)
		return errors.Trace(err)
	})
	return errors.Annotatef(err, "saving cloud container status for unit %q", unitUUID)
}

// SaveUnitAgentStatus saves the given unit agent status, overwriting any current status data.
// If returns an error satisfying [applicationerrors.UnitNotFound] if the unit doesn't exist.
func (st *ApplicationState) SaveUnitAgentStatus(ctx domain.AtomicContext, unitUUID coreunit.UUID, status application.UnitAgentStatusInfo) error {
	statusInfo := unitStatusInfo{
		UnitUUID:  unitUUID.String(),
		StatusID:  int(status.StatusID),
		Message:   status.Message,
		UpdatedAt: status.Since,
	}
	stmt, err := st.Prepare(`
INSERT INTO unit_agent_status (*) VALUES ($unitStatusInfo.*)
ON CONFLICT(unit_uuid) DO UPDATE SET
    status_id = excluded.status_id,
    message = excluded.message,
    updated_at = excluded.updated_at;
`, statusInfo)
	if err != nil {
		return errors.Trace(err)
	}
	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, statusInfo).Run()
		// This is purely defensive and is not expected in practice - the unitUUID
		// is expected to be validated earlier in the atomic txn workflow.
		if jujudb.IsErrConstraintForeignKey(err) {
			return fmt.Errorf("%w: %q", applicationerrors.UnitNotFound, unitUUID)
		}
		err = st.saveStatusData(ctx, tx, "unit_agent_status_data", unitUUID, status.Data)
		return errors.Trace(err)
	})
	return errors.Annotatef(err, "saving unit agent status for unit %q", unitUUID)
}

// SaveUnitWorkloadStatus saves the given unit workload status, overwriting any current status data.
// If returns an error satisfying [applicationerrors.UnitNotFound] if the unit doesn't exist.
func (st *ApplicationState) SaveUnitWorkloadStatus(ctx domain.AtomicContext, unitUUID coreunit.UUID, status application.UnitWorkloadStatusInfo) error {
	statusInfo := unitStatusInfo{
		UnitUUID:  unitUUID.String(),
		StatusID:  int(status.StatusID),
		Message:   status.Message,
		UpdatedAt: status.Since,
	}
	stmt, err := st.Prepare(`
INSERT INTO unit_workload_status (*) VALUES ($unitStatusInfo.*)
ON CONFLICT(unit_uuid) DO UPDATE SET
    status_id = excluded.status_id,
    message = excluded.message,
    updated_at = excluded.updated_at;
`, statusInfo)
	if err != nil {
		return errors.Trace(err)
	}
	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, statusInfo).Run()
		// This is purely defensive and is not expected in practice - the unitUUID
		// is expected to be validated earlier in the atomic txn workflow.
		if jujudb.IsErrConstraintForeignKey(err) {
			return fmt.Errorf("%w: %q", applicationerrors.UnitNotFound, unitUUID)
		}
		err = st.saveStatusData(ctx, tx, "unit_workload_status_data", unitUUID, status.Data)
		return errors.Trace(err)
	})
	return errors.Annotatef(err, "saving unit workload status for unit %q", unitUUID)
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
			uuids[i] = r.UnitUUID
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
		result[u.UnitUUID] = u.LifeID
	}
	return result, nil
}

// GetCharmIDByApplicationName returns a charm ID by application name. It
// returns an error if the charm can not be found by the name. This can also be
// used as a cheap way to see if a charm exists without needing to load the
// charm metadata.
//
// Returns [applicationerrors.ApplicationNameNotValid] if the name is not valid,
// and [applicationerrors.CharmNotFound] if the charm is not found.
func (st *ApplicationState) GetCharmIDByApplicationName(ctx context.Context, name string) (corecharm.ID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	query, err := st.Prepare(`
SELECT &applicationCharmUUID.*
FROM application
WHERE uuid = $applicationID.uuid
	`, applicationCharmUUID{}, applicationID{})
	if err != nil {
		return "", internalerrors.Errorf("preparing query for application %q: %w", name, err)
	}

	var result charmID
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		appID, err := st.lookupApplication(ctx, tx, name)
		if err != nil {
			return internalerrors.Errorf("looking up application %q: %w", name, err)
		}

		appIdent := applicationID{ID: appID.String()}

		var charmIdent applicationCharmUUID
		if err := tx.Query(ctx, query, appIdent).Get(&charmIdent); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return internalerrors.Errorf("application %s: %w", name, applicationerrors.ApplicationNotFound)
			}
			return internalerrors.Errorf("getting charm for application %q: %w", name, err)
		}

		// If the charmUUID is empty, then something went wrong with adding an
		// application.
		if charmIdent.CharmUUID == "" {
			// Do not return a CharmNotFound error here. The application is in
			// a broken state. There isn't anything we can do to fix it here.
			// This will require manual intervention.
			return internalerrors.Errorf("application is missing charm")
		}

		// Now get the charm by the UUID, but if it doesn't exist, return an
		// error.
		chIdent := charmID{UUID: charmIdent.CharmUUID}
		err = st.checkCharmExists(ctx, tx, chIdent)
		if err != nil {
			return internalerrors.Errorf("getting charm for application %q: %w", name, err)
		}

		result = chIdent

		return nil
	}); err != nil {
		return "", errors.Trace(err)
	}

	return corecharm.ID(result.UUID), nil
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
func (st *ApplicationState) GetCharmByApplicationID(ctx context.Context, appID coreapplication.ID) (charm.Charm, charm.CharmOrigin, application.Platform, error) {
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
		return charm.Charm{}, charm.CharmOrigin{}, application.Platform{}, internalerrors.Errorf("preparing query for application %q: %w", appID, err)
	}

	var (
		ch          charm.Charm
		chOrigin    charm.CharmOrigin
		appPlatform application.Platform
	)
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		appIdent := applicationID{ID: appID.String()}

		var charmIdent applicationCharmUUID
		if err := tx.Query(ctx, query, appIdent).Get(&charmIdent); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return internalerrors.Errorf("application %s: %w", appID, applicationerrors.ApplicationNotFound)
			}
			return internalerrors.Errorf("getting charm for application %q: %w", appID, err)
		}

		// If the charmUUID is empty, then something went wrong with adding an
		// application.
		if charmIdent.CharmUUID == "" {
			// Do not return a CharmNotFound error here. The application is in
			// a broken state. There isn't anything we can do to fix it here.
			// This will require manual intervention.
			return internalerrors.Errorf("application is missing charm")
		}

		// Now get the charm by the UUID, but if it doesn't exist, return an
		// error.
		chIdent := charmID{UUID: charmIdent.CharmUUID}
		ch, err = st.getCharm(ctx, tx, chIdent)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return internalerrors.Errorf("application %s: %w", appID, applicationerrors.CharmNotFound)
			}
			return internalerrors.Errorf("getting charm for application %q: %w", appID, err)
		}

		chOrigin, err = st.getCharmOrigin(ctx, tx, chIdent)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return internalerrors.Errorf("application %s: %w", appID, applicationerrors.CharmNotFound)
			}
			return internalerrors.Errorf("getting charm origin for application %q: %w", appID, err)
		}

		appPlatform, err = st.getPlatform(ctx, tx, appIdent)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return internalerrors.Errorf("application %s: %w", appID, applicationerrors.InvalidApplicationState)
			}
			return internalerrors.Errorf("getting charm platform for application %q: %w", appID, err)
		}
		return nil
	}); err != nil {
		return ch, chOrigin, appPlatform, errors.Trace(err)
	}

	return ch, chOrigin, appPlatform, nil
}

// getPlatform returns the application platform for the given charm ID.
func (st *ApplicationState) getPlatform(ctx context.Context, tx *sqlair.TX, ident applicationID) (application.Platform, error) {
	query := `
SELECT &applicationPlatform.*
FROM application_platform
WHERE application_uuid = $applicationID.uuid;
`

	stmt, err := st.Prepare(query, applicationPlatform{}, ident)
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
