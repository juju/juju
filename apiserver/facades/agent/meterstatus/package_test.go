// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus_test

import (
	stdtesting "testing"

	coretesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package meterstatus_test -destination meterstatus_mock_test.go github.com/juju/juju/apiserver/facades/agent/meterstatus MeterStatusState
//go:generate go run go.uber.org/mock/mockgen -package meterstatus_test -destination domain_mock_test.go github.com/juju/juju/apiserver/facades/agent/meterstatus ControllerConfigGetter

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}
