// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrain_test

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/facade_mock.go github.com/juju/juju/api/base FacadeCaller
