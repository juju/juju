// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http_test

import (
	"testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/http_mock.go github.com/juju/juju/api/http HTTPClient
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/httpdoer_mock.go github.com/juju/juju/api/http HTTPDoer

func TestAll(t *testing.T) {
	tc.TestingT(t)
}
