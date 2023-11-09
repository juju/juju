// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type ObjectStoreSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ObjectStoreSuite{})

func (s *ObjectStoreSuite) TestObjectStore(c *gc.C) {
	tests := []struct {
		value    string
		expected string
		err      string
	}{{
		value:    "state",
		expected: "state",
	}, {
		value:    "file",
		expected: "file",
	}, {
		value: "inferi",
		err:   "object store type \"inferi\" not valid",
	}}

	for i, test := range tests {
		c.Logf("test %d: %s", i, test.value)

		backend, err := ParseObjectStoreType(test.value)
		if test.err != "" {
			c.Assert(err, gc.ErrorMatches, test.err)
			continue
		}
		c.Assert(err, gc.IsNil)
		c.Assert(backend, gc.Equals, BackendType(test.expected))
	}
}
