// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle_test

import (
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/charm"
)

type mockCharm struct {
	charm.Charm
}

func (c *mockCharm) Config() *charm.Config {
	return &charm.Config{Options: map[string]charm.Option{
		"foo": {Default: "bar"},
	}}
}

type mockObjectStore struct {
	objectstore.ObjectStore
}
