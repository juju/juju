// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/apibase_mock.go github.com/juju/juju/api/base APICallCloser
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/httprequest_mock.go gopkg.in/httprequest.v1 Doer

