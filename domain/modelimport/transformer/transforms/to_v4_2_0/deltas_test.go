// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package to_v4_2_0

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/export/types/v4_1_0"
)

type deltasSuite struct{}

func TestDeltasSuite(t *testing.T) {
	tc.Run(t, &deltasSuite{})
}

// TestIpAddressNullableDevice verifies that a 4.1.0 device reference is
// carried through as a non-nil pointer, while an empty value maps to nil so
// import cannot fail the ip_address device foreign key.
func (s *deltasSuite) TestIpAddressNullableDevice(c *tc.C) {
	src := []v4_1_0.IpAddress{
		{UUID: "ip-uuid-1", NetNodeUUID: "node-uuid", DeviceUUID: "dev-uuid",
			AddressValue: "10.0.0.1", IsSecondary: new(false), IsShadow: new(false)},
		{UUID: "ip-uuid-2", NetNodeUUID: "node-uuid", DeviceUUID: "",
			AddressValue: "10.0.0.2", IsSecondary: new(false), IsShadow: new(false)},
	}

	got, err := deltas{}.IpAddress(c.Context(), src)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.HasLen, 2)
	c.Check(got[0].DeviceUUID, tc.DeepEquals, new("dev-uuid"))
	c.Check(got[1].DeviceUUID, tc.IsNil)
	c.Check(got[0].AddressValue, tc.Equals, "10.0.0.1")
}
