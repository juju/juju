// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"context"
	"testing"

	"github.com/juju/tc"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/virtualhostname"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type machineSuite struct{}

func TestMachineSuite(t *testing.T) {
	tc.Run(t, &machineSuite{})
}

func (s *machineSuite) TestNewHandlers(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)

	handlers, err := NewHandlers(destination, stubConnector{}, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(handlers.destination, tc.Equals, destination)

	_, err = NewHandlers(destination, nil, loggertesting.WrapCheckLog(c))
	c.Check(err, tc.ErrorMatches, "connector is required")

	_, err = NewHandlers(destination, stubConnector{}, nil)
	c.Check(err, tc.ErrorMatches, "logger is required")

	container, err := virtualhostname.NewInfoContainerTarget("8419cd78-4993-4c3a-928e-c646226beeee", "app/0", "workload")
	c.Assert(err, tc.ErrorIsNil)
	_, err = NewHandlers(container, stubConnector{}, loggertesting.WrapCheckLog(c))
	c.Check(err, tc.ErrorMatches, "destination must be a machine or unit target")
}

type stubConnector struct{}

func (stubConnector) Connect(context.Context, virtualhostname.Info) (*gossh.Client, error) {
	return nil, nil
}
