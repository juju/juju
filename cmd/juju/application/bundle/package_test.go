// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/modelextractor_mock.go github.com/juju/juju/cmd/juju/application/bundle ModelExtractor
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/bundledatasource_mock.go github.com/juju/juju/cmd/juju/application/bundle BundleDataSource
