// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package context -destination context_mock_test.go github.com/juju/juju/environs/context ModelCredentialInvalidator
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/context_mock.go github.com/juju/juju/environs/context ProviderCallContext
func Test(t *testing.T) {
	gc.TestingT(t)
}
