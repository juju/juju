// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"testing"

	gc "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

var (
	NewFacadeLegacyForTest  = newFacadeLegacy
	NewFacadeSidecarForTest = newFacadeSidecar
)
