// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package migrationtarget_test -destination domain_mock_test.go github.com/juju/juju/apiserver/facades/controller/migrationtarget ControllerConfigGetter

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
