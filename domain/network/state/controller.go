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
	domainnetwork "github.com/juju/juju/domain/network"
	networkinternal "github.com/juju/juju/domain/network/internal"
	"github.com/juju/juju/internal/errors"
)

// GetControllerUnitNetwork reads a controller unit's network facts atomically.
// The model must be the controller model and the unit must belong to its
// controller application. Alive and Dying applications/units are accepted;
// Dead units return UnitIsDead and missing/unrelated units return UnitNotFound.
// A missing model record is an error, not an empty address snapshot.
func (st *State) GetControllerUnitNetwork(ctx context.Context, name string) (networkinternal.ControllerUnitNetwork, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return networkinternal.ControllerUnitNetwork{}, errors.Capture(err)
	}
	type controllerModel struct {
		Type         model.ModelType `db:"type"`
		IsController bool            `db:"is_controller_model"`
	}
	modelQuery, err := st.Prepare(`
SELECT m.type AS &controllerModel.type,
       m.is_controller_model AS &controllerModel.is_controller_model
FROM model AS m`, controllerModel{})
	if err != nil {
		return networkinternal.ControllerUnitNetwork{}, errors.Capture(err)
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
		return networkinternal.ControllerUnitNetwork{}, errors.Capture(err)
	}
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
WHERE ipa.net_node_uuid = $controllerUnit.net_node_uuid`, controllerUnit{}, controllerAPIAddress{})
	if err != nil {
		return networkinternal.ControllerUnitNetwork{}, errors.Capture(err)
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
WHERE nnfa.net_node_uuid = $controllerUnit.net_node_uuid`, controllerUnit{}, fqdn{})
	if err != nil {
		return networkinternal.ControllerUnitNetwork{}, errors.Capture(err)
	}
	var result networkinternal.ControllerUnitNetwork
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var m controllerModel
		if err := tx.Query(ctx, modelQuery).Get(&m); err != nil {
			return errors.Errorf("reading controller model: %w", err)
		}
		if !m.IsController {
			return errors.Errorf("controller network requires the controller model: %w", coreerrors.NotValid)
		}
		if !m.Type.IsValid() {
			return errors.Errorf("invalid controller model type %q", m.Type)
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
		var ips []controllerAPIAddress
		if err := tx.Query(ctx, ipQuery, u).GetAll(&ips); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("reading controller IP addresses: %w", err)
		}
		addresses, err := encodeControllerAPIAddresses(ips)
		if err != nil {
			return errors.Capture(err)
		}
		var names []fqdn
		if err := tx.Query(ctx, fqdnQuery, u).GetAll(&names); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("reading controller DNS addresses: %w", err)
		}
		for _, name := range names {
			addresses = append(addresses, domainnetwork.ControllerAPIAddress{
				SpaceID: network.AlphaSpaceId,
				Origin:  network.OriginProvider,
				Value:   name.Address, Type: network.HostName, Scope: name.Scope,
			})
		}
		result = networkinternal.ControllerUnitNetwork{ModelType: m.Type, Addresses: addresses}
		return nil
	})
	if err != nil {
		return networkinternal.ControllerUnitNetwork{}, errors.Capture(err)
	}
	return result, nil
}

// NamespacesForWatchControllerNetwork covers every mutable input to controller
// network queries, including changes that replace a unit's network node.
// Application creation/removal also covers its immutable controller marker.
func (*State) NamespacesForWatchControllerNetwork() []string {
	return []string{
		"unit", "application", "ip_address",
		"fqdn_address", "net_node_fqdn_address", "link_layer_device", "subnet", "space",
	}
}
