// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	stdtesting "testing"

	coretesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package machine_test -destination domain_mock_test.go github.com/juju/juju/apiserver/facades/agent/machine ControllerConfigGetter

func Test(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}
