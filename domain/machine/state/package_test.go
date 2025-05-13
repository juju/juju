// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"
)

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}

func ptr[T any](v T) *T {
	return &v
}
