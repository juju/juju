// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package settings

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
)

type settingsSuite struct{}

var _ = tc.Suite(&settingsSuite{})

func (*settingsSuite) TestItemChangeType(c *tc.C) {
	a := MakeAddition("key", "new-val")
	m := MakeModification("key", "old-val", "new-val")
	d := MakeDeletion("key", "old-val")

	c.Check(a.IsAddition(), jc.IsTrue)
	c.Check(m.IsModification(), jc.IsTrue)
	c.Check(d.IsDeletion(), jc.IsTrue)
}

func (*settingsSuite) TestItemChangesMapNonUniqueError(c *tc.C) {
	_, err := ItemChanges{
		MakeAddition("key", "new-val"),
		MakeAddition("key", "other-val"),
	}.Map()

	c.Assert(err, tc.ErrorMatches, `duplicated key in settings collection: "key"`)
}

func (*settingsSuite) TestItemChangesMapSuccess(c *tc.C) {
	mapped, err := ItemChanges{
		MakeAddition("key1", "new-val"),
		MakeModification("key2", "old-val", "other-val"),
		MakeDeletion("key3", "gone-val"),
	}.Map()

	c.Assert(err, jc.ErrorIsNil)
	c.Check(mapped, tc.DeepEquals, map[string]ItemChange{
		"key1": MakeAddition("key1", "new-val"),
		"key2": MakeModification("key2", "old-val", "other-val"),
		"key3": MakeDeletion("key3", "gone-val"),
	})
}

func (*settingsSuite) TestItemTypeString(c *tc.C) {
	a := MakeAddition("key1", "new-val")
	m := MakeModification("key2", "old-val", "other-val")
	d := MakeDeletion("key3", "gone-val")
	e := ItemChange{Type: 4, Key: "key4", OldValue: "old-val", NewValue: "new-val"}

	c.Check(a.String(), tc.Equals, "setting added: key1 = new-val")
	c.Check(m.String(), tc.Equals, "setting modified: key2 = other-val (was old-val)")
	c.Check(d.String(), tc.Equals, "setting deleted: key3 (was gone-val)")
	c.Check(e.String(), tc.Equals, "unknown setting change type 4: key4 = new-val (was old-val)")
}

func (*settingsSuite) TestApplyDeltaSource(c *tc.C) {
	original := ItemChanges{
		MakeModification("key2", "older-val", "less-new-val"),
		MakeAddition("key4", "older-gone-val"),
		MakeDeletion("key5", "first-deleted-val"),
		MakeDeletion("key6", "reverted-deleted-val"),
	}

	latest := ItemChanges{
		MakeAddition("key1", "new-val"),
		MakeModification("key2", "old-val", "other-val"),
		MakeModification("key3", "another-old-val", "another-val"),
		MakeDeletion("key4", "gone-val"),
		MakeAddition("key5", "re-added-val"),
	}

	// The old values present in original are represented against the
	// matching keys in latest.
	latest, err := latest.ApplyDeltaSource(original)
	c.Assert(err, jc.ErrorIsNil)

	exp := ItemChanges{
		MakeAddition("key1", "new-val"),
		MakeModification("key2", "older-val", "other-val"),
		MakeModification("key3", "another-old-val", "another-val"),
		MakeDeletion("key4", nil),
		MakeModification("key5", "first-deleted-val", "re-added-val"),
		MakeModification("key6", "reverted-deleted-val", "reverted-deleted-val"),
	}

	for i, got := range latest {
		c.Check(got, tc.DeepEquals, exp[i])
	}
}

func (*settingsSuite) TestEffectiveSettings(c *tc.C) {
	changes := ItemChanges{
		MakeAddition("key1", "new-val"),
		MakeModification("key2", "old-val", "other-val"),
		MakeDeletion("key3", "old-deleted-val"),
	}

	defaults := map[string]interface{}{
		"key2": "default-key2-val",
		"key3": "default-deleted-val",
	}

	exp := map[string]interface{}{
		"key1": "new-val",
		"key2": "other-val",
		"key3": "default-deleted-val",
	}
	c.Check(changes.EffectiveChanges(defaults), tc.DeepEquals, exp)
}
