// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package to_v4_1_0

import (
	"testing"
	"time"

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

	got, err := deltas{}.RelationApplicationSetting(c.Context(), src, nil)
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

	got, err := deltas{}.RelationUnitSetting(c.Context(), src, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, []v4_1_0.RelationUnitSetting{
		{RelationUnitUUID: "ru-uuid", Key: "set", Value: "v"},
	})
}

func (s *deltasSuite) TestApplicationScale(c *tc.C) {
	scale := int64(2)
	target := int64(3)
	scaling := true
	src := []v4_0_12.ApplicationScale{{
		ApplicationUUID: "app-uuid",
		Scale:           &scale,
		ScaleTarget:     &target,
		Scaling:         &scaling,
	}}

	got, err := deltas{}.ApplicationScale(c.Context(), src, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, []v4_1_0.ApplicationScale{{
		ApplicationUUID: "app-uuid",
		Scale:           &scale,
		ScaleTarget:     &target,
		Scaling:         &scaling,
		StartOrdinal:    0,
	}})
}

// TestOfferLeavesDescriptionNil verifies that offers exported from a 4.0.12
// model, which has no offer description column, are carried through with a
// nil description.
func (s *deltasSuite) TestOfferLeavesDescriptionNil(c *tc.C) {
	src := []v4_0_12.Offer{
		{UUID: "offer-uuid", Name: "test-offer"},
	}

	got, err := deltas{}.Offer(c.Context(), src, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, []v4_1_0.Offer{
		{UUID: "offer-uuid", Name: "test-offer", Description: nil},
	})
}

func (s *deltasSuite) TestRelationUnitDepartureStartsEmpty(c *tc.C) {
	got, err := deltas{}.RelationUnitDeparture(c.Context(), &v4_0_12.ModelExport{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.HasLen, 0)
}

func (s *deltasSuite) TestUnitResourceAddsNameAndReconcilesDuplicates(c *tc.C) {
	oldTime := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	newTime := oldTime.Add(time.Hour)
	model := &v4_0_12.ModelExport{
		Resource: []v4_0_12.Resource{
			{UUID: "resource-a", CharmUUID: "charm-a", CharmResourceName: "foo"},
			{UUID: "resource-a2", CharmUUID: "charm-a", CharmResourceName: "foo"},
			{UUID: "resource-b", CharmUUID: "charm-b", CharmResourceName: "foo"},
			{UUID: "resource-bar", CharmUUID: "charm-b", CharmResourceName: "bar"},
		},
		Unit: []v4_0_12.Unit{
			{UUID: "unit-0", CharmUUID: "charm-b"},
			{UUID: "unit-1", CharmUUID: "charm-a"},
		},
	}
	src := []v4_0_12.UnitResource{
		{ResourceUUID: "resource-a", UnitUUID: "unit-0", AddedAt: newTime},
		{ResourceUUID: "resource-b", UnitUUID: "unit-0", AddedAt: oldTime},
		{ResourceUUID: "resource-bar", UnitUUID: "unit-0", AddedAt: newTime},
		{ResourceUUID: "resource-a", UnitUUID: "unit-1", AddedAt: oldTime},
		{ResourceUUID: "resource-a2", UnitUUID: "unit-1", AddedAt: newTime},
	}

	got, err := deltas{}.UnitResource(c.Context(), src, model)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, []v4_1_0.UnitResource{
		{
			ResourceUUID:      "resource-b",
			UnitUUID:          "unit-0",
			CharmResourceName: "foo",
			AddedAt:           oldTime,
		}, {
			ResourceUUID:      "resource-bar",
			UnitUUID:          "unit-0",
			CharmResourceName: "bar",
			AddedAt:           newTime,
		}, {
			ResourceUUID:      "resource-a2",
			UnitUUID:          "unit-1",
			CharmResourceName: "foo",
			AddedAt:           newTime,
		},
	})
}

func (s *deltasSuite) TestUnitResourceBreaksEqualTimestampTieByUUID(c *tc.C) {
	addedAt := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	model := &v4_0_12.ModelExport{
		Resource: []v4_0_12.Resource{
			{UUID: "resource-a", CharmUUID: "charm-a", CharmResourceName: "foo"},
			{UUID: "resource-b", CharmUUID: "charm-a", CharmResourceName: "foo"},
		},
		Unit: []v4_0_12.Unit{{UUID: "unit-0", CharmUUID: "charm-a"}},
	}

	for _, src := range [][]v4_0_12.UnitResource{
		{
			{ResourceUUID: "resource-a", UnitUUID: "unit-0", AddedAt: addedAt},
			{ResourceUUID: "resource-b", UnitUUID: "unit-0", AddedAt: addedAt},
		}, {
			{ResourceUUID: "resource-b", UnitUUID: "unit-0", AddedAt: addedAt},
			{ResourceUUID: "resource-a", UnitUUID: "unit-0", AddedAt: addedAt},
		},
	} {
		got, err := deltas{}.UnitResource(c.Context(), src, model)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(got, tc.DeepEquals, []v4_1_0.UnitResource{{
			ResourceUUID:      "resource-b",
			UnitUUID:          "unit-0",
			CharmResourceName: "foo",
			AddedAt:           addedAt,
		}})
	}
}

// TestTextIDsParsedToIntegers verifies that identifier columns that changed
// from TEXT to INTEGER are parsed, including NULL values.
func (s *deltasSuite) TestTextIDsParsedToIntegers(c *tc.C) {
	ctx := c.Context()

	agents, err := deltas{}.ApplicationAgent(ctx, []v4_0_12.ApplicationAgent{
		{ApplicationUUID: "a", PasswordHashAlgorithmID: new("0")},
		{ApplicationUUID: "b"},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(agents, tc.DeepEquals, []v4_1_0.ApplicationAgent{
		{ApplicationUUID: "a", PasswordHashAlgorithmID: new(int64(0))},
		{ApplicationUUID: "b"},
	})

	machines, err := deltas{}.Machine(ctx, []v4_0_12.Machine{
		{UUID: "m", LifeID: 1, PasswordHashAlgorithmID: new("2")},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machines[0].PasswordHashAlgorithmID, tc.DeepEquals, new(int64(2)))
	c.Check(machines[0].LifeID, tc.Equals, int64(1))

	units, err := deltas{}.Unit(ctx, []v4_0_12.Unit{
		{UUID: "u", PasswordHashAlgorithmID: new("3")},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(units[0].PasswordHashAlgorithmID, tc.DeepEquals, new(int64(3)))

	appPlatforms, err := deltas{}.ApplicationPlatform(ctx, []v4_0_12.ApplicationPlatform{
		{ApplicationUUID: "a", OsID: "1", ArchitectureID: 2},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(appPlatforms[0].OsID, tc.Equals, int64(1))
	c.Check(appPlatforms[0].ArchitectureID, tc.Equals, int64(2))

	machinePlatforms, err := deltas{}.MachinePlatform(ctx, []v4_0_12.MachinePlatform{
		{MachineUUID: "m", OsID: "1", ArchitectureID: 2},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machinePlatforms[0].OsID, tc.Equals, int64(1))

	configs, err := deltas{}.CharmConfig(ctx, []v4_0_12.CharmConfig{
		{CharmUUID: "c", Key: "k", TypeID: new("4")},
		{CharmUUID: "c", Key: "nil"},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(configs[0].TypeID, tc.DeepEquals, new(int64(4)))
	c.Check(configs[1].TypeID, tc.IsNil)

	lives, err := deltas{}.ModelLife(ctx, []v4_0_12.ModelLife{{ModelUUID: "m", LifeID: "1"}}, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lives, tc.DeepEquals, []v4_1_0.ModelLife{{ModelUUID: "m", LifeID: 1}})

	perms, err := deltas{}.SecretPermission(ctx, []v4_0_12.SecretPermission{
		{SecretID: "s", RoleID: 1, SubjectTypeID: 2, ScopeTypeID: "3"},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(perms[0].ScopeTypeID, tc.Equals, int64(3))
	c.Check(perms[0].SubjectTypeID, tc.Equals, int64(2))
}

// TestTextIDsRejectNonNumeric verifies that non-numeric values halt the
// transform rather than being dropped.
func (s *deltasSuite) TestTextIDsRejectNonNumeric(c *tc.C) {
	_, err := deltas{}.ModelLife(c.Context(), []v4_0_12.ModelLife{{ModelUUID: "m", LifeID: "alive"}}, nil)
	c.Check(err, tc.ErrorMatches, `invalid life_id "alive".*`)

	_, err = deltas{}.Unit(c.Context(), []v4_0_12.Unit{{UUID: "u", PasswordHashAlgorithmID: new("x")}}, nil)
	c.Check(err, tc.ErrorMatches, `invalid password_hash_algorithm_id "x".*`)
}
