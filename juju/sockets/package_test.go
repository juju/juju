// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sockets_test

import (
	stdtesting "testing"

	coretesting "github.com/juju/juju/v3/testing"
)

func Test(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}
