// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/authentication_mock.go github.com/juju/juju/apiserver/authentication ExpirableStorageBakery

func TestAll(t *testing.T) {
	gc.TestingT(t)
}
