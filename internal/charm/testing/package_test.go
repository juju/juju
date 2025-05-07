// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testing_test

import (
	"testing"

	"github.com/juju/tc"
)

func Test(t *testing.T) {
	tc.TestingT(t)
}
