// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/google/uuid"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/virtualhostname"
)

type machineSuite struct {
}

var _ = gc.Suite(&machineSuite{})

func (s *machineSuite) TestNewHandler(c *gc.C) {
	mockConnector := &MockSSHConnector{}
	mockLogger := loggo.GetLogger("test")
	destination, err := virtualhostname.NewInfoMachineTarget(uuid.NewString(), "0")
	c.Assert(err, jc.ErrorIsNil)

	// Test case: All dependencies provided
	handler, err := NewHandlers(destination, mockConnector, mockLogger)
	c.Assert(err, gc.IsNil)
	c.Assert(handler, gc.NotNil)

	// Test case: Connector is nil
	handler, err = NewHandlers(destination, nil, mockLogger)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, "connector is required not valid")
	c.Assert(handler, gc.IsNil)

	// Test case: Logger is nil
	handler, err = NewHandlers(destination, mockConnector, nil)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, "logger is required not valid")
	c.Assert(handler, gc.IsNil)

	destination, err = virtualhostname.NewInfoContainerTarget(uuid.NewString(), "foo/0", "container")
	c.Assert(err, jc.ErrorIsNil)

	// Test case: Invalid destination
	handler, err = NewHandlers(destination, mockConnector, mockLogger)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, "destination must be a machine or unit target not valid")
	c.Assert(handler, gc.IsNil)
}
