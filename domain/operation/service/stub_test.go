// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
)

type stubSuite struct {
}

func TestStubSuite(t *testing.T) {
	// Keep legacy runner but now we populate with real tests
	tc.Run(t, &stubSuite{})
}

// TestStub lists all tests that were done in the facade, which should be done here
// once implemented, are in the state layer (depending on the implementation)
// They are no longer needed in the facade.
func (s *stubSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- ListOperations querying by status.
- ListOperations querying by action names.
- ListOperations querying by application names.
- ListOperations querying by unit names.
- ListOperations querying by machines.
- ListOperations querying with multiple filters - result is union.
- Operations based on input entity tags.
- EnqueueOperation with some units
- EnqueueOperation but AddAction fails
- EnqueueOperation with a unit specified with a leader receiver
`)
}
