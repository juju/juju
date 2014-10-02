// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

func TestJoyent(t *testing.T) {
	registerLocalTests()
	gc.TestingT(t)
}
