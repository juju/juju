// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination ../mocks/resource_mock.go github.com/juju/juju/internal/worker/uniter/runner/context/resources OpenedResourceClient

func Test(t *testing.T) {
	gc.TestingT(t)
}
