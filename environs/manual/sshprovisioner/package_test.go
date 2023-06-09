// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshprovisioner_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package sshprovisioner_test -destination domain_mock_test.go github.com/juju/juju/environs/manual/sshprovisioner ControllerConfigGetter

func Test(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
