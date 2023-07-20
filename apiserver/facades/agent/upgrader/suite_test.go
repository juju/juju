// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	stdtesting "testing"

	coretesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package upgrader_test -destination domain_mock_test.go github.com/juju/juju/apiserver/facades/agent/upgrader ControllerConfigGetter

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}
