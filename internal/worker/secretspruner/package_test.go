// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretspruner

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/worker_mock.go github.com/juju/juju/internal/worker/secretspruner SecretsFacade

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}
