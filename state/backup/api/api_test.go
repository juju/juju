// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&BackupSuite{})

type BackupSuite struct {
	testing.BaseSuite
}
