// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/modelextractor_mock.go github.com/juju/juju/cmd/juju/application/bundle ModelExtractor
//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/bundledatasource_mock.go github.com/juju/juju/cmd/juju/application/bundle BundleDataSource
