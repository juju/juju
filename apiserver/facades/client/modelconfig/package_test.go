// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/service_mock.go github.com/juju/juju/apiserver/facades/client/modelconfig SecretBackendService
func Test(t *testing.T) {
	gc.TestingT(t)
}
