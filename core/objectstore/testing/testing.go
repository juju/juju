// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/tc"

	coreobjectstore "github.com/juju/juju/core/objectstore"
)

// GenObjectStoreUUID can be used in testing for generating a objectstore UUID
// that is checked for subsequent errors using the test suit's go check
// instance.
func GenObjectStoreUUID(c *tc.C) coreobjectstore.UUID {
	id, err := coreobjectstore.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	return id
}
