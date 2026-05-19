// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader_test

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/apibase_mock.go github.com/juju/juju/api/base APICallCloser
//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/httprequest_mock.go gopkg.in/httprequest.v1 Doer
