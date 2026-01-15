// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsrevoker

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package secretsrevoker_test -destination mocks_test.go github.com/juju/juju/internal/worker/secretsrevoker Logger,SecretsFacade

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
