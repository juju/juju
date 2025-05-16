// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/tracker_mock.go github.com/juju/juju/internal/worker/uniter/secrets SecretStateTracker
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/client_mock.go github.com/juju/juju/internal/worker/uniter/secrets SecretsClient

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}
