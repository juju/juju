// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/charmhub_mock.go github.com/juju/juju/resource/resourceadapters CharmHub
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/opener_mock.go github.com/juju/juju/resource/resourceadapters ResourceOpenerState,Application,Unit,Resources
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/repositories_mock.go github.com/juju/juju/resource/repositories ResourceGetter
