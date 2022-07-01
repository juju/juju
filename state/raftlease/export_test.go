// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/core/lease"
)

func AssertLeaseholderDocEquals(c *gc.C, doc interface{}, key lease.Key, holder string) {
	actual, ok := doc.(*leaseHolderDoc)
	c.Assert(ok, gc.Equals, true)
	expected, err := newLeaseHolderDoc(key.Namespace, key.ModelUUID, key.Lease, holder)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actual, gc.DeepEquals, expected)
}
