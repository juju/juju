// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/virtualhostname"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/sshserver/handlers/machine"
)

type proxySuite struct{}

func TestProxySuite(t *testing.T) {
	tc.Run(t, &proxySuite{})
}

func (s *proxySuite) TestNewSelectsMachineHandlers(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)

	handlers, err := (proxyFactory{
		logger:    loggertesting.WrapCheckLog(c),
		connector: proxyConnector{},
	}).New(c.Context(), destination)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(handlers, tc.FitsTypeOf, &machine.Handlers{})
}

type proxyConnector struct {
	machine.SSHConnector
}
