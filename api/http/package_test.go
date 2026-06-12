// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http_test

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/http_mock.go github.com/juju/juju/api/http HTTPClient
//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/httpdoer_mock.go github.com/juju/juju/api/http HTTPDoer
