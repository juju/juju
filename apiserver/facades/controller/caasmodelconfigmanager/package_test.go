// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/auth_mock.go github.com/juju/juju/apiserver/facade Authorizer
