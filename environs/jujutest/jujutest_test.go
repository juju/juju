// Copyright 2011, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujutest

import (
	"testing"

	"github.com/juju/tc"
)

func Test(t *testing.T) {
	tc.TestingT(t)
}
