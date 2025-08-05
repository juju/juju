// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fanconfigurer_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package fanconfigurer_test -destination state_mock_test.go github.com/juju/juju/apiserver/facades/agent/fanconfigurer MachineAccessor,ModelAccessor,Machine
func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
