// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package providertracker -destination provider_mock_test.go github.com/juju/juju/core/providertracker ProviderFactory,Provider

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}
