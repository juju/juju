// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/objectstore_mock.go github.com/juju/juju/core/objectstore Session

func TestAll(t *testing.T) {
	gc.TestingT(t)
}
