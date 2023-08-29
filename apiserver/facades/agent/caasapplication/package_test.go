// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplication_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package caasapplication_test -destination domain_mock_test.go github.com/juju/juju/apiserver/facades/agent/caasapplication ControllerConfigGetter

func TestAll(t *testing.T) {
	gc.TestingT(t)
}
