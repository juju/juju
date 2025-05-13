// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imageutils_test

import (
	"testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package imageutils_test -destination environs_mock_test.go github.com/juju/juju/environs CredentialInvalidator

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}
