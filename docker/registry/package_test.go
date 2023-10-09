// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/http_mock.go net/http RoundTripper
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/registry_mock.go github.com/juju/juju/docker/registry/internal Registry

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
