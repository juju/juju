// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/utils"
)

type networkSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&networkSuite{})

func (s *networkSuite) TestOpSuccess(c *gc.C) {
	isCalled := false
	f := func() error {
		isCalled = true
		return nil
	}
	err := utils.NetworkOperationWitDefaultRetries(f, "do it")()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isCalled, jc.IsTrue)
}

func (s *networkSuite) TestOpFailureNoRetry(c *gc.C) {
	s.PatchValue(&utils.DefaultNetworkOperationRetryDelay, 1*time.Millisecond)
	netErr := &netError{false}
	callCount := 0
	f := func() error {
		callCount++
		return netErr
	}
	err := utils.NetworkOperationWitDefaultRetries(f, "do it")()
	c.Assert(errors.Cause(err), gc.Equals, netErr)
	c.Assert(callCount, gc.Equals, 1)
}

func (s *networkSuite) TestOpFailureRetries(c *gc.C) {
	s.PatchValue(&utils.DefaultNetworkOperationRetryDelay, 1*time.Millisecond)
	netErr := &netError{true}
	callCount := 0
	f := func() error {
		callCount++
		return netErr
	}
	err := utils.NetworkOperationWitDefaultRetries(f, "do it")()
	c.Assert(errors.Cause(err), gc.Equals, netErr)
	c.Assert(callCount, gc.Equals, 10)
}

func (s *networkSuite) TestOpNestedFailureRetries(c *gc.C) {
	s.PatchValue(&utils.DefaultNetworkOperationRetryDelay, 1*time.Millisecond)
	netErr := &netError{true}
	callCount := 0
	f := func() error {
		callCount++
		return errors.Annotate(errors.Trace(netErr), "create a wrapped error")
	}
	err := utils.NetworkOperationWitDefaultRetries(f, "do it")()
	c.Assert(errors.Cause(err), gc.Equals, netErr)
	c.Assert(callCount, gc.Equals, 10)
}

func (s *networkSuite) TestOpSucceedsAfterRetries(c *gc.C) {
	s.PatchValue(&utils.DefaultNetworkOperationRetryDelay, 1*time.Millisecond)
	netErr := &netError{true}
	callCount := 0
	f := func() error {
		callCount++
		if callCount == 5 {
			return nil
		}
		return netErr
	}
	err := utils.NetworkOperationWitDefaultRetries(f, "do it")()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(callCount, gc.Equals, 5)
}

type netError struct {
	temporary bool
}

func (e *netError) Error() string {
	return "network error"
}

func (e *netError) Temporary() bool {
	return e.temporary
}

func (e *netError) Timeout() bool {
	return false
}
