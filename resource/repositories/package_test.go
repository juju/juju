// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package repositories_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/mocks.go github.com/juju/juju/resource/repositories EntityRepository,ResourceGetter

func Test(t *testing.T) {
	gc.TestingT(t)
}
