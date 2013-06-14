// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc_test

import (
	stdtesting "testing"

	. "launchpad.net/gocheck"
)

func Test(t *stdtesting.T) { TestingT(t) }

type LxcSuite struct{}

var _ = Suite(&LxcSuite{})
