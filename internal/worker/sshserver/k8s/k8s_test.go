// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8s

import (
	"github.com/google/uuid"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	k8sexec "github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/core/virtualhostname"
)

type k8sSuite struct {
}

var _ = gc.Suite(&k8sSuite{})

func (s *k8sSuite) TestNewHandler(c *gc.C) {
	mockResolver := &MockResolver{}
	mockLogger := loggo.GetLogger("test")
	mockGetExecutor := func(name string) (k8sexec.Executor, error) {
		return &MockExecutor{}, nil
	}
	destination, err := virtualhostname.NewInfoContainerTarget(uuid.NewString(), "test/0", "test-container")
	c.Assert(err, jc.ErrorIsNil)

	// Test case: All dependencies provided
	handler, err := NewHandlers(destination, mockResolver, mockLogger, mockGetExecutor)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(handler, gc.NotNil)

	// Test case: Resolver is nil
	handler, err = NewHandlers(destination, nil, mockLogger, mockGetExecutor)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, "k8s resolver is required not valid")
	c.Assert(handler, gc.IsNil)

	// Test case: Logger is nil
	handler, err = NewHandlers(destination, mockResolver, nil, mockGetExecutor)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, "logger is required not valid")
	c.Assert(handler, gc.IsNil)

	// Test case: GetExecutor is nil
	handler, err = NewHandlers(destination, mockResolver, mockLogger, nil)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, "executor is required not valid")
	c.Assert(handler, gc.IsNil)

	// Test case: Invalid destination
	handler, err = NewHandlers(virtualhostname.Info{}, mockResolver, mockLogger, mockGetExecutor)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, "destination must be a container target not valid")
	c.Assert(handler, gc.IsNil)

}
