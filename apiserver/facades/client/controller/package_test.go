// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/domain_mock.go github.com/juju/juju/apiserver/facades/client/controller ControllerAccessService,ModelService,ModelInfoService
