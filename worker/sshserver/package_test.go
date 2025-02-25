// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/facade_client_mock.go github.com/juju/juju/worker/sshserver FacadeClient

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
