// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/keymanager_mock.go github.com/juju/juju/apiserver/facades/client/keymanager Model,BlockChecker

func TestAll(t *testing.T) {
	gc.TestingT(t)
}
