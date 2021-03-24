// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type UpdateSeriesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&UpdateSeriesSuite{})
