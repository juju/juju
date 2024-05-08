// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/auth_mock.go github.com/juju/juju/apiserver/facade Authorizer
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/resources_mock.go github.com/juju/juju/apiserver/facade Resources

func TestAll(t *testing.T) {
	gc.TestingT(t)
}
