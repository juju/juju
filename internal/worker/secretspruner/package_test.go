// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretspruner

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/worker_mock.go github.com/juju/juju/internal/worker/secretspruner Logger,SecretsFacade

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
