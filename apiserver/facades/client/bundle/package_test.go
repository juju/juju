// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package bundle_test -destination package_mock_test.go github.com/juju/juju/apiserver/facades/client/bundle NetworkService
func Test(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
