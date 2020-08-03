// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrade

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/core/network"
)

var logger = loggo.GetLogger("juju.state.upgrade")

// The purpose of this module is to house types specific to upgrades,
// without adding types to the state package.
// For small upgrade functions closed over types are sufficient,
// but where that would make steps unwieldy, types can live here.

// OldAddress27 represents the address stored prior to version 2.7 in the
// collections: machines, cloudservices and cloudcontainers.
// Note that we can reuse this type for the new form because:
// - `omitempty` on SpaceName means we can set it as "" to remove.
// - The bson field `spaceid` is the same for the old SpaceProviderId.
//   and the new SpaceID.
type OldAddress27 struct {
	Value       string `bson:"value"`
	AddressType string `bson:"addresstype"`
	Scope       string `bson:"networkscope,omitempty"`
	Origin      string `bson:"origin,omitempty"`
	SpaceName   string `bson:"spacename,omitempty"`
	SpaceID     string `bson:"spaceid,omitempty"`
}

// Upgrade accepts an address and a name-to-ID space lookup and returns a
// new address representation based on whether space name/ID are populated.
// An error is returned if the address has a non-empty space name that we
// cannot map to an ID.
func (a OldAddress27) Upgrade(lookup network.SpaceInfos) (OldAddress27, error) {
	// Ignore zero-type addresses.
	if a.Value == "" {
		return a, nil
	}

	if a.SpaceName == "" {
		// If both fields are empty, populate with the default space ID.
		if a.SpaceID == "" {
			a.SpaceID = network.AlphaSpaceId
		} else {
			// If we have a space (provider) ID and no name, which should
			// not be possible in old addresses, then this address *looks*
			// like one that has already been converted.
			// Play it safe, leave it alone and log a warning.
			logger.Warningf("not converting address %q with empty space name and ID %q", a.Value, a.SpaceID)
		}
	} else {
		// On old versions of Juju, we did not convert the space names
		// on addresses that the instance poller received from MAAS.
		// We need to ensure that these can be matched with the names in the
		// spaces collected, which *are* converted when reload-spaces is run.
		spaceInfo := lookup.GetByName(network.ConvertSpaceName(a.SpaceName, nil))
		if spaceInfo == nil {
			return a, errors.NotFoundf("space with name: %q", a.SpaceName)
		}
		a.SpaceID = spaceInfo.ID
		a.SpaceName = ""
	}
	return a, nil
}

// OldPortsDoc28 represents a ports document prior to the 2.9 schema changes.
type OldPortsDoc28 struct {
	DocID     string              `bson:"_id"`
	ModelUUID string              `bson:"model-uuid"`
	MachineID string              `bson:"machine-id"`
	SubnetID  string              `bson:"subnet-id"`
	Ports     []OldPortRangeDoc28 `bson:"ports"`
	TxnRevno  int64               `bson:"txn-revno"`
}

// OldPortsDoc28 represents a port range entry document prior to the 2.9 schema changes.
type OldPortRangeDoc28 struct {
	UnitName string `bson:"unitname"`
	FromPort int    `bson:"fromport"`
	ToPort   int    `bson:"toport"`
	Protocol string `bson:"protocol"`
}
