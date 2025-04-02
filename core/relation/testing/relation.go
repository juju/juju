// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/relation"
)

// GenRelationUUID can be used in testing for generating a relation UUID
// that is checked for subsequent errors using the test suite's go check
// instance.
func GenRelationUUID(c *gc.C) relation.UUID {
	id, err := relation.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return id
}

// GenRelationUnitUUID can be used in testing for generating a relation
// Unit UUID that is checked for subsequent errors using the test suite's
// go check instance.
func GenRelationUnitUUID(c *gc.C) relation.UnitUUID {
	id, err := relation.NewUnitUUID()
	c.Assert(err, jc.ErrorIsNil)
	return id
}

// GenNewKey can be used in testing to generate a relation key from its string
// representation. It is checked for errors using the test suite's go check
// instance.
func GenNewKey(c *gc.C, keyString string) relation.Key {
	key, err := relation.NewKeyFromString(keyString)
	c.Assert(err, jc.ErrorIsNil)
	return key
}
