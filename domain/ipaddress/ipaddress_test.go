// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ipaddress

import (
	"testing"

	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
)

type ipAddressSuite struct {
	schematesting.ModelSuite
}

func TestIpAddressSuite(t *testing.T) {
	tc.Run(t, &ipAddressSuite{})
}

// TestConfigTypeDBValues ensures there's no skew between what's in the
// database table for config type and the typed consts used in the state packages.
func (s *ipAddressSuite) TestConfigTypeDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, name FROM ip_address_config_type")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[ConfigType]string)
	for rows.Next() {
		var (
			id   int
			name string
		)
		err := rows.Scan(&id, &name)
		c.Assert(err, tc.ErrorIsNil)
		dbValues[ConfigType(id)] = name
	}
	c.Assert(dbValues, tc.DeepEquals, map[ConfigType]string{
		ConfigTypeUnknown:  "unknown",
		ConfigTypeDHCP:     "dhcp",
		ConfigTypeDHCPv6:   "dhcpv6",
		ConfigTypeSLAAC:    "slaac",
		ConfigTypeStatic:   "static",
		ConfigTypeManual:   "manual",
		ConfigTypeLoopback: "loopback",
	})
}

// TestScopeDBValues ensures there's no skew between what's in the
// database table for scope and the typed consts used in the state packages.
func (s *ipAddressSuite) TestScopeDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, name FROM ip_address_scope")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[Scope]string)
	for rows.Next() {
		var (
			id   int
			name string
		)
		err := rows.Scan(&id, &name)
		c.Assert(err, tc.ErrorIsNil)
		dbValues[Scope(id)] = name
	}
	c.Assert(dbValues, tc.DeepEquals, map[Scope]string{
		ScopeUnknown:      "unknown",
		ScopePublic:       "public",
		ScopeCloudLocal:   "local-cloud",
		ScopeMachineLocal: "local-machine",
		ScopeLinkLocal:    "link-local",
	})
}

// TestAddressTypeDBValues ensures there's no skew between what's in the
// database table for address type and the typed consts used in the state packages.
func (s *ipAddressSuite) TestAddressTypeDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, name FROM ip_address_type")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[AddressType]string)
	for rows.Next() {
		var (
			id   int
			name string
		)
		err := rows.Scan(&id, &name)
		c.Assert(err, tc.ErrorIsNil)
		dbValues[AddressType(id)] = name
	}
	c.Assert(dbValues, tc.DeepEquals, map[AddressType]string{
		AddressTypeIPv4: "ipv4",
		AddressTypeIPv6: "ipv6",
	})
}

// TestOriginDBValues ensures there's no skew between what's in the
// database table for origin and the typed consts used in the state packages.
func (s *ipAddressSuite) TestOriginDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, name FROM ip_address_origin")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[Origin]string)
	for rows.Next() {
		var (
			id   int
			name string
		)
		err := rows.Scan(&id, &name)
		c.Assert(err, tc.ErrorIsNil)
		dbValues[Origin(id)] = name
	}
	c.Assert(dbValues, tc.DeepEquals, map[Origin]string{
		OriginHost:     "machine",
		OriginProvider: "provider",
	})
}
