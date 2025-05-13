// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/relation"
)

// GenRelationUUID can be used in testing for generating a relation UUID
// that is checked for subsequent errors using the test suite's go check
// instance.
func GenRelationUUID(c *tc.C) relation.UUID {
	id, err := relation.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	return id
}

// GenEndpointUUID can be used in testing for generating an
// endpoint UUID that is checked for subsequent errors using the test suite's
// go check instance.
func GenEndpointUUID(c *tc.C) relation.EndpointUUID {
	id, err := relation.NewEndpointUUID()
	c.Assert(err, tc.ErrorIsNil)
	return id
}

// GenRelationUnitUUID can be used in testing for generating a relation
// Unit UUID that is checked for subsequent errors using the test suite's
// go check instance.
func GenRelationUnitUUID(c *tc.C) relation.UnitUUID {
	id, err := relation.NewUnitUUID()
	c.Assert(err, tc.ErrorIsNil)
	return id
}

// GenNewKey can be used in testing to generate a relation key from its string
// representation. It is checked for errors using the test suite's go check
// instance.
func GenNewKey(c *tc.C, keyString string) relation.Key {
	key, err := relation.NewKeyFromString(keyString)
	c.Assert(err, tc.ErrorIsNil)
	return key
}
