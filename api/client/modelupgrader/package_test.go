// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/apibase_mock.go github.com/juju/juju/api/base APICallCloser
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/httprequest_mock.go gopkg.in/httprequest.v1 Doer

func TestAll(t *testing.T) {
	gc.TestingT(t)
}
