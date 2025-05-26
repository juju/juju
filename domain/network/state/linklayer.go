// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/errors"
)

// SetMachineNetConfig updates the network configuration for the machine with
// the input net node UUID.
//   - New devices and their addresses are insert with origin = "machine".
//   - Existing devices and addresses are updated (unchanged rows will not cause
//     triggers to fire).
//   - Existing addresses not in the input will be deleted if origin == "machine"
//   - Devices not observed will be deleted if they have no addresses.
func (st *State) SetMachineNetConfig(ctx context.Context, nodeUUID string, nics []network.NetInterface) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	nUUID := entityUUID{UUID: nodeUUID}

	// TODO (manadart 2025-04-29): This is temporary and serves to get us
	// operational with addresses on Dqlite.
	// We will set devices and addresses for any given machine *one time*
	// with subsequent updates being a no-op until we add the full
	// reconciliation logic.
	var devCount countResult
	devCountSql := "SELECT COUNT(*) AS &countResult.count FROM link_layer_device WHERE net_node_uuid = $entityUUID.uuid"
	devCountStmt, err := st.Prepare(devCountSql, nUUID, devCount)
	if err != nil {
		return errors.Errorf("preparing device count statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, devCountStmt, nUUID).Get(&devCount); err != nil {
			return errors.Errorf("running device count statement: %w", err)
		}

		// If we've done it for this machine before, bug out.
		if devCount.Count > 0 {
			return nil
		}

		// Otherwise, insert the data.
		nicsToInsert, nicNameToUUID, err := st.reconcileNetConfigDevices(nodeUUID, nics)
		if err != nil {
			return errors.Errorf("reconciling incoming network devices: %w", err)
		}

		if err = st.insertLinkLayerDevices(ctx, tx, nicsToInsert); err != nil {
			return errors.Errorf("inserting link layer devices: %w", err)
		}

		addrsToInsert, err := st.reconcileNetConfigAddresses(nodeUUID, nics, nicNameToUUID)
		if err != nil {
			return errors.Errorf("reconciling incoming ip addresses: %w", err)
		}

		if len(addrsToInsert) == 0 {
			return nil
		}

		if err = st.insertIPAddresses(ctx, tx, addrsToInsert); err != nil {
			return errors.Errorf("inserting IP addresses: %w", err)
		}

		return nil
	})

	return errors.Capture(err)
}

func (st *State) reconcileNetConfigDevices(
	nodeUUID string, nics []network.NetInterface,
) ([]linkLayerDeviceDML, map[string]string, error) {
	// TODO (manadart 2025-04-30): This will have to return more types for DNS
	// nameservers/addresses, provider ID entries etc.

	// The idea here will be to set the UUIDs that we know from querying
	// existing devices, then generate new ones for devices we don't have yet.
	// Interfaces that we do not observe will be deleted along with their
	// addresses if they have an origin of "machine".
	nameToUUID := make(map[string]string, len(nics))
	for _, n := range nics {
		nicUUID, err := network.NewInterfaceUUID()
		if err != nil {
			return nil, nil, errors.Capture(err)
		}
		nameToUUID[n.Name] = nicUUID.String()
	}

	nicsDML := make([]linkLayerDeviceDML, len(nics))
	for i, n := range nics {
		var err error
		if nicsDML[i], err = netInterfaceToDML(n, nodeUUID, nameToUUID); err != nil {
			return nil, nil, errors.Capture(err)
		}
	}

	return nicsDML, nameToUUID, nil
}

func (st *State) insertLinkLayerDevices(ctx context.Context, tx *sqlair.TX, devs []linkLayerDeviceDML) error {
	stmt, err := st.Prepare(
		"INSERT INTO link_layer_device (*) VALUES ($linkLayerDeviceDML.*)", devs[0])
	if err != nil {
		return errors.Errorf("preparing device insert statement: %w", err)
	}

	err = tx.Query(ctx, stmt, devs).Run()
	if err != nil {
		return errors.Errorf("running device insert statement: %w", err)
	}

	return nil
}

func (st *State) reconcileNetConfigAddresses(
	nodeUUID string, nics []network.NetInterface, nicNameToUUID map[string]string,
) ([]ipAddressDML, error) {
	var addrsDML []ipAddressDML

	for _, n := range nics {
		// If we don't know this NIC, we can assume that is a deletion candidate.
		devUUID, ok := nicNameToUUID[n.Name]
		if !ok {
			continue
		}

		// As with the interfaces, this will really be formed by querying the
		// existing addresses in addition to creating UUIDs for new ones.
		addrToUUID := make(map[string]string, len(n.Addrs))
		for _, a := range n.Addrs {
			addrUUID, err := network.NewAddressUUID()
			if err != nil {
				return nil, errors.Capture(err)
			}
			addrToUUID[a.AddressValue] = addrUUID.String()
		}

		for _, a := range n.Addrs {
			addrDML, err := netAddrToDML(a, nodeUUID, devUUID, addrToUUID)
			if err != nil {
				return nil, errors.Capture(err)
			}

			addrsDML = append(addrsDML, addrDML)
		}
	}

	return addrsDML, nil
}

func (st *State) insertIPAddresses(ctx context.Context, tx *sqlair.TX, addrs []ipAddressDML) error {
	stmt, err := st.Prepare(
		"INSERT INTO ip_address (*) VALUES ($ipAddressDML.*)", addrs[0])
	if err != nil {
		return errors.Errorf("preparing address insert statement: %w", err)
	}

	err = tx.Query(ctx, stmt, addrs).Run()
	if err != nil {
		return errors.Errorf("running address insert statement: %w", err)
	}

	return nil
}
