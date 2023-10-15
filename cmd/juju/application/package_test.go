// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	stdtesting "testing"

	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

// TODO(wallyworld) - convert tests moved across from commands package to not require mongo

func Test(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
