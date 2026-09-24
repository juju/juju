// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type ProvisioningOperationSuite struct {
	testhelpers.IsolationSuite
}

func TestProvisioningOperationSuite(t *testing.T) {
	tc.Run(t, &ProvisioningOperationSuite{})
}

func (s *ProvisioningOperationSuite) TestIsDifferentOperation(c *tc.C) {
	tests := []struct {
		name        string
		currentOp   ProvisioningOperation
		requestedOp ProvisioningOperation
		expected    bool
	}{
		{
			// A same operation request must not be treated as a
			// conflicting operation, so it can proceed as an
			// idempotent continuation.
			name:        "same scale operation is not conflicting",
			currentOp:   ScaleOperation,
			requestedOp: ScaleOperation,
			expected:    false,
		},
		{
			name:        "same storage update operation is not conflicting",
			currentOp:   StorageUpdateOperation,
			requestedOp: StorageUpdateOperation,
			expected:    false,
		},
		{
			// No operation in progress allows any new operation to
			// proceed.
			name:        "no operation allows scale operation",
			currentOp:   NoOperation,
			requestedOp: ScaleOperation,
			expected:    false,
		},
		{
			name:        "no operation allows storage update operation",
			currentOp:   NoOperation,
			requestedOp: StorageUpdateOperation,
			expected:    false,
		},
		{
			// A scaling operation in progress blocks a conflicting
			// storage update operation.
			name:        "scale operation blocks storage update operation",
			currentOp:   ScaleOperation,
			requestedOp: StorageUpdateOperation,
			expected:    true,
		},
		{
			// A storage update operation in progress blocks a
			// conflicting scale operation.
			name:        "storage update operation blocks scale operation",
			currentOp:   StorageUpdateOperation,
			requestedOp: ScaleOperation,
			expected:    true,
		},
	}

	for _, test := range tests {
		c.Logf("test %q", test.name)
		c.Check(IsDifferentOperation(test.currentOp, test.requestedOp), tc.Equals, test.expected)
	}
}

func (s *ProvisioningOperationSuite) TestString(c *tc.C) {
	tests := []struct {
		op       ProvisioningOperation
		expected string
	}{
		{op: NoOperation, expected: "idle"},
		{op: ScaleOperation, expected: "scale"},
		{op: StorageUpdateOperation, expected: "storage update"},
		{op: ProvisioningOperation("unknown"), expected: "unknown"},
	}

	for _, test := range tests {
		c.Check(test.op.String(), tc.Equals, test.expected)
	}
}
