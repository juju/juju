// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package secret -destination refcount_mock_test.go github.com/juju/juju/domain/secret/service SecretBackendReferenceMutator

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
