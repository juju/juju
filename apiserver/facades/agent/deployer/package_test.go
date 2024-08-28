// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	stdtesting "testing"

	coretesting "github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/domain_mock.go github.com/juju/juju/apiserver/facades/agent/deployer ControllerConfigGetter,ApplicationService

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}
