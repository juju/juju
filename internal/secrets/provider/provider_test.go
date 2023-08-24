// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/secrets/provider"
)

type providerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&providerSuite{})

func (*providerSuite) TestRevisionMetaSlice(c *gc.C) {
	nameMeta := provider.SecretRevisions{}
	nameMeta.Add(&secrets.URI{ID: "a"}, "a-1")
	nameMeta.Add(&secrets.URI{ID: "b"}, "b-1", "b-2")
	nameMeta.Add(&secrets.URI{ID: "c"}, "c-1", "c-2", "c-3")
	nameMeta.Add(&secrets.URI{ID: "d"}, "d-1", "d-2", "d-3")
	nameMeta.Add(&secrets.URI{ID: "d"}, "d-4")
	c.Assert(nameMeta.RevisionIDs(), gc.DeepEquals, []string{
		"a-1",
		"b-1", "b-2",
		"c-1", "c-2", "c-3",
		"d-1", "d-2", "d-3", "d-4",
	})
}
