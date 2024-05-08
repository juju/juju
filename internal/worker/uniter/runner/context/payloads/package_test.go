// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloads_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination ../mocks/payload_mock.go github.com/juju/juju/internal/worker/uniter/runner/context/payloads PayloadAPIClient

func Test(t *testing.T) {
	gc.TestingT(t)
}
