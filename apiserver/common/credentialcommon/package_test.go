// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialcommon_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

// At the moment mocks generated here are in use in the apiserver/facades/client/cloud unit tests.
//
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/credentialcommon_mock.go github.com/juju/juju/apiserver/common/credentialcommon Model,PersistentBackend
func Test(t *stdtesting.T) {
	gc.TestingT(t)
}
