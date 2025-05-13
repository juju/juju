// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret

import (
	"testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package secret -destination backend_mock_test.go github.com/juju/juju/domain/secret/service SecretBackendState

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}
