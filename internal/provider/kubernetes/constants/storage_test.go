// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constants_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
)

// TestWorkloadAndOperatorStorageKeysEqualBootstrapValues tests that the
// cloud config keys defined by the provider are the same values
// registered in environs/bootstrap for routing cloud attributes at
// bootstrap time.
//
// The two definitions are deliberately duplicated to avoid coupling the
// provider to the bootstrap package; this test makes any drift fail
// loudly.
func TestWorkloadAndOperatorStorageKeysEqualBootstrapValues(t *testing.T) {
	tc.Assert(t, constants.WorkloadStorageKey, tc.Equals, bootstrap.WorkloadStorageKey)
	tc.Assert(t, constants.OperatorStorageKey, tc.Equals, bootstrap.OperatorStorageKey)
}
