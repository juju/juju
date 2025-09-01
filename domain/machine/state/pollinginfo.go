// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	domainmachine "github.com/juju/juju/domain/machine"
	"github.com/juju/juju/internal/errors"
)

// GetPollingInfos returns the polling information for the given machines.
func (st *State) GetPollingInfos(ctx context.Context, machineNames []string) (domainmachine.PollingInfos, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	type pollingInfo struct {
		UUID        string `db:"uuid"`
		Name        string `db:"name"`
		InstanceID  string `db:"instance_id"`
		DeviceCount int    `db:"device_count"`
	}

	type names []string

	stmt, err := st.Prepare(`
SELECT 
    m.uuid AS &pollingInfo.uuid,
	m.name AS &pollingInfo.name,
	mci.instance_id AS &pollingInfo.instance_id,
	COUNT(lld.uuid) AS &pollingInfo.device_count
FROM      machine AS m
JOIN      machine_cloud_instance AS mci ON m.uuid = mci.machine_uuid
LEFT JOIN link_layer_device AS lld ON m.net_node_uuid = lld.net_node_uuid
WHERE     m.name IN ($names[:])
GROUP BY  m.uuid, m.name, mci.instance_id
`, names{}, pollingInfo{})
	if err != nil {
		return nil, errors.Errorf("preparing query of polling info for machines %s: %w", machineNames, err)
	}

	var infos []pollingInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, names(machineNames)).GetAll(&infos)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Errorf("retrieving polling info for machines %s: %w", machineNames, err)
	}

	// It shouldn't happen, but if it does, log it.
	if len(infos) != len(machineNames) {
		unknown := set.NewStrings(machineNames...)
		for _, info := range infos {
			unknown.Remove(info.Name)
		}
		// Hitting this log line means that the caller asks for polling info for
		// machines that are not in the database. It shouldn't happen, since polling
		// info is only requested for machines that are alive and in the database,
		// e.g., by the instance-poller. This code is triggered by a change in
		// the machine table.
		// If this log line is hit, the more probable cause is a logic change
		// in the instance-poller worker (see internal/worker/instancepoller)
		st.logger.Warningf(ctx, "fetching polling info for unknown machines: %s", unknown.Values())
	}

	return transform.Slice(infos, func(info pollingInfo) domainmachine.PollingInfo {
		return domainmachine.PollingInfo{
			MachineUUID:         machine.UUID(info.UUID),
			MachineName:         machine.Name(info.Name),
			InstanceID:          instance.Id(info.InstanceID),
			ExistingDeviceCount: info.DeviceCount,
		}
	}), nil
}
