// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/network"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationinternal "github.com/juju/juju/domain/application/internal"
	"github.com/juju/juju/domain/ipaddress"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/errors"
)

// UpsertK8sService replaces the addresses for an application's cloud Service.
// A changed provider ID retains the existing Service record and net node.
// An empty snapshot clears the addresses. It returns ApplicationNotFound if
// the application does not exist. Hostname scopes must be validated by the
// service layer before calling this method.
func (st *State) UpsertK8sService(ctx context.Context, applicationName, providerID string, args applicationinternal.UpsertK8sServiceArgs) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	queryApplication, err := st.Prepare(`
SELECT a.uuid AS &applicationDetails.uuid,
       c.source_id = 2 AS &applicationDetails.is_application_synthetic
FROM application AS a
JOIN charm AS c ON c.uuid = a.charm_uuid
WHERE a.name = $applicationDetails.name`, applicationDetails{})
	if err != nil {
		return errors.Capture(err)
	}
	queryService, err := st.Prepare(`
SELECT ks.* AS &k8sService.*
FROM k8s_service AS ks
WHERE ks.application_uuid = $k8sService.application_uuid`, k8sService{})
	if err != nil {
		return errors.Capture(err)
	}
	updateProviderID, err := st.Prepare(`
UPDATE k8s_service AS ks
SET provider_id = $k8sService.provider_id
WHERE ks.uuid = $k8sService.uuid`, k8sService{})
	if err != nil {
		return errors.Capture(err)
	}
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		app := applicationDetails{Name: applicationName}
		if err := tx.Query(ctx, queryApplication, app).Get(&app); errors.Is(err, sqlair.ErrNoRows) {
			return applicationerrors.ApplicationNotFound
		} else if err != nil {
			return errors.Errorf("querying application: %w", err)
		}
		if app.IsApplicationSynthetic {
			return errors.New("cannot upsert cloud service for synthetic application")
		}
		svc := k8sService{ApplicationUUID: app.UUID, ProviderID: providerID}
		err := tx.Query(ctx, queryService, svc).Get(&svc)
		if errors.Is(err, sqlair.ErrNoRows) {
			svc.UUID = args.ServiceUUID
			svc.NetNodeUUID = args.NetNodeUUID
			if err := st.createK8sService(ctx, tx, svc); err != nil {
				return errors.Capture(err)
			}
		} else if err != nil {
			return errors.Errorf("querying cloud service: %w", err)
		} else if svc.ProviderID != providerID {
			svc.ProviderID = providerID
			if err := tx.Query(ctx, updateProviderID, svc).Run(); err != nil {
				return errors.Errorf("updating cloud service provider ID: %w", err)
			}
		}
		if err := st.deleteK8sServiceAddresses(ctx, tx, svc.NetNodeUUID); err != nil {
			return errors.Capture(err)
		}
		var ipAddresses []applicationinternal.K8sServiceAddress
		for _, addr := range args.Addresses {
			if addr.AddressType() != network.HostName {
				ipAddresses = append(ipAddresses, addr)
				continue
			}
			if err := st.insertK8sServiceFQDN(ctx, tx, svc.NetNodeUUID, addr); err != nil {
				return errors.Capture(err)
			}
		}
		if len(ipAddresses) == 0 {
			return nil
		}
		deviceUUID, err := st.ensureK8sServiceDevice(ctx, tx, svc.NetNodeUUID, applicationName, args.DeviceUUID)
		if err != nil {
			return errors.Capture(err)
		}
		return st.insertK8sServiceIPAddresses(ctx, tx, svc.NetNodeUUID, deviceUUID, ipAddresses)
	})
}

func (st *State) createK8sService(ctx context.Context, tx *sqlair.TX, svc k8sService) error {
	insertNetNode, err := st.Prepare(`
INSERT INTO net_node (uuid) VALUES ($k8sService.net_node_uuid)`, svc)
	if err != nil {
		return errors.Capture(err)
	}
	insertService, err := st.Prepare(`
INSERT INTO k8s_service (*) VALUES ($k8sService.*)`, svc)
	if err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, insertNetNode, svc).Run(); err != nil {
		return errors.Errorf("creating cloud service net node: %w", err)
	}
	if err := tx.Query(ctx, insertService, svc).Run(); err != nil {
		return errors.Errorf("creating cloud service: %w", err)
	}
	return nil
}

func (st *State) deleteK8sServiceAddresses(ctx context.Context, tx *sqlair.TX, netNodeUUID string) error {
	node := dbUUID{UUID: netNodeUUID}
	// Only remove orphaned FQDNs belonging to this Service. Other net nodes
	// may still reference the same address and scope.
	selectFQDNs, err := st.Prepare(`
SELECT nnfa.address_uuid AS &dbUUID.uuid
FROM net_node_fqdn_address AS nnfa
WHERE nnfa.net_node_uuid = $dbUUID.uuid`, node)
	if err != nil {
		return errors.Capture(err)
	}
	var oldFQDNs []dbUUID
	if err := tx.Query(ctx, selectFQDNs, node).GetAll(&oldFQDNs); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Capture(err)
	}
	for _, query := range []string{
		`DELETE FROM ip_address WHERE net_node_uuid = $dbUUID.uuid`,
		`DELETE FROM net_node_fqdn_address WHERE net_node_uuid = $dbUUID.uuid`,
	} {
		stmt, err := st.Prepare(query, node)
		if err != nil {
			return errors.Capture(err)
		}
		if err := tx.Query(ctx, stmt, node).Run(); err != nil {
			return errors.Errorf("removing cloud service addresses: %w", err)
		}
	}
	if len(oldFQDNs) == 0 {
		return nil
	}
	addressUUIDs := make(uuids, len(oldFQDNs))
	for i, addr := range oldFQDNs {
		addressUUIDs[i] = addr.UUID
	}
	deleteOrphan, err := st.Prepare(`
WITH referenced AS (
    SELECT nnfa.address_uuid AS uuid FROM net_node_fqdn_address AS nnfa
)
DELETE FROM fqdn_address AS fa
WHERE fa.uuid IN ($uuids[:]) AND fa.uuid NOT IN referenced`, addressUUIDs)
	if err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, deleteOrphan, addressUUIDs).Run(); err != nil {
		return errors.Errorf("removing orphaned service hostnames: %w", err)
	}
	return nil
}

func (st *State) insertK8sServiceFQDN(ctx context.Context, tx *sqlair.TX, netNodeUUID string, addr applicationinternal.K8sServiceAddress) error {
	type hostname struct {
		UUID        string `db:"uuid"`
		Address     string `db:"address"`
		Scope       string `db:"scope"`
		NetNodeUUID string `db:"net_node_uuid"`
	}
	input := hostname{UUID: addr.UUID, Address: addr.Value, Scope: string(addr.Scope), NetNodeUUID: netNodeUUID}
	insert, err := st.Prepare(`
INSERT INTO fqdn_address (uuid, address, scope_id)
SELECT $hostname.uuid, $hostname.address, nas.id
FROM network_address_scope AS nas WHERE nas.name = $hostname.scope
ON CONFLICT (address, scope_id) DO NOTHING`, input)
	if err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, insert, input).Run(); err != nil {
		return errors.Errorf("inserting service hostname: %w", err)
	}
	link, err := st.Prepare(`
INSERT INTO net_node_fqdn_address (net_node_uuid, address_uuid)
SELECT $hostname.net_node_uuid, fa.uuid
FROM fqdn_address AS fa
JOIN network_address_scope AS nas ON nas.id = fa.scope_id
WHERE fa.address = $hostname.address AND nas.name = $hostname.scope
ON CONFLICT DO NOTHING`, input)
	if err != nil {
		return errors.Capture(err)
	}
	return errors.Capture(tx.Query(ctx, link, input).Run())
}

func (st *State) ensureK8sServiceDevice(ctx context.Context, tx *sqlair.TX, netNodeUUID, appName, deviceUUID string) (string, error) {
	device := k8sServiceDevice{
		UUID: deviceUUID, NetNodeID: netNodeUUID,
		DeviceTypeID:      int(domainnetwork.DeviceTypeUnknown),
		VirtualPortTypeID: int(domainnetwork.NonVirtualPortType),
	}
	query, err := st.Prepare(`
SELECT lld.uuid AS &k8sServiceDevice.uuid
FROM link_layer_device AS lld
WHERE lld.net_node_uuid = $k8sServiceDevice.net_node_uuid`, device)
	if err != nil {
		return "", errors.Capture(err)
	}
	if err := tx.Query(ctx, query, device).Get(&device); err == nil {
		return device.UUID, nil
	} else if !errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Capture(err)
	}
	insert, err := st.Prepare(`INSERT INTO link_layer_device (*) VALUES ($k8sServiceDevice.*)`, device)
	if err != nil {
		return "", errors.Capture(err)
	}
	if err := tx.Query(ctx, insert, device).Run(); err != nil {
		return "", errors.Capture(err)
	}
	return device.UUID, nil
}

func (st *State) insertK8sServiceIPAddresses(ctx context.Context, tx *sqlair.TX, netNodeUUID, deviceUUID string, addresses []applicationinternal.K8sServiceAddress) error {
	subnetUUIDs, err := st.k8sSubnetUUIDsByAddressType(ctx, tx)
	if err != nil {
		return errors.Capture(err)
	}
	insert, err := st.Prepare(`INSERT INTO ip_address (*) VALUES ($ipAddress.*)`, ipAddress{})
	if err != nil {
		return errors.Capture(err)
	}
	for _, addr := range addresses {
		subnetUUID, ok := subnetUUIDs[addr.AddressType()]
		if !ok {
			return errors.Errorf("subnet for address type %q not found", addr.AddressType())
		}
		input := ipAddress{
			AddressUUID: addr.UUID, Value: addr.Value, NetNodeUUID: netNodeUUID,
			SubnetUUID: subnetUUID, DeviceID: deviceUUID,
			ConfigTypeID: int(ipaddress.MarshallConfigType(addr.ConfigType)),
			TypeID:       int(ipaddress.MarshallAddressType(addr.AddressType())),
			OriginID:     int(ipaddress.MarshallOrigin(network.OriginProvider)),
			ScopeID:      int(ipaddress.MarshallScope(addr.AddressScope())),
		}
		if err := tx.Query(ctx, insert, input).Run(); err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}
