// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	coredb "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	domainconstraints "github.com/juju/juju/domain/constraints"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/provisioning"
	"github.com/juju/juju/internal/errors"
)

// ModelState provides direct database access to the model database for
// provisioning info retrieval.
type ModelState struct {
	*domain.StateBase
	logger logger.Logger
}

// NewModelState returns a new model state reference.
func NewModelState(factory coredb.TxnRunnerFactory, logger logger.Logger) *ModelState {
	return &ModelState{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// GetProvisioningInfo retrieves all provisioning data for a machine in a
// single transaction from the model database.
//
// The following errors may be returned:
//   - [github.com/juju/juju/domain/machine/errors.MachineNotFound] if the machine does not exist.
func (st *ModelState) GetProvisioningInfo(ctx context.Context, machineName string, isControllerModel bool) (provisioning.ProvisioningInfoState, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return provisioning.ProvisioningInfoState{}, errors.Capture(err)
	}

	var result provisioning.ProvisioningInfoState

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var txErr error

		// Query 1: Machine base info (UUID, base, placement, constraints,
		// is-controller).
		result.MachineUUID, result.Base, result.PlacementDirective, result.Constraints, result.IsController, txErr = st.getMachineBaseInfo(ctx, tx, machineName, isControllerModel)
		if txErr != nil {
			return txErr
		}

		// Query 2: Units on machine.
		result.UnitNames, txErr = st.getUnitsOnMachine(ctx, tx, machineName)
		if txErr != nil {
			return txErr
		}

		// Query 3: Endpoint bindings for apps on the machine.
		result.EndpointBindings, txErr = st.getEndpointBindings(ctx, tx, result.UnitNames)
		if txErr != nil {
			return txErr
		}

		// Query 4: Volume params and attachments.
		result.VolumeParams, result.VolumeAttachmentParams, txErr = st.getVolumeParams(ctx, tx, string(result.MachineUUID))
		if txErr != nil {
			return txErr
		}

		// Query 5: Root disk storage pool.
		result.RootDiskStoragePool, txErr = st.getRootDiskStoragePool(ctx, tx, result.Constraints)
		if txErr != nil {
			return txErr
		}

		// Query 6: All spaces with subnets and AZs.
		result.Spaces, txErr = st.getAllSpaces(ctx, tx)
		if txErr != nil {
			return txErr
		}

		// Query 7: Model config values for provisioning.
		result.CloudInitUserData, result.ImageStream, result.ResourceTags, result.ResourceTagsFound, txErr = st.getModelConfigValues(ctx, tx)
		if txErr != nil {
			return txErr
		}

		// Query 8: Model identity info (name, cloud type, region).
		result.ModelName, result.CloudType, result.CloudRegion, result.CloudEndpoint, txErr = st.getModelInfo(ctx, tx)
		if txErr != nil {
			return txErr
		}

		// Query 9: Cached image metadata.
		result.CachedImageMetadata, txErr = st.getCachedImageMetadata(ctx, tx, result.Base, result.Constraints)
		if txErr != nil {
			return txErr
		}

		return nil
	})
	if err != nil {
		return provisioning.ProvisioningInfoState{}, errors.Capture(err)
	}

	return result, nil
}

// getMachineBaseInfo fetches the machine UUID, base OS, placement directive,
// constraints, and controller status.
func (st *ModelState) getMachineBaseInfo(
	ctx context.Context,
	tx *sqlair.TX,
	machineName string,
	isControllerModel bool,
) (coremachine.UUID, corebase.Base, *string, constraints.Value, bool, error) {
	// Fetch machine UUID and platform.
	stmt, err := st.Prepare(`
SELECT m.uuid AS &machineRow.uuid,
       vp.os_name AS &machineRow.os_name,
       vp.channel AS &machineRow.channel,
       mp.directive AS &machineRow.directive
FROM machine AS m
LEFT JOIN v_machine_platform AS vp ON vp.machine_uuid = m.uuid
LEFT JOIN machine_placement AS mp ON mp.machine_uuid = m.uuid
WHERE m.name = $machineRow.name
`, machineRow{})
	if err != nil {
		return "", corebase.Base{}, nil, constraints.Value{}, false, errors.Capture(err)
	}

	var mRow machineRow
	mRow.Name = machineName
	err = tx.Query(ctx, stmt, mRow).Get(&mRow)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", corebase.Base{}, nil, constraints.Value{}, false,
			errors.Errorf("machine %q: %w", machineName, machineerrors.MachineNotFound)
	}
	if err != nil {
		return "", corebase.Base{}, nil, constraints.Value{}, false, errors.Capture(err)
	}

	base, err := corebase.ParseBase(mRow.OSName, mRow.Channel)
	if err != nil {
		return "", corebase.Base{}, nil, constraints.Value{}, false,
			errors.Errorf("parsing base for machine %q: %w", machineName, err)
	}

	var placement *string
	if mRow.Directive.Valid {
		placement = &mRow.Directive.V
	}

	// Fetch constraints.
	consValue, err := st.getMachineConstraints(ctx, tx, mRow.UUID)
	if err != nil {
		return "", corebase.Base{}, nil, constraints.Value{}, false, errors.Capture(err)
	}

	// Fetch controller status if in controller model.
	var isController bool
	if isControllerModel {
		isController, err = st.isMachineController(ctx, tx, mRow.UUID)
		if err != nil {
			return "", corebase.Base{}, nil, constraints.Value{}, false, errors.Capture(err)
		}
	}

	return coremachine.UUID(mRow.UUID), base, placement, consValue, isController, nil
}

// getMachineConstraints fetches the constraints for a machine by its UUID.
func (st *ModelState) getMachineConstraints(
	ctx context.Context,
	tx *sqlair.TX,
	machineUUID string,
) (constraints.Value, error) {
	stmt, err := st.Prepare(`
SELECT &constraintRow.*
FROM v_machine_constraint AS vc
WHERE vc.machine_uuid = $machineUUIDParam.uuid
`, constraintRow{}, machineUUIDParam{})
	if err != nil {
		return constraints.Value{}, errors.Capture(err)
	}

	var rows []constraintRow
	err = tx.Query(ctx, stmt, machineUUIDParam{UUID: machineUUID}).GetAll(&rows)
	if errors.Is(err, sqlair.ErrNoRows) {
		return constraints.Value{}, nil
	}
	if err != nil {
		return constraints.Value{}, errors.Capture(err)
	}

	return decodeConstraintRows(rows), nil
}

// isMachineController checks if the machine is a controller by looking
// up the v_machine_is_controller view which joins through
// application_controller.
func (st *ModelState) isMachineController(
	ctx context.Context,
	tx *sqlair.TX,
	machineUUID string,
) (bool, error) {
	stmt, err := st.Prepare(`
SELECT COUNT(*) AS &countResult.count
FROM v_machine_is_controller
WHERE machine_uuid = $machineUUIDParam.uuid
`, countResult{}, machineUUIDParam{})
	if err != nil {
		return false, errors.Capture(err)
	}

	var result countResult
	err = tx.Query(ctx, stmt, machineUUIDParam{UUID: machineUUID}).Get(&result)
	if err != nil {
		return false, errors.Capture(err)
	}

	return result.Count > 0, nil
}

// getUnitsOnMachine fetches the unit names assigned to the machine.
func (st *ModelState) getUnitsOnMachine(
	ctx context.Context,
	tx *sqlair.TX,
	machineName string,
) ([]coreunit.NameWithPrincipal, error) {
	stmt, err := st.Prepare(`
SELECT u.name AS &unitRow.name,
       up.principal_uuid AS &unitRow.principal_uuid
FROM unit AS u
JOIN machine AS m ON u.net_node_uuid = m.net_node_uuid
LEFT JOIN unit_principal AS up ON up.unit_uuid = u.uuid
WHERE m.name = $machineRow.name
`, unitRow{}, machineRow{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var rows []unitRow
	err = tx.Query(ctx, stmt, machineRow{Name: machineName}).GetAll(&rows)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Collect principal UUIDs that need name resolution.
	principalUUIDs := make(map[string]bool)
	for _, row := range rows {
		if row.PrincipalUUID.Valid {
			principalUUIDs[row.PrincipalUUID.String] = true
		}
	}

	// Resolve principal UUIDs to names if needed.
	principalNames := make(map[string]string)
	if len(principalUUIDs) > 0 {
		var err error
		principalNames, err = st.resolveUnitUUIDsToNames(ctx, tx, principalUUIDs)
		if err != nil {
			return nil, errors.Capture(err)
		}
	}

	result := make([]coreunit.NameWithPrincipal, len(rows))
	for i, row := range rows {
		unitName := coreunit.Name(row.Name)
		var principal *coreunit.Name
		if row.PrincipalUUID.Valid {
			if name, ok := principalNames[row.PrincipalUUID.String]; ok {
				pName := coreunit.Name(name)
				principal = &pName
			}
		}
		result[i] = coreunit.NameWithPrincipal{
			Name:      unitName,
			Principal: principal,
		}
	}

	return result, nil
}

// resolveUnitUUIDsToNames resolves a set of unit UUIDs to their names.
func (st *ModelState) resolveUnitUUIDsToNames(
	ctx context.Context,
	tx *sqlair.TX,
	uuids map[string]bool,
) (map[string]string, error) {
	// Build a slice for the IN clause.
	uuidSlice := make(sqlair.S, 0, len(uuids))
	for uuid := range uuids {
		uuidSlice = append(uuidSlice, uuid)
	}

	stmt, err := st.Prepare(`
SELECT u.uuid AS &unitUUIDName.uuid,
       u.name AS &unitUUIDName.name
FROM unit AS u
WHERE u.uuid IN ($S[:])
`, unitUUIDName{}, uuidSlice)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var rows []unitUUIDName
	err = tx.Query(ctx, stmt, uuidSlice).GetAll(&rows)
	if errors.Is(err, sqlair.ErrNoRows) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := make(map[string]string, len(rows))
	for _, row := range rows {
		result[row.UUID] = row.Name
	}
	return result, nil
}

// getEndpointBindings fetches the endpoint bindings for the applications
// of the units on the machine. This queries application_endpoint and
// application_extra_endpoint, resolving NULL space UUIDs to the
// application's default space.
func (st *ModelState) getEndpointBindings(
	ctx context.Context,
	tx *sqlair.TX,
	unitNames []coreunit.NameWithPrincipal,
) (map[string]map[string]network.SpaceUUID, error) {
	if len(unitNames) == 0 {
		return nil, nil
	}

	// Determine unique non-subordinate unit names to get their apps.
	unitNameSlice := make(sqlair.S, 0, len(unitNames))
	for _, u := range unitNames {
		if u.IsSubordinate() {
			continue
		}
		unitNameSlice = append(unitNameSlice, u.Name.String())
	}

	if len(unitNameSlice) == 0 {
		return nil, nil
	}

	// Find application UUIDs and names for the units on the machine.
	appStmt, err := st.Prepare(`
SELECT DISTINCT a.uuid AS &appRow.uuid,
                a.name AS &appRow.name,
                a.space_uuid AS &appRow.space_uuid
FROM application AS a
JOIN unit AS u ON u.application_uuid = a.uuid
WHERE u.name IN ($S[:])
`, appRow{}, unitNameSlice)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var apps []appRow
	err = tx.Query(ctx, appStmt, unitNameSlice).GetAll(&apps)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := make(map[string]map[string]network.SpaceUUID, len(apps))

	for _, app := range apps {
		bindings := make(map[string]network.SpaceUUID)

		// Default binding (the "" endpoint).
		defaultSpace := network.SpaceUUID(app.SpaceUUID)
		bindings[""] = defaultSpace

		// Get relation endpoint bindings.
		relStmt, err := st.Prepare(`
SELECT ae.space_uuid AS &endpointBindingRow.space_uuid,
       cr.name AS &endpointBindingRow.endpoint
FROM application_endpoint AS ae
JOIN charm_relation AS cr ON cr.uuid = ae.charm_relation_uuid
WHERE ae.application_uuid = $appUUIDParam.uuid
`, endpointBindingRow{}, appUUIDParam{})
		if err != nil {
			return nil, errors.Capture(err)
		}

		var relBindings []endpointBindingRow
		err = tx.Query(ctx, relStmt, appUUIDParam{UUID: app.UUID}).GetAll(&relBindings)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return nil, errors.Capture(err)
		}

		for _, b := range relBindings {
			if b.SpaceUUID.Valid {
				bindings[b.Endpoint] = network.SpaceUUID(b.SpaceUUID.V)
			} else {
				bindings[b.Endpoint] = defaultSpace
			}
		}

		// Get extra endpoint bindings.
		extraStmt, err := st.Prepare(`
SELECT aee.space_uuid AS &endpointBindingRow.space_uuid,
       ceb.name AS &endpointBindingRow.endpoint
FROM application_extra_endpoint AS aee
JOIN charm_extra_binding AS ceb ON ceb.uuid = aee.charm_extra_binding_uuid
WHERE aee.application_uuid = $appUUIDParam.uuid
`, endpointBindingRow{}, appUUIDParam{})
		if err != nil {
			return nil, errors.Capture(err)
		}

		var extraBindings []endpointBindingRow
		err = tx.Query(ctx, extraStmt, appUUIDParam{UUID: app.UUID}).GetAll(&extraBindings)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return nil, errors.Capture(err)
		}

		for _, b := range extraBindings {
			if b.SpaceUUID.Valid {
				bindings[b.Endpoint] = network.SpaceUUID(b.SpaceUUID.V)
			} else {
				bindings[b.Endpoint] = defaultSpace
			}
		}

		result[app.Name] = bindings
	}

	return result, nil
}

// getVolumeParams fetches volume provisioning params for the machine.
func (st *ModelState) getVolumeParams(
	ctx context.Context,
	tx *sqlair.TX,
	machineUUID string,
) ([]provisioning.VolumeProvisioningParams, []provisioning.VolumeAttachmentProvisioningParams, error) {
	// TODO(provisioning): Implement volume params query.
	// This needs to query storage_volume, storage_volume_attachment,
	// storage_pool, and storage_pool_attribute tables.
	// For now, return empty slices — volumes will be added in a follow-up.
	return nil, nil, nil
}

// getRootDiskStoragePool fetches the storage pool for the root disk if
// the root-disk-source constraint is set.
func (st *ModelState) getRootDiskStoragePool(
	ctx context.Context,
	tx *sqlair.TX,
	cons constraints.Value,
) (*provisioning.StoragePool, error) {
	if !cons.HasRootDiskSource() {
		return nil, nil
	}

	stmt, err := st.Prepare(`
SELECT (sp.*) AS (&storagePoolRow.*),
       (spa.*) AS (&storagePoolAttrRow.*)
FROM storage_pool AS sp
LEFT JOIN storage_pool_attribute AS spa ON spa.storage_pool_uuid = sp.uuid
WHERE sp.name = $storagePoolNameParam.name
`, storagePoolRow{}, storagePoolAttrRow{}, storagePoolNameParam{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var poolRows []storagePoolRow
	var attrRows []storagePoolAttrRow
	err = tx.Query(ctx, stmt, storagePoolNameParam{Name: *cons.RootDiskSource}).GetAll(&poolRows, &attrRows)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Capture(err)
	}

	if len(poolRows) == 0 {
		return nil, nil
	}

	attrs := make(map[string]string, len(attrRows))
	for _, a := range attrRows {
		if a.Key != "" {
			attrs[a.Key] = a.Value
		}
	}

	return &provisioning.StoragePool{
		Provider: poolRows[0].Provider,
		Attrs:    attrs,
	}, nil
}

// getAllSpaces fetches all spaces with their subnets and availability zones.
func (st *ModelState) getAllSpaces(
	ctx context.Context,
	tx *sqlair.TX,
) (network.SpaceInfos, error) {
	stmt, err := st.Prepare(`
SELECT &spaceSubnetRow.*
FROM v_space_subnet
`, spaceSubnetRow{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var rows []spaceSubnetRow
	err = tx.Query(ctx, stmt).GetAll(&rows)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Capture(err)
	}

	return decodeSpaceSubnetRows(rows), nil
}

// getModelConfigValues fetches model config values relevant to provisioning.
func (st *ModelState) getModelConfigValues(
	ctx context.Context,
	tx *sqlair.TX,
) (map[string]any, string, map[string]string, bool, error) {
	// TODO(provisioning): Implement model config query.
	// Query model_config for cloudinit-userdata, image-stream,
	// resource-tags. For now return defaults.
	return nil, "released", nil, false, nil
}

// getModelInfo fetches the model name, cloud type, region, and endpoint.
func (st *ModelState) getModelInfo(
	ctx context.Context,
	tx *sqlair.TX,
) (string, string, string, string, error) {
	stmt, err := st.Prepare(`
SELECT m.name AS &modelInfoRow.name,
       m.cloud_type AS &modelInfoRow.cloud_type,
       m.cloud_region AS &modelInfoRow.cloud_region
FROM model AS m
`, modelInfoRow{})
	if err != nil {
		return "", "", "", "", errors.Capture(err)
	}

	var row modelInfoRow
	err = tx.Query(ctx, stmt).Get(&row)
	if err != nil {
		return "", "", "", "", errors.Errorf("querying model info: %w", err)
	}

	// TODO(provisioning): Cloud endpoint is not stored in the model table.
	// It would need to be fetched from a cloud_region table or similar.
	return row.Name, row.CloudType, row.CloudRegion, "", nil
}

// getCachedImageMetadata fetches cached image metadata matching the
// machine's base and arch constraints.
func (st *ModelState) getCachedImageMetadata(
	ctx context.Context,
	tx *sqlair.TX,
	machineBase corebase.Base,
	cons constraints.Value,
) ([]provisioning.CloudImageMetadata, error) {
	// TODO(provisioning): Implement cached image metadata query.
	// Query cloud_image_metadata filtered by version and arch.
	return nil, nil
}

// decodeConstraintRows converts constraint query rows into a
// constraints.Value. This mirrors the pattern used in the machine state.
func decodeConstraintRows(rows []constraintRow) constraints.Value {
	if len(rows) == 0 {
		return constraints.Value{}
	}

	first := rows[0]
	cons := domainconstraints.Constraints{}

	if first.Arch.Valid {
		cons.Arch = &first.Arch.String
	}
	if first.CPUCores.Valid {
		v := uint64(first.CPUCores.V)
		cons.CpuCores = &v
	}
	if first.CPUPower.Valid {
		v := uint64(first.CPUPower.V)
		cons.CpuPower = &v
	}
	if first.Mem.Valid {
		v := uint64(first.Mem.V)
		cons.Mem = &v
	}
	if first.RootDisk.Valid {
		v := uint64(first.RootDisk.V)
		cons.RootDisk = &v
	}
	if first.RootDiskSource.Valid {
		cons.RootDiskSource = &first.RootDiskSource.String
	}
	if first.InstanceRole.Valid {
		cons.InstanceRole = &first.InstanceRole.String
	}
	if first.InstanceType.Valid {
		cons.InstanceType = &first.InstanceType.String
	}
	if first.ContainerType.Valid {
		ct := instance.ContainerType(first.ContainerType.String)
		cons.Container = &ct
	}
	if first.VirtType.Valid {
		cons.VirtType = &first.VirtType.String
	}
	if first.AllocatePublicIP.Valid {
		cons.AllocatePublicIP = &first.AllocatePublicIP.Bool
	}
	if first.ImageID.Valid {
		cons.ImageID = &first.ImageID.String
	}

	// Collect multi-valued fields from all rows (tags, spaces, zones).
	var spaceConstraints []domainconstraints.SpaceConstraint
	tagsSeen := make(map[string]bool)
	zonesSeen := make(map[string]bool)
	for _, row := range rows {
		if row.SpaceName.Valid {
			exclude := row.SpaceExclude.Valid && row.SpaceExclude.Bool
			spaceConstraints = append(spaceConstraints, domainconstraints.SpaceConstraint{
				SpaceName: row.SpaceName.String,
				Exclude:   exclude,
			})
		}
		if row.Tag.Valid && !tagsSeen[row.Tag.String] {
			tagsSeen[row.Tag.String] = true
			if cons.Tags == nil {
				tags := make([]string, 0)
				cons.Tags = &tags
			}
			*cons.Tags = append(*cons.Tags, row.Tag.String)
		}
		if row.Zone.Valid && !zonesSeen[row.Zone.String] {
			zonesSeen[row.Zone.String] = true
			if cons.Zones == nil {
				zones := make([]string, 0)
				cons.Zones = &zones
			}
			*cons.Zones = append(*cons.Zones, row.Zone.String)
		}
	}

	if len(spaceConstraints) > 0 {
		cons.Spaces = &spaceConstraints
	}

	return domainconstraints.EncodeConstraints(cons)
}

// decodeSpaceSubnetRows converts space-subnet query rows into SpaceInfos.
func decodeSpaceSubnetRows(rows []spaceSubnetRow) network.SpaceInfos {
	spaceMap := make(map[string]*network.SpaceInfo)

	for _, row := range rows {
		space, ok := spaceMap[row.SpaceUUID]
		if !ok {
			space = &network.SpaceInfo{
				ID:         network.SpaceUUID(row.SpaceUUID),
				Name:       network.SpaceName(row.SpaceName),
				ProviderId: network.Id(row.SpaceProviderID),
			}
			spaceMap[row.SpaceUUID] = space
		}

		if row.SubnetUUID != "" {
			subnet := network.SubnetInfo{
				ID:                network.Id(row.SubnetUUID),
				CIDR:              row.SubnetCIDR,
				ProviderId:        network.Id(row.SubnetProviderID),
				AvailabilityZones: nil,
			}
			if row.AvailabilityZone != "" {
				subnet.AvailabilityZones = []string{row.AvailabilityZone}
			}

			// Check if this subnet already exists (from a different AZ row).
			found := false
			for i := range space.Subnets {
				if string(space.Subnets[i].ID) == row.SubnetUUID {
					if row.AvailabilityZone != "" {
						space.Subnets[i].AvailabilityZones = append(
							space.Subnets[i].AvailabilityZones,
							row.AvailabilityZone,
						)
					}
					found = true
					break
				}
			}
			if !found {
				space.Subnets = append(space.Subnets, subnet)
			}
		}
	}

	result := make(network.SpaceInfos, 0, len(spaceMap))
	for _, space := range spaceMap {
		result = append(result, *space)
	}
	return result
}
