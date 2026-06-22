// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package to_v4_1_0

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/export/types/v4_0_11"
	"github.com/juju/juju/domain/export/types/v4_1_0"
)

type deltasSuite struct{}

func TestDeltasSuite(t *testing.T) {
	tc.Run(t, &deltasSuite{})
}

// TestRelationApplicationSettingFlattensValue verifies that a set value is
// carried through and a NULL value collapses to the empty string, matching the
// NOT NULL value column introduced in 4.1.0.
func (s *deltasSuite) TestRelationApplicationSettingFlattensValue(c *tc.C) {
	src := []v4_0_11.RelationApplicationSetting{
		{RelationEndpointUUID: "re-uuid", Key: "set", Value: new("v")},
		{RelationEndpointUUID: "re-uuid", Key: "null", Value: nil},
	}

	got, err := deltas{}.RelationApplicationSetting(c.Context(), src)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, []v4_1_0.RelationApplicationSetting{
		{RelationEndpointUUID: "re-uuid", Key: "set", Value: "v"},
		{RelationEndpointUUID: "re-uuid", Key: "null", Value: ""},
	})
}

// TestRelationUnitSettingFlattensValue verifies that a set value is carried
// through and a NULL value collapses to the empty string.
func (s *deltasSuite) TestRelationUnitSettingFlattensValue(c *tc.C) {
	src := []v4_0_11.RelationUnitSetting{
		{RelationUnitUUID: "ru-uuid", Key: "set", Value: new("v")},
		{RelationUnitUUID: "ru-uuid", Key: "null", Value: nil},
	}

	got, err := deltas{}.RelationUnitSetting(c.Context(), src)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, []v4_1_0.RelationUnitSetting{
		{RelationUnitUUID: "ru-uuid", Key: "set", Value: "v"},
		{RelationUnitUUID: "ru-uuid", Key: "null", Value: ""},
	})
}
