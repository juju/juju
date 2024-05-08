// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/modelextractor_mock.go github.com/juju/juju/cmd/juju/application/bundle ModelExtractor
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/bundledatasource_mock.go github.com/juju/juju/cmd/juju/application/bundle BundleDataSource
