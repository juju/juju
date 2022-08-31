// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/apibase.go github.com/juju/juju/api/base APICaller

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
