// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslog

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

// Stub stubs out the external functions used in the syslog package.
type Stub struct {
	testing.Stub

	Euid int
}

// Geteuid is a stub for os.Geteuid.
func (s *Stub) Geteuid() int {
	s.AddCall("Geteuid")

	// Pop off the err, even though we don't return it.
	s.NextErr()
	return s.Euid
}

// Restart is a stub for service.Restart.
func (s *Stub) Restart(name string) error {
	s.AddCall("Restart", name)

	return s.NextErr()
}

// BaseSuite is the base suite for use in tests in the syslog package.
type BaseSuite struct {
	testing.IsolationSuite

	Stub *Stub
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.Stub = &Stub{}
	s.PatchValue(&getEuid, s.Stub.Geteuid)
	s.PatchValue(&restart, s.Stub.Restart)
}
