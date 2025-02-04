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
	"github.com/juju/collections/transform"
	jujuerrors "github.com/juju/errors"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/ipaddress"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/linklayerdevice"
	modelerrors "github.com/juju/juju/domain/model/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	storagestate "github.com/juju/juju/domain/storage/state"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	internalerrors "github.com/juju/juju/internal/errors"
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
		if err := st.insertResources(ctx, tx, insertResourcesArgs{
			appID:        appDetails.UUID,
			charmUUID:    appDetails.CharmID,
			charmSource:  args.Charm.Source,
			appResources: args.Resources,
		}); err != nil {
			return errors.Errorf("inserting resources for application %q: %w", name, err)
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

		// The channel is optional for local charms. Although, it would be
		// nice to have a channel for local charms, it's not a requirement.
		if createChannelStmt != nil {
			if err := tx.Query(ctx, createChannelStmt, channelInfo).Run(); err != nil {
				return errors.Errorf("inserting channel row for application %q: %w", name, err)
			}
		}

		for _, unit := range units {
			insertArg := application.InsertUnitArg{
				UnitName: unit.UnitName,
				UnitStatusArg: application.UnitStatusArg{
					AgentStatus:    unit.UnitStatusArg.AgentStatus,
					WorkloadStatus: unit.UnitStatusArg.WorkloadStatus,
				},
			}
			if _, err := st.insertUnit(ctx, tx, appUUID, insertArg); err != nil {
				return errors.Errorf("adding unit for application %q: %w", appUUID, err)
			}
		}

		return nil
	})
	if err != nil {
		return "", errors.Errorf("creating application %q: %w", name, err)
	}
	return appUUID, nil
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
    FROM cloud_service
    WHERE application_uuid = $applicationDetails.uuid
)`, app)
	if err != nil {
		return jujuerrors.Trace(err)
	}

	deleteCloudServiceStmt, err := st.Prepare(`
DELETE FROM cloud_service
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
		"application_endpoint_space",
		"application_endpoint_cidr",
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

// AddUnits adds the specified units to the application.
func (st *State) AddUnits(ctx context.Context, appUUID coreapplication.ID, args ...application.AddUnitArg) error {
	if len(args) == 0 {
		return nil
	}

	db, err := st.DB()
	if err != nil {
		return jujuerrors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for _, arg := range args {
			insertArg := application.InsertUnitArg{
				UnitName: arg.UnitName,
				UnitStatusArg: application.UnitStatusArg{
					AgentStatus:    arg.UnitStatusArg.AgentStatus,
					WorkloadStatus: arg.UnitStatusArg.WorkloadStatus,
				},
			}
			if _, err := st.insertUnit(ctx, tx, appUUID, insertArg); err != nil {
				return errors.Errorf("adding unit for application %q: %w", appUUID, err)
			}
		}
		return nil
	})
	if err != nil {
		return errors.Errorf("adding units to application %q: %w", appUUID, err)
	}
	return nil
}

// GetUnitUUID returns the UUID for the named unit, returning an error
// satisfying [applicationerrors.UnitNotFound] if the unit doesn't exist.
func (st *State) GetUnitUUID(ctx domain.AtomicContext, unitName coreunit.Name) (coreunit.UUID, error) {
	var unitUUID coreunit.UUID
	err := domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		unitUUID, err = st.getUnitUUID(ctx, tx, unitName)
		return err
	})
	if err != nil {
		return "", errors.Errorf("getting unit UUID for unit %q: %w", unitName, err)
	}
	return unitUUID, nil
}

func (st *State) getUnitUUID(ctx context.Context, tx *sqlair.TX, unitName coreunit.Name) (coreunit.UUID, error) {
	unit := unitDetails{Name: unitName}
	getUnitStmt, err := st.Prepare(`SELECT &unitDetails.uuid FROM unit WHERE name = $unitDetails.name`, unit)
	if err != nil {
		return "", jujuerrors.Trace(err)
	}

	err = tx.Query(ctx, getUnitStmt, unit).Get(&unit)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("unit %q not found%w", unitName, jujuerrors.Hide(applicationerrors.UnitNotFound))
	}
	if err != nil {
		return "", errors.Errorf("querying unit %q: %w", unitName, err)
	}
	return unit.UnitUUID, nil
}

// GetUnitUUIDByName returns the UUID for the named unit, returning an error
// satisfying [applicationerrors.UnitNotFound] if the unit doesn't exist.
func (st *State) GetUnitUUIDByName(ctx context.Context, name coreunit.Name) (coreunit.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", jujuerrors.Trace(err)
	}
	unitName := unitName{Name: name}

	query, err := st.Prepare(`
SELECT &unitUUID.*
FROM unit
WHERE name = $unitName.name
`, unitUUID{}, unitName)
	if err != nil {
		return "", errors.Errorf("preparing query: %w", err)
	}

	unitUUID := unitUUID{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, query, unitName).Get(&unitUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("unit %q not found%w", name, jujuerrors.Hide(applicationerrors.UnitNotFound))
		}
		return err
	})
	if err != nil {
		return "", errors.Errorf("querying unit name: %w", err)
	}

	return unitUUID.UnitUUID, nil
}

func (st *State) getUnit(ctx context.Context, tx *sqlair.TX, unitName coreunit.Name) (*unitDetails, error) {
	unit := unitDetails{Name: unitName}
	getUnit := `SELECT &unitDetails.* FROM unit WHERE name = $unitDetails.name`
	getUnitStmt, err := st.Prepare(getUnit, unit)
	if err != nil {
		return nil, jujuerrors.Trace(err)
	}
	err = tx.Query(ctx, getUnitStmt, unit).Get(&unit)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("unit %q not found%w", unitName, jujuerrors.Hide(applicationerrors.UnitNotFound))
	} else if err != nil {
		return nil, jujuerrors.Trace(err)
	}
	return &unit, nil
}

// SetUnitPassword updates the password for the specified unit UUID.
func (st *State) SetUnitPassword(ctx domain.AtomicContext, unitUUID coreunit.UUID, password application.PasswordInfo) error {
	return domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.setUnitPassword(ctx, tx, unitUUID, password)
	})
}

func (st *State) setUnitPassword(ctx context.Context, tx *sqlair.TX, unitUUID coreunit.UUID, password application.PasswordInfo) error {
	info := unitPassword{
		UnitUUID:                unitUUID,
		PasswordHash:            password.PasswordHash,
		PasswordHashAlgorithmID: password.HashAlgorithm,
	}
	updatePasswordStmt, err := st.Prepare(`
UPDATE unit SET
    password_hash = $unitPassword.password_hash,
    password_hash_algorithm_id = $unitPassword.password_hash_algorithm_id
WHERE uuid = $unitPassword.uuid
`, info)
	if err != nil {
		return jujuerrors.Trace(err)
	}

	err = tx.Query(ctx, updatePasswordStmt, info).Run()
	if err != nil {
		return errors.Errorf("updating password for unit %q: %w", unitUUID, err)
	}
	return nil
}

func makeCloudContainerArg(unitName coreunit.Name, cloudContainer application.CloudContainerParams) *application.CloudContainer {
	result := &application.CloudContainer{
		ProviderId: cloudContainer.ProviderId,
		Ports:      cloudContainer.Ports,
	}
	if cloudContainer.Address != nil {
		// TODO(units) - handle the cloudContainer.Address space ID
		// For k8s we'll initially create a /32 subnet off the container address
		// and add that to the default space.
		result.Address = &application.ContainerAddress{
			// For cloud containers, the device is a placeholder without
			// a MAC address and once inserted, not updated. It just exists
			// to tie the address to the net node corresponding to the
			// cloud container.
			Device: application.ContainerDevice{
				Name:              fmt.Sprintf("placeholder for %q cloud container", unitName),
				DeviceTypeID:      linklayerdevice.DeviceTypeUnknown,
				VirtualPortTypeID: linklayerdevice.NonVirtualPortType,
			},
			Value:       cloudContainer.Address.Value,
			AddressType: ipaddress.MarshallAddressType(cloudContainer.Address.AddressType()),
			Scope:       ipaddress.MarshallScope(cloudContainer.Address.Scope),
			Origin:      ipaddress.MarshallOrigin(network.OriginProvider),
			ConfigType:  ipaddress.MarshallConfigType(network.ConfigDHCP),
		}
		if cloudContainer.AddressOrigin != nil {
			result.Address.Origin = ipaddress.MarshallOrigin(*cloudContainer.AddressOrigin)
		}
	}
	return result
}

// InsertCAASUnit inserts the specified CAAS application unit, returning an
// error satisfying [applicationerrors.UnitAlreadyExists] if the unit exists,
// or [applicationerrors.UnitNotAssigned] if the unit was not assigned.
func (st *State) InsertCAASUnit(ctx context.Context, appUUID coreapplication.ID, arg application.RegisterCAASUnitArg) error {
	db, err := st.DB()
	if err != nil {
		return jujuerrors.Trace(err)
	}

	cloudContainerParams := application.CloudContainerParams{
		ProviderId: arg.ProviderId,
		Ports:      arg.Ports,
	}
	if arg.Address != nil {
		addr := network.NewSpaceAddress(*arg.Address, network.WithScope(network.ScopeMachineLocal))
		cloudContainerParams.Address = &addr
		origin := network.OriginProvider
		cloudContainerParams.AddressOrigin = &origin
	}
	cloudContainer := makeCloudContainerArg(arg.UnitName, cloudContainerParams)

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		unitLife, err := st.getUnitLife(ctx, tx, arg.UnitName)
		if errors.Is(err, applicationerrors.UnitNotFound) {
			return st.insertCAASUnit(ctx, tx, appUUID, arg, cloudContainer)
		} else if err != nil {
			return errors.Errorf("checking unit life %q: %w", arg.UnitName, err)
		}
		if unitLife == life.Dead {
			return errors.Errorf("dead unit %q already exists%w", arg.UnitName, jujuerrors.Hide(applicationerrors.UnitAlreadyExists))
		}

		// Unit already exists and is not dead. Update the cloud container.
		toUpdate, err := st.getUnit(ctx, tx, arg.UnitName)
		if err != nil {
			return jujuerrors.Trace(err)
		}
		err = st.upsertUnitCloudContainer(ctx, tx, toUpdate.Name, toUpdate.UnitUUID, toUpdate.NetNodeID, cloudContainer)
		if err != nil {
			return errors.Errorf("updating cloud container for unit %q: %w", arg.UnitName, err)
		}

		err = st.setUnitPassword(ctx, tx, toUpdate.UnitUUID, application.PasswordInfo{
			PasswordHash:  arg.PasswordHash,
			HashAlgorithm: application.HashAlgorithmSHA256,
		})
		if err != nil {
			return errors.Errorf("setting password for unit %q: %w", arg.UnitName, err)
		}
		return nil
	})
	if err != nil {
		return jujuerrors.Trace(err)
	}
	return nil
}

func (st *State) insertCAASUnit(
	ctx context.Context,
	tx *sqlair.TX,
	appID coreapplication.ID,
	arg application.RegisterCAASUnitArg,
	cloudContainer *application.CloudContainer,
) error {
	appScale, err := st.getApplicationScaleState(ctx, tx, appID)
	if err != nil {
		return errors.Errorf("getting application scale state for app %q: %w", appID, err)
	}
	if arg.OrderedId >= appScale.Scale ||
		(appScale.Scaling && arg.OrderedId >= appScale.ScaleTarget) {
		return fmt.Errorf("unrequired unit %s is not assigned%w", arg.UnitName, jujuerrors.Hide(applicationerrors.UnitNotAssigned))
	}

	insertArg := application.InsertUnitArg{
		UnitName: arg.UnitName,
		Password: &application.PasswordInfo{
			PasswordHash:  arg.PasswordHash,
			HashAlgorithm: application.HashAlgorithmSHA256,
		},
		CloudContainer: cloudContainer,
	}
	st.addNewUnitStatusToArg(&insertArg.UnitStatusArg, coremodel.CAAS)

	_, err = st.insertUnit(ctx, tx, appID, insertArg)
	if err != nil {
		return errors.Errorf("inserting unit for CAAS application %q: %w", appID, err)
	}
	return nil
}

func (s *State) addNewUnitStatusToArg(arg *application.UnitStatusArg, modelType coremodel.ModelType) {
	now := s.clock.Now()
	arg.AgentStatus = application.UnitAgentStatusInfo{
		StatusID: application.UnitAgentStatusAllocating,
		StatusInfo: application.StatusInfo{
			Since: now,
		},
	}
	arg.WorkloadStatus = application.UnitWorkloadStatusInfo{
		StatusID: application.UnitWorkloadStatusWaiting,
		StatusInfo: application.StatusInfo{
			Message: corestatus.MessageInstallingAgent,
			Since:   now,
		},
	}
	if modelType == coremodel.IAAS {
		arg.WorkloadStatus.Message = corestatus.MessageWaitForMachine
	}
}

// InsertUnit insert the specified application unit, returning an error
// satisfying [applicationerrors.UnitAlreadyExists] if the unit exists.
func (st *State) InsertUnit(
	ctx context.Context, appUUID coreapplication.ID, args application.InsertUnitArg,
) error {
	db, err := st.DB()
	if err != nil {
		return jujuerrors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		_, err := st.getUnit(ctx, tx, args.UnitName)
		if err == nil {
			return errors.Errorf("unit %q already exists%w", args.UnitName, jujuerrors.Hide(applicationerrors.UnitAlreadyExists))
		}
		if !errors.Is(err, applicationerrors.UnitNotFound) {
			return errors.Errorf("looking up unit %q: %w", args.UnitName, err)
		}

		_, err = st.insertUnit(ctx, tx, appUUID, args)
		if err != nil {
			return errors.Errorf("inserting unit for application %q: %w", appUUID, err)
		}
		return nil
	})
	if err != nil {
		return jujuerrors.Trace(err)
	}
	return nil
}

func (st *State) insertUnit(
	ctx context.Context, tx *sqlair.TX, appUUID coreapplication.ID, args application.InsertUnitArg,
) (string, error) {
	unitUUID, err := coreunit.NewUUID()
	if err != nil {
		return "", jujuerrors.Trace(err)
	}
	nodeUUID, err := uuid.NewUUID()
	if err != nil {
		return "", jujuerrors.Trace(err)
	}
	createParams := unitDetails{
		ApplicationID: appUUID,
		UnitUUID:      unitUUID,
		NetNodeID:     nodeUUID.String(),
		LifeID:        life.Alive,
	}
	if args.Password != nil {
		createParams.PasswordHash = args.Password.PasswordHash
		createParams.PasswordHashAlgorithmID = args.Password.HashAlgorithm
	}

	createUnit := `INSERT INTO unit (*) VALUES ($unitDetails.*)`
	createUnitStmt, err := st.Prepare(createUnit, createParams)
	if err != nil {
		return "", jujuerrors.Trace(err)
	}

	createNode := `INSERT INTO net_node (uuid) VALUES ($unitDetails.net_node_uuid)`
	createNodeStmt, err := st.Prepare(createNode, createParams)
	if err != nil {
		return "", jujuerrors.Trace(err)
	}

	createParams.Name = args.UnitName

	if err := tx.Query(ctx, createNodeStmt, createParams).Run(); err != nil {
		return "", errors.Errorf("creating net node for unit %q: %w", args.UnitName, err)
	}
	if err := tx.Query(ctx, createUnitStmt, createParams).Run(); err != nil {
		return "", errors.Errorf("creating unit for unit %q: %w", args.UnitName, err)
	}
	if args.CloudContainer != nil {
		if err := st.upsertUnitCloudContainer(ctx, tx, args.UnitName, unitUUID, nodeUUID.String(), args.CloudContainer); err != nil {
			return "", errors.Errorf("creating cloud container for unit %q: %w", args.UnitName, err)
		}
	}

	if err := st.setUnitAgentStatus(ctx, tx, unitUUID, args.AgentStatus); err != nil {
		return "", errors.Errorf("saving agent status for unit %q: %w", args.UnitName, err)
	}
	if err := st.setUnitWorkloadStatus(ctx, tx, unitUUID, args.WorkloadStatus); err != nil {
		return "", errors.Errorf("saving workload status for unit %q: %w", args.UnitName, err)
	}
	return unitUUID.String(), nil
}

// UpdateUnitContainer updates the cloud container for specified unit,
// returning an error satisfying [applicationerrors.UnitNotFoundError]
// if the unit doesn't exist.
func (st *State) UpdateUnitContainer(
	ctx domain.AtomicContext, unitName coreunit.Name, container *application.CloudContainer,
) error {
	err := domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		toUpdate, err := st.getUnit(ctx, tx, unitName)
		if err != nil {
			return jujuerrors.Trace(err)
		}
		return st.upsertUnitCloudContainer(ctx, tx, toUpdate.Name, toUpdate.UnitUUID, toUpdate.NetNodeID, container)
	})
	if err != nil {
		return errors.Errorf("updating cloud container unit %q: %w", unitName, err)
	}
	return nil
}

func (st *State) upsertUnitCloudContainer(
	ctx context.Context, tx *sqlair.TX, unitName coreunit.Name, unitUUID coreunit.UUID, netNodeUUID string, cc *application.CloudContainer,
) error {
	containerInfo := cloudContainer{
		UnitUUID:   unitUUID,
		ProviderID: cc.ProviderId,
	}

	queryStmt, err := st.Prepare(`
SELECT &cloudContainer.*
FROM cloud_container
WHERE unit_uuid = $cloudContainer.unit_uuid
`, containerInfo)
	if err != nil {
		return jujuerrors.Trace(err)
	}

	insertStmt, err := st.Prepare(`
INSERT INTO cloud_container (*) VALUES ($cloudContainer.*)
`, containerInfo)
	if err != nil {
		return jujuerrors.Trace(err)
	}

	updateStmt, err := st.Prepare(`
UPDATE cloud_container SET
    provider_id = $cloudContainer.provider_id
WHERE unit_uuid = $cloudContainer.unit_uuid
`, containerInfo)
	if err != nil {
		return jujuerrors.Trace(err)
	}

	err = tx.Query(ctx, queryStmt, containerInfo).Get(&containerInfo)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("looking up cloud container %q: %w", unitName, err)
	}
	if err == nil {
		newProviderId := cc.ProviderId
		if newProviderId != "" &&
			containerInfo.ProviderID != newProviderId {
			st.logger.Debugf(context.TODO(), "unit %q has provider id %q which changed to %q",
				unitName, containerInfo.ProviderID, newProviderId)
		}
		containerInfo.ProviderID = newProviderId
		if err := tx.Query(ctx, updateStmt, containerInfo).Run(); err != nil {
			return errors.Errorf("updating cloud container for unit %q: %w", unitName, err)
		}
	} else {
		if err := tx.Query(ctx, insertStmt, containerInfo).Run(); err != nil {
			return errors.Errorf("inserting cloud container for unit %q: %w", unitName, err)
		}
	}

	if cc.Address != nil {
		if err := st.upsertCloudContainerAddress(ctx, tx, unitName, netNodeUUID, *cc.Address); err != nil {
			return errors.Errorf("updating cloud container address for unit %q: %w", unitName, err)
		}
	}
	if cc.Ports != nil {
		if err := st.upsertCloudContainerPorts(ctx, tx, unitUUID, *cc.Ports); err != nil {
			return errors.Errorf("updating cloud container ports for unit %q: %w", unitName, err)
		}
	}
	return nil
}

func (st *State) upsertCloudContainerAddress(
	ctx context.Context, tx *sqlair.TX, unitName coreunit.Name, netNodeID string, address application.ContainerAddress,
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
		return jujuerrors.Trace(err)
	}

	insertCloudContainerDeviceStmt, err := st.Prepare(`
INSERT INTO link_layer_device (*) VALUES ($cloudContainerDevice.*)
`, cloudContainerDeviceInfo)
	if err != nil {
		return jujuerrors.Trace(err)
	}

	// See if the link layer device exists, if not insert it.
	err = tx.Query(ctx, selectCloudContainerDeviceStmt, cloudContainerDeviceInfo).Get(&cloudContainerDeviceInfo)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("querying cloud container link layer device for unit %q: %w", unitName, err)
	}
	if errors.Is(err, sqlair.ErrNoRows) {
		deviceUUID, err := uuid.NewUUID()
		if err != nil {
			return jujuerrors.Trace(err)
		}
		cloudContainerDeviceInfo.UUID = deviceUUID.String()
		if err := tx.Query(ctx, insertCloudContainerDeviceStmt, cloudContainerDeviceInfo).Run(); err != nil {
			return errors.Errorf("inserting cloud container device for unit %q: %w", unitName, err)
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
		return jujuerrors.Trace(err)
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
		return jujuerrors.Trace(err)
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
			return jujuerrors.Trace(err)
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

func (st *State) upsertCloudContainerPorts(ctx context.Context, tx *sqlair.TX, unitUUID coreunit.UUID, portValues []string) error {
	ccPort := cloudContainerPort{
		UnitUUID: unitUUID,
	}
	deleteStmt, err := st.Prepare(`
DELETE FROM cloud_container_port
WHERE port NOT IN ($ports[:])
AND unit_uuid = $cloudContainerPort.unit_uuid;
`, ports{}, ccPort)
	if err != nil {
		return jujuerrors.Trace(err)
	}

	upsertStmt, err := sqlair.Prepare(`
INSERT INTO cloud_container_port (*)
VALUES ($cloudContainerPort.*)
ON CONFLICT(unit_uuid, port)
DO NOTHING
`, ccPort)
	if err != nil {
		return jujuerrors.Trace(err)
	}

	if err := tx.Query(ctx, deleteStmt, ports(portValues), ccPort).Run(); err != nil {
		return fmt.Errorf("removing cloud container ports for %q: %w", unitUUID, err)
	}

	for _, port := range portValues {
		ccPort.Port = port
		if err := tx.Query(ctx, upsertStmt, ccPort).Run(); err != nil {
			return fmt.Errorf("updating cloud container ports for %q: %w", unitUUID, err)
		}
	}

	return nil
}

// DeleteUnit deletes the specified unit.
// If the unit's application is Dying and no
// other references to it exist, true is returned to
// indicate the application could be safely deleted.
// It will fail if the unit is not Dead.
func (st *State) DeleteUnit(ctx context.Context, unitName coreunit.Name) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, jujuerrors.Trace(err)
	}

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
		return false, jujuerrors.Trace(err)
	}
	canRemoveApplication := false
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.setUnitLife(ctx, tx, unitName, life.Dead)
		if err != nil {
			return errors.Errorf("setting unit %q to Dead: %w", unitName, err)
		}
		// Count the number of units besides this one
		// belonging to the same application.
		var count unitCount
		err = tx.Query(ctx, peerCountStmt, unit).Get(&count)
		if errors.Is(err, sqlair.ErrNoRows) {
			return fmt.Errorf("unit %q not found%w", unitName, jujuerrors.Hide(applicationerrors.UnitNotFound))
		}
		if err != nil {
			return errors.Errorf("querying peer count for unit %q: %w", unitName, err)
		}
		// This should never happen since this method is called by the service
		// after setting the unit to Dead. But we check anyway.
		// There's no need for a typed error.
		if count.UnitLifeID != life.Dead {
			return fmt.Errorf("unit %q is not dead, life is %v", unitName, count.UnitLifeID)
		}

		err = st.deleteUnit(ctx, tx, unitName)
		if err != nil {
			return errors.Errorf("deleting dead unit: %w", err)
		}
		canRemoveApplication = count.Count == 0 && count.ApplicationLifeID != life.Alive
		return nil
	})
	if err != nil {
		return false, errors.Errorf("removing unit %q: %w", unitName, err)
	}
	return canRemoveApplication, nil
}

func (st *State) deleteUnit(ctx context.Context, tx *sqlair.TX, unitName coreunit.Name) error {

	unit := minimalUnit{Name: unitName}

	queryUnit := `SELECT &minimalUnit.* FROM unit WHERE name = $minimalUnit.name`
	queryUnitStmt, err := st.Prepare(queryUnit, unit)
	if err != nil {
		return jujuerrors.Trace(err)
	}

	// NOTE: This is a work around because teardown is not implemented yet. Ideally,
	// our workflow will mean that by the time the unit is dead and we are ready to
	// delete it, a worker will have already cleaned up all dependencies. However,
	// this is not the case yet. Remove the secret owner for the unit, leaving the
	// secret orphaned, to ensure we don't get a foreign key violation.
	deleteSecretOwner := `
DELETE FROM secret_unit_owner
WHERE unit_uuid = $minimalUnit.uuid
`
	deleteSecretOwnerStmt, err := st.Prepare(deleteSecretOwner, unit)
	if err != nil {
		return jujuerrors.Trace(err)
	}

	deleteUnit := `DELETE FROM unit WHERE name = $minimalUnit.name`
	deleteUnitStmt, err := st.Prepare(deleteUnit, unit)
	if err != nil {
		return jujuerrors.Trace(err)
	}

	deleteNode := `
DELETE FROM net_node WHERE uuid = (
    SELECT net_node_uuid FROM unit WHERE name = $minimalUnit.name
)
`
	deleteNodeStmt, err := st.Prepare(deleteNode, unit)
	if err != nil {
		return jujuerrors.Trace(err)
	}

	err = tx.Query(ctx, queryUnitStmt, unit).Get(&unit)
	if errors.Is(err, sqlair.ErrNoRows) {
		// Unit already deleted is a no op.
		return nil
	}
	if err != nil {
		return errors.Errorf("looking up UUID for unit %q: %w", unitName, err)
	}

	err = tx.Query(ctx, deleteSecretOwnerStmt, unit).Run()
	if err != nil {
		return errors.Errorf("deleting secret owner for unit %q: %w", unitName, err)
	}

	if err := st.deleteCloudContainer(ctx, tx, unit.UUID, unit.NetNodeID); err != nil {
		return errors.Errorf("deleting cloud container for unit %q: %w", unitName, err)
	}

	if err := st.deletePorts(ctx, tx, unit.UUID); err != nil {
		return errors.Errorf("deleting port ranges for unit %q: %w", unitName, err)
	}

	// TODO(units) - delete storage, annotations

	if err := st.deleteSimpleUnitReferences(ctx, tx, unit.UUID); err != nil {
		return errors.Errorf("deleting associated records for unit %q: %w", unitName, err)
	}

	if err := tx.Query(ctx, deleteUnitStmt, unit).Run(); err != nil {
		return errors.Errorf("deleting unit %q: %w", unitName, err)
	}
	if err := tx.Query(ctx, deleteNodeStmt, unit).Run(); err != nil {
		return errors.Errorf("deleting net node for unit  %q: %w", unitName, err)
	}
	return nil
}

func (st *State) deleteCloudContainer(ctx context.Context, tx *sqlair.TX, unitUUID coreunit.UUID, netNodeUUID string) error {
	cloudContainer := cloudContainer{UnitUUID: unitUUID}

	if err := st.deleteCloudContainerPorts(ctx, tx, unitUUID); err != nil {
		return jujuerrors.Trace(err)
	}

	if err := st.deleteCloudContainerAddresses(ctx, tx, netNodeUUID); err != nil {
		return jujuerrors.Trace(err)
	}

	deleteCloudContainerStmt, err := st.Prepare(`
DELETE FROM cloud_container
WHERE unit_uuid = $cloudContainer.unit_uuid`, cloudContainer)
	if err != nil {
		return jujuerrors.Trace(err)
	}

	if err := tx.Query(ctx, deleteCloudContainerStmt, cloudContainer).Run(); err != nil {
		return jujuerrors.Trace(err)
	}
	return nil
}

func (st *State) deleteCloudContainerAddresses(ctx context.Context, tx *sqlair.TX, netNodeID string) error {
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
		return jujuerrors.Trace(err)
	}
	deleteDeviceStmt, err := st.Prepare(`
DELETE FROM link_layer_device
WHERE net_node_uuid = $minimalUnit.net_node_uuid`, unit)
	if err != nil {
		return jujuerrors.Trace(err)
	}
	if err := tx.Query(ctx, deleteAddressStmt, unit).Run(); err != nil {
		return fmt.Errorf("removing cloud container addresses for %q: %w", netNodeID, err)
	}
	if err := tx.Query(ctx, deleteDeviceStmt, unit).Run(); err != nil {
		return fmt.Errorf("removing cloud container link layer devices for %q: %w", netNodeID, err)
	}
	return nil
}

func (st *State) deleteCloudContainerPorts(ctx context.Context, tx *sqlair.TX, unitUUID coreunit.UUID) error {
	cloudContainer := cloudContainer{
		UnitUUID: unitUUID,
	}
	deleteStmt, err := st.Prepare(`
DELETE FROM cloud_container_port
WHERE unit_uuid = $cloudContainer.unit_uuid`, cloudContainer)
	if err != nil {
		return jujuerrors.Trace(err)
	}
	if err := tx.Query(ctx, deleteStmt, cloudContainer).Run(); err != nil {
		return fmt.Errorf("removing cloud container ports for %q: %w", unitUUID, err)
	}
	return nil
}

func (st *State) deletePorts(ctx context.Context, tx *sqlair.TX, unitUUID coreunit.UUID) error {
	unit := minimalUnit{UUID: unitUUID}

	deletePortRange := `
DELETE FROM port_range
WHERE unit_uuid = $minimalUnit.uuid
`
	deletePortRangeStmt, err := st.Prepare(deletePortRange, unit)
	if err != nil {
		return jujuerrors.Trace(err)
	}

	if err := tx.Query(ctx, deletePortRangeStmt, unit).Run(); err != nil {
		return errors.Errorf("cannot delete port range records: %w", err)
	}

	return nil
}

func (st *State) deleteSimpleUnitReferences(ctx context.Context, tx *sqlair.TX, unitUUID coreunit.UUID) error {
	unit := minimalUnit{UUID: unitUUID}

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
			return jujuerrors.Trace(err)
		}

		if err := tx.Query(ctx, deleteUnitReferenceStmt, unit).Run(); err != nil {
			return errors.Errorf("deleting reference to unit in table %q: %w", table, err)
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
		life, err = st.getUnitLife(ctx, tx, unitName)
		return err
	})
	if err != nil {
		return 0, errors.Errorf("querying unit %q life: %w", unitName, err)
	}
	return life, nil
}

func (st *State) getUnitLife(ctx context.Context, tx *sqlair.TX, unitName coreunit.Name) (life.Life, error) {
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

// SetUnitLife sets the life of the specified unit, returning an error
// satisfying [applicationerrors.UnitNotFound] if the unit is not found.
func (st *State) SetUnitLife(ctx context.Context, unitName coreunit.Name, l life.Life) error {
	db, err := st.DB()
	if err != nil {
		return jujuerrors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.setUnitLife(ctx, tx, unitName, l)
	})
	if err != nil {
		return errors.Errorf("updating unit life for %q: %w", unitName, err)
	}
	return nil
}

// TODO(units) - check for subordinates and storage attachments
// For IAAS units, we need to do additional checks - these are still done in mongo.
// If a unit still has subordinates, return applicationerrors.UnitHasSubordinates.
// If a unit still has storage attachments, return applicationerrors.UnitHasStorageAttachments.
func (st *State) setUnitLife(ctx context.Context, tx *sqlair.TX, unitName coreunit.Name, l life.Life) error {
	unit := minimalUnit{Name: unitName, LifeID: l}
	query := `
SELECT &minimalUnit.uuid
FROM unit
WHERE name = $minimalUnit.name
`
	stmt, err := st.Prepare(query, unit)
	if err != nil {
		return jujuerrors.Trace(err)
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
		return jujuerrors.Trace(err)
	}

	err = tx.Query(ctx, stmt, unit).Get(&unit)
	if errors.Is(err, sqlair.ErrNoRows) {
		return fmt.Errorf("unit %q not found%w", unitName, jujuerrors.Hide(applicationerrors.UnitNotFound))
	} else if err != nil {
		return errors.Errorf("querying unit %q: %w", unitName, err)
	}
	return tx.Query(ctx, updateLifeStmt, unit).Run()

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

	app := applicationDetails{Name: appName}
	query := `
SELECT &applicationDetails.*
FROM application a
WHERE name = $applicationDetails.name
`
	stmt, err := st.Prepare(query, app)
	if err != nil {
		return "", -1, jujuerrors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, app).Get(&app); err != nil {
			if !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("querying life for application %q: %w", appName, err)
			}
			return fmt.Errorf("%w: %s", applicationerrors.ApplicationNotFound, appName)
		}
		return nil
	})
	return app.UUID, app.LifeID, jujuerrors.Trace(err)
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
func (st *State) SetDesiredApplicationScale(ctx domain.AtomicContext, appUUID coreapplication.ID, scale int) error {
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
	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, upsertStmt, scaleDetails).Run()
	})
	return jujuerrors.Trace(err)
}

// SetApplicationScalingState sets the scaling details for the given caas
// application Scale is optional and is only set if not nil.
func (st *State) SetApplicationScalingState(ctx domain.AtomicContext, appUUID coreapplication.ID, scale *int, targetScale int, scaling bool) error {
	scaleDetails := applicationScale{
		ApplicationID: appUUID,
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
		return jujuerrors.Trace(err)
	}
	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
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
SELECT &cloudService.* FROM cloud_service
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
INSERT INTO cloud_service (*) VALUES ($cloudService.*)
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

type statusKeys []string

// saveStatusData saves the status key value data for the specified unit in the
// specified table. It's called from each different SaveStatus method which
// previously has confirmed the unit UUID exists.
func (st *State) saveStatusData(ctx context.Context, tx *sqlair.TX, table string, unitUUID coreunit.UUID, data map[string]string) error {
	unit := minimalUnit{UUID: unitUUID}
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
		return jujuerrors.Trace(err)
	}

	statusData := unitStatusData{UnitUUID: unitUUID}
	upsertStmt, err := sqlair.Prepare(fmt.Sprintf(`
INSERT INTO %s (*)
VALUES ($unitStatusData.*)
ON CONFLICT(unit_uuid, key) DO UPDATE SET
    data = excluded.data;
`, table), statusData)
	if err != nil {
		return jujuerrors.Trace(err)
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

// SetCloudContainerStatusAtomic saves the given cloud container status, overwriting
// any current status data. If returns an error satisfying
// [applicationerrors.UnitNotFound] if the unit doesn't exist.
func (st *State) SetCloudContainerStatus(ctx domain.AtomicContext, unitUUID coreunit.UUID, status application.CloudContainerStatusStatusInfo) error {
	statusInfo := unitStatusInfo{
		UnitUUID:  unitUUID,
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
		return jujuerrors.Trace(err)
	}
	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, statusInfo).Run()
		// This is purely defensive and is not expected in practice - the
		// unitUUID is expected to be validated earlier in the atomic txn
		// workflow.
		if internaldatabase.IsErrConstraintForeignKey(err) {
			return fmt.Errorf("%w: %q", applicationerrors.UnitNotFound, unitUUID)
		}
		err = st.saveStatusData(ctx, tx, "cloud_container_status_data", unitUUID, status.Data)
		return jujuerrors.Trace(err)
	})
	if err != nil {
		return errors.Errorf("saving cloud container status for unit %q: %w", unitUUID, err)
	}
	return nil
}

// SetUnitAgentStatusAtomic saves the given unit agent status, overwriting any current
// status data. If returns an error satisfying [applicationerrors.UnitNotFound]
// if the unit doesn't exist.
//
// deprecated: RunAtomic methods are deprecated
func (st *State) SetUnitAgentStatusAtomic(ctx domain.AtomicContext, unitUUID coreunit.UUID, status application.UnitAgentStatusInfo) error {
	statusInfo := unitStatusInfo{
		UnitUUID:  unitUUID,
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
		return jujuerrors.Trace(err)
	}
	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, statusInfo).Run()
		// This is purely defensive and is not expected in practice - the unitUUID
		// is expected to be validated earlier in the atomic txn workflow.
		if internaldatabase.IsErrConstraintForeignKey(err) {
			return fmt.Errorf("%w: %q", applicationerrors.UnitNotFound, unitUUID)
		}
		err = st.saveStatusData(ctx, tx, "unit_agent_status_data", unitUUID, status.Data)
		return jujuerrors.Trace(err)
	})
	if err != nil {
		return errors.Errorf("saving unit agent status for unit %q: %w", unitUUID, err)
	}
	return nil
}

// setUnitAgentStatus saves the given unit agent status, overwriting any current
// status data. If returns an error satisfying [applicationerrors.UnitNotFound]
// if the unit doesn't exist.
func (st *State) setUnitAgentStatus(ctx context.Context, tx *sqlair.TX, unitUUID coreunit.UUID, status application.UnitAgentStatusInfo) error {
	statusInfo := unitStatusInfo{
		UnitUUID:  unitUUID,
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
		return jujuerrors.Trace(err)
	}

	err = tx.Query(ctx, stmt, statusInfo).Run()
	// This is purely defensive and is not expected in practice - the unitUUID
	// is expected to be validated earlier in the atomic txn workflow.
	if internaldatabase.IsErrConstraintForeignKey(err) {
		return fmt.Errorf("%w: %q", applicationerrors.UnitNotFound, unitUUID)
	}
	err = st.saveStatusData(ctx, tx, "unit_agent_status_data", unitUUID, status.Data)
	if err != nil {
		return errors.Errorf("saving unit agent status for unit %q: %w", unitUUID, err)
	}
	return nil
}

// SetUnitWorkloadStatusAtomic saves the given unit workload status, overwriting any
// current status data. If returns an error satisfying
// [applicationerrors.UnitNotFound] if the unit doesn't exist.
//
// deprecated: RunAtomic methods are deprecated
func (st *State) SetUnitWorkloadStatusAtomic(ctx domain.AtomicContext, unitUUID coreunit.UUID, status application.UnitWorkloadStatusInfo) error {
	statusInfo := unitStatusInfo{
		UnitUUID:  unitUUID,
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
		return jujuerrors.Trace(err)
	}
	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, statusInfo).Run()
		// This is purely defensive and is not expected in practice - the
		// unitUUID is expected to be validated earlier in the atomic txn
		// workflow.
		if internaldatabase.IsErrConstraintForeignKey(err) {
			return fmt.Errorf("%w: %q", applicationerrors.UnitNotFound, unitUUID)
		}
		err = st.saveStatusData(ctx, tx, "unit_workload_status_data", unitUUID, status.Data)
		return jujuerrors.Trace(err)
	})
	if err != nil {
		return errors.Errorf("saving unit workload status for unit %q: %w", unitUUID, err)
	}
	return nil
}

// setUnitWorkloadStatus saves the given unit workload status, overwriting any
// current status data. If returns an error satisfying
// [applicationerrors.UnitNotFound] if the unit doesn't exist.
func (st *State) setUnitWorkloadStatus(ctx context.Context, tx *sqlair.TX, unitUUID coreunit.UUID, status application.UnitWorkloadStatusInfo) error {
	statusInfo := unitStatusInfo{
		UnitUUID:  unitUUID,
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
		return jujuerrors.Trace(err)
	}

	err = tx.Query(ctx, stmt, statusInfo).Run()
	// This is purely defensive and is not expected in practice - the
	// unitUUID is expected to be validated earlier in the atomic txn
	// workflow.
	if internaldatabase.IsErrConstraintForeignKey(err) {
		return fmt.Errorf("%w: %q", applicationerrors.UnitNotFound, unitUUID)
	}
	err = st.saveStatusData(ctx, tx, "unit_workload_status_data", unitUUID, status.Data)
	if err != nil {
		return errors.Errorf("saving unit workload status for unit %q: %w", unitUUID, err)
	}
	return nil
}

// InitialWatchStatementUnitLife returns the initial namespace query for the
// application unit life watcher.
func (st *State) InitialWatchStatementUnitLife(appName string) (string, eventsource.NamespaceQuery) {
	queryFunc := func(ctx context.Context, runner database.TxnRunner) ([]string, error) {
		app := applicationName{Name: appName}
		stmt, err := st.Prepare(`
SELECT u.uuid AS &unitDetails.uuid
FROM unit u
JOIN application a ON a.uuid = u.application_uuid
WHERE a.name = $applicationName.name
`, app, unitDetails{})
		if err != nil {
			return nil, jujuerrors.Trace(err)
		}
		var result []unitDetails
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
		uuids := make([]string, len(result))
		for i, r := range result {
			uuids[i] = r.UnitUUID.String()
		}
		return uuids, nil
	}
	return "unit", queryFunc
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
			return nil, internalerrors.Errorf("querying unit IDs for %q: %w", appName, err)
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

// GetApplicationUnitLife returns the life values for the specified units of the
// given application. The supplied ids may belong to a different application;
// the application name is used to filter.
func (st *State) GetApplicationUnitLife(ctx context.Context, appName string, ids ...coreunit.UUID) (map[coreunit.UUID]life.Life, error) {
	db, err := st.DB()
	if err != nil {
		return nil, jujuerrors.Trace(err)
	}
	unitUUIDs := unitUUIDs(ids)

	lifeQuery := `
SELECT (u.uuid, u.life_id) AS (&unitDetails.*)
FROM unit u
JOIN application a ON a.uuid = u.application_uuid
WHERE u.uuid IN ($unitUUIDs[:])
AND a.name = $applicationName.name
`

	app := applicationName{Name: appName}
	lifeStmt, err := st.Prepare(lifeQuery, app, unitDetails{}, unitUUIDs)
	if err != nil {
		return nil, jujuerrors.Trace(err)
	}

	var lifes []unitDetails
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, lifeStmt, unitUUIDs, app).GetAll(&lifes)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return jujuerrors.Trace(err)
	})
	if err != nil {
		return nil, errors.Errorf("querying unit life for %q: %w", appName, err)
	}
	result := make(map[coreunit.UUID]life.Life)
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

	unit := unitNameAndUUID{Name: name}
	queryUnit := `
SELECT application_uuid AS &applicationID.uuid
FROM unit
WHERE name = $unitNameAndUUID.name
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
		if err := st.checkApplicationExists(ctx, tx, ident); err != nil {
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
		if err := st.checkApplicationExists(ctx, tx, ident); err != nil {
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
		return internalerrors.Errorf("preparing set hash query: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkApplicationExists(ctx, tx, ident); err != nil {
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
			return internalerrors.Errorf("hashing config and settings: %w", err)
		}

		if err := tx.Query(ctx, setHashStmt, applicationConfigHash{
			ApplicationUUID: ident.ID.String(),
			SHA256:          configHash,
		}).Run(); err != nil {
			return internalerrors.Errorf("setting hash: %w", err)
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
		return "", internalerrors.Capture(err)
	}

	ident := applicationID{ID: appID}

	query := `
SELECT sha256 AS &applicationConfigHash.sha256
FROM application_config_hash
WHERE application_uuid = $applicationID.uuid;
`

	stmt, err := st.Prepare(query, applicationConfigHash{}, ident)
	if err != nil {
		return "", internalerrors.Errorf("preparing query for application config hash: %w", err)
	}

	var hash applicationConfigHash
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkApplicationExists(ctx, tx, ident); err != nil {
			return internalerrors.Capture(err)
		}

		if err := tx.Query(ctx, stmt, ident).Get(&hash); errors.Is(err, sqlair.ErrNoRows) {
			return applicationerrors.ApplicationNotFound
		} else if err != nil {
			return internalerrors.Capture(err)
		}

		return nil
	}); err != nil {
		return "", internalerrors.Capture(err)
	}

	return hash.SHA256, nil
}

// lookupApplication looks up the application by name and returns the
// application.ID.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (st *State) lookupApplication(ctx context.Context, tx *sqlair.TX, name string) (coreapplication.ID, error) {
	app := applicationDetails{Name: name}
	queryApplicationStmt, err := st.Prepare(`
SELECT uuid AS &applicationDetails.uuid
FROM application
WHERE name = $applicationDetails.name
`, app)
	if err != nil {
		return "", jujuerrors.Trace(err)
	}
	err = tx.Query(ctx, queryApplicationStmt, app).Get(&app)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", fmt.Errorf("%w: %s", applicationerrors.ApplicationNotFound, name)
	} else if err != nil {
		return "", errors.Errorf("looking up UUID for application %q: %w", name, err)
	}
	return app.UUID, nil
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
		return internalerrors.Errorf("preparing insert query: %w", err)
	}

	inserts := make([]setApplicationConfig, 0, len(config))
	for k, v := range config {
		typeID, err := encodeConfigType(v.Type)
		if err != nil {
			return internalerrors.Errorf("encoding config type: %w", err)
		}

		inserts = append(inserts, setApplicationConfig{
			ApplicationUUID: appID.String(),
			Key:             k,
			Value:           v.Value,
			TypeID:          typeID,
		})
	}

	if err := tx.Query(ctx, insertStmt, inserts).Run(); err != nil {
		return internalerrors.Errorf("inserting config: %w", err)
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
		return internalerrors.Errorf("preparing insert query: %w", err)
	}

	if err := tx.Query(ctx, insertStmt, setApplicationSettings{
		ApplicationUUID: appID.String(),
		Trust:           settings.Trust,
	}).Run(); err != nil {
		return internalerrors.Errorf("inserting settings: %w", err)
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
		return internalerrors.Errorf("preparing insert query: %w", err)
	}

	if err := tx.Query(ctx, insertStmt, applicationConfigHash{
		ApplicationUUID: appID.String(),
		SHA256:          sha256,
	}).Run(); err != nil {
		return internalerrors.Errorf("inserting hash: %w", err)
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
			return "", internalerrors.Errorf("writing key %q: %w", key, err)
		}

		v, ok := config[key]
		if !ok {
			return "", internalerrors.Errorf("missing value for key %q", key)
		}

		val, err := encodeConfigValue(v)
		if err != nil {
			return "", internalerrors.Errorf("encoding value for key %q: %w", key, err)
		}
		if _, err := h.Write([]byte(val)); err != nil {
			return "", internalerrors.Errorf("writing value for key %q: %w", key, err)
		}
	}
	if _, err := h.Write([]byte(fmt.Sprintf("%t", settings.Trust))); err != nil {
		return "", internalerrors.Errorf("writing trust setting: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func encodeConfigValue(v application.ApplicationConfig) (string, error) {
	switch v.Type {
	case charm.OptionBool:
		b, ok := v.Value.(bool)
		if !ok {
			return "", internalerrors.Errorf("value is not a bool")
		}
		return strconv.FormatBool(b), nil
	case charm.OptionInt:
		switch t := v.Value.(type) {
		case int:
			return strconv.Itoa(t), nil
		case int64:
			return strconv.FormatInt(t, 10), nil
		default:
			return "", internalerrors.Errorf("value is not an int")
		}
	case charm.OptionFloat:
		f, ok := v.Value.(float64)
		if !ok {
			return "", internalerrors.Errorf("value is not a float")
		}
		return fmt.Sprintf("%f", f), nil
	case charm.OptionString, charm.OptionSecret:
		s, ok := v.Value.(string)
		if !ok {
			return "", internalerrors.Errorf("value is not a string")
		}
		return s, nil
	default:
		return "", internalerrors.Errorf("unknown config type %v", v.Type)

	}
}
