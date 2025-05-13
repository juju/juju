// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package linklayerdevice

import (
	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
)

type deviceSuite struct {
	schematesting.ModelSuite
}

var _ = tc.Suite(&deviceSuite{})

// TestLinkLayerDeviceTypeDBValues ensures there's no skew between what's in the
// database table for device type and the typed consts used in the state packages.
func (s *deviceSuite) TestLinkLayerDeviceTypeDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, name FROM link_layer_device_type")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[DeviceType]string)
	for rows.Next() {
		var (
			id   int
			name string
		)
		err := rows.Scan(&id, &name)
		c.Assert(err, tc.ErrorIsNil)
		dbValues[DeviceType(id)] = name
	}
	c.Assert(dbValues, tc.DeepEquals, map[DeviceType]string{
		DeviceTypeUnknown:  "unknown",
		DeviceTypeLoopback: "loopback",
		DeviceTypeEthernet: "ethernet",
		DeviceType8021q:    "802.1q",
		DeviceTypeBond:     "bond",
		DeviceTypeBridge:   "bridge",
		DeviceTypeVXLAN:    "vxlan",
	})
}

// TestVirtualPortTypeDBValues ensures there's no skew between what's in the
// database table for virtual port type and the typed consts used in the state packages.
func (s *deviceSuite) TestVirtualPortTypeDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, name FROM virtual_port_type")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[VirtualPortType]string)
	for rows.Next() {
		var (
			id   int
			name string
		)
		err := rows.Scan(&id, &name)
		c.Assert(err, tc.ErrorIsNil)
		dbValues[VirtualPortType(id)] = name
	}
	c.Assert(dbValues, tc.DeepEquals, map[VirtualPortType]string{
		NonVirtualPortType:         "nonvirtualport",
		OpenVswitchVirtualPortType: "openvswitch",
	})
}
