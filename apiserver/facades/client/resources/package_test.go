// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"testing"

	"github.com/juju/charm/v11"
	charmresource "github.com/juju/charm/v11/resource"
	gc "gopkg.in/check.v1"

	corecharm "github.com/juju/juju/core/charm"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/charmhub.go github.com/juju/juju/apiserver/facades/client/resources CharmHub
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/logger.go github.com/juju/juju/apiserver/facades/client/resources Logger
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/backend.go github.com/juju/juju/apiserver/facades/client/resources Backend,NewCharmRepository

func NewResourceClient(client CharmHub, logger Logger) *CharmHubClient {
	c := &CharmHubClient{
		Client: client,
		logger: logger,
	}
	return c
}

func ResourceInfo(client *CharmHubClient, curl *charm.URL, origin corecharm.Origin, name string, revision int) (charmresource.Resource, error) {
	return client.resourceInfo(curl, origin, name, revision)
}
