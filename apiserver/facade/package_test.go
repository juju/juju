// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facade_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/facade_mock.go github.com/juju/juju/apiserver/facade Resources,Authorizer,WatcherRegistry

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
