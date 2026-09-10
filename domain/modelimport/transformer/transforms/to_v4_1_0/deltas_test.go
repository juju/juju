// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package to_v4_1_0

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/export/types/v4_0_12"
	"github.com/juju/juju/domain/export/types/v4_1_0"
)

type deltasSuite struct{}

func TestDeltasSuite(t *testing.T) {
	tc.Run(t, &deltasSuite{})
}

// TestRelationApplicationSettingDropsEmptyValues verifies that a set value is
// carried through while NULL and empty values are dropped, since the 4.1.0
// value column is NOT NULL and disallows the empty string.
func (s *deltasSuite) TestRelationApplicationSettingDropsEmptyValues(c *tc.C) {
	src := []v4_0_12.RelationApplicationSetting{
		{RelationEndpointUUID: "re-uuid", Key: "set", Value: new("v")},
		{RelationEndpointUUID: "re-uuid", Key: "null", Value: nil},
		{RelationEndpointUUID: "re-uuid", Key: "empty", Value: new("")},
	}

	got, err := deltas{}.RelationApplicationSetting(c.Context(), src)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, []v4_1_0.RelationApplicationSetting{
		{RelationEndpointUUID: "re-uuid", Key: "set", Value: "v"},
	})
}

// TestRelationUnitSettingDropsEmptyValues verifies that a set value is carried
// through while NULL and empty values are dropped, since the 4.1.0 value column
// is NOT NULL and disallows the empty string.
func (s *deltasSuite) TestRelationUnitSettingDropsEmptyValues(c *tc.C) {
	src := []v4_0_12.RelationUnitSetting{
		{RelationUnitUUID: "ru-uuid", Key: "set", Value: new("v")},
		{RelationUnitUUID: "ru-uuid", Key: "null", Value: nil},
		{RelationUnitUUID: "ru-uuid", Key: "empty", Value: new("")},
	}

	got, err := deltas{}.RelationUnitSetting(c.Context(), src)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, []v4_1_0.RelationUnitSetting{
		{RelationUnitUUID: "ru-uuid", Key: "set", Value: "v"},
	})
}
