// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/testhelpers"
)

type typesSuite struct {
	testhelpers.IsolationSuite
}

func TestTypesSuite(t *testing.T) {
	tc.Run(t, &typesSuite{})
}

func (s *typesSuite) TestNamesValues(c *tc.C) {
	n := storage.Names{"a", "b", "c", "a", ""}
	c.Assert(n.Values(), tc.SameContents, []string{"a", "b", "c"})
}

func (s *typesSuite) TestProvidersValues(c *tc.C) {
	p := storage.Providers{"x", "y", "z", "x", ""}
	c.Assert(p.Values(), tc.SameContents, []string{"x", "y", "z"})
}
