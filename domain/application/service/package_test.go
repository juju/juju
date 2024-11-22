// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination package_mock_test.go github.com/juju/juju/domain/application/service ApplicationState,CharmState,DeleteSecretState,ResourceState,ResourceStoreGetter,WatcherFactory,AgentVersionGetter,Provider
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination charm_mock_test.go github.com/juju/juju/internal/charm Charm

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

func ptr[T any](v T) *T {
	return &v
}

type changeEvent struct {
	typ       changestream.ChangeType
	namespace string
	changed   string
}

var _ changestream.ChangeEvent = (*changeEvent)(nil)

func (c *changeEvent) Type() changestream.ChangeType {
	return c.typ
}

func (c *changeEvent) Namespace() string {
	return c.namespace
}

func (c *changeEvent) Changed() string {
	return c.changed
}
