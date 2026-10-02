// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	modelerrors "github.com/juju/juju/domain/model/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/errors"
)

// GetControllerUnitNetwork reads a controller unit's addresses atomically.
// The model must be the controller model and the unit must belong to its
// controller application. Alive and Dying applications/units are accepted;
// Dead units return UnitIsDead and missing/unrelated units return UnitNotFound.
// A missing model record returns [modelerrors.NotFound].
func (st *State) GetControllerUnitNetwork(ctx context.Context, name string) (domainnetwork.ControllerAPIAddresses, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	type controllerUnit struct {
		NetNodeUUID     string    `db:"net_node_uuid"`
		Life            life.Life `db:"life_id"`
		ApplicationLife life.Life `db:"application_life_id"`
	}
	unitQuery, err := st.Prepare(`
SELECT u.net_node_uuid AS &controllerUnit.net_node_uuid,
       u.life_id AS &controllerUnit.life_id,
       a.life_id AS &controllerUnit.application_life_id
FROM unit AS u
JOIN application AS a ON a.uuid = u.application_uuid
JOIN application_controller AS ac ON ac.application_uuid = a.uuid
WHERE u.name = $unitName.name`, controllerUnit{}, unitName{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	var result domainnetwork.ControllerAPIAddresses
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkControllerNetworkModel(ctx, tx); err != nil {
			return errors.Capture(err)
		}
		var u controllerUnit
		if err := tx.Query(ctx, unitQuery, unitName{Name: unit.Name(name)}).Get(&u); errors.Is(err, sqlair.ErrNoRows) {
			return applicationerrors.UnitNotFound
		} else if err != nil {
			return errors.Errorf("reading controller unit: %w", err)
		}
		if u.Life == life.Dead || u.ApplicationLife == life.Dead {
			return applicationerrors.UnitIsDead
		}
		result, err = st.getControllerNodeAddresses(ctx, tx, u.NetNodeUUID)
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return result, nil
}

// GetControllerServiceAddresses reads IP/DNS addresses from the controller
// application's Kubernetes Service, independently of any unit. Missing Services
// and missing or Dead controller applications return an empty selection. Dying
// applications remain eligible. The model must be the controller model.
// A missing model record returns [modelerrors.NotFound].
func (st *State) GetControllerServiceAddresses(ctx context.Context) (domainnetwork.ControllerAPIAddresses, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	query, err := st.Prepare(`
SELECT ks.net_node_uuid AS &entityUUID.uuid
FROM k8s_service AS ks
JOIN application AS a ON a.uuid = ks.application_uuid
JOIN application_controller AS ac ON ac.application_uuid = a.uuid
WHERE a.life_id != 2 /* Dead */`, entityUUID{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	var result domainnetwork.ControllerAPIAddresses
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// A retried transaction may no longer have a Service association.
		result = nil
		if err := st.checkControllerNetworkModel(ctx, tx); err != nil {
			return errors.Capture(err)
		}
		var node entityUUID
		if err := tx.Query(ctx, query).Get(&node); errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("reading controller Service: %w", err)
		}
		result, err = st.getControllerNodeAddresses(ctx, tx, node.UUID)
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return result, nil
}

func (st *State) checkControllerNetworkModel(ctx context.Context, tx *sqlair.TX) error {
	type controllerModel struct {
		IsController bool `db:"is_controller_model"`
	}
	modelQuery, err := st.Prepare(`
SELECT m.is_controller_model AS &controllerModel.is_controller_model
FROM model AS m`, controllerModel{})
	if err != nil {
		return errors.Capture(err)
	}
	var m controllerModel
	if err := tx.Query(ctx, modelQuery).Get(&m); errors.Is(err, sqlair.ErrNoRows) {
		return modelerrors.NotFound
	} else if err != nil {
		return errors.Errorf("reading controller model: %w", err)
	}
	if !m.IsController {
		return errors.Errorf("controller network requires the controller model: %w", coreerrors.NotValid)
	}
	return nil
}

func (st *State) getControllerNodeAddresses(ctx context.Context, tx *sqlair.TX, netNodeUUID string) (domainnetwork.ControllerAPIAddresses, error) {
	ipQuery, err := st.Prepare(`
SELECT ipa.address_value AS &controllerAPIAddress.address_value,
       iact.name AS &controllerAPIAddress.config_type_name,
       iat.name AS &controllerAPIAddress.type_name,
       iao.name AS &controllerAPIAddress.origin_name,
       ias.name AS &controllerAPIAddress.scope_name,
       ipa.device_uuid AS &controllerAPIAddress.device_uuid,
       sn.space_uuid AS &controllerAPIAddress.space_uuid,
       sn.cidr AS &controllerAPIAddress.cidr,
       lld.device_type_id AS &controllerAPIAddress.device_type_id
FROM ip_address AS ipa
JOIN link_layer_device AS lld ON lld.uuid = ipa.device_uuid
    AND lld.net_node_uuid = ipa.net_node_uuid
JOIN ip_address_config_type AS iact ON iact.id = ipa.config_type_id
JOIN ip_address_type AS iat ON iat.id = ipa.type_id
JOIN ip_address_origin AS iao ON iao.id = ipa.origin_id
JOIN ip_address_scope AS ias ON ias.id = ipa.scope_id
LEFT JOIN subnet AS sn ON sn.uuid = ipa.subnet_uuid
WHERE ipa.net_node_uuid = $entityUUID.uuid`, entityUUID{}, controllerAPIAddress{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	type fqdn struct {
		Address string        `db:"address"`
		Scope   network.Scope `db:"scope"`
	}
	fqdnQuery, err := st.Prepare(`
SELECT fa.address AS &fqdn.address, nas.name AS &fqdn.scope
FROM net_node_fqdn_address AS nnfa
JOIN fqdn_address AS fa ON fa.uuid = nnfa.address_uuid
JOIN network_address_scope AS nas ON nas.id = fa.scope_id
WHERE nnfa.net_node_uuid = $entityUUID.uuid`, entityUUID{}, fqdn{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	ident := entityUUID{UUID: netNodeUUID}
	var ips []controllerAPIAddress
	if err := tx.Query(ctx, ipQuery, ident).GetAll(&ips); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("reading controller IP addresses: %w", err)
	}
	addresses, err := encodeControllerAPIAddresses(ips)
	if err != nil {
		return nil, errors.Capture(err)
	}
	var names []fqdn
	if err := tx.Query(ctx, fqdnQuery, ident).GetAll(&names); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("reading controller DNS addresses: %w", err)
	}
	for _, name := range names {
		addresses = append(addresses, domainnetwork.ControllerAPIAddress{
			SpaceAddress: network.SpaceAddress{
				SpaceID: network.AlphaSpaceId,
				Origin:  network.OriginProvider,
				MachineAddress: network.MachineAddress{
					Value: name.Address, Type: network.HostName,
					Scope: name.Scope,
				},
			},
		})
	}
	return addresses, nil
}

// GetModelType returns the type of the current model. A missing model record
// returns [modelerrors.NotFound]. An invalid model type returns an error.
func (st *State) GetModelType(ctx context.Context) (model.ModelType, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}
	type modelType struct {
		Type model.ModelType `db:"type"`
	}
	query, err := st.Prepare(`
SELECT m.type AS &modelType.type
FROM model AS m`, modelType{})
	if err != nil {
		return "", errors.Capture(err)
	}
	var m modelType
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query).Get(&m)
		if errors.Is(err, sqlair.ErrNoRows) {
			return modelerrors.NotFound
		}
		return err
	}); err != nil {
		return "", errors.Errorf("reading model type: %w", err)
	}
	if !m.Type.IsValid() {
		return "", errors.Errorf("invalid model type %q", m.Type)
	}
	return m.Type, nil
}

// NamespacesForWatchControllerNetwork covers every mutable input to controller
// network queries, including changes that replace a unit's network node.
// Application creation/removal also covers its immutable controller marker.
func (*State) NamespacesForWatchControllerNetwork() []string {
	return []string{
		"unit", "application", "k8s_service", "ip_address",
		"fqdn_address", "net_node_fqdn_address", "link_layer_device", "subnet", "space",
	}
}
