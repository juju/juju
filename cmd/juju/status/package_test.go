// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(dimitern): bug http://pad.lv/1425569
// Disabled until we have time to fix these tests on i386 properly.
//
// +build !386

package status

import (
	stdtesting "testing"

	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
