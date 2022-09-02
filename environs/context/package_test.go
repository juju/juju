// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package context -destination context_mock_test.go github.com/juju/juju/environs/context ModelCredentialInvalidator
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/context_mock.go github.com/juju/juju/environs/context ProviderCallContext
func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
