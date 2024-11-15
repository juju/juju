// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination package_mock_test.go github.com/juju/juju/domain/application/service ApplicationState,CharmState,DeleteSecretState,ResourceState,ResourceStoreGetter,WatcherFactory,AgentVersionGetter,Provider
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination charm_mock_test.go github.com/juju/juju/internal/charm Charm

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

func ptr[T any](v T) *T {
	return &v
}
