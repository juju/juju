// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"testing"

	"github.com/juju/tc"
)

func TestPatches(t *testing.T) {
	tc.Run(t, &patchesSuite{})
}

type patchesSuite struct{}

func (s *patchesSuite) TestReadPatches(c *tc.C) {

	// Writes tests for readPatches function.
	patches, postPatches := readPatches(entries, fs, func(s string) string {
		return
	})
}
