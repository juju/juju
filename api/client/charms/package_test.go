// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/s3caller_mock.go github.com/juju/juju/worker/s3caller Session

func TestAll(t *testing.T) {
	gc.TestingT(t)
}
