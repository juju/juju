// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import "github.com/juju/tc"

// removeSuite exists to test the various aspects of model remove options on
// [RemoveModelOptions].
type removeSuite struct{}

var _ = tc.Suite(&removeSuite{})

// TestRemoveModelOptionsWithoutDeleteDB is testing that [WithDeleteDB] sets
// the [RemoveModelOptions.deleteDB] field to true.
func (*removeSuite) TestRemoveModelOptionsWithDeleteDB(c *tc.C) {
	opts := RemoveModelOptions{
		deleteDB: false,
	}

	WithDeleteDB()(&opts)
	c.Check(opts.DeleteDB(), tc.Equals, true)
}

// TestRemoveModelOptionsWithoutDeleteDB is testing that [WithoutDeleteDB] sets
// the [RemoveModelOptions.deleteDB] field to false.
func (*removeSuite) TestRemoveModelOptionsWithoutDeleteDB(c *tc.C) {
	opts := RemoveModelOptions{
		deleteDB: true,
	}

	WithoutDeleteDB()(&opts)
	c.Check(opts.DeleteDB(), tc.Equals, false)
}
