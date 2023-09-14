// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package undertaker_test -destination facade_mock_test.go github.com/juju/juju/worker/undertaker Facade
//go:generate go run go.uber.org/mock/mockgen -package undertaker_test -destination credentialapi_mock_test.go github.com/juju/juju/worker/common CredentialAPI

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
