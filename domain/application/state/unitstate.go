// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"

	"github.com/juju/juju/core/database"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/internal"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/ipaddress"
	"github.com/juju/juju/domain/life"
	domainmachine "github.com/juju/juju/domain/machine"
	machinestate "github.com/juju/juju/domain/machine/state"
	domainsequence "github.com/juju/juju/domain/sequence"
	sequencestate "github.com/juju/juju/domain/sequence/state"
	"github.com/juju/juju/domain/status"
	domainstorage "github.com/juju/juju/domain/storage"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// InsertIAASUnitState represents the minium state required to insert
// an IAAS unit. Splitting this out, allows for sharing with the
// relation domain to facilitate subordinate unit creation.
type InsertIAASUnitState struct {
	*domain.StateBase
	clock  clock.Clock
	logger logger.Logger
}

// NewInsertIAASUnitState returns a new insert IAAS unit state reference.
func NewInsertIAASUnitState(factory database.TxnRunnerFactory, clock clock.Clock, logger logger.Logger) *InsertIAASUnitState {
	return &InsertIAASUnitState{
		StateBase: domain.NewStateBase(factory),
		clock:     clock,
		logger:    logger,
	}
}

// InsertIAASUnit inserts an IAAS unit into state, handling the IAAS
// specific details. It's a public method to share with the relation
// domain for subordinate applications.
func (st *InsertIAASUnitState) InsertIAASUnit(
	ctx context.Context,
	tx *sqlair.TX,
	appUUID, charmUUID string,
	args application.AddIAASUnitArg,
) (coreunit.Name, coreunit.UUID, []coremachine.Name, error) {
	unitName, err := st.newUnitName(ctx, tx, appUUID)
	if err != nil {
		return "", "", nil, errors.Errorf("getting new unit name for application %q: %w", appUUID, err)
	}

	unit, err := coreunit.NewUUID()
	if err != nil {
		return "", "", nil, errors.Capture(err)
	}
	unitUUID := unit.String()

	machineNames, err := st.placeIAASUnitMachine(ctx, tx, args)
	if err != nil {
		return "", "", nil, errors.Capture(err)
	}

	err = st.insertUnit(
		ctx, tx, appUUID, unitUUID, args.NetNodeUUID.String(), insertUnitArg{
			CharmUUID:     charmUUID,
			UnitName:      unitName,
			Constraints:   args.Constraints,
			UnitStatusArg: args.UnitStatusArg,
		},
	)
	if err != nil {
		return "", "", nil, errors.Errorf("inserting unit for application %q: %w", appUUID, err)
	}

	err = st.insertUnitStorageDirectives(
		ctx, tx, unitUUID, charmUUID, args.StorageDirectives,
	)
	if err != nil {
		return "", "", nil, errors.Errorf(
			"creating storage directives for unit %q: %w", unitName, err,
		)
	}

	_, err = st.insertUnitStorageInstances(ctx, tx, args.StorageInstances)
	if err != nil {
		return "", "", nil, errors.Errorf(
			"creating storage instances for unit %q: %w", unitName, err,
		)
	}

	err = st.insertUnitStorageAttachments(
		ctx,
		tx,
		unitUUID,
		args.StorageToAttach,
	)
	if err != nil {
		return "", "", nil, errors.Errorf(
			"creating storage attachments for unit %q: %w", unitName, err,
		)
	}

	err = st.insertUnitStorageOwnership(ctx, tx, unitUUID, args.StorageToOwn)
	if err != nil {
		return "", "", nil, errors.Errorf(
			"inserting storage ownership for unit %q: %w", unitName, err,
		)
	}

	err = st.insertMachineVolumeOwnership(ctx, tx, args.MachineUUID,
		args.VolumesToOwn)
	if err != nil {
		return "", "", nil, errors.Errorf(
			"inserting volume ownership for machine %q: %w",
			args.MachineUUID, err,
		)
	}

	err = st.insertMachineFilesystemOwnership(ctx, tx, args.MachineUUID,
		args.FilesystemsToOwn)
	if err != nil {
		return "", "", nil, errors.Errorf(
			"inserting volume ownership for machine %q: %w",
			args.MachineUUID, err,
		)
	}

	return coreunit.Name(unitName), coreunit.UUID(unitUUID), machineNames, nil
}

type insertUnitArg struct {
	CharmUUID       string
	UnitName        string
	CloudContainer  *application.CloudContainer
	Password        *application.PasswordInfo
	Constraints     constraints.Constraints
	WorkloadVersion string
	application.UnitStatusArg
}

// insertUnit inserts a unit into state. IAAS or CAAS specific details
// are handled by the callers, modulo CloudContainer functionality.
func (st *InsertIAASUnitState) insertUnit(
	ctx context.Context, tx *sqlair.TX,
	appUUID, unitUUID, netNodeUUID string,
	args insertUnitArg,
) error {
	if err := st.checkApplicationAlive(ctx, tx, appUUID); err != nil {
		return errors.Capture(err)
	}

	createParams := unitRow{
		ApplicationID: appUUID,
		UnitUUID:      unitUUID,
		CharmUUID:     args.CharmUUID,
		Name:          args.UnitName,
		NetNodeID:     netNodeUUID,
		LifeID:        life.Alive,
	}
	if args.Password != nil {
		// Unit passwords are optional when we insert a unit (they're mainly
		// used for CAAS units). If they are set they must be unique across
		// all units.
		createParams.PasswordHash = sql.NullString{
			String: args.Password.PasswordHash,
			Valid:  true,
		}
		createParams.PasswordHashAlgorithmID = sql.NullInt16{
			Int16: int16(args.Password.HashAlgorithm),
			Valid: true,
		}
	}

	createUnit := `INSERT INTO unit (*) VALUES ($unitRow.*)`
	createUnitStmt, err := st.Prepare(createUnit, createParams)
	if err != nil {
		return errors.Capture(err)
	}

	err = st.ensureFutureUnitNetNode(ctx, tx, netNodeUUID)
	if err != nil {
		return errors.Errorf(
			"ensuring that the net node %q exists for new unit %q: %w",
			netNodeUUID, args.UnitName, err,
		)
	}

	if err := tx.Query(ctx, createUnitStmt, createParams).Run(); err != nil {
		return errors.Errorf("creating unit for unit %q: %w", args.UnitName, err)
	}
	// TODO (hml) 8-Dec-2025
	// CloudContainer is specific to CAAS units. The callers should handle
	// this after insertUnit when a CAAS unit is created, not inside the
	// generic add unit method.
	if args.CloudContainer != nil {
		if err := st.upsertUnitCloudContainer(ctx, tx, args.UnitName, unitUUID, netNodeUUID, args.CloudContainer); err != nil {
			return errors.Errorf("creating cloud container for unit %q: %w", args.UnitName, err)
		}
	}
	if err := st.setUnitAgentStatus(ctx, tx, unitUUID, args.AgentStatus); err != nil {
		return errors.Errorf("setting agent status for unit %q: %w", args.UnitName, err)
	}
	if err := st.setUnitWorkloadStatus(ctx, tx, unitUUID, args.WorkloadStatus); err != nil {
		return errors.Errorf("setting workload status for unit %q: %w", args.UnitName, err)
	}
	if err := st.setUnitWorkloadVersion(ctx, tx, unitUUID, args.WorkloadVersion); err != nil {
		return errors.Errorf("setting workload version for unit %q: %w", args.UnitName, err)
	}
	return nil
}

// getApplicationName returns the application name. If no application is found,
// an error satisfying [applicationerrors.ApplicationNotFound] is returned.
func (st *InsertIAASUnitState) getApplicationName(
	ctx context.Context,
	tx *sqlair.TX,
	id string,
) (string, error) {
	arg := applicationUUIDAndName{
		ID: id,
	}
	stmt, err := st.Prepare(`
SELECT a.name AS &applicationUUIDAndName.name
FROM   application AS a
JOIN   charm AS c ON c.uuid = a.charm_uuid
WHERE  a.uuid = $applicationUUIDAndName.uuid AND c.source_id < 2;
`, arg)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, arg).Get(&arg)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("application %q not found", id).Add(applicationerrors.ApplicationNotFound)
	} else if err != nil {
		return "", errors.Capture(err)
	}

	return arg.Name, nil
}

// checkApplicationAlive checks if the application exists and it is alive.
func (st *InsertIAASUnitState) checkApplicationAlive(ctx context.Context, tx *sqlair.TX, appUUID string) error {
	type life struct {
		LifeID corelife.Value `db:"value"`
	}

	ident := entityUUID{UUID: appUUID}
	query := `
SELECT &life.*
FROM   application AS a
JOIN   life as l ON a.life_id = l.id
WHERE  a.uuid = $entityUUID.uuid
`
	stmt, err := st.Prepare(query, ident, life{})
	if err != nil {
		return errors.Errorf("preparing query for application %q: %w", ident.UUID, err)
	}

	var result life
	err = tx.Query(ctx, stmt, ident).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return applicationerrors.ApplicationNotFound
	} else if err != nil {
		return errors.Errorf("checking application %q exists: %w", ident.UUID, err)
	}

	switch result.LifeID {
	case corelife.Dead:
		return applicationerrors.ApplicationIsDead
	case corelife.Dying:
		return applicationerrors.ApplicationNotAlive
	default:
		return nil
	}
}

// newUnitName returns a new name for the unit. It increments the unit counter
// on the application.
func (st *InsertIAASUnitState) newUnitName(
	ctx context.Context,
	tx *sqlair.TX,
	appUUID string,
) (string, error) {
	var nextUnitNum uint64
	appName, err := st.getApplicationName(ctx, tx, appUUID)
	if err != nil {
		return "", errors.Capture(err)
	}

	namespace := domainsequence.MakePrefixNamespace(application.ApplicationSequenceNamespace, appName)
	nextUnitNum, err = sequencestate.NextValue(ctx, st, tx, namespace)
	if err != nil {
		return "", errors.Errorf("getting next unit number: %w", err)
	}

	unitName, err := coreunit.NewNameFromParts(appName, int(nextUnitNum))
	return unitName.String(), errors.Capture(err)
}

func (st *InsertIAASUnitState) upsertUnitCloudContainer(
	ctx context.Context,
	tx *sqlair.TX,
	unitName, unitUUID, netNodeUUID string,
	cc *application.CloudContainer,
) error {
	containerInfo := cloudContainer{
		UnitUUID:   unitUUID,
		ProviderID: cc.ProviderID,
	}

	queryStmt, err := st.Prepare(`
SELECT &cloudContainer.*
FROM k8s_pod
WHERE unit_uuid = $cloudContainer.unit_uuid
`, containerInfo)
	if err != nil {
		return errors.Capture(err)
	}

	insertStmt, err := st.Prepare(`
INSERT INTO k8s_pod (*) VALUES ($cloudContainer.*)
`, containerInfo)
	if err != nil {
		return errors.Capture(err)
	}

	updateStmt, err := st.Prepare(`
UPDATE k8s_pod SET
    provider_id = $cloudContainer.provider_id
WHERE unit_uuid = $cloudContainer.unit_uuid
`, containerInfo)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, queryStmt, containerInfo).Get(&containerInfo)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("looking up cloud container %q: %w", unitName, err)
	}
	if err == nil {
		newProviderID := cc.ProviderID
		if newProviderID != "" &&
			containerInfo.ProviderID != newProviderID {
			st.logger.Debugf(ctx, "unit %q has provider id %q which changed to %q",
				unitName, containerInfo.ProviderID, newProviderID)
		}
		containerInfo.ProviderID = newProviderID
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

func (st *InsertIAASUnitState) upsertCloudContainerAddress(
	ctx context.Context, tx *sqlair.TX, unitName, netNodeUUID string, address application.ContainerAddress,
) error {
	// First ensure the address link layer device is upserted.
	// For cloud containers, the device is a placeholder without
	// a MAC address. It just exits to tie the address to the
	// net node corresponding to the cloud container.
	cloudContainerDeviceInfo := cloudContainerDevice{
		Name:              address.Device.Name,
		NetNodeID:         netNodeUUID,
		DeviceTypeID:      int(address.Device.DeviceTypeID),
		VirtualPortTypeID: int(address.Device.VirtualPortTypeID),
	}

	selectCloudContainerDeviceStmt, err := st.Prepare(`
SELECT &cloudContainerDevice.uuid
FROM link_layer_device
WHERE net_node_uuid = $cloudContainerDevice.net_node_uuid
`, cloudContainerDeviceInfo)
	if err != nil {
		return errors.Capture(err)
	}

	insertCloudContainerDeviceStmt, err := st.Prepare(`
INSERT INTO link_layer_device (*) VALUES ($cloudContainerDevice.*)
`, cloudContainerDeviceInfo)
	if err != nil {
		return errors.Capture(err)
	}

	// See if the link layer device exists, if not insert it.
	err = tx.Query(ctx, selectCloudContainerDeviceStmt, cloudContainerDeviceInfo).Get(&cloudContainerDeviceInfo)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("querying cloud container link layer device for unit %q: %w", unitName, err)
	}
	if errors.Is(err, sqlair.ErrNoRows) {
		deviceUUID, err := uuid.NewUUID()
		if err != nil {
			return errors.Capture(err)
		}
		cloudContainerDeviceInfo.UUID = deviceUUID.String()
		if err := tx.Query(ctx, insertCloudContainerDeviceStmt, cloudContainerDeviceInfo).Run(); err != nil {
			return errors.Errorf("inserting cloud container device for unit %q: %w", unitName, err)
		}
	}
	deviceUUID := cloudContainerDeviceInfo.UUID

	subnetUUIDs, err := st.k8sSubnetUUIDsByAddressType(ctx, tx)
	if err != nil {
		return errors.Capture(err)
	}
	subnetUUID, ok := subnetUUIDs[ipaddress.UnMarshallAddressType(address.AddressType)]
	if !ok {
		// Note: This is a programming error. Today the K8S subnets are
		// placeholders which should always be created when a model is
		// added.
		return errors.Errorf("subnet for address type %q not found", address.AddressType)
	}

	// Now process the address details.
	ipAddr := ipAddress{
		Value:        address.Value,
		SubnetUUID:   subnetUUID,
		NetNodeUUID:  netNodeUUID,
		ConfigTypeID: int(address.ConfigType),
		TypeID:       int(address.AddressType),
		OriginID:     int(address.Origin),
		ScopeID:      int(address.Scope),
		DeviceID:     deviceUUID,
	}

	selectAddressUUIDStmt, err := st.Prepare(`
SELECT &ipAddress.uuid
FROM   ip_address
WHERE  device_uuid = $ipAddress.device_uuid;
`, ipAddr)
	if err != nil {
		return errors.Capture(err)
	}

	upsertAddressStmt, err := st.Prepare(`
INSERT INTO ip_address (*)
VALUES ($ipAddress.*)
ON CONFLICT(uuid) DO UPDATE SET
    address_value = excluded.address_value,
    subnet_uuid = excluded.subnet_uuid,
    type_id = excluded.type_id,
    scope_id = excluded.scope_id,
    origin_id = excluded.origin_id,
    config_type_id = excluded.config_type_id
`, ipAddr)
	if err != nil {
		return errors.Capture(err)
	}

	// Container addresses are never deleted unless the container itself is deleted.
	// First see if there's an existing address recorded.
	err = tx.Query(ctx, selectAddressUUIDStmt, ipAddr).Get(&ipAddr)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("querying existing cloud container address for device %q: %w", deviceUUID, err)
	}

	// Create a UUID for new addresses.
	if errors.Is(err, sqlair.ErrNoRows) {
		addrUUID, err := uuid.NewUUID()
		if err != nil {
			return errors.Capture(err)
		}
		ipAddr.AddressUUID = addrUUID.String()
	}

	// Update the address values.
	if err = tx.Query(ctx, upsertAddressStmt, ipAddr).Run(); err != nil {
		return errors.Errorf("updating cloud container address attributes for device %q: %w", deviceUUID, err)
	}
	return nil
}

func (st *InsertIAASUnitState) upsertCloudContainerPorts(ctx context.Context, tx *sqlair.TX, unitUUID string, portValues []string) error {
	type ports []string

	ccPort := unitK8sPodPort{
		UnitUUID: unitUUID,
	}
	deleteStmt, err := st.Prepare(`
DELETE FROM k8s_pod_port
WHERE port NOT IN ($ports[:])
AND unit_uuid = $unitK8sPodPort.unit_uuid;
`, ports{}, ccPort)
	if err != nil {
		return errors.Capture(err)
	}

	upsertStmt, err := st.Prepare(`
INSERT INTO k8s_pod_port (*)
VALUES ($unitK8sPodPort.*)
ON CONFLICT(unit_uuid, port)
DO NOTHING
`, ccPort)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, deleteStmt, ports(portValues), ccPort).Run(); err != nil {
		return errors.Errorf("removing cloud container ports for %q: %w", unitUUID, err)
	}

	for _, port := range portValues {
		ccPort.Port = port
		if err := tx.Query(ctx, upsertStmt, ccPort).Run(); err != nil {
			return errors.Errorf("updating cloud container ports for %q: %w", unitUUID, err)
		}
	}

	return nil
}

func (st *InsertIAASUnitState) k8sSubnetUUIDsByAddressType(ctx context.Context, tx *sqlair.TX) (map[network.AddressType]string, error) {
	result := make(map[network.AddressType]string)
	subnetStmt, err := st.Prepare(`SELECT &subnet.* FROM subnet`, subnet{})
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

// ensureFutureUnitNetNode exists to ensure that a netnode uuid that is about to
// be used for a machine exists. We do this because the business logic around if
// netnode uuid for a unit is shared or already exists is outside the scope of
// state.
func (st *InsertIAASUnitState) ensureFutureUnitNetNode(
	ctx context.Context, tx *sqlair.TX, uuid string,
) error {
	netNodeUUID := netNodeUUID{NetNodeUUID: uuid}

	ensureNode := `INSERT INTO net_node (uuid) VALUES ($netNodeUUID.*)`
	ensureNodeStmt, err := st.Prepare(ensureNode, netNodeUUID)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, ensureNodeStmt, netNodeUUID).Run()
	if internaldatabase.IsErrConstraintPrimaryKey(err) {
		return nil
	} else if err != nil {
		return errors.Errorf("ensuring net node %q for future unit: %w", uuid, err)
	}

	return nil
}

func (st *InsertIAASUnitState) getUnitApplicationUUID(ctx context.Context, tx *sqlair.TX, uuid string) (string, error) {
	unitUUID := unitUUID{UnitUUID: uuid}

	query, err := st.Prepare(`
SELECT a.uuid AS &entityUUID.uuid
FROM application AS a
JOIN unit AS u ON u.application_uuid = a.uuid
WHERE u.uuid = $unitUUID.uuid
	`, entityUUID{}, unitUUID)
	if err != nil {
		return "", errors.Capture(err)
	}

	var appID entityUUID
	err = tx.Query(ctx, query, unitUUID).Get(&appID)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("unit %q not found", uuid).Add(applicationerrors.UnitNotFound)
	} else if err != nil {
		return "", errors.Capture(err)
	}

	return appID.UUID, nil
}

// placeIAASUnitMachine is responsible for making sure that the machine required
// by the unit being added has been placed.
func (st *InsertIAASUnitState) placeIAASUnitMachine(
	ctx context.Context,
	tx *sqlair.TX,
	args application.AddIAASUnitArg,
) ([]coremachine.Name, error) {
	// Handle the placement of the net node and machines accompanying the unit.
	placeMachineArgs := domainmachine.PlaceMachineArgs{
		Constraints: args.Constraints,
		Directive:   args.Placement,
		Platform:    args.Platform,
		Nonce:       args.Nonce,
		NetNodeUUID: args.MachineNetNodeUUID,
		MachineUUID: args.MachineUUID,
	}
	st.logger.Debugf(ctx, "placing machine %q for unit with args: %#v", args.MachineUUID, placeMachineArgs)

	machineNames, err := machinestate.PlaceMachine(
		ctx, tx, st, st.clock, placeMachineArgs,
	)
	if err != nil {
		return nil, errors.Errorf("performing unit placement %#v: %w", args.Placement, err)
	}

	return machineNames, nil
}

// setUnitWorkloadVersion workload version sets the denormalized workload
// version on both the unit and the application. These are on separate tables,
// so we need to do two separate queries. This prevents the workload version
// from trigging a cascade of unwanted updates to the application and or unit
// tables.
func (st *InsertIAASUnitState) setUnitWorkloadVersion(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID string,
	version string,
) error {
	unitQuery, err := st.Prepare(`
INSERT INTO unit_workload_version (*)
VALUES ($unitWorkloadVersion.*)
ON CONFLICT (unit_uuid) DO UPDATE SET
    version = excluded.version;
`, unitWorkloadVersion{})
	if err != nil {
		return errors.Capture(err)
	}

	appQuery, err := st.Prepare(`
INSERT INTO application_workload_version (*)
VALUES ($applicationWorkloadVersion.*)
ON CONFLICT (application_uuid) DO UPDATE SET
	version = excluded.version;
`, applicationWorkloadVersion{})
	if err != nil {
		return errors.Capture(err)
	}

	appID, err := st.getUnitApplicationUUID(ctx, tx, unitUUID)
	if err != nil {
		return errors.Errorf("getting application UUID for unit %q: %w", unitUUID, err)
	}

	if err := tx.Query(ctx, unitQuery, unitWorkloadVersion{
		UnitUUID: unitUUID,
		Version:  version,
	}).Run(); err != nil {
		return errors.Errorf("setting workload version for unit %q: %w", unitUUID, err)
	}

	if err := tx.Query(ctx, appQuery, applicationWorkloadVersion{
		ApplicationUUID: appID,
		Version:         version,
	}).Run(); err != nil {
		return errors.Errorf("setting workload version for application %q: %w", appID, err)
	}
	return nil
}

// status data. If returns an error satisfying [applicationerrors.UnitNotFound]
// if the unit doesn't exist.
func (st *InsertIAASUnitState) setUnitAgentStatus(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID string,
	sts *status.StatusInfo[status.UnitAgentStatusType],
) error {
	if sts == nil {
		return nil
	}

	statusID, err := status.EncodeAgentStatus(sts.Status)
	if err != nil {
		return errors.Capture(err)
	}

	statusInfo := unitStatusInfo{
		UnitUUID:  unitUUID,
		StatusID:  statusID,
		Message:   sts.Message,
		Data:      sts.Data,
		UpdatedAt: sts.Since,
	}
	stmt, err := st.Prepare(`
INSERT INTO unit_agent_status (*) VALUES ($unitStatusInfo.*)
ON CONFLICT(unit_uuid) DO UPDATE SET
    status_id = excluded.status_id,
    message = excluded.message,
    updated_at = excluded.updated_at,
    data = excluded.data;
`, statusInfo)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt, statusInfo).Run(); internaldatabase.IsErrConstraintForeignKey(err) {
		return errors.Errorf("%w: %q", applicationerrors.UnitNotFound, unitUUID)
	} else if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// setUnitWorkloadStatus saves the given unit workload status, overwriting any
// current status data. If returns an error satisfying
// [applicationerrors.UnitNotFound] if the unit doesn't exist.
func (st *InsertIAASUnitState) setUnitWorkloadStatus(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID string,
	sts *status.StatusInfo[status.WorkloadStatusType],
) error {
	if sts == nil {
		return nil
	}

	statusID, err := status.EncodeWorkloadStatus(sts.Status)
	if err != nil {
		return errors.Capture(err)
	}

	statusInfo := unitStatusInfo{
		UnitUUID:  unitUUID,
		StatusID:  statusID,
		Message:   sts.Message,
		Data:      sts.Data,
		UpdatedAt: sts.Since,
	}
	stmt, err := st.Prepare(`
INSERT INTO unit_workload_status (*) VALUES ($unitStatusInfo.*)
ON CONFLICT(unit_uuid) DO UPDATE SET
    status_id = excluded.status_id,
    message = excluded.message,
    updated_at = excluded.updated_at,
    data = excluded.data;
`, statusInfo)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt, statusInfo).Run(); internaldatabase.IsErrConstraintForeignKey(err) {
		return errors.Errorf("%w: %q", applicationerrors.UnitNotFound, unitUUID)
	} else if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// insertUnitStorageAttachments is responsible for creating all of the unit
// storage attachments for the supplied storage instance uuids. This func will
// also create storage attachments for each filesystem and volume
func (st *InsertIAASUnitState) insertUnitStorageAttachments(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID string,
	storageToAttach []internal.CreateUnitStorageAttachmentArg,
) error {
	storageAttachmentArgs := makeInsertUnitStorageAttachmentArgs(
		ctx, unitUUID, storageToAttach,
	)

	fsAttachmentArgs := st.makeInsertUnitFilesystemAttachmentArgs(
		storageToAttach,
	)

	volAttachmentArgs := st.makeInsertUnitVolumeAttachmentArgs(
		storageToAttach,
	)

	insertStorageAttachmentStmt, err := st.Prepare(`
INSERT INTO storage_attachment (*) VALUES ($insertStorageInstanceAttachment.*)
`,
		insertStorageInstanceAttachment{})
	if err != nil {
		return errors.Capture(err)
	}

	insertFSAttachmentStmt, err := st.Prepare(`
INSERT INTO storage_filesystem_attachment (*)
VALUES ($insertStorageFilesystemAttachment.*)
`,
		insertStorageFilesystemAttachment{})
	if err != nil {
		return errors.Capture(err)
	}

	insertVolAttachmentStmt, err := st.Prepare(`
INSERT INTO storage_volume_attachment (*)
VALUES ($insertStorageVolumeAttachment.*)
`,
		insertStorageVolumeAttachment{})
	if err != nil {
		return errors.Capture(err)
	}

	if len(storageAttachmentArgs) != 0 {
		err := tx.Query(ctx, insertStorageAttachmentStmt, storageAttachmentArgs).Run()
		if err != nil {
			return errors.Errorf(
				"create storage attachments for unit %q: %w", unitUUID, err,
			)
		}
	}

	if len(fsAttachmentArgs) != 0 {
		err := tx.Query(ctx, insertFSAttachmentStmt, fsAttachmentArgs).Run()
		if err != nil {
			return errors.Errorf(
				"create filesystem attachments for unit %q: %w", unitUUID, err,
			)
		}
	}

	if len(volAttachmentArgs) != 0 {
		err := tx.Query(ctx, insertVolAttachmentStmt, volAttachmentArgs).Run()
		if err != nil {
			return errors.Errorf(
				"create volume attachments for unit %q: %w", unitUUID, err,
			)
		}
	}

	return nil
}

// insertUnitStorageDirectives is responsible for creating the storage
// directives for a unit. This func assumes that no storage directives exist
// already for the unit.
//
// The storage directives supply must match the storage defined by the charm.
// It is expected that the caller is satisfied this check has been performed.
func (st *InsertIAASUnitState) insertUnitStorageDirectives(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID, charmUUID string,
	args []internal.CreateUnitStorageDirectiveArg,
) error {
	if len(args) == 0 {
		return nil
	}

	insertStorageDirectiveStmt, err := st.Prepare(`
INSERT INTO unit_storage_directive (*) VALUES ($insertUnitStorageDirective.*)
`,
		insertUnitStorageDirective{})
	if err != nil {
		return errors.Capture(err)
	}

	insertArgs := make([]insertUnitStorageDirective, 0, len(args))
	for _, arg := range args {
		insertArgs = append(insertArgs, insertUnitStorageDirective{
			CharmUUID:       charmUUID,
			Count:           arg.Count,
			Size:            arg.Size,
			StorageName:     arg.Name.String(),
			StoragePoolUUID: arg.PoolUUID.String(),
			UnitUUID:        unitUUID,
		})
	}

	err = tx.Query(ctx, insertStorageDirectiveStmt, insertArgs).Run()
	if err != nil {
		return errors.Errorf(
			"creating unit %q storage directives: %w", unitUUID, err,
		)
	}

	return nil
}

// insertUnitStorageInstances is responsible for creating all of the needed
// storage instances to satisfy the storage instance arguments supplied.
// The IDs of the new storage instances are returned.
func (st *InsertIAASUnitState) insertUnitStorageInstances(
	ctx context.Context,
	tx *sqlair.TX,
	stArgs []internal.CreateUnitStorageInstanceArg,
) ([]string, error) {
	storageInstArgs, err := st.makeInsertUnitStorageInstanceArgs(
		ctx, tx, stArgs,
	)
	if err != nil {
		return nil, errors.Errorf(
			"creating database input for making unit storage instances: %w",
			err,
		)
	}

	fsArgs, fsInstanceArgs, fsStatusArgs, err := st.makeInsertUnitFilesystemArgs(
		ctx, tx, stArgs,
	)
	if err != nil {
		return nil, errors.Errorf(
			"creating database input for making unit storage filesystems: %w",
			err,
		)
	}

	vArgs, vInstanceArgs, vStatusArgs, err := st.makeInsertUnitVolumeArgs(
		ctx, tx, stArgs,
	)
	if err != nil {
		return nil, errors.Errorf(
			"creating database input for making unit storage volumes: %w",
			err,
		)
	}

	insertStorageInstStmt, err := st.Prepare(`
INSERT INTO storage_instance (*) VALUES ($insertStorageInstance.*)
`,
		insertStorageInstance{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	insertStorageFilesystemStmt, err := st.Prepare(`
INSERT INTO storage_filesystem (*) VALUES ($insertStorageFilesystem.*)
`,
		insertStorageFilesystem{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	insertStorageFilesystemInstStmt, err := st.Prepare(`
INSERT INTO storage_instance_filesystem (*) VALUES ($insertStorageFilesystemInstance.*)
`,
		insertStorageFilesystemInstance{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	insertStorageFilesystemStatusStmt, err := st.Prepare(`
INSERT INTO storage_filesystem_status (*) VALUES ($insertStorageFilesystemStatus.*)
`,
		insertStorageFilesystemStatus{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	insertStorageVolumeStmt, err := st.Prepare(`
INSERT INTO storage_volume (*) VALUES ($insertStorageVolume.*)
`,
		insertStorageVolume{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	insertStorageVolumeInstStmt, err := st.Prepare(`
INSERT INTO storage_instance_volume (*) VALUES ($insertStorageVolumeInstance.*)
`,
		insertStorageVolumeInstance{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	insertStorageVolumeStatusStmt, err := st.Prepare(`
INSERT INTO storage_volume_status (*) VALUES ($insertStorageVolumeStatus.*)
`,
		insertStorageVolumeStatus{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	// We guard against zero length insert args below. This is because there is
	// no correlation between input args and the number of inserts that happen.
	// Empty inserts will result in an error that we don't need to consider.
	if len(storageInstArgs) != 0 {
		err := tx.Query(ctx, insertStorageInstStmt, storageInstArgs).Run()
		if err != nil {
			return nil, errors.Errorf(
				"creating %d storage instance(s): %w",
				len(storageInstArgs), err,
			)
		}
	}

	if len(fsArgs) != 0 {
		err := tx.Query(ctx, insertStorageFilesystemStmt, fsArgs).Run()
		if err != nil {
			return nil, errors.Errorf(
				"creating %d storage filesystems: %w",
				len(fsArgs), err,
			)
		}
	}

	if len(fsInstanceArgs) != 0 {
		err := tx.Query(ctx, insertStorageFilesystemInstStmt, fsInstanceArgs).Run()
		if err != nil {
			return nil, errors.Errorf(
				"setting storage filesystem to instance relationship for new filesystems: %w",
				err,
			)
		}
	}

	if len(fsStatusArgs) != 0 {
		err := tx.Query(ctx, insertStorageFilesystemStatusStmt, fsStatusArgs).Run()
		if err != nil {
			return nil, errors.Errorf(
				"setting newly create storage filesystem(s) status: %w",
				err,
			)
		}
	}

	if len(vArgs) != 0 {
		err := tx.Query(ctx, insertStorageVolumeStmt, vArgs).Run()
		if err != nil {
			return nil, errors.Errorf(
				"creating %d storage volumes: %w",
				len(fsArgs), err,
			)
		}
	}

	if len(vInstanceArgs) != 0 {
		err := tx.Query(ctx, insertStorageVolumeInstStmt, vInstanceArgs).Run()
		if err != nil {
			return nil, errors.Errorf(
				"setting storage volume to instance relationship for new volumes: %w",
				err,
			)
		}
	}

	if len(vStatusArgs) != 0 {
		err := tx.Query(ctx, insertStorageVolumeStatusStmt, vStatusArgs).Run()
		if err != nil {
			return nil, errors.Errorf(
				"setting newly create storage volume(s) status: %w",
				err,
			)
		}
	}

	var result []string
	for _, inst := range storageInstArgs {
		result = append(result, inst.StorageID)
	}
	return result, nil
}

// insertUnitStorageOwnership is responsible setting unit ownership records for
// the supplied storage instance uuids.
func (st *InsertIAASUnitState) insertUnitStorageOwnership(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID string,
	storageToOwn []domainstorage.StorageInstanceUUID,
) error {
	args := makeInsertUnitStorageOwnerArgs(ctx, unitUUID, storageToOwn)
	if len(args) == 0 {
		return nil
	}

	insertStorageOwnerStmt, err := st.Prepare(`
INSERT INTO storage_unit_owner (*) VALUES ($insertStorageUnitOwner.*)
`,
		insertStorageUnitOwner{})
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, insertStorageOwnerStmt, args).Run()
	if err != nil {
		return errors.Errorf(
			"setting storage instance unit owner: %w", err,
		)
	}

	return nil
}

// insertMachineVolumeOwnership is responsible setting machine ownership records
// for the supplied volume uuids.
func (st *InsertIAASUnitState) insertMachineVolumeOwnership(
	ctx context.Context,
	tx *sqlair.TX,
	machineUUID coremachine.UUID,
	volumesToOwn []domainstorage.VolumeUUID,
) error {
	args := makeInsertMachineVolumeOwnerArgs(ctx, machineUUID, volumesToOwn)
	if len(args) == 0 {
		return nil
	}

	stmt, err := st.Prepare(`
INSERT INTO machine_volume (*) VALUES ($insertVolumeMachineOwner.*)
`, insertVolumeMachineOwner{})
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, args).Run()
	if err != nil {
		return errors.Errorf(
			"setting volume machine owner: %w", err,
		)
	}

	return nil
}

// insertMachineFilesystemOwnership is responsible setting machine ownership
// records for the supplied filesystem uuids.
func (st *InsertIAASUnitState) insertMachineFilesystemOwnership(
	ctx context.Context,
	tx *sqlair.TX,
	machineUUID coremachine.UUID,
	filesystemsToOwn []domainstorage.FilesystemUUID,
) error {
	args := makeInsertMachineFilesystemOwnerArgs(ctx, machineUUID,
		filesystemsToOwn)
	if len(args) == 0 {
		return nil
	}

	stmt, err := st.Prepare(`
INSERT INTO machine_filesystem (*) VALUES ($insertFilesystemMachineOwner.*)
`, insertFilesystemMachineOwner{})
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, args).Run()
	if err != nil {
		return errors.Errorf(
			"setting filesystem machine owner: %w", err,
		)
	}

	return nil
}

// makeInsertUnitFilesystemArgs is responsible for making the insert args to
// establish new filesystems linked to a storage instance in the model.
func (st *InsertIAASUnitState) makeInsertUnitFilesystemArgs(
	ctx context.Context,
	tx *sqlair.TX,
	args []internal.CreateUnitStorageInstanceArg,
) (
	[]insertStorageFilesystem,
	[]insertStorageFilesystemInstance,
	[]insertStorageFilesystemStatus,
	error,
) {
	argIndexes := make([]int, 0, len(args))
	for i, arg := range args {
		// If the caller does not provide a filesystem uuid then they don't
		// expect one to be created.
		if arg.Filesystem == nil {
			continue
		}
		argIndexes = append(argIndexes, i)
	}

	// early exit
	if len(argIndexes) == 0 {
		return nil, nil, nil, nil
	}

	// now that we have the set of filesystems we can generate the ids
	fsIDS, err := sequencestate.NextNValues(
		ctx, st, tx, uint64(len(argIndexes)), filesystemNamespace,
	)
	if err != nil {
		return nil, nil, nil, errors.Errorf(
			"generating %d new filesystem ids: %w", len(argIndexes), err,
		)
	}

	fsStatus, err := status.EncodeStorageFilesystemStatus(
		status.StorageFilesystemStatusTypePending,
	)
	if err != nil {
		return nil, nil, nil, errors.Errorf(
			"encoding filesystem status pending for new filesystem args: err",
		)
	}

	fsRval := make([]insertStorageFilesystem, 0, len(argIndexes))
	fsInstanceRval := make([]insertStorageFilesystemInstance, 0, len(argIndexes))
	fsStatusRval := make([]insertStorageFilesystemStatus, 0, len(argIndexes))
	statusTime := st.clock.Now().UTC()
	for i, argIndex := range argIndexes {
		instArg := args[argIndex]
		fsRval = append(fsRval, insertStorageFilesystem{
			FilesystemID:     fmt.Sprintf("%d", fsIDS[i]),
			LifeID:           int(life.Alive),
			UUID:             instArg.Filesystem.UUID.String(),
			ProvisionScopeID: int(instArg.Filesystem.ProvisionScope),
		})
		fsInstanceRval = append(fsInstanceRval, insertStorageFilesystemInstance{
			StorageInstanceUUID:    instArg.UUID.String(),
			StorageFilesystemUUUID: instArg.Filesystem.UUID.String(),
		})
		fsStatusRval = append(fsStatusRval, insertStorageFilesystemStatus{
			FilesystemUUID: instArg.Filesystem.UUID.String(),
			StatusID:       fsStatus,
			UpdateAt:       statusTime,
		})
	}

	return fsRval, fsInstanceRval, fsStatusRval, nil
}

// makeInsertUnitFilesystemAttachmentArgs will make a slice of
// [insertStorageFilesystemAttachment] for each filesystem attachment defined in
// args.
func (st *InsertIAASUnitState) makeInsertUnitFilesystemAttachmentArgs(
	args []internal.CreateUnitStorageAttachmentArg,
) []insertStorageFilesystemAttachment {
	rval := []insertStorageFilesystemAttachment{}
	for _, arg := range args {
		if arg.FilesystemAttachment == nil {
			continue
		}

		rval = append(rval, insertStorageFilesystemAttachment{
			LifeID:                int(life.Alive),
			NetNodeUUID:           arg.FilesystemAttachment.NetNodeUUID.String(),
			ProvisionScopeID:      int(arg.FilesystemAttachment.ProvisionScope),
			StorageFilesystemUUID: arg.FilesystemAttachment.FilesystemUUID.String(),
			UUID:                  arg.FilesystemAttachment.UUID.String(),
		})
	}

	return rval
}

// makeInsertUnitStorageInstanceArgs is responsible for making the insert args
// required for instantiating new storage instances that match a unit's storage
// directive Included in the return is the set of insert values required for
// making the unit the owner of the new storage instance(s). Attachment records
// are also returned for each of the storage instances.
func (st *InsertIAASUnitState) makeInsertUnitStorageInstanceArgs(
	ctx context.Context,
	tx *sqlair.TX,
	args []internal.CreateUnitStorageInstanceArg,
) ([]insertStorageInstance, error) {
	storageInstancesRval := make([]insertStorageInstance, 0, len(args))

	for _, arg := range args {
		id, err := sequencestate.NextValue(ctx, st, tx, storageNamespace)
		if err != nil {
			return nil, errors.Errorf(
				"creating unique storage instance id: %w", err,
			)
		}
		storageID := corestorage.MakeID(
			corestorage.Name(arg.Name), id,
		).String()

		storageInstancesRval = append(storageInstancesRval, insertStorageInstance{
			CharmName:       arg.CharmName,
			LifeID:          int(life.Alive),
			RequestSizeMiB:  arg.RequestSizeMiB,
			StorageID:       storageID,
			StorageKindID:   int(arg.Kind),
			StorageName:     arg.Name.String(),
			StoragePoolUUID: arg.StoragePoolUUID.String(),
			UUID:            arg.UUID.String(),
		})
	}

	return storageInstancesRval, nil
}

// makeInsertUnitVolumeArgs is responsible for making the insert args to
// establish new volumes linked to a storage instance in the model.
func (st *InsertIAASUnitState) makeInsertUnitVolumeArgs(
	ctx context.Context,
	tx *sqlair.TX,
	args []internal.CreateUnitStorageInstanceArg,
) (
	[]insertStorageVolume,
	[]insertStorageVolumeInstance,
	[]insertStorageVolumeStatus,
	error,
) {
	argIndexes := make([]int, 0, len(args))
	for i, arg := range args {
		// If the caller does not provide a volume uuid then they don't
		// expect one to be created.
		if arg.Volume == nil {
			continue
		}
		argIndexes = append(argIndexes, i)
	}

	// early exit
	if len(argIndexes) == 0 {
		return nil, nil, nil, nil
	}

	// now that we have the set of volumes we can generate the ids
	fsIDS, err := sequencestate.NextNValues(
		ctx, st, tx, uint64(len(argIndexes)), volumeNamespace,
	)
	if err != nil {
		return nil, nil, nil, errors.Errorf(
			"generating %d new volume ids: %w", len(argIndexes), err,
		)
	}

	vStatus, err := status.EncodeStorageVolumeStatus(
		status.StorageVolumeStatusTypePending,
	)
	if err != nil {
		return nil, nil, nil, errors.Errorf(
			"encoding volume status pending for new volume args: err",
		)
	}

	vRval := make([]insertStorageVolume, 0, len(argIndexes))
	vInstanceRval := make([]insertStorageVolumeInstance, 0, len(argIndexes))
	vStatusRval := make([]insertStorageVolumeStatus, 0, len(argIndexes))
	statusTime := st.clock.Now().UTC()
	for i, argIndex := range argIndexes {
		instArg := args[argIndex]
		vRval = append(vRval, insertStorageVolume{
			VolumeID:         fmt.Sprintf("%d", fsIDS[i]),
			LifeID:           int(life.Alive),
			UUID:             instArg.Volume.UUID.String(),
			ProvisionScopeID: int(instArg.Volume.ProvisionScope),
		})
		vInstanceRval = append(vInstanceRval, insertStorageVolumeInstance{
			StorageInstanceUUID: instArg.UUID.String(),
			StorageVolumeUUID:   instArg.Volume.UUID.String(),
		})
		vStatusRval = append(vStatusRval, insertStorageVolumeStatus{
			VolumeUUID: instArg.Volume.UUID.String(),
			StatusID:   vStatus,
			UpdateAt:   statusTime,
		})
	}

	return vRval, vInstanceRval, vStatusRval, nil
}

// makeInsertUnitVolumeAttachmentArgs will make a slice of
// [insertStorageVolumeAttachment] values for each volume attachment argument
// supplied.
func (st *InsertIAASUnitState) makeInsertUnitVolumeAttachmentArgs(
	args []internal.CreateUnitStorageAttachmentArg,
) []insertStorageVolumeAttachment {
	rval := []insertStorageVolumeAttachment{}
	for _, arg := range args {
		if arg.VolumeAttachment == nil {
			continue
		}

		rval = append(rval, insertStorageVolumeAttachment{
			LifeID:            int(life.Alive),
			NetNodeUUID:       arg.VolumeAttachment.NetNodeUUID.String(),
			ProvisionScopeID:  int(arg.VolumeAttachment.ProvisionScope),
			StorageVolumeUUID: arg.VolumeAttachment.VolumeUUID.String(),
			UUID:              arg.VolumeAttachment.UUID.String(),
		})
	}

	return rval
}
