// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/http_mock.go github.com/juju/juju/api/http HTTPClient
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/httpdoer_mock.go github.com/juju/juju/api/http HTTPDoer

func Test(t *testing.T) {
	gc.TestingT(t)
}
