// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"net"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type safemodeSuite struct {
	testhelpers.IsolationSuite
}

func TestSafemodeSuite(t *testing.T) {
	tc.Run(t, &safemodeSuite{})
}

func (s *safemodeSuite) TestEnsuringControllerNotRunningPortUnset(c *tc.C) {
	err := ensuringControllerNotRunning(0)
	c.Check(err, tc.ErrorIsNil)
}

func (s *safemodeSuite) TestEnsuringControllerNotRunningDialTimeout(c *tc.C) {
	// Find an unused port that has no listener.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, tc.ErrorIsNil)
	port := l.Addr().(*net.TCPAddr).Port
	c.Assert(l.Close(), tc.ErrorIsNil)

	// Dial to a port with no listener should timeout and return nil.
	err = ensuringControllerNotRunning(port)
	c.Check(err, tc.ErrorIsNil)
}

func (s *safemodeSuite) TestEnsuringControllerNotRunningControllerActive(c *tc.C) {
	// Start a listener on a random port to simulate a running controller.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, tc.ErrorIsNil)
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	err = ensuringControllerNotRunning(port)
	c.Check(err, tc.ErrorIs, errors.AlreadyExists)
}
