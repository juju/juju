// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type steps118Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps118Suite{})

func (s *steps118Suite) TestStateStepsFor118(c *gc.C) {
	expected := []string{
		"update rsyslog port",
		"remove deprecated environment config settings",
		"migrate local provider agent config",
	}
	assertStateSteps(c, version.MustParse("1.18.0"), expected)
}

func (s *steps118Suite) TestStepsFor118(c *gc.C) {
	expected := []string{
		"make $DATADIR/locks owned by ubuntu:ubuntu",
		"generate system ssh key",
		"install rsyslog-gnutls",
		"make /home/ubuntu/.profile source .juju-proxy file",
	}
	assertSteps(c, version.MustParse("1.18.0"), expected)
}
