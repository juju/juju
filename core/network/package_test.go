// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"testing"

	jujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

// BaseSuite exposes base testing functionality to the network tests,
// including patching package-private functions/variables.
type BaseSuite struct {
	jujutesting.IsolationSuite
}

// PatchGOOS allows us to simulate the running OS.
func (s *BaseSuite) PatchGOOS(os string) {
	s.PatchValue(&goos, func() string { return os })
}

// PatchUnIPRouteShow allows us to simulate the return from running
// "ip route show" on the host.
func (s *BaseSuite) PatchRunIPRouteShow(run func() (string, error)) {
	s.PatchValue(&runIPRouteShow, run)
}
