// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/charmhub_mock.go github.com/juju/juju/internal/resource CharmHub
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/cache_mock.go github.com/juju/juju/internal/resource Resources
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/opener_mock.go github.com/juju/juju/internal/resource ResourceGetter,ResourceDownloadLock
