// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/applicationapi_mock.go github.com/juju/juju/cmd/juju/application ApplicationAPI,RemoveApplicationAPI
//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/modelconfigapi_mock.go github.com/juju/juju/cmd/juju/application ModelConfigClient
//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/deployer_mock.go github.com/juju/juju/cmd/juju/application/deployer Deployer,DeployerFactory
//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/expose_mock.go github.com/juju/juju/cmd/juju/application ApplicationExposeAPI
